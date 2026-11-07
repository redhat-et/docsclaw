package cmd

import (
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
