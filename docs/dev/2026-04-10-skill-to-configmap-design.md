# Skill-to-ConfigMap conversion tooling

**Issue:** #6
**Date:** 2026-04-10
**Status:** Approved

## Goal

Provide Makefile targets that convert a local agent config directory
into Kubernetes ConfigMap YAML files and optionally apply them to a
cluster. The source directory is the single source of truth.

## Makefile targets

| Target | Purpose |
|--------|---------|
| `configmap-gen` | Generate ConfigMap YAML files from a config directory |
| `configmap-apply` | Generate and apply to cluster (idempotent) |

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_DIR` | (required) | Path to agent config directory |
| `NAME` | basename of `CONFIG_DIR` | Prefix for ConfigMap names |
| `OUTDIR` | `deploy/generated/$(NAME)/` | Output directory for YAML files |
| `NAMESPACE` | current context namespace | Target K8s namespace |
| `KUBECTL` | `oc` | CLI tool (`oc` or `kubectl`) |

## Generated ConfigMaps

Two ConfigMaps are generated per agent config directory:

### `<name>-config`

Contains all non-skill files from the config directory root:

- `system-prompt.txt` (required)
- `agent-card.json` (if present)
- `agent-config.yaml` (if present)
- `prompts.json` (if present)

### `<name>-skills`

Contains all skill files, keyed as `<skill-name>/SKILL.md`. Only
generated if the `skills/` subdirectory exists.

## Generated output structure

```
deploy/generated/<name>/
├── config.yaml     # ConfigMap: <name>-config
└── skills.yaml     # ConfigMap: <name>-skills
```

## Apply mechanism

The `configmap-apply` target uses the `create --dry-run=client`
pattern for idempotent updates:

```bash
kubectl create configmap <name>-config \
  --from-file=system-prompt.txt=<path> \
  --from-file=agent-config.yaml=<path> \
  ... \
  --dry-run=client -o yaml | kubectl apply -f -
```

This always regenerates from the source directory. Adding a new
skill means adding the `SKILL.md` file and re-running
`make configmap-apply`.

## .gitignore

Add `deploy/generated/` to `.gitignore`. These are derived
artifacts. Users who want to version them can override `OUTDIR`.

## Usage examples

Generate YAML files with defaults (name derived from directory):

```bash
make configmap-gen CONFIG_DIR=testdata/standalone
```

Generate with custom name and output:

```bash
make configmap-gen CONFIG_DIR=testdata/standalone NAME=my-agent OUTDIR=deploy/my-agent/
```

Apply directly to cluster:

```bash
make configmap-apply CONFIG_DIR=testdata/standalone NAMESPACE=agents
```

Use kubectl instead of oc:

```bash
make configmap-apply CONFIG_DIR=testdata/standalone KUBECTL=kubectl
```
