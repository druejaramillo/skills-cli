# Skill Creation Session

Help the user create the Agent Skill named `{{SKILL_NAME}}` in `{{PROJECT_PATH}}/.agents/skills/{{SKILL_NAME}}`.

First interview the user. Ask only the questions needed to understand the real task, when the skill should trigger, usual inputs, expected outputs or side effects, and important constraints or gotchas. Do not draft generic instructions before you have those facts.

Then create one focused skill directory. `SKILL.md` must begin with YAML frontmatter whose `name` is exactly `{{SKILL_NAME}}` and whose non-empty `description` says what the skill does and when to use it. Write a concise, actionable procedure with useful validation and only add scripts, references, or assets when they materially improve execution.

Before finishing, verify that the directory name matches the frontmatter name, frontmatter is valid, references exist, and the instructions are specific to what the user described. Do not create files outside this skill directory unless the user explicitly asks.
