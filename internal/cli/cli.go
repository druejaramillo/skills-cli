package cli

import (
	"context"
	"encoding/json"
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
	case "list":
		return app.runList(args[1:])
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

func (app *App) runList(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: skills list")
	}
	skills, err := project.ListSkills(app.WorkingDir)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		fmt.Fprintln(app.Stdout, "No skills are installed.")
		return nil
	}
	for _, skill := range skills {
		if skill.Description == "" {
			fmt.Fprintln(app.Stdout, skill.Name)
			continue
		}
		fmt.Fprintf(app.Stdout, "%s\t%s\n", skill.Name, skill.Description)
	}
	return nil
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
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprint(app.Stdout, `Usage:
	  skills agents create <fragment-path> [--source <local-source>] [--model <provider/model>]
	  skills agents revise <fragment-path> [--source <local-source>] [--model <provider/model>]
	  skills agents validate [--source <name>] [--path <relative-dir>]
	  skills agents generate [--source <name>] [--model <provider/model>] [--path <relative-dir>] [--force]
	  skills agents update [--source <name>] [--model <provider/model>] [--path <relative-dir>]
`)
		return nil
	}
	switch args[0] {
	case "create":
		if len(args) == 2 && isHelp(args[1]) {
			fmt.Fprint(app.Stdout, "Usage: skills agents create <fragment-path> [--source <local-source>] [--model <provider/model>]\n")
			return nil
		}
		return app.runAgentsCreate(ctx, args[1:])
	case "revise":
		if len(args) == 2 && isHelp(args[1]) {
			fmt.Fprint(app.Stdout, "Usage: skills agents revise <fragment-path> [--source <local-source>] [--model <provider/model>]\n")
			return nil
		}
		return app.runAgentsRevise(ctx, args[1:])
	case "validate":
		if len(args) == 2 && isHelp(args[1]) {
			fmt.Fprint(app.Stdout, "Usage: skills agents validate [--source <name>] [--path <relative-dir>]\n")
			return nil
		}
		return app.runAgentsValidate(ctx, args[1:])
	case "generate":
		if len(args) == 2 && isHelp(args[1]) {
			fmt.Fprint(app.Stdout, "Usage: skills agents generate [--source <name>] [--model <provider/model>] [--path <relative-dir>] [--force]\n")
			return nil
		}
		return app.runAgentsGenerate(ctx, args[1:])
	case "update":
		if len(args) == 2 && isHelp(args[1]) {
			fmt.Fprint(app.Stdout, "Usage: skills agents update [--source <name>] [--model <provider/model>] [--path <relative-dir>]\n")
			return nil
		}
		return app.runAgentsUpdate(ctx, args[1:])
	default:
		return fmt.Errorf("unknown agents command %q", args[0])
	}
}

func (app *App) runAgentsCreate(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "model": true})
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: skills agents create <fragment-path> [--source <local-source>] [--model <provider/model>]")
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
		return fmt.Errorf("source %q is remote; skills agents create needs a local source so it can leave changes for your Git review", sourceName)
	}
	sourceRoot, err := source.Prepare(ctx, src, sourceName, app.CacheDir)
	if err != nil {
		return err
	}
	destination, relativePath, err := source.AgentsFragmentPath(sourceRoot, positionals[0])
	if err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("agents fragment %q already exists", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect agents fragment destination: %w", err)
	}
	model := flags["model"]
	if model == "" {
		model = cfg.Creator.Model
	}
	stagingRoot, err := os.MkdirTemp(app.WorkingDir, ".skills-agents-*")
	if err != nil {
		return fmt.Errorf("create agents fragment staging directory: %w", err)
	}
	defer os.RemoveAll(stagingRoot)
	stagingPath := filepath.Join(stagingRoot, filepath.FromSlash(relativePath))
	if err := creator.RunAgentsFragment(ctx, creator.AgentsFragmentRequest{
		ProjectPath:  app.WorkingDir,
		SourceRoot:   sourceRoot,
		FragmentPath: relativePath,
		StagingPath:  stagingPath,
		Model:        model,
		Stdin:        app.Stdin,
		Stdout:       app.Stdout,
		Stderr:       app.Stderr,
	}); err != nil {
		return err
	}
	if _, err := os.Lstat(stagingPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(app.Stdout, "OpenCode exited without creating %s; nothing was published.\n", stagingPath)
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect generated agents fragment: %w", err)
	}
	if err := source.ValidateAgentsFragment(stagingPath); err != nil {
		return fmt.Errorf("created agents fragment was not published: %w", err)
	}
	if err := validateAgentsFragmentCandidate(sourceRoot, stagingPath, ""); err != nil {
		return fmt.Errorf("created agents fragment was not published: %w", err)
	}
	if err := project.PublishFile(stagingPath, destination); err != nil {
		return fmt.Errorf("publish agents fragment to source: %w", err)
	}
	fmt.Fprintf(app.Stdout, "Created %s and published it to %s. Review and commit the source change yourself.\n", relativePath, destination)
	return nil
}

