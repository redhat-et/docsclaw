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
		if toolCopy.InputSchema == nil {
			toolCopy.InputSchema = map[string]any{"type": "object"}
		}
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
	t.Cleanup(func() { _ = session.Close() })
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

func newTestServerWithHandler(t *testing.T, toolName string, handler func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "v1.0.0",
	}, nil)
	server.AddTool(&mcp.Tool{
		Name:        toolName,
		Description: "test tool",
		InputSchema: map[string]any{"type": "object"},
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
	t.Cleanup(func() { _ = session.Close() })
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
