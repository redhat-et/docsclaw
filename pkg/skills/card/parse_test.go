package card

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, "skill.yaml")

	content := `apiVersion: skills.agent.io/v1alpha1
kind: SkillCard
metadata:
  name: test-skill
  namespace: example
  ref: ghcr.io/example/test-skill:v1.0.0
  version: v1.0.0
  description: A test skill for validation
  author: Test Author
  license: Apache-2.0
  metadata:
    category: testing
spec:
  tools:
    required:
      - file-read
      - file-write
    optional:
      - web-fetch
  allowedTools: "file-read,file-write,web-fetch"
  resources:
    estimatedMemory: 128Mi
    estimatedCPU: 100m
  compatibility:
    minAgentVersion: v1.0.0
    environment: kubernetes
`

	if err := os.WriteFile(skillPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	sc, err := Parse(skillPath)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if sc.Metadata.Name != "test-skill" {
		t.Errorf("expected name test-skill, got %s", sc.Metadata.Name)
	}

	if sc.Metadata.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", sc.Metadata.Version)
	}

	if len(sc.Spec.Tools.Required) != 2 {
		t.Errorf("expected 2 required tools, got %d", len(sc.Spec.Tools.Required))
	}

	if sc.Spec.Resources.EstimatedMemory != "128Mi" {
		t.Errorf("expected estimatedMemory 128Mi, got %s", sc.Spec.Resources.EstimatedMemory)
	}
}

func TestValidateValid(t *testing.T) {
	sc := SkillCard{
		APIVersion: "skills.agent.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: SkillCardMeta{
			Name:        "valid-skill",
			Namespace:   "example",
			Ref:         "ghcr.io/example/valid-skill:v1.0.0",
			Version:     "v1.0.0",
			Description: "A valid skill card",
			Author:      "Test Author",
		},
	}

	if err := Validate(sc); err != nil {
		t.Errorf("Validate failed for valid SkillCard: %v", err)
	}
}

func TestValidateMissingName(t *testing.T) {
	sc := SkillCard{
		APIVersion: "skills.agent.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: SkillCardMeta{
			Name:        "", // Missing name
			Namespace:   "example",
			Ref:         "ghcr.io/example/skill:v1.0.0",
			Version:     "v1.0.0",
			Description: "A skill card",
			Author:      "Test Author",
		},
	}

	err := Validate(sc)
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}

	if !strings.Contains(err.Error(), "metadata.name is required") {
		t.Errorf("expected 'metadata.name is required' error, got: %v", err)
	}
}

func TestValidateBadName(t *testing.T) {
	tests := []struct {
		name      string
		skillName string
		wantErr   bool
	}{
		{
			name:      "uppercase",
			skillName: "InvalidName",
			wantErr:   true,
		},
		{
			name:      "starts-with-hyphen",
			skillName: "-invalid",
			wantErr:   true,
		},
		{
			name:      "ends-with-hyphen",
			skillName: "invalid-",
			wantErr:   true,
		},
		{
			name:      "consecutive-hyphens",
			skillName: "invalid--name",
			wantErr:   true,
		},
		{
			name:      "too-long",
			skillName: "a123456789012345678901234567890123456789012345678901234567890123456",
			wantErr:   true,
		},
		{
			name:      "valid",
			skillName: "valid-skill-name-123",
			wantErr:   false,
		},
		{
			name:      "single-char",
			skillName: "a",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := SkillCard{
				APIVersion: "skills.agent.io/v1alpha1",
				Kind:       "SkillCard",
				Metadata: SkillCardMeta{
					Name:        tt.skillName,
					Namespace:   "example",
					Ref:         "ghcr.io/example/skill:v1.0.0",
					Version:     "v1.0.0",
					Description: "A skill card",
					Author:      "Test Author",
				},
			}

			err := Validate(sc)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for name %q, got nil", tt.skillName)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error for name %q, got: %v", tt.skillName, err)
			}
		})
	}
}
