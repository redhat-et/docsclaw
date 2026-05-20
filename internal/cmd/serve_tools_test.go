package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadToolsJSON(t *testing.T) {
	t.Run("valid tools.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "tools.json")
		content := `{
  "manifestVersion": "1.0.0",
  "agentName": "test-agent",
  "base": "registry.access.redhat.com/ubi9-minimal",
  "highestTier": "tier-3",
  "riskScore": 30,
  "tools": [
    {
      "name": "curl",
      "package": "curl",
      "tier": "tier-1",
      "risk": {
        "score": 10,
        "codeExecution": false,
        "networkCapable": true
      }
    },
    {
      "name": "jq",
      "package": "jq",
      "tier": "tier-1",
      "risk": {
        "score": 5,
        "codeExecution": false,
        "networkCapable": false
      }
    }
  ]
}`
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		tj, err := loadToolsJSON(path)
		if err != nil {
			t.Fatalf("loadToolsJSON failed: %v", err)
		}
		if tj == nil {
			t.Fatal("expected ToolsJSON, got nil")
			return
		}
		if tj.AgentName != "test-agent" {
			t.Errorf("expected agent name 'test-agent', got %q", tj.AgentName)
		}
		if len(tj.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(tj.Tools))
		}
		if tj.RiskScore != 30 {
			t.Errorf("expected risk score 30, got %d", tj.RiskScore)
		}
		if tj.Tools[0].Name != "curl" {
			t.Errorf("expected first tool 'curl', got %q", tj.Tools[0].Name)
		}
	})

	t.Run("missing file returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")

		tj, err := loadToolsJSON(path)
		if err != nil {
			t.Fatalf("expected nil error for missing file, got: %v", err)
		}
		if tj != nil {
			t.Fatalf("expected nil ToolsJSON for missing file, got: %+v", tj)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "invalid.json")
		if err := os.WriteFile(path, []byte("{invalid json"), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		tj, err := loadToolsJSON(path)
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
		if tj != nil {
			t.Fatalf("expected nil ToolsJSON for invalid JSON, got: %+v", tj)
		}
	})
}
