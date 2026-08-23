package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWorkspacePathEmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	resolved, err := ResolveWorkspacePath(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != path {
		t.Fatalf("expected %q, got %q", path, resolved)
	}
}

func TestResolveWorkspacePathRelativeInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	resolved, err := ResolveWorkspacePath("file.txt", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(dir, "file.txt")
	if resolved != expected {
		t.Fatalf("expected %q, got %q", expected, resolved)
	}
}

func TestResolveWorkspacePathAbsoluteInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	resolved, err := ResolveWorkspacePath(path, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != path {
		t.Fatalf("expected %q, got %q", path, resolved)
	}
}

func TestResolveWorkspacePathOutsideWorkspaceBlocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "..", "escaped.txt")

	_, err := ResolveWorkspacePath(path, dir)
	if err == nil {
		t.Fatal("expected error for path outside workspace")
	}
	if !strings.Contains(err.Error(), "path outside workspace") {
		t.Fatalf("expected outside workspace error, got %q", err)
	}
}
