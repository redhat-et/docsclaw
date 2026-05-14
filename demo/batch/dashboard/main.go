package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFS embed.FS

type AgentState struct {
	Name       string  `json:"name"`
	Label      string  `json:"label"`
	DocumentID string  `json:"documentId"`
	Status     string  `json:"status"` // pending, deploying, ready, working, completed, failed
	TaskID     string  `json:"taskId"`
	Result     string  `json:"result"`
	StartTime  string  `json:"startTime"`
	EndTime    string  `json:"endTime"`
	MemoryMiB  float64 `json:"memoryMiB"`
	CPUmcpu    float64 `json:"cpuMcpu"`
	InputTok   int     `json:"inputTokens"`
	OutputTok  int     `json:"outputTokens"`
	Error      string  `json:"error,omitempty"`
}

type ScenarioState struct {
	Phase     string       `json:"phase"` // idle, deploying, running, done, failed
	StartTime string       `json:"startTime"`
	Elapsed   float64      `json:"elapsed"`
	Agents    []AgentState `json:"agents"`
}

type server struct {
	k8s        *K8sClient
	a2a        *A2AClient
	namespace  string
	inCluster  bool
	scenarios  map[string]Scenario
	mu         sync.Mutex
	states     map[string]*scenarioRuntime
}

type scenarioRuntime struct {
	phase     string
	startTime time.Time
	agents    []agentRuntime
}

type agentRuntime struct {
	assign    AgentAssignment
	status    string
	taskID    string
	result    string
	startTime time.Time
	endTime   time.Time
	memMiB    float64
	cpuMcpu   float64
	inputTok  int
	outputTok int
	errMsg    string
}

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	namespace := flag.String("namespace", "", "Kubernetes namespace")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (for local dev)")
	flag.Parse()

	k8s, err := NewK8sClient(*namespace, *kubeconfig)
	if err != nil {
		slog.Error("failed to create K8s client", "error", err)
		os.Exit(1)
	}

	srv := &server{
		k8s:       k8s,
		a2a:       NewA2AClient(),
		namespace: k8s.namespace,
		inCluster: *kubeconfig == "",
		scenarios: AllScenarios(k8s.namespace),
		states:    make(map[string]*scenarioRuntime),
	}

	mux := http.NewServeMux()

	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		slog.Error("failed to create static sub-filesystem", "error", err)
		os.Exit(1)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := staticFS.ReadFile("static/index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	mux.HandleFunc("GET /scenarios", func(w http.ResponseWriter, r *http.Request) {
		var list []map[string]any
		for _, s := range srv.scenarios {
			list = append(list, map[string]any{
				"name":   s.Name,
				"title":  s.Title,
				"agents": len(s.Agents),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, list)
	})

	mux.HandleFunc("POST /api/run/{scenario}", srv.handleRun)
	mux.HandleFunc("GET /api/status/{scenario}", srv.handleStatus)
	mux.HandleFunc("POST /api/cleanup/{scenario}", srv.handleCleanup)
	mux.HandleFunc("GET /api/document/{id}", srv.handleDocument)

	slog.Info("dashboard ready", "addr", *addr, "namespace", srv.namespace)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("scenario")
	scenario, ok := s.scenarios[name]
	if !ok {
		http.Error(w, "unknown scenario", http.StatusNotFound)
		return
	}
	if len(scenario.Agents) == 0 {
		http.Error(w, "scenario not configured", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if existing, ok := s.states[name]; ok && existing.phase != "idle" && existing.phase != "done" && existing.phase != "failed" {
		s.mu.Unlock()
		http.Error(w, "scenario already running", http.StatusConflict)
		return
	}

	rt := &scenarioRuntime{
		phase:     "deploying",
		startTime: time.Now(),
		agents:    make([]agentRuntime, len(scenario.Agents)),
	}
	for i, a := range scenario.Agents {
		rt.agents[i] = agentRuntime{assign: a, status: "pending"}
	}
	s.states[name] = rt
	s.mu.Unlock()

	go s.runScenario(name, scenario, rt)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "started"})
}

