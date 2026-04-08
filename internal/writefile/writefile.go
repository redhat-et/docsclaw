package writefile

import (
	"context"
	"os"
	"path/filepath"

	"github.com/redhat-et/docsclaw/internal/workspace"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type writeFileTool struct {
	workspaceDir string
}

// NewWriteFileTool creates a new write file tool scoped to the given workspace.
func NewWriteFileTool(workspaceDir string) tools.Tool {
	return &writeFileTool{workspaceDir: workspaceDir}
}

func (t *writeFileTool) Name() string { return "write_file" }
func (t *writeFileTool) Description() string {
	return "Write content to a file. Creates the file if it doesn't exist, " +
		"overwrites if it does."
}
func (t *writeFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *writeFileTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return tools.Errorf("path is required")
	}

	if t.workspaceDir != "" {
		if !workspace.IsInsideWorkspace(path, t.workspaceDir) {
			return tools.Errorf("Access denied: path outside workspace")
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return tools.Errorf("failed to create directory: %s", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return tools.Errorf("failed to write file: %s", err)
	}

	return tools.OK("File written successfully: " + path)
}
