package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/content/memory"
)

func TestPushAndPull(t *testing.T) {
	ctx := context.Background()

	// 1. Create a skill directory
	skillDir := t.TempDir()
	writeSkillDir(t, skillDir)

	// 2. Use memory.New() as the registry
	registry := memory.New()

	// 3. Push with ref
	ref := "localhost:5000/test/skill-test-skill:1.0.0"
	pushOpts := PushOptions{
		AsImage:  false,
		Registry: registry,
	}

	err := Push(ctx, skillDir, ref, pushOpts)
	if err != nil {
		t.Fatalf("Push() failed: %v", err)
	}

	// 4. Pull to a new temp dir
	pullDir := t.TempDir()
	pullOpts := PullOptions{
		Registry: registry,
	}

	err = Pull(ctx, ref, pullDir, pullOpts)
	if err != nil {
		t.Fatalf("Pull() failed: %v", err)
	}

	// 5. Verify test-skill/SKILL.md and test-skill/skill.yaml exist after extraction
	skillMDPath := filepath.Join(pullDir, "test-skill", "SKILL.md")
	skillYAMLPath := filepath.Join(pullDir, "test-skill", "skill.yaml")

	if _, err := os.Stat(skillMDPath); err != nil {
		t.Errorf("expected SKILL.md to exist at %s, got error: %v", skillMDPath, err)
	}

	if _, err := os.Stat(skillYAMLPath); err != nil {
		t.Errorf("expected skill.yaml to exist at %s, got error: %v", skillYAMLPath, err)
	}
}

func TestInspect(t *testing.T) {
	ctx := context.Background()

	// 1. Create a skill directory
	skillDir := t.TempDir()
	writeSkillDir(t, skillDir)

	// 2. Push a skill to memory registry
	registry := memory.New()
	ref := "localhost:5000/test/skill-test-skill:1.0.0"
	pushOpts := PushOptions{
		AsImage:  false,
		Registry: registry,
	}

	err := Push(ctx, skillDir, ref, pushOpts)
	if err != nil {
		t.Fatalf("Push() failed: %v", err)
	}

	// 3. Call Inspect
	inspectOpts := InspectOptions{
		Registry: registry,
	}

	sc, err := Inspect(ctx, ref, inspectOpts)
	if err != nil {
		t.Fatalf("Inspect() failed: %v", err)
	}

	// 4. Verify sc.Metadata.Name == "test-skill" and sc.Metadata.Version == "1.0.0"
	if sc.Metadata.Name != "test-skill" {
		t.Errorf("expected Name=test-skill, got %s", sc.Metadata.Name)
	}

	if sc.Metadata.Version != "1.0.0" {
		t.Errorf("expected Version=1.0.0, got %s", sc.Metadata.Version)
	}
}
