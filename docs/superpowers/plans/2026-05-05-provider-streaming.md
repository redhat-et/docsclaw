# Provider Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `StreamWithTools` to the LLM provider interface and
wire real token-by-token streaming into the OpenAI-compatible API
endpoint for phase 1 mode.

**Architecture:** New `StreamWithTools` method on `Provider` with a
callback-based API. Both Anthropic and OpenAI providers implement
native streaming. The OpenAI API handler pipes provider stream
events directly to SSE chunks for phase 1; phase 2 keeps simulated
streaming.

**Tech Stack:** Anthropic SDK `Messages.NewStreaming` + `Accumulate`,
OpenAI SSE parsing with `bufio.Scanner`, existing `net/http` SSE
output.

---

## File structure

| File | Responsibility |
|------|---------------|
| Modify: `pkg/llm/types.go` | Add `StreamEvent`, `StreamEventType` |
| Modify: `pkg/llm/provider.go` | Add `StreamWithTools` to `Provider` interface |
| Modify: `internal/anthropic/anthropic.go` | Implement `StreamWithTools` using SDK streaming |
| Modify: `internal/openai/openai.go` | Implement `StreamWithTools` using SSE parsing |
| Modify: `internal/openaiapi/handler.go` | Use real streaming for phase 1 |
| Modify: `internal/openaiapi/stream.go` | Add `StreamFromProvider` function |
| Create: `internal/openai/openai_test.go` | Test SSE streaming with mock server |
| Modify: `internal/openaiapi/handler_test.go` | Test real streaming path |

---

### Task 1: Add StreamEvent types and Provider interface method

**Files:**
- Modify: `pkg/llm/types.go`
- Modify: `pkg/llm/provider.go`

- [ ] **Step 1: Add StreamEvent types to types.go**

Add at the end of `pkg/llm/types.go`:

```go
// StreamEventType identifies the kind of streaming event.
type StreamEventType string

const (
	StreamEventTextDelta StreamEventType = "text_delta"
	StreamEventDone      StreamEventType = "done"
	StreamEventError     StreamEventType = "error"
)

// StreamEvent carries a single event from a streaming LLM response.
type StreamEvent struct {
	Type    StreamEventType
	Content string // text chunk for TextDelta, error message for Error
	Usage   Usage  // populated only for Done events
}
```

- [ ] **Step 2: Add StreamWithTools to Provider interface**

In `pkg/llm/provider.go`, add the new method to the `Provider`
interface between `CompleteWithTools` and `Model`:

```go
type Provider interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)

	CompleteWithTools(ctx context.Context, messages []Message,
		tools []ToolDefinition) (*Response, error)

	// StreamWithTools is like CompleteWithTools but calls onEvent
	// with incremental text deltas as they arrive. If onEvent is
	// nil, events are discarded. Returns the accumulated Response.
	StreamWithTools(ctx context.Context, messages []Message,
		tools []ToolDefinition, onEvent func(StreamEvent)) (*Response, error)

	Model() string
	ProviderName() string
}
```

- [ ] **Step 3: Verify the project no longer compiles**

Run: `go build ./... 2>&1 | head -10`
Expected: compilation errors in `internal/anthropic/` and
`internal/openai/` because they don't implement `StreamWithTools`.
Also `pkg/tools/loop_test.go` will fail (mockProvider).

- [ ] **Step 4: Add stub StreamWithTools to mock in loop_test.go**

In `pkg/tools/loop_test.go`, add to the `mockProvider` type:

```go
func (m *mockProvider) StreamWithTools(_ context.Context,
	msgs []llm.Message, tools []llm.ToolDefinition,
	onEvent func(llm.StreamEvent)) (*llm.Response, error) {
	return m.CompleteWithTools(context.Background(), msgs, tools)
}
```

- [ ] **Step 5: Verify compilation succeeds (except providers)**

Run: `go build ./pkg/... 2>&1`
Expected: builds OK.

