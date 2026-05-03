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
	tags = append(tags, sc.Spec.Tools.Required...)

	return tags
}
