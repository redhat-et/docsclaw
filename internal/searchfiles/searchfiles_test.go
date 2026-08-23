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

func TestSearchFilesToolRelativeTraversalBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	outsideDir := t.TempDir()

	tool := NewSearchFilesTool(tmpDir)

	cases := []struct {
		name string
		path string
	}{
		{name: "parent traversal", path: ".."},
		{name: "nested traversal escape", path: filepath.Join("subdir", "..", "..", filepath.Base(outsideDir))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tool.Execute(context.Background(), map[string]any{
				"path":  tc.path,
				"regex": "secret",
			})
			if !result.Error {
				t.Fatal("expected error for relative path outside workspace")
			}
			if !strings.Contains(result.Output, "outside workspace") {
				t.Fatalf("expected outside workspace error, got %q", result.Output)
			}
		})
	}
}

func TestSearchFilesToolInvalidRegex(t *testing.T) {
	tool := NewSearchFilesTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{
		"path":  ".",
		"regex": "[",
	})

	if !result.Error {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(result.Output, "invalid regex") {
		t.Fatalf("expected invalid regex error, got %q", result.Output)
	}
}

func TestSearchFilesToolFilePattern(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}

	tmpDir := t.TempDir()
	if err := writeFile(filepath.Join(tmpDir, "a.go"), "foo\n"); err != nil {
		t.Fatalf("failed to write go file: %v", err)
	}
	if err := writeFile(filepath.Join(tmpDir, "a.txt"), "foo\n"); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	tool := NewSearchFilesTool(tmpDir)
	result := tool.Execute(context.Background(), map[string]any{
		"path":         tmpDir,
		"regex":        "foo",
		"file_pattern": "*.go",
	})

	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "a.go") {
		t.Fatalf("expected a.go match, got %q", result.Output)
	}
	if strings.Contains(result.Output, "a.txt") {
		t.Fatalf("did not expect a.txt match, got %q", result.Output)
	}
}

func TestSearchFilesToolRelativePath(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := writeFile(filepath.Join(subDir, "test.go"), "helloWorld\n"); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := NewSearchFilesTool(tmpDir)
	result := tool.Execute(context.Background(), map[string]any{
		"path":  "subdir",
		"regex": "helloWorld",
	})

	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "test.go:1:helloWorld") {
		t.Fatalf("expected relative path to resolve, got %q", result.Output)
	}
}

func TestSearchFilesToolSymlinkEscapeBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := writeFile(filepath.Join(outsideDir, "secret.txt"), "secret\n"); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	linkPath := filepath.Join(tmpDir, "escape")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	tool := NewSearchFilesTool(tmpDir)
	result := tool.Execute(context.Background(), map[string]any{
		"path":  linkPath,
		"regex": "secret",
	})

	if !result.Error {
		t.Fatal("expected error for symlink outside workspace")
	}
	if !strings.Contains(result.Output, "outside workspace") {
		t.Fatalf("expected outside workspace error, got %q", result.Output)
	}
}

// Runtime requirement: search_files shells out to ripgrep (rg). Production
// container images install the ripgrep package; if rg is missing from PATH,
// Execute returns a clear error.
func TestSearchFilesToolMissingRipgrep(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewSearchFilesTool(tmpDir)

	// Hide rg from PATH so LookPath cannot find it.
	t.Setenv("PATH", "")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  ".",
		"regex": "foo",
	})

	if !result.Error {
		t.Fatal("expected error when rg is not available")
	}
	if !strings.Contains(result.Output, "ripgrep (rg) is not installed") {
		t.Fatalf("expected ripgrep missing error, got %q", result.Output)
	}
}

func TestSearchFilesToolInvalidRegexEmptyWorkspace(t *testing.T) {
	tool := NewSearchFilesTool("")
	result := tool.Execute(context.Background(), map[string]any{
		"path":  ".",
		"regex": "[",
	})

	if !result.Error {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(result.Output, "invalid regex") {
		t.Fatalf("expected invalid regex error, got %q", result.Output)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
