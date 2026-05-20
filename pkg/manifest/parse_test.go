package manifest

import "testing"

func TestParseValid(t *testing.T) {
	m, err := ParseFile("../../testdata/manifest/nps-agent.yaml")
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if m.Metadata.Name != "nps-assistant" {
		t.Errorf("name = %q, want nps-assistant", m.Metadata.Name)
	}
	if m.Spec.Base.Image == "" {
		t.Error("base image is empty")
	}
	if len(m.Spec.Tools) != 3 {
		t.Errorf("tools count = %d, want 3", len(m.Spec.Tools))
	}
	if m.Spec.Prompt.Text == "" {
		t.Error("prompt text is empty")
	}
	if len(m.Spec.Skills) != 1 {
		t.Errorf("skills count = %d, want 1", len(m.Spec.Skills))
	}
	if len(m.Spec.Secrets) != 2 {
		t.Errorf("secrets count = %d, want 2", len(m.Spec.Secrets))
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := ParseFile("../../testdata/manifest/bad-manifest.yaml")
	if err == nil {
		t.Fatal("expected validation error for bad manifest")
	}
}

func TestValidate_MissingName(t *testing.T) {
	m := AgentManifest{
		APIVersion: "docsclaw.io/v1alpha1",
		Kind:       "AgentManifest",
		Spec: ManifestSpec{
			Base:   BaseImage{Image: "some-image"},
			Tools:  []string{"curl"},
			Prompt: PromptConfig{Text: "hello"},
		},
	}
	err := Validate(m)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidate_MissingBaseImage(t *testing.T) {
	m := AgentManifest{
		APIVersion: "docsclaw.io/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   ManifestMeta{Name: "test"},
		Spec: ManifestSpec{
			Tools:  []string{"curl"},
			Prompt: PromptConfig{Text: "hello"},
		},
	}
	err := Validate(m)
	if err == nil {
		t.Fatal("expected error for missing base image")
	}
}

func TestValidate_MissingPrompt(t *testing.T) {
	m := AgentManifest{
		APIVersion: "docsclaw.io/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   ManifestMeta{Name: "test"},
		Spec: ManifestSpec{
			Base:  BaseImage{Image: "some-image"},
			Tools: []string{"curl"},
		},
	}
	err := Validate(m)
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}
