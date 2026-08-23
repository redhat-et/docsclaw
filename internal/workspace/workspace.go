package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveWorkspacePath resolves path against workspaceDir, returning an
// absolute path. If workspaceDir is empty, path is resolved against the
// current working directory. If workspaceDir is non-empty and the resolved
// path lies outside the workspace, an error is returned.
func ResolveWorkspacePath(path, workspaceDir string) (string, error) {
	if workspaceDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(workspaceDir, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if workspaceDir != "" && !IsInsideWorkspace(absPath, workspaceDir) {
		return "", fmt.Errorf("path outside workspace: %s", absPath)
	}

	return absPath, nil
}

// IsInsideWorkspace checks if path is inside workspace, resolving
// symlinks to prevent traversal bypasses.
func IsInsideWorkspace(path, workspace string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}

	// Resolve symlinks. For new files EvalSymlinks fails, so
	// resolve the parent directory instead.
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	} else if resolved, err := filepath.EvalSymlinks(filepath.Dir(absPath)); err == nil {
		absPath = filepath.Join(resolved, filepath.Base(absPath))
	}
	if resolved, err := filepath.EvalSymlinks(absWorkspace); err == nil {
		absWorkspace = resolved
	}

	// Ensure trailing separator to prevent /workspace/agent matching
	// /workspace/agent-evil
	return strings.HasPrefix(absPath, absWorkspace+string(filepath.Separator)) ||
		absPath == absWorkspace
}
