package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestArtifactModeLayerAnnotations(t *testing.T) {
	ctx := context.Background()
	skillDir := t.TempDir()
	writeSkillDir(t, skillDir)

	store := memory.New()
	desc, err := Pack(ctx, skillDir, store, PackOptions{AsImage: false})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Fetch and parse manifest to verify layer annotations.
	manifestData, err := fetchBlob(ctx, store, desc)
	if err != nil {
		t.Fatalf("fetchBlob: %v", err)
	}

	var manifest struct {
		Layers []struct {
			MediaType   string            `json:"mediaType"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Each layer should have a title annotation.
	titles := map[string]bool{}
	for _, layer := range manifest.Layers {
		title := layer.Annotations[AnnotationTitle]
		if title == "" {
			t.Errorf("layer %s has no title annotation", layer.MediaType)
		}
		if layer.MediaType != FileMediaType {
			t.Errorf("layer %s has media type %s, want %s", title, layer.MediaType, FileMediaType)
		}
		titles[title] = true
	}

	if !titles["SKILL.md"] {
		t.Error("no layer with title SKILL.md")
	}
	if !titles["skill.yaml"] {
		t.Error("no layer with title skill.yaml")
	}
}

func TestArtifactModePullExtractsFiles(t *testing.T) {
	ctx := context.Background()
	skillDir := t.TempDir()
	writeSkillDir(t, skillDir)

	reg := memory.New()
	ref := "localhost:5000/test/skill-test-skill:1.0.0"

	if err := Push(ctx, skillDir, ref, PushOptions{Registry: reg}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	pullDir := t.TempDir()
	if err := Pull(ctx, ref, pullDir, PullOptions{Registry: reg}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Verify files exist (Pull creates skill-name subdirectory).
	for _, name := range []string{"SKILL.md", "skill.yaml"} {
		path := filepath.Join(pullDir, "test-skill", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s not found at %s", name, path)
		}
	}

	// Verify content is correct.
	data, err := os.ReadFile(filepath.Join(pullDir, "test-skill", "skill.yaml"))
	if err != nil {
		t.Fatalf("read skill.yaml: %v", err)
	}
	if !strings.Contains(string(data), "name: test-skill") {
		t.Error("skill.yaml does not contain expected content")
	}
}

func TestImageModePushAndPull(t *testing.T) {
	ctx := context.Background()
	skillDir := t.TempDir()
	writeSkillDir(t, skillDir)

	reg := memory.New()
	ref := "localhost:5000/test/skill-test-skill:1.0.0-image"

	if err := Push(ctx, skillDir, ref, PushOptions{AsImage: true, Registry: reg}); err != nil {
		t.Fatalf("Push --as-image: %v", err)
	}

	pullDir := t.TempDir()
	if err := Pull(ctx, ref, pullDir, PullOptions{Registry: reg}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Image mode extracts from tarball. Skill name comes from annotation.
	for _, name := range []string{"SKILL.md", "skill.yaml"} {
		path := filepath.Join(pullDir, "test-skill", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s not found at %s", name, path)
		}
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
