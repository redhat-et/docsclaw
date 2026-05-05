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

	// Content chunks: split by lines, then by words within each line.
	// This preserves newlines so markdown renders correctly.
	if content != "" {
		lines := strings.SplitAfter(content, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			words := strings.Fields(line)
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
			// Emit the trailing newline if the line had one
			if strings.HasSuffix(line, "\n") {
				writeChunk(w, ChatCompletionChunk{
					ID:      id,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   model,
					Choices: []ChatChunkChoice{
						{Index: 0, Delta: ChatDelta{Content: "\n"}},
					},
				})
				flusher.Flush()
			}
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

// noopFlusher satisfies http.Flusher for non-streaming writers.
type noopFlusher struct{}

func (noopFlusher) Flush() {}
