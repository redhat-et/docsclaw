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