Run: `go build ./internal/anthropic/ ./internal/openai/ 2>&1`
Expected: still fails (providers don't implement StreamWithTools).

- [ ] **Step 6: Commit**

```bash
git add pkg/llm/types.go pkg/llm/provider.go pkg/tools/loop_test.go
git commit -s -m "feat(llm): add StreamWithTools to Provider interface

Adds StreamEvent types and StreamWithTools method with callback-based
streaming API. Providers will implement this in subsequent commits.

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 2: Implement Anthropic provider streaming

**Files:**
- Modify: `internal/anthropic/anthropic.go`

- [ ] **Step 1: Implement StreamWithTools**

Add the following method to `AnthropicProvider` in
`internal/anthropic/anthropic.go`. The existing imports already
include the anthropic SDK. This method replaces the non-streaming
call with the SDK's `NewStreaming` + `Accumulate` pattern:

```go
// StreamWithTools sends a streaming request to the Anthropic API.
// It calls onEvent with text deltas as they arrive and returns the
// accumulated response.
func (p *AnthropicProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	onEvent func(llm.StreamEvent)) (*llm.Response, error) {

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Convert messages (same logic as CompleteWithTools)
	var anthropicMsgs []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemPrompt = msg.Content
		case "user":
			anthropicMsgs = append(anthropicMsgs,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Args, tc.Name))
			}
			anthropicMsgs = append(anthropicMsgs,
				anthropic.MessageParam{Role: "assistant", Content: blocks})
		case "tool":
			var blocks []anthropic.ContentBlockParamUnion
			for _, tr := range msg.ToolResults {
				blocks = append(blocks, anthropic.NewToolResultBlock(
					tr.ToolUseID, tr.Output, tr.IsError))
			}
			anthropicMsgs = append(anthropicMsgs,
				anthropic.MessageParam{Role: "user", Content: blocks})
		}
	}

	// Convert tool definitions
	var anthropicTools []anthropic.ToolUnionParam
	for _, td := range tools {
		properties := td.Parameters["properties"]
		var required []string
		switch v := td.Parameters["required"].(type) {
		case []string:
			required = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					required = append(required, s)
				}
			}
		}

		schema := anthropic.ToolInputSchemaParam{
			Properties: properties,
			Required:   required,
		}
		tool := anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        td.Name,
				Description: anthropic.String(td.Description),
				InputSchema: schema,
			},
		}
		anthropicTools = append(anthropicTools, tool)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: int64(p.maxTokens),
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}

	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	accumulated := &anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := accumulated.Accumulate(event); err != nil {
			if onEvent != nil {
				onEvent(llm.StreamEvent{
					Type:    llm.StreamEventError,
					Content: err.Error(),
				})
			}
			return nil, fmt.Errorf("stream accumulate error: %w", err)
		}

		// Emit text deltas
		if onEvent != nil && event.Type == "content_block_delta" {
			delta := event.Delta.AsTextDelta()
			if delta.Text != "" {
				onEvent(llm.StreamEvent{
					Type:    llm.StreamEventTextDelta,
					Content: delta.Text,
				})
			}
		}
	}

	if err := stream.Err(); err != nil {
		if onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventError,
				Content: err.Error(),
			})
		}
		return nil, fmt.Errorf("API stream failed: %w", err)
	}

	// Build response from accumulated message
	resp := &llm.Response{
		Usage: llm.Usage{
			InputTokens:      int(accumulated.Usage.InputTokens),
			OutputTokens:     int(accumulated.Usage.OutputTokens),
			CacheReadTokens:  int(accumulated.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(accumulated.Usage.CacheCreationInputTokens),
			TotalTokens:      int(accumulated.Usage.InputTokens + accumulated.Usage.OutputTokens),
		},
	}
	switch accumulated.StopReason {
	case "tool_use":
		resp.StopReason = llm.StopReasonToolUse
	default:
		resp.StopReason = llm.StopReasonEndTurn
	}

	for _, block := range accumulated.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			var args map[string]any
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &args); err != nil {
					return nil, fmt.Errorf("failed to parse tool input: %w", err)
				}
			}
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:   block.ID,
				Name: block.Name,
				Args: args,
			})
		}
	}

	if onEvent != nil {
		onEvent(llm.StreamEvent{
			Type:  llm.StreamEventDone,
			Usage: resp.Usage,
		})
	}

	return resp, nil
}
```

- [ ] **Step 2: Simplify CompleteWithTools to delegate**

Replace the existing `CompleteWithTools` method body with:

```go
func (p *AnthropicProvider) CompleteWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return p.StreamWithTools(ctx, messages, tools, nil)
}
```

Remove the old implementation body (the message conversion, API
call, and response parsing code that is now in `StreamWithTools`).

- [ ] **Step 3: Verify Anthropic provider compiles**

Run: `go build ./internal/anthropic/ 2>&1`
Expected: builds OK.

- [ ] **Step 4: Commit**

```bash
git add internal/anthropic/anthropic.go
git commit -s -m "feat(anthropic): implement StreamWithTools with native streaming

