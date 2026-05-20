# Agent Manifest and Container Tooling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a declarative agent manifest (`agent-manifest.yaml`), a tiered tool catalog with risk scoring, and `docsclaw build`/`docsclaw deploy` commands that generate Containerfiles, K8s manifests, and OCI-annotated images from the manifest.

**Architecture:** New `pkg/catalog` package defines the tool catalog with risk scores and tier metadata. New `pkg/manifest` package defines the AgentManifest type, parses/validates it, checks skill compatibility, and generates Containerfiles, `tools.json`, and K8s YAML. Two new Cobra subcommands (`build`, `deploy`) orchestrate the pipeline. At runtime, the agent reads `/etc/docsclaw/tools.json` and injects the available tool list into the system prompt.

**Tech Stack:** Go stdlib (`text/template`, `embed`, `encoding/json`, `os`), Cobra/Viper (existing), `gopkg.in/yaml.v3` (existing dependency).

**Design doc:** `docs/superpowers/specs/2026-05-20-agent-manifest-tooling-design.md`

---

## Scope

This plan covers the core infrastructure. The shopping cart web UI is a separate follow-up plan.

| In scope | Out of scope (follow-up) |
|----------|--------------------------|
| Tool catalog types + embedded default | Shopping cart HTML UI |
| Agent manifest parsing/validation | OCI registry skill.yaml fetching |
| Containerfile generation | `--push` flag (build + push) |
| K8s manifest generation | Build policy enforcement |
| Compatibility checking (local skills) | Alpine/apk support |
| `docsclaw build --output` | Image building (podman/docker exec) |
| `docsclaw deploy` | |
| Runtime tools.json loading | |
| System prompt tool injection | |

---

## File structure

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/catalog/types.go` | Create | ToolCatalog, ToolEntry, RiskScore types |
| `pkg/catalog/catalog.go` | Create | Load, lookup, tier/risk helpers |
| `pkg/catalog/catalog_test.go` | Create | Catalog loading and lookup tests |
| `pkg/catalog/default.yaml` | Create | Embedded default tool catalog |
| `pkg/manifest/types.go` | Create | AgentManifest, ManifestSpec types |
| `pkg/manifest/parse.go` | Create | Parse + validate manifest YAML |
| `pkg/manifest/parse_test.go` | Create | Parsing and validation tests |
| `pkg/manifest/check.go` | Create | Skill compatibility checker |
| `pkg/manifest/check_test.go` | Create | Compatibility checking tests |
| `pkg/manifest/containerfile.go` | Create | Containerfile generator |
| `pkg/manifest/containerfile_test.go` | Create | Containerfile generation tests |
| `pkg/manifest/toolsjson.go` | Create | tools.json generator |
| `pkg/manifest/toolsjson_test.go` | Create | tools.json tests |
| `pkg/manifest/k8s.go` | Create | K8s manifest generator |
| `pkg/manifest/k8s_test.go` | Create | K8s manifest generation tests |
| `internal/cmd/build.go` | Create | `docsclaw build` Cobra command |
| `internal/cmd/deploy.go` | Create | `docsclaw deploy` Cobra command |
| `internal/cmd/serve.go` | Modify | Load tools.json, inject into prompt |
| `testdata/manifest/nps-agent.yaml` | Create | Example agent manifest |
| `testdata/manifest/bad-manifest.yaml` | Create | Invalid manifest for tests |

---

### Task 1: Tool catalog types and default catalog

**Files:**
- Create: `pkg/catalog/types.go`
- Create: `pkg/catalog/catalog.go`
- Create: `pkg/catalog/catalog_test.go`
- Create: `pkg/catalog/default.yaml`

- [ ] **Step 1: Write the catalog types**

Create `pkg/catalog/types.go`:

```go
package catalog

type ToolCatalog struct {
	APIVersion string                `yaml:"apiVersion"`
	Kind       string                `yaml:"kind"`
	Metadata   CatalogMeta           `yaml:"metadata"`
	Tiers      map[string]TierDef    `yaml:"tiers"`
	Tools      map[string]ToolEntry  `yaml:"tools"`
}

type CatalogMeta struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type TierDef struct {
	Description string `yaml:"description"`
	AutoInclude bool   `yaml:"autoInclude,omitempty"`
	Warning     string `yaml:"warning,omitempty"`
}

type ToolEntry struct {
	Tier        string            `yaml:"tier"`
	Package     map[string]string `yaml:"package"`
	Size        string            `yaml:"size"`
	Description string            `yaml:"description"`
	Risk        RiskScore         `yaml:"risk"`
}

type RiskScore struct {
	Score     int         `yaml:"score"`
	Factors   RiskFactors `yaml:"factors"`
	Rationale string      `yaml:"rationale"`
}

type RiskFactors struct {
	CodeExecution  bool   `yaml:"codeExecution"`
	NetworkCapable bool   `yaml:"networkCapable"`
	Dependencies   int    `yaml:"dependencies"`
	CVEHistory     string `yaml:"cveHistory"`
}
```

- [ ] **Step 2: Write the catalog loader tests**

Create `pkg/catalog/catalog_test.go`:

```go
package catalog

import "testing"

func TestLoadDefault(t *testing.T) {
	cat, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}
	if cat.Metadata.Name != "docsclaw-default" {
		t.Errorf("name = %q, want docsclaw-default", cat.Metadata.Name)
	}
	if _, ok := cat.Tools["curl"]; !ok {
		t.Error("curl not in default catalog")
	}
	if _, ok := cat.Tools["jq"]; !ok {
		t.Error("jq not in default catalog")
	}
}

func TestLoadFromFile(t *testing.T) {
	cat, err := LoadFromFile("testdata/custom-catalog.yaml")
	if err != nil {
		t.Fatalf("LoadFromFile() error: %v", err)
	}
	if cat.Metadata.Name != "test-catalog" {
		t.Errorf("name = %q, want test-catalog", cat.Metadata.Name)
	}
}

