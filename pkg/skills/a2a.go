package skills

import (
	"os"
	"path/filepath"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"gopkg.in/yaml.v3"
)

// skillYAML is a loose schema for extracting AgentSkill-relevant
// fields from any skill.yaml, regardless of apiVersion.
type skillYAML struct {
	Metadata struct {
		Name        string   `yaml:"name"`
		Namespace   string   `yaml:"namespace"`
		Description string   `yaml:"description"`
		Author      string   `yaml:"author"`
		Tags        []string `yaml:"tags"`
		Authors     []struct {
			Name string `yaml:"name"`
		} `yaml:"authors"`
	} `yaml:"metadata"`
	Spec struct {
		Tools struct {
			Required []string `yaml:"required"`
		} `yaml:"tools"`
		Examples []struct {
			Input string `yaml:"input"`
		} `yaml:"examples"`
	} `yaml:"spec"`
}

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
			Tags:        []string{},
		}

		cardPath := filepath.Join(m.Dir, "skill.yaml")
		if sy, err := parseSkillYAML(cardPath); err == nil {
			if sy.Metadata.Description != "" {
				skill.Description = sy.Metadata.Description
			}
			skill.Tags = deriveTags(sy)
			skill.Examples = deriveExamples(sy)
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

func parseSkillYAML(path string) (skillYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillYAML{}, err
	}
	var sy skillYAML
	if err := yaml.Unmarshal(data, &sy); err != nil {
		return skillYAML{}, err
	}
	return sy, nil
}

func deriveTags(sy skillYAML) []string {
	// Prefer explicit tags (skillimage format)
	if len(sy.Metadata.Tags) > 0 {
		return sy.Metadata.Tags
	}

	// Fall back to deriving from namespace/author/tools (docsclaw format)
	var tags []string
	if sy.Metadata.Namespace != "" {
		tags = append(tags, sy.Metadata.Namespace)
	}
	if sy.Metadata.Author != "" {
		tags = append(tags, sy.Metadata.Author)
	} else if len(sy.Metadata.Authors) > 0 && sy.Metadata.Authors[0].Name != "" {
		tags = append(tags, sy.Metadata.Authors[0].Name)
	}
	tags = append(tags, sy.Spec.Tools.Required...)

	if len(tags) == 0 {
		return []string{}
	}
	return tags
}

func deriveExamples(sy skillYAML) []string {
	if len(sy.Spec.Examples) == 0 {
		return nil
	}
	examples := make([]string, 0, len(sy.Spec.Examples))
	for _, e := range sy.Spec.Examples {
		if e.Input != "" {
			examples = append(examples, e.Input)
		}
	}
	if len(examples) == 0 {
		return nil
	}
	return examples
}
