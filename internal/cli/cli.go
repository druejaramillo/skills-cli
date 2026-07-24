package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/druejaramillo/skills-cli/internal/config"
	"github.com/druejaramillo/skills-cli/internal/creator"
	"github.com/druejaramillo/skills-cli/internal/project"
	"github.com/druejaramillo/skills-cli/internal/source"
)

type App struct {
	ConfigPath string
	CacheDir   string
	WorkingDir string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

func New() (*App, error) {
	configPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	cacheDir, err := config.DefaultCacheDir()
	if err != nil {
		return nil, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	return &App{
		ConfigPath: configPath,
		CacheDir:   cacheDir,
		WorkingDir: workingDir,
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}, nil
}

func (app *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		app.usage()
		return nil
	}
	switch args[0] {
	case "source":
		return app.runSource(args[1:])
	case "add":
		return app.runAdd(ctx, args[1:])
	case "remove":
		return app.runRemove(args[1:])
	case "config":
		return app.runConfig(args[1:])
	case "create":
		return app.runCreate(ctx, args[1:])
	case "agents":
		return app.runAgents(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q; run `skills --help`", args[0])
	}
}

func (app *App) runSource(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprint(app.Stdout, "Usage: skills source <add|list|remove|default>\n")
		return nil
	}
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		positionals, flags, err := parseArgs(args[1:], map[string]bool{"default": false})
		if err != nil {
			return err
		}
		if len(positionals) != 2 {
			return errors.New("usage: skills source add <name> <path-or-git-url> [--default]")
		}
		name := positionals[0]
		if err := source.ValidateName(name); err != nil {
			return fmt.Errorf("invalid source name: %w", err)
		}
		if _, exists := cfg.Sources[name]; exists {
			return fmt.Errorf("source %q already exists", name)
		}
		location, err := source.AddLocation(positionals[1])
		if err != nil {
			return err
		}
		cfg.Sources[name] = location
		if flags["default"] == "true" || cfg.DefaultSource == "" {
			cfg.DefaultSource = name
		}
		if err := config.Save(app.ConfigPath, cfg); err != nil {
			return err
		}
		fmt.Fprintf(app.Stdout, "Added %s source %q.\n", locationKind(location), name)
		return nil
	case "list":
		if len(args) != 1 {
			return errors.New("usage: skills source list")
		}
		names := make([]string, 0, len(cfg.Sources))
		for name := range cfg.Sources {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) == 0 {
			fmt.Fprintln(app.Stdout, "No skill sources are configured.")
			return nil
		}
		for _, name := range names {
			marker := ""
			if name == cfg.DefaultSource {
				marker = " (default)"
			}
			fmt.Fprintf(app.Stdout, "%s\t%s\t%s%s\n", name, locationKind(cfg.Sources[name]), cfg.Sources[name].Location, marker)
		}
		return nil
	case "remove":
		if len(args) != 2 {
			return errors.New("usage: skills source remove <name>")
		}
		name := args[1]
		removed, exists := cfg.Sources[name]
		if !exists {
			return fmt.Errorf("source %q is not configured", name)
		}
		if removed.Remote && source.ValidateName(name) == nil {
			if err := os.RemoveAll(filepath.Join(app.CacheDir, name)); err != nil {
				return fmt.Errorf("remove source cache: %w", err)
			}
		}
		delete(cfg.Sources, name)
		if cfg.DefaultSource == name {
			cfg.DefaultSource = ""
		}
		if err := config.Save(app.ConfigPath, cfg); err != nil {
			return err
		}
		fmt.Fprintf(app.Stdout, "Removed source %q.\n", name)
		return nil
	case "default":
		if len(args) != 2 {
			return errors.New("usage: skills source default <name>")
		}
		name := args[1]
		if _, exists := cfg.Sources[name]; !exists {
			return fmt.Errorf("source %q is not configured", name)
		}
		cfg.DefaultSource = name
		if err := config.Save(app.ConfigPath, cfg); err != nil {
			return err
		}
		fmt.Fprintf(app.Stdout, "Set default source to %q.\n", name)
		return nil
	default:
		return fmt.Errorf("unknown source command %q", args[0])
	}
}