Uses SDK's Messages.NewStreaming + Accumulate for token-by-token
delivery. CompleteWithTools now delegates to StreamWithTools(nil).

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 3: Implement OpenAI provider streaming

**Files:**
- Modify: `internal/openai/openai.go`
- Create: `internal/openai/openai_test.go`

- [ ] **Step 1: Write failing test for SSE streaming**

Create `internal/openai/openai_test.go`:

```go
package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestStreamWithTools(t *testing.T) {
	sseResponse := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse)
	}))
	defer server.Close()

	provider := &OpenAICompatProvider{
		baseURL:      server.URL,
		apiKey:       "test-key",
		model:        "test-model",
		maxTokens:    100,
		timeout:      30,
		client:       server.Client(),
		providerName: "test",
	}

	var deltas []string
	var doneEvent *llm.StreamEvent

	onEvent := func(e llm.StreamEvent) {
		switch e.Type {
		case llm.StreamEventTextDelta:
			deltas = append(deltas, e.Content)
		case llm.StreamEventDone:
			doneEvent = &e
		}
	}

	msgs := []llm.Message{{Role: "user", Content: "Hi"}}
	resp, err := provider.StreamWithTools(context.Background(), msgs, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify streaming deltas
	if len(deltas) != 2 {
		t.Fatalf("expected 2 text deltas, got %d: %v", len(deltas), deltas)
	}
	if deltas[0] != "Hello" || deltas[1] != " world" {
		t.Fatalf("unexpected deltas: %v", deltas)
	}

	// Verify accumulated response
	if resp.Content != "Hello world" {
		t.Fatalf("expected content 'Hello world', got %q", resp.Content)
	}
	if resp.StopReason != llm.StopReasonEndTurn {
		t.Fatalf("expected EndTurn, got %v", resp.StopReason)
	}

	// Verify done event
	if doneEvent == nil {
		t.Fatal("expected Done event")
	}
	if doneEvent.Usage.TotalTokens != 12 {
		t.Fatalf("expected 12 total tokens, got %d", doneEvent.Usage.TotalTokens)
	}
}

func TestStreamWithToolsNilCallback(t *testing.T) {
	sseResponse := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse)
	}))
	defer server.Close()

	provider := &OpenAICompatProvider{
		baseURL:      server.URL,
		apiKey:       "test-key",
		model:        "test-model",
		maxTokens:    100,
		timeout:      30,
		client:       server.Client(),
		providerName: "test",
	}

	msgs := []llm.Message{{Role: "user", Content: "Hi"}}
	resp, err := provider.StreamWithTools(context.Background(), msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Fatalf("expected 'Hello', got %q", resp.Content)
	}
}

func TestStreamWithToolsToolCalls(t *testing.T) {
	sseResponse := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Let me check."},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse)
	}))
	defer server.Close()

	provider := &OpenAICompatProvider{
		baseURL:      server.URL,
		apiKey:       "test-key",
		model:        "test-model",
		maxTokens:    100,
		timeout:      30,
		client:       server.Client(),
		providerName: "test",
	}

	var deltas []string
	onEvent := func(e llm.StreamEvent) {
		if e.Type == llm.StreamEventTextDelta {
			deltas = append(deltas, e.Content)
		}
	}

	msgs := []llm.Message{{Role: "user", Content: "Weather?"}}
	resp, err := provider.StreamWithTools(context.Background(), msgs, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StopReason != llm.StopReasonToolUse {
		t.Fatalf("expected ToolUse, got %v", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("expected get_weather, got %q", resp.ToolCalls[0].Name)
	}
	city, _ := resp.ToolCalls[0].Args["city"].(string)
	if city != "NYC" {
		t.Fatalf("expected city NYC, got %q", city)
	}

	// Text delta should still have been emitted
	if len(deltas) != 1 || deltas[0] != "Let me check." {
		t.Fatalf("expected text delta 'Let me check.', got %v", deltas)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/openai/ -v -run TestStream 2>&1 | head -5`
Expected: compilation error — `StreamWithTools` not defined.

- [ ] **Step 3: Add SSE streaming types**

Add these types to `internal/openai/openai.go` after the existing
type definitions (after `openAIChatResponseWithTools`):

