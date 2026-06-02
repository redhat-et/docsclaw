package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test-skill\ndescription: A test skill for unit testing\n---\n\n# Test skill\n\nDo something useful.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "test-skill" {
		t.Fatalf("expected name test-skill, got %q", skills[0].Name)
	}
	if skills[0].Description == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestDiscoverSkillsEmpty(t *testing.T) {
	dir := t.TempDir()
	skills, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}

func TestDiscoverSkillsNoDir(t *testing.T) {
	skills, err := Discover("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}

func TestLoadSkillContent(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: my-skill\ndescription: My test skill\n---\n\n# My skill\n\nStep 1: Do this\nStep 2: Do that\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadContent(dir, "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty content")
	}
	if len(result) < 20 {
		t.Fatalf("content too short: %q", result)
	}
}

func TestLoadSkillContentNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadContent(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}

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

func TestDiscoverNestedSkills(t *testing.T) {
	dir := t.TempDir()

	for _, path := range []string{
		"static/code-review",
		"static/url-summary",
		"dynamic/fetched-skill",
	} {
		skillDir := filepath.Join(dir, path)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		content := "---\nname: " + name + "\ndescription: test\n---\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	skills, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}

	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
	}
	for _, want := range []string{"code-review", "url-summary", "fetched-skill"} {
		if !names[want] {
			t.Errorf("missing skill %q", want)
		}
	}
}

func TestDiscoverSkipsK8sDataSymlink(t *testing.T) {
	dir := t.TempDir()

	// Simulate K8s ConfigMap mount: real file in ..data, symlink in skill dir
	skillDir := filepath.Join(dir, "my-skill")
	dataDir := filepath.Join(skillDir, "..data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "---\nname: my-skill\ndescription: test\n---\n"
	if err := os.WriteFile(filepath.Join(dataDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Symlink SKILL.md -> ..data/SKILL.md (like K8s does)
	if err := os.Symlink("..data/SKILL.md", filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	skills, err := Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should find exactly 1, not 2 (the ..data copy should be skipped)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "my-skill" {
		t.Errorf("name = %q, want %q", skills[0].Name, "my-skill")
	}
}

func TestLoadContentRecursive(t *testing.T) {
	dir := t.TempDir()

	// Skill nested under static/
	skillDir := filepath.Join(dir, "static", "deep-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: deep-skill\ndescription: nested\n---\n# Deep\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadContent(dir, "deep-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "deep-skill") {
		t.Errorf("result = %q, want to contain 'deep-skill'", result)
	}
}

func TestBuildSkillSummary(t *testing.T) {
	skills := []SkillMeta{
		{Name: "code-review", Description: "Review code for bugs"},
		{Name: "pdf-summary", Description: "Convert and summarize PDFs"},
	}

	summary := BuildSummary(skills)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(summary, "code-review") {
		t.Fatal("expected code-review in summary")
	}
	if !strings.Contains(summary, "pdf-summary") {
		t.Fatal("expected pdf-summary in summary")
	}
}
