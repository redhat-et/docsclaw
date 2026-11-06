package fetchdoc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/redhat-et/docsclaw/internal/egress"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type fetchDocTool struct {
	services map[string]string
	client   *http.Client
	log      *slog.Logger
}

// New creates the fetch_document tool for a set of named document services.
func New(services map[string]string, client *http.Client, log *slog.Logger) tools.Tool {
	return &fetchDocTool{services: services, client: client, log: log}
}

func (t *fetchDocTool) Name() string { return "fetch_document" }

func (t *fetchDocTool) Description() string {
	return fmt.Sprintf(
		"Fetch a document from a named document service by ID (e.g., DOC-001). "+
			"Returns the document title and content. Available services: %s.",
		strings.Join(t.serviceNames(), ", "),
	)
}

func (t *fetchDocTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service": map[string]any{
				"type":        "string",
				"description": "Document service name",
				"enum":        t.serviceNames(),
			},
			"document_id": map[string]any{
				"type":        "string",
				"description": "Document ID (e.g., DOC-001)",
			},
		},
		"required": []string{"service", "document_id"},
	}
}

func (t *fetchDocTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	service, ok := args["service"].(string)
	if !ok || service == "" {
		return tools.Errorf("service is required")
	}
	docID, ok := args["document_id"].(string)
	if !ok || docID == "" {
		return tools.Errorf("document_id is required")
	}

	base, ok := t.services[service]
	if !ok {
		return tools.Errorf("unknown document service %q; available: %s",
			service, strings.Join(t.serviceNames(), ", "))
	}

	endpoint := strings.TrimRight(base, "/") + "/documents/" + url.PathEscape(docID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tools.Errorf("%s: %s", docID, egress.DescribeError(err))
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		reason := egress.DescribeError(err)
		egress.LogCall(t.log, t.Name(), req.URL.Host, 0, reason)
		return tools.Errorf("%s: %s", docID, reason)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		reason := egress.DescribeResponse(resp)
		egress.LogCall(t.log, t.Name(), req.URL.Host, resp.StatusCode, reason)
		return tools.Errorf("%s: %s", docID, reason)
	}
	egress.LogCall(t.log, t.Name(), req.URL.Host, resp.StatusCode, "ok")

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return tools.Errorf("%s: failed to decode response: %s", docID, err)
	}

	doc := result
	if nested, ok := result["document"].(map[string]any); ok {
		doc = nested
	}
	title, _ := doc["title"].(string)
	content, _ := doc["content"].(string)

	return tools.OK(fmt.Sprintf("**Document:** %s\n\n%s", title, content))
}

func (t *fetchDocTool) serviceNames() []string {
	names := make([]string, 0, len(t.services))
	for name := range t.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
