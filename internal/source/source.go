package source

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/druejaramillo/skills-cli/internal/config"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const (
	AgentsManifestVersion = 1
	AgentsManifestPrefix  = "<!-- skills-agents:"

	maxAgentsMarkerBytes = 1 << 20
)

type Skill struct {
	Name         string
	Path         string
	RelativePath string
}

type AgentsFragment struct {
	Path         string
	RelativePath string
	Manifest     *AgentsManifest
	Unclassified bool

	manifestInvalid bool
}

type AgentsScope string

const (
	AgentsScopeProject   AgentsScope = "project"
	AgentsScopeDirectory AgentsScope = "directory"
)

type AgentsRelationship string

const (
	AgentsRelationshipAll AgentsRelationship = "all"
	AgentsRelationshipAny AgentsRelationship = "any"
)

type AgentsLayer string

const (
	AgentsLayerCore         AgentsLayer = "core"
	AgentsLayerRoot         AgentsLayer = "root"
	AgentsLayerScoped       AgentsLayer = "scoped"
	AgentsLayerStack        AgentsLayer = "stack"
	AgentsLayerVerification AgentsLayer = "verification"
)

type AgentsGuidanceRelationship string

const (
	AgentsGuidanceAdditive  AgentsGuidanceRelationship = "additive"
	AgentsGuidanceException AgentsGuidanceRelationship = "exception"
)

type AgentsContentMarker struct {
	Path     string `json:"path"`
	Contains string `json:"contains"`
}

// AgentsManifest is the JSON object in a leading agents HTML comment.
// Evidence is evaluated relative to the project or target directory selected by Scope.
type AgentsManifest struct {
	Version              int                        `json:"version"`
	ID                   string                     `json:"id"`
	Layer                AgentsLayer                `json:"layer"`
	Scope                AgentsScope                `json:"scope"`
	Relationship         AgentsRelationship         `json:"relationship"`
	TargetPaths          []string                   `json:"target_paths,omitempty"`
	GuidanceRelationship AgentsGuidanceRelationship `json:"guidance_relationship,omitempty"`
	Owner                string                     `json:"owner"`
	Canonical            []string                   `json:"canonical"`
	ReviewOn             []string                   `json:"review_on"`
	Paths                []string                   `json:"paths,omitempty"`
	Globs                []string                   `json:"globs,omitempty"`
	ContentMarkers       []AgentsContentMarker      `json:"content_markers,omitempty"`
}

type AgentsDiagnostic struct {
	Path    string
	Message string
}

// AgentsDiagnostics separates non-blocking legacy warnings from invalid source errors.
type AgentsDiagnostics struct {
	Warnings []AgentsDiagnostic
	Errors   []AgentsDiagnostic
}

func (diagnostics AgentsDiagnostics) HasErrors() bool {
	return len(diagnostics.Errors) != 0
}

func (diagnostics AgentsDiagnostics) Err() error {
	if !diagnostics.HasErrors() {
		return nil
	}
	messages := make([]string, 0, len(diagnostics.Errors))
	for _, diagnostic := range diagnostics.Errors {
		if diagnostic.Path == "" {
			messages = append(messages, diagnostic.Message)
			continue
		}
		messages = append(messages, diagnostic.Path+": "+diagnostic.Message)
	}
	return errors.New(strings.Join(messages, "; "))
}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) || len(name) > 64 {
		return fmt.Errorf("%q must use lowercase letters, numbers, and single hyphens (1-64 characters)", name)
	}
	return nil
}

func AgentsFragmentPath(root, reference string) (string, string, error) {
	if reference == "" || filepath.IsAbs(reference) || strings.Contains(reference, "\\") {
		return "", "", fmt.Errorf("invalid agents fragment path %q", reference)
	}
	parts := strings.Split(reference, "/")
	for _, part := range parts {
		if err := ValidateName(part); err != nil {
			return "", "", fmt.Errorf("invalid agents fragment path %q: %w", reference, err)
		}
	}
	relative := "agents-md/" + strings.Join(parts, "/") + ".md"
	return filepath.Join(root, filepath.FromSlash(relative)), relative, nil
}

