# Agent configuration reference

The `agent-config.yaml` file controls tool availability, execution
limits, and optional integrations for a DocsClaw agent. It lives in
the agent's `--config-dir` alongside `system-prompt.txt` and
`agent-card.json`.

Without this file the agent runs in **single-shot mode** (phase 1) —
one LLM call, no tools. Adding it enables the **agentic loop**
(phase 2) where the LLM can call tools iteratively.

## Minimal example

```yaml
tools:
  allowed:
    - exec
    - read_file
    - write_file
  workspace: /workspace

loop:
  max_iterations: 10
```

## Full example

```yaml
skills_dir: /skills

tools:
  allowed:
    - exec
    - web_fetch
    - read_file
    - write_file
    - rag_search
    - fetch_document
  exec:
    timeout: 30
    max_output: 50000
  web_fetch:
    allowed_hosts:
      - "api.example.com"
      - "svc.cluster.local"
  workspace: /workspace
  mcp:
    - name: db
      transport: streamable_http
      url: http://db-mcp:8080/mcp
    - name: git
      transport: stdio
      command: /usr/local/bin/mcp-git
      args: ["--repo", "/workspace"]
      env:
        GIT_AUTHOR_NAME: "agent"

loop:
  max_iterations: 10

rag:
  backend: weaviate
  url: http://weaviate:8080
  collection: Docs
  text_field: content
  default_limit: 5
  max_limit: 20
```

## Structure

The file has four top-level sections: `skills_dir`, `tools`, `loop`,
and `rag`.

```text
agent-config.yaml
├── skills_dir           # where to find skill subdirectories
├── tools
│   ├── allowed          # which tools the LLM can call
│   ├── exec             # shell execution settings
│   ├── web_fetch        # HTTP fetch restrictions
│   ├── workspace        # file I/O boundary
│   └── mcp[]            # external MCP tool servers
├── loop
│   └── max_iterations   # agentic loop limit
└── rag                  # vector search integration (optional)
```

## Field reference

### `skills_dir`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `skills_dir` | `string` | `<config-dir>/skills` | Directory containing skill subdirectories. Each subdirectory must have a `SKILL.md` file. The CLI flag `--skills-dir` overrides this value. |

### `tools`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `allowed` | `[]string` | `[]` (none) | Tools the LLM may call. See [available tools](#available-tools). |
| `workspace` | `string` | `/workspace` | Root directory for `read_file` and `write_file`. The agent cannot access files outside this path. Can also be set via `--workspace` flag or `DOCSCLAW_WORKSPACE` env var (config file takes precedence). |

### `tools.exec`

Controls the `exec` tool (shell command execution).

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `timeout` | `int` | `30` | Max seconds per command. |
| `max_output` | `int` | `50000` | Truncate stdout/stderr after this many characters. |

### `tools.web_fetch`

Controls the `web_fetch` tool (HTTP GET requests).

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `allowed_hosts` | `[]string` | `[]` (all allowed) | Restrict HTTP requests to these hosts. Entries match exactly or as a suffix (e.g., `svc.cluster.local` matches itself and any subdomain like `foo.svc.cluster.local`). Empty list allows all hosts. |

### `tools.mcp[]`

