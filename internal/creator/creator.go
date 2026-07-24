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
	ProjectPath   string
	SourceRoot    string
	FragmentPaths []string
	Model         string
	Command       string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
}

func Run(ctx context.Context, request Request) error {
	prompt := strings.ReplaceAll(promptTemplate, "{{SKILL_NAME}}", request.SkillName)
	prompt = strings.ReplaceAll(prompt, "{{PROJECT_PATH}}", request.ProjectPath)
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
}

func RunAgents(ctx context.Context, request AgentsRequest) error {
	prompt := strings.ReplaceAll(agentsPromptTemplate, "{{PROJECT_PATH}}", request.ProjectPath)
	prompt = strings.ReplaceAll(prompt, "{{SOURCE_ROOT}}", request.SourceRoot)
	prompt = strings.ReplaceAll(prompt, "{{FRAGMENT_PATHS}}", strings.Join(request.FragmentPaths, "\n"))
	return run(ctx, request.ProjectPath, request.Model, request.Command, prompt, request.Stdin, request.Stdout, request.Stderr)
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
