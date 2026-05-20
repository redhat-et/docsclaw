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
