package egress

import (
	"net/http"
	"time"
)

// NewTransport returns a clone of http.DefaultTransport. The clone keeps the
// stock dialer and Proxy: http.ProxyFromEnvironment: OpenShell mediates egress
// in the network namespace, but an operator-configured upstream proxy is still
// honored.
func NewTransport() *http.Transport {
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		return tr.Clone()
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment}
}

// NewClient returns an HTTP client backed by NewTransport. A zero timeout
// means no client-level timeout.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewTransport(),
		Timeout:   timeout,
	}
}