func TestLookupTool(t *testing.T) {
	cat, _ := LoadDefault()

	entry, ok := cat.Lookup("curl")
	if !ok {
		t.Fatal("curl not found")
	}
	if entry.Tier != "core" {
		t.Errorf("curl tier = %q, want core", entry.Tier)
	}

	_, ok = cat.Lookup("nonexistent")
	if ok {
		t.Error("nonexistent tool should not be found")
	}
}

func TestCoreTierTools(t *testing.T) {
	cat, _ := LoadDefault()
	core := cat.CoreTools()
	if len(core) == 0 {
		t.Fatal("no core tools found")
	}
	for _, name := range core {
		entry, _ := cat.Lookup(name)
		if entry.Tier != "core" {
			t.Errorf("%s tier = %q, want core", name, entry.Tier)
		}
	}
}

func TestHighestTier(t *testing.T) {
	cat, _ := LoadDefault()

	tests := []struct {
		tools []string
		want  string
	}{
		{[]string{"curl", "jq"}, "core"},
		{[]string{"curl", "git"}, "standard"},
		{[]string{"curl", "pandoc"}, "extended"},
		{[]string{"curl", "python3"}, "runtime"},
	}
	for _, tt := range tests {
		got := cat.HighestTier(tt.tools)
		if got != tt.want {
			t.Errorf("HighestTier(%v) = %q, want %q", tt.tools, got, tt.want)
		}
	}
}

func TestMaxRiskScore(t *testing.T) {
	cat, _ := LoadDefault()
	score := cat.MaxRiskScore([]string{"curl", "jq"})
	if score < 1 || score > 3 {
		t.Errorf("core-only risk score = %d, expected 1-3", score)
	}

	score = cat.MaxRiskScore([]string{"curl", "python3"})
	if score < 7 {
		t.Errorf("python3 risk score = %d, expected >= 7", score)
	}
}

