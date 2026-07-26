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
	if !fragments[0].Unclassified || !fragments[1].Unclassified {
		t.Fatalf("legacy fragments must be retained as unclassified: %#v", fragments)
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

func TestAgentsFragmentPath(t *testing.T) {
	root := t.TempDir()
	path, relative, err := AgentsFragmentPath(root, "frontend/htmx")
	if err != nil {
		t.Fatalf("AgentsFragmentPath: %v", err)
	}
	if path != filepath.Join(root, "agents-md", "frontend", "htmx.md") || relative != "agents-md/frontend/htmx.md" {
		t.Fatalf("AgentsFragmentPath = %q, %q", path, relative)
	}
	for _, reference := range []string{"", "go.md", "../go", "/go", "frontend\\htmx", "frontend//htmx"} {
		if _, _, err := AgentsFragmentPath(root, reference); err == nil {
			t.Fatalf("AgentsFragmentPath accepted %q", reference)
		}
	}
}

func TestValidateAgentsFragment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.md")
	if err := ValidateAgentsFragment(path); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing fragment error = %v", err)
	}
	if err := os.WriteFile(path, []byte(" \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFragment(path); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty fragment error = %v", err)
	}
	if err := os.WriteFile(path, []byte("# Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFragment(path); err == nil || !strings.Contains(err.Error(), "must begin") {
		t.Fatalf("legacy fragment validation error = %v, want manifest error", err)
	}
	if err := os.WriteFile(path, []byte("---\nname: go\n---\n# Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFragment(path); err == nil || !strings.Contains(err.Error(), "YAML") {
		t.Fatalf("YAML fragment validation error = %v, want YAML error", err)
	}
	if err := os.WriteFile(path, []byte(agentsManifestMarkdown(`{"version":1,"id":"go","layer":"stack","scope":"project","relationship":"all","owner":"maintainers","canonical":["docs/testing.md"],"review_on":["Go toolchain changes"]}`, "# Go\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgentsFragment(path); err != nil {
		t.Fatalf("ValidateAgentsFragment: %v", err)
	}
}

func TestParseAgentsManifest(t *testing.T) {
	manifest, present, err := ParseAgentsManifest([]byte(agentsManifestMarkdown(`{"version":1,"id":"go","layer":"stack","scope":"project","relationship":"all","owner":"maintainers","canonical":["docs/testing.md"],"review_on":["Go toolchain changes"],"paths":["go.mod"]}`, "# Go\n")))
	if err != nil || !present {
		t.Fatalf("ParseAgentsManifest = %#v, %t, %v", manifest, present, err)
	}
	if manifest.ID != "go" || manifest.Paths[0] != "go.mod" {
		t.Fatalf("manifest = %#v", manifest)
	}

	manifest, present, err = ParseAgentsManifest([]byte("# Legacy\n"))
	if err != nil || present || manifest != nil {
		t.Fatalf("legacy ParseAgentsManifest = %#v, %t, %v", manifest, present, err)
	}
	for _, markdown := range []string{
		"\n" + agentsManifestMarkdown(`{"version":1}`, "# Go\n"),
		agentsManifestMarkdown(`null`, "# Go\n"),
		agentsManifestMarkdown(`{"version":1,"unknown":true}`, "# Go\n"),
		"---\nname: go\n---\n# Go\n",
		agentsManifestMarkdown(`{"version":1}`, "---\nname: go\n---\n# Go\n"),
	} {
		if _, _, err := ParseAgentsManifest([]byte(markdown)); err == nil {
			t.Fatalf("ParseAgentsManifest accepted %q", markdown)
		}
	}
}

func TestValidateAgentsManifest(t *testing.T) {
	valid := AgentsManifest{
		Version:      AgentsManifestVersion,
		ID:           "go",
		Layer:        AgentsLayerStack,
		Scope:        AgentsScopeProject,
		Relationship: AgentsRelationshipAll,
		Owner:        "maintainers",
		Canonical:    []string{"docs/testing.md"},
		ReviewOn:     []string{"Go toolchain changes"},
		Paths:        []string{"go.mod"},
		Globs:        []string{"**/*.go"},
		ContentMarkers: []AgentsContentMarker{{
			Path:     "go.mod",
			Contains: "module ",
		}},
	}
	if err := ValidateAgentsManifest(valid); err != nil {
		t.Fatalf("ValidateAgentsManifest valid manifest: %v", err)
	}

	for _, test := range []struct {
		name     string
		manifest AgentsManifest
		want     string
	}{
		{"version", AgentsManifest{ID: "go", Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll}, "version"},
		{"scope", AgentsManifest{Version: 1, ID: "go", Scope: "workspace", Relationship: AgentsRelationshipAll}, "scope"},
		{"relationship", AgentsManifest{Version: 1, ID: "go", Scope: AgentsScopeProject, Relationship: "none"}, "relationship"},
		{"relative path", AgentsManifest{Version: 1, ID: "go", Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll, Paths: []string{"../go.mod"}}, "canonical relative path"},
		{"glob", AgentsManifest{Version: 1, ID: "go", Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll, Globs: []string{"src/**go"}}, "recursive wildcard"},
		{"marker", AgentsManifest{Version: 1, ID: "go", Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll, ContentMarkers: []AgentsContentMarker{{Path: "./go.mod"}}}, "content marker"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAgentsManifest(test.manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAgentsManifest error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInspectAgentsFragmentsReportsValidation(t *testing.T) {
	root := t.TempDir()
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "legacy.md"), "# Legacy\n")
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "empty.md"), " \n\t")
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "one.md"), agentsManifestMarkdown(`{"version":1,"id":"duplicate","layer":"stack","scope":"project","relationship":"all","owner":"maintainers","canonical":["docs/testing.md"],"review_on":["Go toolchain changes"]}`, "# One\n"))
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "two.md"), agentsManifestMarkdown(`{"version":1,"id":"duplicate","layer":"stack","scope":"workspace","relationship":"all","owner":"maintainers","canonical":["docs/testing.md"],"review_on":["Go toolchain changes"]}`, "# Two\n"))
	writeAgentsFragment(t, filepath.Join(root, "agents-md", "bad-json.md"), agentsManifestMarkdown(`{"version":1,`, "# Bad\n"))

	fragments, diagnostics, err := InspectAgentsFragments(root)
	if err != nil {
		t.Fatalf("InspectAgentsFragments: %v", err)
	}
	if len(fragments) != 5 {
		t.Fatalf("InspectAgentsFragments returned %d fragments, want 5", len(fragments))
	}
	if len(diagnostics.Warnings) != 1 || diagnostics.Warnings[0].Path != "agents-md/legacy.md" {
		t.Fatalf("warnings = %#v", diagnostics.Warnings)
	}
	if !diagnostics.HasErrors() {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"fragment is empty", "duplicate manifest id", "scope must", "invalid manifest"} {
		if err := diagnostics.Err(); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("diagnostics error = %v, want %q", err, want)
		}
	}
	if _, err := DiscoverAgentsFragments(root); err == nil || !strings.Contains(err.Error(), "duplicate manifest id") {
		t.Fatalf("DiscoverAgentsFragments error = %v, want validation error", err)
	}
}