func (app *App) runAgentsRevise(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "model": true})
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: skills agents revise <fragment-path> [--source <local-source>] [--model <provider/model>]")
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
		return fmt.Errorf("source %q is remote; skills agents revise needs a local source so it can leave changes for your Git review", sourceName)
	}
	sourceRoot, err := source.Prepare(ctx, src, sourceName, app.CacheDir)
	if err != nil {
		return err
	}
	destination, relativePath, err := source.AgentsFragmentPath(sourceRoot, positionals[0])
	if err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("agents fragment %q does not exist; use `skills agents create`", destination)
	}
	if err != nil {
		return fmt.Errorf("inspect agents fragment destination: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("agents fragment %q is not a regular file", destination)
	}

	stagingRoot, err := os.MkdirTemp(app.WorkingDir, ".skills-agents-*")
	if err != nil {
		return fmt.Errorf("create agents fragment staging directory: %w", err)
	}
	defer os.RemoveAll(stagingRoot)
	stagingPath := filepath.Join(stagingRoot, filepath.FromSlash(relativePath))
	model := creatorModel(cfg, flags)
	if err := creator.RunAgentsFragmentRevision(ctx, creator.AgentsFragmentRequest{
		ProjectPath:          app.WorkingDir,
		SourceRoot:           sourceRoot,
		FragmentPath:         relativePath,
		ExistingFragmentPath: destination,
		StagingPath:          stagingPath,
		Model:                model,
		Stdin:                app.Stdin,
		Stdout:               app.Stdout,
		Stderr:               app.Stderr,
	}); err != nil {
		return err
	}
	if _, err := os.Lstat(stagingPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(app.Stdout, "OpenCode exited without creating %s; the existing fragment was unchanged.\n", stagingPath)
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect revised agents fragment: %w", err)
	}
	if err := source.ValidateAgentsFragment(stagingPath); err != nil {
		return fmt.Errorf("revised agents fragment was not published: %w", err)
	}
	if err := validateAgentsFragmentCandidate(sourceRoot, stagingPath, destination); err != nil {
		return fmt.Errorf("revised agents fragment was not published: %w", err)
	}
	if err := project.ReplaceFile(stagingPath, destination); err != nil {
		return fmt.Errorf("replace agents fragment in source: %w", err)
	}
	fmt.Fprintf(app.Stdout, "Revised %s at %s. Review and commit the source change yourself.\n", relativePath, destination)
	return nil
}

func (app *App) runAgentsGenerate(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "model": true, "path": true, "force": false})
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: skills agents generate [--source <name>] [--model <provider/model>] [--path <relative-dir>] [--force]")
	}
	composition, cfg, err := app.prepareAgentsComposition(ctx, flags, true)
	if err != nil {
		return err
	}
	if err := ensureAgentsDestination(composition.destination, flags["force"] == "true", false); err != nil {
		return err
	}
	return app.renderAgents(ctx, cfg, flags, composition, false)
}

func (app *App) runAgentsUpdate(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "model": true, "path": true})
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: skills agents update [--source <name>] [--model <provider/model>] [--path <relative-dir>]")
	}
	composition, cfg, err := app.prepareAgentsComposition(ctx, flags, true)
	if err != nil {
		return err
	}
	if err := ensureAgentsDestination(composition.destination, true, true); err != nil {
		return err
	}
	return app.renderAgents(ctx, cfg, flags, composition, true)
}

func (app *App) runAgentsValidate(ctx context.Context, args []string) error {
	positionals, flags, err := parseArgs(args, map[string]bool{"source": true, "path": true})
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: skills agents validate [--source <name>] [--path <relative-dir>]")
	}
	composition, _, err := app.prepareAgentsComposition(ctx, flags, false)
	if err != nil {
		return err
	}
	fmt.Fprintf(app.Stdout, "Eligible fragments for %s:\n", composition.targetRelativeDir)
	for _, fragment := range composition.fragments {
		fmt.Fprintf(app.Stdout, "  %s\n", agentsFragmentSummary(fragment))
	}
	return nil
}

