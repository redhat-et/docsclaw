package bridge

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// DynamicCardHandler returns an http.Handler that serves the agent card
// with SupportedInterfaces URLs rewritten to match the incoming request's
// Host header and scheme. This ensures the card always reflects how the
// caller reached the agent, regardless of Route, port-forward, or
// internal service access.
func DynamicCardHandler(card *a2a.AgentCard, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clone the card to avoid mutating the original.
		cardCopy := *card

		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		// Trust X-Forwarded-Proto from reverse proxies (OpenShift Router).
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}

		baseURL := scheme + "://" + r.Host

		// Rewrite all interface URLs to use the caller's base URL.
		rewritten := make([]*a2a.AgentInterface, len(cardCopy.SupportedInterfaces))
		for i, iface := range cardCopy.SupportedInterfaces {
			copied := *iface
			copied.URL = baseURL
			rewritten[i] = &copied
		}
		// If no interfaces were declared, add a default JSONRPC one.
		if len(rewritten) == 0 {
			rewritten = []*a2a.AgentInterface{
				a2a.NewAgentInterface(baseURL, a2a.TransportProtocolJSONRPC),
			}
		}
		cardCopy.SupportedInterfaces = rewritten

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(&cardCopy); err != nil && log != nil {
			log.Error("Failed to encode agent card", "error", err)
		}
	})
}
