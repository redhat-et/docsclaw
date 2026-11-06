package inboundauth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// authError is a denial with a stable code that demo scripts can assert on.
type authError struct {
	status int
	code   string
	desc   string
	scopes []string
}

func (e *authError) write(w http.ResponseWriter) {
	switch {
	case e.status == http.StatusUnauthorized:
		w.Header().Set("WWW-Authenticate", `Bearer realm="docsclaw", error="`+e.code+`"`)
	case e.code == "insufficient_scope":
		w.Header().Set("WWW-Authenticate", `Bearer realm="docsclaw", error="insufficient_scope", scope="`+
			strings.Join(e.scopes, " ")+`"`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.status)
	_ = json.NewEncoder(w).Encode(struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}{e.code, e.desc})
}

var (
	errMissingToken           = &authError{http.StatusUnauthorized, "missing_token", "an Authorization header is required", nil}
	errMalformedAuthorization = &authError{http.StatusUnauthorized, "malformed_authorization", "Authorization must be exactly one Bearer token", nil}
	errUnsupportedAlgorithm   = &authError{http.StatusUnauthorized, "unsupported_algorithm", "token algorithm must be ES256 or RS256", nil}
	errInvalidToken           = &authError{http.StatusUnauthorized, "invalid_token", "token is not valid", nil}
	errTokenExpired           = &authError{http.StatusUnauthorized, "token_expired", "token has expired", nil}
	errTokenNotYetValid       = &authError{http.StatusUnauthorized, "token_not_yet_valid", "token is not valid yet", nil}
	errInvalidIssuer          = &authError{http.StatusUnauthorized, "invalid_issuer", "token issuer is not accepted", nil}
	errInvalidAudience        = &authError{http.StatusForbidden, "invalid_audience", "token audience is not accepted", nil}
	errCallerNotAllowed       = &authError{http.StatusForbidden, "caller_not_allowed", "caller is not allowed", nil}
	errJWKSUnavailable        = &authError{http.StatusServiceUnavailable, "jwks_unavailable", "issuer keys are unavailable", nil}
)
