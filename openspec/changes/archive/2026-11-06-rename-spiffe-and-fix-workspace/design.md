## Context

DocsClaw originated as a SPIFFE Zero Trust demo and still carries
naming artifacts from that era. The env var prefix `SPIFFE_DEMO_*`,
config path `/etc/spiffe-demo/`, Prometheus namespace `spiffe_demo`,
and logger env var `SPIFFE_DEMO_LOG_FORMAT` all reference the old
project identity. The project has no external consumers of these
interfaces, making this a clean rename.

Separately, the default workspace `/tmp/agent-workspace` breaks on
containers with `readOnlyRootFilesystem: true`, and the LLM is never
told where the workspace is, causing it to guess `/tmp`.

## Goals / Non-Goals

**Goals:**
- Remove all `spiffe`/`SPIFFE` references from runtime code
- Change default workspace to a container-friendly path
- Make the LLM aware of the workspace directory

**Non-Goals:**
- Backward compatibility with `SPIFFE_DEMO_*` env vars (no transition
  period -- project has no production deployments)
- Making workspace path configurable via CLI flag (separate issue)

## Decisions

### Rename strategy: clean break, no dual-prefix support

Rename all references in one pass. No fallback to the old prefix.

*Alternative considered:* Accept both prefixes during transition.
Rejected because there are no known external consumers and the
complexity is not worth it.

### Default workspace: `/workspace`

Use `/workspace` as the default when `agent-config.yaml` does not
specify a workspace path. In K8s deployments, this is mounted as
an emptyDir or PVC.

*Alternative considered:* `./workspace` (relative). Rejected because
container workdirs vary and absolute paths are more predictable in K8s.

### Prompt injection: append to system prompt

Add a single line after the system prompt content:
`"Your workspace directory is {path}. Always write files there."`

*Alternative considered:* Add workspace info to individual tool
descriptions. Rejected as redundant -- system prompt guidance is
sufficient and easier to maintain.

## Risks / Trade-offs

- [Prometheus metric rename] Anyone with existing dashboards referencing
  `spiffe_demo_*` metrics must update them. No known consumers exist.
  Mitigation: document in release notes.
- [Env var rename] Existing deployment manifests using `SPIFFE_DEMO_*`
  will silently stop working. Mitigation: update all manifests in this
  change; document in release notes.

## Migration Plan

1. Apply all renames in a single commit
2. Update deployment manifests in the same commit
3. Grep project-wide for remaining `spiffe` references
4. Tag as v0.14.0 with release notes documenting breaking changes
