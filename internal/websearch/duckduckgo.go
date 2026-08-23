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
	// resultStartRe locates the opening tag of a DuckDuckGo result block.
	// Each result block is then extracted by balancing nested <div> tags so
	// that title, snippet, and URL are kept aligned per result even when a
	// field is missing from one result.
	resultStartRe = regexp.MustCompile(`<div[^>]*class="result[^"]*"`)
	titleRe       = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*>(.*?)</a>`)
	snippetRe     = regexp.MustCompile(`<div[^>]*class="result__snippet"[^>]*>(.*?)</div>`)
	hrefRe        = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]*)"`)
	stripHTMLRe   = regexp.MustCompile("<[^>]*>")
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
	results := make([]Result, 0, limit)
	starts := resultStartRe.FindAllStringIndex(htmlBody, -1)
	for _, start := range starts {
		if len(results) >= limit {
			break
		}
		block := extractResultBlock(htmlBody, start[0])
		titleMatch := titleRe.FindStringSubmatch(block)
		snippetMatch := snippetRe.FindStringSubmatch(block)
		hrefMatch := hrefRe.FindStringSubmatch(block)
		if titleMatch == nil || snippetMatch == nil || hrefMatch == nil {
			continue
		}
		results = append(results, Result{
			Title:   stripHTML(titleMatch[1]),
			Snippet: stripHTML(snippetMatch[1]),
			URL:     resolveDDGURL(hrefMatch[1]),
		})
	}
	return results
}

// extractResultBlock returns the <div class="result..."> block starting at
// start, balancing nested <div> tags. If no balanced closing tag is found, it
// returns the remainder of htmlBody.
func extractResultBlock(htmlBody string, start int) string {
	if start >= len(htmlBody) {
		return ""
	}
	tagEnd := strings.Index(htmlBody[start:], ">")
	if tagEnd == -1 {
		return htmlBody[start:]
	}
	tagEnd += start + 1

	depth := 1
	pos := tagEnd
	for pos < len(htmlBody) && depth > 0 {
		if strings.HasPrefix(htmlBody[pos:], "<div") {
			depth++
			pos += 4
		} else if strings.HasPrefix(htmlBody[pos:], "</div>") {
			depth--
			if depth == 0 {
				return htmlBody[start : pos+6]
			}
			pos += 6
		} else {
			pos++
		}
	}
	return htmlBody[start:]
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
