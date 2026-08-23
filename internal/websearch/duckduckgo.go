package websearch

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DuckDuckGoBaseURL is the HTML endpoint used by the DuckDuckGo provider.
const DuckDuckGoBaseURL = "https://html.duckduckgo.com/html/"

const maxResponseBytes = 1 << 20 // 1 MiB

var (
	titleRe     = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*>(.*?)</a>`)
	snippetRe   = regexp.MustCompile(`<div[^>]*class="result__snippet"[^>]*>(.*?)</div>`)
	hrefRe      = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]*)"`)
	stripHTMLRe = regexp.MustCompile("<[^>]*>")
)

type duckDuckGoProvider struct {
	client  *http.Client
	baseURL string
}

// NewDuckDuckGoProvider creates a DuckDuckGo search provider.
func NewDuckDuckGoProvider(client *http.Client) Provider {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &duckDuckGoProvider{
		client:  client,
		baseURL: DuckDuckGoBaseURL,
	}
}

// Search queries DuckDuckGo and returns parsed results.
func (p *duckDuckGoProvider) Search(ctx context.Context, query string, numResults int) ([]Result, error) {
	if numResults <= 0 {
		numResults = 1
	}

	form := url.Values{}
	form.Set("q", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "DocsClaw web_search tool (https://github.com/redhat-et/docsclaw)")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseDDGResults(string(body), numResults), nil
}

// parseDDGResults scrapes DuckDuckGo HTML with regexes. This is inherently
// fragile and tightly coupled to DuckDuckGo's current markup. The Provider
// interface allows swapping to a more robust backend (e.g., Brave or Tavily)
// without changing the tool implementation.
func parseDDGResults(htmlBody string, limit int) []Result {
	titles := titleRe.FindAllStringSubmatch(htmlBody, -1)
	snippets := snippetRe.FindAllStringSubmatch(htmlBody, -1)
	hrefs := hrefRe.FindAllStringSubmatch(htmlBody, -1)

	count := min(len(titles), len(snippets), len(hrefs), limit)
	results := make([]Result, 0, count)
	for i := 0; i < count; i++ {
		results = append(results, Result{
			Title:   stripHTML(titles[i][1]),
			Snippet: stripHTML(snippets[i][1]),
			URL:     resolveDDGURL(hrefs[i][1]),
		})
	}
	return results
}

func stripHTML(s string) string {
	s = html.UnescapeString(s)
	s = stripHTMLRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func resolveDDGURL(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if (u.Host == "duckduckgo.com" || u.Host == "html.duckduckgo.com") && u.Path == "/l/" {
		if uddg := u.Query().Get("uddg"); uddg != "" {
			return uddg
		}
	}
	return href
}
