package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWeaviateSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/graphql" {
			t.Fatalf("expected /v1/graphql, got %s", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["query"] == "" {
			t.Fatal("expected non-empty query field")
		}

		resp := map[string]any{
			"data": map[string]any{
				"Get": map[string]any{
					"Docs": []any{
						map[string]any{
							"content": "DocsClaw is a lightweight agent runtime.",
							"_additional": map[string]any{
								"id":       "abc-123",
								"distance": 0.1,
							},
						},
						map[string]any{
							"content": "A2A defines agent communication.",
							"_additional": map[string]any{
								"id":       "def-456",
								"distance": 0.2,
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := NewWeaviateClient(srv.URL, "Docs", "content")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	chunks, err := client.Search(context.Background(), "agent runtime", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].ID != "abc-123" {
		t.Errorf("chunk[0].ID = %q, want %q", chunks[0].ID, "abc-123")
	}
	if chunks[0].Text != "DocsClaw is a lightweight agent runtime." {
		t.Errorf("chunk[0].Text = %q", chunks[0].Text)
	}
	if chunks[0].Score != 0.9 {
		t.Errorf("chunk[0].Score = %v, want 0.9", chunks[0].Score)
	}
}

func TestWeaviateSearchGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data":   nil,
			"errors": []map[string]string{{"message": "class Docs not found"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, _ := NewWeaviateClient(srv.URL, "Docs", "content")
	_, err := client.Search(context.Background(), "test", 5)
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
	if !strings.Contains(err.Error(), "class Docs not found") {
		t.Errorf("error = %q, want to contain 'class Docs not found'", err)
	}
}

func TestWeaviateSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, _ := NewWeaviateClient(srv.URL, "Docs", "content")
	_, err := client.Search(context.Background(), "test", 5)
	if err == nil {
		t.Fatal("expected error for HTTP 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error = %q, want to contain '503'", err)
	}
}

func TestWeaviateSearchEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"Get": map[string]any{
					"Docs": []any{},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, _ := NewWeaviateClient(srv.URL, "Docs", "content")
	chunks, err := client.Search(context.Background(), "nonexistent", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestNewWeaviateClientValidation(t *testing.T) {
	_, err := NewWeaviateClient("", "Docs", "content")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}

	_, err = NewWeaviateClient("http://localhost:8080", "", "content")
	if err == nil {
		t.Fatal("expected error for empty collection")
	}
}

func TestQuoteGraphQL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple query", `"simple query"`},
		{`query with "quotes"`, `"query with \"quotes\""`},
		{`back\slash`, `"back\\slash"`},
		{"newline\nin query", `"newline\nin query"`},
		{"tab\there", `"tab\there"`},
	}
	for _, tt := range tests {
		got := quoteGraphQL(tt.input)
		if got != tt.want {
			t.Errorf("quoteGraphQL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
