package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuntimeToolsShareWorkspace documents the single-tenant boundary: every
// caller of one process uses the same workspace, so a fact written by the
// remember tool is readable by read_file. Run one instance per tenant.
func TestRuntimeToolsShareWorkspace(t *testing.T) {
	dir := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "system-prompt.txt"), []byte("test"), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"),
		[]byte("tools:\n  allowed: [remember, read_file]\n"), 0o600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}

	cfg := Config{ConfigDir: dir, Workspace: workspace}
	rt, err := buildRuntime(context.Background(), cfg, runtimeOptions{Provider: &fakeProvider{}})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() {
		for i := len(rt.closers) - 1; i >= 0; i-- {
			_ = rt.closers[i]()
		}
	})

	remember, ok := rt.Registry.Get("remember")
	if !ok {
		t.Fatal("remember tool not registered")
	}
	if res := remember.Execute(context.Background(), map[string]any{"entry": "the number 7"}); res.Error {
		t.Fatalf("remember failed: %s", res.Output)
	}

	readFile, ok := rt.Registry.Get("read_file")
	if !ok {
		t.Fatal("read_file tool not registered")
	}
	res := readFile.Execute(context.Background(), map[string]any{
		"path": filepath.Join(workspace, "MEMORY.md"),
	})
	if res.Error {
		t.Fatalf("read_file failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "the number 7") {
		t.Fatalf("expected shared workspace memory, got: %s", res.Output)
	}
}