func TestSelectAgentsFragments(t *testing.T) {
	projectRoot := t.TempDir()
	writeProjectFile(t, filepath.Join(projectRoot, "go.mod"), "module example.com/api\n")
	writeProjectFile(t, filepath.Join(projectRoot, "cmd", "main.go"), "package main\n")
	writeProjectFile(t, filepath.Join(projectRoot, "packages", "api", "package.json"), `{"dependencies":{"react":"1"}}`)
	writeProjectFile(t, filepath.Join(projectRoot, "packages", "api", "src", "app.tsx"), "export const App = () => null\n")

	fragments := []AgentsFragment{
		{RelativePath: "agents-md/legacy.md", Unclassified: true},
		{RelativePath: "agents-md/base.md", Manifest: &AgentsManifest{Version: 1, ID: "base", Layer: AgentsLayerCore, Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll, Owner: "maintainers", Canonical: []string{"docs/base.md"}, ReviewOn: []string{"policy changes"}}},
		{RelativePath: "agents-md/go.md", Manifest: &AgentsManifest{
			Version:      1,
			ID:           "go",
			Layer:        AgentsLayerStack,
			Scope:        AgentsScopeProject,
			Relationship: AgentsRelationshipAll,
			Owner:        "maintainers",
			Canonical:    []string{"docs/go.md"},
			ReviewOn:     []string{"Go toolchain changes"},
			Paths:        []string{"go.mod"},
			Globs:        []string{"**/*.go"},
			ContentMarkers: []AgentsContentMarker{{
				Path:     "go.mod",
				Contains: "module example.com",
			}},
		}},
		{RelativePath: "agents-md/react.md", Manifest: &AgentsManifest{
			Version:      1,
			ID:           "react",
			Layer:        AgentsLayerStack,
			Scope:        AgentsScopeDirectory,
			Relationship: AgentsRelationshipAll,
			Owner:        "maintainers",
			Canonical:    []string{"docs/frontend.md"},
			ReviewOn:     []string{"frontend build changes"},
			Paths:        []string{"package.json"},
			Globs:        []string{"src/**/*.tsx"},
			ContentMarkers: []AgentsContentMarker{{
				Path:     "package.json",
				Contains: "react",
			}},
		}},
		{RelativePath: "agents-md/missing.md", Manifest: &AgentsManifest{
			Version:      1,
			ID:           "missing",
			Layer:        AgentsLayerStack,
			Scope:        AgentsScopeProject,
			Relationship: AgentsRelationshipAny,
			Owner:        "maintainers",
			Canonical:    []string{"docs/missing.md"},
			ReviewOn:     []string{"tooling changes"},
			Paths:        []string{"missing.txt"},
			Globs:        []string{"**/*.rs"},
		}},
	}

	selected, err := SelectAgentsFragments(projectRoot, "packages/api", fragments)
	if err != nil {
		t.Fatalf("SelectAgentsFragments: %v", err)
	}
	if got, want := selectedFragmentIDs(selected), []string{"legacy", "base", "go", "react"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("selected fragments = %v, want %v", got, want)
	}
	selected, err = SelectAgentsFragments(projectRoot, ".", fragments)
	if err != nil {
		t.Fatalf("SelectAgentsFragments root: %v", err)
	}
	if got, want := selectedFragmentIDs(selected), []string{"legacy", "base", "go"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("root selected fragments = %v, want %v", got, want)
	}
	for _, target := range []string{"", "../outside", "packages//api"} {
		if _, err := SelectAgentsFragments(projectRoot, target, fragments); err == nil {
			t.Fatalf("SelectAgentsFragments accepted target %q", target)
		}
	}
}

