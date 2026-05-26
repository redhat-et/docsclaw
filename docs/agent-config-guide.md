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
  workspace: /tmp/agent-workspace

loop:
  max_iterations: 10
```

## Full example

```yaml
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
      - ".svc.cluster.local"
  workspace: /tmp/agent-workspace
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

The file has three top-level sections: `tools`, `loop`, and `rag`.

```text
agent-config.yaml
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

### `tools`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `allowed` | `[]string` | `[]` (none) | Tools the LLM may call. See [available tools](#available-tools). |
| `workspace` | `string` | `/tmp/agent-workspace` | Root directory for `read_file` and `write_file`. The agent cannot access files outside this path. |

### `tools.exec`

Controls the `exec` tool (shell command execution).

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `timeout` | `int` | `0` (no limit) | Max seconds per command. |
| `max_output` | `int` | `0` (no limit) | Truncate stdout/stderr after this many characters. |

### `tools.web_fetch`

Controls the `web_fetch` tool (HTTP GET requests).

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `allowed_hosts` | `[]string` | `[]` (all allowed) | Restrict HTTP requests to these hosts. Supports prefix matching with a leading dot (e.g., `.svc.cluster.local` matches any subdomain). Empty list allows all hosts. |

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
| `exec` | Run shell commands (requires `tools.exec` config) |
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
