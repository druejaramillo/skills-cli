package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/druejaramillo/skills-cli/internal/source"
)

const agentsOutputPrefix = "<!-- skills-agents-output:"

func SkillPath(root, name string) (string, error) {
	if err := source.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(root, ".agents", "skills", name), nil
}

func AgentsPath(root string) string {
	return filepath.Join(root, "AGENTS.md")
}

// AgentsPathAt resolves an AGENTS.md target directory inside root. The target
// must already be a real directory; symlink targets and unexpected subtrees
// are rejected during resolution.
func AgentsPathAt(root, targetRelativeDir string) (string, error) {
	if targetRelativeDir == "" {
		return "", errors.New("AGENTS.md target path is required")
	}
	if targetRelativeDir != "." {
		if filepath.IsAbs(targetRelativeDir) || strings.Contains(targetRelativeDir, "\\") || path.Clean(targetRelativeDir) != targetRelativeDir || targetRelativeDir == ".." || strings.HasPrefix(targetRelativeDir, "../") {
			return "", fmt.Errorf("invalid AGENTS.md target path %q", targetRelativeDir)
		}
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("inspect project root: %w", err)
	}
	if !rootInfo.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", root)
	}

	target := filepath.Join(resolvedRoot, filepath.FromSlash(targetRelativeDir))
	info, err := os.Lstat(target)
	if err != nil {
		return "", fmt.Errorf("inspect AGENTS.md target directory %q: %w", targetRelativeDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("AGENTS.md target path %q is not a directory", targetRelativeDir)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve AGENTS.md target directory %q: %w", targetRelativeDir, err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil {
		return "", fmt.Errorf("verify AGENTS.md target directory %q: %w", targetRelativeDir, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("AGENTS.md target path %q resolves outside the project", targetRelativeDir)
	}
	return filepath.Join(resolvedTarget, "AGENTS.md"), nil
}

type SkillSummary struct {
	Name        string
	Description string
}

func ListSkills(root string) ([]SkillSummary, error) {
	skillsRoot := filepath.Join(root, ".agents", "skills")
	entries, err := os.ReadDir(skillsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project skills directory: %w", err)
	}

	summaries := make([]SkillSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(skillsRoot, entry.Name(), "SKILL.md")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect skill %q: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("skill file %q is not a regular file", path)
		}

		summary := SkillSummary{Name: entry.Name()}
		name, description, present, err := source.ReadSkillFrontmatter(path)
		if err != nil {
			return nil, fmt.Errorf("read skill %q: %w", entry.Name(), err)
		}
		if present {
			summary.Name = name
			summary.Description = description
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}

func ValidateAgentsFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("generated AGENTS.md %q does not exist", path)
	}
	if err != nil {
		return fmt.Errorf("inspect generated AGENTS.md: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("generated AGENTS.md %q is not a regular file", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated AGENTS.md: %w", err)
	}
	if strings.TrimSpace(string(contents)) == "" {
		return fmt.Errorf("generated AGENTS.md %q is empty", path)
	}
	return nil
}

// ValidateGeneratedAgentsFile verifies the minimal provenance emitted by the
// generation prompts in addition to the regular, nonblank file checks.
func ValidateGeneratedAgentsFile(path, targetRelativeDir string) error {
	if err := ValidateAgentsFile(path); err != nil {
		return err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated AGENTS.md: %w", err)
	}
	if !bytes.HasPrefix(contents, []byte(agentsOutputPrefix)) {
		return fmt.Errorf("generated AGENTS.md %q is missing skills-agents output provenance", path)
	}
	comment := contents[len(agentsOutputPrefix):]
	end := bytes.Index(comment, []byte("-->"))
	if end < 0 {
		return fmt.Errorf("generated AGENTS.md %q has an unclosed skills-agents output provenance", path)
	}
	var provenance struct {
		Version int    `json:"version"`
		Runtime string `json:"runtime"`
		Target  string `json:"target"`
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(comment[:end])))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provenance); err != nil {
		return fmt.Errorf("decode generated AGENTS.md provenance: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("generated AGENTS.md %q has multiple provenance values", path)
		}
		return fmt.Errorf("decode generated AGENTS.md provenance: %w", err)
	}
	if provenance.Version != 1 || provenance.Runtime != "opencode-local" || provenance.Target != targetRelativeDir {
		return fmt.Errorf("generated AGENTS.md %q has invalid provenance", path)
	}
	return nil
}