type agentsComposition struct {
	sourceName        string
	sourceRoot        string
	targetRelativeDir string
	destination       string
	fragments         []source.AgentsFragment
}

func (app *App) prepareAgentsComposition(ctx context.Context, flags map[string]string, requireFragments bool) (agentsComposition, config.Config, error) {
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		return agentsComposition{}, config.Config{}, err
	}
	sourceName, src, err := selectedSource(cfg, flags["source"])
	if err != nil {
		return agentsComposition{}, config.Config{}, err
	}
	sourceRoot, err := source.Prepare(ctx, src, sourceName, app.CacheDir)
	if err != nil {
		return agentsComposition{}, config.Config{}, err
	}
	targetRelativeDir := flags["path"]
	if targetRelativeDir == "" {
		targetRelativeDir = "."
	}
	destination, err := project.AgentsPathAt(app.WorkingDir, targetRelativeDir)
	if err != nil {
		return agentsComposition{}, config.Config{}, err
	}
	fragments, diagnostics, err := source.InspectAgentsFragments(sourceRoot)
	if err != nil {
		return agentsComposition{}, config.Config{}, err
	}
	writeAgentsDiagnostics(app.Stdout, diagnostics)
	if err := diagnostics.Err(); err != nil {
		return agentsComposition{}, config.Config{}, fmt.Errorf("validate agents fragments: %w", err)
	}
	selected, err := source.SelectAgentsFragments(app.WorkingDir, targetRelativeDir, fragments)
	if err != nil {
		return agentsComposition{}, config.Config{}, err
	}
	if requireFragments && len(selected) == 0 {
		return agentsComposition{}, config.Config{}, fmt.Errorf("no agents fragments are eligible for target %q", targetRelativeDir)
	}
	return agentsComposition{
		sourceName:        sourceName,
		sourceRoot:        sourceRoot,
		targetRelativeDir: targetRelativeDir,
		destination:       destination,
		fragments:         selected,
	}, cfg, nil
}

func (app *App) renderAgents(ctx context.Context, cfg config.Config, flags map[string]string, composition agentsComposition, update bool) error {
	stagingRoot, err := os.MkdirTemp(app.WorkingDir, ".skills-agents-*")
	if err != nil {
		return fmt.Errorf("create AGENTS.md staging directory: %w", err)
	}
	defer os.RemoveAll(stagingRoot)
	stagingPath := filepath.Join(stagingRoot, "AGENTS.md")
	request := creator.AgentsRequest{
		ProjectPath:       app.WorkingDir,
		TargetRelativeDir: composition.targetRelativeDir,
		TargetPath:        composition.destination,
		StagingPath:       stagingPath,
		SourceRoot:        composition.sourceRoot,
		FragmentPaths:     agentsFragmentPaths(composition.fragments),
		FragmentManifest:  agentsFragmentManifest(composition.fragments),
		Model:             creatorModel(cfg, flags),
		Stdin:             app.Stdin,
		Stdout:            app.Stdout,
		Stderr:            app.Stderr,
	}
	if update {
		err = creator.RunAgentsUpdate(ctx, request)
	} else {
		err = creator.RunAgents(ctx, request)
	}
	if err != nil {
		return err
	}
	if _, err := os.Lstat(stagingPath); errors.Is(err, os.ErrNotExist) {
		if update {
			fmt.Fprintf(app.Stdout, "OpenCode exited without creating %s; the existing AGENTS.md was unchanged.\n", stagingPath)
		} else {
			fmt.Fprintf(app.Stdout, "OpenCode exited without creating %s; nothing was generated.\n", stagingPath)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect generated AGENTS.md: %w", err)
	}
	if err := project.ValidateGeneratedAgentsFile(stagingPath, composition.targetRelativeDir); err != nil {
		return err
	}
	currentDestination, err := project.AgentsPathAt(app.WorkingDir, composition.targetRelativeDir)
	if err != nil {
		return fmt.Errorf("recheck AGENTS.md target after OpenCode session: %w", err)
	}
	if currentDestination != composition.destination {
		return errors.New("AGENTS.md target changed during the OpenCode session")
	}
	if err := project.ReplaceFile(stagingPath, composition.destination); err != nil {
		return fmt.Errorf("publish AGENTS.md: %w", err)
	}
	if update {
		fmt.Fprintf(app.Stdout, "Updated %s from source %q.\n", composition.destination, composition.sourceName)
	} else {
		fmt.Fprintf(app.Stdout, "Created %s from source %q.\n", composition.destination, composition.sourceName)
	}
	return nil
}

func ensureAgentsDestination(destination string, allowExisting, requireExisting bool) error {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		if requireExisting {
			return fmt.Errorf("AGENTS.md %q does not exist; use `skills agents generate`", destination)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect AGENTS.md destination: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("AGENTS.md destination %q is not a regular file", destination)
	}
	if !allowExisting {
		return fmt.Errorf("AGENTS.md %q already exists; use --force to replace it or `skills agents update` to reconcile it", destination)
	}
	return nil
}

func creatorModel(cfg config.Config, flags map[string]string) string {
	if flags["model"] != "" {
		return flags["model"]
	}
	return cfg.Creator.Model
}

func writeAgentsDiagnostics(output io.Writer, diagnostics source.AgentsDiagnostics) {
	for _, diagnostic := range diagnostics.Warnings {
		fmt.Fprintf(output, "Warning: %s: %s\n", diagnostic.Path, diagnostic.Message)
	}
	for _, diagnostic := range diagnostics.Errors {
		fmt.Fprintf(output, "Error: %s: %s\n", diagnostic.Path, diagnostic.Message)
	}
}

func agentsFragmentPaths(fragments []source.AgentsFragment) []string {
	paths := make([]string, len(fragments))
	for index, fragment := range fragments {
		paths[index] = fragment.RelativePath
	}
	return paths
}

func agentsFragmentManifest(fragments []source.AgentsFragment) string {
	lines := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment.Manifest == nil {
			lines = append(lines, fragment.RelativePath+" (legacy, unclassified)")
			continue
		}
		encoded, err := json.Marshal(fragment.Manifest)
		if err != nil {
			lines = append(lines, fragment.RelativePath)
			continue
		}
		lines = append(lines, fragment.RelativePath+" "+string(encoded))
	}
	return strings.Join(lines, "\n")
}

