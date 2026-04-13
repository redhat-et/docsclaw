# OCI skill distribution implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement OCI-based skill packaging, signing, and
distribution so that skills can be pushed to registries, verified,
and pulled by agents.

**Architecture:** Three new packages — `pkg/skills/card/` for
SkillCard types and validation, `internal/oci/` for OCI artifact
operations via oras-go, `internal/verify/` for sigstore signature
verification. Five CLI subcommands under `docsclaw skill`. Backward
compatibility in existing `pkg/skills/loader.go`.

**Tech Stack:** oras-go v2 (OCI push/pull), sigstore-go (signature
verification), Cobra (CLI), gopkg.in/yaml.v3 (already a dependency)

**Spec:** `docs/dev/2026-04-12-oci-skill-distribution-design.md`

**Example skills:** `examples/skills/resume-screener/`,
`examples/skills/policy-comparator/`,
`examples/skills/checklist-auditor/`

---

## File structure

| File | Responsibility |
| ---- | -------------- |
| `pkg/skills/card/types.go` | SkillCard struct, nested types |
| `pkg/skills/card/parse.go` | Parse and Validate functions |
| `pkg/skills/card/parse_test.go` | Tests for parsing and validation |
| `internal/oci/media.go` | Media type and annotation constants |
| `internal/oci/pack.go` | Pack skill dir into OCI artifact |
| `internal/oci/pack_test.go` | Tests for packing |
| `internal/oci/push.go` | Push artifact to registry |
| `internal/oci/pull.go` | Pull artifact from registry |
| `internal/oci/push_pull_test.go` | Integration tests (in-memory registry) |
| `internal/oci/inspect.go` | Fetch config + SkillCard without full pull |
| `internal/verify/verify.go` | Signature verification via sigstore-go |
| `internal/verify/policy.go` | Policy types (mode, trusted keys) |
| `internal/verify/verify_test.go` | Tests for verification |
| `internal/cmd/skill.go` | Parent `skill` command + subcommand wiring |
| `internal/cmd/skill_pack.go` | `docsclaw skill pack` |
| `internal/cmd/skill_push.go` | `docsclaw skill push` |
| `internal/cmd/skill_pull.go` | `docsclaw skill pull` |
| `internal/cmd/skill_verify.go` | `docsclaw skill verify` |
| `internal/cmd/skill_inspect.go` | `docsclaw skill inspect` |
| `pkg/skills/loader.go` | Modified: read skill.yaml when present |
| `pkg/skills/loader_test.go` | Modified: test SkillCard-aware discovery |

---

## Task 1: Add oras-go and sigstore-go dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add oras-go dependency**

```bash
cd /Users/panni/work/docsclaw
go get oras.land/oras-go/v2@latest
```

- [ ] **Step 2: Add sigstore-go dependency**

```bash
go get github.com/sigstore/sigstore-go@latest
```

- [ ] **Step 3: Verify dependencies resolve**

```bash
go mod tidy
```

Expected: clean exit, no errors.

- [ ] **Step 4: Verify build still works**

```bash
make build
```

Expected: `bin/docsclaw` binary produced.

- [ ] **Step 5: Run existing tests**

```bash
make test
```

Expected: all existing tests pass.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum
git commit -s -m "chore: add oras-go and sigstore-go dependencies

Add oras.land/oras-go/v2 for OCI artifact push/pull and
github.com/sigstore/sigstore-go for signature verification.

Relates to #4"
```

---

## Task 2: SkillCard types

**Files:**
- Create: `pkg/skills/card/types.go`

- [ ] **Step 1: Create the types file**

```go
// Package card defines the SkillCard schema for OCI-distributed skills.
package card

// SkillCard is the top-level type for skill.yaml files.
type SkillCard struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   SkillCardMeta   `yaml:"metadata"`
	Spec       SkillCardSpec   `yaml:"spec"`
}

// SkillCardMeta holds the skill's identity and descriptive metadata.
type SkillCardMeta struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Ref         string            `yaml:"ref"`
	Version     string            `yaml:"version"`
	Description string            `yaml:"description"`
	Author      string            `yaml:"author"`
	License     string            `yaml:"license,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty"`
}

// SkillCardSpec holds the skill's operational requirements.
type SkillCardSpec struct {
	Tools         ToolDeps         `yaml:"tools,omitempty"`
	AllowedTools  string           `yaml:"allowedTools,omitempty"`
	Dependencies  Dependencies     `yaml:"dependencies,omitempty"`
	Resources     ResourceHints    `yaml:"resources,omitempty"`
	Compatibility Compatibility    `yaml:"compatibility,omitempty"`
}

// ToolDeps declares which tools the skill needs from the agent.
type ToolDeps struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

// Dependencies declares external requirements.
type Dependencies struct {
	Skills    []string      `yaml:"skills,omitempty"`
	ToolPacks []ToolPackRef `yaml:"toolPacks,omitempty"`
}

// ToolPackRef is a reference to a tool pack OCI artifact.
type ToolPackRef struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"ref"`
}

// ResourceHints provides estimated resource usage for quota enforcement.
type ResourceHints struct {
	EstimatedMemory string `yaml:"estimatedMemory,omitempty"`
	EstimatedCPU    string `yaml:"estimatedCPU,omitempty"`
}

// Compatibility declares version and environment constraints.
type Compatibility struct {
	MinAgentVersion string `yaml:"minAgentVersion,omitempty"`
	Environment     string `yaml:"environment,omitempty"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./pkg/skills/card/
