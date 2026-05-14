package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

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
	allResumes := make([]string, 100)
	for i := range 100 {
		allResumes[i] = fmt.Sprintf("DOC-R%03d", i+1)
	}
	rand.Shuffle(len(allResumes), func(i, j int) {
		allResumes[i], allResumes[j] = allResumes[j], allResumes[i]
	})

	agents := make([]AgentAssignment, 10)
	for i := range 10 {
		num := fmt.Sprintf("%03d", i+1)
		resumeIDs := allResumes[i*10 : (i+1)*10]

		agents[i] = AgentAssignment{
			Name:       "hr-screener-" + num,
			Label:      fmt.Sprintf("Batch %s (10 resumes)", num),
			DocumentID: resumeIDs[0],
			Prompt: fmt.Sprintf(
				"Evaluate resumes %s against job description DOC-JD001. Fetch the job description first, then fetch and score each resume.",
				joinIDs(resumeIDs),
			),
		}
	}

	return Scenario{
		Name:       "hr",
		Title:      "Resume Screening",
		ConfigMap:  "hr-screener-config",
		DocService: fmt.Sprintf("http://document-service.%s.svc:8080", namespace),
		LLMTimeout: 120,
		Agents:     agents,
	}
}

func joinIDs(ids []string) string {
	return strings.Join(ids, ", ")
}

func AllScenarios(namespace string) map[string]Scenario {
	return map[string]Scenario{
		"finance":  FinanceScenario(namespace),
		"security": SecurityScenario(namespace),
		"hr":       HRScenario(namespace),
	}
}
