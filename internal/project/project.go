package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/druejaramillo/skills-cli/internal/source"
)

func SkillPath(root, name string) (string, error) {
	if err := source.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(root, ".agents", "skills", name), nil
}

func AgentsPath(root string) string {
	return filepath.Join(root, "AGENTS.md")
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
	if info.Size() == 0 {
		return fmt.Errorf("generated AGENTS.md %q is empty", path)
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
