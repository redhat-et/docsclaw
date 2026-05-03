# Skill Cleanup and AgentCard Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove OCI skill management code that moved to the skillimage project, and populate the AgentCard skills array from discovered skill files at runtime.

**Architecture:** Delete `internal/oci/`, `internal/verify/`, and five OCI-related skill subcommands. Add a `ToAgentSkills()` converter in `pkg/skills/a2a.go` that maps `SkillMeta` + optional `SkillCard` metadata to `a2a.AgentSkill`. Wire the converter into `serve.go` so the A2A endpoint reflects runtime-available skills.

**Tech Stack:** Go 1.25+, a2a-go v2, Cobra/Viper

---

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Delete | `internal/oci/` (7 files) | OCI pack/push/pull/inspect library |
| Delete | `internal/verify/` (3 files) | Cosign signature verification |
| Delete | `internal/cmd/skill_pack.go` | `docsclaw skill pack` command |
| Delete | `internal/cmd/skill_push.go` | `docsclaw skill push` command |
| Delete | `internal/cmd/skill_pull.go` | `docsclaw skill pull` command |
| Delete | `internal/cmd/skill_inspect.go` | `docsclaw skill inspect` command |
| Delete | `internal/cmd/skill_verify.go` | `docsclaw skill verify` command |
| Modify | `internal/cmd/skill.go` | Update parent command description |
| Modify | `go.mod` | Remove unused dependencies |
| Create | `pkg/skills/a2a.go` | `ToAgentSkills()` converter |
| Create | `pkg/skills/a2a_test.go` | Tests for converter |
| Modify | `internal/cmd/serve.go` | Wire skill discovery into AgentCard |

---

### Task 1: Delete OCI Package and Verify Package

**Files:**
- Delete: `internal/oci/pack.go`
- Delete: `internal/oci/push.go`
- Delete: `internal/oci/pull.go`
- Delete: `internal/oci/inspect.go`
- Delete: `internal/oci/media.go`
- Delete: `internal/oci/pack_test.go`
- Delete: `internal/oci/push_pull_test.go`
- Delete: `internal/oci/extract_test.go`
- Delete: `internal/verify/verify.go`
- Delete: `internal/verify/policy.go`
- Delete: `internal/verify/verify_test.go`

- [ ] **Step 1: Delete internal/oci/ directory**

```bash
rm -rf internal/oci/
```

- [ ] **Step 2: Delete internal/verify/ directory**

```bash
rm -rf internal/verify/
```

- [ ] **Step 3: Verify the project still compiles (it won't yet — commands reference these packages)**

```bash
go build ./... 2>&1 | head -20
```

Expected: compile errors in `internal/cmd/skill_pack.go`, `skill_push.go`, `skill_pull.go`, `skill_inspect.go`, `skill_verify.go` (and `skill_pull.go` also imports `verify`). This confirms we've removed the right packages and nothing else depends on them.

---

### Task 2: Delete OCI Skill Subcommands

**Files:**
- Delete: `internal/cmd/skill_pack.go`
- Delete: `internal/cmd/skill_push.go`
- Delete: `internal/cmd/skill_pull.go`
- Delete: `internal/cmd/skill_inspect.go`
- Delete: `internal/cmd/skill_verify.go`

- [ ] **Step 1: Delete the five OCI command files**

```bash
rm internal/cmd/skill_pack.go
rm internal/cmd/skill_push.go
rm internal/cmd/skill_pull.go
rm internal/cmd/skill_inspect.go
rm internal/cmd/skill_verify.go
```

- [ ] **Step 2: Update parent skill command description**

Modify `internal/cmd/skill.go` — change the Short and Long descriptions to reflect that only local skill management commands remain (list, delete):

```go
package cmd

import "github.com/spf13/cobra"

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage locally available skills",
	Long:  "List and delete locally available skills.",
}

func init() {
	rootCmd.AddCommand(skillCmd)
}
```

- [ ] **Step 3: Verify compilation succeeds**

```bash
go build ./...
```

Expected: SUCCESS — no compile errors. The remaining `skill_list.go` and `skill_delete.go` do not import `internal/oci` or `internal/verify`.

- [ ] **Step 4: Run go mod tidy to remove unused dependencies**

```bash
go mod tidy
```

Then verify that `oras.land/oras-go/v2` and `github.com/opencontainers/image-spec` are gone from `go.mod`:

```bash
grep -E 'oras\.land|opencontainers/image-spec' go.mod
```

Expected: no output (both removed).

- [ ] **Step 5: Run tests**

```bash
make test
```

Expected: all tests pass. The deleted test files are gone, and no remaining test references the removed packages.

- [ ] **Step 6: Run linter**

```bash
make lint
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -s -m "refactor: remove OCI skill management code (moved to skillimage)

Remove internal/oci/, internal/verify/, and CLI commands skill pack,
push, pull, inspect, verify. This code now lives in the skillimage
project and its skillctl CLI.

Keep local skill management (list, delete), runtime skill discovery
(pkg/skills/), and SkillCard parsing (pkg/skills/card/).

Closes #38

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 3: Write ToAgentSkills converter test

**Files:**
- Create: `pkg/skills/a2a_test.go`

- [ ] **Step 1: Write the test file**

Create `pkg/skills/a2a_test.go` with tests covering all four cases: skill with `skill.yaml` (full mapping), skill with only `SKILL.md` (fallback), empty input, and mixed input.

The test creates temp directories with fixture files and calls `ToAgentSkills()`.

```go
package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestToAgentSkillsWithSkillCard(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "resume-screener")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillMD := "---\nname: resume-screener\ndescription: From SKILL.md\n---\n# Resume screener\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	skillYAML := `apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: resume-screener
  namespace: official
  ref: quay.io/docsclaw/skill-resume-screener
  version: 1.0.0
  description: Screen resumes against a job description and produce structured feedback.
  author: Red Hat ET
  license: Apache-2.0
spec:
  tools:
    required:
      - read_file
    optional:
      - write_file
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	metas := []SkillMeta{
		{Name: "resume-screener", Description: "From SKILL.md", Dir: skillDir},
	}

	result := ToAgentSkills(metas)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}

	skill := result[0]
	if skill.ID != "resume-screener" {
		t.Errorf("ID = %q, want %q", skill.ID, "resume-screener")
	}
	if skill.Name != "resume-screener" {
		t.Errorf("Name = %q, want %q", skill.Name, "resume-screener")
	}
	if skill.Description != "Screen resumes against a job description and produce structured feedback." {
		t.Errorf("Description = %q", skill.Description)
	}

	wantTags := map[string]bool{"official": true, "Red Hat ET": true, "read_file": true}
	for _, tag := range skill.Tags {
		if !wantTags[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
		delete(wantTags, tag)
	}
	for tag := range wantTags {
		t.Errorf("missing tag %q", tag)
	}
}

