package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type K8sClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	namespace  string
}

func NewK8sClient(namespace, kubeconfig string) (*K8sClient, error) {
	if kubeconfig != "" {
		return newFromKubeconfig(namespace, kubeconfig)
	}
	return newInCluster(namespace)
}

func newInCluster(namespace string) (*K8sClient, error) {
	token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, fmt.Errorf("read SA token: %w", err)
	}
	caCert, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read SA CA: %w", err)
	}
	if namespace == "" {
		ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return nil, fmt.Errorf("read SA namespace: %w", err)
		}
		namespace = string(ns)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	return &K8sClient{
		baseURL: "https://kubernetes.default.svc",
		token:   string(token),
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool},
			},
		},
		namespace: namespace,
	}, nil
}

func newFromKubeconfig(namespace, path string) (*K8sClient, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig: %w", err)
	}

	var kc struct {
		CurrentContext string `yaml:"current-context"`
		Clusters       []struct {
			Name    string `yaml:"name"`
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
		Contexts []struct {
			Name    string `yaml:"name"`
			Context struct {
				Cluster   string `yaml:"cluster"`
				User      string `yaml:"user"`
				Namespace string `yaml:"namespace"`
			} `yaml:"context"`
		} `yaml:"contexts"`
		Users []struct {
			Name string `yaml:"name"`
			User struct {
				Token string `yaml:"token"`
			} `yaml:"user"`
		} `yaml:"users"`
	}

	if err := yaml.Unmarshal(data, &kc); err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	// Find current context.
	var clusterName, userName string
	for _, ctx := range kc.Contexts {
		if ctx.Name == kc.CurrentContext {
			clusterName = ctx.Context.Cluster
			userName = ctx.Context.User
			if namespace == "" {
				namespace = ctx.Context.Namespace
			}
			break
		}
	}

	var server string
	for _, c := range kc.Clusters {
		if c.Name == clusterName {
			server = c.Cluster.Server
			break
		}
	}
	if server == "" {
		return nil, fmt.Errorf("no server found for context %q", kc.CurrentContext)
	}

	var token string
	for _, u := range kc.Users {
		if u.Name == userName {
			token = u.User.Token
			break
		}
	}

	slog.Info("kubeconfig loaded", "context", kc.CurrentContext, "server", server,
		"user", userName, "hasToken", token != "")

	return &K8sClient{
		baseURL: strings.TrimRight(server, "/"),
		token:   token,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		namespace: namespace,
	}, nil
}

func (c *K8sClient) do(method, path string, body io.Reader) ([]byte, int, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// CreateAgentDeployment creates a Deployment and Service for an agent instance.
func (c *K8sClient) CreateAgentDeployment(name, configMap, docServiceURL string, llmTimeout int) error {
	dep := buildDeploymentJSON(name, c.namespace, configMap, docServiceURL, llmTimeout)
	data, err := json.Marshal(dep)
	if err != nil {
		return fmt.Errorf("marshal deployment: %w", err)
	}

	path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments", c.namespace)
	_, code, err := c.do("POST", path, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	if code == 409 {
		slog.Info("deployment already exists", "name", name)
	} else if code >= 300 {
		return fmt.Errorf("create deployment %s: HTTP %d", name, code)
	}

	svc := buildServiceJSON(name, c.namespace)
	data, err = json.Marshal(svc)
	if err != nil {
		return fmt.Errorf("marshal service: %w", err)
	}

	path = fmt.Sprintf("/api/v1/namespaces/%s/services", c.namespace)
	_, code, err = c.do("POST", path, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if code == 409 {
		slog.Info("service already exists", "name", name)
	} else if code >= 300 {
		return fmt.Errorf("create service %s: HTTP %d", name, code)
	}

	return nil
}

// CreateRoute creates an OpenShift Route for an agent (needed for out-of-cluster access).
func (c *K8sClient) CreateRoute(name string) error {
	route := map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata": map[string]any{
			"name":      name,
			"namespace": c.namespace,
			"labels": map[string]string{
				"app":        name,
				"batch-role": "agent",
				"managed-by": "dashboard",
			},
			"annotations": map[string]string{
				"haproxy.router.openshift.io/timeout": "120s",
			},
		},
		"spec": map[string]any{
			"to":   map[string]any{"kind": "Service", "name": name},
			"port": map[string]any{"targetPort": "http"},
			"tls": map[string]any{
				"termination":                   "edge",
				"insecureEdgeTerminationPolicy": "Redirect",
			},
		},
	}

	data, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal route: %w", err)
	}

	path := fmt.Sprintf("/apis/route.openshift.io/v1/namespaces/%s/routes", c.namespace)
	_, code, err := c.do("POST", path, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("create route: %w", err)
	}
	if code == 409 {
		slog.Info("route already exists", "name", name)
	} else if code >= 300 {
		return fmt.Errorf("create route %s: HTTP %d", name, code)
	}
	return nil
}

