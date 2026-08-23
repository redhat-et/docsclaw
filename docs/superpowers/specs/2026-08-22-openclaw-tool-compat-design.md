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

The `tools.Registry` already supports aliases: `Get(alias)` resolves to the
canonical tool, while `Definitions()` only exposes canonical names. The
following aliases are registered when Phase 2 mode is enabled:

| Alias     | Canonical tool |
| --------- | -------------- |
| `read`    | `read_file`    |
| `write`   | `write_file`   |
| `terminal`| `exec`         |

The allowed-tools filter is applied to the canonical name, so an alias is
usable only when its canonical target is allowed.

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
  `Dockerfile.release`, and `containers/Containerfile.security`.
- **Behavior**: runs `rg --no-config --json -e <regex> -- <path>` (with optional
  `-g <file_pattern>`), parses JSON match events, and returns lines as
  `file:line:match`. Output is truncated at 50,000 characters. Returns
  "No matches found." when ripgrep exits with code 1 and no matches.

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

Registration of the new tools is intentionally batched in Task 7; `web_search`
is not registered yet.

## Testing

- Unit tests in each tool package (`internal/searchfiles`,
  `internal/websearch`, `internal/applypatch`, `internal/edit`).
- Existing `pkg/tools/registry_test.go` already covers alias registration and
  allowed-tool filtering.
- Run `go test ./...`, `make test`, and `make lint` before merging.
- Verify container images build and contain `rg`.

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