```

Expected: clean exit.

- [ ] **Step 3: Commit**

```bash
git add pkg/skills/card/types.go
git commit -s -m "feat(card): add SkillCard types for skill.yaml schema

Defines SkillCard, SkillCardMeta, SkillCardSpec, ToolDeps,
Dependencies, ResourceHints, and Compatibility types.

Relates to #4"
```

---

## Task 3: SkillCard parsing and validation

**Files:**
- Create: `pkg/skills/card/parse.go`
- Create: `pkg/skills/card/parse_test.go`

- [ ] **Step 1: Write the failing test for Parse**

Create `pkg/skills/card/parse_test.go`:

```go
package card

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseValid(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: resume-screener
  namespace: official
  ref: quay.io/docsclaw/official/skill-resume-screener
  version: 1.0.0
  description: Screen resumes against a job description.
  author: Red Hat ET
  license: Apache-2.0
spec:
  tools:
    required: [read_file]
    optional: [write_file]
  resources:
    estimatedMemory: 32Mi
    estimatedCPU: 100m
  compatibility:
    minAgentVersion: "0.5.0"
`
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sc, err := Parse(filepath.Join(dir, "skill.yaml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sc.Metadata.Name != "resume-screener" {
		t.Errorf("Name = %q, want %q", sc.Metadata.Name, "resume-screener")
	}
	if sc.Metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", sc.Metadata.Version, "1.0.0")
	}
	if len(sc.Spec.Tools.Required) != 1 || sc.Spec.Tools.Required[0] != "read_file" {
		t.Errorf("Tools.Required = %v, want [read_file]", sc.Spec.Tools.Required)
	}
	if sc.Spec.Resources.EstimatedMemory != "32Mi" {
		t.Errorf("EstimatedMemory = %q, want %q", sc.Spec.Resources.EstimatedMemory, "32Mi")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./pkg/skills/card/ -run TestParseValid -v
```

Expected: FAIL — `Parse` undefined.

- [ ] **Step 3: Write the failing tests for Validate**

Append to `pkg/skills/card/parse_test.go`:

```go
func TestValidateValid(t *testing.T) {
	sc := SkillCard{
		APIVersion: "docsclaw.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: SkillCardMeta{
			Name:        "resume-screener",
			Namespace:   "official",
			Ref:         "quay.io/docsclaw/official/skill-resume-screener",
			Version:     "1.0.0",
			Description: "Screen resumes.",
			Author:      "Red Hat ET",
		},
	}
	if err := Validate(sc); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateMissingName(t *testing.T) {
	sc := SkillCard{
		APIVersion: "docsclaw.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: SkillCardMeta{
			Namespace:   "official",
			Ref:         "quay.io/docsclaw/official/skill-test",
			Version:     "1.0.0",
			Description: "Test.",
			Author:      "Test",
		},
	}
	if err := Validate(sc); err == nil {
		t.Error("Validate: expected error for missing name")
	}
}

func TestValidateBadName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"uppercase", "Resume-Screener", true},
		{"starts with hyphen", "-screener", true},
		{"ends with hyphen", "screener-", true},
		{"consecutive hyphens", "resume--screener", true},
		{"too long", string(make([]byte, 65)), true},
		{"valid", "resume-screener", false},
		{"single char", "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := SkillCard{
				APIVersion: "docsclaw.io/v1alpha1",
				Kind:       "SkillCard",
				Metadata: SkillCardMeta{
					Name:        tt.input,
					Namespace:   "official",
					Ref:         "quay.io/test",
					Version:     "1.0.0",
					Description: "Test.",
					Author:      "Test",
				},
			}
			err := Validate(sc)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 4: Implement Parse and Validate**

Create `pkg/skills/card/parse.go`:

```go
package card

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// namePattern enforces Agent Skills spec naming rules:
// lowercase, hyphens, 1-64 chars, no leading/trailing/consecutive hyphens.
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// Parse reads and validates a skill.yaml file.
func Parse(path string) (SkillCard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillCard{}, fmt.Errorf("read skill.yaml: %w", err)
	}

	var sc SkillCard
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return SkillCard{}, fmt.Errorf("parse skill.yaml: %w", err)
	}

	if err := Validate(sc); err != nil {
		return SkillCard{}, err
	}

	return sc, nil
}

// Validate checks that all required fields are present and well-formed.
func Validate(sc SkillCard) error {
	if sc.APIVersion == "" {
		return fmt.Errorf("validate: apiVersion is required")
	}
	if sc.Kind != "SkillCard" {
		return fmt.Errorf("validate: kind must be SkillCard, got %q", sc.Kind)
	}

	m := sc.Metadata
	if m.Name == "" {
		return fmt.Errorf("validate: metadata.name is required")
	}
	if len(m.Name) > 64 {
		return fmt.Errorf("validate: metadata.name exceeds 64 characters")
	}
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("validate: metadata.name %q does not match naming rules (lowercase, hyphens, no leading/trailing/consecutive hyphens)", m.Name)
	}
	if m.Namespace == "" {
		return fmt.Errorf("validate: metadata.namespace is required")
	}
	if m.Ref == "" {
		return fmt.Errorf("validate: metadata.ref is required")
	}
	if m.Version == "" {
		return fmt.Errorf("validate: metadata.version is required")
	}
	if m.Description == "" {
		return fmt.Errorf("validate: metadata.description is required")
	}
	if len(m.Description) > 1024 {
		return fmt.Errorf("validate: metadata.description exceeds 1024 characters")
	}
	if m.Author == "" {
		return fmt.Errorf("validate: metadata.author is required")
	}

	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./pkg/skills/card/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Run lint**

