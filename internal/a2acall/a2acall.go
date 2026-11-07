package a2acall

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redhat-et/docsclaw/internal/bridge"
	"github.com/redhat-et/docsclaw/internal/egress"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// Peer is one configured A2A peer the tool may call.
type Peer struct {
	Name        string
	URL         string
	Description string
	Timeout     time.Duration
}

// New creates the a2a_call tool for a set of configured peers. The client
// carries the agent's egress-configured HTTP client, so every call keeps the
// agent's proxy, TLS, and timeout settings. Peer names must be unique; a later
// entry with the same name replaces an earlier one. A nil client falls back to
// an egress-configured default.
func New(peers []Peer, client *bridge.A2AClient, log *slog.Logger) tools.Tool {
	if log == nil {
		log = slog.Default()
	}
	if client == nil {
		client = bridge.NewA2AClient(egress.NewClient(0), log)
	}
	byName := make(map[string]Peer, len(peers))
	for _, p := range peers {
		byName[p.Name] = p
	}
	return &a2aCallTool{peers: peers, byName: byName, client: client, log: log}
}

type a2aCallTool struct {
	peers  []Peer
	byName map[string]Peer
	client *bridge.A2AClient
	log    *slog.Logger
}

func (t *a2aCallTool) Name() string { return "a2a_call" }

func (t *a2aCallTool) Description() string {
	var b strings.Builder
	b.WriteString("Call a configured peer A2A agent by name. Peers: ")
	for i, p := range t.peers {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(p.Name)
		b.WriteString(": ")
		b.WriteString(p.Description)
	}
	b.WriteString(".")
	return b.String()
}

func (t *a2aCallTool) Parameters() map[string]any {
	names := make([]string, 0, len(t.peers))
	for _, p := range t.peers {
		names = append(names, p.Name)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"peer": map[string]any{
				"type":        "string",
				"description": "Peer agent name",
				"enum":        names,
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message to send to the peer",
			},
		},
		"required": []string{"peer", "message"},
	}
}

func (t *a2aCallTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	peerName, ok := args["peer"].(string)
	if !ok || peerName == "" {
		return tools.Errorf("peer is required")
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return tools.Errorf("message is required")
	}

	peer, ok := t.byName[peerName]
	if !ok {
		return tools.Errorf("unknown peer %q; available: %s",
			peerName, strings.Join(t.peerNames(), ", "))
	}

	host := ""
	if u, err := url.Parse(peer.URL); err == nil {
		host = u.Host
	}

	callCtx, cancel := context.WithTimeout(ctx, peer.timeout())
	defer cancel()

	// Copy the injected client so the per-call capture wraps its transport
	// instead of replacing it: the agent's egress settings (proxy, TLS trust,
	// connection pool) must still apply to peer calls.
	base := *t.client.HTTPClient()
	capture := &captureTransport{base: base.Transport}
	if capture.base == nil {
		capture.base = http.DefaultTransport
	}
	base.Transport = capture
	callClient := bridge.NewA2AClient(&base, t.log)
	result, err := callClient.Invoke(callCtx, &bridge.InvokeRequest{
		AgentURL:    peer.URL,
		MessageText: message,
	})
	if err != nil {
		if status, body, ok := capture.denial(); ok {
			reason := egress.DescribeResponse(&http.Response{
				StatusCode: status,
				Body:       io.NopCloser(bytes.NewReader(body)),
			})
			egress.LogCall(t.log, "a2a_call", host, status, reason)
			return tools.Errorf("peer %s: %s", peerName, reason)
		}
		reason := egress.DescribeError(err)
		egress.LogCall(t.log, "a2a_call", host, 0, reason)
		return tools.Errorf("peer %s: %s", peerName, reason)
	}

	egress.LogCall(t.log, "a2a_call", host, http.StatusOK, result.State)
	return formatResult(result.State, result.Text)
}

func (t *a2aCallTool) peerNames() []string {
	names := make([]string, 0, len(t.peers))
	for _, p := range t.peers {
		names = append(names, p.Name)
	}
	return names
}

func (p Peer) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return 120 * time.Second
}

// stateName normalizes both the A2A task-state constants and the shorter
// "completed" used for direct message replies.
func stateName(raw string) string {
	s := strings.TrimPrefix(strings.ToLower(raw), "task_state_")
	return strings.ReplaceAll(s, "_", "-")
}

func formatResult(rawState, text string) *tools.ToolResult {
	state := stateName(rawState)
	switch state {
	case "failed", "rejected", "canceled", "cancelled":
		return tools.Errorf("[%s] %s", state, text)
	case "input-required", "auth-required":
		return tools.OK(fmt.Sprintf("[%s] peer needs more input: %s", state, text))
	default:
		return tools.OK(fmt.Sprintf("[%s] %s", state, text))
	}
}

// captureTransport records the status and body of a non-2xx response while
// still handing the body to the A2A client. a2a-go surfaces non-2xx responses
// as parse errors, so this is the only way to report the peer's JSON error code.
type captureTransport struct {
	base http.RoundTripper

	mu     sync.Mutex
	status int
	body   []byte
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if resp.StatusCode >= 400 {
		// Error bodies are small; bound the read so a hostile peer cannot
		// make the agent buffer an unbounded response.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))

		t.mu.Lock()
		t.status = resp.StatusCode
		t.body = body
		t.mu.Unlock()
	}
	return resp, nil
}

func (t *captureTransport) denial() (int, []byte, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status, t.body, t.status >= 400
}