func PublishFile(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source file %q is not a regular file", from)
	}
	parent := filepath.Dir(to)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create file destination: %w", err)
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("destination %q already exists", to)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}

	temporary, err := os.CreateTemp(parent, "."+filepath.Base(to)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	input, err := os.Open(from)
	if err != nil {
		temporary.Close()
		return fmt.Errorf("open source file: %w", err)
	}
	_, copyErr := io.Copy(temporary, input)
	inputCloseErr := input.Close()
	closeErr := temporary.Close()
	if copyErr != nil {
		return fmt.Errorf("copy file: %w", copyErr)
	}
	if inputCloseErr != nil {
		return fmt.Errorf("close source file: %w", inputCloseErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close temporary file: %w", closeErr)
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("destination %q already exists", to)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}
	if err := os.Rename(temporaryName, to); err != nil {
		return fmt.Errorf("publish file: %w", err)
	}
	return nil
}

// ReplaceFile atomically replaces a regular destination with a staged regular
// file when the operating system supports an overwrite rename. The temporary
// file is always created in the destination directory to keep the rename on
// one filesystem.
func ReplaceFile(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source file %q is not a regular file", from)
	}
	parent := filepath.Dir(to)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect destination directory: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return fmt.Errorf("destination directory %q is not a directory", parent)
	}

	if destinationInfo, err := os.Lstat(to); err == nil {
		if !destinationInfo.Mode().IsRegular() {
			return fmt.Errorf("destination %q is not a regular file", to)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}

	temporary, err := os.CreateTemp(parent, "."+filepath.Base(to)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	input, err := os.Open(from)
	if err != nil {
		temporary.Close()
		return fmt.Errorf("open source file: %w", err)
	}
	_, copyErr := io.Copy(temporary, input)
	inputCloseErr := input.Close()
	closeErr := temporary.Close()
	if copyErr != nil {
		return fmt.Errorf("copy file: %w", copyErr)
	}
	if inputCloseErr != nil {
		return fmt.Errorf("close source file: %w", inputCloseErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close temporary file: %w", closeErr)
	}

	destinationInfo, err := os.Lstat(to)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(temporaryName, to); err != nil {
			return fmt.Errorf("publish replacement: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect destination before replacement: %w", err)
	}
	if !destinationInfo.Mode().IsRegular() {
		return fmt.Errorf("destination %q is not a regular file", to)
	}

	if err := os.Rename(temporaryName, to); err != nil {
		return fmt.Errorf("publish replacement: %w", err)
	}
	return nil
}

func CopySkill(from, to string, force bool) error {
	if _, err := source.ValidateSkillDirectory(from); err != nil {
		return err
	}
	parent := filepath.Dir(to)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create skill destination: %w", err)
	}
	if _, err := os.Lstat(to); err == nil && !force {
		return fmt.Errorf("destination %q already exists; use --force to replace it", to)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}

	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(to)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary skill directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := copyDirectory(from, temporary); err != nil {
		return err
	}

	if _, err := os.Lstat(to); err == nil {
		if !force {
			return fmt.Errorf("destination %q already exists; use --force to replace it", to)
		}
		backup, err := os.MkdirTemp(parent, "."+filepath.Base(to)+"-backup-*")
		if err != nil {
			return fmt.Errorf("create destination backup: %w", err)
		}
		if err := os.Remove(backup); err != nil {
			return fmt.Errorf("prepare destination backup: %w", err)
		}
		if err := os.Rename(to, backup); err != nil {
			return fmt.Errorf("back up existing skill: %w", err)
		}
		if err := os.Rename(temporary, to); err != nil {
			if restoreErr := os.Rename(backup, to); restoreErr != nil {
				return fmt.Errorf("install skill: %w (also failed to restore previous skill: %v)", err, restoreErr)
			}
			return fmt.Errorf("install skill: %w", err)
		}
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous skill backup: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}

	if err := os.Rename(temporary, to); err != nil {
		return fmt.Errorf("install skill: %w", err)
	}
	return nil
}

func RemoveSkill(root, name string) error {
	path, err := SkillPath(root, name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("skill %q is not installed", name)
	}
	if err != nil {
		return fmt.Errorf("inspect installed skill: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("installed skill path %q is not a directory", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove skill %q: %w", name, err)
	}
	return nil
}

func copyDirectory(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		destination := filepath.Join(to, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, destination)
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type %q", path)
		}
		return copyFile(path, destination, info.Mode().Perm())
	})
}

func copyFile(from, to string, mode fs.FileMode) error {
	input, err := os.Open(from)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
