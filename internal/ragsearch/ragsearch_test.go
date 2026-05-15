package ragsearch

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/rag"
)

type mockRAGClient struct {
	chunks    []rag.Chunk
	err       error
	lastLimit int
}

func (m *mockRAGClient) Search(_ context.Context, _ string, limit int) ([]rag.Chunk, error) {
	m.lastLimit = limit
	if m.err != nil {
		return nil, m.err
	}
	if limit < len(m.chunks) {
		return m.chunks[:limit], nil
	}
	return m.chunks, nil
}

func TestRAGSearchToolName(t *testing.T) {
	tool := NewRAGSearchTool(&mockRAGClient{}, &rag.Config{DefaultLimit: 5, MaxLimit: 20})
	if tool.Name() != "rag_search" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "rag_search")
	}
}

func TestRAGSearchToolExecute(t *testing.T) {
	client := &mockRAGClient{
		chunks: []rag.Chunk{
			{ID: "1", Text: "DocsClaw is a lightweight agent runtime.", Score: 0.91},
			{ID: "2", Text: "A2A defines agent communication.", Score: 0.87},
		},
	}
	tool := NewRAGSearchTool(client, &rag.Config{DefaultLimit: 5, MaxLimit: 20})

	result := tool.Execute(context.Background(), map[string]any{
		"query": "agent runtime",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "[1]") {
		t.Error("output missing chunk numbering")
	}
	if !strings.Contains(result.Output, "0.91") {
		t.Error("output missing score")
	}
	if !strings.Contains(result.Output, "DocsClaw is a lightweight agent runtime.") {
		t.Error("output missing chunk text")
	}
}

func TestRAGSearchToolMissingQuery(t *testing.T) {
	tool := NewRAGSearchTool(&mockRAGClient{}, &rag.Config{DefaultLimit: 5, MaxLimit: 20})

	result := tool.Execute(context.Background(), map[string]any{})
	if !result.Error {
		t.Fatal("expected error for missing query")
	}
}

func TestRAGSearchToolDefaultLimit(t *testing.T) {
	client := &mockRAGClient{
		chunks: make([]rag.Chunk, 10),
	}
	tool := NewRAGSearchTool(client, &rag.Config{DefaultLimit: 3, MaxLimit: 20})

	result := tool.Execute(context.Background(), map[string]any{
		"query": "test",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
}

func TestRAGSearchToolMaxLimit(t *testing.T) {
	client := &mockRAGClient{
		chunks: make([]rag.Chunk, 100),
	}
	tool := NewRAGSearchTool(client, &rag.Config{DefaultLimit: 5, MaxLimit: 10})

	result := tool.Execute(context.Background(), map[string]any{
		"query": "test",
		"limit": float64(50),
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if client.lastLimit != 10 {
		t.Errorf("limit passed to client = %d, want 10 (capped from 50)", client.lastLimit)
	}
}

func TestRAGSearchToolClientError(t *testing.T) {
	client := &mockRAGClient{err: fmt.Errorf("connection refused")}
	tool := NewRAGSearchTool(client, &rag.Config{DefaultLimit: 5, MaxLimit: 20})

	result := tool.Execute(context.Background(), map[string]any{
		"query": "test",
	})
	if !result.Error {
		t.Fatal("expected error when client fails")
	}
	if !strings.Contains(result.Output, "connection refused") {
		t.Errorf("error = %q, want to contain 'connection refused'", result.Output)
	}
}

func TestRAGSearchToolNoResults(t *testing.T) {
	client := &mockRAGClient{chunks: []rag.Chunk{}}
	tool := NewRAGSearchTool(client, &rag.Config{DefaultLimit: 5, MaxLimit: 20})

	result := tool.Execute(context.Background(), map[string]any{
		"query": "nonexistent topic",
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No results") {
		t.Errorf("expected 'No results' message, got %q", result.Output)
	}
}
