package source

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/druejaramillo/skills-cli/internal/config"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Skill struct {
	Name         string
	Path         string
	RelativePath string
}

type AgentsFragment struct {
	Path         string
	RelativePath string
}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) || len(name) > 64 {
		return fmt.Errorf("%q must use lowercase letters, numbers, and single hyphens (1-64 characters)", name)
	}
	return nil
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

func DiscoverAgentsFragments(root string) ([]AgentsFragment, error) {
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
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("read agents fragment %q: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close agents fragment %q: %w", path, err)
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

func ValidateSkillDirectory(path string) (Skill, error) {
	name := filepath.Base(path)
	if err := ValidateName(name); err != nil {
		return Skill{}, fmt.Errorf("invalid skill directory %q: %w", path, err)
	}
	file, err := os.Open(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("read skill %q: %w", path, err)
	}
	defer file.Close()

	frontmatterName, description, err := parseFrontmatter(file)
	if err != nil {
		return Skill{}, fmt.Errorf("invalid SKILL.md in %q: %w", path, err)
	}
	if frontmatterName != name {
		return Skill{}, fmt.Errorf("skill directory %q does not match frontmatter name %q", name, frontmatterName)
	}
	if err := ValidateName(frontmatterName); err != nil {
		return Skill{}, fmt.Errorf("invalid frontmatter name: %w", err)
	}
	if description == "" {
		return Skill{}, fmt.Errorf("invalid SKILL.md in %q: description is required", path)
	}
	return Skill{Name: name, Path: path}, nil
}

func parseFrontmatter(file *os.File) (string, string, error) {
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", "", errors.New("must begin with YAML frontmatter")
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
		return "", "", fmt.Errorf("read frontmatter: %w", err)
	}
	if !closed {
		return "", "", errors.New("frontmatter is not closed")
	}
	if name == "" {
		return "", "", errors.New("name is required")
	}
	return name, description, nil
}

func Resolve(skills []Skill, reference string) (Skill, error) {
	if reference == "" || filepath.IsAbs(reference) || strings.Contains(reference, "\\") {
		return Skill{}, fmt.Errorf("invalid skill reference %q", reference)
	}
	cleaned := pathClean(reference)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return Skill{}, fmt.Errorf("invalid skill reference %q", reference)
	}
	for _, skill := range skills {
		if skill.RelativePath == cleaned {
			return skill, nil
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
