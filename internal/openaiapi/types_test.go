package openaiapi

import (
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestConvertMessages(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "You are a helpful client assistant."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}
	serverPrompt := "You are the server system prompt."

	got, sysPrompt := ConvertMessages(msgs, serverPrompt)

	// System message should be excluded from the result.
	for _, m := range got {
		if m.Role == "system" {
			t.Fatal("system message should not appear in converted messages")
		}
	}

	// Expect 3 non-system messages.
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}

	// Check roles and content.
	wantRoles := []string{"user", "assistant", "user"}
	wantContent := []string{"Hello", "Hi there!", "How are you?"}
	for i, m := range got {
		if m.Role != wantRoles[i] {
			t.Errorf("message %d: role = %q, want %q", i, m.Role, wantRoles[i])
		}
		if m.Content != wantContent[i] {
			t.Errorf("message %d: content = %q, want %q", i, m.Content, wantContent[i])
		}
	}

	// System prompt should be server prompt + client system message.
	if !strings.HasPrefix(sysPrompt, serverPrompt) {
		t.Errorf("system prompt should start with server prompt, got %q", sysPrompt)
	}
	if !strings.Contains(sysPrompt, "You are a helpful client assistant.") {
		t.Errorf("system prompt should contain client system message, got %q", sysPrompt)
	}
	// Separator should be "\n\n".
	expected := serverPrompt + "\n\n" + "You are a helpful client assistant."
	if sysPrompt != expected {
		t.Errorf("system prompt = %q, want %q", sysPrompt, expected)
	}
}

func TestConvertMessagesNoClientSystem(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}
	serverPrompt := "You are the server system prompt."

	got, sysPrompt := ConvertMessages(msgs, serverPrompt)

	// Without client system message, server prompt should be unchanged.
	if sysPrompt != serverPrompt {
		t.Errorf("system prompt = %q, want %q", sysPrompt, serverPrompt)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if got[0].Role != "user" || got[0].Content != "Hello" {
		t.Errorf("unexpected message: %+v", got[0])
	}
}

func TestBuildResponse(t *testing.T) {
	usage := llm.Usage{
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
	}
	resp := BuildResponse("chatcmpl-abc123", "test-model", "Hello!", usage)

	if resp.ID != "chatcmpl-abc123" {
		t.Errorf("ID = %q, want %q", resp.ID, "chatcmpl-abc123")
	}
	if resp.Object != "chat.completion" {
		t.Errorf("Object = %q, want %q", resp.Object, "chat.completion")
	}
	if resp.Model != "test-model" {
		t.Errorf("Model = %q, want %q", resp.Model, "test-model")
	}
	if resp.Created == 0 {
		t.Error("Created should not be zero")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}

	choice := resp.Choices[0]
	if choice.Index != 0 {
		t.Errorf("choice Index = %d, want 0", choice.Index)
	}
	if choice.Message.Role != "assistant" {
		t.Errorf("choice Message.Role = %q, want %q", choice.Message.Role, "assistant")
	}
	if choice.Message.Content != "Hello!" {
		t.Errorf("choice Message.Content = %q, want %q", choice.Message.Content, "Hello!")
	}
	if choice.FinishReason != "stop" {
		t.Errorf("choice FinishReason = %q, want %q", choice.FinishReason, "stop")
	}

	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 20 {
		t.Errorf("CompletionTokens = %d, want 20", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, want 30", resp.Usage.TotalTokens)
	}
}

func TestGenerateID(t *testing.T) {
	id := GenerateID()
	if !strings.HasPrefix(id, "chatcmpl-") {
		t.Errorf("ID should start with 'chatcmpl-', got %q", id)
	}
	// Should have some hex after the prefix.
	suffix := strings.TrimPrefix(id, "chatcmpl-")
	if len(suffix) == 0 {
		t.Error("ID suffix should not be empty")
	}

	// Two calls should produce different IDs.
	id2 := GenerateID()
	if id == id2 {
		t.Error("GenerateID should produce unique IDs")
	}
}
