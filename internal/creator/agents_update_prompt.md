# AGENTS.md Update Session

Write the complete reconciled AGENTS.md only to `{{STAGING_PATH}}`. Read the existing project guidance at `{{TARGET_PATH}}` and reconcile it with current inspected project facts and selected source fragments for target directory `{{TARGET_RELATIVE_DIR}}`. Do not modify the live target, source fragments, or any other file unless the user explicitly asks.

OpenCode may use this nearer local rule instead of a root rule. Keep this output self-contained for its target directory; do not claim root-rule inheritance.

First inspect the target subtree, relevant implementation and tests, contributor/architecture/security documents, CI configuration, and existing local guidance. Ask only questions needed to resolve unknown product boundaries or conflicting facts.

The selected skills source is rooted at `{{SOURCE_ROOT}}`. The CLI has already selected fragments whose declared scope and project evidence make them eligible:

```text
{{FRAGMENT_MANIFEST}}
```

Read every selected fragment from that source. Preserve verified, still-applicable local facts and user-authored constraints from the existing file. Add or update only material supported by inspected evidence, user answers, or selected fragments. Remove stale or conflicting material only when an inspected canonical source establishes the replacement. If two rules conflict without a canonical decision, stop and ask the user rather than guessing.

Keep work bounded to human-approved tasks. Require a pause before external, irreversible, security-sensitive, public-facing, or scope-expanding work. AGENTS.md is guidance, not authority or enforcement.

For verification, state exact commands only when a local source confirms them. Otherwise point to a canonical procedure or explicitly record the unknown/escalation path. Require reporting of changed scope, checks actually run and their outcomes, plus checks not run and why.

Begin with this provenance comment, replacing the target value as shown:

```md
<!-- skills-agents-output: {"version":1,"runtime":"opencode-local","target":"{{TARGET_RELATIVE_DIR}}"} -->
```

Before finishing, confirm the staged file is non-empty, project-specific, self-contained for its local scope, and free of duplicated or contradictory instructions.
