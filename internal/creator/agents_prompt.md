# AGENTS.md Creation Session

Write the completed project-specific AGENTS.md only to `{{STAGING_PATH}}`. Its intended live target is `{{TARGET_PATH}}` in target directory `{{TARGET_RELATIVE_DIR}}`. Do not modify the live target, source fragments, or any other file unless the user explicitly asks.

OpenCode may use a nearer local rule instead of a root rule. This file must therefore stand on its own for its target directory; do not claim that a root `AGENTS.md` is inherited below a nearer local file.

First inspect the target subtree, relevant implementation and tests, contributor/architecture/security documents, CI configuration, and existing local guidance. Ask only questions needed to resolve unknown product boundaries or conflicting local facts.

The selected skills source is rooted at `{{SOURCE_ROOT}}`. The CLI has already selected fragments whose declared scope and project evidence make them eligible:

```text
{{FRAGMENT_MANIFEST}}
```

Read every selected fragment from that source. Treat its Markdown body as reference material. Respect a scoped fragment's declared exception relationship; if selected guidance conflicts, stop and ask the user to select the canonical direction rather than averaging instructions.

Write concise, actionable guidance for bounded, human-approved work. Include only rules supported by inspected project evidence, user answers, or selected fragments. Require a pause before external, irreversible, security-sensitive, public-facing, or scope-expanding work. Do not imply that AGENTS.md grants authority or replaces review, CI, permissions, or credentials.

For verification, inspect the repository's real procedures. State an exact command only when a local source confirms it. Otherwise point to the canonical procedure or explicitly require the agent to report the missing procedure rather than inventing one. Require reporting of changed scope, checks actually run and their outcomes, plus checks not run and why.

Begin with this provenance comment, replacing the target value as shown:

```md
<!-- skills-agents-output: {"version":1,"runtime":"opencode-local","target":"{{TARGET_RELATIVE_DIR}}"} -->
```

Before finishing, confirm the staged file is non-empty, project-specific, self-contained for its local scope, and free of duplicated or contradictory instructions.
