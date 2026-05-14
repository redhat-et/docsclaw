# MCP Client Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable DocsClaw agents to connect to external MCP tool
servers and use their tools transparently through the existing tool
registry.

**Architecture:** New `internal/mcpclient/` package with three files:
transport factory, tool adapter, and connection manager. The manager
connects to configured MCP servers on startup, wraps each discovered
tool as a `tools.Tool`, and registers them via `RegisterAlwaysAllowed`.
Integration is a single block in `serve.go`.

**Tech Stack:** `github.com/modelcontextprotocol/go-sdk/mcp` (official
MCP Go SDK v1.x), existing `pkg/tools` interfaces.

**Spec:** `docs/superpowers/specs/2026-05-07-mcp-client-integration-design.md`

---

## File Structure

| File | Responsibility |
| ---- | -------------- |
| `internal/mcpclient/transport.go` | Config types, validation, transport factory |
| `internal/mcpclient/transport_test.go` | Config validation tests |
| `internal/mcpclient/tool.go` | MCP-to-Tool adapter, result conversion |
| `internal/mcpclient/tool_test.go` | Tool adapter tests with in-memory MCP server |
| `internal/mcpclient/mcpclient.go` | Manager: connect, discover, lifecycle |
| `internal/mcpclient/mcpclient_test.go` | Manager tests with in-memory MCP servers |
| `internal/cmd/agentconfig.go` | Add `MCP` field and `MCPServerConfig` struct |
| `internal/cmd/serve.go` | Wire up MCP manager after built-in tools |

---

### Task 1: Add go-sdk dependency

**Files:**

- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/modelcontextprotocol/go-sdk/mcp@latest
```

- [ ] **Step 2: Tidy modules**

```bash
go mod tidy
```

- [ ] **Step 3: Verify the dependency resolves**

```bash
go list -m github.com/modelcontextprotocol/go-sdk
```

Expected: prints module path and version (v1.5.0 or later).

- [ ] **Step 4: Verify existing tests still pass**

```bash
make test
```

Expected: all tests pass — no behavior changes.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -s -m "deps: add modelcontextprotocol/go-sdk for MCP client"
```

---

### Task 2: Config types and transport factory

**Files:**

- Create: `internal/mcpclient/transport.go`
- Create: `internal/mcpclient/transport_test.go`

- [ ] **Step 1: Write failing tests for config validation**

Create `internal/mcpclient/transport_test.go`:

```go
package mcpclient

import (
	"testing"
)

func TestValidateConfig_StreamableHTTP(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "weather",
		Transport: "streamable_http",
		URL:       "http://localhost:8080/mcp",
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid streamable_http config rejected: %v", err)
	}
}

func TestValidateConfig_StreamableHTTP_MissingURL(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "weather",
		Transport: "streamable_http",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestValidateConfig_Stdio(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "s3",
		Transport: "stdio",
		Command:   "python",
		Args:      []string{"-m", "s3_server"},
		Env:       map[string]string{"AWS_REGION": "us-east-1"},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid stdio config rejected: %v", err)
	}
}

func TestValidateConfig_Stdio_MissingCommand(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "s3",
		Transport: "stdio",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestValidateConfig_MissingName(t *testing.T) {
	cfg := MCPServerConfig{
		Transport: "stdio",
		Command:   "python",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateConfig_UnknownTransport(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "bad",
		Transport: "grpc",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd internal/mcpclient && go test -v -run TestValidateConfig
```

Expected: compilation error — `MCPServerConfig` not defined.

- [ ] **Step 3: Implement config types and validation**

Create `internal/mcpclient/transport.go`:

```go
package mcpclient

import (
	"fmt"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerConfig defines an MCP server connection in agent-config.yaml.
type MCPServerConfig struct {
	Name      string            `yaml:"name"`
	Transport string            `yaml:"transport"`
	URL       string            `yaml:"url"`
	Command   string            `yaml:"command"`
	Args      []string          `yaml:"args"`
	Env       map[string]string `yaml:"env"`
}

func (c *MCPServerConfig) validate() error {
	if c.Name == "" {
		return fmt.Errorf("MCP server config: name is required")
	}
	switch c.Transport {
	case "streamable_http":
		if c.URL == "" {
			return fmt.Errorf("MCP server %q: url is required for streamable_http transport", c.Name)
		}
	case "stdio":
		if c.Command == "" {
			return fmt.Errorf("MCP server %q: command is required for stdio transport", c.Name)
		}
	default:
		return fmt.Errorf("MCP server %q: unsupported transport %q (must be \"streamable_http\" or \"stdio\")", c.Name, c.Transport)
	}
	return nil
}

func (c *MCPServerConfig) newTransport() mcp.Transport {
	switch c.Transport {
	case "streamable_http":
		return &mcp.StreamableClientTransport{
			Endpoint: c.URL,
		}
	case "stdio":
		cmd := exec.Command(c.Command, c.Args...)
		if len(c.Env) > 0 {
			cmd.Env = append(cmd.Environ(), envSlice(c.Env)...)
		}
		return &mcp.CommandTransport{
			Command: cmd,
		}
	default:
		panic("unreachable: validate() should be called before newTransport()")
	}
}

func envSlice(env map[string]string) []string {
	s := make([]string, 0, len(env))
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd internal/mcpclient && go test -v -run TestValidateConfig
```

Expected: all 6 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/mcpclient/transport.go internal/mcpclient/transport_test.go
git commit -s -m "feat(mcp): add config types and transport factory"
```

---

### Task 3: Tool adapter with result conversion

**Files:**

- Create: `internal/mcpclient/tool.go`
- Create: `internal/mcpclient/tool_test.go`

- [ ] **Step 1: Write failing test for tool name and metadata**

Create `internal/mcpclient/tool_test.go`:

```go
package mcpclient

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestServerAndSession(t *testing.T, tools ...*mcp.Tool) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "v1.0.0",
	}, nil)

	for _, tool := range tools {
		toolCopy := tool
		server.AddTool(toolCopy, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "default"}},
			}, nil
		})
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "v1.0.0",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestMCPTool_NameAndDescription(t *testing.T) {
	session := newTestServerAndSession(t, &mcp.Tool{
		Name:        "get_forecast",
		Description: "Get weather forecast",
	})

	tool := &mcpTool{
		prefix:  "weather",
		name:    "get_forecast",
		desc:    "Get weather forecast",
		schema:  map[string]any{"type": "object"},
		session: session,
	}

	if got := tool.Name(); got != "weather_get_forecast" {
		t.Fatalf("expected weather_get_forecast, got %q", got)
	}
	if got := tool.Description(); got != "Get weather forecast" {
		t.Fatalf("expected description pass-through, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd internal/mcpclient && go test -v -run TestMCPTool_NameAndDescription
```

Expected: compilation error — `mcpTool` not defined.

- [ ] **Step 3: Implement the tool adapter struct**

Create `internal/mcpclient/tool.go`:

```go
package mcpclient

import (
	"context"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type mcpTool struct {
	prefix  string
	name    string
	desc    string
	schema  map[string]any
	session *mcp.ClientSession
}

func (t *mcpTool) Name() string {
	return t.prefix + "_" + t.name
}

func (t *mcpTool) Description() string {
	return t.desc
}

func (t *mcpTool) Parameters() map[string]any {
	return t.schema
}

func (t *mcpTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	slog.Debug("calling MCP tool", "server", t.prefix, "tool", t.name)

	result, err := t.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      t.name,
		Arguments: args,
	})
	if err != nil {
		return tools.Errorf("MCP call failed: %v", err)
	}
	return convertResult(t.prefix, t.name, result)
}

func convertResult(prefix, name string, result *mcp.CallToolResult) *tools.ToolResult {
	var parts []string
	for _, c := range result.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		default:
			slog.Warn("skipping non-text MCP content",
				"server", prefix, "tool", name,
			)
		}
	}
	output := strings.Join(parts, "\n")
	if result.IsError {
		return tools.Errorf("%s", output)
	}
	return tools.OK(output)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd internal/mcpclient && go test -v -run TestMCPTool_NameAndDescription
```

Expected: PASS.

- [ ] **Step 5: Write test for Execute and result conversion**

Add to `internal/mcpclient/tool_test.go`:

```go
func newTestServerWithHandler(t *testing.T, toolName string, handler func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "v1.0.0",
	}, nil)
	server.AddTool(&mcp.Tool{
		Name:        toolName,
		Description: "test tool",
	}, handler)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "v1.0.0",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestMCPTool_Execute_TextResult(t *testing.T) {
	session := newTestServerWithHandler(t, "greet",
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "hello world"}},
			}, nil
		})

	tool := &mcpTool{
		prefix:  "test",
		name:    "greet",
		schema:  map[string]any{"type": "object"},
		session: session,
	}

	result := tool.Execute(context.Background(), map[string]any{})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "hello world" {
		t.Fatalf("expected 'hello world', got %q", result.Output)
	}
}

