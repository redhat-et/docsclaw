package ragsearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/redhat-et/docsclaw/pkg/rag"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type ragSearchTool struct {
	client rag.Client
	config *rag.Config
}

func NewRAGSearchTool(client rag.Client, config *rag.Config) tools.Tool {
	return &ragSearchTool{client: client, config: config}
}

func (t *ragSearchTool) Name() string { return "rag_search" }

func (t *ragSearchTool) Description() string {
	return "Search the document store for chunks semantically related to the query. " +
		"Use this when the user's question requires information from indexed documents. " +
		"Returns the top-k most relevant text chunks."
}

func (t *ragSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query text",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Number of chunks to return (default: %d, max: %d)", t.config.DefaultLimit, t.config.MaxLimit),
			},
		},
		"required": []string{"query"},
	}
}

func (t *ragSearchTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return tools.Errorf("query is required")
	}

	limit := t.config.DefaultLimit
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	if limit > t.config.MaxLimit {
		limit = t.config.MaxLimit
	}

	chunks, err := t.client.Search(ctx, query, limit)
	if err != nil {
		return tools.Errorf("rag search failed: %s", err)
	}

	if len(chunks) == 0 {
		return tools.OK("No results found for the query.")
	}

	var sb strings.Builder
	for i, chunk := range chunks {
		fmt.Fprintf(&sb, "[%d] (score: %.2f)\n%s\n\n", i+1, chunk.Score, chunk.Text)
	}
	return tools.OK(strings.TrimRight(sb.String(), "\n"))
}
