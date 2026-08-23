package websearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/redhat-et/docsclaw/pkg/tools"
)

const (
	defaultNumResults = 5
	maxNumResults     = 10
	minNumResults     = 1
)

type webSearchTool struct {
	provider Provider
}

// NewWebSearchTool creates a web_search tool backed by the given provider.
func NewWebSearchTool(provider Provider) tools.Tool {
	return &webSearchTool{provider: provider}
}

func (t *webSearchTool) Name() string { return "web_search" }
func (t *webSearchTool) Description() string {
	return "Search the web and return a list of relevant results."
}

func (t *webSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default 5, max 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *webSearchTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return tools.Errorf("query is required")
	}

	numResults := defaultNumResults
	if v, ok := args["num_results"]; ok {
		switch n := v.(type) {
		case int:
			numResults = n
		case int64:
			numResults = int(n)
		case float64:
			numResults = int(n)
		default:
			return tools.Errorf("num_results must be an integer")
		}
	}
	if numResults > maxNumResults {
		numResults = maxNumResults
	}
	if numResults < minNumResults {
		numResults = minNumResults
	}

	results, err := t.provider.Search(ctx, query, numResults)
	if err != nil {
		return tools.Errorf("web search failed: %s", err)
	}
	if len(results) == 0 {
		return tools.OK("No results found.")
	}

	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "* [%s](%s): %s\n", r.Title, r.URL, r.Snippet)
	}
	return tools.OK(strings.TrimSpace(b.String()))
}
