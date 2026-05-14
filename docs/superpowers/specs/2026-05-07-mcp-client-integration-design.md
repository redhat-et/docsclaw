# MCP Client Integration Design

**Issue:** #2 — MCP client integration for external tool servers
**Date:** 2026-05-07

## Overview

Add MCP client capabilities to DocsClaw so agents can connect to
external MCP tool servers. MCP tools integrate transparently into
the existing tool registry — the agentic loop and LLM providers
are unaware of the difference between built-in and MCP-sourced tools.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Library | `github.com/modelcontextprotocol/go-sdk` | Official SDK, v1.x semver stability, robust streamable HTTP transport (reconnection, backoff, stream resumption), middleware support |
| Transports | Both stdio and streamable HTTP | Stdio for co-located servers in the same container; streamable HTTP for remote servers (kagenti compatibility) |
| Allowlist | Auto-allow all MCP tools | Configuring a server implies trust; tools register via `RegisterAlwaysAllowed()` |
| Naming | Prefix with server name | `weather_get_forecast` — prevents collisions between servers and with built-in tools |
| Connection lifecycle | Connect on agent startup | Fail fast if unreachable; go-sdk handles transport-level reconnection |
| Resilience | Fail and report | No custom retry logic; go-sdk transports handle transient reconnection; tool call failures surface as errors to the LLM |

## Configuration

MCP servers are configured in `agent-config.yaml` under `tools.mcp`:

```yaml
tools:
  allowed:
    - exec
    - read_file
  mcp:
    - name: weather
      transport: streamable_http
      url: "http://weather-tool:8000/mcp"
    - name: s3storage
      transport: stdio
      command: python
      args: ["-m", "s3_mcp_server"]
      env:
        AWS_REGION: us-east-1
```

Fields:

| Field | Required | Transport | Description |
|-------|----------|-----------|-------------|
| `name` | yes | both | Server name, used as tool name prefix |
| `transport` | yes | both | `streamable_http` or `stdio` |
| `url` | yes | streamable_http | Server endpoint URL |
| `command` | yes | stdio | Executable to run |
| `args` | no | stdio | Command arguments |
| `env` | no | stdio | Environment variables for the subprocess |

Config struct additions in `internal/cmd/agentconfig.go`:

```go
type ToolsConfig struct {
    Allowed   []string          `yaml:"allowed"`
    Exec      ExecToolConfig    `yaml:"exec"`
    WebFetch  WebFetchCfg       `yaml:"web_fetch"`
    Workspace string            `yaml:"workspace"`
    MCP       []MCPServerConfig `yaml:"mcp"`
}

type MCPServerConfig struct {
    Name      string            `yaml:"name"`
    Transport string            `yaml:"transport"`
    URL       string            `yaml:"url"`
    Command   string            `yaml:"command"`
    Args      []string          `yaml:"args"`
    Env       map[string]string `yaml:"env"`
}
```

## Package Structure

New package: `internal/mcpclient/` with three files.

### `mcpclient.go` — Connection Manager

```go
type serverConn struct {
    name    string
    session *mcp.ClientSession
    tools   []*mcpTool
}

type Manager struct {
    servers []*serverConn
}
```

Responsibilities:

- `NewManager(ctx, configs)` — connect to all configured MCP servers,
  call initialize + tools/list on each, fail fast if any is unreachable
- `Tools()` — return all discovered MCP tools as `tools.Tool`
  implementations
- `Close()` — shut down all connections

### `tool.go` — Tool Adapter

```go
type mcpTool struct {
    prefix  string             // server name
    name    string             // original MCP tool name
    desc    string             // MCP tool description
    schema  map[string]any     // JSON Schema from MCP inputSchema
    session *mcp.ClientSession // go-sdk session
}
```

Implements `tools.Tool`:

- `Name()` → `"{prefix}_{name}"` (e.g. `"weather_get_forecast"`)
- `Description()` → pass-through from MCP server
- `Parameters()` → pass-through JSON Schema from MCP server
- `Execute(ctx, args)` → calls `session.CallTool()`, converts result

### `transport.go` — Transport Factory

Takes `MCPServerConfig`, returns `mcp.Transport`:

- `streamable_http` → `mcp.StreamableClientTransport{Endpoint: url}`
- `stdio` → `mcp.CommandTransport{Command: exec.Command(cmd, args...)}`
  with env vars applied

Validates required fields per transport type before constructing.

## Integration Point

