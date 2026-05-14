package main

import "fmt"

type AgentAssignment struct {
	Name       string
	Label      string
	DocumentID string
	Prompt     string
}

type Scenario struct {
	Name       string
	Title      string
	ConfigMap  string
	DocService string
	LLMTimeout int
	Agents     []AgentAssignment
}

func FinanceScenario(namespace string) Scenario {
	contracts := []struct {
		vendor string
		id     string
	}{
		{"CloudCompute Inc", "DOC-CTR-V001"},
		{"StorageMax Solutions", "DOC-CTR-V002"},
		{"BandwidthPro", "DOC-CTR-V003"},
		{"SupportFirst", "DOC-CTR-V004"},
		{"DataCenter Co", "DOC-CTR-V005"},
	}

	agents := make([]AgentAssignment, len(contracts))
	for i, c := range contracts {
		num := fmt.Sprintf("%03d", i+1)
		agents[i] = AgentAssignment{
			Name:       "finance-analyst-" + num,
			Label:      c.vendor,
			DocumentID: c.id,
			Prompt:     "run analysis for document " + c.id,
		}
	}

	return Scenario{
		Name:       "finance",
		Title:      "Invoice Anomaly Detection",
		ConfigMap:  "finance-analyst-config",
		DocService: fmt.Sprintf("http://document-service.%s.svc:8080", namespace),
		LLMTimeout: 90,
		Agents:     agents,
	}
}

func SecurityScenario(namespace string) Scenario {
	return Scenario{
		Name:       "security",
		Title:      "Vulnerability Triage",
		ConfigMap:  "security-analyst-config",
		DocService: fmt.Sprintf("http://document-service.%s.svc:8080", namespace),
		LLMTimeout: 120,
		Agents: []AgentAssignment{
			{
				Name:   "security-analyst",
				Label:  "Vuln scan + asset inventory",
				Prompt: "triage the vulnerability scan report",
			},
		},
	}
}

func HRScenario(namespace string) Scenario {
	return Scenario{
		Name:       "hr",
		Title:      "Resume Screening",
		ConfigMap:  "hr-analyst-config",
		DocService: fmt.Sprintf("http://document-service.%s.svc:8080", namespace),
		LLMTimeout: 90,
		Agents:     nil, // placeholder
	}
}

func AllScenarios(namespace string) map[string]Scenario {
	return map[string]Scenario{
		"finance":  FinanceScenario(namespace),
		"security": SecurityScenario(namespace),
		"hr":       HRScenario(namespace),
	}
}