func agentsFragmentSummary(fragment source.AgentsFragment) string {
	if fragment.Manifest == nil {
		return fragment.RelativePath + " (legacy, unclassified)"
	}
	return fmt.Sprintf("%s (%s, %s)", fragment.RelativePath, fragment.Manifest.Layer, fragment.Manifest.ID)
}

func validateAgentsFragmentCandidate(sourceRoot, candidatePath, replacedPath string) error {
	if _, err := os.Stat(filepath.Join(sourceRoot, "agents-md")); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect agents fragment directory: %w", err)
	}
	fragments, diagnostics, err := source.InspectAgentsFragments(sourceRoot)
	if err != nil {
		return err
	}
	if err := diagnostics.Err(); err != nil {
		return fmt.Errorf("validate existing agents fragments: %w", err)
	}
	contents, err := os.ReadFile(candidatePath)
	if err != nil {
		return fmt.Errorf("read staged agents fragment: %w", err)
	}
	manifest, present, err := source.ParseAgentsManifest(contents)
	if err != nil {
		return fmt.Errorf("parse staged agents fragment manifest: %w", err)
	}
	if !present || manifest == nil {
		return errors.New("staged agents fragment is missing a manifest")
	}
	for _, fragment := range fragments {
		if fragment.Path == replacedPath || fragment.Manifest == nil {
			continue
		}
		if fragment.Manifest.ID == manifest.ID {
			return fmt.Errorf("manifest id %q is already used by %s", manifest.ID, fragment.RelativePath)
		}
	}
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
  skills list
  skills source add <name> <path-or-git-url> [--default]
  skills source list
  skills source remove <name>
  skills source default <name>
  skills add <skill-or-relative-path> [--source <name>] [--force]
  skills remove <skill-name>
  skills config set creator.model <provider/model>
  skills config get creator.model
  skills create <skill-name> [--source <local-source>] [--model <provider/model>]
  skills agents create <fragment-path> [--source <local-source>] [--model <provider/model>]
  skills agents revise <fragment-path> [--source <local-source>] [--model <provider/model>]
  skills agents validate [--source <name>] [--path <relative-dir>]
  skills agents generate [--source <name>] [--model <provider/model>] [--path <relative-dir>] [--force]
  skills agents update [--source <name>] [--model <provider/model>] [--path <relative-dir>]

Run skills create to open an interactive OpenCode session that interviews you
and creates a skill in .agents/skills before copying it to your local source.
Run skills agents create or revise to maintain source agents-md fragments.
Use skills agents validate to inspect eligible fragments, then generate or update
an OpenCode-targeted AGENTS.md from those fragments.
`)
}
