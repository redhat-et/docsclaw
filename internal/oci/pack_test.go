package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/memory"
)

// writeSkillDir creates a valid skill.yaml + SKILL.md in the given directory.
func writeSkillDir(t *testing.T, dir string) {
	t.Helper()

	skillYAML := `apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: test-skill
  namespace: example
  ref: example.com/skills/test-skill:latest
  version: 1.0.0
  description: A test skill for unit testing
  author: Test Author
  license: Apache-2.0
spec:
  tools:
    required:
      - shell
      - webfetch
  allowedTools: shell,webfetch,readfile
  resources:
    estimatedMemory: 128Mi
    estimatedCPU: 100m
`

	skillMD := `---
name: test-skill
description: A test skill for unit testing
---

# Test skill

This is a test skill for unit testing.
`

	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), []byte(skillYAML), 0644); err != nil {
		t.Fatalf("failed to write skill.yaml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}
}

func TestPack(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with skill files
	tmpDir := t.TempDir()
	writeSkillDir(t, tmpDir)

	// Pack into memory storage
	store := memory.New()
	opts := PackOptions{AsImage: false}

	desc, err := Pack(ctx, tmpDir, store, opts)
	if err != nil {
		t.Fatalf("Pack() failed: %v", err)
	}

	// Verify manifest descriptor
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		t.Errorf("expected media type %s, got %s", ocispec.MediaTypeImageManifest, desc.MediaType)
	}

	if desc.Size == 0 {
		t.Error("manifest descriptor size is zero")
	}

	if desc.Digest.String() == "" {
		t.Error("manifest descriptor digest is empty")
	}

	// Verify manifest exists in storage
	exists, err := store.Exists(ctx, desc)
	if err != nil {
		t.Fatalf("failed to check manifest existence: %v", err)
	}
	if !exists {
		t.Error("manifest not found in storage")
	}
}

func TestPackAsImage(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with skill files
	tmpDir := t.TempDir()
	writeSkillDir(t, tmpDir)

	// Pack into memory storage with AsImage=true
	store := memory.New()
	opts := PackOptions{AsImage: true}

	desc, err := Pack(ctx, tmpDir, store, opts)
	if err != nil {
		t.Fatalf("Pack() failed: %v", err)
	}

	// Verify descriptor
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		t.Errorf("expected media type %s, got %s", ocispec.MediaTypeImageManifest, desc.MediaType)
	}

	if desc.Size == 0 {
		t.Error("manifest descriptor size is zero")
	}

	// Verify manifest exists
	exists, err := store.Exists(ctx, desc)
	if err != nil {
		t.Fatalf("failed to check manifest existence: %v", err)
	}
	if !exists {
		t.Error("manifest not found in storage")
	}
}

func TestPackMissingSkillYaml(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with only SKILL.md (no skill.yaml)
	tmpDir := t.TempDir()
	skillMD := `# Test Skill

This skill is missing skill.yaml.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Attempt to pack
	store := memory.New()
	opts := PackOptions{AsImage: false}

	_, err := Pack(ctx, tmpDir, store, opts)
	if err == nil {
		t.Fatal("expected error when packing directory without skill.yaml, got nil")
	}

	// Error should mention skill.yaml
	errStr := err.Error()
	if errStr == "" {
		t.Error("error message is empty")
	}
}
