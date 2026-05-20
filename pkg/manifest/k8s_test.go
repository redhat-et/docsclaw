package manifest

import (
	"strings"
	"testing"
)

func TestGenerateK8s_ConfigMap(t *testing.T) {
	m := testManifest()
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("GenerateK8s() error: %v", err)
	}

	if !strings.Contains(k8s.ConfigMap, "kind: ConfigMap") {
		t.Error("missing ConfigMap kind")
	}
	if !strings.Contains(k8s.ConfigMap, "name: nps-assistant-config") {
		t.Error("missing ConfigMap name")
	}
	if !strings.Contains(k8s.ConfigMap, "system-prompt.txt") {
		t.Error("missing system-prompt.txt key")
	}
	if !strings.Contains(k8s.ConfigMap, "agent-config.yaml") {
		t.Error("missing agent-config.yaml key")
	}
}

func TestGenerateK8s_Deployment(t *testing.T) {
	m := testManifest()
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(k8s.Deployment, "kind: Deployment") {
		t.Error("missing Deployment kind")
	}
	if !strings.Contains(k8s.Deployment, "runAsNonRoot: true") {
		t.Error("missing security context")
	}
	if !strings.Contains(k8s.Deployment, "skill-nps-api") {
		t.Error("missing skill volume")
	}
}

func TestGenerateK8s_Secret(t *testing.T) {
	m := testManifest()
	secrets := map[string]string{
		"NPS_API_KEY": "test-key",
		"LLM_API_KEY": "test-llm",
	}
	k8s, err := GenerateK8s(m, secrets)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if k8s.Secret == "" {
		t.Fatal("secret should be generated when values provided")
	}
	if !strings.Contains(k8s.Secret, "kind: Secret") {
		t.Error("missing Secret kind")
	}
}

func TestGenerateK8s_NoSecrets(t *testing.T) {
	m := testManifest()
	m.Spec.Secrets = nil
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if k8s.Secret != "" {
		t.Error("should not generate secret when none declared")
	}
}

func TestGenerateK8s_Service(t *testing.T) {
	m := testManifest()
	k8s, err := GenerateK8s(m, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(k8s.Service, "kind: Service") {
		t.Error("missing Service kind")
	}
	if !strings.Contains(k8s.Service, "port: 8000") {
		t.Error("missing http port")
	}
}

func testManifest() *AgentManifest {
	return &AgentManifest{
		Metadata: ManifestMeta{Name: "nps-assistant", Version: "1.0.0"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image: "ghcr.io/redhat-et/docsclaw:latest",
			},
			Tools: []string{"curl", "jq"},
			Prompt: PromptConfig{
				Text: "You are a national parks assistant.",
			},
			Skills: []SkillRef{
				{Name: "nps-api", Image: "quay.io/docsclaw/skill-nps-api:1.0.0-image"},
			},
			Runtime: RuntimeConfig{
				Tools: RuntimeToolsConfig{
					Allowed: []string{"exec", "read_file", "load_skill"},
					Exec:    ExecConfig{Timeout: 30, MaxOutput: 50000},
				},
				Loop: RuntimeLoopConfig{MaxIterations: 15},
			},
			Secrets: []SecretDecl{
				{Name: "NPS_API_KEY", Required: true},
				{Name: "LLM_API_KEY", Required: true},
			},
			Deploy: DeployConfig{
				Replicas: 1,
				Resources: ResourceConfig{
					Requests: ResourceValues{CPU: "100m", Memory: "64Mi"},
					Limits:   ResourceValues{CPU: "500m", Memory: "256Mi"},
				},
			},
		},
	}
}