In `internal/cmd/serve.go`, after existing tool registration
(~line 242):

```go
if agentCfg != nil && len(agentCfg.Tools.MCP) > 0 {
    mcpMgr, err := mcpclient.NewManager(ctx, agentCfg.Tools.MCP)
    if err != nil {
        return fmt.Errorf("failed to connect to MCP servers: %w", err)
    }
    defer mcpMgr.Close()

    for _, t := range mcpMgr.Tools() {
        toolRegistry.RegisterAlwaysAllowed(t)
    }
}
```

No changes required to `pkg/tools/` (registry, loop, hooks),
`pkg/llm/` (providers, types), or any existing tool implementations.

## MCP Result Conversion

MCP `CallToolResult` contains typed content blocks. Conversion to
`tools.ToolResult`:

- **Text content**: concatenate all text blocks, newline-separated
- **Image/resource content**: skip for v1 (text-only tool results);
  log warning when content is dropped
- **`IsError` flag**: map directly to `ToolResult.Error`

```go
func convertResult(result *mcp.CallToolResult) *tools.ToolResult {
    var parts []string
    for _, block := range result.Content {
        if textContent, ok := block.(mcp.TextContent); ok {
            parts = append(parts, textContent.Text)
        }
    }
    output := strings.Join(parts, "\n")
    if result.IsError {
        return tools.Errorf("%s", output)
    }
    return tools.OK(output)
}
```

## Error Handling

### Startup Errors (Fail Fast)

| Error | Behavior |
| ------- | ---------- |
| Invalid transport type | Validation error naming the server |
| Missing required fields | Validation error before connection attempt |
| Server unreachable | Error naming the server and underlying cause |
| Initialize/ListTools fails | Same — agent refuses to start |
| Duplicate server name | Validation error |
| Tool name collision | Error naming both the tool and originating server |

### Runtime Errors

`session.CallTool()` failures return `tools.Errorf(...)` — the LLM
sees the error and decides how to proceed. No custom retry logic;
go-sdk transports handle transient reconnection at the transport layer.

## Logging

Using `log/slog` per project conventions:

| Event | Level | Fields |
| ------- | ------- | -------- |
| Connected to server | Info | name, transport, tool count |
| Tool call | Debug | server, tool name |
| Non-text content skipped | Warn | server, tool, content type |
| Disconnected | Info | name |

## Testing

### Unit Tests (`internal/mcpclient/`)

**`mcpclient_test.go`**: Test `NewManager` with go-sdk's
`NewInMemoryTransports()`. Create in-process MCP servers with test
tools, verify discovery, prefixing, duplicate rejection, empty tool
list handling, connection failure propagation.

**`tool_test.go`**: Test `mcpTool.Execute()` against in-memory MCP
server. Verify text concatenation, multi-block results, error flag
mapping, non-text content handling.

**`transport_test.go`**: Config validation tests (no connections).
Missing URL for streamable_http, missing command for stdio, unknown
transport type.

### Integration Test

**`internal/mcpclient/integration_test.go`**: Build-tagged
(`//go:build integration`). Starts a real stdio MCP server subprocess
(small Go program in `testdata/`), connects, lists tools, calls a
tool, verifies the result end-to-end.

### Existing Tests

No modifications needed — MCP tools are `tools.Tool` implementations;
the registry and loop tests remain unchanged.

## Files Changed

| File | Change |
| ------ | -------- |
| `go.mod` | Add `github.com/modelcontextprotocol/go-sdk` |
| `internal/cmd/agentconfig.go` | Add `MCP` field to `ToolsConfig`, add `MCPServerConfig` struct |
| `internal/cmd/serve.go` | Add MCP manager initialization after built-in tool registration |
| `internal/mcpclient/mcpclient.go` | New — connection manager |
| `internal/mcpclient/tool.go` | New — tool adapter |
| `internal/mcpclient/transport.go` | New — transport factory |
| `internal/mcpclient/*_test.go` | New — unit tests |
| `internal/mcpclient/integration_test.go` | New — integration test |
| `testdata/` | New — test MCP server fixture, test agent-config with MCP |

## Out of Scope for v1

- Image/resource content in tool results (requires `ToolResult` redesign)
- Per-server tool allowlists/denylists
- MCP resources and prompts (only tools)
- OAuth/authentication for streamable HTTP servers
- Dynamic tool list changes (re-listing tools after `ToolListChanged`
  notification)
