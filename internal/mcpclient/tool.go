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
