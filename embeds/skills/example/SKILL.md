---
name: example
description: Example skill demonstrating the SKILL.md format
---

# Example Skill

This is an example skill that demonstrates the format for creating custom skills.

## Purpose

Skills extend the agent's capabilities by providing specialized knowledge, workflows, and instructions for specific tasks or domains.

## Structure

Each skill is a directory in `skills/` containing:

- `SKILL.md` (required): Main documentation with frontmatter metadata
- Additional files: Scripts, configs, or reference materials (optional)

## Frontmatter

The SKILL.md file must start with YAML frontmatter:

```yaml
---
name: skill-name
description: Brief description of what the skill does
---
```

## Usage

The agent automatically loads all skills from `skills/` and includes their content in the context. You can:

- Create new skills with the `create_skill` tool
- List available skills with the `list_skills` tool
- Read skill content with the `read_skill` tool
- Delete skills with the `delete_skill` tool

## Tips

- Keep skills concise and focused
- Use concrete examples over lengthy explanations
- Include command templates when applicable
- One skill per domain