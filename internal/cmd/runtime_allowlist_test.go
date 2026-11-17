package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRuntimeOmittedAllowlistEnablesNoTools(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "system-prompt.txt"), []byte("test"), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"), []byte("tools: {}\n"), 0o600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}

	cfg := Config{ConfigDir: dir, Workspace: t.TempDir()}
	rt, err := buildRuntime(context.Background(), cfg, runtimeOptions{Provider: &fakeProvider{}})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() {
		for i := len(rt.closers) - 1; i >= 0; i-- {
			_ = rt.closers[i]()
		}
	})

	enabled := make(map[string]bool)
	for _, def := range rt.Registry.Definitions() {
		enabled[def.Name] = true
	}
	for _, name := range []string{"exec", "write_file", "read_file", "remember"} {
		if enabled[name] {
			t.Errorf("tool %q is enabled with an omitted allowlist", name)
		}
	}
}
