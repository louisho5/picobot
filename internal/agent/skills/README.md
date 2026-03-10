# Skills System

The skills system allows you to extend picobot with custom knowledge, workflows, and domain expertise.

## Overview

Skills are modular packages that provide:
- **Specialized workflows**: Multi-step procedures for specific tasks
- **Domain knowledge**: Company-specific info, schemas, business logic
- **Tool integrations**: Instructions for working with specific APIs or formats
- **Bundled resources**: Scripts, configs, and reference materials

## Structure

Each skill is a directory in `skills/` containing:

```
skills/
  └── skill-name/
      ├── skill.md        # Required: Main documentation with frontmatter
      └── [other files]   # Optional: Scripts, configs, references
```

## skill.md Format

Every skill must have a `skill.md` file with YAML frontmatter:

```markdown
---
name: skill-name
description: Brief description of what this skill does
---

# Skill Name

## Purpose

What this skill helps you accomplish.

## Usage

Instructions, examples, and procedures.

## Examples

\`\`\`bash
# Command examples
curl example.com/api
\`\`\`

## Tips

- Best practices
- Common pitfalls
- References
```

## Management Tools

Picobot provides built-in tools for managing skills:

### `create_skill`
Create a new skill in the `skills` directory.

**Arguments:**
```json
{
  "name": "skill-name",
  "description": "Brief description",
  "content": "# Skill Content\n\nYour markdown content here"
}
```

**Example usage:**
```
Agent: I'll create a skill for weather checking.
[calls create_skill with appropriate args]
```

### `list_skills`
List all available skills.

**Arguments:** None

**Returns:** JSON array of skills with names and descriptions.

### `read_skill`
Read the content of a specific skill.

**Arguments:**
```json
{
  "name": "skill-name"
}
```

**Returns:** Full skill content including frontmatter.

### `delete_skill`
Delete a skill from the `skills` directory.

**Arguments:**
```json
{
  "name": "skill-name"
}
```

## How Skills Work

1. **Loading**: When the agent starts processing a message, all skills are loaded from `skills/`
2. **Context**: Skill content is included in the agent's context automatically
3. **Access**: The agent can reference skills when responding to relevant queries
4. **Management**: The agent can create/modify/delete skills using the skill tools

## Creating Effective Skills

### Keep It Concise
- The agent is already smart—only add what it doesn't know
- Use examples over explanations
- Challenge each paragraph: "Does this justify its token cost?"

### Match Specificity to Need

**High freedom (instructions)**:
- Multiple valid approaches
- Context-dependent decisions
- Heuristic guidance

**Medium freedom (pseudocode/templates)**:
- Preferred patterns exist
- Some variation acceptable
- Configuration parameters

**Low freedom (exact scripts)**:
- Fragile operations
- Consistency critical
- Specific sequence required

### Structure Example

```markdown
---
name: api-integration
description: How to integrate with our internal API
---

# API Integration

## Authentication

Use Bearer token from environment:
\`\`\`bash
export API_KEY="your-key-here"
curl -H "Authorization: Bearer $API_KEY" https://api.example.com
\`\`\`

## Common Endpoints

- GET /users - List users
- POST /users - Create user (requires: name, email)
- GET /users/{id} - Get user details

## Error Handling

- 401: Check API_KEY is set
- 429: Rate limited, wait 60s
- 500: Check status page at status.example.com
```

## Example Skills

After onboarding, check `skills/example/skill.md` for a demonstration of the format.

## Best Practices

1. **One skill per domain**: Keep skills focused on specific areas
2. **Include examples**: Show concrete usage, not just theory
3. **Reference tools**: Mention which picobot tools are relevant
4. **Update regularly**: Keep skills current as processes change
5. **Test instructions**: Verify commands/procedures actually work

## Integration with Memory

Skills complement the memory system:
- **Skills**: Static knowledge that rarely changes (procedures, APIs)
- **Memory**: Dynamic context that evolves (project status, decisions)

Use `write_memory` for temporary/evolving information, skills for permanent knowledge.

## CLI Management (Manual)

You can also manage skills manually:

```bash
# List skills
ls ~/.picobot/workspace/skills/

# Create skill
mkdir -p ~/.picobot/workspace/skills/my-skill
cat > ~/.picobot/workspace/skills/my-skill/skill.md <<EOF
---
name: my-skill
description: My custom skill
---

# My Skill

Content here...
EOF

# Delete skill
rm -rf ~/.picobot/workspace/skills/my-skill
```

## Troubleshooting

**Skills not loading?**
- Check `skills/` exists in workspace
- Verify `skill.md` has valid frontmatter with `name` field
- Check file permissions (should be readable)

**Skill content too long?**
- Break into multiple focused skills
- Remove redundant explanations
- Use links for detailed references

**Agent not using skill knowledge?**
- Ensure skill name/description are clear and relevant
- Add keywords that match user queries
- Consider splitting broad skills into specific ones