func (s *server) runScenario(name string, scenario Scenario, rt *scenarioRuntime) {
	slog.Info("starting scenario", "name", name, "agents", len(scenario.Agents))

	// Phase 1: Deploy all agents.
	for i, a := range scenario.Agents {
		s.mu.Lock()
		rt.agents[i].status = "deploying"
		s.mu.Unlock()

		err := s.k8s.CreateAgentDeployment(a.Name, scenario.ConfigMap, scenario.DocService, scenario.LLMTimeout)
		if err != nil {
			slog.Error("deploy failed", "agent", a.Name, "error", err)
			s.mu.Lock()
			rt.agents[i].status = "failed"
			rt.agents[i].errMsg = err.Error()
			s.mu.Unlock()
			continue
		}
		if !s.inCluster {
			if err := s.k8s.CreateRoute(a.Name); err != nil {
				slog.Warn("route creation failed", "agent", a.Name, "error", err)
			}
		}
		slog.Info("agent deployed", "name", a.Name)
	}

	// Phase 2: Wait for all agents to be ready (in parallel).
	s.mu.Lock()
	rt.phase = "waiting"
	s.mu.Unlock()

	var readyWg sync.WaitGroup
	for i, a := range scenario.Agents {
		s.mu.Lock()
		status := rt.agents[i].status
		s.mu.Unlock()
		if status == "failed" {
			continue
		}

		readyWg.Add(1)
		go func(idx int, name string) {
			defer readyWg.Done()
			if err := s.waitForAgent(name, 90*time.Second); err != nil {
				slog.Error("agent not ready", "name", name, "error", err)
				s.mu.Lock()
				rt.agents[idx].status = "failed"
				rt.agents[idx].errMsg = "pod not ready: " + err.Error()
				s.mu.Unlock()
				return
			}
			s.mu.Lock()
			rt.agents[idx].status = "ready"
			s.mu.Unlock()
		}(i, a.Name)
	}
	readyWg.Wait()

	// Phase 3: Send tasks to all agents in parallel.
	s.mu.Lock()
	rt.phase = "running"
	s.mu.Unlock()

	var wg sync.WaitGroup
	for i := range scenario.Agents {
		s.mu.Lock()
		status := rt.agents[i].status
		s.mu.Unlock()
		if status != "ready" {
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.runAgent(name, scenario, rt, idx)
		}(i)
	}
	wg.Wait()

	phase := "done"
	for _, a := range rt.agents {
		if a.status == "failed" {
			phase = "failed"
			break
		}
	}

	s.mu.Lock()
	rt.phase = phase
	s.mu.Unlock()

	slog.Info("scenario complete", "name", name,
		"elapsed", time.Since(rt.startTime).Round(time.Second))
}

