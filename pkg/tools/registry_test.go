package tools

import (
	"context"
	"testing"
)

// mockTool implements Tool for testing.
type mockTool struct {
	name   string
	output string
}

func (m *mockTool) Name() string               { return m.name }
func (m *mockTool) Description() string        { return "mock tool" }
func (m *mockTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (m *mockTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	return &ToolResult{Output: m.output}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "test_tool", output: "ok"})

	tool, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("expected tool to be registered")
	}
	if tool.Name() != "test_tool" {
		t.Fatalf("expected test_tool, got %q", tool.Name())
	}
}

func TestRegistryAllowedFilter(t *testing.T) {
	r := NewRegistry([]string{"allowed_tool"})
	r.Register(&mockTool{name: "allowed_tool"})
	r.Register(&mockTool{name: "blocked_tool"})

	if _, ok := r.Get("allowed_tool"); !ok {
		t.Fatal("allowed_tool should be accessible")
	}
	if _, ok := r.Get("blocked_tool"); ok {
		t.Fatal("blocked_tool should be filtered out")
	}
}

func TestRegistryDefinitions(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "tool_a"})
	r.Register(&mockTool{name: "tool_b"})

	defs := r.Definitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(defs))
	}
}

func TestRegistryAlwaysAllowed(t *testing.T) {
	r := NewRegistry([]string{"only_this"})
	r.Register(&mockTool{name: "only_this"})
	r.RegisterAlwaysAllowed(&mockTool{name: "load_skill"})

	if _, ok := r.Get("load_skill"); !ok {
		t.Fatal("load_skill should bypass allowed filter")
	}
	if _, ok := r.Get("only_this"); !ok {
		t.Fatal("only_this should be accessible")
	}
}

func TestRegistryAliasLookup(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "read_file"})
	if err := r.RegisterAlias("read", "read_file"); err != nil {
		t.Fatalf("RegisterAlias failed: %v", err)
	}

	tool, ok := r.Get("read")
	if !ok {
		t.Fatal("expected alias read to resolve to read_file")
	}
	if tool.Name() != "read_file" {
		t.Fatalf("expected read_file, got %q", tool.Name())
	}
}

func TestRegistryDefinitionsExcludeAliases(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "read_file"})
	if err := r.RegisterAlias("read", "read_file"); err != nil {
		t.Fatalf("RegisterAlias failed: %v", err)
	}

	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "read_file" {
		t.Fatalf("expected read_file, got %q", defs[0].Name)
	}
}

func TestRegistryAllowedToolsWithAlias(t *testing.T) {
	r := NewRegistry([]string{"read_file"})
	r.Register(&mockTool{name: "read_file"})
	r.Register(&mockTool{name: "write_file"})
	if err := r.RegisterAlias("read", "read_file"); err != nil {
		t.Fatalf("RegisterAlias read failed: %v", err)
	}
	if err := r.RegisterAlias("write", "write_file"); err != nil {
		t.Fatalf("RegisterAlias write failed: %v", err)
	}

	if _, ok := r.Get("read"); !ok {
		t.Fatal("alias read should be allowed because read_file is allowed")
	}
	if _, ok := r.Get("write"); ok {
		t.Fatal("alias write should be blocked because write_file is not allowed")
	}
}

func TestRegistryAllowedToolsUsesCanonicalName(t *testing.T) {
	r := NewRegistry([]string{"read"})
	r.Register(&mockTool{name: "read_file"})
	if err := r.RegisterAlias("read", "read_file"); err != nil {
		t.Fatalf("RegisterAlias failed: %v", err)
	}

	// Definitions only include tools whose canonical name is allowed.
	defs := r.Definitions()
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions because canonical name read_file is not allowed, got %d", len(defs))
	}

	// Get("read") resolves to read_file before the allowed filter, so it is
	// blocked because read_file is not in allowedTools.
	if _, ok := r.Get("read"); ok {
		t.Fatal("expected Get(read) to be false because canonical name read_file is not allowed")
	}
}

func TestRegisterAliasValidation(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "existing_tool"})

	if err := r.RegisterAlias("", "read_file"); err == nil {
		t.Fatal("expected error for empty alias")
	}
	if err := r.RegisterAlias("read", ""); err == nil {
		t.Fatal("expected error for empty canonical")
	}
	if err := r.RegisterAlias("existing_tool", "other"); err == nil {
		t.Fatal("expected error when alias conflicts with existing tool name")
	}
}