```go
// openAIStreamChunk represents a single SSE chunk from the streaming API.
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string                  `json:"role,omitempty"`
			Content   string                  `json:"content,omitempty"`
			ToolCalls []openAIStreamToolDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
}

// openAIStreamToolDelta represents a tool call delta in streaming.
type openAIStreamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// openAIStreamRequest extends the request with streaming support.
type openAIStreamRequest struct {
	Model     string            `json:"model"`
	Messages  []json.RawMessage `json:"messages"`
	Tools     []openAIToolDef   `json:"tools,omitempty"`
	MaxTokens int               `json:"max_tokens,omitempty"`
	Stream    bool              `json:"stream"`
}
```

- [ ] **Step 4: Implement StreamWithTools**

Add this method to `OpenAICompatProvider`. Add `"bufio"` and
`"strings"` to the import block if not already present:

```go
// StreamWithTools sends a streaming request to the OpenAI-compatible API.
func (p *OpenAICompatProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	onEvent func(llm.StreamEvent)) (*llm.Response, error) {

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Convert messages (same as CompleteWithTools)
	var openaiMsgs []json.RawMessage
	for _, msg := range messages {
		var raw []byte
		var err error

		switch msg.Role {
		case "system", "user":
			raw, err = json.Marshal(map[string]string{
				"role":    msg.Role,
				"content": msg.Content,
			})
		case "assistant":
			assistantMsg := map[string]any{"role": "assistant"}
			if msg.Content != "" {
				assistantMsg["content"] = msg.Content
			}
			if len(msg.ToolCalls) > 0 {
				var tcs []openAIToolCall
				for _, tc := range msg.ToolCalls {
					argsJSON, marshalErr := json.Marshal(tc.Args)
					if marshalErr != nil {
						return nil, fmt.Errorf("failed to marshal tool args: %w", marshalErr)
					}
					tcs = append(tcs, openAIToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{Name: tc.Name, Arguments: string(argsJSON)},
					})
				}
				assistantMsg["tool_calls"] = tcs
			}
			raw, err = json.Marshal(assistantMsg)
		case "tool":
			for _, tr := range msg.ToolResults {
				toolMsg, marshalErr := json.Marshal(map[string]string{
					"role":         "tool",
					"tool_call_id": tr.ToolUseID,
					"content":      tr.Output,
				})
				if marshalErr != nil {
					return nil, fmt.Errorf("failed to marshal tool result: %w", marshalErr)
				}
				openaiMsgs = append(openaiMsgs, toolMsg)
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to marshal message: %w", err)
		}
		openaiMsgs = append(openaiMsgs, raw)
	}

	// Convert tool definitions
	var openaiTools []openAIToolDef
	for _, td := range tools {
		def := openAIToolDef{Type: "function"}
		def.Function.Name = td.Name
		def.Function.Description = td.Description
		def.Function.Parameters = td.Parameters
		openaiTools = append(openaiTools, def)
	}

	reqBody := openAIStreamRequest{
		Model:     p.model,
		Messages:  openaiMsgs,
		Tools:     openaiTools,
		MaxTokens: p.maxTokens,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	httpResp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("API returned status %d: %s",
			httpResp.StatusCode, string(body))
	}

	// Parse SSE stream
	var content strings.Builder
	var finishReason string
	var usage llm.Usage
	toolCalls := map[int]*openAIToolCall{}

	scanner := bufio.NewScanner(httpResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			if line == "data: [DONE]" {
				break
			}
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Text content
		if choice.Delta.Content != "" {
			content.WriteString(choice.Delta.Content)
			if onEvent != nil {
				onEvent(llm.StreamEvent{
					Type:    llm.StreamEventTextDelta,
					Content: choice.Delta.Content,
				})
			}
		}

		// Tool call deltas
		for _, tcd := range choice.Delta.ToolCalls {
			tc, ok := toolCalls[tcd.Index]
			if !ok {
				tc = &openAIToolCall{ID: tcd.ID, Type: tcd.Type}
				tc.Function.Name = tcd.Function.Name
				toolCalls[tcd.Index] = tc
			}
			tc.Function.Arguments += tcd.Function.Arguments
		}

		// Finish reason
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}

		// Usage (some providers include it in the last chunk)
		if chunk.Usage != nil {
			usage = llm.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				TotalTokens:  chunk.Usage.TotalTokens,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventError,
				Content: err.Error(),
			})
		}
		return nil, fmt.Errorf("stream read error: %w", err)
	}

	// Build response
	resp := &llm.Response{
		Content: content.String(),
		Usage:   usage,
	}

	if finishReason == "tool_calls" {
		resp.StopReason = llm.StopReasonToolUse
	} else {
		resp.StopReason = llm.StopReasonEndTurn
	}

	for i := 0; i < len(toolCalls); i++ {
		tc := toolCalls[i]
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		})
	}

	if onEvent != nil {
		onEvent(llm.StreamEvent{
			Type:  llm.StreamEventDone,
			Usage: resp.Usage,
		})
	}

	return resp, nil
}
```

