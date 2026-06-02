# Skill-puller sidecar example

This example shows how to deploy DocsClaw with a skill-puller
sidecar that lets the agent pull skills dynamically at runtime.

## How it works

The pod has two containers:

- **agent** — runs DocsClaw, mounts `/skills/dynamic` as
  **read-only**
- **skill-puller** — lightweight HTTP server, mounts the same
  `emptyDir` volume as **read-write**

The agent cannot modify pulled skills. The read-only enforcement
is at the kernel (VFS mount) level.

## Directory structure

```text
/skills/
├── static/        # ConfigMap mount (read-only)
│   └── ...
└── dynamic/       # emptyDir shared with skill-puller (read-only to agent)
    └── ...
```

## Usage

The agent uses the `skill-pull` skill (included in
`skills/skill-pull/SKILL.md`) to call the sidecar API:

```bash
# Pull a skill from a URL
curl -X POST http://localhost:9100/skills/pull \
  -d '{"source": "url", "ref": "https://example.com/my-skill/SKILL.md"}'

# Pull from GitHub
curl -X POST http://localhost:9100/skills/pull \
  -d '{"source": "github", "ref": "org/repo/skills/my-skill", "version": "v1.0.0"}'

# List pulled skills
curl http://localhost:9100/skills/list
```

## Deploy

```bash
kubectl apply -f deployment.yaml
```

## Design

See `docs/dev/skill-puller-sidecar-design.md` for the full
design document.
