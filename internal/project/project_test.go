package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopySkillAndReplace(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source", "tdd")
	writeSkill(t, source, "tdd")
	if err := os.MkdirAll(filepath.Join(source, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "references", "guide.md"), []byte("guide"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".git"), []byte("gitdir: ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination, err := SkillPath(filepath.Join(root, "project"), "tdd")
	if err != nil {
		t.Fatal(err)
	}
	if err := CopySkill(source, destination, false); err != nil {
		t.Fatalf("CopySkill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "references", "guide.md")); err != nil {
		t.Fatalf("copied reference missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".git")); !os.IsNotExist(err) {
		t.Fatalf("copied Git metadata: %v", err)
	}
	if err := CopySkill(source, destination, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CopySkill collision error = %v, want collision", err)
	}
	if err := os.WriteFile(filepath.Join(source, "new.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopySkill(source, destination, true); err != nil {
		t.Fatalf("CopySkill force: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "new.md")); err != nil {
		t.Fatalf("forced copy did not replace destination: %v", err)
	}
}

func TestRemoveSkill(t *testing.T) {
	root := t.TempDir()
	skill, err := SkillPath(root, "tdd")
	if err != nil {
		t.Fatal(err)
	}
	writeSkill(t, skill, "tdd")
	neighbor := filepath.Join(root, ".agents", "skills", "other")
	writeSkill(t, neighbor, "other")
	if err := RemoveSkill(root, "tdd"); err != nil {
		t.Fatalf("RemoveSkill: %v", err)
	}
	if _, err := os.Stat(neighbor); err != nil {
		t.Fatalf("remove affected neighboring skill: %v", err)
	}
	if err := RemoveSkill(root, "../other"); err == nil {
		t.Fatal("RemoveSkill accepted traversal name")
	}
}

func TestAgentsPathAndValidation(t *testing.T) {
	root := t.TempDir()
	path := AgentsPath(root)
	if path != filepath.Join(root, "AGENTS.md") {
		t.Fatalf("AgentsPath = %q", path)
	}
	if err := ValidateAgentsFile(path); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing AGENTS.md error = %v", err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFile(path); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty AGENTS.md error = %v", err)
	}
	if err := os.WriteFile(path, []byte("# Project guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFile(path); err != nil {
		t.Fatalf("ValidateAgentsFile: %v", err)
	}
	if err := ValidateGeneratedAgentsFile(path, "."); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("missing provenance error = %v", err)
	}
	provenance := "<!-- skills-agents-output: {\"version\":1,\"runtime\":\"opencode-local\",\"target\":\".\"} -->\n# Project guidance\n"
	if err := os.WriteFile(path, []byte(provenance), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGeneratedAgentsFile(path, "."); err != nil {
		t.Fatalf("ValidateGeneratedAgentsFile: %v", err)
	}
	if err := ValidateGeneratedAgentsFile(path, "internal/api"); err == nil || !strings.Contains(err.Error(), "invalid provenance") {
		t.Fatalf("wrong provenance target error = %v", err)
	}
	if err := os.WriteFile(path, []byte(" \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFile(path); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("whitespace AGENTS.md error = %v", err)
	}
}

func TestAgentsPathAt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := AgentsPathAt(root, "internal/api")
	if err != nil {
		t.Fatalf("AgentsPathAt: %v", err)
	}
	if path != filepath.Join(root, "internal", "api", "AGENTS.md") {
		t.Fatalf("AgentsPathAt = %q", path)
	}
	for _, target := range []string{"", "../outside", "/tmp", "internal//api", "missing"} {
		if _, err := AgentsPathAt(root, target); err == nil {
			t.Fatalf("AgentsPathAt accepted %q", target)
		}
	}
}

func TestPublishFile(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "generated.md")
	if err := os.WriteFile(from, []byte("# Guidance\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	to := filepath.Join(root, "source", "agents-md", "go.md")
	if err := PublishFile(from, to); err != nil {
		t.Fatalf("PublishFile: %v", err)
	}
	contents, err := os.ReadFile(to)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "# Guidance\n" {
		t.Fatalf("published contents = %q", contents)
	}
	if err := PublishFile(from, to); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("PublishFile collision error = %v", err)
	}
}

func TestReplaceFile(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "staged.md")
	to := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(from, []byte("# New\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, []byte("# Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceFile(from, to); err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}
	contents, err := os.ReadFile(to)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "# New\n" {
		t.Fatalf("replacement contents = %q", contents)
	}
	if info, err := os.Stat(to); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("replacement permissions = %v, %v", info, err)
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