// GetRouteHost returns the hostname assigned to a Route.
func (c *K8sClient) GetRouteHost(name string) (string, error) {
	path := fmt.Sprintf("/apis/route.openshift.io/v1/namespaces/%s/routes/%s", c.namespace, name)
	data, code, err := c.do("GET", path, nil)
	if err != nil {
		return "", err
	}
	if code >= 300 {
		return "", fmt.Errorf("get route %s: HTTP %d", name, code)
	}

	var route struct {
		Spec struct {
			Host string `json:"host"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &route); err != nil {
		return "", err
	}
	return route.Spec.Host, nil
}

// DeleteAgent removes the Deployment, Service, and Route for an agent instance.
func (c *K8sClient) DeleteAgent(name string) error {
	path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", c.namespace, name)
	_, code, err := c.do("DELETE", path, nil)
	if err != nil {
		return err
	}
	if code >= 300 && code != 404 {
		return fmt.Errorf("delete deployment %s: HTTP %d", name, code)
	}

	path = fmt.Sprintf("/api/v1/namespaces/%s/services/%s", c.namespace, name)
	_, code, err = c.do("DELETE", path, nil)
	if err != nil {
		return err
	}
	if code >= 300 && code != 404 {
		return fmt.Errorf("delete service %s: HTTP %d", name, code)
	}

	// Best-effort Route cleanup.
	path = fmt.Sprintf("/apis/route.openshift.io/v1/namespaces/%s/routes/%s", c.namespace, name)
	c.do("DELETE", path, nil)

	return nil
}

// PodInfo holds basic pod status information.
type PodInfo struct {
	Name   string
	Phase  string
	Ready  bool
}

// ListPods returns pods matching a label selector.
func (c *K8sClient) ListPods(labelSelector string) ([]PodInfo, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods?labelSelector=%s", c.namespace, labelSelector)
	data, code, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("list pods: HTTP %d", code)
	}

	var resp struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Phase            string `json:"phase"`
				ContainerStatuses []struct {
					Ready bool `json:"ready"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var pods []PodInfo
	for _, item := range resp.Items {
		ready := len(item.Status.ContainerStatuses) > 0 && item.Status.ContainerStatuses[0].Ready
		pods = append(pods, PodInfo{
			Name:  item.Metadata.Name,
			Phase: item.Status.Phase,
			Ready: ready,
		})
	}
	return pods, nil
}

// PodMetrics holds resource usage for a pod.
type PodMetrics struct {
	Name      string
	MemoryMiB float64
	CPUmcpu   float64
}

// GetPodMetrics returns resource usage for pods matching a label selector.
func (c *K8sClient) GetPodMetrics(labelSelector string) ([]PodMetrics, error) {
	path := fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods?labelSelector=%s",
		c.namespace, labelSelector)
	data, code, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("get pod metrics: HTTP %d", code)
	}

	var resp struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Containers []struct {
				Usage struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"usage"`
			} `json:"containers"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var metrics []PodMetrics
	for _, item := range resp.Items {
		var mem, cpu float64
		for _, c := range item.Containers {
			mem += parseMemory(c.Usage.Memory)
			cpu += parseCPU(c.Usage.CPU)
		}
		metrics = append(metrics, PodMetrics{
			Name:      item.Metadata.Name,
			MemoryMiB: mem,
			CPUmcpu:   cpu,
		})
	}
	return metrics, nil
}

// GetPodLogs returns recent logs for a pod.
func (c *K8sClient) GetPodLogs(podName string, sinceSeconds int) (string, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log?sinceSeconds=%d",
		c.namespace, podName, sinceSeconds)
	data, code, err := c.do("GET", path, nil)
	if err != nil {
		return "", err
	}
	if code >= 300 {
		return "", fmt.Errorf("get logs for %s: HTTP %d", podName, code)
	}
	return string(data), nil
}