- [ ] **Step 5: Simplify CompleteWithTools to delegate**

Replace the body of `CompleteWithTools` in
`internal/openai/openai.go` with:

```go
func (p *OpenAICompatProvider) CompleteWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return p.StreamWithTools(ctx, messages, tools, nil)
}
```

Remove the old method body (message conversion, HTTP call,
response parsing).

- [ ] **Step 6: Add `"bufio"` to imports if not present**

Ensure `internal/openai/openai.go` imports include `"bufio"`. The
file already imports `"bytes"`, `"context"`, `"encoding/json"`,
`"fmt"`, `"io"`, `"net/http"`, `"strings"`, `"time"`.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/openai/ -v 2>&1`
Expected: all 3 streaming tests PASS.

- [ ] **Step 8: Run full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: all tests PASS.

- [ ] **Step 9: Run linter**

Run: `golangci-lint run ./internal/openai/ 2>&1`
Expected: 0 issues.

- [ ] **Step 10: Commit**

```bash
git add internal/openai/openai.go internal/openai/openai_test.go
git commit -s -m "feat(openai): implement StreamWithTools with SSE parsing

Parses SSE line-by-line, emits text deltas via callback, accumulates
tool call deltas by index. CompleteWithTools now delegates to
StreamWithTools(nil).

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 4: Wire real streaming into OpenAI API endpoint

**Files:**
- Modify: `internal/openaiapi/handler.go`
- Modify: `internal/openaiapi/stream.go`
- Modify: `internal/openaiapi/handler_test.go`

- [ ] **Step 1: Add StreamFromProvider to stream.go**

Add this function to `internal/openaiapi/stream.go`. It needs
`"time"` and `"encoding/json"` in imports (already present):

```go
// StreamFromProvider writes real SSE chunks as they arrive from the
// LLM provider's streaming callback.
func StreamFromProvider(w http.ResponseWriter, id, model string) (
	onEvent func(llm.StreamEvent), flush func()) {

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		flusher = noopFlusher{}
	}

	created := time.Now().Unix()

	// Send initial role chunk
	writeChunk(w, ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []ChatChunkChoice{
			{Index: 0, Delta: ChatDelta{Role: "assistant"}},
		},
	})
	flusher.Flush()

	onEvent = func(event llm.StreamEvent) {
		switch event.Type {
		case llm.StreamEventTextDelta:
			writeChunk(w, ChatCompletionChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []ChatChunkChoice{
					{Index: 0, Delta: ChatDelta{Content: event.Content}},
				},
			})
			flusher.Flush()
		case llm.StreamEventDone:
			stop := "stop"
			writeChunk(w, ChatCompletionChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []ChatChunkChoice{
					{Index: 0, Delta: ChatDelta{}, FinishReason: &stop},
				},
			})
			flusher.Flush()
			_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		case llm.StreamEventError:
			StreamError(w, event.Content)
		}
	}

	flush = func() {
		flusher.Flush()
	}

	return onEvent, flush
}
```

Add `"github.com/redhat-et/docsclaw/pkg/llm"` to the imports in
`stream.go`.

- [ ] **Step 2: Update handler to use real streaming for phase 1**

In `internal/openaiapi/handler.go`, modify the `ChatCompletion`
method. Replace the streaming/non-streaming section (after the
error check and before the end of the function) with logic that
uses real streaming for phase 1:

Replace this section (approximately lines 58-80):

```go
	id := GenerateID()
	model := "docsclaw"
	if h.Provider != nil {
		model = h.Provider.Model()
	}

	if req.Stream {
		StreamResponse(w, id, model, content)
		return
	}

	resp := BuildResponse(id, model, content, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
```

With:

```go
	id := GenerateID()
	model := "docsclaw"
	if h.Provider != nil {
		model = h.Provider.Model()
	}

	if req.Stream {
		h.streamCompletion(w, r, id, model, systemPrompt, msgs)
		return
	}

	resp := BuildResponse(id, model, content, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
```

