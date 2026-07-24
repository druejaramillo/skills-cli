package creator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
