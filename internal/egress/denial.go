package egress

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// maxErrorBody bounds how much of an error response is read before parsing.
// Supervisor and document-service errors are small; a hostile or misbehaving
// upstream must not make the agent read an unbounded body.
const maxErrorBody = 4 * 1024

// sandboxPolicyErrors are the supervisor error codes that mean "OpenShell
// network policy refused this request". They come from the sandbox proxy, not
// from the application being called, and are reported as a policy block rather
// than as an HTTP status.
var sandboxPolicyErrors = map[string]bool{
	"policy_denied":                true,
	"ssrf_denied":                  true,
	"middleware_denied":            true,
	"middleware_failed":            true,
	"credential_endpoint_mismatch": true,
	"unsupported_l7_protocol":      true,
}

// blockedByPolicy is the message the LLM sees when the sandbox refused egress.
const blockedByPolicy = "blocked by sandbox network policy"

// DescribeResponse turns a non-2xx response into a short reason for the LLM.
// It reads at most 4 KiB and prefers the JSON "error" field, falling back to
// the HTTP status text. Application errors such as invalid_audience are
// preserved verbatim; supervisor policy codes are reported as a policy block.
func DescribeResponse(resp *http.Response) string {
	if resp == nil {
		return "request failed: empty response"
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		if sandboxPolicyErrors[payload.Error] {
			return blockedByPolicy
		}
		return fmt.Sprintf("%d %s", resp.StatusCode, payload.Error)
	}

	reason := http.StatusText(resp.StatusCode)
	if reason == "" {
		reason = "unknown status"
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, reason)
}

// DescribeError maps a transport error to a reason for the LLM.
//
// Two shapes mean the sandbox policy blocked egress: a "proxyconnect" failure,
// and Go's "server gave HTTP response to HTTPS client", which is what an HTTPS
// target produces because OpenShell writes its plaintext 403 before the TLS
// handshake. Other failures (DNS, timeouts, connection refused) are ordinary
// request failures: inside a sandbox the proxy is the only reachable endpoint,
// so a refused connection is an operational problem, not a policy denial.
func DescribeError(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if strings.Contains(msg, "http: server gave HTTP response to HTTPS client") {
		return blockedByPolicy
	}
	if strings.Contains(strings.ToLower(msg), "proxyconnect") {
		return blockedByPolicy
	}
	return "request failed: " + msg
}

// LogCall records one outbound tool call at info level. It deliberately takes
// only non-sensitive fields: never headers, tokens, or query strings.
func LogCall(log *slog.Logger, tool, host string, status int, reason string) {
	if log == nil {
		return
	}
	log.Info("outbound call",
		"tool", tool,
		"host", host,
		"status", status,
		"reason", reason,
	)
}