But wait — for streaming we need to skip the `h.complete()` call
earlier. Restructure `ChatCompletion` so that streaming takes a
different path. The full replacement for the method body after
message conversion:

```go
	msgs, systemPrompt := ConvertMessages(req.Messages, h.SystemPrompt)

	id := GenerateID()
	model := "docsclaw"
	if h.Provider != nil {
		model = h.Provider.Model()
	}

	if req.Stream {
		h.streamCompletion(w, id, model, systemPrompt, msgs)
		return
	}

	content, usage, err := h.complete(r.Context(), systemPrompt, msgs)
	if err != nil {
		slog.Error("LLM completion failed", "error", err)
		writeError(w, http.StatusBadGateway, "server_error",
			"llm_error", "LLM error: "+err.Error())
		return
	}

	resp := BuildResponse(id, model, content, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
```

- [ ] **Step 3: Add streamCompletion method**

Add this method to `Handler` in `handler.go`:

```go
// streamCompletion handles streaming responses. Phase 1 uses real
// provider streaming; phase 2 runs the tool loop then simulates.
func (h *Handler) streamCompletion(w http.ResponseWriter,
	id, model, systemPrompt string, msgs []llm.Message) {

	if h.Provider == nil {
		StreamResponse(w, id, model, "LLM provider not configured.")
		return
	}

	// Phase 2: tool loop + simulated streaming
	if h.Registry != nil && len(h.Registry.Definitions()) > 0 {
		allMsgs := append([]llm.Message{{
			Role:    "system",
			Content: systemPrompt,
		}}, msgs...)

		content, err := tools.RunToolLoop(context.Background(),
			h.Provider, allMsgs, h.Registry, h.LoopConfig)
		if err != nil {
			slog.Error("tool loop failed", "error", err)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			StreamError(w, "LLM error: "+err.Error())
			return
		}
		StreamResponse(w, id, model, content)
		return
	}

	// Phase 1: real streaming from provider
	allMsgs := append([]llm.Message{{
		Role:    "system",
		Content: systemPrompt,
	}}, msgs...)

	onEvent, _ := StreamFromProvider(w, id, model)
	_, err := h.Provider.StreamWithTools(context.Background(),
		allMsgs, nil, onEvent)
	if err != nil {
		slog.Error("streaming failed", "error", err)
	}
}
```

- [ ] **Step 4: Update handler_test.go mock provider**

Add `StreamWithTools` to the mock provider in
`internal/openaiapi/handler_test.go`:

```go
func (m *mockProvider) StreamWithTools(_ context.Context,
	msgs []llm.Message, tools []llm.ToolDefinition,
	onEvent func(llm.StreamEvent)) (*llm.Response, error) {

	resp, err := m.CompleteWithTools(context.Background(), msgs, tools)
	if err != nil {
		return nil, err
	}

	if onEvent != nil {
		// Simulate streaming by emitting the content as a single delta
		if resp.Content != "" {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventTextDelta,
				Content: resp.Content,
			})
		}
		onEvent(llm.StreamEvent{
			Type:  llm.StreamEventDone,
			Usage: resp.Usage,
		})
	}

	return resp, nil
}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./... 2>&1 | tail -20`
Expected: all tests PASS.

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./internal/openaiapi/ ./internal/openai/ 2>&1`
Expected: 0 issues.

- [ ] **Step 7: Commit**

```bash
git add internal/openaiapi/handler.go internal/openaiapi/stream.go \
       internal/openaiapi/handler_test.go
git commit -s -m "feat(openaiapi): wire real provider streaming for phase 1

Phase 1 streams tokens directly from the provider via
StreamFromProvider callback. Phase 2 keeps simulated streaming
via the existing StreamResponse function.

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 5: Final verification

**Files:** None (testing only)

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: all tests PASS.

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: 0 issues.

- [ ] **Step 3: Build**

Run: `make build`
Expected: binary builds to `bin/docsclaw`.

- [ ] **Step 4: Manual test with curl (non-streaming)**

```bash
curl -s http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"docsclaw","messages":[{"role":"user","content":"Say hello in one word."}]}' | jq .
```

Expected: JSON response with content.

- [ ] **Step 5: Manual test with curl (streaming)**

```bash
curl -s -N http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"docsclaw","stream":true,"messages":[{"role":"user","content":"Say hello in one word."}]}'
```

Expected: SSE chunks arriving one token at a time (not word-by-word
bursts), ending with `data: [DONE]`.
