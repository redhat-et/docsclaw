package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func newTestProvider(serverURL string, client *http.Client) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		baseURL:      serverURL,
		apiKey:       "test-key",
		model:        "test-model",
		maxTokens:    100,
		timeout:      30 * time.Second,
		client:       client,
		providerName: "test",
	}
}

func TestStreamWithTools(t *testing.T) {
	sseBody := `data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseBody)
	}))
	defer server.Close()

	provider := newTestProvider(server.URL, server.Client())

	var events []llm.StreamEvent
	onEvent := func(e llm.StreamEvent) {
		events = append(events, e)
	}

	messages := []llm.Message{
		{Role: "user", Content: "Say hello"},
	}

	resp, err := provider.StreamWithTools(context.Background(), messages, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify text delta events
	var textDeltas []llm.StreamEvent
	for _, e := range events {
		if e.Type == llm.StreamEventTextDelta {
			textDeltas = append(textDeltas, e)
		}
	}
	if len(textDeltas) != 2 {
		t.Fatalf("expected 2 text delta events, got %d", len(textDeltas))
	}
	if textDeltas[0].Content != "Hello" {
		t.Errorf("first delta = %q, want %q", textDeltas[0].Content, "Hello")
	}
	if textDeltas[1].Content != " world" {
		t.Errorf("second delta = %q, want %q", textDeltas[1].Content, " world")
	}

	// Verify accumulated content
	if resp.Content != "Hello world" {
		t.Errorf("content = %q, want %q", resp.Content, "Hello world")
	}

	// Verify stop reason
	if resp.StopReason != llm.StopReasonEndTurn {
		t.Errorf("stop reason = %q, want %q", resp.StopReason, llm.StopReasonEndTurn)
	}

	// Verify done event with usage
	var doneEvents []llm.StreamEvent
	for _, e := range events {
		if e.Type == llm.StreamEventDone {
			doneEvents = append(doneEvents, e)
		}
	}
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 done event, got %d", len(doneEvents))
	}
	if doneEvents[0].Usage.InputTokens != 10 {
		t.Errorf("usage input tokens = %d, want 10", doneEvents[0].Usage.InputTokens)
	}
	if doneEvents[0].Usage.OutputTokens != 5 {
		t.Errorf("usage output tokens = %d, want 5", doneEvents[0].Usage.OutputTokens)
	}
	if doneEvents[0].Usage.TotalTokens != 15 {
		t.Errorf("usage total tokens = %d, want 15", doneEvents[0].Usage.TotalTokens)
	}
}

func TestStreamWithToolsNilCallback(t *testing.T) {
	sseBody := `data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseBody)
	}))
	defer server.Close()

	provider := newTestProvider(server.URL, server.Client())

	messages := []llm.Message{
		{Role: "user", Content: "Say hello"},
	}

	resp, err := provider.StreamWithTools(context.Background(), messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Hello world" {
		t.Errorf("content = %q, want %q", resp.Content, "Hello world")
	}

	if resp.StopReason != llm.StopReasonEndTurn {
		t.Errorf("stop reason = %q, want %q", resp.StopReason, llm.StopReasonEndTurn)
	}

	if resp.Usage.InputTokens != 10 {
		t.Errorf("usage input tokens = %d, want 10", resp.Usage.InputTokens)
	}
}

func TestStreamWithToolsToolCalls(t *testing.T) {
	sseBody := `data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Let me check."},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc123","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}

data: [DONE]
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseBody)
	}))
	defer server.Close()

	provider := newTestProvider(server.URL, server.Client())

	var events []llm.StreamEvent
	onEvent := func(e llm.StreamEvent) {
		events = append(events, e)
	}

	messages := []llm.Message{
		{Role: "user", Content: "What is the weather?"},
	}
	tools := []llm.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for a location",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "City name",
					},
				},
				"required": []any{"location"},
			},
		},
	}

	resp, err := provider.StreamWithTools(context.Background(), messages, tools, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify text delta was emitted
	var textDeltas []llm.StreamEvent
	for _, e := range events {
		if e.Type == llm.StreamEventTextDelta {
			textDeltas = append(textDeltas, e)
		}
	}
	if len(textDeltas) != 1 {
		t.Fatalf("expected 1 text delta event, got %d", len(textDeltas))
	}
	if textDeltas[0].Content != "Let me check." {
		t.Errorf("text delta = %q, want %q", textDeltas[0].Content, "Let me check.")
	}

	// Verify stop reason
	if resp.StopReason != llm.StopReasonToolUse {
		t.Errorf("stop reason = %q, want %q", resp.StopReason, llm.StopReasonToolUse)
	}

	// Verify tool call
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "call_abc123")
	}
	if tc.Name != "get_weather" {
		t.Errorf("tool call name = %q, want %q", tc.Name, "get_weather")
	}
	loc, ok := tc.Args["location"]
	if !ok {
		t.Fatal("tool call args missing 'location'")
	}
	if loc != "NYC" {
		t.Errorf("tool call location = %q, want %q", loc, "NYC")
	}

	// Verify content was also accumulated
	if resp.Content != "Let me check." {
		t.Errorf("content = %q, want %q", resp.Content, "Let me check.")
	}
}