// TokenUsage holds aggregated token counts from log parsing.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// ParseTokensFromLogs extracts token counts from agent log lines.
var tokenLogRe = regexp.MustCompile(`input_tokens=(\d+)\s+output_tokens=(\d+)`)

func ParseTokensFromLogs(logs string) TokenUsage {
	var usage TokenUsage
	for _, match := range tokenLogRe.FindAllStringSubmatch(logs, -1) {
		if len(match) >= 3 {
			in, _ := strconv.Atoi(match[1])
			out, _ := strconv.Atoi(match[2])
			usage.InputTokens += in
			usage.OutputTokens += out
		}
	}
	return usage
}

// parseMemory converts K8s memory strings (e.g. "12345Ki") to MiB.
func parseMemory(s string) float64 {
	s = strings.TrimSpace(s)
	if v, ok := strings.CutSuffix(s, "Ki"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f / 1024.0
	}
	if v, ok := strings.CutSuffix(s, "Mi"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	if v, ok := strings.CutSuffix(s, "Gi"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f * 1024.0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f / (1024 * 1024)
}

// parseCPU converts K8s CPU strings (e.g. "8m", "100n") to millicores.
func parseCPU(s string) float64 {
	s = strings.TrimSpace(s)
	if v, ok := strings.CutSuffix(s, "n"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f / 1e6
	}
	if v, ok := strings.CutSuffix(s, "m"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f * 1000
}

// buildDeploymentJSON constructs the K8s Deployment object.
func buildDeploymentJSON(name, namespace, configMap, docServiceURL string, llmTimeout int) map[string]any {
	args := []any{
		"serve",
		"--config-dir", "/config/agent",
		"--listen-plain-http",
		"--document-service-url", docServiceURL,
		"--session-db", "/tmp/agent-workspace/sessions.db",
	}
	if llmTimeout > 0 {
		args = append(args, "--llm-timeout", fmt.Sprintf("%d", llmTimeout))
	}

	return map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]string{
				"app":        name,
				"batch-role": "agent",
				"managed-by": "dashboard",
			},
		},
		"spec": map[string]any{
			"replicas": 1,
			"selector": map[string]any{
				"matchLabels": map[string]string{"app": name},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{
						"app":        name,
						"batch-role": "agent",
						"managed-by": "dashboard",
					},
				},
				"spec": map[string]any{
					"securityContext": map[string]any{"runAsNonRoot": true},
					"containers": []map[string]any{{
						"name":  "docsclaw",
						"image": "ghcr.io/redhat-et/docsclaw:latest",
						"args":  args,
						"ports": []map[string]any{
							{"containerPort": 8000, "name": "http"},
							{"containerPort": 8100, "name": "health"},
						},
						"envFrom": []map[string]any{
							{"secretRef": map[string]string{"name": "llm-secret"}},
						},
						"resources": map[string]any{
							"requests": map[string]string{"cpu": "50m", "memory": "32Mi"},
							"limits":   map[string]string{"cpu": "500m", "memory": "256Mi"},
						},
						"livenessProbe": map[string]any{
							"httpGet":             map[string]any{"path": "/health", "port": "health"},
							"initialDelaySeconds": 5,
						},
						"readinessProbe": map[string]any{
							"httpGet":             map[string]any{"path": "/ready", "port": "health"},
							"initialDelaySeconds": 3,
						},
						"volumeMounts": []map[string]any{
							{"name": "agent-config", "mountPath": "/config/agent", "readOnly": true},
							{"name": "workspace", "mountPath": "/tmp/agent-workspace"},
						},
						"securityContext": map[string]any{
							"allowPrivilegeEscalation": false,
							"capabilities":             map[string]any{"drop": []string{"ALL"}},
							"readOnlyRootFilesystem":   true,
						},
					}},
					"volumes": []map[string]any{
						{"name": "agent-config", "configMap": map[string]string{"name": configMap}},
						{"name": "workspace", "emptyDir": map[string]any{}},
					},
				},
			},
		},
	}
}

// buildServiceJSON constructs the K8s Service object.
func buildServiceJSON(name, namespace string) map[string]any {
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]string{
				"app":        name,
				"batch-role": "agent",
				"managed-by": "dashboard",
			},
		},
		"spec": map[string]any{
			"selector": map[string]string{"app": name},
			"ports": []map[string]any{{
				"port":       8080,
				"targetPort": 8000,
				"name":       "http",
			}},
		},
	}
}
