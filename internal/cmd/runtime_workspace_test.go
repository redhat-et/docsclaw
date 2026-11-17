package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/inboundauth"
)

// TestSingleTenantMemorySharedAcrossCallers documents the single-tenant
// boundary: two authenticated callers with different subjects use the same
// workspace, so a fact written by one is readable by the other. Run one
// instance per tenant; isolation between tenants belongs to the
// container/OpenShell layer.
func TestSingleTenantMemorySharedAcrossCallers(t *testing.T) {
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

	alice := inboundauth.ContextWithClaims(context.Background(), inboundauth.Claims{Subject: "alice"})
	bob := inboundauth.ContextWithClaims(context.Background(), inboundauth.Claims{Subject: "bob"})

	remember, ok := rt.Registry.Get("remember")
	if !ok {
		t.Fatal("remember tool not registered")
	}
	if res := remember.Execute(alice, map[string]any{"entry": "the number 7"}); res.Error {
		t.Fatalf("remember failed: %s", res.Output)
	}

	readFile, ok := rt.Registry.Get("read_file")
	if !ok {
		t.Fatal("read_file tool not registered")
	}
	res := readFile.Execute(bob, map[string]any{
		"path": filepath.Join(workspace, "MEMORY.md"),
	})
	if res.Error {
		t.Fatalf("read_file failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "the number 7") {
		t.Fatalf("expected shared workspace memory, got: %s", res.Output)
	}
}
