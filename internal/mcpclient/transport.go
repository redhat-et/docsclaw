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
		return fmt.Errorf("MCP server: name is required")
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
