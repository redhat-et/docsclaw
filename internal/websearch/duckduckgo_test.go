package websearch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDuckDuckGoProvider_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.FormValue("q"); got != "golang" {
			t.Errorf("expected query golang, got %q", got)
		}

		ua := r.UserAgent()
		if !strings.Contains(ua, "DocsClaw") {
			t.Errorf("expected User-Agent to contain DocsClaw, got %q", ua)
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, fakeDDGHTML)
	}))
	defer server.Close()

	client := server.Client()
	provider := NewDuckDuckGoProvider(client)
	ddg := provider.(*duckDuckGoProvider)
	ddg.baseURL = server.URL

	results, err := provider.Search(context.Background(), "golang", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	want := []Result{
		{
			Title:   "Go Programming Language",
			Snippet: "The Go programming language is an open source project.",
			URL:     "https://go.dev/",
		},
		{
			Title:   "A Tour of Go",
			Snippet: "Welcome to a tour of the Go programming language.",
			URL:     "https://go.dev/tour/",
		},
	}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("result %d = %+v, want %+v", i, results[i], w)
		}
	}
}

func TestDuckDuckGoProvider_SearchErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewDuckDuckGoProvider(server.Client())
	ddg := provider.(*duckDuckGoProvider)
	ddg.baseURL = server.URL
	_, err := provider.Search(context.Background(), "test", 1)
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
}

func TestDuckDuckGoProvider_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><body><div class="no-results">No results</div></body></html>`)
	}))
	defer server.Close()

	provider := NewDuckDuckGoProvider(server.Client())
	ddg := provider.(*duckDuckGoProvider)
	ddg.baseURL = server.URL
	results, err := provider.Search(context.Background(), "xyznothing", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestDuckDuckGoProvider_DefaultClient(t *testing.T) {
	provider := NewDuckDuckGoProvider(nil)
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestDuckDuckGoProvider_BodySizeCap(t *testing.T) {
	large := strings.Repeat(" ", 2*1024*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>`+large+`</body></html>`)
	}))
	defer server.Close()

	provider := NewDuckDuckGoProvider(server.Client())
	ddg := provider.(*duckDuckGoProvider)
	ddg.baseURL = server.URL
	results, err := provider.Search(context.Background(), "test", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results from oversized body, got %d", len(results))
	}
}

const fakeDDGHTML = `<!DOCTYPE html>
<html>
<head><title>golang at DuckDuckGo</title></head>
<body>
<div class="result results_links results_links_deep web-result">
	<div class="links_main links_deep result__body">
		<h2 class="result__title">
			<a class="result__a" href="https://go.dev/">Go Programming Language</a>
		</h2>
		<a class="result__url" href="https://go.dev/">go.dev</a>
		<div class="result__snippet">The Go programming language is an open source project.</div>
	</div>
</div>
<div class="result results_links results_links_deep web-result">
	<div class="links_main links_deep result__body">
		<h2 class="result__title">
			<a class="result__a" href="https://go.dev/tour/">A Tour of Go</a>
		</h2>
		<a class="result__url" href="https://go.dev/tour/">go.dev/tour</a>
		<div class="result__snippet">Welcome to a tour of the Go programming language.</div>
	</div>
</div>
<div class="result results_links results_links_deep web-result">
	<div class="links_main links_deep result__body">
		<h2 class="result__title">
			<a class="result__a" href="https://pkg.go.dev/">Go Packages</a>
		</h2>
		<a class="result__url" href="https://pkg.go.dev/">pkg.go.dev</a>
		<div class="result__snippet">Find, add, and share Go packages.</div>
	</div>
</div>
</body>
</html>`
