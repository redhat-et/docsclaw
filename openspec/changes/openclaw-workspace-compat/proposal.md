## Why

OpenClaw's workspace file convention (SOUL.md, USER.md, AGENTS.md,
IDENTITY.md, TOOLS.md) is becoming a de facto standard for agent
configuration. Users who already have OpenClaw agents configured should
be able to reuse those same workspace files with DocsClaw -- a much
leaner runtime. This gives people a migration path: save your OpenClaw
workspace to GitHub, mount it as a ConfigMap, and DocsClaw consumes it.

DocsClaw currently uses a monolithic `system-prompt.txt` for persona
configuration. Adding OpenClaw file support does not replace this --
it layers workspace context on top, so operators keep control of the
base system prompt while users provide project-level personality and
context through familiar OpenClaw files.

## What Changes

- Load OpenClaw workspace files from the workspace directory at
  session start
- Assemble loaded files into a "Project Context" section appended
  to the system prompt (after `system-prompt.txt` content)
- Support the five core bootstrap files in OpenClaw's documented
  order: AGENTS.md, SOUL.md, USER.md, IDENTITY.md, TOOLS.md
- Skip missing files silently (all files are optional)
- Apply truncation limits matching OpenClaw defaults: 20,000 chars
  per file, 60,000 chars total
- Log which workspace files were loaded and total character count

## Capabilities

### New Capabilities

- `workspace-context-loading`: Discover and load OpenClaw-compatible
  workspace files (AGENTS.md, SOUL.md, USER.md, IDENTITY.md, TOOLS.md)
  from the workspace directory and inject them as structured context
  into the system prompt

### Modified Capabilities

(none)

## Impact

- `internal/cmd/serve.go` -- new `loadWorkspaceContext()` function,
  integration into system prompt assembly after `loadSystemPrompt()`
- Workspace directory (`/workspace` or configured path) -- now serves
  dual purpose: file I/O for the agent AND source of context files
- Deployment manifests may need updated ConfigMap examples showing
  how to mount OpenClaw workspace files
- No breaking changes -- existing deployments with only
  `system-prompt.txt` continue to work unchanged
