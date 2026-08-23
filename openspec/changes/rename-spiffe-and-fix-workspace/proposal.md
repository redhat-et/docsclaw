## Why

DocsClaw still carries naming artifacts from its origin as a SPIFFE
Zero Trust demo. The env var prefix `SPIFFE_DEMO_*`, config path
`/etc/spiffe-demo/`, and Prometheus namespace `spiffe_demo` confuse
new users and conflict with the project's current identity. Separately,
the default workspace path `/tmp/agent-workspace` fails on containers
with `readOnlyRootFilesystem: true` and the LLM is never told where
the workspace is, so it guesses `/tmp`. Both are low-effort fixes that
should ship together.

## What Changes

- **BREAKING**: Rename env var prefix from `SPIFFE_DEMO` to `DOCSCLAW`
  (e.g. `SPIFFE_DEMO_OTEL_ENABLED` becomes `DOCSCLAW_OTEL_ENABLED`)
- **BREAKING**: Rename Prometheus metrics namespace from `spiffe_demo`
  to `docsclaw` (all 7 metrics)
- **BREAKING**: Rename config path from `/etc/spiffe-demo/` to
  `/etc/docsclaw/` (aligns with existing `/etc/docsclaw/tools.json`)
- Rename legacy logger env var from `SPIFFE_DEMO_LOG_FORMAT` to
  `DOCSCLAW_LOG_FORMAT`
- Change default workspace from `/tmp/agent-workspace` to `/workspace`
- Inject workspace path into system prompt so the LLM writes files
  to the correct location
- Update deployment manifests to use new env var names and mount
  an emptyDir at `/workspace`

## Capabilities

### New Capabilities

- `workspace-prompt-injection`: Inject the workspace directory path
  into the system prompt so the LLM knows where to read/write files

### Modified Capabilities

(none -- the env prefix and metrics namespace are configuration, not
spec-level behavior)

## Impact

- `internal/config/config.go` -- env prefix, config path
- `internal/logger/logger.go` -- legacy env var alias, comment
- `internal/metrics/metrics.go` -- Prometheus namespace (7 metrics)
- `internal/cmd/serve.go` -- default workspace, prompt injection
- `deploy/agent-with-skills.yaml` -- env var names, workspace volume
- `deploy/standalone-agent.yaml` -- env var names, workspace volume
- Anyone using `SPIFFE_DEMO_*` env vars must switch to `DOCSCLAW_*`
- Prometheus dashboards referencing `spiffe_demo_*` metrics must update
  (no known external consumers)