func (app *App) runAdd(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "force": false})
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: skills add <skill-or-relative-path> [--source <name>] [--force]")
	}
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		return err
	}
	name, src, err := selectedSource(cfg, flags["source"])
	if err != nil {
		return err
	}
	root, err := source.Prepare(ctx, src, name, app.CacheDir)
	if err != nil {
		return err
	}
	skills, err := source.Discover(root)
	if err != nil {
		return err
	}
	skill, err := source.Resolve(skills, positionals[0])
	if err != nil {
		return err
	}
	destination, err := project.SkillPath(app.WorkingDir, skill.Name)
	if err != nil {
		return err
	}
	if err := project.CopySkill(skill.Path, destination, flags["force"] == "true"); err != nil {
		return err
	}
	fmt.Fprintf(app.Stdout, "Installed skill %q from source %q at %s.\n", skill.Name, name, destination)
	return nil
}

func (app *App) runRemove(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: skills remove <skill-name>")
	}
	if err := project.RemoveSkill(app.WorkingDir, args[0]); err != nil {
		return err
	}
	fmt.Fprintf(app.Stdout, "Removed skill %q.\n", args[0])
	return nil
}

func (app *App) runConfig(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprint(app.Stdout, "Usage: skills config <set|get> creator.model [provider/model]\n")
		return nil
	}
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		return err
	}
	switch args[0] {
	case "set":
		if len(args) != 3 || args[1] != "creator.model" || strings.TrimSpace(args[2]) == "" {
			return errors.New("usage: skills config set creator.model <provider/model>")
		}
		cfg.Creator.Model = args[2]
		if err := config.Save(app.ConfigPath, cfg); err != nil {
			return err
		}
		fmt.Fprintln(app.Stdout, "Set creator model.")
		return nil
	case "get":
		if len(args) != 2 || args[1] != "creator.model" {
			return errors.New("usage: skills config get creator.model")
		}
		if cfg.Creator.Model == "" {
			return errors.New("creator model is not configured; run `skills config set creator.model <provider/model>`")
		}
		fmt.Fprintln(app.Stdout, cfg.Creator.Model)
		return nil
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func (app *App) runCreate(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "model": true})
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: skills create <skill-name> [--source <local-source>] [--model <provider/model>]")
	}
	skillName := positionals[0]
	if err := source.ValidateName(skillName); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		return err
	}
	sourceName, src, err := selectedSource(cfg, flags["source"])
	if err != nil {
		return err
	}
	if src.Remote {
		return fmt.Errorf("source %q is remote; skills create needs a local source so it can leave changes for your Git review", sourceName)
	}
	sourceRoot, err := source.Prepare(ctx, src, sourceName, app.CacheDir)
	if err != nil {
		return err
	}
	projectDestination, err := project.SkillPath(app.WorkingDir, skillName)
	if err != nil {
		return err
	}
	sourceDestination := filepath.Join(sourceRoot, skillName)
	if pathExists(projectDestination) {
		return fmt.Errorf("project skill %q already exists", projectDestination)
	}
	if pathExists(sourceDestination) {
		return fmt.Errorf("source skill %q already exists", sourceDestination)
	}
	model := flags["model"]
	if model == "" {
		model = cfg.Creator.Model
	}
	if err := creator.Run(ctx, creator.Request{
		ProjectPath: app.WorkingDir,
		SkillName:   skillName,
		Model:       model,
		Stdin:       app.Stdin,
		Stdout:      app.Stdout,
		Stderr:      app.Stderr,
	}); err != nil {
		return err
	}
	if !pathExists(projectDestination) {
		fmt.Fprintf(app.Stdout, "OpenCode exited without creating %s; nothing was published.\n", projectDestination)
		return nil
	}
	if _, err := source.ValidateSkillDirectory(projectDestination); err != nil {
		return fmt.Errorf("created skill was not published: %w", err)
	}
	if err := project.CopySkill(projectDestination, sourceDestination, false); err != nil {
		return fmt.Errorf("publish skill to source: %w", err)
	}
	fmt.Fprintf(app.Stdout, "Created %s and published it to %s. Review and commit the source change yourself.\n", projectDestination, sourceDestination)
	return nil
}