func TestMCPTool_Execute_MultipleTextBlocks(t *testing.T) {
	session := newTestServerWithHandler(t, "multi",
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "line one"},
					&mcp.TextContent{Text: "line two"},
				},
			}, nil
		})

	tool := &mcpTool{
		prefix:  "test",
		name:    "multi",
		schema:  map[string]any{"type": "object"},
		session: session,
	}

	result := tool.Execute(context.Background(), map[string]any{})
	if result.Output != "line one\nline two" {
		t.Fatalf("expected concatenated output, got %q", result.Output)
	}
}

func TestMCPTool_Execute_ErrorResult(t *testing.T) {
	session := newTestServerWithHandler(t, "fail",
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "something went wrong"}},
				IsError: true,
			}, nil
		})

	tool := &mcpTool{
		prefix:  "test",
		name:    "fail",
		schema:  map[string]any{"type": "object"},
		session: session,
	}

	result := tool.Execute(context.Background(), map[string]any{})
	if !result.Error {
		t.Fatal("expected error result")
	}
	if result.Output != "something went wrong" {
		t.Fatalf("expected error message, got %q", result.Output)
	}
}
```

- [ ] **Step 6: Run all tool tests**

```bash
cd internal/mcpclient && go test -v -run TestMCPTool
```

Expected: all 4 tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/mcpclient/tool.go internal/mcpclient/tool_test.go
git commit -s -m "feat(mcp): add tool adapter with result conversion"
```

---

### Task 4: Connection manager

**Files:**

- Create: `internal/mcpclient/mcpclient.go`
- Create: `internal/mcpclient/mcpclient_test.go`

- [ ] **Step 1: Write failing test for Manager with single server**

Create `internal/mcpclient/mcpclient_test.go`:

```go
package mcpclient

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testServer struct {
	server          *mcp.Server
	serverTransport *mcp.InMemoryTransport
	clientTransport *mcp.InMemoryTransport
}

func newInMemoryTestServer(t *testing.T, name string, toolNames ...string) *testServer {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: "v1.0.0",
	}, nil)
	for _, tn := range toolNames {
		toolName := tn
		server.AddTool(&mcp.Tool{
			Name:        toolName,
			Description: "test tool " + toolName,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "result from " + toolName}},
			}, nil
		})
	}
	st, ct := mcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	return &testServer{server: server, serverTransport: st, clientTransport: ct}
}

func TestManager_SingleServer(t *testing.T) {
	ts := newInMemoryTestServer(t, "weather", "get_forecast", "get_alerts")

	mgr, err := newManagerFromTransports(context.Background(), []transportEntry{
		{name: "weather", transport: ts.clientTransport},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	tools := mgr.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	if !names["weather_get_forecast"] {
		t.Fatal("expected weather_get_forecast")
	}
	if !names["weather_get_alerts"] {
		t.Fatal("expected weather_get_alerts")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd internal/mcpclient && go test -v -run TestManager_SingleServer
```

