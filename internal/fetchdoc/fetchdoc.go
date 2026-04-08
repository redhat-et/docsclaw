package fetchdoc

import (
	"context"
	"fmt"

	"github.com/redhat-et/docsclaw/pkg/tools"
)

// DocumentFetcher is a function that fetches a document by ID.
type DocumentFetcher func(ctx context.Context, documentID, bearerToken string) (map[string]any, error)

type fetchDocTool struct {
	fetcher DocumentFetcher
}

// NewFetchDocTool creates a new fetch document tool with the given fetcher.
func NewFetchDocTool(fetcher DocumentFetcher) tools.Tool {
	return &fetchDocTool{fetcher: fetcher}
}

func (t *fetchDocTool) Name() string { return "fetch_document" }
func (t *fetchDocTool) Description() string {
	return "Fetch a document from the document service by ID (e.g., DOC-001). " +
		"Returns the document title and content. " +
		"Subject to the same delegation and OPA authorization as the initial request."
}
func (t *fetchDocTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"document_id": map[string]any{
				"type":        "string",
				"description": "Document ID (e.g., DOC-001)",
			},
		},
		"required": []string{"document_id"},
	}
}

func (t *fetchDocTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	docID, ok := args["document_id"].(string)
	if !ok || docID == "" {
		return tools.Errorf("document_id is required")
	}

	doc, err := t.fetcher(ctx, docID, "")
	if err != nil {
		return tools.Errorf("Failed to fetch document %s: %s", docID, err)
	}

	title, _ := doc["title"].(string)
	content, _ := doc["content"].(string)

	return tools.OK(fmt.Sprintf("**Document:** %s\n\n%s", title, content))
}
