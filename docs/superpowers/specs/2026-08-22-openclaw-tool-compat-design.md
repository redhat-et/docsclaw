# OpenClaw Tool Compatibility

**Date**: 2026-08-22
**Status**: In Progress

## Context

DocsClaw's Phase 2 agentic loop uses a local `tools.Registry`. Several
OpenClaw-style clients expect tool names such as `read`, `write`, and
`terminal`, and rely on tools such as `search_files`, `web_search`,
`apply_patch`, and `edit` that either have different names in DocsClaw or do
not exist yet. This design adds the missing aliases and tools while keeping
DocsClaw's workspace-scoped security model.

## Goal

Make DocsClaw's Phase 2 tool set compatible with common OpenClaw agent tool
names, with clear runtime errors when a required external binary is missing and
without breaking existing DocsClaw tools.

## Alias resolution

This PR adds alias support to `tools.Registry`. `RegisterAlias(alias,
canonical)` maps an alias to a canonical tool name; `Get(alias)` resolves to
the canonical tool, while `Definitions()` only exposes canonical names. The
following aliases are registered when Phase 2 mode is enabled:

| Alias      | Canonical tool |
| ---------- | -------------- |
| `read`     | `read_file`    |
| `write`    | `write_file`   |
| `terminal` | `exec`         |

The allowed-tools filter is applied to the canonical name, so an alias is
usable only when its canonical target is allowed. Registering a tool whose
name collides with an existing alias (or vice versa) returns an error.

### Breaking API change

`Registry.Register` and `Registry.RegisterAlwaysAllowed` now return `error` so
that collisions between tools and aliases can be surfaced to callers. Existing
call sites in `internal/cmd/serve.go` and tests have been updated to check the
returned error.

## New tools

| Tool name      | Purpose                                          | Package                    |
| -------------- | ------------------------------------------------ | -------------------------- |
| `search_files` | Search file contents with ripgrep                | `internal/searchfiles`     |
| `web_search`   | Search the web via DuckDuckGo                    | `internal/websearch`       |
| `apply_patch`  | Apply a unified-diff style patch to a file       | `internal/applypatch`      |
| `edit`         | Replace a single string occurrence inside a file | `internal/edit`            |

## Per-tool details

### `search_files`

- **Parameters**:
  - `path` (string, required): directory to search, resolved against the workspace.
  - `regex` (string, required): regular expression to match.
  - `file_pattern` (string, optional): glob filter such as `*.go`.
- **Runtime dependency**: shells out to `rg` (ripgrep). On execution it calls
  `exec.LookPath("rg")`; if ripgrep is missing it returns:
  ```
  ripgrep (rg) is not installed; search_files requires it
  ```
- **Container images**: `ripgrep` is installed in `Containerfile`, `Dockerfile`,
  `Dockerfile.release`, and `containers/Containerfile.security`. Image builds
  were updated locally but were not exercised in CI for this change.
- **Behavior**: runs `rg --no-config --json -e <regex> -- <path>` (with optional
  `-g <file_pattern>`), parses JSON match events, and returns lines as
  `file:line:match`. Output is truncated at 50,000 characters. Returns
  "No matches found." when ripgrep exits with code 1 and no matches.
- **Safety features**:
  - The `regex` argument is pre-validated with `regexp.Compile` before invoking
    ripgrep; a malformed pattern returns a clear error instead of a subprocess
    failure.
  - Execution is bounded by a 30-second context timeout.
  - The stdout scanner uses a 4 MiB maximum token size so a single oversized
    line cannot exhaust the parser.

### `web_search`

- **Provider interface** (`internal/websearch`):
  ```go
  type Provider interface {
      Search(ctx context.Context, query string, numResults int) ([]Result, error)
  }
  type Result struct {
      Title   string
      Snippet string
      URL     string
  }
  ```
