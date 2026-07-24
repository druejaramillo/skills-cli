package creator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunStartsInteractiveCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nmkdir -p \"$1/.agents/skills/tdd\"\nprintf '%s\\n' '---' 'name: tdd' 'description: Test skill.' '---' > \"$1/.agents/skills/tdd/SKILL.md\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Request{ProjectPath: project, SkillName: "tdd", Model: "test/model", Command: command}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "tdd", "SKILL.md")); err != nil {
		t.Fatalf("fake OpenCode did not receive project argument: %v", err)
	}
}

func TestRunRequiresModel(t *testing.T) {
	if err := Run(context.Background(), Request{ProjectPath: t.TempDir(), SkillName: "tdd"}); err == nil {
		t.Fatal("Run accepted missing model")
	}
}

func TestRunAgentsExpandsFragmentManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s' \"$5\" > \"" + promptPath + "\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAgents(context.Background(), AgentsRequest{
		ProjectPath:   project,
		SourceRoot:    "/tmp/source",
		FragmentPaths: []string{"agents-md/go.md", "agents-md/frontend/htmx.md"},
		Model:         "test/model",
		Command:       command,
	}); err != nil {
		t.Fatalf("RunAgents: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "/tmp/source") || !strings.Contains(string(prompt), "agents-md/frontend/htmx.md") {
		t.Fatalf("expanded prompt = %q", prompt)
	}
}

func TestRunAgentsFragmentExpandsOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s' \"$5\" > \"" + promptPath + "\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentsFragment(context.Background(), AgentsFragmentRequest{
		ProjectPath:  project,
		SourceRoot:   "/tmp/source",
		FragmentPath: "agents-md/frontend/htmx.md",
		StagingPath:  "/tmp/staging/agents-md/frontend/htmx.md",
		Model:        "test/model",
		Command:      command,
	}); err != nil {
		t.Fatalf("RunAgentsFragment: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "agents-md/frontend/htmx.md") || !strings.Contains(string(prompt), "/tmp/staging/agents-md/frontend/htmx.md") {
		t.Fatalf("expanded prompt = %q", prompt)
	}
}
