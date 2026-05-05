package openaiapi

import (
	"encoding/json"
	"fmt"
	"net/http"
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

	// Content chunks: tokenize into whitespace and non-whitespace runs
	// to preserve exact formatting (indentation, multiple spaces, newlines).
	if content != "" {
		for _, token := range tokenize(content) {
			writeChunk(w, ChatCompletionChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []ChatChunkChoice{
					{Index: 0, Delta: ChatDelta{Content: token}},
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

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
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
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeChunk(w http.ResponseWriter, chunk ChatCompletionChunk) {
	data, _ := json.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

// tokenize splits content into alternating non-whitespace and
// whitespace runs, preserving exact formatting.
func tokenize(s string) []string {
	var tokens []string
	i := 0
	for i < len(s) {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			tokens = append(tokens, s[i:j])
			i = j
		} else {
			j := i
			for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' && s[j] != '\r' {
				j++
			}
			tokens = append(tokens, s[i:j])
			i = j
		}
	}
	return tokens
}

// noopFlusher satisfies http.Flusher for non-streaming writers.
type noopFlusher struct{}

func (noopFlusher) Flush() {}
