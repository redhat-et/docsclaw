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
	defer func() { _ = result.Body.Close() }()

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

func TestStreamResponsePreservesNewlines(t *testing.T) {
	w := httptest.NewRecorder()
	content := "## Title\n\nFirst paragraph.\n\n- item one\n- item two\n"

	StreamResponse(w, "test-id", "test-model", content)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	scanner := bufio.NewScanner(result.Body)
	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		sb.WriteString(chunk.Choices[0].Delta.Content)
	}

	if got := sb.String(); got != content {
		t.Fatalf("newlines not preserved:\nexpected: %q\ngot:      %q", content, got)
	}
}

func TestStreamResponseEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	StreamResponse(w, "test-id", "test-model", "")

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

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