func TestToAgentSkillsFallback(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillMD := "---\nname: code-review\ndescription: Review code for bugs\n---\n# Code review\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	metas := []SkillMeta{
		{Name: "code-review", Description: "Review code for bugs", Dir: skillDir},
	}

	result := ToAgentSkills(metas)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].ID != "code-review" {
		t.Errorf("ID = %q, want %q", result[0].ID, "code-review")
	}
	if result[0].Description != "Review code for bugs" {
		t.Errorf("Description = %q", result[0].Description)
	}
	if len(result[0].Tags) != 0 {
		t.Errorf("Tags = %v, want empty", result[0].Tags)
	}
}

func TestToAgentSkillsEmpty(t *testing.T) {
	result := ToAgentSkills(nil)
	if len(result) != 0 {
		t.Fatalf("len(result) = %d, want 0", len(result))
	}
}

func TestToAgentSkillsMixed(t *testing.T) {
	dir := t.TempDir()

	// Skill with skill.yaml
	richDir := filepath.Join(dir, "rich-skill")
	if err := os.MkdirAll(richDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(richDir, "SKILL.md"), []byte("---\nname: rich-skill\ndescription: Fallback\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	richYAML := `apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: rich-skill
  namespace: community
  ref: quay.io/test/rich-skill
  version: 2.0.0
  description: Rich skill with full metadata.
  author: Test Author
`
	if err := os.WriteFile(filepath.Join(richDir, "skill.yaml"), []byte(richYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Skill without skill.yaml
	bareDir := filepath.Join(dir, "bare-skill")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareDir, "SKILL.md"), []byte("---\nname: bare-skill\ndescription: Just a SKILL.md\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	metas := []SkillMeta{
		{Name: "rich-skill", Description: "Fallback", Dir: richDir},
		{Name: "bare-skill", Description: "Just a SKILL.md", Dir: bareDir},
	}

	result := ToAgentSkills(metas)

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	// Rich skill gets SkillCard description and tags
	if result[0].Description != "Rich skill with full metadata." {
		t.Errorf("result[0].Description = %q", result[0].Description)
	}
	if len(result[0].Tags) == 0 {
		t.Error("result[0].Tags should not be empty")
	}

	// Bare skill gets SkillMeta description and no tags
	if result[1].Description != "Just a SKILL.md" {
		t.Errorf("result[1].Description = %q", result[1].Description)
	}
	if len(result[1].Tags) != 0 {
		t.Errorf("result[1].Tags = %v, want empty", result[1].Tags)
	}
}

func TestToAgentSkillsDedup(t *testing.T) {
	existing := []a2a.AgentSkill{
		{ID: "code-review", Name: "code-review", Description: "Static definition"},
	}

	discovered := []a2a.AgentSkill{
		{ID: "code-review", Name: "code-review", Description: "From discovery"},
		{ID: "url-summary", Name: "url-summary", Description: "Summarize URLs"},
	}

	merged := MergeSkills(existing, discovered)

	if len(merged) != 2 {
		t.Fatalf("len(merged) = %d, want 2", len(merged))
	}

	for _, s := range merged {
		if s.ID == "code-review" && s.Description != "Static definition" {
			t.Errorf("static entry should win: got Description = %q", s.Description)
		}
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./pkg/skills/ -run TestToAgentSkills -v 2>&1 | head -20
```

Expected: FAIL — `ToAgentSkills` and `MergeSkills` are not defined.

---

### Task 4: Implement ToAgentSkills converter

**Files:**
- Create: `pkg/skills/a2a.go`

- [ ] **Step 1: Write the converter**

Create `pkg/skills/a2a.go`:

```go
package skills

import (
	"path/filepath"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/redhat-et/docsclaw/pkg/skills/card"
)

// ToAgentSkills converts discovered SkillMeta entries into a2a.AgentSkill
// values suitable for inclusion in an AgentCard. If a skill directory
// contains a skill.yaml, its richer metadata is used; otherwise only
// the SKILL.md frontmatter fields are mapped.
func ToAgentSkills(metas []SkillMeta) []a2a.AgentSkill {
	out := make([]a2a.AgentSkill, 0, len(metas))

	for _, m := range metas {
		skill := a2a.AgentSkill{
			ID:          m.Name,
			Name:        m.Name,
			Description: m.Description,
		}

		cardPath := filepath.Join(m.Dir, "skill.yaml")
		if sc, err := card.Parse(cardPath); err == nil {
			skill.Description = sc.Metadata.Description
			skill.Tags = deriveTags(sc)
		}

		out = append(out, skill)
	}

	return out
}

// MergeSkills combines static AgentCard skills with discovered skills.
// Static entries win on ID conflict.
func MergeSkills(static, discovered []a2a.AgentSkill) []a2a.AgentSkill {
	seen := make(map[string]bool, len(static))
	merged := make([]a2a.AgentSkill, 0, len(static)+len(discovered))

	for _, s := range static {
		seen[s.ID] = true
		merged = append(merged, s)
	}

	for _, s := range discovered {
		if !seen[s.ID] {
			merged = append(merged, s)
		}
	}

	return merged
}

func deriveTags(sc card.SkillCard) []string {
	var tags []string

	if sc.Metadata.Namespace != "" {
		tags = append(tags, sc.Metadata.Namespace)
	}
	if sc.Metadata.Author != "" {
		tags = append(tags, sc.Metadata.Author)
	}
	for _, tool := range sc.Spec.Tools.Required {
		tags = append(tags, tool)
	}

	return tags
}
```

- [ ] **Step 2: Run the tests**

```bash
go test ./pkg/skills/ -run TestToAgentSkills -v
```

Expected: all five tests PASS.

- [ ] **Step 3: Run the full test suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 4: Run linter**

```bash
make lint
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/skills/a2a.go pkg/skills/a2a_test.go
git commit -s -m "feat: add ToAgentSkills converter for AgentCard population

Map discovered SkillMeta to a2a.AgentSkill with optional SkillCard
enrichment (tags from namespace, author, required tools). MergeSkills
handles deduplication with static entries winning on ID conflict.

Refs #39

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 5: Wire skill discovery into AgentCard in serve.go

**Files:**
- Modify: `internal/cmd/serve.go:307-322` (move skill discovery out of toolRegistry block)
- Modify: `internal/cmd/serve.go:389` (inject skills into agentCard before DynamicCardHandler)

Currently, skill discovery is inside the `if toolRegistry != nil` block (phase 2 only), but the AgentCard is served in both modes. We need to:

1. Move skill discovery before the `toolRegistry` block so it runs in both phases.
2. Inject discovered skills into the AgentCard before `DynamicCardHandler` is created.
3. Keep the tool registration and summary building inside the `toolRegistry` block.

- [ ] **Step 1: Refactor serve.go**

In `internal/cmd/serve.go`, replace the block from the `var toolRegistry` declaration through the end of the `if toolRegistry != nil` block (lines 213-327) with the restructured version. The key changes are:

1. Move skill discovery (`skills.Discover`) before the `toolRegistry` block.
2. Add `ToAgentSkills` + `MergeSkills` call to populate the AgentCard.
3. Keep tool registration and `BuildSummary` inside the `toolRegistry` block.

Replace the block starting at line 212 (`// Set up tool registry`) through line 327 (`}` closing the tools-enabled log) with:

```go
	// Set up tool registry and skill loading if agent config exists
	var toolRegistry *tools.Registry
	var loopCfg tools.LoopConfig
	var skillsSummary string

	if agentCfg != nil {
		toolRegistry = tools.NewRegistry(agentCfg.Tools.Allowed)

		workspace := agentCfg.Tools.Workspace
		if workspace == "" {
			workspace = "/tmp/agent-workspace"
		}
		if err := os.MkdirAll(workspace, 0755); err != nil {
			return fmt.Errorf("failed to create workspace: %w", err)
		}

		toolRegistry.Register(exec.NewExecTool(exec.ExecConfig{
			Timeout:   agentCfg.Tools.Exec.Timeout,
			MaxOutput: agentCfg.Tools.Exec.MaxOutput,
		}))
		toolRegistry.Register(webfetch.NewWebFetchTool(webfetch.WebFetchConfig{
			AllowedHosts: agentCfg.Tools.WebFetch.AllowedHosts,
		}))
		toolRegistry.Register(readfile.NewReadFileTool(workspace))
		toolRegistry.Register(writefile.NewWriteFileTool(workspace))

		loopCfg = agentCfg.toLoopConfig()
	}

	log := logger.New(logger.ComponentAgent)

	// HTTP client with delegation transport
	httpClient := &http.Client{
		Transport: &bridge.DelegationTransport{Base: http.DefaultTransport},
		Timeout:   30 * time.Second,
	}

	// Initialize LLM provider
	var llmProvider llm.Provider
	if cfg.LLM.APIKey != "" {
		llmProvider, err = llm.NewProvider(cfg.LLM)
		if err != nil {
			return fmt.Errorf("failed to create LLM provider: %w", err)
		}
		log.Info("LLM provider initialized",
			"provider", llmProvider.ProviderName(),
			"model", llmProvider.Model())
	} else {
		log.Warn("LLM API key not configured - will use mock responses")
	}

	// Discover skills (runs in both phase 1 and phase 2)
	skillsDir := filepath.Join(cfg.ConfigDir, "skills")
	discoveredSkills, err := skills.Discover(skillsDir)
	if err != nil {
		log.Warn("Failed to load skills", "error", err)
	}
	if len(discoveredSkills) > 0 {
		agentSkills := skills.ToAgentSkills(discoveredSkills)
		agentCard.Skills = skills.MergeSkills(agentCard.Skills, agentSkills)

		log.Info("Skills discovered",
			"count", len(discoveredSkills),
			"names", skillNames(discoveredSkills))
	}

	// Register additional tools and skills when in phase 2 mode
	if toolRegistry != nil {
		// Register fetch_document tool (uses delegation transport)
		toolRegistry.Register(fetchdoc.NewFetchDocTool(
			func(ctx context.Context, docID, token string) (map[string]any, error) {
				return fetchDocument(ctx, docID, token)
			},
		))

		// Register skill loading tool in phase 2
		if len(discoveredSkills) > 0 {
			skillsSummary = skills.BuildSummary(discoveredSkills)

			toolRegistry.RegisterAlwaysAllowed(&loadSkillTool{
				skillsDir: skillsDir,
			})
		}

		log.Info("Tools enabled",
			"allowed", len(toolRegistry.Definitions()),
			"max_iterations", loopCfg.MaxIterations)
	}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

Expected: SUCCESS.

- [ ] **Step 3: Run full tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 4: Run linter**

```bash
make lint
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/serve.go
git commit -s -m "feat: populate AgentCard skills from discovered skill files

Move skill discovery before the toolRegistry block so discovered
skills are injected into the AgentCard in both phase 1 and phase 2
modes. The A2A endpoint now reflects runtime-available skills.

Static skills from agent-card.json are preserved; discovered skills
are merged with static entries winning on ID conflict.

Closes #39

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 6: Final Verification

- [ ] **Step 1: Run full build, test, and lint**

```bash
make build && make test && make lint
```

Expected: all pass.

- [ ] **Step 2: Verify oras dependency is gone**

```bash
grep -E 'oras\.land|opencontainers/image-spec' go.mod
```

Expected: no output.

- [ ] **Step 3: Verify no references to deleted packages remain**

```bash
grep -rn 'internal/oci\|internal/verify' --include='*.go' .
```

Expected: no output.

- [ ] **Step 4: Review the git log**

```bash
git log --oneline main..HEAD
```

Expected: three commits (spec doc, OCI removal, converter, serve.go wiring) plus the design spec commit.
