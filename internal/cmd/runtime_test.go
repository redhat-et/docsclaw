package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func testConfigDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", name)
}

func TestBuildRuntime_SingleShot(t *testing.T) {
	fake := &fakeProvider{}
	cfg := Config{
		ConfigDir: testConfigDir(t, "summarizer-hr"),
		Workspace: t.TempDir(),
	}

	rt, err := buildRuntime(context.Background(), cfg, runtimeOptions{
		Provider:  fake,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	if rt.Registry != nil {
		t.Fatal("expected nil registry in single-shot mode")
	}
	if rt.Provider != fake {
		t.Fatal("expected injected provider")
	}

	if _, err := rt.Run(context.Background(),
		[]llm.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(fake.completeCalls) != 1 {
		t.Fatalf("expected 1 Complete call, got %d", len(fake.completeCalls))
	}
	if fake.completeCalls[0].UserPrompt != "hi" {
		t.Fatalf("expected user prompt %q, got %q", "hi", fake.completeCalls[0].UserPrompt)
	}
	if !strings.Contains(fake.completeCalls[0].SystemPrompt, "document summarizer") {
		t.Fatalf("expected system prompt to be forwarded, got %q",
			fake.completeCalls[0].SystemPrompt)
	}
}

func TestBuildRuntime_ToolMode(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "agent")
	if err := os.CopyFS(configDir, os.DirFS(testConfigDir(t, "research-agent"))); err != nil {
		t.Fatalf("copy testdata: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	configPath := filepath.Join(configDir, "agent-config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read agent config: %v", err)
	}
	updated := strings.ReplaceAll(string(data), "workspace: /workspace",
		"workspace: "+workspace)
	if err := os.WriteFile(configPath, []byte(updated), 0600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}

	rt, err := buildRuntime(context.Background(), Config{
		ConfigDir: configDir,
		Workspace: workspace,
	}, runtimeOptions{Provider: &fakeProvider{}, LogWriter: io.Discard})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	if rt.Registry == nil {
		t.Fatal("expected registry in tool mode")
	}
	var found bool
	for _, def := range rt.Registry.Definitions() {
		if def.Name == "fetch_document" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fetch_document not registered; definitions: %+v", rt.Registry.Definitions())
	}
}

func TestBuildRuntime_A2ACallRegistration(t *testing.T) {
	tests := []struct {
		name     string
		allowed  string
		peers    string
		wantTool bool
		wantLog  string
	}{
		{
			name:     "peers and allowed",
			allowed:  "    - a2a_call\n",
			peers:    "a2a_peers:\n  - name: engineering-analyst\n    url: http://127.0.0.1:1234\n    description: test peer\n",
			wantTool: true,
			wantLog:  "a2a_call enabled",
		},
		{
			name:    "peers but not allowed",
			allowed: "",
			peers:   "a2a_peers:\n  - name: engineering-analyst\n    url: http://127.0.0.1:1234\n",
		},
		{
			name:    "allowed but no peers",
			allowed: "    - a2a_call\n",
			peers:   "",
			wantLog: "a2a_call allowed but no a2a_peers configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configDir := filepath.Join(dir, "agent")
			if err := os.CopyFS(configDir, os.DirFS(testConfigDir(t, "summarizer-hr"))); err != nil {
				t.Fatalf("copy testdata: %v", err)
			}
			workspace := filepath.Join(dir, "workspace")
			agentCfg := "tools:\n  allowed:\n" + tt.allowed +
				"  workspace: " + workspace + "\n" + tt.peers
			if err := os.WriteFile(filepath.Join(configDir, "agent-config.yaml"), []byte(agentCfg), 0600); err != nil {
				t.Fatalf("write agent config: %v", err)
			}

			var logBuf bytes.Buffer
			rt, err := buildRuntime(context.Background(), Config{
				ConfigDir: configDir,
				Workspace: workspace,
			}, runtimeOptions{Provider: &fakeProvider{}, LogWriter: &logBuf})
			if err != nil {
				t.Fatalf("buildRuntime: %v", err)
			}
			t.Cleanup(func() { _ = rt.Close() })

			var found bool
			for _, def := range rt.Registry.Definitions() {
				if def.Name == "a2a_call" {
					found = true
				}
			}
			if found != tt.wantTool {
				t.Fatalf("a2a_call registered = %v, want %v; defs: %+v",
					found, tt.wantTool, rt.Registry.Definitions())
			}
			if tt.wantLog != "" && !strings.Contains(logBuf.String(), tt.wantLog) {
				t.Fatalf("log does not contain %q:\n%s", tt.wantLog, logBuf.String())
			}
		})
	}
}

func TestNewRuntimeLogger_NilUsesFormatDetection(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	log := newRuntimeLogger(nil)
	log.Info("hello", "key", "value")

	_ = w.Close()
	os.Stdout = oldStdout
	t.Cleanup(func() { os.Stdout = oldStdout })

	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &parsed); err != nil {
		t.Fatalf("serve logger did not emit JSON with LOG_FORMAT=json: %q", raw)
	}
	if parsed["msg"] != "hello" {
		t.Fatalf("unexpected log message: %v", parsed["msg"])
	}
}

type blockingProvider struct {
	started chan struct{}
}

func (p *blockingProvider) Complete(ctx context.Context, _, _ string) (string, error) {
	close(p.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func (p *blockingProvider) CompleteWithTools(ctx context.Context,
	_ []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {

	close(p.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (p *blockingProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	_ func(llm.StreamEvent)) (*llm.Response, error) {

	return p.CompleteWithTools(ctx, messages, tools)
}

func (p *blockingProvider) Model() string        { return "blocking" }
func (p *blockingProvider) ProviderName() string { return "blocking" }

func TestRuntime_RunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &blockingProvider{started: make(chan struct{})}

	rt, err := buildRuntime(ctx, Config{
		ConfigDir: testConfigDir(t, "summarizer-hr"),
		Workspace: t.TempDir(),
	}, runtimeOptions{Provider: provider, LogWriter: io.Discard})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	go func() {
		<-provider.started
		cancel()
	}()

	_, err = rt.Run(ctx, []llm.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected an error after the context was canceled")
	}
}

func TestNewServeHandler_Routes(t *testing.T) {
	rt, err := buildRuntime(context.Background(), Config{
		ConfigDir: testConfigDir(t, "summarizer-hr"),
		Workspace: t.TempDir(),
	}, runtimeOptions{Provider: &fakeProvider{}, LogWriter: io.Discard})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	handler, closeHandler, err := newServeHandler(rt)
	if err != nil {
		t.Fatalf("newServeHandler: %v", err)
	}
	t.Cleanup(func() { _ = closeHandler() })

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("GET agent card: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("agent card status = %d, want 200", resp.StatusCode)
	}

	resp, err = srv.Client().Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}

	client, err := a2aclient.NewFromEndpoints(context.Background(),
		[]*a2a.AgentInterface{a2a.NewAgentInterface(srv.URL, a2a.TransportProtocolJSONRPC)},
		a2aclient.WithDefaultsDisabled(),
		a2aclient.WithJSONRPCTransport(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new A2A client: %v", err)
	}
	result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hi")),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !strings.Contains(string(raw), "answer") {
		t.Fatalf("expected answer in A2A result, got %s", raw)
	}
}