Expected: compilation error — `Manager`, `newManagerFromTransports`,
`transportEntry` not defined.

- [ ] **Step 3: Implement the Manager**

Create `internal/mcpclient/mcpclient.go`:

```go
package mcpclient

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type serverConn struct {
	name    string
	session *mcp.ClientSession
	tools   []*mcpTool
}

// Manager manages connections to MCP servers and exposes their tools.
type Manager struct {
	servers []*serverConn
}

// transportEntry is used internally and in tests to pass pre-built transports.
type transportEntry struct {
	name      string
	transport mcp.Transport
}

// NewManager connects to all configured MCP servers, discovers their
// tools, and returns a Manager. It fails fast if any server is
// unreachable or has configuration errors.
func NewManager(ctx context.Context, configs []MCPServerConfig) (*Manager, error) {
	seen := make(map[string]bool, len(configs))
	entries := make([]transportEntry, 0, len(configs))

	for i := range configs {
		if err := configs[i].validate(); err != nil {
			return nil, err
		}
		if seen[configs[i].Name] {
			return nil, fmt.Errorf("MCP server %q: duplicate name", configs[i].Name)
		}
		seen[configs[i].Name] = true
		entries = append(entries, transportEntry{
			name:      configs[i].Name,
			transport: configs[i].newTransport(),
		})
	}

	return newManagerFromTransports(ctx, entries)
}

func newManagerFromTransports(ctx context.Context, entries []transportEntry) (*Manager, error) {
	mgr := &Manager{}
	toolNames := make(map[string]string)

	for _, entry := range entries {
		client := mcp.NewClient(&mcp.Implementation{
			Name:    "docsclaw",
			Version: "v1.0.0",
		}, nil)

		session, err := client.Connect(ctx, entry.transport, nil)
		if err != nil {
			mgr.Close()
			return nil, fmt.Errorf("MCP server %q: failed to connect: %w", entry.name, err)
		}

		result, err := session.ListTools(ctx, nil)
		if err != nil {
			mgr.Close()
			return nil, fmt.Errorf("MCP server %q: failed to list tools: %w", entry.name, err)
		}

		conn := &serverConn{
			name:    entry.name,
			session: session,
		}

		for _, t := range result.Tools {
			prefixedName := entry.name + "_" + t.Name
			if existingServer, ok := toolNames[prefixedName]; ok {
				mgr.Close()
				return nil, fmt.Errorf("MCP tool %q: already registered (from server %q)", prefixedName, existingServer)
			}
			toolNames[prefixedName] = entry.name

			schema, _ := t.InputSchema.(map[string]any)
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}

			conn.tools = append(conn.tools, &mcpTool{
				prefix:  entry.name,
				name:    t.Name,
				desc:    t.Description,
				schema:  schema,
				session: session,
			})
		}

		slog.Info("connected to MCP server",
			"name", entry.name,
			"tools", len(conn.tools),
		)
		mgr.servers = append(mgr.servers, conn)
	}

	return mgr, nil
}

// Tools returns all discovered MCP tools as tools.Tool implementations.
func (m *Manager) Tools() []tools.Tool {
	var result []tools.Tool
	for _, s := range m.servers {
		for _, t := range s.tools {
			result = append(result, t)
		}
	}
	return result
}

// Close shuts down all MCP server connections.
func (m *Manager) Close() error {
	var lastErr error
	for _, s := range m.servers {
		if err := s.session.Close(); err != nil {
			slog.Info("disconnected from MCP server", "name", s.name)
			lastErr = err
		} else {
			slog.Info("disconnected from MCP server", "name", s.name)
		}
	}
	return lastErr
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd internal/mcpclient && go test -v -run TestManager_SingleServer
```

