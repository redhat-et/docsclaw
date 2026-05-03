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

func TestToAgentSkillsSkillimageFormat(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "document-summarizer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillMD := "---\nname: document-summarizer\ndescription: From SKILL.md\n---\n# Summarizer\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	skillYAML := `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: document-summarizer
  namespace: business
  version: 1.0.0
  description: Summarizes technical documents into actionable summaries.
  tags:
    - summarization
    - documents
    - productivity
  authors:
    - name: OCTO Team
      email: octo@redhat.com
spec:
  prompt: SKILL.md
  examples:
    - input: "Summarize this design doc for the team standup."
    - input: "Extract action items from the architecture review."
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	metas := []SkillMeta{
		{Name: "document-summarizer", Description: "From SKILL.md", Dir: skillDir},
	}

	result := ToAgentSkills(metas)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}

	skill := result[0]
	if skill.Description != "Summarizes technical documents into actionable summaries." {
		t.Errorf("Description = %q", skill.Description)
	}

	wantTags := []string{"summarization", "documents", "productivity"}
	if len(skill.Tags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", skill.Tags, wantTags)
	}
	for i, tag := range wantTags {
		if skill.Tags[i] != tag {
			t.Errorf("Tags[%d] = %q, want %q", i, skill.Tags[i], tag)
		}
	}

	if len(skill.Examples) != 2 {
		t.Fatalf("Examples = %v, want 2 entries", skill.Examples)
	}
	if skill.Examples[0] != "Summarize this design doc for the team standup." {
		t.Errorf("Examples[0] = %q", skill.Examples[0])
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