- **DuckDuckGo provider** (`internal/websearch/duckduckgo.go`):
  - Posts `q=<query>` to `https://html.duckduckgo.com/html/`.
  - Sets `User-Agent: DocsClaw web_search tool (...)`.
  - Parses `result__a` title links and `result__snippet` snippets.
  - Resolves DuckDuckGo redirect URLs (`/l/?uddg=...`) to the target URL.
- **Parameters**:
  - `query` (string, required): search query.
  - `num_results` (integer, optional): defaults to 5, clamped to the range 1-10.
- **Output**: markdown list `* [Title](URL): Snippet`.

### `apply_patch`

- **Parameters**:
  - `path` (string, required): file to patch, resolved against the workspace.
  - `patch` (string, required): unified-diff style patch string.
- **Behavior**:
  - Rejects paths outside the workspace.
  - The target file must exist.
  - Applies hunks sequentially; each hunk's context lines must match exactly.
  - On mismatch, returns an error identifying the hunk/line.
  - Handles the `\ No newline at end of file` marker best-effort: the final
    output omits a trailing newline when the marker appears on the last hunk.
    If the marker refers only to the old side of the last hunk, the output may
    incorrectly lose its trailing newline.
  - On success, returns `Patched <path>`.

### `edit`

- **Parameters**:
  - `path` (string, required): file to edit, resolved against the workspace.
  - `old_string` (string, required): exact text to replace.
  - `new_string` (string, required): replacement text.
- **Behavior**:
  - Rejects paths outside the workspace.
  - The target file must exist.
  - Replaces the first exact occurrence of `old_string` with `new_string`.
  - Returns an error if `old_string` is empty or not found.
  - When `old_string` occurs more than once, the first occurrence is replaced
    and the success message includes a warning with the total occurrence count.
  - On success, returns `Edited <path>`.

## Registration in `serve.go`

When Phase 2 mode is enabled (`toolRegistry != nil` block in
`internal/cmd/serve.go`):

1. Register aliases:
   - `read` -> `read_file`
   - `write` -> `write_file`
   - `terminal` -> `exec`
2. Register canonical tools already present (`read_file`, `write_file`, `exec`,
   `search_files`).
3. Register new tools: `web_search`, `apply_patch`, `edit`.

### Registration error policy

- Alias registration errors log a warning and continue. A duplicate or
  conflicting alias does not prevent the agent from starting because the
  canonical tools are still available.
- Tool registration errors abort startup. A missing canonical tool would leave
  the agent unable to perform basic operations, so startup fails fast with a
  clear error.

## Implementation details

- `internal/fileutil.WriteFileAtomically` is a small helper used by
  `apply_patch` and `edit` to write changes through a temp file and rename,
  reducing the chance of leaving a file partially written on error or crash.

## Testing

- Unit tests in each tool package (`internal/searchfiles`,
  `internal/websearch`, `internal/applypatch`, `internal/edit`).
- `pkg/tools/registry_test.go` covers alias registration, allowed-tool
  filtering, and the new collision behavior between tools and aliases.
- Run `go test ./...`, `make test`, and `make lint` before merging.
- Container image Dockerfiles/Containerfiles were updated to install
  `ripgrep`, but image builds were not verified in CI for this change.

## Risks and mitigations

| Risk                                              | Mitigation                                              |
| ------------------------------------------------- | ------------------------------------------------------- |
| DuckDuckGo HTML layout changes and breaks parsing | Keep parsing regexes localized; provider interface allows swapping backends later. |
| `rg` missing in a custom image                    | Runtime `LookPath` check returns a clear error; official images install `ripgrep`. |
| Workspace escape via aliases or new tools         | All file tools use `workspace.IsInsideWorkspace` before reading/writing. |
| Patch application corrupts file on partial match  | Validate context lines before writing; abort on first mismatch. |
| `edit` replaces wrong occurrence                  | Require exact `old_string`; only the first match is replaced. |

## Out of scope

- Additional search providers for `web_search` (Google, Bing, etc.).
- Semantic/file-content search beyond ripgrep regex.
- Interactive or multi-file patch editors.
- Git-specific tools (commit, push, diff generation).
- Network-mounted or remote filesystem support.