Connect to external tool servers via the
[Model Context Protocol](https://modelcontextprotocol.io/).
Each entry adds one MCP server whose tools become available to the
agent. See the [MCP client guide](mcp-client-guide.md) for details.

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `name` | `string` | Yes | Unique identifier. Tools are prefixed as `<name>_<tool>`. |
| `transport` | `string` | Yes | `streamable_http` or `stdio`. |
| `url` | `string` | For `streamable_http` | MCP server endpoint URL. |
| `command` | `string` | For `stdio` | Path to the executable. |
| `args` | `[]string` | No | Command-line arguments (stdio only). |
| `env` | `map[string]string` | No | Environment variables for the subprocess (stdio only). |

### `loop`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `max_iterations` | `int` | `10` | Maximum tool-call rounds before the agent stops. Prevents runaway loops. |

### `rag`

Optional. Enables the `rag_search` tool for vector similarity search.
Currently supports Weaviate as the backend.

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `backend` | `string` | — (required) | Vector database engine. Currently `weaviate`. |
| `url` | `string` | — (required) | Vector database URL. |
| `collection` | `string` | — (required) | Collection (class) name to search. |
| `text_field` | `string` | `content` | Field name that contains the document text. |
| `default_limit` | `int` | `5` | Default number of results per query. |
| `max_limit` | `int` | `20` | Hard ceiling on results the LLM can request. |

## Available tools

These are the built-in tool names you can list in `tools.allowed`:

| Tool | Description |
| ---- | ----------- |
| `exec` | Run shell commands (optional: configure `tools.exec` to customize timeout/max_output) |
| `web_fetch` | HTTP GET requests (respects `tools.web_fetch.allowed_hosts`) |
| `read_file` | Read files from the workspace directory |
| `write_file` | Write files to the workspace directory |
| `rag_search` | Vector similarity search (requires `rag` config) |
| `fetch_document` | Fetch documents from a document service |
| `load_skill` | Load a skill's full content on demand (always available when skills exist) |

MCP tools do not need to be listed in `allowed` — they are
registered automatically when the MCP server connects.

## Phase 1 vs phase 2

| | Phase 1 (no config) | Phase 2 (with config) |
| - | ------------------- | --------------------- |
| **Mode** | Single-shot | Agentic loop |
| **Tools** | None | As configured |
| **LLM calls** | One | Up to `max_iterations` |
| **Use case** | Simple Q&A, summarization | API calls, file operations, multi-step tasks |

To run in phase 1, simply omit `agent-config.yaml` from the config
directory.

## Workspace context

DocsClaw supports [OpenClaw](https://openclaw.dev)-compatible
workspace files. When present in the workspace directory, these
files are loaded at startup and appended to the system prompt as
structured project context. See the
[OpenClaw agent workspace docs](https://docs.openclaw.ai/concepts/agent-workspace)
for the full specification of these files.

This creates an overlay model: the operator controls the base
behavior via `system-prompt.txt`, while users provide project-level
personality and context through workspace files.

### Supported files

Files are loaded in this order (all optional):

| File | Purpose |
| ---- | ------- |
| `AGENTS.md` | Operating instructions |
| `SOUL.md` | Persona and tone |
| `USER.md` | User context |
| `IDENTITY.md` | Agent name and role |
| `TOOLS.md` | Tool guidance (advisory) |

Missing files are skipped silently. Empty files are ignored.

### Truncation limits

| Limit | Default | Description |
| ----- | ------- | ----------- |
| Per file | 20,000 chars | Files exceeding this are truncated with a warning log |
| Total | 60,000 chars | Combined content across all files is capped; files load in order and the last file crossing the boundary is truncated |

### Prompt assembly order

The final system prompt is assembled in this order:

1. `system-prompt.txt` content (from config directory)
2. Workspace path injection (`"Your workspace directory is ..."`)
3. **Workspace context** (`## Project Context` with `### <FILE>`
   headers per loaded file)
4. OS tool inventory (if `/etc/docsclaw/tools.json` exists)
5. Skills summary (if skills are discovered)

### Example output

When SOUL.md and USER.md are present, the injected section looks
like:

```text
## Project Context

### SOUL
Be direct and concise. Avoid unnecessary pleasantries.

### USER
Pavel, OCTO team at Red Hat.
```

### Deployment

Store workspace files in a ConfigMap and use an init container to
seed them into a writable emptyDir (so `write_file` still works):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: docsclaw-workspace
data:
  SOUL.md: |
    Be direct and concise.
  USER.md: |
    Pavel, OCTO team at Red Hat.
---
# In the Deployment spec:
initContainers:
  - name: seed-workspace
    image: ghcr.io/redhat-et/docsclaw:latest
    command: ["sh", "-c", "cp /openclaw-files/*.md /workspace/"]
    volumeMounts:
      - name: openclaw-files
        mountPath: /openclaw-files
        readOnly: true
      - name: workspace
        mountPath: /workspace
# Main container:
volumeMounts:
  - name: workspace
    mountPath: /workspace
volumes:
  - name: openclaw-files
    configMap:
      name: docsclaw-workspace
  - name: workspace
    emptyDir: {}
```

See [`deploy/openclaw-workspace-agent.yaml`](../deploy/openclaw-workspace-agent.yaml)
for a complete example.

### Phase compatibility

Workspace context loads regardless of whether `agent-config.yaml`
exists. It works in both phase 1 (single-shot) and phase 2
(agentic loop). The workspace directory is resolved with this
priority:

1. `agent-config.yaml` `tools.workspace` (config file wins)
2. `--workspace` flag or `DOCSCLAW_WORKSPACE` env var
3. Default: `/workspace`
