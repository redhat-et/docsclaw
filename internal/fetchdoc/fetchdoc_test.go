package fetchdoc

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchDocument(t *testing.T) {
	const docBody = `{"document":{"title":"Contract V001","content":"rates and terms"}}`

	tests := []struct {
		name       string
		args       map[string]any
		handler    http.HandlerFunc
		wantErr    bool
		wantOutput string
		wantReqs   int32
	}{
		{
			name: "ok",
			args: map[string]any{"service": "finance", "document_id": "DOC-001"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(docBody))
			},
			wantOutput: "**Document:** Contract V001\n\nrates and terms",
			wantReqs:   1,
		},
		{
			name:       "unknown service makes no request",
			args:       map[string]any{"service": "hr", "document_id": "DOC-001"},
			wantErr:    true,
			wantOutput: `unknown document service "hr"; available: finance`,
			wantReqs:   0,
		},
		{
			name:       "missing service",
			args:       map[string]any{"document_id": "DOC-001"},
			wantErr:    true,
			wantOutput: "service is required",
			wantReqs:   0,
		},
		{
			name:       "missing document id",
			args:       map[string]any{"service": "finance"},
			wantErr:    true,
			wantOutput: "document_id is required",
			wantReqs:   0,
		},
		{
			name: "application denial is preserved",
			args: map[string]any{"service": "finance", "document_id": "DOC-001"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"invalid_audience"}`))
			},
			wantErr:    true,
			wantOutput: "DOC-001: 403 invalid_audience",
			wantReqs:   1,
		},
		{
			name: "sandbox policy denial is reported",
			args: map[string]any{"service": "finance", "document_id": "DOC-001"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"policy_denied","detail":"not permitted"}`))
			},
			wantErr:    true,
			wantOutput: "DOC-001: blocked by sandbox network policy",
			wantReqs:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			handler := tt.handler
			if handler == nil {
				handler = func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(docBody))
				}
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				handler(w, r)
			}))
			defer srv.Close()

			tool := New(
				map[string]string{"finance": srv.URL},
				srv.Client(),
				slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
			)

			res := tool.Execute(context.Background(), tt.args)
			if res.Error != tt.wantErr {
				t.Fatalf("Error = %v, want %v (output %q)", res.Error, tt.wantErr, res.Output)
			}
			if !strings.Contains(res.Output, tt.wantOutput) {
				t.Fatalf("Output = %q, want it to contain %q", res.Output, tt.wantOutput)
			}
			if got := requests.Load(); got != tt.wantReqs {
				t.Fatalf("requests = %d, want %d", got, tt.wantReqs)
			}
		})
	}
}

func TestFetchDocument_PathEscaping(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"document":{"title":"t","content":"c"}}`))
	}))
	defer srv.Close()

	tool := New(
		map[string]string{"finance": srv.URL + "/"},
		srv.Client(),
		slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
	)

	res := tool.Execute(context.Background(), map[string]any{
		"service":     "finance",
		"document_id": "../x?y",
	})
	if res.Error {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if gotPath != "/documents/..%2Fx%3Fy" {
		t.Fatalf("path = %q, want %q", gotPath, "/documents/..%2Fx%3Fy")
	}
}

func TestFetchDocument_TrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"document":{"title":"t","content":"c"}}`))
	}))
	defer srv.Close()

	tool := New(
		map[string]string{"finance": srv.URL + "/"},
		srv.Client(),
		slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
	)

	res := tool.Execute(context.Background(), map[string]any{
		"service":     "finance",
		"document_id": "DOC-001",
	})
	if res.Error {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if gotPath != "/documents/DOC-001" {
		t.Fatalf("path = %q, want %q", gotPath, "/documents/DOC-001")
	}
}

type leakingTransport struct {
	base http.RoundTripper
}

func (l leakingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer leak")
	return l.base.RoundTrip(clone)
}

func TestFetchDocument_LogsWithoutHeaders(t *testing.T) {
	var logs strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"document":{"title":"t","content":"c"}}`))
	}))
	defer srv.Close()

	tool := New(
		map[string]string{"finance": srv.URL},
		&http.Client{Transport: leakingTransport{base: http.DefaultTransport}},
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	res := tool.Execute(context.Background(), map[string]any{
		"service":     "finance",
		"document_id": "DOC-001",
	})
	if res.Error {
		t.Fatalf("unexpected error: %s", res.Output)
	}

	out := logs.String()
	for _, want := range []string{"host=", "status=200"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log %q does not contain %q", out, want)
		}
	}
	for _, forbidden := range []string{"leak", "Authorization"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("log %q contains %q", out, forbidden)
		}
	}
}
