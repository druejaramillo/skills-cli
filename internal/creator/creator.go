package creator

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

//go:embed prompt.md
var promptTemplate string

//go:embed agents_prompt.md
var agentsPromptTemplate string

//go:embed agents_update_prompt.md
var agentsUpdatePromptTemplate string

//go:embed agents_fragment_prompt.md
var agentsFragmentPromptTemplate string

//go:embed agents_fragment_revise_prompt.md
var agentsFragmentRevisePromptTemplate string

type Request struct {
	ProjectPath string
	SkillName   string
	Model       string
	Command     string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

type AgentsRequest struct {
	ProjectPath       string
	TargetRelativeDir string
	TargetPath        string
	StagingPath       string
	SourceRoot        string
	FragmentPaths     []string
	FragmentManifest  string
	Model             string
	Command           string
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer
}

type AgentsFragmentRequest struct {
	ProjectPath          string
	SourceRoot           string
	FragmentPath         string
	ExistingFragmentPath string
	StagingPath          string
	Model                string
	Command              string
	Stdin                io.Reader
	Stdout               io.Writer
	Stderr               io.Writer
}

func Run(ctx context.Context, request Request) error {
	prompt := strings.ReplaceAll(promptTemplate, "{{SKILL_NAME}}", request.SkillName)
	prompt = strings.ReplaceAll(prompt, "{{PROJECT_PATH}}", request.ProjectPath)
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
}

func RunAgents(ctx context.Context, request AgentsRequest) error {
	prompt := expandAgentsPrompt(agentsPromptTemplate, request)
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
}

func RunAgentsUpdate(ctx context.Context, request AgentsRequest) error {
	prompt := expandAgentsPrompt(agentsUpdatePromptTemplate, request)
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
}

func RunAgentsFragment(ctx context.Context, request AgentsFragmentRequest) error {
	prompt := strings.ReplaceAll(agentsFragmentPromptTemplate, "{{PROJECT_PATH}}", request.ProjectPath)
	prompt = strings.ReplaceAll(prompt, "{{SOURCE_ROOT}}", request.SourceRoot)
	prompt = strings.ReplaceAll(prompt, "{{FRAGMENT_PATH}}", request.FragmentPath)
	prompt = strings.ReplaceAll(prompt, "{{STAGING_PATH}}", request.StagingPath)
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
}

func RunAgentsFragmentRevision(ctx context.Context, request AgentsFragmentRequest) error {
	prompt := strings.ReplaceAll(agentsFragmentRevisePromptTemplate, "{{PROJECT_PATH}}", request.ProjectPath)
	prompt = strings.ReplaceAll(prompt, "{{SOURCE_ROOT}}", request.SourceRoot)
	prompt = strings.ReplaceAll(prompt, "{{FRAGMENT_PATH}}", request.FragmentPath)
	prompt = strings.ReplaceAll(prompt, "{{EXISTING_FRAGMENT_PATH}}", request.ExistingFragmentPath)
	prompt = strings.ReplaceAll(prompt, "{{STAGING_PATH}}", request.StagingPath)
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
}

func expandAgentsPrompt(template string, request AgentsRequest) string {
	manifest := request.FragmentManifest
	if manifest == "" {
		manifest = strings.Join(request.FragmentPaths, "\n")
	}
	prompt := strings.ReplaceAll(template, "{{PROJECT_PATH}}", request.ProjectPath)
	prompt = strings.ReplaceAll(prompt, "{{TARGET_RELATIVE_DIR}}", request.TargetRelativeDir)
	prompt = strings.ReplaceAll(prompt, "{{TARGET_PATH}}", request.TargetPath)
	prompt = strings.ReplaceAll(prompt, "{{STAGING_PATH}}", request.StagingPath)
	prompt = strings.ReplaceAll(prompt, "{{SOURCE_ROOT}}", request.SourceRoot)
	prompt = strings.ReplaceAll(prompt, "{{FRAGMENT_MANIFEST}}", manifest)
	prompt = strings.ReplaceAll(prompt, "{{FRAGMENT_PATHS}}", strings.Join(request.FragmentPaths, "\n"))
	return prompt
}

func run(ctx context.Context, projectPath, model, command, prompt string, stdin io.Reader, stdout, stderr io.Writer) error {
	if model == "" {
		return fmt.Errorf("no creator model is configured; run `skills config set creator.model <provider/model>`")
	}
	if command == "" {
		command = "opencode"
	}
	cmd := exec.CommandContext(ctx, command, projectPath, "--model", model, "--prompt", prompt)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if _, lookupErr := exec.LookPath(command); lookupErr != nil {
			return fmt.Errorf("OpenCode is required; install it and authenticate with `opencode providers`: %w", lookupErr)
		}
		return fmt.Errorf("OpenCode session failed: %w", err)
	}
	return nil
}
