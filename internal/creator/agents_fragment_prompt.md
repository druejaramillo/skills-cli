# Agent Fragment Creation Session

Create the reusable agent-guidance fragment that will be published as `{{FRAGMENT_PATH}}`. Write the completed Markdown file only to `{{STAGING_PATH}}`. Do not modify the source or create or edit other files unless the user explicitly asks.

First inspect the current project at `{{PROJECT_PATH}}` and interview the user about this fragment's topic, intended technologies, standards, and constraints. Ask only the questions needed to make the guidance specific and reusable.

The local skills source is rooted at `{{SOURCE_ROOT}}`. You may read existing fragments under its `agents-md` directory for consistency, but do not modify anything in the source.

The file must begin at byte zero with a single JSON manifest inside an HTML comment. Do not use YAML frontmatter. Interview for the layer (`core`, `root`, `scoped`, `stack`, or `verification`), evidence scope (`project` or `directory`), evidence relationship (`all` or `any`), owner, source-relative canonical reference, review trigger, and any required target paths. Scoped fragments also need an `additive` or `exception` guidance relationship. Use only safe relative paths and globs. A minimal example is:

```md
<!-- skills-agents: {"version":1,"id":"go","layer":"stack","scope":"project","relationship":"all","owner":"maintainers","canonical":["docs/testing.md"],"review_on":["Go toolchain changes"],"paths":["go.mod"]} -->
# Go
```

Include concrete, actionable standards and workflows relevant to the requested topic. Verification fragments must require evidence and reporting but must not fabricate a project's commands. Avoid generic advice, duplicated rules, and project-specific conventions that are not supported by the user's answers or the project. Before finishing, verify the staged file is non-empty and contains only the requested fragment.
