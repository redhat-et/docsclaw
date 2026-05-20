package manifest

import (
	"testing"

	"github.com/redhat-et/docsclaw/pkg/catalog"
	skillcard "github.com/redhat-et/docsclaw/pkg/skills/card"
)

func TestCheckCompatibility_AllSatisfied(t *testing.T) {
	tools := []string{"curl", "jq"}
	skills := []SkillCheck{
		{Name: "nps-api", Required: []string{"curl", "jq"}, HasSkillYAML: true},
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
		{Name: "doc-converter", Required: []string{"curl", "pandoc"}, HasSkillYAML: true},
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
			Name:         "nps-api",
			Required:     []string{"curl", "jq"},
			Optional:     []string{"python3"},
			HasSkillYAML: true,
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
