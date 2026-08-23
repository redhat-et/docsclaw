package edit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditToolSuccess(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("hello world\nline two\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "world",
		"new_string": "universe",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "hello universe\nline two\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
	if !strings.HasPrefix(result.Output, "Edited ") {
		t.Fatalf("expected success output, got %q", result.Output)
	}
}

func TestEditToolMissingOldString(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "missing",
		"new_string": "replacement",
	})
	if !result.Error {
		t.Fatal("expected error for missing old_string")
	}
	if !strings.Contains(result.Output, "old_string not found") {
		t.Fatalf("expected old_string not found error, got %q", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "hello world\n" {
		t.Fatalf("file should be unchanged, got %q", string(data))
	}
}

func TestEditToolMultipleOccurrencesWarns(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("foo bar foo baz\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "foo",
		"new_string": "qux",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "qux bar foo baz\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
	if !strings.Contains(result.Output, "Warning") {
		t.Fatalf("expected warning in output, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "2") {
		t.Fatalf("expected occurrence count in output, got %q", result.Output)
	}
}

func TestEditToolWorkspaceEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "..", "escaped.txt")

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "old",
		"new_string": "new",
	})
	if !result.Error {
		t.Fatal("expected error for path outside workspace")
	}
	if !strings.Contains(result.Output, "outside workspace") {
		t.Fatalf("expected outside workspace error, got %q", result.Output)
	}
}

func TestEditToolRelativePathResolvedAgainstWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	relPath := "subdir/file.txt"
	absPath := filepath.Join(dir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("alpha\nbeta\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       relPath,
		"old_string": "beta",
		"new_string": "gamma",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "alpha\ngamma\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
	if !strings.Contains(result.Output, absPath) {
		t.Fatalf("expected absolute path in output, got %q", result.Output)
	}
}

func TestEditToolEmptyOldString(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "",
		"new_string": "replacement",
	})
	if !result.Error {
		t.Fatal("expected error for empty old_string")
	}
	if !strings.Contains(result.Output, "old_string must not be empty") {
		t.Fatalf("expected old_string must not be empty error, got %q", result.Output)
	}
}

func TestEditToolMissingFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "missing.txt")

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "old",
		"new_string": "new",
	})
	if !result.Error {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(result.Output, "file does not exist") {
		t.Fatalf("expected file does not exist error, got %q", result.Output)
	}
}

func TestEditToolDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	subdir := filepath.Join(dir, "subdir")

	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       subdir,
		"old_string": "old",
		"new_string": "new",
	})
	if !result.Error {
		t.Fatal("expected error for directory path")
	}
	if !strings.Contains(result.Output, "path is not a regular file") {
		t.Fatalf("expected not a regular file error, got %q", result.Output)
	}
}

func TestEditToolSymlinkEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)

	outsideFile := filepath.Join(dir, "..", "outside.txt")
	outsideFile, err := filepath.Abs(outsideFile)
	if err != nil {
		t.Fatalf("failed to resolve outside file: %v", err)
	}
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outsideFile) })

	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       linkPath,
		"old_string": "secret",
		"new_string": "exposed",
	})
	if !result.Error {
		t.Fatal("expected error for symlink workspace escape")
	}
	if !strings.Contains(result.Output, "outside workspace") {
		t.Fatalf("expected outside workspace error, got %q", result.Output)
	}

	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("failed to read outside file: %v", err)
	}
	if string(data) != "secret\n" {
		t.Fatalf("outside file should be unchanged, got %q", string(data))
	}
}

func TestEditToolEmptyWorkspaceAllowsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool("")
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "world",
		"new_string": "universe",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "hello universe\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestEditToolPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)
	path := filepath.Join(dir, "script.sh")

	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"old_string": "echo hello",
		"new_string": "echo world",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("expected mode 0755, got %04o", info.Mode().Perm())
	}
}
