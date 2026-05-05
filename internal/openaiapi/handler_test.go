package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/redhat-et/docsclaw/pkg/llm"
)

// mockProvider implements llm.Provider for testing.
type mockProvider struct {
	response string
}

func (m *mockProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return m.response, nil
}

func (m *mockProvider) CompleteWithTools(_ context.Context,
	_ []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{
		StopReason: llm.StopReasonEndTurn,
		Content:    m.response,
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}, nil
}

func (m *mockProvider) Model() string      { return "mock-model" }
func (m *mockProvider) ProviderName() string { return "mock" }

func TestChatCompletionHandler(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "Hello from the agent!"},
		SystemPrompt: "You are a test assistant.",
		AgentName:    "test-agent",
	}

	body := ChatCompletionRequest{
		Model:    "docsclaw",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ChatCompletion(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", result.StatusCode)
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(result.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Errorf("object = %q, want %q", resp.Object, "chat.completion")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello from the agent!" {
		t.Errorf("content = %q, want %q", resp.Choices[0].Message.Content, "Hello from the agent!")
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("role = %q, want %q", resp.Choices[0].Message.Role, "assistant")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want %q", resp.Choices[0].FinishReason, "stop")
	}
}

func TestChatCompletionHandlerStreaming(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "Streamed response here"},
		SystemPrompt: "You are a test assistant.",
		AgentName:    "test-agent",
	}

	body := ChatCompletionRequest{
		Model:    "docsclaw",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		Stream:   true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ChatCompletion(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	ct := result.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatal("expected [DONE] terminator in streaming response")
	}
	if !strings.Contains(bodyStr, "Streamed") {
		t.Fatal("expected content in streaming response")
	}
}

func TestChatCompletionHandlerInvalidJSON(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "unused"},
		SystemPrompt: "You are a test assistant.",
		AgentName:    "test-agent",
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ChatCompletion(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", result.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(result.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("error type = %q, want %q", errResp.Error.Type, "invalid_request_error")
	}
}

func TestChatCompletionHandlerEmptyMessages(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "unused"},
		SystemPrompt: "You are a test assistant.",
		AgentName:    "test-agent",
	}

	body := ChatCompletionRequest{
		Model:    "docsclaw",
		Messages: []ChatMessage{},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ChatCompletion(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", result.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(result.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("error type = %q, want %q", errResp.Error.Type, "invalid_request_error")
	}
}

func TestModelsHandler(t *testing.T) {
	h := &Handler{
		Provider:  &mockProvider{response: "unused"},
		AgentName: "my-agent",
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.Models(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", result.StatusCode)
	}

	var list ModelList
	if err := json.NewDecoder(result.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if list.Object != "list" {
		t.Errorf("object = %q, want %q", list.Object, "list")
	}
	if len(list.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(list.Data))
	}
	if list.Data[0].ID != "docsclaw" {
		t.Errorf("model id = %q, want %q", list.Data[0].ID, "docsclaw")
	}
	if list.Data[0].OwnedBy != "docsclaw" {
		t.Errorf("owned_by = %q, want %q", list.Data[0].OwnedBy, "docsclaw")
	}
	if !strings.Contains(list.Data[0].Description, "my-agent") {
		t.Errorf("description should mention agent name, got %q", list.Data[0].Description)
	}
}

func TestSkillsHandler(t *testing.T) {
	h := &Handler{
		AgentCard: &a2a.AgentCard{
			Skills: []a2a.AgentSkill{
				{ID: "skill-1", Name: "Skill One", Description: "First skill"},
				{ID: "skill-2", Name: "Skill Two", Description: "Second skill"},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	w := httptest.NewRecorder()
	h.Skills(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", result.StatusCode)
	}

	var list SkillList
	if err := json.NewDecoder(result.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(list.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(list.Skills))
	}
	if list.Skills[0].ID != "skill-1" {
		t.Errorf("skill[0].ID = %q, want %q", list.Skills[0].ID, "skill-1")
	}
	if list.Skills[0].Name != "Skill One" {
		t.Errorf("skill[0].Name = %q, want %q", list.Skills[0].Name, "Skill One")
	}
	if list.Skills[1].Description != "Second skill" {
		t.Errorf("skill[1].Description = %q, want %q", list.Skills[1].Description, "Second skill")
	}
}

func TestSkillsHandlerNoSkills(t *testing.T) {
	h := &Handler{
		AgentCard: &a2a.AgentCard{},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	w := httptest.NewRecorder()
	h.Skills(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", result.StatusCode)
	}

	var list SkillList
	if err := json.NewDecoder(result.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if list.Skills == nil {
		t.Fatal("skills should be an empty list, not nil")
	}
	if len(list.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(list.Skills))
	}
}

func TestChatCompletionPhase1(t *testing.T) {
	h := &Handler{
		Provider:     &mockProvider{response: "Phase 1 response"},
		SystemPrompt: "You are a phase 1 assistant.",
		AgentName:    "test-agent",
		Registry:     nil, // phase 1: no registry
	}

	body := ChatCompletionRequest{
		Model:    "docsclaw",
		Messages: []ChatMessage{{Role: "user", Content: "Hello phase 1"}},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ChatCompletion(w, req)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", result.StatusCode)
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(result.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Choices[0].Message.Content != "Phase 1 response" {
		t.Errorf("content = %q, want %q", resp.Choices[0].Message.Content, "Phase 1 response")
	}
}