Expected: PASS.

- [ ] **Step 5: Write test for multiple servers**

Add to `internal/mcpclient/mcpclient_test.go`:

```go
func TestManager_MultipleServers(t *testing.T) {
	ts1 := newInMemoryTestServer(t, "weather", "get_forecast")
	ts2 := newInMemoryTestServer(t, "storage", "list_files", "read_file")

	mgr, err := newManagerFromTransports(context.Background(), []transportEntry{
		{name: "weather", transport: ts1.clientTransport},
		{name: "storage", transport: ts2.clientTransport},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	tools := mgr.Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	if !names["weather_get_forecast"] {
		t.Fatal("missing weather_get_forecast")
	}
	if !names["storage_list_files"] {
		t.Fatal("missing storage_list_files")
	}
	if !names["storage_read_file"] {
		t.Fatal("missing storage_read_file")
	}
}
```

- [ ] **Step 6: Run test**

```bash
cd internal/mcpclient && go test -v -run TestManager_MultipleServers
```

Expected: PASS.

- [ ] **Step 7: Write test for duplicate server name rejection**

Add to `internal/mcpclient/mcpclient_test.go`:

```go
func TestManager_DuplicateServerName(t *testing.T) {
	cfg := []MCPServerConfig{
		{Name: "weather", Transport: "streamable_http", URL: "http://a:8080/mcp"},
		{Name: "weather", Transport: "streamable_http", URL: "http://b:8080/mcp"},
	}
	_, err := NewManager(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for duplicate server name")
	}
}
```

- [ ] **Step 8: Run test**

```bash
cd internal/mcpclient && go test -v -run TestManager_DuplicateServerName
```

Expected: PASS.

- [ ] **Step 9: Write test for empty server list**

Add to `internal/mcpclient/mcpclient_test.go`:

```go
func TestManager_EmptyConfig(t *testing.T) {
	mgr, err := NewManager(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	if len(mgr.Tools()) != 0 {
		t.Fatal("expected no tools from empty config")
	}
}
```

- [ ] **Step 10: Run test**

```bash
cd internal/mcpclient && go test -v -run TestManager_EmptyConfig
```

Expected: PASS.

- [ ] **Step 11: Write test for tool execution through Manager**

Add to `internal/mcpclient/mcpclient_test.go`:

```go
func TestManager_ToolExecution(t *testing.T) {
	ts := newInMemoryTestServer(t, "echo", "say")

	mgr, err := newManagerFromTransports(context.Background(), []transportEntry{
		{name: "echo", transport: ts.clientTransport},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	tools := mgr.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	result := tools[0].Execute(context.Background(), map[string]any{})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "result from say" {
		t.Fatalf("expected 'result from say', got %q", result.Output)
	}
}
```

- [ ] **Step 12: Run all Manager tests**

```bash
cd internal/mcpclient && go test -v -run TestManager
```

Expected: all 5 tests pass.

- [ ] **Step 13: Run full project tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 14: Commit**

```bash
git add internal/mcpclient/mcpclient.go internal/mcpclient/mcpclient_test.go
git commit -s -m "feat(mcp): add connection manager with tool discovery"
```

---

### Task 5: Wire up agent config and serve.go

**Files:**

- Modify: `internal/cmd/agentconfig.go`
- Modify: `internal/cmd/serve.go`

- [ ] **Step 1: Add MCP config to agentconfig.go**

In `internal/cmd/agentconfig.go`, add the `MCP` field to
`ToolsConfig` and import the mcpclient package:

