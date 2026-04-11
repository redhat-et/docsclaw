# OpenCode vs DocsClaw Resource Comparison

## Context

Evaluated whether deploying an existing open-source agent (OpenCode)
on OpenShift is a viable alternative to building DocsClaw. OpenCode
is a TypeScript-based coding agent running on the Bun runtime.

**Cluster:** OpenShift (namespace `panni-opencode`)
**Date:** 2026-04-11

## Deployment observations

OpenCode required workarounds for OpenShift's non-root security model:

- Container writes to `/.local`, `/.cache`, `/.config` at startup,
  causing `EACCES` errors under non-root. Fixed with `emptyDir`
  volume mounts.
- OOMKilled at 512Mi memory limit; required raising to 2Gi.

## Resource comparison (idle/steady-state)

| Agent | Runtime | Memory (each) | Memory limit |
|-------|---------|---------------|--------------|
| OpenCode | Bun/Node (TypeScript) | 487Mi | 2Gi |
| docsclaw | Go | 9Mi | 256Mi |
| docsclaw-skills | Go | 12Mi | 256Mi |

Both agents show negligible CPU at idle (1m each) since LLM inference
happens server-side.

## Key takeaway

OpenCode uses **~23x more memory** than both DocsClaw instances
combined (487Mi vs 21Mi). For deploying multiple specialized agents
for business users on OpenShift, the Go-based approach is significantly
more efficient. A cluster running 50 specialized DocsClaw agents would
use roughly 500Mi total — about the same as a single OpenCode instance.

## Implications for DocsClaw

| Factor | OpenCode | DocsClaw |
|--------|----------|----------|
| Memory per agent | ~500Mi | ~10Mi |
| Multi-agent deployment | Expensive | Practical |
| Target audience | Developers | Business users (configurable) |
| Specialization model | Single general agent | Many lightweight specialized agents |
| OpenShift compatibility | Requires workarounds | Runs natively as non-root |

The resource efficiency of Go makes it feasible to run a fleet of
specialized agents (finance, legal, HR, etc.) on the same cluster
where a single OpenCode instance would consume equivalent resources.
This directly supports the skills-based architecture: one lightweight
runtime, many domain-specific configurations.

## Source

Full evaluation report: external experiment at
`~/work/experiments/opencode/REPORT.md`
