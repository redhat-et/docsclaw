## Context

The workspace directory serves three purposes in DocsClaw:

1. Root for `read_file` and `write_file` tools (phase 2 only)
2. Source of OpenClaw workspace context files (both phases)
3. Injected into the system prompt so the LLM knows where to write

Currently the path is resolved in two separate places in
`serve.go`: once inside the `if agentCfg != nil` block (for tools)
and once outside it (for OpenClaw context). Both fall back to
`defaultWorkspace` ("/workspace") and check `agentCfg.Tools.Workspace`.

The `serve` command already uses Viper for config binding with
env var prefix `DOCSCLAW_`. Adding a flag with Viper binding gives
env var support for free.

## Goals / Non-Goals

**Goals:**

- Add `--workspace` flag and `DOCSCLAW_WORKSPACE` env var
- Clear, documented priority order for workspace resolution
- Single resolution point replacing the two current ad-hoc blocks

**Non-Goals:**

- Per-tool workspace overrides (e.g. different paths for read vs
  write)
- Workspace validation beyond `os.MkdirAll` (e.g. checking
  permissions, disk space)
- Runtime workspace changes (path is fixed at startup)

## Decisions

### Priority order: config file > flag/env > default

```
agent-config.yaml tools.workspace  →  highest (explicit config)
--workspace / DOCSCLAW_WORKSPACE   →  middle  (operator override)
defaultWorkspace constant          →  lowest  (fallback)
```

This matches the existing Cobra/Viper pattern in the codebase
where config file values take precedence over flags. The
`agent-config.yaml` workspace is an explicit choice by whoever
wrote the config; the flag is a deployment-time convenience.

*Alternative: flag overrides config file.* Rejected because
`agent-config.yaml` is the authoritative tool configuration — the
workspace path must match what `read_file`/`write_file` expect.
If the flag could silently override it, the tools and the system
prompt would agree on a path but the config file would say
something different.

### Unified resolution function

Extract workspace resolution into a helper:

```go
func resolveWorkspace(cfgWorkspace, flagWorkspace string) string
```

This replaces the two inline resolution blocks and makes the
priority logic testable. Both the tool registration block and the
OpenClaw context block call this function.

*Alternative: keep inline resolution.* Rejected because the logic
is now in two places and a third source (flag) would make three.

### Add to Config struct, not standalone flag

Add `Workspace` field to the `Config` struct with Viper binding.
This gives `DOCSCLAW_WORKSPACE` env var support automatically
(Viper env prefix is already `DOCSCLAW`).

## Risks / Trade-offs

- [Minimal change] This is a small, focused change. The risk of
  over-engineering is higher than the risk of bugs. Keep the
  implementation to the flag, resolution function, and tests.

## Open Questions

(none — the issue acceptance criteria are clear)
