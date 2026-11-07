package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

// e2eTokenMarker is embedded in every minted token's jti claim so a leak can
// be detected even if the token is decoded somewhere along the way.
const e2eTokenMarker = "e2e-token-marker-7f3a"

// e2eProvider scripts agent A: it asks for one a2a_call and then echoes the
// tool result as its final answer. Every input is recorded so the test can
// prove the raw token never reaches the LLM.
type e2eProvider struct {
	mu        sync.Mutex
	inputs    [][]llm.Message
	responses []*llm.Response
}

func (p *e2eProvider) Complete(_ context.Context, systemPrompt, userPrompt string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inputs = append(p.inputs, []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	return "answer", nil
}

func (p *e2eProvider) CompleteWithTools(_ context.Context,
	messages []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {

	p.mu.Lock()
	defer p.mu.Unlock()
	p.inputs = append(p.inputs, copyMessages(messages))
	if len(p.responses) > 0 {
		resp := p.responses[0]
		p.responses = p.responses[1:]
		return resp, nil
	}
	// Echo the most recent tool result so the final text is the call outcome.
	for i := len(messages) - 1; i >= 0; i-- {
		if len(messages[i].ToolResults) > 0 {
			return &llm.Response{
				StopReason: llm.StopReasonEndTurn,
				Content:    messages[i].ToolResults[0].Output,
			}, nil
		}
	}
	return &llm.Response{StopReason: llm.StopReasonEndTurn, Content: "answer"}, nil
}

func (p *e2eProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	_ func(llm.StreamEvent)) (*llm.Response, error) {

	return p.CompleteWithTools(ctx, messages, tools)
}

func (p *e2eProvider) Model() string        { return "fake-model" }
func (p *e2eProvider) ProviderName() string { return "fake" }

func copyMessages(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		out[i].ToolCalls = append([]llm.ToolCall(nil), m.ToolCalls...)
		out[i].ToolResults = append([]llm.ToolResultContent(nil), m.ToolResults...)
	}
	return out
}

// newE2EBRuntime builds agent B: tool mode with no tools, inbound auth on,
// and a SQLite session store so the test can inspect what was persisted.
func newE2EBRuntime(t *testing.T, jwksURL, sessionDB string,
	provider llm.Provider, logWriter io.Writer) *agentRuntime {

	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "system-prompt.txt"),
		[]byte("You are agent B. Answer briefly."), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`tools:
  allowed: []
  workspace: %q
inbound_auth:
  enabled: true
  issuer: %q
  jwks_url: %q
  audience: %q
  required_scopes: [engineering]
  allowed_callers: [%q]
`,
		filepath.Join(dir, "workspace"),
		authTestIssuerURL, jwksURL, authTestAudience, authTestAZPPrefix,
	)
	if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"),
		[]byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := buildRuntime(context.Background(), Config{
		ConfigDir: dir,
		Workspace: t.TempDir(),
		SessionDB: sessionDB,
	}, runtimeOptions{Provider: provider, LogWriter: logWriter})
	if err != nil {
		t.Fatalf("buildRuntime(B): %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

// newE2EARuntime builds agent A with one a2a_call peer.
func newE2EARuntime(t *testing.T, peerURL string, provider llm.Provider) *agentRuntime {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "system-prompt.txt"),
		[]byte("You are agent A. Use a2a_call when asked."), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`tools:
  allowed: [a2a_call]
  workspace: %q
a2a_peers:
  - name: engineering-analyst
    url: %q
    description: Analyzes engineering documents
    timeout_seconds: 10
`, filepath.Join(dir, "workspace"), peerURL)
	if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"),
		[]byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := buildRuntime(context.Background(), Config{
		ConfigDir: dir,
		Workspace: t.TempDir(),
	}, runtimeOptions{Provider: provider, LogWriter: io.Discard})
	if err != nil {
		t.Fatalf("buildRuntime(A): %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

// newSupervisor stands in for the OpenShell supervisor: it forwards to B and
// injects (or withholds) the bearer token, plus a session ID.
func newSupervisor(t *testing.T, targetURL string, token func() string) *httptest.Server {
	t.Helper()
	target, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			if tok := token(); tok != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+tok)
			} else {
				pr.Out.Header.Del("Authorization")
			}
			pr.Out.Header.Set("x-session-id", "s1")
		},
	}
	srv := httptest.NewServer(proxy)
	t.Cleanup(srv.Close)
	return srv
}

func a2aCallResponse() *llm.Response {
	return &llm.Response{
		StopReason: llm.StopReasonToolUse,
		ToolCalls: []llm.ToolCall{{
			ID:   "call-1",
			Name: "a2a_call",
			Args: map[string]any{
				"peer":    "engineering-analyst",
				"message": "hello",
			},
		}},
	}
}

func runAgentA(t *testing.T, rt *agentRuntime) string {
	t.Helper()
	result, err := rt.Run(context.Background(), []llm.Message{
		{Role: "system", Content: rt.SystemPrompt},
		{Role: "user", Content: "Ask the engineering analyst for a status update."},
	}, nil)
	if err != nil {
		t.Fatalf("A.Run: %v", err)
	}
	return result
}