// ParseAgentsManifest reads the optional leading agents manifest without reading a file.
// A nil manifest and present=false identifies a legacy unclassified fragment.
func ParseAgentsManifest(markdown []byte) (manifest *AgentsManifest, present bool, err error) {
	manifest, present, _, err = parseAgentsManifest(markdown)
	return manifest, present, err
}

// ValidateAgentsManifest validates the schema and all source-relative references.
func ValidateAgentsManifest(manifest AgentsManifest) error {
	problems := agentsManifestProblems(manifest)
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// ValidateAgentsFragments reports manifest errors, duplicate IDs, and legacy fragments.
// Use InspectAgentsFragments to also validate on-disk content, including blank files.
func ValidateAgentsFragments(fragments []AgentsFragment) AgentsDiagnostics {
	var diagnostics AgentsDiagnostics
	ids := make(map[string]string)
	for _, fragment := range fragments {
		fragmentPath := agentsFragmentDiagnosticPath(fragment)
		if fragment.Manifest == nil {
			switch {
			case fragment.Unclassified:
				diagnostics.Warnings = append(diagnostics.Warnings, AgentsDiagnostic{
					Path:    fragmentPath,
					Message: "legacy fragment has no manifest and is unclassified",
				})
			case !fragment.manifestInvalid:
				diagnostics.Errors = append(diagnostics.Errors, AgentsDiagnostic{
					Path:    fragmentPath,
					Message: "manifest is missing or invalid",
				})
			}
			continue
		}

		for _, problem := range agentsManifestProblems(*fragment.Manifest) {
			diagnostics.Errors = append(diagnostics.Errors, AgentsDiagnostic{Path: fragmentPath, Message: problem})
		}
		if fragment.Manifest.ID == "" {
			continue
		}
		if previous, found := ids[fragment.Manifest.ID]; found {
			diagnostics.Errors = append(diagnostics.Errors, AgentsDiagnostic{
				Path:    fragmentPath,
				Message: fmt.Sprintf("duplicate manifest id %q (also used by %s)", fragment.Manifest.ID, previous),
			})
			continue
		}
		ids[fragment.Manifest.ID] = fragmentPath
	}
	return diagnostics
}

func ValidateAgentsFragment(path string) error {
	if filepath.Ext(path) != ".md" {
		return fmt.Errorf("agents fragment %q is not a Markdown file", path)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("agents fragment %q does not exist", path)
	}
	if err != nil {
		return fmt.Errorf("inspect agents fragment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("agents fragment %q is not a regular file", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read agents fragment %q: %w", path, err)
	}
	manifest, present, body, err := parseAgentsManifest(contents)
	if err != nil {
		return fmt.Errorf("invalid agents fragment manifest in %q: %w", path, err)
	}
	if strings.TrimSpace(string(body)) == "" {
		return fmt.Errorf("agents fragment %q is empty", path)
	}
	if !present {
		return fmt.Errorf("agents fragment %q must begin with an agents manifest", path)
	}
	if err := ValidateAgentsManifest(*manifest); err != nil {
		return fmt.Errorf("invalid agents fragment manifest in %q: %w", path, err)
	}
	return nil
}

func parseAgentsManifest(markdown []byte) (*AgentsManifest, bool, []byte, error) {
	if hasLeadingYAMLFrontmatter(markdown) {
		return nil, false, markdown, errors.New("YAML frontmatter is not supported")
	}
	if !bytes.HasPrefix(markdown, []byte(AgentsManifestPrefix)) {
		if bytes.HasPrefix(bytes.TrimLeft(markdown, " \t\r\n"), []byte(AgentsManifestPrefix)) {
			return nil, true, markdown, errors.New("agents manifest must be the first content in the file")
		}
		return nil, false, markdown, nil
	}

	comment := markdown[len(AgentsManifestPrefix):]
	end := bytes.Index(comment, []byte("-->"))
	if end < 0 {
		return nil, true, nil, errors.New("agents manifest HTML comment is not closed")
	}
	encoded := bytes.TrimSpace(comment[:end])
	if len(encoded) == 0 || encoded[0] != '{' {
		return nil, true, markdown[end+3:], errors.New("agents manifest must be a JSON object")
	}
	var manifest AgentsManifest
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, true, markdown[end+3:], fmt.Errorf("decode agents manifest JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, true, markdown[end+3:], errors.New("agents manifest must contain one JSON object")
		}
		return nil, true, markdown[end+3:], fmt.Errorf("decode agents manifest JSON: %w", err)
	}
	body := markdown[len(AgentsManifestPrefix)+end+3:]
	if hasLeadingYAMLFrontmatter(body) {
		return nil, true, body, errors.New("YAML frontmatter is not supported")
	}
	return &manifest, true, body, nil
}

