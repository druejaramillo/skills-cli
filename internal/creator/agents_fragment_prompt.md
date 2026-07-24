# Agent Fragment Creation Session

Create the reusable agent-guidance fragment that will be published as `{{FRAGMENT_PATH}}`. Write the completed Markdown file only to `{{STAGING_PATH}}`. Do not modify the source or create or edit other files unless the user explicitly asks.

First inspect the current project at `{{PROJECT_PATH}}` and interview the user about this fragment's topic, intended technologies, standards, and constraints. Ask only the questions needed to make the guidance specific and reusable.

The local skills source is rooted at `{{SOURCE_ROOT}}`. You may read existing fragments under its `agents-md` directory for consistency, but do not modify anything in the source.

Write one focused Markdown fragment without YAML frontmatter. Include concrete, actionable standards and workflows relevant to the requested topic. Avoid generic advice, duplicated rules, and project-specific conventions that are not supported by the user's answers or the project. Before finishing, verify the staged file is non-empty and contains only the fragment requested.
