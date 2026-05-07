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
			slog.Warn("error disconnecting from MCP server", "name", s.name, "error", err)
			lastErr = err
		} else {
			slog.Info("disconnected from MCP server", "name", s.name)
		}
	}
	return lastErr
}