func hasLeadingYAMLFrontmatter(markdown []byte) bool {
	trimmed := bytes.TrimLeft(markdown, " \t\r\n")
	lineEnd := bytes.IndexByte(trimmed, '\n')
	if lineEnd < 0 {
		lineEnd = len(trimmed)
	}
	return strings.TrimSpace(string(trimmed[:lineEnd])) == "---"
}

func agentsManifestProblems(manifest AgentsManifest) []string {
	var problems []string
	if manifest.Version != AgentsManifestVersion {
		problems = append(problems, fmt.Sprintf("version must be %d", AgentsManifestVersion))
	}
	if err := ValidateName(manifest.ID); err != nil {
		problems = append(problems, "id "+err.Error())
	}
	switch manifest.Layer {
	case AgentsLayerCore, AgentsLayerRoot, AgentsLayerScoped, AgentsLayerStack, AgentsLayerVerification:
	default:
		problems = append(problems, fmt.Sprintf("layer must be %q, %q, %q, %q, or %q", AgentsLayerCore, AgentsLayerRoot, AgentsLayerScoped, AgentsLayerStack, AgentsLayerVerification))
	}
	switch manifest.Scope {
	case AgentsScopeProject, AgentsScopeDirectory:
	default:
		problems = append(problems, fmt.Sprintf("scope must be %q or %q", AgentsScopeProject, AgentsScopeDirectory))
	}
	switch manifest.Relationship {
	case AgentsRelationshipAll, AgentsRelationshipAny:
	default:
		problems = append(problems, fmt.Sprintf("relationship must be %q or %q", AgentsRelationshipAll, AgentsRelationshipAny))
	}
	if strings.TrimSpace(manifest.Owner) == "" {
		problems = append(problems, "owner is required")
	}
	if len(manifest.Canonical) == 0 {
		problems = append(problems, "at least one canonical source is required")
	}
	problems = append(problems, agentsReferenceProblems("canonical", manifest.Canonical, validateAgentsRelativeReference)...)
	if len(manifest.ReviewOn) == 0 {
		problems = append(problems, "at least one review_on trigger is required")
	}
	for _, trigger := range manifest.ReviewOn {
		if strings.TrimSpace(trigger) == "" {
			problems = append(problems, "review_on trigger must not be blank")
		}
	}
	for _, target := range manifest.TargetPaths {
		if err := validateAgentsRelativeDirectory(target); err != nil {
			problems = append(problems, fmt.Sprintf("target path %q: %v", target, err))
		}
	}
	if manifest.Layer == AgentsLayerScoped {
		if len(manifest.TargetPaths) == 0 {
			problems = append(problems, "scoped fragments require at least one target path")
		}
		switch manifest.GuidanceRelationship {
		case AgentsGuidanceAdditive, AgentsGuidanceException:
		default:
			problems = append(problems, fmt.Sprintf("scoped guidance relationship must be %q or %q", AgentsGuidanceAdditive, AgentsGuidanceException))
		}
	} else if manifest.GuidanceRelationship != "" {
		problems = append(problems, "guidance relationship is only valid for scoped fragments")
	}
	problems = append(problems, agentsReferenceProblems("paths", manifest.Paths, validateAgentsRelativeReference)...)
	problems = append(problems, agentsReferenceProblems("globs", manifest.Globs, validateAgentsGlob)...)

	markers := make(map[string]struct{})
	for _, marker := range manifest.ContentMarkers {
		if err := validateAgentsRelativeReference(marker.Path); err != nil {
			problems = append(problems, fmt.Sprintf("content marker path %q: %v", marker.Path, err))
		}
		if strings.TrimSpace(marker.Contains) == "" {
			problems = append(problems, fmt.Sprintf("content marker %q must have non-empty contains text", marker.Path))
		}
		key := marker.Path + "\x00" + marker.Contains
		if _, found := markers[key]; found {
			problems = append(problems, fmt.Sprintf("content marker %q is duplicated", marker.Path))
		}
		markers[key] = struct{}{}
	}
	return problems
}

