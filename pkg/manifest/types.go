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
	Base    BaseImage     `yaml:"base"`
	Tools   []string      `yaml:"tools"`
	Prompt  PromptConfig  `yaml:"prompt"`
	Skills  []SkillRef    `yaml:"skills"`
	Runtime RuntimeConfig `yaml:"runtime,omitempty"`
	Secrets []SecretDecl  `yaml:"secrets,omitempty"`
	Deploy  DeployConfig  `yaml:"deploy,omitempty"`
}

type BaseImage struct {
	Image       string `yaml:"image"`
	GoBuilder   string `yaml:"goBuilder,omitempty"`
	ToolBuilder string `yaml:"toolBuilder"`
}

type PromptConfig struct {
	Text   string        `yaml:"text,omitempty"`
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
	Allowed  []string       `yaml:"allowed"`
	Exec     ExecConfig     `yaml:"exec,omitempty"`
	WebFetch WebFetchConfig `yaml:"webFetch,omitempty"`
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
	Replicas  int            `yaml:"replicas,omitempty"`
	Resources ResourceConfig `yaml:"resources,omitempty"`
}

type ResourceConfig struct {
	Requests ResourceValues `yaml:"requests,omitempty"`
	Limits   ResourceValues `yaml:"limits,omitempty"`
}

type ResourceValues struct {
	CPU    string `yaml:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}