func (s *server) runAgent(_ string, scenario Scenario, rt *scenarioRuntime, idx int) {
	a := scenario.Agents[idx]

	var agentURL string
	if s.inCluster {
		agentURL = fmt.Sprintf("http://%s.%s.svc:8080", a.Name, s.namespace)
	} else {
		host, err := s.k8s.GetRouteHost(a.Name)
		if err != nil {
			slog.Error("get route failed", "agent", a.Name, "error", err)
			s.mu.Lock()
			rt.agents[idx].status = "failed"
			rt.agents[idx].errMsg = "no route: " + err.Error()
			s.mu.Unlock()
			return
		}
		agentURL = "https://" + host
	}

	s.mu.Lock()
	rt.agents[idx].status = "working"
	rt.agents[idx].startTime = time.Now()
	s.mu.Unlock()

	result, err := s.a2a.SendMessage(agentURL, a.Prompt)
	if err != nil {
		slog.Error("send failed", "agent", a.Name, "error", err)
		s.mu.Lock()
		rt.agents[idx].status = "failed"
		rt.agents[idx].errMsg = err.Error()
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	rt.agents[idx].taskID = result.ID
	rt.agents[idx].status = result.State
	rt.agents[idx].result = result.Artifact
	rt.agents[idx].endTime = time.Now()
	if rt.agents[idx].status == "" {
		rt.agents[idx].status = "completed"
	}
	s.mu.Unlock()
	slog.Info("agent finished", "name", a.Name, "state", result.State)
}

func (s *server) waitForAgent(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	selector := "app=" + name

	for time.Now().Before(deadline) {
		pods, err := s.k8s.ListPods(selector)
		if err == nil {
			for _, p := range pods {
				if p.Ready {
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("scenario")

	s.mu.Lock()
	rt, ok := s.states[name]
	if !ok {
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, ScenarioState{Phase: "idle"})
		return
	}

	state := ScenarioState{
		Phase:     rt.phase,
		StartTime: rt.startTime.Format(time.RFC3339),
		Elapsed:   time.Since(rt.startTime).Seconds(),
		Agents:    make([]AgentState, len(rt.agents)),
	}

	for i, a := range rt.agents {
		state.Agents[i] = AgentState{
			Name:       a.assign.Name,
			Label:      a.assign.Label,
			DocumentID: a.assign.DocumentID,
			Status:     a.status,
			TaskID:     a.taskID,
			Result:     a.result,
			MemoryMiB:  a.memMiB,
			CPUmcpu:    a.cpuMcpu,
			InputTok:   a.inputTok,
			OutputTok:  a.outputTok,
			Error:      a.errMsg,
		}
		if !a.startTime.IsZero() {
			state.Agents[i].StartTime = a.startTime.Format(time.RFC3339)
		}
		if !a.endTime.IsZero() {
			state.Agents[i].EndTime = a.endTime.Format(time.RFC3339)
		}
	}
	s.mu.Unlock()

	// Fetch live metrics and token counts (best-effort, don't block on errors).
	s.enrichWithMetrics(&state)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, state)
}

func (s *server) enrichWithMetrics(state *ScenarioState) {
	metrics, err := s.k8s.GetPodMetrics("managed-by=dashboard")
	if err != nil {
		return
	}

	metricsByApp := make(map[string]PodMetrics)
	for _, m := range metrics {
		for i := range state.Agents {
			if strings.HasPrefix(m.Name, state.Agents[i].Name) {
				metricsByApp[state.Agents[i].Name] = m
			}
		}
	}

	for i := range state.Agents {
		if m, ok := metricsByApp[state.Agents[i].Name]; ok {
			state.Agents[i].MemoryMiB = m.MemoryMiB
			state.Agents[i].CPUmcpu = m.CPUmcpu
		}

		// Parse tokens from logs.
		pods, err := s.k8s.ListPods("app=" + state.Agents[i].Name)
		if err != nil || len(pods) == 0 {
			continue
		}
		logs, err := s.k8s.GetPodLogs(pods[0].Name, 600)
		if err != nil {
			continue
		}
		tokens := ParseTokensFromLogs(logs)
		state.Agents[i].InputTok = tokens.InputTokens
		state.Agents[i].OutputTok = tokens.OutputTokens
	}
}

func (s *server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("scenario")
	scenario, ok := s.scenarios[name]
	if !ok {
		http.Error(w, "unknown scenario", http.StatusNotFound)
		return
	}

	for _, a := range scenario.Agents {
		if err := s.k8s.DeleteAgent(a.Name); err != nil {
			slog.Error("cleanup failed", "agent", a.Name, "error", err)
		} else {
			slog.Info("agent deleted", "name", a.Name)
		}
	}

	s.mu.Lock()
	delete(s.states, name)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "cleaned"})
}

func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

var validDocID = regexp.MustCompile(`^[A-Za-z0-9][-A-Za-z0-9]*$`)

var docHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (s *server) handleDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("id")
	if !validDocID.MatchString(docID) {
		http.Error(w, "invalid document ID", http.StatusBadRequest)
		return
	}

	var docURL string
	if s.inCluster {
		docURL = fmt.Sprintf("http://document-service.%s.svc:8080/documents/%s", s.namespace, docID)
	} else {
		host, err := s.k8s.GetRouteHost("document-service")
		if err != nil {
			http.Error(w, "document-service route not found", http.StatusBadGateway)
			return
		}
		docURL = fmt.Sprintf("https://%s/documents/%s", host, docID)
	}

	resp, err := docHTTPClient.Get(docURL)
	if err != nil {
		http.Error(w, "failed to reach document-service", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