func agentsReferenceProblems(field string, references []string, validate func(string) error) []string {
	var problems []string
	seen := make(map[string]struct{})
	for _, reference := range references {
		if err := validate(reference); err != nil {
			problems = append(problems, fmt.Sprintf("%s reference %q: %v", field, reference, err))
		}
		if _, found := seen[reference]; found {
			problems = append(problems, fmt.Sprintf("%s reference %q is duplicated", field, reference))
		}
		seen[reference] = struct{}{}
	}
	return problems
}

func validateAgentsRelativeReference(reference string) error {
	if reference == "" || strings.ContainsRune(reference, '\x00') || strings.HasPrefix(reference, "/") || strings.Contains(reference, "\\") || reference == "." || reference == ".." || strings.HasPrefix(reference, "../") || path.Clean(reference) != reference {
		return errors.New("must be a canonical relative path")
	}
	return nil
}

func validateAgentsRelativeDirectory(directory string) error {
	if directory == "." {
		return nil
	}
	return validateAgentsRelativeReference(directory)
}

func validateAgentsGlob(glob string) error {
	if glob == "" || strings.ContainsRune(glob, '\x00') || strings.HasPrefix(glob, "/") || strings.Contains(glob, "\\") {
		return errors.New("must be a safe relative glob")
	}
	for _, part := range strings.Split(glob, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("must be a canonical relative glob")
		}
		if part == "**" {
			continue
		}
		if strings.Contains(part, "**") {
			return errors.New("recursive wildcard must occupy a complete path segment")
		}
		if _, err := path.Match(part, ""); err != nil {
			return fmt.Errorf("invalid glob pattern: %w", err)
		}
	}
	return nil
}

func agentsFragmentDiagnosticPath(fragment AgentsFragment) string {
	if fragment.RelativePath != "" {
		return fragment.RelativePath
	}
	return fragment.Path
}

func AddLocation(location string) (config.Source, error) {
	if location == "" {
		return config.Source{}, errors.New("source location is required")
	}
	info, err := os.Stat(location)
	if err == nil {
		if !info.IsDir() {
			return config.Source{}, fmt.Errorf("local source %q is not a directory", location)
		}
		absolute, err := filepath.Abs(location)
		if err != nil {
			return config.Source{}, fmt.Errorf("resolve local source: %w", err)
		}
		return config.Source{Location: absolute}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return config.Source{}, fmt.Errorf("inspect source location: %w", err)
	}

	if strings.Count(location, "/") == 1 && !strings.Contains(location, "://") && !strings.HasPrefix(location, "git@") {
		parts := strings.Split(location, "/")
		if parts[0] != "" && parts[1] != "" {
			return config.Source{Location: "https://github.com/" + location + ".git", Remote: true}, nil
		}
	}
	if strings.HasPrefix(location, "https://") || strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "ssh://") || strings.HasPrefix(location, "git@") || strings.HasPrefix(location, "file://") {
		return config.Source{Location: location, Remote: true}, nil
	}
	return config.Source{}, fmt.Errorf("%q is neither an existing directory nor a supported Git repository URL", location)
}

