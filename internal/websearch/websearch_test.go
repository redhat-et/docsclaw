package websearch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeProvider struct {
	query      string
	numResults int
	results    []Result
	err        error
}

func (f *fakeProvider) Search(_ context.Context, query string, numResults int) ([]Result, error) {
	f.query = query
	f.numResults = numResults
	if f.err != nil {
		return nil, f.err
	}
	if numResults > len(f.results) {
		return f.results, nil
	}
	return f.results[:numResults], nil
}

func TestWebSearchTool_NameDescriptionParameters(t *testing.T) {
	tool := NewWebSearchTool(&fakeProvider{})
	if tool.Name() != "web_search" {
		t.Errorf("name = %q, want web_search", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["query"]; !ok {
		t.Error("expected query property")
	}
	if _, ok := props["num_results"]; !ok {
		t.Error("expected num_results property")
	}
	required, ok := params["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "query" {
		t.Errorf("expected required [query], got %v", params["required"])
	}
}

func TestWebSearchTool_RequiresQuery(t *testing.T) {
	tool := NewWebSearchTool(&fakeProvider{})
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.Error {
		t.Fatal("expected error result for missing query")
	}
	if !strings.Contains(result.Output, "query is required") {
		t.Errorf("expected 'query is required', got %q", result.Output)
	}
}

func TestWebSearchTool_DefaultNumResults(t *testing.T) {
	fake := &fakeProvider{results: makeResults(10)}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{"query": "go"})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if fake.numResults != 5 {
		t.Errorf("expected default num_results 5, got %d", fake.numResults)
	}
	if strings.Count(result.Output, "* [") != 5 {
		t.Errorf("expected 5 markdown list items, got:\n%s", result.Output)
	}
}

func TestWebSearchTool_ExplicitNumResults(t *testing.T) {
	fake := &fakeProvider{results: makeResults(10)}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{
		"query":       "go",
		"num_results": 3,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if fake.numResults != 3 {
		t.Errorf("expected num_results 3, got %d", fake.numResults)
	}
	if strings.Count(result.Output, "* [") != 3 {
		t.Errorf("expected 3 markdown list items, got:\n%s", result.Output)
	}
}

func TestWebSearchTool_NumResultsFloat64(t *testing.T) {
	fake := &fakeProvider{results: makeResults(10)}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{
		"query":       "go",
		"num_results": float64(2),
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if fake.numResults != 2 {
		t.Errorf("expected num_results 2, got %d", fake.numResults)
	}
}

func TestWebSearchTool_NumResultsMaxCap(t *testing.T) {
	fake := &fakeProvider{results: makeResults(15)}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{
		"query":       "go",
		"num_results": 20,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if fake.numResults != 10 {
		t.Errorf("expected num_results capped to 10, got %d", fake.numResults)
	}
	if strings.Count(result.Output, "* [") != 10 {
		t.Errorf("expected 10 markdown list items, got:\n%s", result.Output)
	}
}

func TestWebSearchTool_NumResultsMinCap(t *testing.T) {
	fake := &fakeProvider{results: makeResults(10)}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{
		"query":       "go",
		"num_results": -3,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if fake.numResults != 1 {
		t.Errorf("expected num_results clamped to 1, got %d", fake.numResults)
	}
}

func TestWebSearchTool_ProviderError(t *testing.T) {
	fake := &fakeProvider{err: errors.New("search failed")}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{"query": "go"})
	if !result.Error {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.Output, "search failed") {
		t.Errorf("expected error message to contain 'search failed', got %q", result.Output)
	}
}

func TestWebSearchTool_NoResults(t *testing.T) {
	fake := &fakeProvider{results: []Result{}}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{"query": "xyznothing"})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No results") {
		t.Errorf("expected 'No results' message, got %q", result.Output)
	}
}

func TestWebSearchTool_InvalidNumResultsType(t *testing.T) {
	tool := NewWebSearchTool(&fakeProvider{})
	result := tool.Execute(context.Background(), map[string]any{
		"query":       "go",
		"num_results": "ten",
	})
	if !result.Error {
		t.Fatal("expected error result for invalid num_results type")
	}
	if !strings.Contains(result.Output, "num_results must be an integer") {
		t.Errorf("expected 'num_results must be an integer', got %q", result.Output)
	}
}

func TestWebSearchTool_WhitespaceOnlyQuery(t *testing.T) {
	tool := NewWebSearchTool(&fakeProvider{})
	result := tool.Execute(context.Background(), map[string]any{"query": "   "})
	if !result.Error {
		t.Fatal("expected error result for whitespace-only query")
	}
	if !strings.Contains(result.Output, "query is required") {
		t.Errorf("expected 'query is required', got %q", result.Output)
	}
}

func TestWebSearchTool_OutputFormat(t *testing.T) {
	fake := &fakeProvider{results: []Result{{
		Title:   "Example",
		Snippet: "An example site.",
		URL:     "https://example.com/",
	}}}
	tool := NewWebSearchTool(fake)
	result := tool.Execute(context.Background(), map[string]any{
		"query":       "example",
		"num_results": 1,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	want := "* [Example](https://example.com/): An example site."
	if !strings.Contains(result.Output, want) {
		t.Errorf("expected output to contain %q, got:\n%s", want, result.Output)
	}
}

func makeResults(n int) []Result {
	results := make([]Result, n)
	for i := 0; i < n; i++ {
		results[i] = Result{
			Title:   "Result",
			Snippet: "Snippet",
			URL:     "https://example.com/",
		}
	}
	return results
}
