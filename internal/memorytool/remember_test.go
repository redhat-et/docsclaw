package memorytool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/memory"
)

func TestRememberToolRequiresEntry(t *testing.T) {
	dir := t.TempDir()
	store := memory.NewFileStore(filepath.Join(dir, "MEMORY.md"))
	tool := NewRememberTool(store)

	result := tool.Execute(context.Background(), map[string]any{})
	if !result.Error {
		t.Fatal("expected error for missing entry")
	}
	if !strings.Contains(result.Output, "entry is required") {
		t.Fatalf("expected entry required message, got %q", result.Output)
	}
}

func TestRememberToolAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	store := memory.NewFileStore(path)
	tool := NewRememberTool(store)

	result := tool.Execute(context.Background(), map[string]any{
		"entry": "User prefers Go over Python.",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("memory file should exist: %v", err)
	}
	if !strings.Contains(string(data), "User prefers Go over Python.") {
		t.Fatalf("expected entry in memory, got %q", string(data))
	}
}