func Prepare(ctx context.Context, src config.Source, sourceName, cacheRoot string) (string, error) {
	if !src.Remote {
		info, err := os.Stat(src.Location)
		if err != nil {
			return "", fmt.Errorf("read local source: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("local source %q is not a directory", src.Location)
		}
		return src.Location, nil
	}
	if err := ValidateName(sourceName); err != nil {
		return "", fmt.Errorf("invalid source name: %w", err)
	}
	path := filepath.Join(cacheRoot, sourceName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
			return "", fmt.Errorf("create source cache: %w", err)
		}
		if err := runGit(ctx, "clone", "--depth=1", src.Location, path); err != nil {
			return "", fmt.Errorf("clone source %q: %w", sourceName, err)
		}
		return path, nil
	} else if err != nil {
		return "", fmt.Errorf("inspect source cache: %w", err)
	}

	if err := runGit(ctx, "-C", path, "fetch", "--depth=1", "origin", "HEAD"); err != nil {
		return "", fmt.Errorf("refresh source %q: %w", sourceName, err)
	}
	if err := runGit(ctx, "-C", path, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return "", fmt.Errorf("update source %q: %w", sourceName, err)
	}
	return path, nil
}

func runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}

func Discover(root string) ([]Skill, error) {
	var skills []Skill
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		skill, err := ValidateSkillDirectory(filepath.Dir(path))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, skill.Path)
		if err != nil {
			return fmt.Errorf("make skill path relative: %w", err)
		}
		skill.RelativePath = filepath.ToSlash(relative)
		skills = append(skills, skill)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan source: %w", err)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].RelativePath < skills[j].RelativePath })
	return skills, nil
}

// InspectAgentsFragments loads every Markdown fragment and reports source problems.
// Unmanifested legacy fragments are returned with Unclassified set and a warning.
func InspectAgentsFragments(root string) ([]AgentsFragment, AgentsDiagnostics, error) {
	fragments, err := discoverAgentsFragmentFiles(root)
	if err != nil {
		return nil, AgentsDiagnostics{}, err
	}

	var diagnostics AgentsDiagnostics
	for index := range fragments {
		contents, err := os.ReadFile(fragments[index].Path)
		if err != nil {
			return nil, AgentsDiagnostics{}, fmt.Errorf("read agents fragment %q: %w", fragments[index].Path, err)
		}
		manifest, present, body, err := parseAgentsManifest(contents)
		if err != nil {
			fragments[index].manifestInvalid = true
			diagnostics.Errors = append(diagnostics.Errors, AgentsDiagnostic{
				Path:    fragments[index].RelativePath,
				Message: "invalid manifest: " + err.Error(),
			})
			continue
		}
		empty := strings.TrimSpace(string(body)) == ""
		if empty {
			diagnostics.Errors = append(diagnostics.Errors, AgentsDiagnostic{
				Path:    fragments[index].RelativePath,
				Message: "fragment is empty",
			})
		}
		if !present {
			if empty {
				fragments[index].manifestInvalid = true
			} else {
				fragments[index].Unclassified = true
			}
			continue
		}
		fragments[index].Manifest = manifest
	}
	diagnostics.merge(ValidateAgentsFragments(fragments))
	return fragments, diagnostics, nil
}

// DiscoverAgentsFragments retains the original discovery API and rejects invalid
// manifests. Call InspectAgentsFragments when warnings need to be shown to users.
func DiscoverAgentsFragments(root string) ([]AgentsFragment, error) {
	fragments, diagnostics, err := InspectAgentsFragments(root)
	if err != nil {
		return nil, err
	}
	if err := diagnostics.Err(); err != nil {
		return nil, fmt.Errorf("validate agents fragments: %w", err)
	}
	return fragments, nil
}

