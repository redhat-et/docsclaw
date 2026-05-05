# OpenAI-Compatible API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/v1/chat/completions`, `/v1/models`, and `/v1/skills`
endpoints to DocsClaw so users can connect OpenAI-compatible chat UIs.

**Architecture:** New `internal/openaiapi/` package with OpenAI
request/response types, HTTP handlers, and SSE streaming. Handlers
call the LLM provider and tool loop directly (no A2A bridge). Routes
registered on the existing serve mux alongside A2A endpoints.

**Tech Stack:** Go stdlib (`net/http`, `encoding/json`), existing
`pkg/llm`, `pkg/tools`, `pkg/skills` packages. No new dependencies.

---

## File structure

| File | Responsibility |
|------|---------------|
| Create: `internal/openaiapi/types.go` | OpenAI request/response structs |
| Create: `internal/openaiapi/types_test.go` | Message conversion tests |
| Create: `internal/openaiapi/handler.go` | HTTP handlers for all three endpoints |
| Create: `internal/openaiapi/handler_test.go` | Integration tests (HTTP round-trip) |
| Create: `internal/openaiapi/stream.go` | SSE streaming logic |
| Create: `internal/openaiapi/stream_test.go` | SSE encoding tests |
| Modify: `internal/cmd/serve.go` | Register `/v1/*` routes on the mux |

---

### Task 1: OpenAI types and message conversion

**Files:**
- Create: `internal/openaiapi/types.go`
- Create: `internal/openaiapi/types_test.go`

- [ ] **Step 1: Write failing tests for message conversion**

Create `internal/openaiapi/types_test.go`:

```go
package openaiapi

import (
	"testing"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestConvertMessages(t *testing.T) {
	input := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}

	systemPrompt := "I am DocsClaw."
	msgs, merged := ConvertMessages(input, systemPrompt)

	if merged != "I am DocsClaw.\n\nYou are helpful." {
		t.Fatalf("expected merged system prompt, got %q", merged)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (no system), got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Hi there!" {
		t.Fatalf("unexpected second message: %+v", msgs[1])
	}
}

func TestConvertMessagesNoClientSystem(t *testing.T) {
	input := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	_, merged := ConvertMessages(input, "I am DocsClaw.")

	if merged != "I am DocsClaw." {
		t.Fatalf("expected unchanged system prompt, got %q", merged)
	}
}

func TestBuildResponse(t *testing.T) {
	resp := BuildResponse("test-id", "test-model", "Hello!", llm.Usage{
		InputTokens:  10,
		OutputTokens: 5,
		TotalTokens:  15,
	})

	if resp.ID != "test-id" {
		t.Fatalf("expected id test-id, got %q", resp.ID)
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("expected object chat.completion, got %q", resp.Object)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", resp.Choices[0].Message.Role)
	}
	if resp.Choices[0].Message.Content != "Hello!" {
		t.Fatalf("expected content 'Hello!', got %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("expected finish_reason stop, got %q", resp.Choices[0].FinishReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Fatalf("expected prompt_tokens 10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Fatalf("expected completion_tokens 5, got %d", resp.Usage.CompletionTokens)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/openaiapi/ -v`
Expected: compilation error — package does not exist.

- [ ] **Step 3: Implement types and conversion functions**

Create `internal/openaiapi/types.go`:

```go
package openaiapi

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

// ChatCompletionRequest is the OpenAI chat completions request body.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

// ChatMessage is a single message in the OpenAI format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse is the OpenAI chat completions response.
type ChatCompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatChoice       `json:"choices"`
	Usage   ChatUsage          `json:"usage"`
}

// ChatChoice is a single completion choice.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatUsage maps LLM usage to OpenAI's token count format.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionChunk is a single SSE chunk for streaming.
type ChatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []ChatChunkChoice `json:"choices"`
}

// ChatChunkChoice is a single choice in a streaming chunk.
type ChatChunkChoice struct {
	Index        int          `json:"index"`
	Delta        ChatDelta    `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

// ChatDelta is the incremental content in a streaming chunk.
type ChatDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ErrorResponse is the OpenAI error format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error information.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// ModelObject is a single entry in the /v1/models response.
type ModelObject struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy    string `json:"owned_by"`
	Description string `json:"description,omitempty"`
}

