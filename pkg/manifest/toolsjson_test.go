package manifest

import (
	"encoding/json"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

func TestGenerateToolsJSON(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "test-agent", Version: "1.0.0"},
		Spec: ManifestSpec{
			Base: BaseImage{Image: "hi/core-runtime:latest"},
			Tools: []string{"curl", "jq", "git"},
		},
	}
	cat, _ := catalog.LoadDefault()

	data, err := GenerateToolsJSON(m, cat)
	if err != nil {
		t.Fatalf("GenerateToolsJSON() error: %v", err)
	}

	var tj ToolsJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if tj.AgentName != "test-agent" {
		t.Errorf("agentName = %q, want test-agent", tj.AgentName)
	}
	if tj.HighestTier != "standard" {
		t.Errorf("highestTier = %q, want standard", tj.HighestTier)
	}
	if len(tj.Tools) != 3 {
		t.Errorf("tools count = %d, want 3", len(tj.Tools))
	}

	found := false
	for _, tool := range tj.Tools {
		if tool.Name == "git" {
			found = true
			if tool.Tier != "standard" {
				t.Errorf("git tier = %q, want standard", tool.Tier)
			}
		}
	}
	if !found {
		t.Error("git not in tools list")
	}
}
