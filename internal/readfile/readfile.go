package readfile

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/redhat-et/docsclaw/internal/workspace"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type readFileTool struct {
	workspaceDir string
}

// NewReadFileTool creates a new read file tool scoped to the given workspace.
func NewReadFileTool(workspaceDir string) tools.Tool {
	return &readFileTool{workspaceDir: workspaceDir}
}

func (t *readFileTool) Name() string { return "read_file" }
func (t *readFileTool) Description() string {
	return "Read a file and return its contents with numbered lines. " +
		"Use when you need to examine file contents."
}
func (t *readFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *readFileTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return tools.Errorf("path is required")
	}

	if t.workspaceDir != "" {
		if !workspace.IsInsideWorkspace(path, t.workspaceDir) {
			return tools.Errorf("Access denied: path outside workspace")
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return tools.Errorf("Error reading file: %s", err)
	}

	lines := strings.Split(string(data), "\n")
	var numbered strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&numbered, "%4d\t%s\n", i+1, line)
	}

	output := numbered.String()
	if len(output) > 50000 {
		output = output[:50000] + "\n...(truncated)"
	}

	return tools.OK(output)
}
