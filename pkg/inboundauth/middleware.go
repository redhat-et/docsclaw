package inboundauth

import (
	"net/http"
	"path"
	"strings"
)

// Middleware verifies the bearer token on every request except the exempt
// paths. It fails closed and never calls next with an unverified request.
func Middleware(cfg Config, next http.Handler) (http.Handler, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ks := newKeySource(cfg)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isExempt(r.URL.Path, cfg.ExemptPaths) {
			// Exempt endpoints skip verification, but the raw token must
			// still not reach the handler.
			r.Header.Del("Authorization")
			next.ServeHTTP(w, r)
			return
		}

		values := r.Header.Values("Authorization")
		switch {
		case len(values) == 0:
			errMissingToken.write(w)
			return
		case len(values) > 1:
			errMalformedAuthorization.write(w)
			return
		}

		token, ok := parseBearer(values[0])
		if !ok {
			errMalformedAuthorization.write(w)
			return
		}

		claims, authErr := verifyToken(r.Context(), cfg, ks, token)
		if authErr != nil {
			authErr.write(w)
			return
		}

		// The raw token must never reach the next handler, the LLM, tools,
		// logs, or the session store.
		r.Header.Del("Authorization")
		next.ServeHTTP(w, r.WithContext(ContextWithClaims(r.Context(), claims)))
	}), nil
}

// isExempt compares the cleaned request path against each configured entry
// verbatim, so an entry "/health/" does not match the path "/health".
func isExempt(requestPath string, exempt []string) bool {
	clean := path.Clean(requestPath)
	for _, entry := range exempt {
		if entry == clean {
			return true
		}
	}
	return false
}

func parseBearer(value string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(value), " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}
