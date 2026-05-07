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
			InputSchema: map[string]any{"type": "object"},
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
