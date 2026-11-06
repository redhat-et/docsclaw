## Context

OpenClaw uses a set of Markdown files in the agent workspace to define
identity, persona, user context, and operating rules. These files are
loaded at session start and injected as a "Project Context" block in
the system prompt. This convention is gaining adoption and DocsClaw
should support it so users can reuse their OpenClaw workspace
configurations with a leaner runtime.

DocsClaw currently builds its system prompt from a monolithic
`system-prompt.txt` file loaded from a config directory. The OpenClaw
workspace files serve a complementary purpose: the operator defines
the agent's job via `system-prompt.txt`, while the user/project
provides context and personality via workspace files.

A competing project (Hermes) may use a different file convention.
The design should keep DocsClaw flexible: default to the existing
`system-prompt.txt` model, with pluggable workspace context loading
on top.

## Goals / Non-Goals

**Goals:**
- Load OpenClaw workspace files and inject into system prompt
- Maintain backward compatibility with system-prompt.txt
- Match OpenClaw's truncation defaults (20K/file, 60K total)
- Keep the implementation simple and self-contained

**Non-Goals:**
- Replace system-prompt.txt with OpenClaw files
- Support MEMORY.md or the `memory/` directory (future work)
- Support BOOTSTRAP.md first-run ritual
- Support HEARTBEAT.md or BOOT.md lifecycle hooks
- Plugin/hook system for prompt assembly interception
- Support for other workspace conventions (e.g. Hermes) -- will be
  evaluated separately when their conventions stabilize

## Decisions

### Option C: overlay model (system-prompt.txt + workspace files)

The base system prompt comes from `system-prompt.txt` (operator
control). OpenClaw workspace files are appended as a "Project
Context" section. Both can coexist; neither depends on the other.

```
 system-prompt.txt             workspace/
 (operator: what the           (user/project: how to
  agent does)                   interact)
       │                              │
       ▼                              ▼
 ┌──────────────────────────────────────────┐
 │ Final System Prompt                      │
 │                                          │
 │ [system-prompt.txt content]              │
 │                                          │
 │ [workspace path injection from #92]      │
 │                                          │
 │ ## Project Context                       │
 │ ### AGENTS                               │
 │ [AGENTS.md content]                      │
 │ ### SOUL                                 │
 │ [SOUL.md content]                        │
 │ ...                                      │
 └──────────────────────────────────────────┘
```

*Alternative A (backward-compatible overlay):* Same as C but with
explicit "if OpenClaw files exist, also load them" logic. Essentially
what C does -- no practical difference.

*Alternative B (clean migration):* If OpenClaw files exist, ignore
system-prompt.txt. Rejected because it forces an either/or choice
and makes the transition harder for existing users.

### File discovery: workspace directory only

Look for OpenClaw files in the workspace directory (the same path
used for file tools). In K8s, this is `/workspace` mounted as a
ConfigMap, PVC, or image volume.

*Alternative:* Also search the config directory. Rejected because
OpenClaw files are project/user context, not operator configuration.
Keeping them in the workspace matches OpenClaw's own model.

### Assembly function: single `loadWorkspaceContext`

One function that takes a directory path, reads the five files in
order, applies truncation, and returns the assembled string. Returns
empty string if no files found. Keeps the integration point in
`serve.go` minimal.

### Section headers: markdown H2/H3

Use `## Project Context` as the section wrapper with `### AGENTS`,
`### SOUL`, etc. for each file. This is human-readable, grep-friendly,
and the LLM will parse it naturally.

*Alternative:* XML tags like `<project-context>`. Rejected because
the rest of the system prompt is plain text/markdown.

## Risks / Trade-offs

- [Workspace dual-purpose] The workspace directory now serves as both
  a file I/O area for the agent and a source of context files. The
  agent could theoretically overwrite SOUL.md during operation.
  Mitigation: OpenClaw has the same design and it works in practice.
  Future: consider read-only mounting for context files.

- [Context window budget] Five files at up to 20K each could consume
  100K characters (reduced to 60K by the total cap). This is
  significant context budget. Mitigation: truncation limits match
  OpenClaw's tested defaults. Users who need lean context can use
  shorter files.

- [Hermes compatibility] If Hermes uses different file names or
  conventions, we may need a second loader. Mitigation: the overlay
  model is additive -- a Hermes loader would follow the same pattern
  without conflicting.

## Open Questions

- Should the file list and order be configurable via agent-config.yaml?
  For now, hardcode to match OpenClaw. Add configurability if a second
  convention (Hermes) needs different files.
