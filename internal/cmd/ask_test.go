package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"LLM_PROVIDER", "LLM_API_KEY", "ANTHROPIC_API_KEY",
		"LLM_BASE_URL", "LLM_MODEL",
	} {
		t.Setenv(key, "")
	}
}

func setAskProvider(t *testing.T, provider llm.Provider) {
	t.Helper()
	previous := askRuntimeOptions
	askRuntimeOptions = runtimeOptions{Provider: provider}
	t.Cleanup(func() { askRuntimeOptions = previous })
}

func executeAsk(t *testing.T, args []string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newAskCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestAsk_PrintsAnswerOnly(t *testing.T) {
	clearLLMEnv(t)
	setAskProvider(t, &fakeProvider{})

	stdout, stderr, err := executeAsk(t, []string{
		"--config-dir", testConfigDir(t, "summarizer-hr"), "hi",
	})
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}
	if stdout != "answer\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "answer\n")
	}
	if strings.Contains(stderr, "answer") {
		t.Fatalf("stderr leaked the answer: %q", stderr)
	}
}

func TestAsk_NoProviderFails(t *testing.T) {
	clearLLMEnv(t)
	setAskProvider(t, nil)

	stdout, _, err := executeAsk(t, []string{
		"--config-dir", testConfigDir(t, "summarizer-hr"), "hi",
	})
	if err == nil {
		t.Fatal("expected an error when no provider is configured")
	}
	if !strings.Contains(err.Error(), "LLM API key not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestAsk_TraceWritesToolCalls(t *testing.T) {
	clearLLMEnv(t)

	docServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents/DOC-1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"document":{"title":"Doc","content":"Body"}}`)
	}))
	t.Cleanup(docServer.Close)

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
	updated := strings.ReplaceAll(string(data), "http://localhost:8080", docServer.URL)
	updated = strings.ReplaceAll(updated, "workspace: /workspace", "workspace: "+workspace)
	if err := os.WriteFile(configPath, []byte(updated), 0600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}

	fake := &fakeProvider{responses: []*llm.Response{
		{
			StopReason: llm.StopReasonToolUse,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Name: "fetch_document",
				Args: map[string]any{
					"service":     "research",
					"document_id": "DOC-1",
				},
			}},
		},
		{StopReason: llm.StopReasonEndTurn, Content: "done"},
	}}
	setAskProvider(t, fake)

	stdout, stderr, err := executeAsk(t, []string{
		"--config-dir", configDir, "--trace", "Summarize DOC-1",
	})
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}
	if stdout != "done\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "done\n")
	}
	if !strings.Contains(stderr, "→ fetch_document") {
		t.Fatalf("stderr missing tool-call marker: %q", stderr)
	}
	if !strings.Contains(stderr, "← fetch_document ok") {
		t.Fatalf("stderr missing tool-result marker: %q", stderr)
	}
}

func TestAsk_ArgCount(t *testing.T) {
	clearLLMEnv(t)
	setAskProvider(t, &fakeProvider{})

	for _, args := range [][]string{{}, {"one", "two"}} {
		_, _, err := executeAsk(t, args)
		if err == nil {
			t.Fatalf("expected usage error for args %v", args)
		}
		if code := exitCodeFor(err); code != 2 {
			t.Fatalf("exit code for args %v = %d, want 2", args, code)
		}
	}
}

func TestAsk_LoopErrorNonZero(t *testing.T) {
	clearLLMEnv(t)
	setAskProvider(t, &fakeProvider{err: errors.New("boom")})

	stdout, _, err := executeAsk(t, []string{
		"--config-dir", testConfigDir(t, "summarizer-hr"), "hi",
	})
	if err == nil {
		t.Fatal("expected an error from the provider")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}
