package searchfiles

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFilesToolSuccess(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := writeFile(testFile, "package main\n\nfunc main() {\n\thelloWorld()\n}\n"); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := NewSearchFilesTool(tmpDir)
	result := tool.Execute(context.Background(), map[string]any{
		"path":         tmpDir,
		"regex":        "helloWorld",
		"file_pattern": "*.go",
	})

	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "test.go:4:helloWorld") {
		t.Fatalf("expected match 'test.go:4:helloWorld', got %q", result.Output)
	}
}

func TestSearchFilesToolNoMatches(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := writeFile(testFile, "package main\n"); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := NewSearchFilesTool(tmpDir)
	result := tool.Execute(context.Background(), map[string]any{
		"path":  tmpDir,
		"regex": "notpresent",
	})

	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "No matches found." {
		t.Fatalf("expected 'No matches found.', got %q", result.Output)
	}
}

func TestSearchFilesToolWorkspaceEscapeBlocked(t *testing.T) {
	tool := NewSearchFilesTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{
		"path":  "/etc",
		"regex": "root",
	})

	if !result.Error {
		t.Fatal("expected error for path outside workspace")
	}
	if !strings.Contains(result.Output, "outside workspace") {
		t.Fatalf("expected outside workspace error, got %q", result.Output)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