```bash
make lint
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add pkg/skills/card/parse.go pkg/skills/card/parse_test.go
git commit -s -m "feat(card): add SkillCard parsing and validation

Parse reads and validates skill.yaml files. Validate enforces Agent
Skills spec naming rules and required field checks.

Relates to #4"
```

---

## Task 4: OCI media type and annotation constants

**Files:**
- Create: `internal/oci/media.go`

- [ ] **Step 1: Create the constants file**

```go
// Package oci provides OCI artifact operations for skill distribution.
package oci

// Media types for skill OCI artifacts.
const (
	// ArtifactType identifies a skill artifact (community spec).
	ArtifactType = "application/vnd.agentskills.skill.v1"

	// ConfigMediaType is the config blob media type (community spec).
	// Used when pushing as a pure OCI artifact.
	ConfigMediaType = "application/vnd.agentskills.skill.config.v1+json"

	// ImageConfigMediaType is the standard OCI image config media type.
	// Used when pushing as a mountable image (--as-image).
	ImageConfigMediaType = "application/vnd.oci.image.config.v1+json"

	// CardMediaType is the SkillCard layer media type (DocsClaw extension).
	CardMediaType = "application/vnd.docsclaw.skill.card.v1+yaml"

	// ContentMediaType is the skill content layer media type (community spec).
	ContentMediaType = "application/vnd.agentskills.skill.content.v1.tar+gzip"
)

// Annotation keys.
const (
	// Standard OCI annotations.
	AnnotationTitle       = "org.opencontainers.image.title"
	AnnotationVersion     = "org.opencontainers.image.version"
	AnnotationDescription = "org.opencontainers.image.description"
	AnnotationLicenses    = "org.opencontainers.image.licenses"
	AnnotationCreated     = "org.opencontainers.image.created"

	// Community annotations.
	AnnotationSkillName = "io.agentskills.skill.name"

	// DocsClaw-specific annotations.
	AnnotationResourcesMemory = "io.docsclaw.skill.resources.memory"
	AnnotationResourcesCPU    = "io.docsclaw.skill.resources.cpu"
	AnnotationToolsRequired   = "io.docsclaw.skill.tools.required"
)
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/oci/
```

Expected: clean exit.

- [ ] **Step 3: Commit**

```bash
git add internal/oci/media.go
git commit -s -m "feat(oci): add media type and annotation constants

Define community-compatible media types (agentskills namespace) and
standard OCI + DocsClaw-specific annotation keys.

Relates to #4"
```

---

## Task 5: Pack skill directory into OCI artifact

**Files:**
- Create: `internal/oci/pack.go`
- Create: `internal/oci/pack_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/oci/pack_test.go`:

```go
package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/content/memory"
)

func writeSkillDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), []byte(`apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: test-skill
  namespace: official
  ref: quay.io/test/skill-test-skill
  version: 1.0.0
  description: A test skill.
  author: Test
spec:
  tools:
    required: [read_file]
  resources:
    estimatedMemory: 32Mi
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: test-skill
description: A test skill.
---

# Test skill

Do the test thing.
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPack(t *testing.T) {
	dir := t.TempDir()
	writeSkillDir(t, dir)

	store := memory.New()
	desc, err := Pack(context.Background(), dir, store, PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if desc.MediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Errorf("MediaType = %q, want OCI manifest", desc.MediaType)
	}
	if desc.Size == 0 {
		t.Error("Descriptor size is 0")
	}
}

func TestPackAsImage(t *testing.T) {
	dir := t.TempDir()
	writeSkillDir(t, dir)

	store := memory.New()
	desc, err := Pack(context.Background(), dir, store, PackOptions{AsImage: true})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if desc.Size == 0 {
		t.Error("Descriptor size is 0")
	}
}

func TestPackMissingSkillYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := memory.New()
	_, err := Pack(context.Background(), dir, store, PackOptions{})
	if err == nil {
		t.Error("Pack: expected error for missing skill.yaml")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/oci/ -run TestPack -v
```

Expected: FAIL — `Pack` undefined.

- [ ] **Step 3: Implement Pack**

Create `internal/oci/pack.go`:

```go
package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/redhat-et/docsclaw/pkg/skills/card"
)

// PackOptions configures how a skill is packaged.
type PackOptions struct {
	// AsImage produces a kubelet-mountable image instead of a pure OCI artifact.
	AsImage bool
}