func (diagnostics *AgentsDiagnostics) merge(other AgentsDiagnostics) {
	diagnostics.Warnings = append(diagnostics.Warnings, other.Warnings...)
	diagnostics.Errors = append(diagnostics.Errors, other.Errors...)
}

func discoverAgentsFragmentFiles(root string) ([]AgentsFragment, error) {
	fragmentsRoot := filepath.Join(root, "agents-md")
	info, err := os.Stat(fragmentsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("agents fragment directory %q does not exist", fragmentsRoot)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect agents fragment directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("agents fragment path %q is not a directory", fragmentsRoot)
	}

	var fragments []AgentsFragment
	err = filepath.WalkDir(fragmentsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("agents fragment %q must not be a symbolic link", path)
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect agents fragment %q: %w", path, err)
		}
		if !fileInfo.Mode().IsRegular() {
			return fmt.Errorf("agents fragment %q is not a regular file", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make agents fragment path relative: %w", err)
		}
		fragments = append(fragments, AgentsFragment{Path: path, RelativePath: filepath.ToSlash(relative)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan agents fragments: %w", err)
	}
	if len(fragments) == 0 {
		return nil, fmt.Errorf("agents fragment directory %q contains no Markdown files", fragmentsRoot)
	}
	sort.Slice(fragments, func(i, j int) bool { return fragments[i].RelativePath < fragments[j].RelativePath })
	return fragments, nil
}

// SelectAgentsFragments returns the fragments applicable to targetRelativeDir.
// targetRelativeDir must be "." or a canonical directory path relative to projectRoot.
// Callers should reject errors from ValidateAgentsFragments before selecting.
func SelectAgentsFragments(projectRoot, targetRelativeDir string, fragments []AgentsFragment) ([]AgentsFragment, error) {
	if err := validateAgentsRelativeDirectory(targetRelativeDir); err != nil {
		return nil, fmt.Errorf("invalid target relative directory %q: %w", targetRelativeDir, err)
	}
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	projectRoot, err = filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	projectInfo, err := os.Stat(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect project root: %w", err)
	}
	if !projectInfo.IsDir() {
		return nil, fmt.Errorf("project root %q is not a directory", projectRoot)
	}
	targetRoot, found, err := agentsProjectPath(projectRoot, targetRelativeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve target relative directory %q: %w", targetRelativeDir, err)
	}
	if !found {
		return nil, fmt.Errorf("target relative directory %q does not exist", targetRelativeDir)
	}
	targetInfo, err := os.Stat(targetRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect target relative directory %q: %w", targetRelativeDir, err)
	}
	if !targetInfo.IsDir() {
		return nil, fmt.Errorf("target relative directory %q is not a directory", targetRelativeDir)
	}

	selected := make([]AgentsFragment, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment.Manifest == nil {
			if !fragment.Unclassified {
				return nil, fmt.Errorf("agents fragment %q has an invalid manifest", agentsFragmentDiagnosticPath(fragment))
			}
			selected = append(selected, fragment)
			continue
		}
		if err := ValidateAgentsManifest(*fragment.Manifest); err != nil {
			return nil, fmt.Errorf("agents fragment %q has an invalid manifest: %w", agentsFragmentDiagnosticPath(fragment), err)
		}
		if !agentsLayerMatchesTarget(*fragment.Manifest, targetRelativeDir) {
			continue
		}

		evidenceRoot := projectRoot
		if fragment.Manifest.Scope == AgentsScopeDirectory {
			evidenceRoot = targetRoot
		}
		eligible, err := agentsManifestMatches(evidenceRoot, *fragment.Manifest)
		if err != nil {
			return nil, fmt.Errorf("select agents fragment %q: %w", agentsFragmentDiagnosticPath(fragment), err)
		}
		if eligible {
			selected = append(selected, fragment)
		}
	}
	return selected, nil
}

func agentsLayerMatchesTarget(manifest AgentsManifest, targetRelativeDir string) bool {
	switch manifest.Layer {
	case AgentsLayerCore:
		return true
	case AgentsLayerRoot:
		return targetRelativeDir == "."
	case AgentsLayerScoped:
		return agentsTargetMatches(manifest.TargetPaths, targetRelativeDir)
	case AgentsLayerStack, AgentsLayerVerification:
		return len(manifest.TargetPaths) == 0 || agentsTargetMatches(manifest.TargetPaths, targetRelativeDir)
	default:
		return false
	}
}

func agentsTargetMatches(scopes []string, targetRelativeDir string) bool {
	for _, scope := range scopes {
		if scope == "." || targetRelativeDir == scope || strings.HasPrefix(targetRelativeDir, scope+"/") {
			return true
		}
	}
	return false
}

func agentsManifestMatches(root string, manifest AgentsManifest) (bool, error) {
	matches := make([]bool, 0, len(manifest.Paths)+len(manifest.Globs)+len(manifest.ContentMarkers))
	for _, reference := range manifest.Paths {
		match, err := agentsPathExists(root, reference)
		if err != nil {
			return false, err
		}
		matches = append(matches, match)
	}
	for _, glob := range manifest.Globs {
		match, err := agentsGlobMatches(root, glob)
		if err != nil {
			return false, err
		}
		matches = append(matches, match)
	}
	for _, marker := range manifest.ContentMarkers {
		match, err := agentsContentMarkerMatches(root, marker)
		if err != nil {
			return false, err
		}
		matches = append(matches, match)
	}
	if len(matches) == 0 {
		return true, nil
	}
	if manifest.Relationship == AgentsRelationshipAny {
		for _, match := range matches {
			if match {
				return true, nil
			}
		}
		return false, nil
	}
	for _, match := range matches {
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func agentsPathExists(root, reference string) (bool, error) {
	filePath, found, err := agentsProjectPath(root, reference)
	if err != nil {
		return false, fmt.Errorf("resolve evidence path %q: %w", reference, err)
	}
	if !found {
		return false, nil
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return false, fmt.Errorf("inspect evidence path %q: %w", reference, err)
	}
	return (info.Mode().IsRegular() || info.IsDir()) && info.Mode()&os.ModeSymlink == 0, nil
}

func agentsGlobMatches(root, glob string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		if agentsGlobMatch(glob, filepath.ToSlash(relative)) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("scan glob %q: %w", glob, err)
	}
	return found, nil
}

func agentsGlobMatch(glob, value string) bool {
	globParts := strings.Split(glob, "/")
	valueParts := strings.Split(value, "/")
	type position struct{ glob, value int }
	memo := make(map[position]bool)
	var match func(int, int) bool
	match = func(globIndex, valueIndex int) bool {
		current := position{glob: globIndex, value: valueIndex}
		if result, found := memo[current]; found {
			return result
		}
		var result bool
		if globIndex == len(globParts) {
			result = valueIndex == len(valueParts)
		} else if globParts[globIndex] == "**" {
			for index := valueIndex; index <= len(valueParts); index++ {
				if match(globIndex+1, index) {
					result = true
					break
				}
			}
		} else if valueIndex < len(valueParts) {
			matched, err := path.Match(globParts[globIndex], valueParts[valueIndex])
			result = err == nil && matched && match(globIndex+1, valueIndex+1)
		}
		memo[current] = result
		return result
	}
	return match(0, 0)
}

func agentsContentMarkerMatches(root string, marker AgentsContentMarker) (bool, error) {
	filePath, found, err := agentsProjectPath(root, marker.Path)
	if err != nil {
		return false, fmt.Errorf("resolve content marker %q: %w", marker.Path, err)
	}
	if !found {
		return false, nil
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return false, fmt.Errorf("inspect content marker %q: %w", marker.Path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("read content marker %q: %w", marker.Path, err)
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxAgentsMarkerBytes+1))
	if err != nil {
		return false, fmt.Errorf("read content marker %q: %w", marker.Path, err)
	}
	if len(contents) > maxAgentsMarkerBytes {
		return false, nil
	}
	return bytes.Contains(contents, []byte(marker.Contains)), nil
}

func agentsProjectPath(root, reference string) (string, bool, error) {
	candidate := filepath.Join(root, filepath.FromSlash(reference))
	resolved, err := filepath.EvalSymlinks(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", false, err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false, nil
	}
	return resolved, true, nil
}

func ValidateSkillDirectory(path string) (Skill, error) {
	file, err := os.Open(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("read skill %q: %w", path, err)
	}
	defer file.Close()

	frontmatterName, description, err := parseFrontmatter(file)
	if err != nil {
		return Skill{}, fmt.Errorf("invalid SKILL.md in %q: %w", path, err)
	}
	if err := ValidateName(frontmatterName); err != nil {
		return Skill{}, fmt.Errorf("invalid frontmatter name: %w", err)
	}
	if description == "" {
		return Skill{}, fmt.Errorf("invalid SKILL.md in %q: description is required", path)
	}
	return Skill{Name: frontmatterName, Path: path}, nil
}

func ReadSkillFrontmatter(path string) (name, description string, present bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", false, fmt.Errorf("read skill frontmatter %q: %w", path, err)
	}
	defer file.Close()

	return parseOptionalFrontmatter(file)
}

func parseFrontmatter(file *os.File) (string, string, error) {
	name, description, present, err := parseOptionalFrontmatter(file)
	if err != nil {
		return "", "", err
	}
	if !present {
		return "", "", errors.New("must begin with YAML frontmatter")
	}
	return name, description, nil
}

func parseOptionalFrontmatter(file *os.File) (string, string, bool, error) {
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", "", false, nil
	}
	var name, description string
	closed := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			closed = true
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", true, fmt.Errorf("read frontmatter: %w", err)
	}
	if !closed {
		return "", "", true, errors.New("frontmatter is not closed")
	}
	if name == "" {
		return "", "", true, errors.New("name is required")
	}
	return name, description, true, nil
}

