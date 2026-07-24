package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/druejaramillo/skills-cli/internal/config"
)

func TestDiscoverAndResolve(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "engineering", "tdd"), "tdd")
	writeSkill(t, filepath.Join(root, "frontend", "tdd"), "tdd")

	skills, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("Discover returned %d skills, want 2", len(skills))
	}
	if _, err := Resolve(skills, "tdd"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("Resolve ambiguous skill error = %v, want ambiguity error", err)
	}
	got, err := Resolve(skills, "engineering/tdd")
	if err != nil {
		t.Fatalf("Resolve relative path: %v", err)
	}
	if got.RelativePath != "engineering/tdd" {
		t.Fatalf("resolved path = %q, want engineering/tdd", got.RelativePath)
	}
}

func TestDiscoverRejectsMalformedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "wrong-directory"), "other-name")
	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Discover malformed skill error = %v, want mismatch error", err)
	}
}

func TestDiscoverAgentsFragments(t *testing.T) {
	root := t.TempDir()
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "frontend", "tailwind.md"), "# Tailwind\n")
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "go.md"), "# Go\n")
	if err := os.WriteFile(filepath.Join(root, "agents-md", "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	fragments, err := DiscoverAgentsFragments(root)
	if err != nil {
		t.Fatalf("DiscoverAgentsFragments: %v", err)
	}
	if len(fragments) != 2 {
		t.Fatalf("DiscoverAgentsFragments returned %d fragments, want 2", len(fragments))
	}
	if fragments[0].RelativePath != "agents-md/frontend/tailwind.md" || fragments[1].RelativePath != "agents-md/go.md" {
		t.Fatalf("fragment paths = %#v", fragments)
	}
}

func TestDiscoverAgentsFragmentsRequiresMarkdown(t *testing.T) {
	root := t.TempDir()
	if _, err := DiscoverAgentsFragments(root); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing fragment directory error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "agents-md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverAgentsFragments(root); err == nil || !strings.Contains(err.Error(), "contains no Markdown") {
		t.Fatalf("empty fragment directory error = %v", err)
	}
}

func TestAddLocation(t *testing.T) {
	src, err := AddLocation("acme/skills")
	if err != nil {
		t.Fatalf("AddLocation shorthand: %v", err)
	}
	if !src.Remote || src.Location != "https://github.com/acme/skills.git" {
		t.Fatalf("AddLocation shorthand = %#v", src)
	}

	dir := t.TempDir()
	src, err = AddLocation(dir)
	if err != nil {
		t.Fatalf("AddLocation local: %v", err)
	}
	if src.Remote || src.Location != dir {
		t.Fatalf("AddLocation local = %#v", src)
	}
}

func TestPrepareClonesRemoteSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	working := filepath.Join(root, "working")
	runTestGit(t, root, "init", working)
	writeSkill(t, filepath.Join(working, "tdd"), "tdd")
	runTestGit(t, working, "add", ".")
	runTestGit(t, working, "-c", "user.name=Skills Test", "-c", "user.email=skills@example.com", "commit", "-m", "initial")
	bare := filepath.Join(root, "source.git")
	runTestGit(t, root, "clone", "--bare", working, bare)

	prepared, err := Prepare(context.Background(), config.Source{Location: "file://" + bare, Remote: true}, "mine", filepath.Join(root, "cache"))
	if err != nil {
		t.Fatalf("Prepare remote source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prepared, "tdd", "SKILL.md")); err != nil {
		t.Fatalf("cloned skill missing: %v", err)
	}
}

func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: " + name + "\ndescription: Test skill.\n---\n\n# Test\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeAgentsFragment(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