// Pack reads a skill directory and stores it as an OCI artifact in the given target.
// The directory must contain skill.yaml and SKILL.md.
// Returns the manifest descriptor.
func Pack(ctx context.Context, skillDir string, target content.Storage, opts PackOptions) (ocispec.Descriptor, error) {
	// Parse and validate SkillCard.
	sc, err := card.Parse(filepath.Join(skillDir, "skill.yaml"))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack: %w", err)
	}

	// Build config blob.
	configMediaType := ConfigMediaType
	if opts.AsImage {
		configMediaType = ImageConfigMediaType
	}
	configBlob, err := buildConfig(sc)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack config: %w", err)
	}
	configDesc, err := pushBlob(ctx, target, configMediaType, configBlob)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack config push: %w", err)
	}

	// Layer 0: SkillCard YAML.
	cardData, err := os.ReadFile(filepath.Join(skillDir, "skill.yaml"))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack read skill.yaml: %w", err)
	}
	cardDesc, err := pushBlob(ctx, target, CardMediaType, cardData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack card push: %w", err)
	}

	// Layer 1: skill directory as tar+gzip.
	tarData, err := tarDirectory(skillDir, sc.Metadata.Name)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack tar: %w", err)
	}
	contentDesc, err := pushBlob(ctx, target, ContentMediaType, tarData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack content push: %w", err)
	}

	// Build annotations.
	annotations := map[string]string{
		AnnotationTitle:       sc.Metadata.Name,
		AnnotationVersion:     sc.Metadata.Version,
		AnnotationDescription: sc.Metadata.Description,
		AnnotationSkillName:   sc.Metadata.Name,
		AnnotationCreated:     time.Now().UTC().Format(time.RFC3339),
	}
	if sc.Metadata.License != "" {
		annotations[AnnotationLicenses] = sc.Metadata.License
	}
	if sc.Spec.Resources.EstimatedMemory != "" {
		annotations[AnnotationResourcesMemory] = sc.Spec.Resources.EstimatedMemory
	}
	if sc.Spec.Resources.EstimatedCPU != "" {
		annotations[AnnotationResourcesCPU] = sc.Spec.Resources.EstimatedCPU
	}
	if len(sc.Spec.Tools.Required) > 0 {
		annotations[AnnotationToolsRequired] = strings.Join(sc.Spec.Tools.Required, ",")
	}

	// Build and push manifest.
	manifest := ocispec.Manifest{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageManifest,
		Config:        configDesc,
		Layers:        []ocispec.Descriptor{cardDesc, contentDesc},
		Annotations:   annotations,
	}
	if !opts.AsImage {
		manifest.ArtifactType = ArtifactType
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack marshal manifest: %w", err)
	}

	manifestDesc, err := pushBlob(ctx, target, ocispec.MediaTypeImageManifest, manifestJSON)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack manifest push: %w", err)
	}

	return manifestDesc, nil
}

func buildConfig(sc card.SkillCard) ([]byte, error) {
	cfg := map[string]any{
		"schemaVersion": "1",
		"name":          sc.Metadata.Name,
		"version":       sc.Metadata.Version,
		"description":   sc.Metadata.Description,
	}
	if sc.Metadata.License != "" {
		cfg["license"] = sc.Metadata.License
	}
	if sc.Spec.AllowedTools != "" {
		cfg["allowedTools"] = sc.Spec.AllowedTools
	}
	return json.Marshal(cfg)
}