func Resolve(skills []Skill, reference string) (Skill, error) {
	if reference == "" || filepath.IsAbs(reference) || strings.Contains(reference, "\\") {
		return Skill{}, fmt.Errorf("invalid skill reference %q", reference)
	}
	cleaned := pathClean(reference)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return Skill{}, fmt.Errorf("invalid skill reference %q", reference)
	}
	if strings.Contains(cleaned, "/") {
		for _, skill := range skills {
			if skill.RelativePath == cleaned {
				return skill, nil
			}
		}
	}

	var matches []Skill
	for _, skill := range skills {
		if skill.Name == cleaned {
			matches = append(matches, skill)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		paths := make([]string, len(matches))
		for i, match := range matches {
			paths[i] = match.RelativePath
		}
		return Skill{}, fmt.Errorf("skill %q is ambiguous; use one of: %s", reference, strings.Join(paths, ", "))
	}

	for _, skill := range skills {
		if filepath.Base(skill.Path) == cleaned {
			matches = append(matches, skill)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		paths := make([]string, len(matches))
		for i, match := range matches {
			paths[i] = match.RelativePath
		}
		return Skill{}, fmt.Errorf("skill %q is ambiguous; use one of: %s", reference, strings.Join(paths, ", "))
	}
	return Skill{}, fmt.Errorf("skill %q was not found", reference)
}

func pathClean(value string) string {
	parts := strings.Split(value, "/")
	var clean []string
	for _, part := range parts {
		switch part {
		case "", ".":
		case "..":
			if len(clean) == 0 {
				return "../"
			}
			clean = clean[:len(clean)-1]
		default:
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return "."
	}
	return strings.Join(clean, "/")
}
