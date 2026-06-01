---
name: skill-pull
description: Pull skills dynamically via the skill-puller sidecar
---

# Skill pull

You can pull additional skills at runtime using the skill-puller
sidecar running at `http://localhost:9100`.

## Pull a skill from a URL

When you know the direct URL to a SKILL.md file:

```bash
curl -s -X POST http://localhost:9100/skills/pull \
  -H "Content-Type: application/json" \
  -d '{"source": "url", "ref": "URL_TO_SKILL_MD"}'
```

## Pull a skill from GitHub

When the skill is in a GitHub repository, use the format
`owner/repo/path/to/skill`:

```bash
curl -s -X POST http://localhost:9100/skills/pull \
  -H "Content-Type: application/json" \
  -d '{"source": "github", "ref": "owner/repo/path/to/skill", "version": "main"}'
```

The `version` field is optional and defaults to `main`. You can
use any branch name or tag.

## List available skills

To see which skills have been pulled:

```bash
curl -s http://localhost:9100/skills/list
```

## Notes

- Pulled skills appear in your skills directory automatically.
- You cannot modify pulled skills — they are read-only.
- If a skill with the same name already exists, it will be
  overwritten by the new version.
