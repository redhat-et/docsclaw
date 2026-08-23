package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchCleanApply(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-line2",
		"+line2modified",
		" line3",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "Patched "+path {
		t.Fatalf("expected success message %q, got %q", "Patched "+path, result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified\nline3\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestApplyPatchFailedHunkReturnsError(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	original := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-wrongline",
		"+replacement",
		" line3",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for failed hunk")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != original {
		t.Fatalf("file should be unchanged, expected %q, got %q", original, string(data))
	}
}

func TestApplyPatchWorkspaceEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "..", "escaped.txt")

	patch := strings.Join([]string{
		"--- a/escaped.txt",
		"+++ b/escaped.txt",
		"@@ -1,1 +1,1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for path outside workspace")
	}
	if !strings.Contains(result.Output, "workspace") {
		t.Fatalf("expected workspace error, got %q", result.Output)
	}
}

func TestApplyPatchMissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "missing.txt")

	patch := strings.Join([]string{
		"--- a/missing.txt",
		"+++ b/missing.txt",
		"@@ -1,1 +1,1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(result.Output, "does not exist") {
		t.Fatalf("expected missing file error, got %q", result.Output)
	}
}

func TestApplyPatchRelativePathResolvedAgainstWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	relPath := "subdir/file.txt"
	absPath := filepath.Join(dir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/subdir/file.txt",
		"+++ b/subdir/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-line2",
		"+line2modified",
		" line3",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  relPath,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "Patched "+absPath {
		t.Fatalf("expected success message %q, got %q", "Patched "+absPath, result.Output)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified\nline3\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}
