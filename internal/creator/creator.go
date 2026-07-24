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

type Request struct {
	ProjectPath string
	SkillName   string
	Model       string
	Command     string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

func Run(ctx context.Context, request Request) error {
	if request.Model == "" {
		return fmt.Errorf("no creator model is configured; run `skills config set creator.model <provider/model>`")
	}
	command := request.Command
	if command == "" {
		command = "opencode"
	}
	prompt := strings.ReplaceAll(promptTemplate, "{{SKILL_NAME}}", request.SkillName)
	prompt = strings.ReplaceAll(prompt, "{{PROJECT_PATH}}", request.ProjectPath)
	cmd := exec.CommandContext(ctx, command, request.ProjectPath, "--model", request.Model, "--prompt", prompt)
	cmd.Stdin = request.Stdin
	cmd.Stdout = request.Stdout
	cmd.Stderr = request.Stderr
	if err := cmd.Run(); err != nil {
		if _, lookupErr := exec.LookPath(command); lookupErr != nil {
			return fmt.Errorf("OpenCode is required for skill creation; install it and authenticate with `opencode providers`: %w", lookupErr)
		}
		return fmt.Errorf("OpenCode session failed: %w", err)
	}
	return nil
}