func TestSelectAgentsFragmentsDoesNotFollowEvidenceSymlinksOutsideProject(t *testing.T) {
	projectRoot := t.TempDir()
	outside := t.TempDir()
	writeProjectFile(t, filepath.Join(outside, "go.mod"), "module outside\n")
	if err := os.Symlink(outside, filepath.Join(projectRoot, "linked")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	fragments := []AgentsFragment{{
		RelativePath: "agents-md/outside.md",
		Manifest: &AgentsManifest{
			Version:      1,
			ID:           "outside",
			Layer:        AgentsLayerStack,
			Scope:        AgentsScopeProject,
			Relationship: AgentsRelationshipAll,
			Owner:        "maintainers",
			Canonical:    []string{"docs/outside.md"},
			ReviewOn:     []string{"project moves"},
			Paths:        []string{"linked/go.mod"},
			ContentMarkers: []AgentsContentMarker{{
				Path:     "linked/go.mod",
				Contains: "module outside",
			}},
		},
	}}

	selected, err := SelectAgentsFragments(projectRoot, ".", fragments)
	if err != nil {
		t.Fatalf("SelectAgentsFragments: %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("selected external symlink evidence: %#v", selected)
	}
}

func TestSelectAgentsFragmentsHonorsLayersAndScopedTargets(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	fragments := []AgentsFragment{
		{RelativePath: "agents-md/core.md", Manifest: &AgentsManifest{
			Version: 1, ID: "core", Layer: AgentsLayerCore, Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll,
			Owner: "maintainers", Canonical: []string{"docs/core.md"}, ReviewOn: []string{"policy changes"},
		}},
		{RelativePath: "agents-md/root.md", Manifest: &AgentsManifest{
			Version: 1, ID: "root", Layer: AgentsLayerRoot, Scope: AgentsScopeProject, Relationship: AgentsRelationshipAll,
			Owner: "maintainers", Canonical: []string{"docs/root.md"}, ReviewOn: []string{"layout changes"},
		}},
		{RelativePath: "agents-md/api.md", Manifest: &AgentsManifest{
			Version: 1, ID: "api", Layer: AgentsLayerScoped, Scope: AgentsScopeDirectory, Relationship: AgentsRelationshipAll,
			TargetPaths: []string{"services"}, GuidanceRelationship: AgentsGuidanceException,
			Owner: "api-maintainers", Canonical: []string{"docs/api.md"}, ReviewOn: []string{"route changes"},
		}},
	}

	root, err := SelectAgentsFragments(projectRoot, ".", fragments)
	if err != nil {
		t.Fatalf("SelectAgentsFragments root: %v", err)
	}
	if got, want := selectedFragmentIDs(root), []string{"core", "root"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("root fragments = %v, want %v", got, want)
	}
	api, err := SelectAgentsFragments(projectRoot, "services/api", fragments)
	if err != nil {
		t.Fatalf("SelectAgentsFragments api: %v", err)
	}
	if got, want := selectedFragmentIDs(api), []string{"core", "api"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("api fragments = %v, want %v", got, want)
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

func agentsManifestMarkdown(manifest, body string) string {
	return AgentsManifestPrefix + " " + manifest + " -->\n" + body
}

func writeProjectFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func selectedFragmentIDs(fragments []AgentsFragment) []string {
	ids := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment.Manifest != nil {
			ids = append(ids, fragment.Manifest.ID)
			continue
		}
		ids = append(ids, strings.TrimSuffix(filepath.Base(fragment.RelativePath), ".md"))
	}
	return ids
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
