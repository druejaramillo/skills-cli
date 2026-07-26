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

`skills add` requires Git only for remote sources. `skills create`, `skills agents create`, `skills agents revise`, `skills agents generate`, and `skills agents update` also require [OpenCode](https://opencode.ai), plus an authenticated provider and model configured through `opencode providers`.

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

List the skills installed in the current project:

```bash
skills list
```

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

## Create Agent Fragments

Sources can also provide Markdown fragments for project-level agent guidance. Put them below a source-root `agents-md/` directory; nested directories are supported:

```text
my-skills/
  agents-md/
    go.md
    frontend/
      htmx.md
      tailwind.md
```

Use the same OpenCode model configuration as skill creation, then start an interactive session for an extensionless fragment path:

```bash
skills agents create go --source mine
skills agents create frontend/htmx --source mine --model openai/gpt-5.6-terra
```

OpenCode inspects the current project and interviews you about the fragment's topic, standards, constraints, evidence, owner, and review trigger. It is instructed to write one Markdown fragment to a staging directory. `skills` validates that staged artifact and publishes it into the local source only after validation succeeds. Existing fragments are protected from replacement. Review and commit the source change yourself; `skills` never commits or pushes. Remote sources cannot be used because this command publishes a change.

New fragments begin at byte zero with a JSON manifest in an HTML comment. This is source metadata, not YAML frontmatter and not a runtime `AGENTS.md` standard:

```md
<!-- skills-agents: {
  "version": 1,
  "id": "go",
  "layer": "stack",
  "scope": "project",
  "relationship": "all",
  "owner": "maintainers",
  "canonical": ["docs/testing.md"],
  "review_on": ["Go toolchain changes"],
  "paths": ["go.mod"],
  "globs": ["**/*.go"]
} -->

# Go

- Inspect the active module and nearby tests before changing Go code.
- Use the repository-approved formatting and narrow verification procedure.
```

`layer` is one of `core`, `root`, `scoped`, `stack`, or `verification`. `scope` chooses whether evidence is evaluated from the project root or the generated file's target directory. `relationship` combines `paths`, `globs`, and `content_markers` with `all` or `any`. A `root` fragment applies only to a root target. A `core` fragment is materialized into every generated file; it is not a separately configured OpenCode instruction layer. A `scoped` fragment also requires `target_paths` and `guidance_relationship` (`additive` or `exception`). `stack` and `verification` fragments may optionally limit themselves with `target_paths`.

All manifests need an owner, at least one source-relative canonical reference, and at least one event-based `review_on` trigger. Literal project commands belong only in a deliberately project-specific fragment with a canonical local source. Reusable verification fragments should require evidence and reporting, not invent commands.

Revise an existing local-source fragment through the same staged flow:

```bash
skills agents revise frontend/htmx --source mine
```

Check a source and see which fragments are eligible for the current project or a local target. This command validates source metadata and selection only; it does not execute project commands:

```bash
skills agents validate --source mine
skills agents validate --source mine --path internal/api
```

Fragments created before manifests remain usable as unclassified legacy fragments. `validate` reports them as migration warnings. Invalid or empty fragments stop generation.

## Generate AGENTS.md

Generate a concise root `AGENTS.md` from eligible fragments in a selected source:

```bash
skills agents generate
skills agents generate --source mine --model openai/gpt-5.6-terra
```

OpenCode inspects the project, relevant implementation/tests/docs/CI, and selected source fragments. It is instructed to write a staged candidate. `skills` validates that candidate, then publishes it with a same-directory replacement. If OpenCode exits without a staged file or emits an invalid file, `skills` does not replace the live target.

Generate a local rule for an existing project subdirectory with `--path`:

```bash
skills agents generate --path internal/api
```

This workflow targets OpenCode's local-rule behavior. A nearer local `AGENTS.md` can be selected instead of a root rule, so every generated local file is self-contained for its own target. Do not assume a root file is inherited below it. `skills` does not edit `opencode.json` or configure a globally loaded instruction core.

Existing `AGENTS.md` files are protected unless `--force` is supplied. `--force` explicitly creates a fresh replacement through the same staged atomic publication flow:

```bash
skills agents generate --force
```

Use `update` instead of `--force` when an existing file should be reconciled with current project facts and source fragments. The session is instructed to read the live file and write only the staged candidate; verified local and user-authored constraints are preserved unless inspected canonical evidence supersedes them:

```bash
skills agents update
skills agents update --path internal/api --source mine
```

Generated guidance requires bounded, human-approved work and reporting of changed scope, checks run/results, and checks not run with reasons. It may name a locally verified command or canonical procedure, but `skills` never runs verification commands and never guesses missing ones. OpenCode runs with the user's normal filesystem authority, so staging instructions are not a sandbox; inspect the worktree after every session. `AGENTS.md` is guidance only: it does not grant authority, replace human review, enforce permissions, or protect secrets and external systems.

Use `skills --help` for the complete command reference.
