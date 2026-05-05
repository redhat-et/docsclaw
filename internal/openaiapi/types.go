// Package openaiapi defines types for the OpenAI-compatible Chat
// Completions API and provides conversion functions between OpenAI
// message format and the internal llm.Message format.
package openaiapi

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

// ChatCompletionRequest is the request body for POST /v1/chat/completions.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

// ChatMessage represents a single message in the OpenAI chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse is the non-streaming response from
// /v1/chat/completions.
type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   ChatUsage    `json:"usage"`
}

// ChatChoice represents a single completion choice.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatUsage reports token consumption in the OpenAI format.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionChunk is a single SSE chunk for streaming responses.
type ChatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []ChatChunkChoice `json:"choices"`
}

// ChatChunkChoice represents a single choice in a streaming chunk.
type ChatChunkChoice struct {
	Index        int       `json:"index"`
	Delta        ChatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

// ChatDelta carries incremental content in a streaming chunk.
type ChatDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ErrorResponse is the OpenAI-compatible error envelope.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes the error inside an ErrorResponse.
type ErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Code    string  `json:"code"`
}

// ModelObject represents a single model in the /v1/models response.
type ModelObject struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Description string `json:"description,omitempty"`
}

// ModelList is the response body for GET /v1/models.
type ModelList struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// SkillObject represents a loaded skill in the /v1/skills response.
type SkillObject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillList is the response body for GET /v1/skills.
type SkillList struct {
	Skills []SkillObject `json:"skills"`
}

// ConvertMessages converts OpenAI-format messages to internal
// llm.Message values. System messages are extracted and merged with
// the server prompt (server prompt takes priority; client system
// messages are appended after it separated by "\n\n"). The merged
// system prompt and the non-system messages are returned.
func ConvertMessages(msgs []ChatMessage, serverPrompt string) ([]llm.Message, string) {
	var clientSystem []string
	var converted []llm.Message

	for _, m := range msgs {
		if m.Role == "system" {
			clientSystem = append(clientSystem, m.Content)
			continue
		}
		converted = append(converted, llm.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	if len(clientSystem) == 0 {
		return converted, serverPrompt
	}

	var sb strings.Builder
	sb.WriteString(serverPrompt)
	for _, cs := range clientSystem {
		sb.WriteString("\n\n")
		sb.WriteString(cs)
	}

	return converted, sb.String()
}

// BuildResponse creates a ChatCompletionResponse with the given
// parameters, setting standard fields like Object and Created.
func BuildResponse(id, model, content string, usage llm.Usage) ChatCompletionResponse {
	return ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: content,
				},
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

// GenerateID creates a unique ID in the format "chatcmpl-<hex>".
func GenerateID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("chatcmpl-%x", b)
}
