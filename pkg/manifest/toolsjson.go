package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

type ToolsJSON struct {
	ManifestVersion string          `json:"manifestVersion"`
	AgentName       string          `json:"agentName"`
	Base            string          `json:"base"`
	HighestTier     string          `json:"highestTier"`
	RiskScore       int             `json:"riskScore"`
	Tools           []ToolJSONEntry `json:"tools"`
}

type ToolJSONEntry struct {
	Name    string       `json:"name"`
	Package string       `json:"package"`
	Tier    string       `json:"tier"`
	Risk    ToolJSONRisk `json:"risk"`
}

type ToolJSONRisk struct {
	Score          int  `json:"score"`
	CodeExecution  bool `json:"codeExecution"`
	NetworkCapable bool `json:"networkCapable"`
}

func GenerateToolsJSON(m *AgentManifest, cat *catalog.ToolCatalog) ([]byte, error) {
	allTools := MergeWithCore(m.Spec.Tools, cat)
	sort.Strings(allTools)

	tj := ToolsJSON{
		ManifestVersion: m.Metadata.Version,
		AgentName:       m.Metadata.Name,
		Base:            m.Spec.Base.Image,
		HighestTier:     cat.HighestTier(allTools),
		RiskScore:       cat.MaxRiskScore(allTools),
	}

	for _, name := range allTools {
		entry, ok := cat.Lookup(name)
		if !ok {
			continue
		}
		tj.Tools = append(tj.Tools, ToolJSONEntry{
			Name:    name,
			Package: entry.Package["dnf"],
			Tier:    entry.Tier,
			Risk: ToolJSONRisk{
				Score:          entry.Risk.Score,
				CodeExecution:  entry.Risk.Factors.CodeExecution,
				NetworkCapable: entry.Risk.Factors.NetworkCapable,
			},
		})
	}

	data, err := json.MarshalIndent(tj, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tools.json: %w", err)
	}
	return append(data, '\n'), nil
}