func (app *App) runAgents(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) || (args[0] == "create" && len(args) == 2 && isHelp(args[1])) {
		fmt.Fprint(app.Stdout, "Usage: skills agents create [--source <name>] [--model <provider/model>] [--force]\n")
		return nil
	}
	if args[0] != "create" {
		return fmt.Errorf("unknown agents command %q", args[0])
	}
	positionals, flags, err := parseArgs(args[1:], map[string]bool{"source": true, "model": true, "force": false})
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: skills agents create [--source <name>] [--model <provider/model>] [--force]")
	}
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		return err
	}
	sourceName, src, err := selectedSource(cfg, flags["source"])
	if err != nil {
		return err
	}
	sourceRoot, err := source.Prepare(ctx, src, sourceName, app.CacheDir)
	if err != nil {
		return err
	}
	fragments, err := source.DiscoverAgentsFragments(sourceRoot)
	if err != nil {
		return err
	}
	destination := project.AgentsPath(app.WorkingDir)
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("AGENTS.md destination %q is not a regular file", destination)
		}
		if flags["force"] != "true" {
			return fmt.Errorf("AGENTS.md %q already exists; use --force to replace it", destination)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect AGENTS.md destination: %w", err)
	}
	model := flags["model"]
	if model == "" {
		model = cfg.Creator.Model
	}
	fragmentPaths := make([]string, len(fragments))
	for i, fragment := range fragments {
		fragmentPaths[i] = fragment.RelativePath
	}
	if err := creator.RunAgents(ctx, creator.AgentsRequest{
		ProjectPath:   app.WorkingDir,
		SourceRoot:    sourceRoot,
		FragmentPaths: fragmentPaths,
		Model:         model,
		Stdin:         app.Stdin,
		Stdout:        app.Stdout,
		Stderr:        app.Stderr,
	}); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(app.Stdout, "OpenCode exited without creating %s; nothing was generated.\n", destination)
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect generated AGENTS.md: %w", err)
	}
	if err := project.ValidateAgentsFile(destination); err != nil {
		return err
	}
	fmt.Fprintf(app.Stdout, "Created %s from source %q.\n", destination, sourceName)
	return nil
}

func selectedSource(cfg config.Config, requested string) (string, config.Source, error) {
	name := requested
	if name == "" {
		name = cfg.DefaultSource
	}
	if name == "" {
		return "", config.Source{}, errors.New("no default source is configured; run `skills source add <name> <path-or-git-url> --default`")
	}
	src, ok := cfg.Sources[name]
	if !ok {
		return "", config.Source{}, fmt.Errorf("source %q is not configured", name)
	}
	return name, src, nil
}

func parseArgs(args []string, allowed map[string]bool) ([]string, map[string]string, error) {
	positionals := make([]string, 0, len(args))
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		needsValue, known := allowed[name]
		if !known {
			return nil, nil, fmt.Errorf("unknown option %q", arg)
		}
		if _, exists := flags[name]; exists {
			return nil, nil, fmt.Errorf("option %q was provided more than once", arg)
		}
		if !needsValue {
			flags[name] = "true"
			continue
		}
		if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
			return nil, nil, fmt.Errorf("option %q requires a value", arg)
		}
		i++
		flags[name] = args[i]
	}
	return positionals, flags, nil
}

func locationKind(src config.Source) string {
	if src.Remote {
		return "remote"
	}
	return "local"
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isHelp(value string) bool {
	return value == "--help" || value == "-h" || value == "help"
}

func (app *App) usage() {
	fmt.Fprint(app.Stdout, `Skills installs and creates Agent Skills.

Usage:
  skills source add <name> <path-or-git-url> [--default]
  skills source list
  skills source remove <name>
  skills source default <name>
  skills add <skill-or-relative-path> [--source <name>] [--force]
  skills remove <skill-name>
  skills config set creator.model <provider/model>
  skills config get creator.model
  skills create <skill-name> [--source <local-source>] [--model <provider/model>]
  skills agents create [--source <name>] [--model <provider/model>] [--force]

Run skills create to open an interactive OpenCode session that interviews you
and creates a skill in .agents/skills before copying it to your local source.
Run skills agents create to synthesize a project-root AGENTS.md from the
selected source's agents-md fragments.
`)
}
