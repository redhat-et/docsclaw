package manifest

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"
)

type K8sOutput struct {
	ConfigMap      string
	Deployment     string
	Service        string
	ServiceAccount string
	Secret         string
}

func GenerateK8s(m *AgentManifest, secrets map[string]string) (*K8sOutput, error) {
	output := &K8sOutput{}

	// Generate agent-config.yaml content
	agentConfigYAML, err := buildAgentConfigYAML(m)
	if err != nil {
		return nil, fmt.Errorf("build agent-config.yaml: %w", err)
	}

	// ConfigMap
	cmData := struct {
		Name            string
		SystemPrompt    string
		AgentConfigYAML string
	}{
		Name:            m.Metadata.Name,
		SystemPrompt:    m.Spec.Prompt.Text,
		AgentConfigYAML: agentConfigYAML,
	}

	var cmBuf bytes.Buffer
	if err := configMapTemplate.Execute(&cmBuf, cmData); err != nil {
		return nil, fmt.Errorf("execute configmap template: %w", err)
	}
	output.ConfigMap = cmBuf.String()

	// Deployment
	agentImage := m.Spec.Deploy.Image
	if agentImage == "" {
		agentImage = m.Spec.Base.Image
	}

	deployData := struct {
		Name       string
		AgentImage string
		Skills     []SkillRef
		Replicas   int
		Resources  ResourceConfig
		HasSecrets bool
	}{
		Name:       m.Metadata.Name,
		AgentImage: agentImage,
		Skills:     m.Spec.Skills,
		Replicas:   m.Spec.Deploy.Replicas,
		Resources:  m.Spec.Deploy.Resources,
		HasSecrets: len(secrets) > 0,
	}
	if deployData.Replicas == 0 {
		deployData.Replicas = 1
	}

	var deployBuf bytes.Buffer
	if err := deploymentTemplate.Execute(&deployBuf, deployData); err != nil {
		return nil, fmt.Errorf("execute deployment template: %w", err)
	}
	output.Deployment = deployBuf.String()

	// Service
	svcData := struct {
		Name string
	}{
		Name: m.Metadata.Name,
	}

	var svcBuf bytes.Buffer
	if err := serviceTemplate.Execute(&svcBuf, svcData); err != nil {
		return nil, fmt.Errorf("execute service template: %w", err)
	}
	output.Service = svcBuf.String()

	// ServiceAccount
	saData := struct {
		Name string
	}{
		Name: "docsclaw-agent",
	}

	var saBuf bytes.Buffer
	if err := serviceAccountTemplate.Execute(&saBuf, saData); err != nil {
		return nil, fmt.Errorf("execute serviceaccount template: %w", err)
	}
	output.ServiceAccount = saBuf.String()

	// Secret (only if secrets map provided)
	if len(secrets) > 0 {
		secretData := struct {
			Name    string
			Secrets map[string]string
		}{
			Name:    m.Metadata.Name,
			Secrets: encodeSecrets(secrets),
		}

		var secretBuf bytes.Buffer
		if err := secretTemplate.Execute(&secretBuf, secretData); err != nil {
			return nil, fmt.Errorf("execute secret template: %w", err)
		}
		output.Secret = secretBuf.String()
	}

	return output, nil
}

func buildAgentConfigYAML(m *AgentManifest) (string, error) {
	var buf strings.Builder

	rt := m.Spec.Runtime

	buf.WriteString("tools:\n")
	buf.WriteString("  allowed:\n")
	for _, t := range rt.Tools.Allowed {
		fmt.Fprintf(&buf, "    - %s\n", t)
	}

	if rt.Tools.Exec.Timeout > 0 || rt.Tools.Exec.MaxOutput > 0 {
		buf.WriteString("  exec:\n")
		if rt.Tools.Exec.Timeout > 0 {
			fmt.Fprintf(&buf, "    timeout: %d\n", rt.Tools.Exec.Timeout)
		}
		if rt.Tools.Exec.MaxOutput > 0 {
			fmt.Fprintf(&buf, "    maxOutput: %d\n", rt.Tools.Exec.MaxOutput)
		}
	}

	if rt.Loop.MaxIterations > 0 {
		buf.WriteString("loop:\n")
		fmt.Fprintf(&buf, "  maxIterations: %d\n", rt.Loop.MaxIterations)
	}

	return buf.String(), nil
}

