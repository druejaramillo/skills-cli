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

func TestRunUsesPlanAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsPath + "\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := Run(context.Background(), Request{ProjectPath: project, SkillName: "tdd", Model: "test/model", Command: command}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(got) < 5 || got[0] != project || got[1] != "--agent" || got[2] != "plan" || got[3] != "--model" || got[4] != "test/model" {
		t.Fatalf("args = %q, want project --agent plan --model test/model ...", got)
	}
}

func TestRunAgentsExpandsFragmentManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s' \"$7\" > \"" + promptPath + "\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAgents(context.Background(), AgentsRequest{
		ProjectPath:       project,
		TargetRelativeDir: "internal/api",
		TargetPath:        "/tmp/project/internal/api/AGENTS.md",
		StagingPath:       "/tmp/staging/AGENTS.md",
		SourceRoot:        "/tmp/source",
		FragmentPaths:     []string{"agents-md/go.md", "agents-md/frontend/htmx.md"},
		FragmentManifest:  "agents-md/go.md {\"layer\":\"stack\"}",
		Model:             "test/model",
		Command:           command,
	}); err != nil {
		t.Fatalf("RunAgents: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "/tmp/source") || !strings.Contains(string(prompt), "agents-md/go.md {\"layer\":\"stack\"}") || !strings.Contains(string(prompt), "/tmp/staging/AGENTS.md") || !strings.Contains(string(prompt), "internal/api") {
		t.Fatalf("expanded prompt = %q", prompt)
	}
}

func TestRunAgentsUpdateExpandsStagingAndReconciliationPrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s' \"$7\" > \"" + promptPath + "\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentsUpdate(context.Background(), AgentsRequest{
		ProjectPath:       project,
		TargetRelativeDir: ".",
		TargetPath:        filepath.Join(project, "AGENTS.md"),
		StagingPath:       "/tmp/staging/AGENTS.md",
		SourceRoot:        "/tmp/source",
		FragmentManifest:  "agents-md/core.md {\"layer\":\"core\"}",
		Model:             "test/model",
		Command:           command,
	}); err != nil {
		t.Fatalf("RunAgentsUpdate: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "Preserve verified") || !strings.Contains(string(prompt), "/tmp/staging/AGENTS.md") || !strings.Contains(string(prompt), "agents-md/core.md") {
		t.Fatalf("update prompt = %q", prompt)
	}
}

func TestRunAgentsFragmentExpandsOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s' \"$7\" > \"" + promptPath + "\"\n"
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

func TestRunAgentsFragmentRevisionExpandsExistingPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	project := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	command := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s' \"$7\" > \"" + promptPath + "\"\n"
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentsFragmentRevision(context.Background(), AgentsFragmentRequest{
		ProjectPath:          project,
		SourceRoot:           "/tmp/source",
		FragmentPath:         "agents-md/go.md",
		ExistingFragmentPath: "/tmp/source/agents-md/go.md",
		StagingPath:          "/tmp/staging/go.md",
		Model:                "test/model",
		Command:              command,
	}); err != nil {
		t.Fatalf("RunAgentsFragmentRevision: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "/tmp/source/agents-md/go.md") || !strings.Contains(string(prompt), "/tmp/staging/go.md") {
		t.Fatalf("revision prompt = %q", prompt)
	}
}
