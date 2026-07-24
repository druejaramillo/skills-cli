package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/druejaramillo/skills-cli/internal/config"
)

func TestAddAndRemove(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	writeSkill(t, filepath.Join(sourceRoot, "engineering", "tdd"), "tdd")
	app := testApp(t, root, sourceRoot, "")
	if err := app.Run(context.Background(), []string{"add", "engineering/tdd"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	installed := filepath.Join(app.WorkingDir, ".agents", "skills", "tdd", "SKILL.md")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("installed skill missing: %v", err)
	}
	if err := app.Run(context.Background(), []string{"remove", "tdd"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Fatalf("removed skill still exists: %v", err)
	}
}

func TestListSkills(t *testing.T) {
	root := t.TempDir()
	app := &App{
		WorkingDir: filepath.Join(root, "project"),
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	}
	for _, name := range []string{"alpha", "beta", "no-skill-file"} {
		if err := os.MkdirAll(filepath.Join(app.WorkingDir, ".agents", "skills", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(app.WorkingDir, ".agents", "skills", "alpha", "SKILL.md"), []byte("---\nname: alpha\ndescription: Alpha skill.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app.WorkingDir, ".agents", "skills", "beta", "SKILL.md"), []byte("# Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := app.Run(context.Background(), []string{"list"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	want := "alpha\tAlpha skill.\nbeta\n"
	if got := app.Stdout.(*bytes.Buffer).String(); got != want {
		t.Fatalf("list output = %q, want %q", got, want)
	}
}

func TestListSkillsWhenNoneAreInstalled(t *testing.T) {
	app := &App{WorkingDir: t.TempDir(), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := app.Run(context.Background(), []string{"list"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := app.Stdout.(*bytes.Buffer).String(); got != "No skills are installed.\n" {
		t.Fatalf("list output = %q", got)
	}
}

func TestCreatePublishesAfterOpenCodeSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nmkdir -p \"$1/.agents/skills/tdd\"\nprintf '%s\\n' '---' 'name: tdd' 'description: Test skill.' '---' > \"$1/.agents/skills/tdd/SKILL.md\"\n"
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app := testApp(t, root, sourceRoot, "test/model")
	if err := app.Run(context.Background(), []string{"create", "tdd"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "tdd", "SKILL.md")); err != nil {
		t.Fatalf("published source skill missing: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "published") {
		t.Fatalf("create output = %q, want publish confirmation", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestCreateWithoutSkillDoesNotPublish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app := testApp(t, root, sourceRoot, "test/model")
	if err := app.Run(context.Background(), []string{"create", "tdd"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "tdd")); !os.IsNotExist(err) {
		t.Fatalf("source was published without a generated skill: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "nothing was published") {
		t.Fatalf("create output = %q, want no-publish confirmation", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestAgentsGenerateWritesProjectAGENTS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	writeAgentsFragment(t, filepath.Join(sourceRoot, "agents-md", "go.md"), "# Go\n")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' '# Project guidance' > \"$1/AGENTS.md\"\n"
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app := testApp(t, root, sourceRoot, "test/model")
	if err := app.Run(context.Background(), []string{"agents", "generate"}); err != nil {
		t.Fatalf("agents generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.WorkingDir, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md missing: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "Created") {
		t.Fatalf("agents generate output = %q", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestAgentsGenerateRequiresForce(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	writeAgentsFragment(t, filepath.Join(sourceRoot, "agents-md", "go.md"), "# Go\n")
	app := testApp(t, root, sourceRoot, "test/model")
	if err := os.WriteFile(filepath.Join(app.WorkingDir, "AGENTS.md"), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background(), []string{"agents", "generate"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("agents generate collision error = %v", err)
	}
}

func TestAgentsCreatePublishesFragment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nstaging=$(printf '%s' \"$5\" | tr '\\n' ' ' | cut -d '`' -f4)\nmkdir -p \"$(dirname \"$staging\")\"\nprintf '%s\\n' '# HTMX' 'Use server-rendered HTML.' > \"$staging\"\n"
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app := testApp(t, root, sourceRoot, "test/model")
	if err := app.Run(context.Background(), []string{"agents", "create", "frontend/htmx"}); err != nil {
		t.Fatalf("agents create: %v", err)
	}
	published := filepath.Join(sourceRoot, "agents-md", "frontend", "htmx.md")
	if _, err := os.Stat(published); err != nil {
		t.Fatalf("published fragment missing: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "published") {
		t.Fatalf("agents create output = %q", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestAgentsCreateWithoutFragmentDoesNotPublish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app := testApp(t, root, sourceRoot, "test/model")
	if err := app.Run(context.Background(), []string{"agents", "create", "go"}); err != nil {
		t.Fatalf("agents create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "agents-md", "go.md")); !os.IsNotExist(err) {
		t.Fatalf("fragment was published without generated output: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "nothing was published") {
		t.Fatalf("agents create output = %q", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestAgentsCreateRejectsExistingFragment(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	writeAgentsFragment(t, filepath.Join(sourceRoot, "agents-md", "go.md"), "# Go\n")
	app := testApp(t, root, sourceRoot, "test/model")
	if err := app.Run(context.Background(), []string{"agents", "create", "go"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("agents create collision error = %v", err)
	}
}

func TestAgentsCreateRejectsRemoteSource(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	if err := config.Save(configPath, config.Config{
		DefaultSource: "mine",
		Sources:       map[string]config.Source{"mine": {Location: "https://example.com/skills.git", Remote: true}},
		Creator:       config.Creator{Model: "test/model"},
	}); err != nil {
		t.Fatal(err)
	}
	app := &App{ConfigPath: configPath, CacheDir: filepath.Join(root, "cache"), WorkingDir: t.TempDir(), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := app.Run(context.Background(), []string{"agents", "create", "go"}); err == nil || !strings.Contains(err.Error(), "remote") {
		t.Fatalf("agents create remote source error = %v", err)
	}
}

func TestRemoveRemoteSourceClearsManagedCache(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	cacheDir := filepath.Join(root, "cache")
	if err := config.Save(configPath, config.Config{
		DefaultSource: "mine",
		Sources:       map[string]config.Source{"mine": {Location: "https://example.com/skills.git", Remote: true}},
	}); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(cacheDir, "mine")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	app := &App{ConfigPath: configPath, CacheDir: cacheDir, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := app.Run(context.Background(), []string{"source", "remove", "mine"}); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("managed cache remains after source removal: %v", err)
	}
}

func testApp(t *testing.T, root, sourceRoot, model string) *App {
	t.Helper()
	configPath := filepath.Join(root, "config.json")
	cfg := config.Config{
		DefaultSource: "mine",
		Sources:       map[string]config.Source{"mine": {Location: sourceRoot}},
		Creator:       config.Creator{Model: model},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	return &App{
		ConfigPath: configPath,
		CacheDir:   filepath.Join(root, "cache"),
		WorkingDir: project,
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
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