func encodeSecrets(secrets map[string]string) map[string]string {
	encoded := make(map[string]string, len(secrets))
	for k, v := range secrets {
		encoded[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return encoded
}

// nindent indents each line of text by n spaces
func nindent(n int, text string) string {
	indent := strings.Repeat(" ", n)
	lines := strings.Split(text, "\n")
	var result strings.Builder
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		if line != "" {
			result.WriteString(indent)
			result.WriteString(line)
		}
	}
	return result.String()
}

var configMapTemplate = template.Must(template.New("configmap").Funcs(template.FuncMap{
	"nindent": nindent,
}).Parse(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.Name}}-config
  labels:
    app: {{.Name}}
data:
  system-prompt.txt: |
    {{.SystemPrompt}}
  agent-config.yaml: |
{{.AgentConfigYAML | nindent 4}}
`))

var deploymentTemplate = template.Must(template.New("deployment").Funcs(template.FuncMap{
	"nindent": nindent,
}).Parse(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.Name}}
  labels:
    app: {{.Name}}
spec:
  replicas: {{.Replicas}}
  selector:
    matchLabels:
      app: {{.Name}}
  template:
    metadata:
      labels:
        app: {{.Name}}
    spec:
      serviceAccountName: docsclaw-agent
      securityContext:
        runAsNonRoot: true
      containers:
      - name: agent
        image: {{.AgentImage}}
        args:
        - serve
        - --config-dir
        - /config/agent
        - --listen-plain-http
        ports:
        - name: http
          containerPort: 8000
          protocol: TCP
        - name: health
          containerPort: 8100
          protocol: TCP
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
{{- if .HasSecrets}}
        envFrom:
        - secretRef:
            name: {{.Name}}-secrets
{{- end}}
        volumeMounts:
        - name: agent-config
          mountPath: /config/agent
          readOnly: true
{{- range .Skills}}
        - name: skill-{{.Name}}
          mountPath: /config/agent/skills/{{.Name}}
          readOnly: true
{{- end}}
        - name: tmp
          mountPath: /tmp
{{- if or .Resources.Requests.CPU .Resources.Requests.Memory .Resources.Limits.CPU .Resources.Limits.Memory}}
        resources:
{{- if or .Resources.Requests.CPU .Resources.Requests.Memory}}
          requests:
{{- if .Resources.Requests.CPU}}
            cpu: {{.Resources.Requests.CPU}}
{{- end}}
{{- if .Resources.Requests.Memory}}
            memory: {{.Resources.Requests.Memory}}
{{- end}}
{{- end}}
{{- if or .Resources.Limits.CPU .Resources.Limits.Memory}}
          limits:
{{- if .Resources.Limits.CPU}}
            cpu: {{.Resources.Limits.CPU}}
{{- end}}
{{- if .Resources.Limits.Memory}}
            memory: {{.Resources.Limits.Memory}}
{{- end}}
{{- end}}
{{- end}}
        livenessProbe:
          httpGet:
            path: /health
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: health
          initialDelaySeconds: 3
          periodSeconds: 5
      volumes:
      - name: agent-config
        configMap:
          name: {{.Name}}-config
{{- range .Skills}}
      - name: skill-{{.Name}}
        image:
          reference: {{.Image}}
{{- end}}
      - name: tmp
        emptyDir: {}
`))

var serviceTemplate = template.Must(template.New("service").Parse(`apiVersion: v1
kind: Service
metadata:
  name: {{.Name}}
  labels:
    app: {{.Name}}
spec:
  selector:
    app: {{.Name}}
  ports:
  - name: http
    port: 8000
    targetPort: http
    protocol: TCP
  - name: health
    port: 8100
    targetPort: health
    protocol: TCP
`))

var serviceAccountTemplate = template.Must(template.New("serviceaccount").Parse(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.Name}}
  labels:
    app: docsclaw
`))

var secretTemplate = template.Must(template.New("secret").Parse(`apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}-secrets
  labels:
    app: {{.Name}}
type: Opaque
data:
{{- range $key, $value := .Secrets}}
  {{$key}}: {{$value}}
{{- end}}
`))

