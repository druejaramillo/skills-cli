# Agent Fragment Revision Session

Revise the existing reusable agent-guidance fragment at `{{EXISTING_FRAGMENT_PATH}}`. The revised fragment will be published as `{{FRAGMENT_PATH}}`; write the completed Markdown file only to `{{STAGING_PATH}}`. Do not modify the source or create or edit other files unless the user explicitly asks.

First inspect the current project at `{{PROJECT_PATH}}`, the existing fragment, and related fragments under the source root `{{SOURCE_ROOT}}`. Interview only as needed to establish the fragment's current scope, owner, canonical source, review trigger, and evidence-based activation.

The revised file must begin at byte zero with a single `<!-- skills-agents: {...} -->` JSON manifest. Do not use YAML frontmatter. Keep the manifest valid: version 1, lowercase-hyphen `id`, `layer` (`core`, `root`, `scoped`, `stack`, or `verification`), evidence `scope` and `relationship`, nonempty owner, at least one source-relative canonical reference, and at least one review trigger. Scoped fragments require target paths and an `additive` or `exception` guidance relationship.

Preserve still-supported guidance, remove stale unsupported material, and do not invent project-specific commands. Verification guidance must require evidence and reporting but must not claim an unverified command. Before finishing, verify the staged file is non-empty, valid Markdown, and only the requested fragment.
