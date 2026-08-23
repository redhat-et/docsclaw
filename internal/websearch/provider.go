package websearch

import "context"

// Provider defines the interface for web search backends.
type Provider interface {
	Search(ctx context.Context, query string, numResults int) ([]Result, error)
}

// Result represents a single search result.
type Result struct {
	Title   string
	Snippet string
	URL     string
}
