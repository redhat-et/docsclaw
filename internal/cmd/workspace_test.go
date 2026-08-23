package cmd

import (
	"testing"
)

func TestResolveWorkspaceConfigWins(t *testing.T) {
	result := resolveWorkspace("/from-config", "/from-flag")
	if result != "/from-config" {
		t.Fatalf("expected /from-config, got %q", result)
	}
}

func TestResolveWorkspaceFlagFallback(t *testing.T) {
	result := resolveWorkspace("", "/from-flag")
	if result != "/from-flag" {
		t.Fatalf("expected /from-flag, got %q", result)
	}
}

func TestResolveWorkspaceDefault(t *testing.T) {
	result := resolveWorkspace("", "")
	if result != defaultWorkspace {
		t.Fatalf("expected %q, got %q", defaultWorkspace, result)
	}
}

func TestResolveWorkspaceWhitespaceIgnored(t *testing.T) {
	result := resolveWorkspace("   ", "/from-flag")
	if result != "/from-flag" {
		t.Fatalf("expected /from-flag, got %q", result)
	}
}