func TestE2E_AgentACallsAgentB(t *testing.T) {
	issuer := newAuthTestIssuer(t)
	token := issuer.mint(t, func(claims map[string]any) {
		claims["jti"] = e2eTokenMarker
	})

	bLog := &syncBuffer{}
	bProvider := &fakeProvider{responses: []*llm.Response{{
		StopReason: llm.StopReasonEndTurn,
		Content:    "B says hi",
	}}}
	sessionDB := filepath.Join(t.TempDir(), "sessions.db")
	rtB := newE2EBRuntime(t, issuer.jwksURL(), sessionDB, bProvider, bLog)

	handlerB, closeB, err := newServeHandler(rtB)
	if err != nil {
		t.Fatalf("newServeHandler(B): %v", err)
	}
	serverB := httptest.NewServer(handlerB)
	t.Cleanup(serverB.Close)

	supervisor := newSupervisor(t, serverB.URL, func() string { return token })

	aProvider := &e2eProvider{responses: []*llm.Response{a2aCallResponse()}}
	rtA := newE2EARuntime(t, supervisor.URL, aProvider)

	result := runAgentA(t, rtA)
	if !strings.Contains(result, "[completed] B says hi") {
		t.Fatalf("final text = %q, want it to contain %q", result, "[completed] B says hi")
	}

	// B authenticated the call and logged the verified identity.
	logOut := bLog.String()
	for _, want := range []string{
		"sub=alice",
		"azp=" + authTestAZPPrefix + "alice-assistant",
		"scope=engineering read",
		"aud=" + authTestAudience,
	} {
		if !strings.Contains(logOut, want) {
			t.Fatalf("B log missing %q:\n%s", want, logOut)
		}
	}

	// The call really reached B's LLM.
	if len(bProvider.toolMessages) == 0 {
		t.Fatal("B provider was never called")
	}
	bInput := flattenProviderInputs(bProvider.toolMessages)
	if !strings.Contains(bInput, "hello") {
		t.Fatalf("B provider input does not contain the user message:\n%s", bInput)
	}

	// Exhaustive token isolation.
	assertNoToken(t, "B provider input", token, bInput)
	assertNoToken(t, "B log output", token, logOut)
	assertNoToken(t, "A provider input", token, flattenProviderInputs(aProvider.inputs))

	// Session store contents, after closing the store so WAL is flushed.
	if err := closeB(); err != nil {
		t.Fatalf("close B: %v", err)
	}
	sessions := readSessionFiles(t, sessionDB)
	if !strings.Contains(sessions, "hello") {
		t.Fatalf("session store does not contain the user message:\n%s", sessions)
	}
	assertNoToken(t, "B session store", token, sessions)
}

func TestE2E_DenyCodesReachCaller(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		inject     bool
		killIssuer bool
		want       string
	}{
		{
			name:   "expired",
			mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour) },
			inject: true,
			want:   "401 token_expired",
		},
		{
			name:   "wrong audience",
			mutate: func(c map[string]any) { c["aud"] = []string{"other-audience"} },
			inject: true,
			want:   "403 invalid_audience",
		},
		{
			name:   "missing scope",
			mutate: func(c map[string]any) { c["scope"] = "read" },
			inject: true,
			want:   "403 insufficient_scope",
		},
		{
			name:   "disallowed caller",
			mutate: func(c map[string]any) { c["azp"] = "spiffe://evil.example/sandbox/x" },
			inject: true,
			want:   "403 caller_not_allowed",
		},
		{
			name:   "supervisor injects nothing",
			inject: false,
			want:   "401 missing_token",
		},
		{
			name:       "jwks unavailable",
			inject:     true,
			killIssuer: true,
			want:       "503 jwks_unavailable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newAuthTestIssuer(t)
			token := issuer.mint(t, func(claims map[string]any) {
				claims["jti"] = e2eTokenMarker
				if tc.mutate != nil {
					tc.mutate(claims)
				}
			})
			if tc.killIssuer {
				issuer.server.Close()
			}

			bProvider := &fakeProvider{responses: []*llm.Response{{
				StopReason: llm.StopReasonEndTurn,
				Content:    "B says hi",
			}}}
			rtB := newE2EBRuntime(t, issuer.jwksURL(), "", bProvider, io.Discard)
			handlerB, closeB, err := newServeHandler(rtB)
			if err != nil {
				t.Fatalf("newServeHandler(B): %v", err)
			}
			t.Cleanup(func() { _ = closeB() })
			serverB := httptest.NewServer(handlerB)
			t.Cleanup(serverB.Close)

			supervisor := newSupervisor(t, serverB.URL, func() string {
				if tc.inject {
					return token
				}
				return ""
			})

			aProvider := &e2eProvider{responses: []*llm.Response{a2aCallResponse()}}
			rtA := newE2EARuntime(t, supervisor.URL, aProvider)

			result := runAgentA(t, rtA)
			want := "peer engineering-analyst: " + tc.want
			if !strings.Contains(result, want) {
				t.Fatalf("tool result = %q, want it to contain %q", result, want)
			}
			assertNoToken(t, "A provider input", token, flattenProviderInputs(aProvider.inputs))
		})
	}
}

// flattenProviderInputs renders every recorded message, tool call, and tool
// result into one searchable string.
func flattenProviderInputs(inputs [][]llm.Message) string {
	var b strings.Builder
	for _, msgs := range inputs {
		for _, m := range msgs {
			b.WriteString(m.Content)
			b.WriteByte('\n')
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "%s %v\n", tc.Name, tc.Args)
			}
			for _, tr := range m.ToolResults {
				b.WriteString(tr.Output)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func assertNoToken(t *testing.T, where, token, haystack string) {
	t.Helper()
	if strings.Contains(haystack, token) {
		t.Fatalf("%s contains the raw token", where)
	}
	if strings.Contains(haystack, e2eTokenMarker) {
		t.Fatalf("%s contains the token marker", where)
	}
}

// readSessionFiles reads the SQLite database and any WAL/SHM sidecars so the
// assertion covers data that has not been checkpointed yet.
func readSessionFiles(t *testing.T, dbPath string) string {
	t.Helper()
	var b strings.Builder
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		b.Write(data)
	}
	return b.String()
}