// ModelList is the /v1/models response.
type ModelList struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// SkillObject is a single entry in the /v1/skills response.
type SkillObject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillList is the /v1/skills response.
type SkillList struct {
	Skills []SkillObject `json:"skills"`
}

// ConvertMessages converts OpenAI chat messages to llm.Message
// slices and merges system prompts. The server's system prompt
// takes priority; client system messages are appended after it.
// Returns the non-system messages and the merged system prompt.
func ConvertMessages(msgs []ChatMessage, serverPrompt string) ([]llm.Message, string) {
	systemPrompt := serverPrompt
	var converted []llm.Message

	for _, m := range msgs {
		switch m.Role {
		case "system":
			systemPrompt = serverPrompt + "\n\n" + m.Content
		case "user", "assistant":
			converted = append(converted, llm.Message{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}

	return converted, systemPrompt
}

// BuildResponse creates a ChatCompletionResponse from an LLM result.
func BuildResponse(id, model, content string, usage llm.Usage) ChatCompletionResponse {
	return ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatChoice{
			{
				Index:        0,
				Message:      ChatMessage{Role: "assistant", Content: content},
				FinishReason: "stop",
			},
		},
		Usage: ChatUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.TotalTokens,
		},
	}
}

// GenerateID creates a unique completion ID.
func GenerateID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("chatcmpl-%x", b)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/openaiapi/ -v`
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/openaiapi/types.go internal/openaiapi/types_test.go
git commit -s -m "feat(openaiapi): add OpenAI types and message conversion

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 2: SSE streaming

**Files:**
- Create: `internal/openaiapi/stream.go`
- Create: `internal/openaiapi/stream_test.go`

- [ ] **Step 1: Write failing tests for SSE encoding**

Create `internal/openaiapi/stream_test.go`:

```go
package openaiapi

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamResponse(t *testing.T) {
	w := httptest.NewRecorder()
	content := "Hello world, this is a test response."

	StreamResponse(w, "test-id", "test-model", content)

	result := w.Result()
	defer result.Body.Close()

	if ct := result.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	scanner := bufio.NewScanner(result.Body)
	var chunks []ChatCompletionChunk
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			break
		}
		data := strings.TrimPrefix(line, "data: ")
		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("failed to parse chunk: %v\nline: %s", err, data)
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk has role
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Fatalf("expected first chunk to set role, got %+v", chunks[0].Choices[0].Delta)
	}

	// Last chunk has finish_reason
	last := chunks[len(chunks)-1]
	if last.Choices[0].FinishReason == nil || *last.Choices[0].FinishReason != "stop" {
		t.Fatal("expected last chunk to have finish_reason stop")
	}

	// Reconstruct content from all chunks
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Choices[0].Delta.Content)
	}
	if got := sb.String(); got != content {
		t.Fatalf("reconstructed content mismatch:\nexpected: %q\ngot:      %q", content, got)
	}
}

func TestStreamResponseEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	StreamResponse(w, "test-id", "test-model", "")

	result := w.Result()
	defer result.Body.Close()

	scanner := bufio.NewScanner(result.Body)
	var hasDone bool
	for scanner.Scan() {
		if scanner.Text() == "data: [DONE]" {
			hasDone = true
		}
	}
	if !hasDone {
		t.Fatal("expected [DONE] terminator")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/openaiapi/ -v -run TestStream`
Expected: compilation error — `StreamResponse` not defined.

- [ ] **Step 3: Implement SSE streaming**

Create `internal/openaiapi/stream.go`:

```go
package openaiapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StreamResponse writes an SSE-streamed chat completion response.
// The content is split into word-sized chunks to simulate streaming.
func StreamResponse(w http.ResponseWriter, id, model, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		flusher = noopFlusher{}
	}

	created := time.Now().Unix()

	// First chunk: set the role
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

	// Content chunks: split by words
	if content != "" {
		words := strings.Fields(content)
		for i, word := range words {
			text := word
			if i < len(words)-1 {
				text += " "
			}
			writeChunk(w, ChatCompletionChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []ChatChunkChoice{
					{Index: 0, Delta: ChatDelta{Content: text}},
				},
			})
			flusher.Flush()
		}
	}

	// Final chunk: finish_reason
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

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// StreamError writes an error as an SSE event followed by [DONE].
func StreamError(w http.ResponseWriter, msg string) {
	errResp := ErrorResponse{
		Error: ErrorDetail{
			Message: msg,
			Type:    "server_error",
			Code:    "internal_error",
		},
	}
	data, _ := json.Marshal(errResp)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeChunk(w http.ResponseWriter, chunk ChatCompletionChunk) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// noopFlusher satisfies http.Flusher for non-streaming writers
// (e.g., httptest.ResponseRecorder).
type noopFlusher struct{}

func (noopFlusher) Flush() {}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/openaiapi/ -v -run TestStream`
Expected: both stream tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/openaiapi/stream.go internal/openaiapi/stream_test.go
git commit -s -m "feat(openaiapi): add SSE streaming for chat completions

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 3: HTTP handlers

**Files:**
- Create: `internal/openaiapi/handler.go`
- Create: `internal/openaiapi/handler_test.go`

- [ ] **Step 1: Write failing tests for all three endpoints**

Create `internal/openaiapi/handler_test.go`:

```go
package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/redhat-et/docsclaw/pkg/llm"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// mockProvider implements llm.Provider for handler tests.
type mockProvider struct {
	response string
}

func (m *mockProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return m.response, nil
}

func (m *mockProvider) CompleteWithTools(_ context.Context,
	msgs []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{
		StopReason: llm.StopReasonEndTurn,
		Content:    m.response,
		Usage: llm.Usage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
	}, nil
}

func (m *mockProvider) Model() string        { return "mock-model" }
func (m *mockProvider) ProviderName() string { return "mock" }

func TestChatCompletionHandler(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "Hello from DocsClaw!"},
		SystemPrompt: "You are DocsClaw.",
		Registry:     tools.NewRegistry(nil),
		LoopConfig:   tools.DefaultLoopConfig(),
	}

	body := ChatCompletionRequest{
		Model: "anything",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hi"},
		},
	}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Choices[0].Message.Content != "Hello from DocsClaw!" {
		t.Fatalf("unexpected content: %q", resp.Choices[0].Message.Content)
	}
	if resp.Model != "mock-model" {
		t.Fatalf("expected model mock-model, got %q", resp.Model)
	}
}

func TestChatCompletionHandlerStreaming(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "Streamed hello!"},
		SystemPrompt: "You are DocsClaw.",
		Registry:     tools.NewRegistry(nil),
		LoopConfig:   tools.DefaultLoopConfig(),
	}

	body := ChatCompletionRequest{
		Model:  "anything",
		Stream: true,
		Messages: []ChatMessage{
			{Role: "user", Content: "Hi"},
		},
	}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatal("expected [DONE] terminator in streaming response")
	}
}

func TestChatCompletionHandlerInvalidJSON(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "unused"},
		SystemPrompt: "You are DocsClaw.",
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Fatalf("expected invalid_request_error, got %q", errResp.Error.Type)
	}
}