func pushBlob(ctx context.Context, target content.Storage, mediaType string, data []byte) (ocispec.Descriptor, error) {
	desc := content.NewDescriptorFromBytes(mediaType, data)
	if err := target.Push(ctx, desc, bytes.NewReader(data)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

func tarDirectory(dir, rootName string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(rootName, rel))

		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		// Deterministic: fixed mtime for reproducible digests.
		header.ModTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/oci/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Run lint**

```bash
make lint
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/oci/pack.go internal/oci/pack_test.go
git commit -s -m "feat(oci): implement Pack for skill OCI artifacts

Pack reads a skill directory (skill.yaml + SKILL.md + assets),
validates the SkillCard, and produces an OCI manifest with two
layers (SkillCard YAML + content tarball). Supports --as-image
mode for kubelet-mountable images.

Relates to #4"
```

---

## Task 6: Push and pull operations

**Files:**
- Create: `internal/oci/push.go`
- Create: `internal/oci/pull.go`
- Create: `internal/oci/inspect.go`
- Create: `internal/oci/push_pull_test.go`

- [ ] **Step 1: Write the failing integration test**

Create `internal/oci/push_pull_test.go`:

```go
package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
)

func TestPushAndPull(t *testing.T) {
	// Create skill directory.
	srcDir := t.TempDir()
	writeSkillDir(t, srcDir)

	// Use in-memory registry.
	reg := memory.New()

	ctx := context.Background()
	ref := "localhost:5000/test/skill-test-skill:1.0.0"

	// Push.
	err := Push(ctx, srcDir, ref, PushOptions{Registry: reg})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Pull.
	destDir := t.TempDir()
	err = Pull(ctx, ref, destDir, PullOptions{Registry: reg})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Verify extracted files.
	skillMD := filepath.Join(destDir, "test-skill", "SKILL.md")
	if _, err := os.Stat(skillMD); err != nil {
		t.Errorf("SKILL.md not found after pull: %v", err)
	}
	skillYAML := filepath.Join(destDir, "test-skill", "skill.yaml")
	if _, err := os.Stat(skillYAML); err != nil {
		t.Errorf("skill.yaml not found after pull: %v", err)
	}
}

func TestInspect(t *testing.T) {
	srcDir := t.TempDir()
	writeSkillDir(t, srcDir)

	reg := memory.New()
	ctx := context.Background()
	ref := "localhost:5000/test/skill-test-skill:1.0.0"

	if err := Push(ctx, srcDir, ref, PushOptions{Registry: reg}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	sc, err := Inspect(ctx, ref, InspectOptions{Registry: reg})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if sc.Metadata.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", sc.Metadata.Name, "test-skill")
	}
	if sc.Metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", sc.Metadata.Version, "1.0.0")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/oci/ -run TestPushAndPull -v
```

Expected: FAIL — `Push` undefined.

- [ ] **Step 3: Implement Push**

Create `internal/oci/push.go`:

```go
package oci

import (
	"context"
	"fmt"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

// PushOptions configures the push operation.
type PushOptions struct {
	AsImage bool
	// Registry overrides the default remote registry (for testing).
	Registry oras.Target
}

// Push packs a skill directory and pushes it to an OCI registry.
func Push(ctx context.Context, skillDir, ref string, opts PushOptions) error {
	store := memory.New()

	manifestDesc, err := Pack(ctx, skillDir, store, PackOptions{AsImage: opts.AsImage})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	target, err := resolveTarget(ref, opts.Registry)
	if err != nil {
		return fmt.Errorf("push resolve: %w", err)
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("push parse ref: %w", err)
	}
	tag := parsedRef.Reference

	if err := store.Tag(ctx, manifestDesc, tag); err != nil {
		return fmt.Errorf("push tag: %w", err)
	}

	_, err = oras.Copy(ctx, store, tag, target, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("push copy: %w", err)
	}

	return nil
}

func resolveTarget(ref string, override oras.Target) (oras.Target, error) {
	if override != nil {
		return override, nil
	}
	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return nil, err
	}
	repo, err := remote.NewRepository(parsedRef.Registry + "/" + parsedRef.Repository)
	if err != nil {
		return nil, err
	}
	return repo, nil
}
```

- [ ] **Step 4: Implement Pull**

Create `internal/oci/pull.go`:

```go
package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
)

// PullOptions configures the pull operation.
type PullOptions struct {
	// Registry overrides the default remote registry (for testing).
	Registry oras.Target
}

// Pull fetches a skill artifact from a registry and extracts it to destDir.
func Pull(ctx context.Context, ref, destDir string, opts PullOptions) error {
	target, err := resolveTarget(ref, opts.Registry)
	if err != nil {
		return fmt.Errorf("pull resolve: %w", err)
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("pull parse ref: %w", err)
	}
	tag := parsedRef.Reference

	store := memory.New()
	desc, err := oras.Copy(ctx, target, tag, store, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("pull copy: %w", err)
	}

	// Read manifest to find the content layer.
	manifestReader, err := store.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("pull fetch manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestData, err := io.ReadAll(manifestReader)
	if err != nil {
		return fmt.Errorf("pull read manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("pull parse manifest: %w", err)
	}

	// Find the content layer (tar+gzip).
	for _, layer := range manifest.Layers {
		if layer.MediaType == ContentMediaType {
			layerReader, err := store.Fetch(ctx, layer)
			if err != nil {
				return fmt.Errorf("pull fetch content layer: %w", err)
			}
			defer layerReader.Close()

			if err := extractTarGzip(layerReader, destDir); err != nil {
				return fmt.Errorf("pull extract: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("pull: no content layer found in manifest")
}

func extractTarGzip(r io.Reader, destDir string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Prevent path traversal.
		cleanName := filepath.Clean(header.Name)
		if strings.Contains(cleanName, "..") {
			return fmt.Errorf("path traversal detected: %s", header.Name)
		}

		target := filepath.Join(destDir, cleanName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
```

- [ ] **Step 5: Implement Inspect**

Create `internal/oci/inspect.go`:

```go
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"

	"github.com/redhat-et/docsclaw/pkg/skills/card"

	"gopkg.in/yaml.v3"
)

// InspectOptions configures the inspect operation.
type InspectOptions struct {
	// Registry overrides the default remote registry (for testing).
	Registry oras.Target
}

// Inspect fetches only the SkillCard layer from a registry without
// pulling the full content. Returns the parsed SkillCard.
func Inspect(ctx context.Context, ref string, opts InspectOptions) (card.SkillCard, error) {
	target, err := resolveTarget(ref, opts.Registry)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("inspect resolve: %w", err)
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("inspect parse ref: %w", err)
	}
	tag := parsedRef.Reference

	store := memory.New()
	desc, err := oras.Copy(ctx, target, tag, store, tag, oras.DefaultCopyOptions)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("inspect copy: %w", err)
	}

	// Read manifest.
	manifestReader, err := store.Fetch(ctx, desc)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("inspect fetch manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestData, err := io.ReadAll(manifestReader)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("inspect read manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return card.SkillCard{}, fmt.Errorf("inspect parse manifest: %w", err)
	}

	// Find the SkillCard layer.
	for _, layer := range manifest.Layers {
		if layer.MediaType == CardMediaType {
			cardReader, err := store.Fetch(ctx, layer)
			if err != nil {
				return card.SkillCard{}, fmt.Errorf("inspect fetch card: %w", err)
			}
			defer cardReader.Close()

			cardData, err := io.ReadAll(cardReader)
			if err != nil {
				return card.SkillCard{}, fmt.Errorf("inspect read card: %w", err)
			}

			var sc card.SkillCard
			if err := yaml.Unmarshal(cardData, &sc); err != nil {
				return card.SkillCard{}, fmt.Errorf("inspect parse card: %w", err)
			}
			return sc, nil
		}
	}

	return card.SkillCard{}, fmt.Errorf("inspect: no SkillCard layer found")
}
```

- [ ] **Step 6: Run all tests**

```bash
go test ./internal/oci/ -v
```

Expected: all tests PASS.

- [ ] **Step 7: Run lint**

```bash
make lint
```

Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add internal/oci/push.go internal/oci/pull.go internal/oci/inspect.go internal/oci/push_pull_test.go
git commit -s -m "feat(oci): implement Push, Pull, and Inspect operations

Push packs and copies a skill artifact to an OCI registry. Pull
fetches and extracts the content layer. Inspect fetches only the
SkillCard layer for metadata-only queries. All operations use
oras-go with in-memory registry support for testing.

Relates to #4"
```

---

## Task 7: Signature verification

**Files:**
- Create: `internal/verify/policy.go`
- Create: `internal/verify/verify.go`
- Create: `internal/verify/verify_test.go`

- [ ] **Step 1: Create the Policy types**

Create `internal/verify/policy.go`:

```go
// Package verify provides signature verification for OCI skill artifacts.
package verify

// Mode controls verification behavior.
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeWarn    Mode = "warn"
	ModeSkip    Mode = "skip"
)

// Policy defines trust rules for signature verification.
type Policy struct {
	Mode       Mode
	PublicKey  string // path to cosign public key file
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/verify/verify_test.go`:

```go
package verify

import (
	"context"
	"testing"
)

func TestVerifySkipMode(t *testing.T) {
	policy := Policy{Mode: ModeSkip}
	err := Verify(context.Background(), "quay.io/test/skill:1.0.0", policy)
	if err != nil {
		t.Errorf("Verify with skip mode: %v", err)
	}
}

func TestVerifyEnforceNoKey(t *testing.T) {
	policy := Policy{Mode: ModeEnforce}
	err := Verify(context.Background(), "quay.io/test/skill:1.0.0", policy)
	if err == nil {
		t.Error("Verify with enforce mode and no key: expected error")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/verify/ -v
```

Expected: FAIL — `Verify` undefined.

- [ ] **Step 4: Implement Verify**

Create `internal/verify/verify.go`:

```go
package verify

import (
	"context"
	"fmt"
	"log/slog"
)

// Verify checks the cosign signature of an OCI artifact against the given policy.
// For the PoC, this implements key-based verification.
func Verify(ctx context.Context, ref string, policy Policy) error {
	switch policy.Mode {
	case ModeSkip:
		slog.Debug("signature verification skipped", "ref", ref)
		return nil

	case ModeWarn:
		if policy.PublicKey == "" {
			slog.Warn("no public key configured, skipping verification", "ref", ref)
			return nil
		}
		if err := verifySignature(ctx, ref, policy.PublicKey); err != nil {
			slog.Warn("signature verification failed", "ref", ref, "error", err)
			return nil
		}
		slog.Info("signature verified", "ref", ref)
		return nil

	case ModeEnforce:
		if policy.PublicKey == "" {
			return fmt.Errorf("verify %s: enforce mode requires a public key", ref)
		}
		if err := verifySignature(ctx, ref, policy.PublicKey); err != nil {
			return fmt.Errorf("verify %s: signature verification failed: %w", ref, err)
		}
		slog.Info("signature verified", "ref", ref)
		return nil

	default:
		return fmt.Errorf("verify: unknown mode %q", policy.Mode)
	}
}

// verifySignature performs the actual sigstore verification.
// TODO(#4): Integrate sigstore-go for real cosign verification.
// For the initial PoC, this is a placeholder that will be replaced
// with actual sigstore-go calls once we have signing infrastructure.
func verifySignature(ctx context.Context, ref, publicKeyPath string) error {
	return fmt.Errorf("sigstore verification not yet implemented (ref=%s, key=%s)", ref, publicKeyPath)
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/verify/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/verify/
git commit -s -m "feat(verify): add signature verification with policy modes

Implements enforce/warn/skip verification modes. Sigstore-go
integration is stubbed for the initial PoC — the policy
enforcement logic is fully tested, actual signature checking
will be wired in a follow-up.

Relates to #4"
```

---

## Task 8: CLI skill command group

**Files:**
- Create: `internal/cmd/skill.go`
- Create: `internal/cmd/skill_pack.go`
- Create: `internal/cmd/skill_push.go`
- Create: `internal/cmd/skill_pull.go`
- Create: `internal/cmd/skill_verify.go`
- Create: `internal/cmd/skill_inspect.go`

- [ ] **Step 1: Create the parent skill command**

Create `internal/cmd/skill.go`:

```go
package cmd

import "github.com/spf13/cobra"

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage OCI-distributed skills",
	Long:  "Package, push, pull, verify, and inspect OCI-distributed skills.",
}

func init() {
	rootCmd.AddCommand(skillCmd)
}
```

- [ ] **Step 2: Create skill pack command**

Create `internal/cmd/skill_pack.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/content/oci"

	ociops "github.com/redhat-et/docsclaw/internal/oci"
)

var skillPackCmd = &cobra.Command{
	Use:   "pack <skill-dir>",
	Short: "Package a skill directory into a local OCI layout",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillPack,
}

func init() {
	skillCmd.AddCommand(skillPackCmd)
	skillPackCmd.Flags().Bool("as-image", false, "produce a kubelet-mountable image")
	skillPackCmd.Flags().StringP("output", "o", "", "output directory for OCI layout (default: <skill-dir>/oci-layout)")
}

func runSkillPack(cmd *cobra.Command, args []string) error {
	skillDir := args[0]
	asImage, _ := cmd.Flags().GetBool("as-image")
	outputDir, _ := cmd.Flags().GetString("output")

	if outputDir == "" {
		outputDir = skillDir + "/oci-layout"
	}

	store, err := oci.New(outputDir)
	if err != nil {
		return fmt.Errorf("create OCI layout: %w", err)
	}

	desc, err := ociops.Pack(context.Background(), skillDir, store, ociops.PackOptions{
		AsImage: asImage,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Packed skill to %s\n", outputDir)
	fmt.Printf("Digest: %s\n", desc.Digest)
	fmt.Printf("Size:   %d bytes\n", desc.Size)
	return nil
}
```

- [ ] **Step 3: Create skill push command**

Create `internal/cmd/skill_push.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	ociops "github.com/redhat-et/docsclaw/internal/oci"
)

var skillPushCmd = &cobra.Command{
	Use:   "push <skill-dir> <ref>",
	Short: "Pack and push a skill to an OCI registry",
	Args:  cobra.ExactArgs(2),
	RunE:  runSkillPush,
}

func init() {
	skillCmd.AddCommand(skillPushCmd)
	skillPushCmd.Flags().Bool("as-image", false, "push as a kubelet-mountable image")
}

func runSkillPush(cmd *cobra.Command, args []string) error {
	skillDir := args[0]
	ref := args[1]
	asImage, _ := cmd.Flags().GetBool("as-image")

	err := ociops.Push(context.Background(), skillDir, ref, ociops.PushOptions{
		AsImage: asImage,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Pushed %s to %s\n", skillDir, ref)
	return nil
}
```

- [ ] **Step 4: Create skill pull command**

Create `internal/cmd/skill_pull.go`:

```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	ociops "github.com/redhat-et/docsclaw/internal/oci"
	"github.com/redhat-et/docsclaw/internal/verify"
)

var skillPullCmd = &cobra.Command{
	Use:   "pull <ref>",
	Short: "Pull a skill from an OCI registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillPull,
}

func init() {
	skillCmd.AddCommand(skillPullCmd)
	skillPullCmd.Flags().Bool("verify", false, "verify signature before accepting")
	skillPullCmd.Flags().String("key", "", "public key for signature verification")
	skillPullCmd.Flags().StringP("output", "o", "", "output directory (default: ~/.docsclaw/skills/)")
}

func runSkillPull(cmd *cobra.Command, args []string) error {
	ref := args[0]
	doVerify, _ := cmd.Flags().GetBool("verify")
	keyPath, _ := cmd.Flags().GetString("key")
	outputDir, _ := cmd.Flags().GetString("output")

	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		outputDir = filepath.Join(home, ".docsclaw", "skills")
	}

	if doVerify {
		mode := verify.ModeEnforce
		if keyPath == "" {
			mode = verify.ModeWarn
		}
		policy := verify.Policy{Mode: mode, PublicKey: keyPath}
		if err := verify.Verify(context.Background(), ref, policy); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	err := ociops.Pull(context.Background(), ref, outputDir, ociops.PullOptions{})
	if err != nil {
		return err
	}

	fmt.Printf("Pulled %s to %s\n", ref, outputDir)
	return nil
}
```

- [ ] **Step 5: Create skill verify command**

Create `internal/cmd/skill_verify.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/internal/verify"
)

var skillVerifyCmd = &cobra.Command{
	Use:   "verify <ref>",
	Short: "Verify the signature of a skill artifact",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillVerify,
}

func init() {
	skillCmd.AddCommand(skillVerifyCmd)
	skillVerifyCmd.Flags().String("key", "", "public key for verification (required)")
	_ = skillVerifyCmd.MarkFlagRequired("key")
}

func runSkillVerify(cmd *cobra.Command, args []string) error {
	ref := args[0]
	keyPath, _ := cmd.Flags().GetString("key")

	policy := verify.Policy{
		Mode:      verify.ModeEnforce,
		PublicKey: keyPath,
	}

	if err := verify.Verify(context.Background(), ref, policy); err != nil {
		return err
	}

	fmt.Printf("Signature verified: %s\n", ref)
	return nil
}
```

- [ ] **Step 6: Create skill inspect command**

Create `internal/cmd/skill_inspect.go`:

```go
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	ociops "github.com/redhat-et/docsclaw/internal/oci"
)

var skillInspectCmd = &cobra.Command{
	Use:   "inspect <ref>",
	Short: "Show the SkillCard metadata for a skill artifact",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillInspect,
}

func init() {
	skillCmd.AddCommand(skillInspectCmd)
}

func runSkillInspect(cmd *cobra.Command, args []string) error {
	ref := args[0]

	sc, err := ociops.Inspect(context.Background(), ref, ociops.InspectOptions{})
	if err != nil {
		return err
	}

	fmt.Printf("Name:        %s\n", sc.Metadata.Name)
	fmt.Printf("Namespace:   %s\n", sc.Metadata.Namespace)
	fmt.Printf("Version:     %s\n", sc.Metadata.Version)
	fmt.Printf("Description: %s\n", sc.Metadata.Description)
	fmt.Printf("Author:      %s\n", sc.Metadata.Author)
	if sc.Metadata.License != "" {
		fmt.Printf("License:     %s\n", sc.Metadata.License)
	}
	if len(sc.Spec.Tools.Required) > 0 {
		fmt.Printf("Tools:       %s\n", strings.Join(sc.Spec.Tools.Required, ", "))
	}
	if sc.Spec.Resources.EstimatedMemory != "" {
		fmt.Printf("Memory:      %s\n", sc.Spec.Resources.EstimatedMemory)
	}
	if sc.Spec.Resources.EstimatedCPU != "" {
		fmt.Printf("CPU:         %s\n", sc.Spec.Resources.EstimatedCPU)
	}
	if sc.Metadata.Ref != "" {
		fmt.Printf("OCI Ref:     %s\n", sc.Metadata.Ref)
	}

	return nil
}
```

- [ ] **Step 7: Verify build**

```bash
make build
```

Expected: `bin/docsclaw` binary produced.

- [ ] **Step 8: Verify help output**

```bash
./bin/docsclaw skill --help
```

Expected: shows pack, push, pull, verify, inspect subcommands.

- [ ] **Step 9: Run all tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 10: Run lint**

```bash
make lint
```

Expected: clean.

- [ ] **Step 11: Commit**

```bash
git add internal/cmd/skill*.go
git commit -s -m "feat(cmd): add docsclaw skill CLI commands

Add skill subcommand group with pack, push, pull, verify, and
inspect commands. Commands are thin wrappers over the internal/oci
and internal/verify packages.

Relates to #4"
```

---

## Task 9: Backward compatibility in skill loader

**Files:**
- Modify: `pkg/skills/loader.go`
- Modify: `pkg/skills/loader_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/skills/loader_test.go`:

```go
func TestDiscoverWithSkillCard(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write SKILL.md with frontmatter.
	skillMD := "---\nname: my-skill\ndescription: From SKILL.md\n---\n# My skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write skill.yaml with richer metadata.
	skillYAML := `apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: my-skill
  namespace: official
  ref: quay.io/test/skill-my-skill
  version: 1.0.0
  description: From SkillCard with more detail.
  author: Test
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	skills, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("len(skills) = %d, want 1", len(skills))
	}
	// When both exist, SkillCard description takes precedence.
	if !strings.Contains(skills[0].Description, "From SkillCard") {
		t.Errorf("Description = %q, want SkillCard description", skills[0].Description)
	}
}
```

Add `"strings"` to the imports if not already present.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./pkg/skills/ -run TestDiscoverWithSkillCard -v
```

Expected: FAIL — SkillCard description not used.

- [ ] **Step 3: Modify Discover to read skill.yaml**

In `pkg/skills/loader.go`, modify the `Discover` function. After
the existing `parseFrontmatter` call, add a check for `skill.yaml`:

```go
// In the WalkDir callback, after parseFrontmatter succeeds:

// If skill.yaml exists, prefer its metadata.
cardPath := filepath.Join(filepath.Dir(skillMDPath), "skill.yaml")
if _, err := os.Stat(cardPath); err == nil {
	sc, err := card.Parse(cardPath)
	if err == nil {
		meta.Description = sc.Metadata.Description
	}
}
```

Add `"github.com/redhat-et/docsclaw/pkg/skills/card"` to the
imports of `loader.go`.

- [ ] **Step 4: Run all skills tests**

```bash
go test ./pkg/skills/ -v
```

Expected: all tests pass, including the new one.

- [ ] **Step 5: Run full test suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 6: Run lint**

```bash
make lint
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add pkg/skills/loader.go pkg/skills/loader_test.go
git commit -s -m "feat(skills): read skill.yaml for enriched metadata

When a skill directory contains both SKILL.md and skill.yaml, the
SkillCard description takes precedence. Skills without skill.yaml
continue to work unchanged.

Relates to #4"
```

---

## Task 10: End-to-end test with example skills

**Files:**
- No new files — uses existing `examples/skills/`

- [ ] **Step 1: Build the binary**

```bash
make build
```

- [ ] **Step 2: Pack the resume-screener skill**

```bash
./bin/docsclaw skill pack examples/skills/resume-screener
```

Expected: prints digest and size, creates
`examples/skills/resume-screener/oci-layout/`.

- [ ] **Step 3: Verify the layout was created**

```bash
ls examples/skills/resume-screener/oci-layout/
```

Expected: `blobs/`, `index.json`, `oci-layout` files.

- [ ] **Step 4: Pack all three demo skills**

```bash
./bin/docsclaw skill pack examples/skills/policy-comparator
./bin/docsclaw skill pack examples/skills/checklist-auditor
```

Expected: both succeed.

- [ ] **Step 5: Clean up generated OCI layouts**

```bash
rm -rf examples/skills/*/oci-layout
```

- [ ] **Step 6: Add oci-layout to .gitignore**

Append to `.gitignore`:

```text
# OCI layouts generated by docsclaw skill pack
oci-layout/
```

- [ ] **Step 7: Run full test suite one final time**

```bash
make test && make lint
```

Expected: all pass, clean lint.

- [ ] **Step 8: Commit**

```bash
git add .gitignore
git commit -s -m "chore: add oci-layout to gitignore

Prevents generated OCI layouts from docsclaw skill pack from
being accidentally committed.

Relates to #4"
```

---

## Summary

| Task | Component | Tests |
| ---- | --------- | ----- |
| 1 | Dependencies (oras-go, sigstore-go) | Existing tests pass |
| 2 | SkillCard types | Compile check |
| 3 | SkillCard parsing + validation | 5 tests (parse, validate, naming) |
| 4 | OCI media type constants | Compile check |
| 5 | Pack skill into OCI artifact | 3 tests (pack, as-image, missing yaml) |
| 6 | Push, Pull, Inspect | 2 tests (round-trip, inspect) |
| 7 | Signature verification | 2 tests (skip mode, enforce-no-key) |
| 8 | CLI commands (pack/push/pull/verify/inspect) | Build + help check |
| 9 | Backward compatibility in loader | 1 test (SkillCard-aware discover) |
| 10 | End-to-end with example skills | Manual verification |
