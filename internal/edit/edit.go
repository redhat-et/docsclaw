package edit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/docsclaw/internal/workspace"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type editTool struct {
	workspaceDir string
}

// NewEditTool creates a new edit tool scoped to the given workspace.
// An empty workspaceDir disables workspace sandboxing, allowing absolute
// paths anywhere on the filesystem (consistent with read_file/write_file).
func NewEditTool(workspaceDir string) tools.Tool {
	return &editTool{workspaceDir: workspaceDir}
}

func (t *editTool) Name() string { return "edit" }
func (t *editTool) Description() string {
	return "Replace the first occurrence of an exact string in a file. " +
		"If the old string occurs multiple times, only the first is replaced " +
		"and a warning is returned."
}
func (t *editTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit. Relative paths are resolved against the workspace directory.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact string to replace",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement string",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *editTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return tools.Errorf("path is required")
	}

	oldString, ok := args["old_string"].(string)
	if !ok {
		return tools.Errorf("old_string is required")
	}
	if oldString == "" {
		return tools.Errorf("old_string must not be empty")
	}

	newString, ok := args["new_string"].(string)
	if !ok {
		return tools.Errorf("new_string is required")
	}

	if t.workspaceDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(t.workspaceDir, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return tools.Errorf("failed to resolve path: %s", err)
	}

	if t.workspaceDir != "" {
		if !workspace.IsInsideWorkspace(absPath, t.workspaceDir) {
			return tools.Errorf("Access denied: path outside workspace")
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.Errorf("file does not exist: %s", absPath)
		}
		return tools.Errorf("failed to stat file: %s", err)
	}
	if !info.Mode().IsRegular() {
		return tools.Errorf("path is not a regular file: %s", absPath)
	}
	originalMode := info.Mode().Perm()

	original, err := os.ReadFile(absPath)
	if err != nil {
		return tools.Errorf("failed to read file: %s", err)
	}

	content := string(original)
	if !strings.Contains(content, oldString) {
		return tools.Errorf("old_string not found in file: %s", absPath)
	}

	count := strings.Count(content, oldString)
	replaced := strings.Replace(content, oldString, newString, 1)

	tmpFile, err := os.CreateTemp(filepath.Dir(absPath), filepath.Base(absPath)+".*.tmp")
	if err != nil {
		return tools.Errorf("failed to create temp file: %s", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(replaced); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return tools.Errorf("failed to write file: %s", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return tools.Errorf("failed to close temp file: %s", err)
	}
	if err := os.Chmod(tmpPath, originalMode); err != nil {
		_ = os.Remove(tmpPath)
		return tools.Errorf("failed to set file permissions: %s", err)
	}
	if err := os.Rename(tmpPath, absPath); err != nil {
		_ = os.Remove(tmpPath)
		return tools.Errorf("failed to apply edit: %s", err)
	}

	output := fmt.Sprintf("Edited %s", absPath)
	if count > 1 {
		output += fmt.Sprintf("\nWarning: %d occurrences of old_string found; only the first was replaced", count)
	}
	return tools.OK(output)
}