func TestChatCompletionHandlerEmptyMessages(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "unused"},
		SystemPrompt: "You are DocsClaw.",
	}

	body := ChatCompletionRequest{Messages: []ChatMessage{}}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestModelsHandler(t *testing.T) {
	h := &Handler{
		Provider:  &mockProvider{},
		AgentName: "TestAgent",
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	h.Models(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ModelList
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Object != "list" {
		t.Fatalf("expected object 'list', got %q", resp.Object)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "docsclaw" {
		t.Fatalf("expected id 'docsclaw', got %q", resp.Data[0].ID)
	}
	if resp.Data[0].OwnedBy != "docsclaw" {
		t.Fatalf("expected owned_by 'docsclaw', got %q", resp.Data[0].OwnedBy)
	}
}

func TestSkillsHandler(t *testing.T) {
	h := &Handler{
		AgentCard: &a2a.AgentCard{
			Skills: []a2a.AgentSkill{
				{ID: "summarize", Name: "Summarize", Description: "Summarizes docs"},
				{ID: "translate", Name: "Translate", Description: "Translates text"},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	w := httptest.NewRecorder()

	h.Skills(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp SkillList
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(resp.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(resp.Skills))
	}
	if resp.Skills[0].ID != "summarize" {
		t.Fatalf("expected first skill 'summarize', got %q", resp.Skills[0].ID)
	}
}

func TestSkillsHandlerNoSkills(t *testing.T) {
	h := &Handler{
		AgentCard: &a2a.AgentCard{},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	w := httptest.NewRecorder()

	h.Skills(w, req)

	var resp SkillList
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(resp.Skills))
	}
}

func TestChatCompletionPhase1(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "Phase 1 response"},
		SystemPrompt: "You are DocsClaw.",
	}

	body := ChatCompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatCompletionResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Choices[0].Message.Content != "Phase 1 response" {
		t.Fatalf("unexpected content: %q", resp.Choices[0].Message.Content)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/openaiapi/ -v -run 'TestChat|TestModels|TestSkills'`
Expected: compilation error — `Handler` type not defined.

- [ ] **Step 3: Implement the handlers**

Create `internal/openaiapi/handler.go`:

```go
package openaiapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/redhat-et/docsclaw/pkg/llm"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// Handler holds the dependencies for OpenAI-compatible endpoints.
type Handler struct {
	Provider     llm.Provider
	SystemPrompt string
	Registry     *tools.Registry
	LoopConfig   tools.LoopConfig
	AgentCard    *a2a.AgentCard
	AgentName    string
}

// ChatCompletion handles POST /v1/chat/completions.
func (h *Handler) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error",
			"invalid_request", "Invalid JSON: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request_error",
			"invalid_request", "messages must not be empty")
		return
	}

	msgs, systemPrompt := ConvertMessages(req.Messages, h.SystemPrompt)

	content, usage, err := h.complete(r.Context(), systemPrompt, msgs)
	if err != nil {
		slog.Error("completion failed", "error", err)
		if req.Stream {
			StreamError(w, err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "server_error",
			"upstream_error", err.Error())
		return
	}

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
	resp.Model = model

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// complete runs the LLM with or without tools depending on
// whether a tool registry is configured (phase 1 vs phase 2).
func (h *Handler) complete(ctx context.Context,
	systemPrompt string, msgs []llm.Message) (string, llm.Usage, error) {

	if h.Provider == nil {
		return "LLM provider not configured.", llm.Usage{}, nil
	}

	// Phase 2: tool-use loop
	if h.Registry != nil && len(h.Registry.Definitions()) > 0 {
		allMsgs := append([]llm.Message{
			{Role: "system", Content: systemPrompt},
		}, msgs...)

		content, err := tools.RunToolLoop(ctx, h.Provider, allMsgs,
			h.Registry, h.LoopConfig)
		if err != nil {
			return "", llm.Usage{}, err
		}
		return content, llm.Usage{}, nil
	}

	// Phase 1: single-shot
	userMsg := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			userMsg = msgs[i].Content
			break
		}
	}

	result, err := h.Provider.Complete(ctx, systemPrompt, userMsg)
	if err != nil {
		return "", llm.Usage{}, err
	}
	return result, llm.Usage{}, nil
}

// Models handles GET /v1/models.
func (h *Handler) Models(w http.ResponseWriter, _ *http.Request) {
	model := "docsclaw"
	desc := "DocsClaw agent"
	if h.Provider != nil {
		desc += " (backed by " + h.Provider.Model() + ")"
	}
	if h.AgentName != "" {
		desc = h.AgentName + " (backed by " + h.Provider.Model() + ")"
	}

	resp := ModelList{
		Object: "list",
		Data: []ModelObject{
			{
				ID:          "docsclaw",
				Object:      "model",
				Created:     time.Now().Unix(),
				OwnedBy:    "docsclaw",
				Description: desc,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Skills handles GET /v1/skills.
func (h *Handler) Skills(w http.ResponseWriter, _ *http.Request) {
	var skillObjs []SkillObject

	if h.AgentCard != nil {
		for _, s := range h.AgentCard.Skills {
			skillObjs = append(skillObjs, SkillObject{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
			})
		}
	}

	if skillObjs == nil {
		skillObjs = []SkillObject{}
	}

	resp := SkillList{Skills: skillObjs}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, status int, errType, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Message: msg,
			Type:    errType,
			Code:    code,
		},
	})
}
```

Note: The `complete` method needs to import `context`. Add this
to the import block:

```go
import (
	"context"
	"encoding/json"
	...
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/openaiapi/ -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/openaiapi/handler.go internal/openaiapi/handler_test.go
git commit -s -m "feat(openaiapi): add chat completion, models, and skills handlers

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 4: Register routes in serve command

**Files:**
- Modify: `internal/cmd/serve.go:396-454` (mux setup section)

- [ ] **Step 1: Write failing test for route registration**

There is no easy unit test for route registration — the serve
command does integration-level setup. Instead, we verify this
works by running the full test suite after the change and
then manually testing with curl.

Skip to implementation.

- [ ] **Step 2: Add import and register routes**

In `internal/cmd/serve.go`, add the import:

```go
"github.com/redhat-et/docsclaw/internal/openaiapi"
```

Then, after the A2A handler setup (after the line
`mux.Handle("POST /{$}", jsonrpcHandler)` at line 454),
add the OpenAI API routes:

```go
	// OpenAI-compatible API
	openaiHandler := &openaiapi.Handler{
		Provider:     llmProvider,
		SystemPrompt: systemPrompt + skillsSummary,
		Registry:     toolRegistry,
		LoopConfig:   loopCfg,
		AgentCard:    agentCard,
		AgentName:    agentCard.Name,
	}
	mux.HandleFunc("POST /v1/chat/completions", openaiHandler.ChatCompletion)
	mux.HandleFunc("GET /v1/models", openaiHandler.Models)
	mux.HandleFunc("GET /v1/skills", openaiHandler.Skills)
```

Also add a startup log line after the existing LLM log line
(around line 497):

```go
	log.Info("OpenAI-compatible API enabled",
		"endpoints", "/v1/chat/completions, /v1/models, /v1/skills")
```

- [ ] **Step 3: Run existing tests to verify no regressions**

Run: `go test ./... 2>&1 | tail -20`
Expected: all tests PASS. No compilation errors.

- [ ] **Step 4: Run linter**

Run: `golangci-lint run ./internal/openaiapi/ ./internal/cmd/`
Expected: no lint errors.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/serve.go
git commit -s -m "feat(serve): register OpenAI-compatible API routes

Adds /v1/chat/completions, /v1/models, and /v1/skills endpoints
alongside existing A2A routes on port 8000.

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 5: Manual verification with curl

**Files:** None (testing only)

- [ ] **Step 1: Start the server locally**

Create a minimal test config directory:

```bash
mkdir -p /tmp/docsclaw-test
echo "You are a helpful assistant." > /tmp/docsclaw-test/system-prompt.txt
```

Start the server:

```bash
export LLM_PROVIDER=anthropic  # or openai
export LLM_API_KEY=<your-key>
export LLM_MODEL=claude-sonnet-4-20250514  # or gpt-4o
go run ./cmd/docsclaw serve --config-dir /tmp/docsclaw-test
```

- [ ] **Step 2: Test /v1/models**

```bash
curl -s http://localhost:8000/v1/models | jq .
```

Expected: JSON with `"object": "list"` and one model entry.

- [ ] **Step 3: Test /v1/skills**

```bash
curl -s http://localhost:8000/v1/skills | jq .
```

Expected: JSON with `"skills": []` (empty, no skills configured
in the test directory).

- [ ] **Step 4: Test /v1/chat/completions (non-streaming)**

```bash
curl -s http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "messages": [{"role": "user", "content": "Say hello in one sentence."}]
  }' | jq .
```

Expected: JSON with `"object": "chat.completion"` and a response
in `choices[0].message.content`.

- [ ] **Step 5: Test /v1/chat/completions (streaming)**

```bash
curl -s -N http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "stream": true,
    "messages": [{"role": "user", "content": "Say hello in one sentence."}]
  }'
```

Expected: SSE stream with `data:` lines containing chunks,
ending with `data: [DONE]`.

- [ ] **Step 6: Test error handling**

```bash
curl -s http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d 'not json' | jq .
```

Expected: 400 with `"type": "invalid_request_error"`.

---

### Task 6: Final cleanup and full test run

**Files:** None

- [ ] **Step 1: Run full test suite**

```bash
make test
```

Expected: all tests PASS.

- [ ] **Step 2: Run linter**

```bash
make lint
```

Expected: no lint errors.

- [ ] **Step 3: Verify build**

```bash
make build
```

Expected: binary builds successfully at `bin/docsclaw`.
