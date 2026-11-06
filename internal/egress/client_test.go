package egress

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// http.ProxyFromEnvironment caches the environment on first use, so the
	// proxy variable must be set before any test can initialize the cache.
	// This test binary makes no other outbound requests.
	for _, key := range []string{
		"HTTP_PROXY", "http_proxy",
		"NO_PROXY", "no_proxy",
		"ALL_PROXY", "all_proxy",
	} {
		_ = os.Unsetenv(key)
	}
	_ = os.Setenv("HTTPS_PROXY", "http://proxy.test:3128")
	os.Exit(m.Run())
}

func TestNewClient_ProxyFromEnvironment(t *testing.T) {
	tr, ok := NewClient(time.Second).Transport.(*http.Transport)
	if !ok {
		t.Fatal("NewClient transport is not *http.Transport")
	}
	if tr.Proxy == nil {
		t.Fatal("transport has a nil Proxy function; operator upstream proxies would be bypassed")
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/doc", nil)
	proxyURL, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}
	if proxyURL == nil {
		t.Fatal("Proxy returned nil; HTTPS_PROXY was not honored")
	}
	if got, want := proxyURL.String(), "http://proxy.test:3128"; got != want {
		t.Fatalf("proxy = %q, want %q", got, want)
	}
}

func TestNewClient_NoCustomDialer(t *testing.T) {
	tr := NewClient(0).Transport.(*http.Transport)
	if tr.DialContext == nil {
		t.Fatal("DialContext is nil; the stock dialer is required")
	}
	if tr.DialTLSContext != nil {
		t.Fatal("DialTLSContext is set; custom TLS dialing bypasses the supervisor")
	}
}

func TestNewClient_Timeout(t *testing.T) {
	if got := NewClient(5 * time.Second).Timeout; got != 5*time.Second {
		t.Fatalf("Timeout = %v, want 5s", got)
	}
	if got := NewClient(0).Timeout; got != 0 {
		t.Fatalf("Timeout = %v, want 0 (no timeout)", got)
	}
}