func TestPackageName(t *testing.T) {
	cat, _ := LoadDefault()
	entry, _ := cat.Lookup("openssh-client")
	if entry.Package["dnf"] != "openssh-clients" {
		t.Errorf("openssh-client dnf package = %q, want openssh-clients", entry.Package["dnf"])
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/catalog/ -v`
Expected: compilation errors — `LoadDefault`, `Lookup`, etc. not defined.

- [ ] **Step 4: Create the default catalog YAML**

Create `pkg/catalog/default.yaml` with the catalog from the design
spec (all 9 tools: curl, jq, git, openssh-client, pandoc,
poppler-utils, imagemagick, python3, nodejs).

- [ ] **Step 5: Write the catalog loader**

Create `pkg/catalog/catalog.go`:

```go
package catalog

import (
	"embed"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultCatalog embed.FS

var tierOrder = map[string]int{
	"core":     0,
	"standard": 1,
	"extended": 2,
	"runtime":  3,
}

func LoadDefault() (*ToolCatalog, error) {
	data, err := defaultCatalog.ReadFile("default.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded catalog: %w", err)
	}
	return parse(data)
}

func LoadFromFile(path string) (*ToolCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog %s: %w", path, err)
	}
	return parse(data)
}

func parse(data []byte) (*ToolCatalog, error) {
	var cat ToolCatalog
	if err := yaml.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	return &cat, nil
}

func (c *ToolCatalog) Lookup(name string) (ToolEntry, bool) {
	entry, ok := c.Tools[name]
	return entry, ok
}

func (c *ToolCatalog) CoreTools() []string {
	var names []string
	for name, entry := range c.Tools {
		if entry.Tier == "core" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (c *ToolCatalog) HighestTier(tools []string) string {
	highest := "core"
	for _, name := range tools {
		entry, ok := c.Tools[name]
		if !ok {
			continue
		}
		if tierOrder[entry.Tier] > tierOrder[highest] {
			highest = entry.Tier
		}
	}
	return highest
}

func (c *ToolCatalog) MaxRiskScore(tools []string) int {
	max := 0
	for _, name := range tools {
		entry, ok := c.Tools[name]
		if !ok {
			continue
		}
		if entry.Risk.Score > max {
			max = entry.Risk.Score
		}
	}
	return max
}

func (c *ToolCatalog) PackageNames(tools []string, distro string) []string {
	var pkgs []string
	for _, name := range tools {
		entry, ok := c.Tools[name]
		if !ok {
			continue
		}
		if pkg, ok := entry.Package[distro]; ok {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

func (c *ToolCatalog) Validate(tools []string) error {
	for _, name := range tools {
		if _, ok := c.Tools[name]; !ok {
			return fmt.Errorf("unknown tool %q not in catalog", name)
		}
	}
	return nil
}
```

- [ ] **Step 6: Create test fixture for custom catalog**

Create `pkg/catalog/testdata/custom-catalog.yaml`:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: ToolCatalog
metadata:
  name: test-catalog
  version: 1.0.0
tiers:
  core:
    description: "Always included"
    autoInclude: true
tools:
  curl:
    tier: core
    package: { dnf: curl, apk: curl }
    size: ~1MB
    description: "HTTP client"
    risk:
      score: 2
      factors:
        codeExecution: false
        networkCapable: true
        dependencies: 2
        cveHistory: low
      rationale: "Network-capable but single-purpose"
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/catalog/ -v`
Expected: all tests PASS.

- [ ] **Step 8: Run linter**

Run: `cd /Users/panni/work/docsclaw && golangci-lint run ./pkg/catalog/`
Expected: no errors.

- [ ] **Step 9: Commit**

```bash
git add pkg/catalog/
git commit -s -m "feat: add tool catalog with risk scoring

Tiered tool catalog (core/standard/extended/runtime) with per-tool
risk scores, distro-specific package names, and embedded default
catalog."
```

---

### Task 2: Agent manifest types and parsing

**Files:**
- Create: `pkg/manifest/types.go`
- Create: `pkg/manifest/parse.go`
- Create: `pkg/manifest/parse_test.go`
- Create: `testdata/manifest/nps-agent.yaml`
- Create: `testdata/manifest/bad-manifest.yaml`

- [ ] **Step 1: Write the manifest types**

Create `pkg/manifest/types.go`:

```go
package manifest

type AgentManifest struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   ManifestMeta `yaml:"metadata"`
	Spec       ManifestSpec `yaml:"spec"`
}

type ManifestMeta struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type ManifestSpec struct {
	Base    BaseImage      `yaml:"base"`
	Tools   []string       `yaml:"tools"`
	Prompt  PromptConfig   `yaml:"prompt"`
	Skills  []SkillRef     `yaml:"skills"`
	Runtime RuntimeConfig  `yaml:"runtime,omitempty"`
	Secrets []SecretDecl   `yaml:"secrets,omitempty"`
	Deploy  DeployConfig   `yaml:"deploy,omitempty"`
}

type BaseImage struct {
	Image   string `yaml:"image"`
	Builder string `yaml:"builder"`
}

type PromptConfig struct {
	Text   string       `yaml:"text,omitempty"`
	Source *PromptSource `yaml:"source,omitempty"`
}

type PromptSource struct {
	Git  string `yaml:"git"`
	Path string `yaml:"path"`
	Ref  string `yaml:"ref"`
}

type SkillRef struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

type RuntimeConfig struct {
	Tools RuntimeToolsConfig `yaml:"tools"`
	Loop  RuntimeLoopConfig  `yaml:"loop"`
}

type RuntimeToolsConfig struct {
	Allowed  []string        `yaml:"allowed"`
	Exec     ExecConfig      `yaml:"exec,omitempty"`
	WebFetch WebFetchConfig  `yaml:"webFetch,omitempty"`
}

type ExecConfig struct {
	Timeout   int `yaml:"timeout,omitempty"`
	MaxOutput int `yaml:"maxOutput,omitempty"`
}

type WebFetchConfig struct {
	AllowedHosts []string `yaml:"allowedHosts,omitempty"`
}

type RuntimeLoopConfig struct {
	MaxIterations int `yaml:"maxIterations,omitempty"`
}

type SecretDecl struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required"`
}

type DeployConfig struct {
	Replicas  int             `yaml:"replicas,omitempty"`
	Resources ResourceConfig  `yaml:"resources,omitempty"`
}

type ResourceConfig struct {
	Requests ResourceValues `yaml:"requests,omitempty"`
	Limits   ResourceValues `yaml:"limits,omitempty"`
}

type ResourceValues struct {
	CPU    string `yaml:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}
```

- [ ] **Step 2: Create test fixtures**

Create `testdata/manifest/nps-agent.yaml`:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: AgentManifest
metadata:
  name: nps-assistant
  version: 1.0.0
spec:
  base:
    image: registry.access.redhat.com/hi/core-runtime:latest
    builder: registry.access.redhat.com/hi/core-runtime:latest-builder
  tools:
    - curl
    - jq
    - git
  prompt:
    text: |
      You are a national parks assistant.
  skills:
    - name: nps-api
      image: quay.io/docsclaw/skill-nps-api:1.0.0-image
  runtime:
    tools:
      allowed:
        - exec
        - read_file
        - web_fetch
        - load_skill
      exec:
        timeout: 30
        maxOutput: 50000
    loop:
      maxIterations: 15
  secrets:
    - name: NPS_API_KEY
      description: "API key for developer.nps.gov"
      required: true
    - name: LLM_API_KEY
      description: "LLM provider API key"
      required: true
  deploy:
    replicas: 1
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
      limits:
        cpu: 500m
        memory: 256Mi
```

Create `testdata/manifest/bad-manifest.yaml`:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: AgentManifest
metadata:
  name: ""
spec:
  base:
    image: ""
  tools: []
  prompt: {}
```

- [ ] **Step 3: Write parsing tests**

Create `pkg/manifest/parse_test.go`:

```go
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
			Base:  BaseImage{Image: "some-image"},
			Tools: []string{"curl"},
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
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestParse`
Expected: compilation errors — `ParseFile`, `Validate` not defined.

- [ ] **Step 5: Write the parser and validator**

Create `pkg/manifest/parse.go`:

```go
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func ParseFile(path string) (*AgentManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (*AgentManifest, error) {
	var m AgentManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := Validate(m); err != nil {
		return nil, err
	}
	return &m, nil
}

func Validate(m AgentManifest) error {
	if m.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if m.Spec.Base.Image == "" {
		return fmt.Errorf("spec.base.image is required")
	}
	if m.Spec.Prompt.Text == "" && m.Spec.Prompt.Source == nil {
		return fmt.Errorf("spec.prompt.text or spec.prompt.source is required")
	}
	if m.Spec.Prompt.Text != "" && m.Spec.Prompt.Source != nil {
		return fmt.Errorf("spec.prompt.text and spec.prompt.source are mutually exclusive")
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestParse`
Expected: all tests PASS.

- [ ] **Step 7: Run linter**

Run: `cd /Users/panni/work/docsclaw && golangci-lint run ./pkg/manifest/`
Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add pkg/manifest/types.go pkg/manifest/parse.go \
  pkg/manifest/parse_test.go testdata/manifest/
git commit -s -m "feat: add agent manifest types and parser

AgentManifest declares base image, OS tools, system prompt, skills,
secrets, and deployment config. Validation ensures required fields
are present."
```

---

### Task 3: Skill compatibility checker

**Files:**
- Create: `pkg/manifest/check.go`
- Create: `pkg/manifest/check_test.go`

- [ ] **Step 1: Write the compatibility check types and tests**

Create `pkg/manifest/check_test.go`:

```go
package manifest

import (
	"testing"

	"github.com/redhat-et/docsclaw/pkg/catalog"
	skillcard "github.com/redhat-et/docsclaw/pkg/skills/card"
)

func TestCheckCompatibility_AllSatisfied(t *testing.T) {
	tools := []string{"curl", "jq"}
	skills := []SkillCheck{
		{Name: "nps-api", Required: []string{"curl", "jq"}},
	}
	cat, _ := catalog.LoadDefault()

	results := CheckCompatibility(tools, skills, cat)
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	if !results[0].Satisfied {
		t.Error("nps-api should be satisfied")
	}
	if len(results[0].MissingRequired) != 0 {
		t.Errorf("missing required = %v, want none", results[0].MissingRequired)
	}
}

func TestCheckCompatibility_MissingRequired(t *testing.T) {
	tools := []string{"curl", "jq"}
	skills := []SkillCheck{
		{Name: "doc-converter", Required: []string{"curl", "pandoc"}},
	}
	cat, _ := catalog.LoadDefault()

	results := CheckCompatibility(tools, skills, cat)
	if results[0].Satisfied {
		t.Error("doc-converter should NOT be satisfied")
	}
	if len(results[0].MissingRequired) != 1 || results[0].MissingRequired[0] != "pandoc" {
		t.Errorf("missing required = %v, want [pandoc]", results[0].MissingRequired)
	}
}

func TestCheckCompatibility_OptionalMissing(t *testing.T) {
	tools := []string{"curl", "jq"}
	skills := []SkillCheck{
		{
			Name:     "nps-api",
			Required: []string{"curl", "jq"},
			Optional: []string{"python3"},
		},
	}
	cat, _ := catalog.LoadDefault()

	results := CheckCompatibility(tools, skills, cat)
	if !results[0].Satisfied {
		t.Error("should be satisfied (optional missing is OK)")
	}
	if len(results[0].MissingOptional) != 1 || results[0].MissingOptional[0] != "python3" {
		t.Errorf("missing optional = %v, want [python3]", results[0].MissingOptional)
	}
}

func TestCheckCompatibility_NoSkillYAML(t *testing.T) {
	tools := []string{"curl"}
	skills := []SkillCheck{
		{Name: "unknown-skill", HasSkillYAML: false},
	}
	cat, _ := catalog.LoadDefault()

	results := CheckCompatibility(tools, skills, cat)
	if !results[0].Satisfied {
		t.Error("skill without skill.yaml should be marked satisfied")
	}
	if !results[0].Unknown {
		t.Error("should be marked as unknown requirements")
	}
}

func TestSkillCheckFromCard(t *testing.T) {
	card := skillcard.SkillCard{
		Metadata: skillcard.SkillCardMeta{Name: "test-skill"},
		Spec: skillcard.SkillCardSpec{
			Tools: skillcard.ToolDeps{
				Required: []string{"curl"},
				Optional: []string{"jq"},
			},
		},
	}
	sc := SkillCheckFromCard(card)
	if sc.Name != "test-skill" {
		t.Errorf("name = %q, want test-skill", sc.Name)
	}
	if !sc.HasSkillYAML {
		t.Error("should have HasSkillYAML = true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestCheck`
Expected: compilation errors.

- [ ] **Step 3: Write the compatibility checker**

Create `pkg/manifest/check.go`:

```go
package manifest

import (
	"github.com/redhat-et/docsclaw/pkg/catalog"
	skillcard "github.com/redhat-et/docsclaw/pkg/skills/card"
)

type SkillCheck struct {
	Name         string
	Required     []string
	Optional     []string
	HasSkillYAML bool
}

type CheckResult struct {
	SkillName       string
	Satisfied       bool
	Unknown         bool
	MissingRequired []string
	MissingOptional []string
}

func SkillCheckFromCard(card skillcard.SkillCard) SkillCheck {
	return SkillCheck{
		Name:         card.Metadata.Name,
		Required:     card.Spec.Tools.Required,
		Optional:     card.Spec.Tools.Optional,
		HasSkillYAML: true,
	}
}

func CheckCompatibility(manifestTools []string, skills []SkillCheck, cat *catalog.ToolCatalog) []CheckResult {
	installed := make(map[string]bool, len(manifestTools))
	for _, t := range manifestTools {
		installed[t] = true
	}
	for _, name := range cat.CoreTools() {
		installed[name] = true
	}

	var results []CheckResult
	for _, skill := range skills {
		r := CheckResult{SkillName: skill.Name}

		if !skill.HasSkillYAML {
			r.Satisfied = true
			r.Unknown = true
			results = append(results, r)
			continue
		}

		r.Satisfied = true
		for _, req := range skill.Required {
			if !installed[req] {
				r.MissingRequired = append(r.MissingRequired, req)
				r.Satisfied = false
			}
		}
		for _, opt := range skill.Optional {
			if !installed[opt] {
				r.MissingOptional = append(r.MissingOptional, opt)
			}
		}
		results = append(results, r)
	}
	return results
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestCheck`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/manifest/check.go pkg/manifest/check_test.go
git commit -s -m "feat: add skill compatibility checker

Validates that manifest tools satisfy skill requirements from
skill.yaml. Reports missing required and optional tools. Skills
without skill.yaml are flagged as unknown."
```

---

### Task 4: Containerfile generator

**Files:**
- Create: `pkg/manifest/containerfile.go`
- Create: `pkg/manifest/containerfile_test.go`

- [ ] **Step 1: Write Containerfile generation tests**

Create `pkg/manifest/containerfile_test.go`:

```go
package manifest

import (
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

func TestGenerateContainerfile_HardenedImage(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "test-agent"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image:   "registry.access.redhat.com/hi/core-runtime:latest",
				Builder: "registry.access.redhat.com/hi/core-runtime:latest-builder",
			},
			Tools: []string{"curl", "jq", "git"},
		},
	}
	cat, _ := catalog.LoadDefault()

	out, err := GenerateContainerfile(m, cat)
	if err != nil {
		t.Fatalf("GenerateContainerfile() error: %v", err)
	}

	checks := []string{
		"FROM registry.access.redhat.com/hi/core-runtime:latest",
		"io.docsclaw.tools/installed",
		"curl,git,jq",
		"--mount=type=bind,from=registry.access.redhat.com/hi/core-runtime:latest-builder",
		"/builder/usr/bin/dnf install -y",
		"curl git jq",
		"USER 65532",
		"COPY docsclaw /app/docsclaw",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGenerateContainerfile_Labels(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "my-agent"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image:   "registry.access.redhat.com/hi/core-runtime:latest",
				Builder: "registry.access.redhat.com/hi/core-runtime:latest-builder",
			},
			Tools: []string{"curl", "jq", "python3"},
		},
	}
	cat, _ := catalog.LoadDefault()

	out, err := GenerateContainerfile(m, cat)
	if err != nil {
		t.Fatalf("GenerateContainerfile() error: %v", err)
	}

	if !strings.Contains(out, `io.docsclaw.tools/tier="runtime"`) {
		t.Error("missing tier label for runtime")
	}
	if !strings.Contains(out, `io.docsclaw.tools/agent-name="my-agent"`) {
		t.Error("missing agent-name label")
	}
}

func TestGenerateContainerfile_CoreOnlyNoBuilder(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "minimal"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image: "registry.access.redhat.com/hi/core-runtime:latest",
			},
			Tools: []string{"curl", "jq"},
		},
	}
	cat, _ := catalog.LoadDefault()

	out, err := GenerateContainerfile(m, cat)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(out, "FROM registry.access.redhat.com/hi/core-runtime:latest") {
		t.Error("missing FROM line")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestGenerateContainerfile`
Expected: compilation errors.

- [ ] **Step 3: Write the Containerfile generator**

Create `pkg/manifest/containerfile.go`:

```go
package manifest

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

var containerfileTmpl = template.Must(template.New("containerfile").Parse(`FROM {{ .BaseImage }}

LABEL io.docsclaw.tools/installed="{{ .InstalledCSV }}"
LABEL io.docsclaw.tools/tier="{{ .HighestTier }}"
LABEL io.docsclaw.tools/risk-score="{{ .RiskScore }}"
LABEL io.docsclaw.tools/agent-name="{{ .AgentName }}"
{{ if .HasBuilder }}
# Adding tools to the minimal hardened image expands its attack surface.
# Only add what is strictly necessary for runtime operation.
# Review each addition with your security team.
USER root
RUN --mount=type=bind,from={{ .BuilderImage }},target=/builder \
    LD_LIBRARY_PATH=/builder/lib64:/builder/usr/lib64 \
    RPM_CONFIGDIR=/builder/usr/lib/rpm \
    /builder/usr/bin/dnf install -y \
    --installroot=/ \
    --setopt=reposdir=/builder/etc/yum.repos.d \
    --setopt=install_weak_deps=False \
    --setopt=tsflags=nodocs \
    {{ .PackageList }}
USER 65532
{{ end }}
WORKDIR /app
COPY docsclaw /app/docsclaw

EXPOSE 8000

ENTRYPOINT ["/app/docsclaw"]
CMD ["serve"]
`))

type containerfileData struct {
	BaseImage    string
	BuilderImage string
	HasBuilder   bool
	AgentName    string
	InstalledCSV string
	HighestTier  string
	RiskScore    int
	PackageList  string
}

func GenerateContainerfile(m *AgentManifest, cat *catalog.ToolCatalog) (string, error) {
	allTools := mergeWithCore(m.Spec.Tools, cat)
	sort.Strings(allTools)

	pkgs := cat.PackageNames(allTools, "dnf")
	sort.Strings(pkgs)

	data := containerfileData{
		BaseImage:    m.Spec.Base.Image,
		BuilderImage: m.Spec.Base.Builder,
		HasBuilder:   m.Spec.Base.Builder != "",
		AgentName:    m.Metadata.Name,
		InstalledCSV: strings.Join(allTools, ","),
		HighestTier:  cat.HighestTier(allTools),
		RiskScore:    cat.MaxRiskScore(allTools),
		PackageList:  strings.Join(pkgs, " "),
	}

	var buf bytes.Buffer
	if err := containerfileTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render containerfile: %w", err)
	}
	return buf.String(), nil
}

func mergeWithCore(tools []string, cat *catalog.ToolCatalog) []string {
	seen := make(map[string]bool)
	var merged []string
	for _, name := range cat.CoreTools() {
		if !seen[name] {
			seen[name] = true
			merged = append(merged, name)
		}
	}
	for _, name := range tools {
		if !seen[name] {
			seen[name] = true
			merged = append(merged, name)
		}
	}
	return merged
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestGenerateContainerfile`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/manifest/containerfile.go pkg/manifest/containerfile_test.go
git commit -s -m "feat: add Containerfile generator from agent manifest

Generates Containerfile with dnf bind-mount pattern, OCI annotation
labels (installed tools, tier, risk score, agent name), and hardened
image security comment."
```

---

### Task 5: tools.json generator

**Files:**
- Create: `pkg/manifest/toolsjson.go`
- Create: `pkg/manifest/toolsjson_test.go`

- [ ] **Step 1: Write tools.json generation tests**

Create `pkg/manifest/toolsjson_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestGenerateToolsJSON`
Expected: compilation errors.

- [ ] **Step 3: Write the tools.json generator**

Create `pkg/manifest/toolsjson.go`:

```go
package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

type ToolsJSON struct {
	ManifestVersion string         `json:"manifestVersion"`
	AgentName       string         `json:"agentName"`
	Base            string         `json:"base"`
	HighestTier     string         `json:"highestTier"`
	RiskScore       int            `json:"riskScore"`
	Tools           []ToolJSONEntry `json:"tools"`
}

type ToolJSONEntry struct {
	Name           string `json:"name"`
	Package        string `json:"package"`
	Tier           string `json:"tier"`
	Risk           ToolJSONRisk `json:"risk"`
}

type ToolJSONRisk struct {
	Score          int  `json:"score"`
	CodeExecution  bool `json:"codeExecution"`
	NetworkCapable bool `json:"networkCapable"`
}

func GenerateToolsJSON(m *AgentManifest, cat *catalog.ToolCatalog) ([]byte, error) {
	allTools := mergeWithCore(m.Spec.Tools, cat)
	sort.Strings(allTools)

	tj := ToolsJSON{
		ManifestVersion: m.Metadata.Version,
		AgentName:       m.Metadata.Name,
		Base:            m.Spec.Base.Image,
		HighestTier:     cat.HighestTier(allTools),
		RiskScore:       cat.MaxRiskScore(allTools),
	}

	for _, name := range allTools {
		entry, ok := cat.Lookup(name)
		if !ok {
			continue
		}
		tj.Tools = append(tj.Tools, ToolJSONEntry{
			Name:    name,
			Package: entry.Package["dnf"],
			Tier:    entry.Tier,
			Risk: ToolJSONRisk{
				Score:          entry.Risk.Score,
				CodeExecution:  entry.Risk.Factors.CodeExecution,
				NetworkCapable: entry.Risk.Factors.NetworkCapable,
			},
		})
	}

	data, err := json.MarshalIndent(tj, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tools.json: %w", err)
	}
	return data, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestGenerateToolsJSON`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/manifest/toolsjson.go pkg/manifest/toolsjson_test.go
git commit -s -m "feat: add tools.json generator for runtime metadata

Generates /etc/docsclaw/tools.json with installed tool inventory,
tier, risk scores for runtime compatibility checking."
```

---

### Task 6: K8s manifest generator

**Files:**
- Create: `pkg/manifest/k8s.go`
- Create: `pkg/manifest/k8s_test.go`

- [ ] **Step 1: Write K8s generation tests**

Create `pkg/manifest/k8s_test.go`:

```go
package manifest

import (
	"strings"
	"testing"
)

func TestGenerateK8s_ConfigMap(t *testing.T) {
	m := testManifest()
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("GenerateK8s() error: %v", err)
	}

	if !strings.Contains(k8s.ConfigMap, "kind: ConfigMap") {
		t.Error("missing ConfigMap kind")
	}
	if !strings.Contains(k8s.ConfigMap, "name: nps-assistant-config") {
		t.Error("missing ConfigMap name")
	}
	if !strings.Contains(k8s.ConfigMap, "system-prompt.txt") {
		t.Error("missing system-prompt.txt key")
	}
	if !strings.Contains(k8s.ConfigMap, "agent-config.yaml") {
		t.Error("missing agent-config.yaml key")
	}
}

func TestGenerateK8s_Deployment(t *testing.T) {
	m := testManifest()
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(k8s.Deployment, "kind: Deployment") {
		t.Error("missing Deployment kind")
	}
	if !strings.Contains(k8s.Deployment, "runAsNonRoot: true") {
		t.Error("missing security context")
	}
	if !strings.Contains(k8s.Deployment, "skill-nps-api") {
		t.Error("missing skill volume")
	}
}

func TestGenerateK8s_Secret(t *testing.T) {
	m := testManifest()
	secrets := map[string]string{
		"NPS_API_KEY": "test-key",
		"LLM_API_KEY": "test-llm",
	}
	k8s, err := GenerateK8s(m, secrets)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if k8s.Secret == "" {
		t.Fatal("secret should be generated when values provided")
	}
	if !strings.Contains(k8s.Secret, "kind: Secret") {
		t.Error("missing Secret kind")
	}
}

func TestGenerateK8s_NoSecrets(t *testing.T) {
	m := testManifest()
	m.Spec.Secrets = nil
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if k8s.Secret != "" {
		t.Error("should not generate secret when none declared")
	}
}

func TestGenerateK8s_Service(t *testing.T) {
	m := testManifest()
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(k8s.Service, "kind: Service") {
		t.Error("missing Service kind")
	}
	if !strings.Contains(k8s.Service, "port: 8000") {
		t.Error("missing http port")
	}
}

func testManifest() *AgentManifest {
	return &AgentManifest{
		Metadata: ManifestMeta{Name: "nps-assistant", Version: "1.0.0"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image: "ghcr.io/redhat-et/docsclaw:latest",
			},
			Tools: []string{"curl", "jq"},
			Prompt: PromptConfig{
				Text: "You are a national parks assistant.",
			},
			Skills: []SkillRef{
				{Name: "nps-api", Image: "quay.io/docsclaw/skill-nps-api:1.0.0-image"},
			},
			Runtime: RuntimeConfig{
				Tools: RuntimeToolsConfig{
					Allowed: []string{"exec", "read_file", "load_skill"},
					Exec:    ExecConfig{Timeout: 30, MaxOutput: 50000},
				},
				Loop: RuntimeLoopConfig{MaxIterations: 15},
			},
			Secrets: []SecretDecl{
				{Name: "NPS_API_KEY", Required: true},
				{Name: "LLM_API_KEY", Required: true},
			},
			Deploy: DeployConfig{
				Replicas: 1,
				Resources: ResourceConfig{
					Requests: ResourceValues{CPU: "100m", Memory: "64Mi"},
					Limits:   ResourceValues{CPU: "500m", Memory: "256Mi"},
				},
			},
		},
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestGenerateK8s`
Expected: compilation errors.

- [ ] **Step 3: Write the K8s manifest generator**

Create `pkg/manifest/k8s.go`. This file generates ConfigMap,
Deployment, Service, ServiceAccount, and Secret YAML using
`text/template`. The Deployment template includes security context
(runAsNonRoot, drop ALL, readOnlyRootFilesystem), skill image
volumes, health probes, and resource limits from the manifest.

The `K8sOutput` struct holds each generated YAML as a string field:

```go
type K8sOutput struct {
	ConfigMap      string
	Deployment     string
	Service        string
	ServiceAccount string
	Secret         string
}

func GenerateK8s(m *AgentManifest, secrets map[string]string) (*K8sOutput, error)
```

Key template logic:
- ConfigMap data keys: `system-prompt.txt` (from `m.Spec.Prompt.Text`),
  `agent-config.yaml` (generated from `m.Spec.Runtime`)
- Deployment volumes: `agent-config` ConfigMap volume + one
  `image:` volume per skill in `m.Spec.Skills`
- Secret: only generated when `m.Spec.Secrets` is non-empty and
  `secrets` map is provided. Uses `base64.StdEncoding` for values.
  Deployment references it via `envFrom.secretRef`.

Follow the pattern from the existing skillimage demo
(`~/work/skillimage/site/demo/index.html` lines 1144-1315) for
the YAML structure.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./pkg/manifest/ -v -run TestGenerateK8s`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/manifest/k8s.go pkg/manifest/k8s_test.go
git commit -s -m "feat: add K8s manifest generator from agent manifest

Generates ConfigMap, Deployment, Service, ServiceAccount, and Secret
YAML with hardened security context and skill image volumes."
```

---

### Task 7: `docsclaw build` command

**Files:**
- Create: `internal/cmd/build.go`

- [ ] **Step 1: Write the build command**

Create `internal/cmd/build.go`. Register it in `root.go` via
`rootCmd.AddCommand(buildCmd)`.

The command:
- Flags: `--manifest` (required), `--output` (directory),
  `--only` (containerfile|k8s), `--dry-run`, `--catalog` (optional
  custom catalog path), `--max-risk` (int, 0 = no limit)
- Validates manifest against schema
- Loads tool catalog (default + optional custom)
- Validates all tools exist in catalog
- Runs compatibility check (for skills with local skill.yaml)
- Prints compatibility report to stderr
- If `--dry-run`: print report and exit
- If `--max-risk N` and risk > N: fail with error
- If `--output`: generate files to directory
- Default (no `--output`): print generated Containerfile to stdout

```go
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build agent image from manifest",
	Long:  "Generate Containerfile and K8s manifests from an agent manifest.",
	RunE:  runBuild,
}

func init() {
	buildCmd.Flags().String("manifest", "", "path to agent-manifest.yaml (required)")
	buildCmd.Flags().String("output", "", "directory to write generated files")
	buildCmd.Flags().String("only", "", "generate only: containerfile, k8s")
	buildCmd.Flags().Bool("dry-run", false, "print compatibility report only")
	buildCmd.Flags().String("catalog", "", "path to custom tool catalog")
	buildCmd.Flags().Int("max-risk", 0, "max allowed risk score (0 = no limit)")
	_ = buildCmd.MarkFlagRequired("manifest")
}
```

The `runBuild` function orchestrates the pipeline:

1. `manifest.ParseFile(manifestPath)`
2. `catalog.LoadDefault()` (or `LoadFromFile` if `--catalog`)
3. `cat.Validate(m.Spec.Tools)` — fail on unknown tools
4. Print risk score and tier to stderr
5. If `--dry-run`: exit
6. `manifest.GenerateContainerfile(m, cat)`
7. `manifest.GenerateToolsJSON(m, cat)`
8. `manifest.GenerateK8s(m, nil)` (if not `--only containerfile`)
9. If `--output`: write files to directory; else print to stdout

- [ ] **Step 2: Register the command in root.go**

Add `rootCmd.AddCommand(buildCmd)` to `internal/cmd/root.go` in
the `init()` function, alongside the existing `serveCmd` and
`chatCmd` registrations.

- [ ] **Step 3: Test the command manually**

Run: `cd /Users/panni/work/docsclaw && go run ./cmd/docsclaw build --manifest testdata/manifest/nps-agent.yaml --dry-run`
Expected: prints compatibility report with tool list and risk score.

Run: `cd /Users/panni/work/docsclaw && go run ./cmd/docsclaw build --manifest testdata/manifest/nps-agent.yaml --output /tmp/docsclaw-build-test`
Expected: generates files in `/tmp/docsclaw-build-test/`.

- [ ] **Step 4: Run linter**

Run: `cd /Users/panni/work/docsclaw && golangci-lint run ./internal/cmd/`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/build.go internal/cmd/root.go
git commit -s -m "feat: add docsclaw build command

Reads agent manifest, validates tools against catalog, checks skill
compatibility, and generates Containerfile + K8s manifests to an
output directory."
```

---

### Task 8: `docsclaw deploy` command

**Files:**
- Create: `internal/cmd/deploy.go`

- [ ] **Step 1: Write the deploy command**

Create `internal/cmd/deploy.go`. Register it in `root.go`.

The command:
- Flags: `--manifest` (required), `--secret` (string slice,
  `NAME=value`), `--output` (directory, optional)
- Resolves secrets: `--secret` flag > environment variable >
  fail if required
- Calls `manifest.GenerateK8s(m, resolvedSecrets)`
- If `--output`: write files to directory
- Default: print all manifests joined by `---` to stdout
  (pipeable to `oc apply -f -`)

```go
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Generate K8s manifests from agent manifest",
	Long:  "Resolve secrets and generate deployment manifests.",
	RunE:  runDeploy,
}

func init() {
	deployCmd.Flags().String("manifest", "", "path to agent-manifest.yaml (required)")
	deployCmd.Flags().StringSlice("secret", nil, "secret values as NAME=value")
	deployCmd.Flags().String("output", "", "directory to write generated files")
	_ = deployCmd.MarkFlagRequired("manifest")
}
```

Secret resolution in `runDeploy`:

```go
func resolveSecrets(decls []manifest.SecretDecl, flagSecrets []string) (map[string]string, error) {
	overrides := make(map[string]string)
	for _, s := range flagSecrets {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --secret format: %q (expected NAME=value)", s)
		}
		overrides[parts[0]] = parts[1]
	}

	resolved := make(map[string]string)
	for _, decl := range decls {
		if v, ok := overrides[decl.Name]; ok {
			resolved[decl.Name] = v
		} else if v := os.Getenv(decl.Name); v != "" {
			resolved[decl.Name] = v
		} else if decl.Required {
			return nil, fmt.Errorf("required secret %q not set (use --secret %s=value or export %s)", decl.Name, decl.Name, decl.Name)
		}
	}
	return resolved, nil
}
```

- [ ] **Step 2: Register the command in root.go**

Add `rootCmd.AddCommand(deployCmd)` to `init()`.

- [ ] **Step 3: Test the command manually**

Run:
```bash
export NPS_API_KEY=test123
export LLM_API_KEY=sk-test
go run ./cmd/docsclaw deploy --manifest testdata/manifest/nps-agent.yaml
```
Expected: prints ConfigMap, Deployment, Service, ServiceAccount,
Secret YAML separated by `---`.

Run without secrets:
```bash
unset NPS_API_KEY
go run ./cmd/docsclaw deploy --manifest testdata/manifest/nps-agent.yaml
```
Expected: fails with `required secret "NPS_API_KEY" not set`.

- [ ] **Step 4: Run linter**

Run: `cd /Users/panni/work/docsclaw && golangci-lint run ./internal/cmd/`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/deploy.go internal/cmd/root.go
git commit -s -m "feat: add docsclaw deploy command

Generates K8s manifests with secrets resolved from --secret flags
or environment variables. Output is pipeable to oc apply -f -."
```

---

### Task 9: Runtime tools.json loading and system prompt injection

**Files:**
- Modify: `internal/cmd/serve.go`

- [ ] **Step 1: Add tools.json loader**

Add a `loadToolsJSON` function to `internal/cmd/serve.go` that
reads `/etc/docsclaw/tools.json` (or a configurable path). If the
file doesn't exist, return nil (backward compatible — existing
images without tools.json continue to work).

```go
func loadToolsJSON(path string) (*manifest.ToolsJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tools.json: %w", err)
	}
	var tj manifest.ToolsJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse tools.json: %w", err)
	}
	return &tj, nil
}
```

- [ ] **Step 2: Inject available tools into system prompt**

In the `runServe` function, after loading the system prompt and
before passing it to the A2A handler, append the tool inventory
if tools.json was found:

```go
const toolsJSONPath = "/etc/docsclaw/tools.json"

tj, err := loadToolsJSON(toolsJSONPath)
if err != nil {
	slog.Warn("failed to load tools.json", "error", err)
}
if tj != nil {
	var names []string
	for _, t := range tj.Tools {
		names = append(names, t.Name)
	}
	systemPrompt += fmt.Sprintf(
		"\n\nAvailable OS tools: %s\nDo NOT attempt to use tools not in this list.",
		strings.Join(names, ", "),
	)
	slog.Info("loaded OS tool inventory", "tools", names, "risk_score", tj.RiskScore)
}
```

- [ ] **Step 3: Test manually with a local tools.json**

Create a test tools.json and set the path via an env var or
flag to test locally without building a container:

```bash
mkdir -p /tmp/docsclaw-test
cat > /tmp/docsclaw-test/tools.json << 'EOF'
{
  "manifestVersion": "1.0.0",
  "agentName": "test",
  "base": "test",
  "highestTier": "core",
  "riskScore": 2,
  "tools": [
    {"name": "curl", "package": "curl", "tier": "core", "risk": {"score": 2, "codeExecution": false, "networkCapable": true}},
    {"name": "jq", "package": "jq", "tier": "core", "risk": {"score": 1, "codeExecution": false, "networkCapable": false}}
  ]
}
EOF
```

Verify the startup log shows:
`INFO loaded OS tool inventory tools=[curl, jq] risk_score=2`

- [ ] **Step 4: Run tests and linter**

Run: `cd /Users/panni/work/docsclaw && go test ./... && golangci-lint run ./...`
Expected: all tests pass, no lint errors.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/serve.go
git commit -s -m "feat: inject OS tool inventory into system prompt

At startup, reads /etc/docsclaw/tools.json and appends available
tool names to the system prompt so the LLM knows exactly which
OS tools exist. Prevents wasted agentic loop iterations on
missing tools."
```

---

### Task 10: Final integration test and cleanup

**Files:**
- All files from tasks 1-9

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/panni/work/docsclaw && go test ./... -v`
Expected: all tests pass including new catalog, manifest, and
existing tests.

- [ ] **Step 2: Run linter on entire project**

Run: `cd /Users/panni/work/docsclaw && golangci-lint run ./...`
Expected: no errors.

- [ ] **Step 3: Test the full build pipeline end-to-end**

```bash
cd /Users/panni/work/docsclaw

# Dry run
go run ./cmd/docsclaw build \
  --manifest testdata/manifest/nps-agent.yaml \
  --dry-run

# Generate all artifacts
go run ./cmd/docsclaw build \
  --manifest testdata/manifest/nps-agent.yaml \
  --output /tmp/docsclaw-build-e2e

# Verify generated files
ls -la /tmp/docsclaw-build-e2e/
cat /tmp/docsclaw-build-e2e/Containerfile
cat /tmp/docsclaw-build-e2e/tools.json

# Test deploy with secrets
export NPS_API_KEY=test123 LLM_API_KEY=sk-test
go run ./cmd/docsclaw deploy \
  --manifest testdata/manifest/nps-agent.yaml

# Test deploy with --secret flag
unset NPS_API_KEY LLM_API_KEY
go run ./cmd/docsclaw deploy \
  --manifest testdata/manifest/nps-agent.yaml \
  --secret NPS_API_KEY=test123 \
  --secret LLM_API_KEY=sk-test
```

- [ ] **Step 4: Commit any remaining fixes**

If any issues found in integration testing, fix and commit.