Add import:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/redhat-et/docsclaw/internal/mcpclient"
	"github.com/redhat-et/docsclaw/pkg/tools"
)
```

Add the `MCP` field to `ToolsConfig`:

```go
type ToolsConfig struct {
	Allowed   []string                   `yaml:"allowed"`
	Exec      ExecToolConfig             `yaml:"exec"`
	WebFetch  WebFetchCfg                `yaml:"web_fetch"`
	Workspace string                     `yaml:"workspace"`
	MCP       []mcpclient.MCPServerConfig `yaml:"mcp"`
}
```

- [ ] **Step 2: Add MCP initialization to serve.go**

In `internal/cmd/serve.go`, add import for mcpclient:

```go
"github.com/redhat-et/docsclaw/internal/mcpclient"
```

After the existing tool registration block (after line ~244,
`loopCfg = agentCfg.toLoopConfig()`), add:

```go
		if len(agentCfg.Tools.MCP) > 0 {
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

- [ ] **Step 3: Verify compilation**

```bash
make build
```

Expected: compiles without errors.

- [ ] **Step 4: Run full test suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/agentconfig.go internal/cmd/serve.go
git commit -s -m "feat(mcp): wire MCP client into agent config and serve"
```

---

### Task 6: Add test agent config fixture

**Files:**

- Create: `testdata/mcp-agent/agent-config.yaml`
- Create: `testdata/mcp-agent/system-prompt.txt`

- [ ] **Step 1: Create test fixture directory and files**

Create `testdata/mcp-agent/system-prompt.txt`:

```text
You are a test agent with MCP tools.
```

Create `testdata/mcp-agent/agent-config.yaml`:

```yaml
tools:
  allowed:
    - exec
  mcp:
    - name: weather
      transport: streamable_http
      url: "http://weather-tool:8000/mcp"
    - name: localtools
      transport: stdio
      command: python
      args: ["-m", "local_mcp_server"]
      env:
        LOG_LEVEL: debug

loop:
  max_iterations: 10
```

- [ ] **Step 2: Verify the config parses correctly**

Add a test to verify MCP config parsing. Create or add to an
existing test file. The simplest check: load the YAML and verify
the MCP field is populated.

Add to `internal/mcpclient/transport_test.go`:

```go
func TestMCPServerConfig_YAMLParsing(t *testing.T) {
	data := []byte(`
- name: weather
  transport: streamable_http
  url: "http://weather-tool:8000/mcp"
- name: localtools
  transport: stdio
  command: python
  args: ["-m", "local_mcp_server"]
  env:
    LOG_LEVEL: debug
`)
	var configs []MCPServerConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("YAML parse: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
	if configs[0].Name != "weather" || configs[0].URL != "http://weather-tool:8000/mcp" {
		t.Fatalf("unexpected first config: %+v", configs[0])
	}
	if configs[1].Command != "python" || len(configs[1].Args) != 2 {
		t.Fatalf("unexpected second config: %+v", configs[1])
	}
	if configs[1].Env["LOG_LEVEL"] != "debug" {
		t.Fatalf("expected env LOG_LEVEL=debug, got %v", configs[1].Env)
	}
}
```

Add `"gopkg.in/yaml.v3"` to the imports in `transport_test.go`.

- [ ] **Step 3: Run test**

```bash
cd internal/mcpclient && go test -v -run TestMCPServerConfig_YAML
```

Expected: PASS.

- [ ] **Step 4: Run full test suite**

```bash
make test && make lint
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add testdata/mcp-agent/ internal/mcpclient/transport_test.go
git commit -s -m "feat(mcp): add test fixtures and YAML parsing test"
```

---

### Task 7: Documentation update

**Files:**

- Modify: `CLAUDE.md`

- [ ] **Step 1: Update project structure table in CLAUDE.md**

Add the `internal/mcpclient/` entry to the project structure table
in `CLAUDE.md`, after the `internal/writefile/` row:

```
| `internal/mcpclient/` | MCP client: connect to external MCP tool servers |
```

- [ ] **Step 2: Run linter**

```bash
make lint
```

Expected: passes.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -s -m "docs: add mcpclient to project structure table"
```
