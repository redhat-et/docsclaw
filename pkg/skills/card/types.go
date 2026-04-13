// Package card defines the SkillCard schema for OCI-distributed skills.
package card

// SkillCard is the top-level type for skill.yaml files.
type SkillCard struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   SkillCardMeta `yaml:"metadata"`
	Spec       SkillCardSpec `yaml:"spec"`
}

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

type SkillCardSpec struct {
	Tools         ToolDeps      `yaml:"tools,omitempty"`
	AllowedTools  string        `yaml:"allowedTools,omitempty"`
	Dependencies  Dependencies  `yaml:"dependencies,omitempty"`
	Resources     ResourceHints `yaml:"resources,omitempty"`
	Compatibility Compatibility `yaml:"compatibility,omitempty"`
}

type ToolDeps struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

type Dependencies struct {
	Skills    []string      `yaml:"skills,omitempty"`
	ToolPacks []ToolPackRef `yaml:"toolPacks,omitempty"`
}

type ToolPackRef struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"ref"`
}

type ResourceHints struct {
	EstimatedMemory string `yaml:"estimatedMemory,omitempty"`
	EstimatedCPU    string `yaml:"estimatedCPU,omitempty"`
}

type Compatibility struct {
	MinAgentVersion string `yaml:"minAgentVersion,omitempty"`
	Environment     string `yaml:"environment,omitempty"`
}
