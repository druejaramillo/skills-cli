# Skills CLI

`skills` installs and creates [Agent Skills](https://agentskills.io) from a personal or shared skill repository. Installed skills live in the current project's `.agents/skills/<skill-name>` directory.

## Install

```bash
go install github.com/druejaramillo/skills-cli/cmd/skills@latest
```

For a local checkout:

```bash
go install ./cmd/skills
```

`skills add` requires Git only for remote sources. `skills create` and `skills agents create` also require [OpenCode](https://opencode.ai), plus an authenticated provider and model configured through `opencode providers`.

## Sources

Register a named source. It can be a local directory, a Git URL, or a GitHub `owner/repo` shorthand:

```bash
skills source add mine ~/Code/my-skills --default
skills source add community acme/agent-skills
skills source list
```

Local sources are read directly, so uncommitted changes are available immediately. Remote sources are shallow-cloned into the user cache and refreshed before each read.

A source can organize skills in nested directories. Every skill must be a directory containing a `SKILL.md` with YAML frontmatter. The directory name and frontmatter `name` must match and use lowercase letters, numbers, and hyphens:

```text
my-skills/
  engineering/
    tdd/
      SKILL.md
      references/
```

## Install and Remove Skills

Install a skill by its unique name or its source-relative path:

```bash
skills add tdd
skills add engineering/tdd --source mine
skills remove tdd
```

The entire skill directory is copied to `.agents/skills/tdd`, including `scripts`, `references`, and `assets`. Existing skills are protected unless `--force` is supplied.

## Create a Skill

Configure the model that OpenCode should use:

```bash
opencode providers
skills config set creator.model openai/gpt-5.6-terra
```

Then create a skill against a local source:

```bash
skills create tdd --source mine
```

This opens OpenCode's normal interactive terminal UI, preloaded with instructions to interview you about the skill. Answer its follow-up questions in that session. OpenCode creates the result in the current project's `.agents/skills/tdd`; after you exit, `skills` validates it and copies it into the local source as `~/Code/my-skills/tdd`.

Skills never commits or pushes. Review the source repository and commit or push it yourself.

## Create AGENTS.md

Sources can also provide Markdown fragments for project-level agent guidance. Put them below a source-root `agents-md/` directory; nested directories are supported:

```text
my-skills/
  agents-md/
    go.md
    frontend/
      htmx.md
      tailwind.md
```

Use the same OpenCode model configuration as skill creation, then start an interactive session:

```bash
skills agents create
skills agents create --source mine --model openai/gpt-5.6-terra
```

OpenCode inspects the project, asks about its stack and conventions, reads the relevant fragments from the selected source, and writes a concise `AGENTS.md` in the current project directory. Local sources are read directly; remote sources are refreshed in the cache before the session. Existing `AGENTS.md` files are protected unless `--force` is supplied:

```bash
skills agents create --force
```

Use `skills --help` for the complete command reference.
