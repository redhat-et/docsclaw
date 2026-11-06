package egress

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func newTestLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, nil))
}

func TestDescribeResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "json error is preserved",
			status: http.StatusForbidden,
			body:   `{"error":"invalid_audience"}`,
			want:   "403 invalid_audience",
		},
		{
			name:   "department permission error is preserved",
			status: http.StatusForbidden,
			body:   `{"error":"insufficient_department_permissions"}`,
			want:   "403 insufficient_department_permissions",
		},
		{
			name:   "empty json falls back to status text",
			status: http.StatusNotFound,
			body:   `{}`,
			want:   "404 Not Found",
		},
		{
			name:   "html body falls back to status text",
			status: http.StatusBadGateway,
			body:   `<html><body>bad gateway</body></html>`,
			want:   "502 Bad Gateway",
		},
		{
			name:   "oversized body is truncated and does not hang",
			status: http.StatusForbidden,
			body:   strings.Repeat("x", 10*1024),
			want:   "403 Forbidden",
		},
		{
			name:   "sandbox policy denial is reported as blocked",
			status: http.StatusForbidden,
			body:   `{"error":"policy_denied","detail":"CONNECT example.com:443 not permitted by policy"}`,
			want:   "blocked by sandbox network policy",
		},
		{
			name:   "ssrf denial is reported as blocked",
			status: http.StatusForbidden,
			body:   `{"error":"ssrf_denied"}`,
			want:   "blocked by sandbox network policy",
		},
		{
			name:   "token grant failure is preserved",
			status: http.StatusBadGateway,
			body:   `{"error":"token_grant_failed"}`,
			want:   "502 token_grant_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			if got := DescribeResponse(resp); got != tt.want {
				t.Fatalf("DescribeResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDescribeError(t *testing.T) {
	proxyErr := &url.Error{
		Op:  "Get",
		URL: "https://example.com",
		Err: &net.OpError{Op: "proxyconnect", Net: "tcp", Err: errors.New("connection refused")},
	}
	dnsErr := &url.Error{
		Op:  "Get",
		URL: "http://missing.invalid",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "missing.invalid"}},
	}
	tlsErr := &url.Error{
		Op:  "Get",
		URL: "https://denied.invalid",
		Err: errors.New("http: server gave HTTP response to HTTPS client"),
	}
	refusedErr := &url.Error{
		Op:  "Get",
		URL: "http://document-service:8080",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")},
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "proxyconnect is a sandbox policy denial",
			err:  proxyErr,
			want: "blocked by sandbox network policy",
		},
		{
			name: "https plaintext denial is a sandbox policy denial",
			err:  tlsErr,
			want: "blocked by sandbox network policy",
		},
		{
			name: "dns failure is a request failure",
			err:  dnsErr,
			want: "request failed: " + dnsErr.Error(),
		},
		{
			name: "deadline is a request failure",
			err:  context.DeadlineExceeded,
			want: "request failed: context deadline exceeded",
		},
		{
			name: "connection refused is not a policy denial",
			err:  refusedErr,
			want: "request failed: " + refusedErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DescribeError(tt.err); got != tt.want {
				t.Fatalf("DescribeError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogCallHasNoHeaders(t *testing.T) {
	var buf strings.Builder
	log := newTestLogger(&buf)

	LogCall(log, "fetch_document", "document-service:8080", http.StatusOK, "ok")

	out := buf.String()
	for _, want := range []string{"tool=fetch_document", "host=document-service:8080", "status=200", "reason=ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output %q does not contain %q", out, want)
		}
	}
	for _, forbidden := range []string{"Authorization", "Bearer", "leak"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("log output %q contains %q", out, forbidden)
		}
	}
}
