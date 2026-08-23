## Why

The workspace directory defaults to `/workspace` and can only be
overridden via `agent-config.yaml`. Operators deploying on
Kubernetes need to edit the ConfigMap to change the path, which is
impractical when the ConfigMap is managed by a different team or
shared across agents. A CLI flag and env var would let operators
set the workspace at the pod/container level without touching the
ConfigMap.

## What Changes

- Add `--workspace` flag to the `serve` command
- Add `DOCSCLAW_WORKSPACE` env var fallback (via Viper binding)
- Unify workspace resolution into a single function with clear
  priority: `agent-config.yaml` > CLI flag/env var > default
- The workspace path is currently resolved in two places
  (`serve.go:349` for tools, `serve.go:388` for OpenClaw context);
  both will use the unified resolution

## Capabilities

### New Capabilities

- `workspace-cli-config`: CLI flag and env var override for the
  agent workspace directory path

### Modified Capabilities

(none)

## Impact

- `internal/cmd/serve.go` — new `--workspace` flag, Config struct
  field, unified workspace resolution replacing two ad-hoc blocks
- Deployment manifests — add commented-out example of the env var
- No breaking changes — existing behavior unchanged when flag is
  not set
