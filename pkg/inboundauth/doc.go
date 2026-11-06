// Package inboundauth verifies incoming JWT bearer tokens against a JWKS and
// enforces the issuer, audience, scope, and caller rules an agent needs when it
// is exposed through OpenShell's bearer-passthrough service.
//
// # Threat model
//
// OpenShell's `--authorization-mode bearer-passthrough` forwards one bearer
// token unchanged, but requests without a token also pass through. The
// application cannot rely on the network path to authenticate callers, so this
// middleware authenticates every request itself. It rejects unsigned and
// HMAC-signed tokens (including `alg: none` and the RSA-public-key-as-HMAC-key
// confusion attack), and it fails closed when the issuer's JWKS cannot be
// fetched.
//
// On success the verified claims are stored in the request context with
// [ContextWithClaims] and the Authorization header is deleted before the next
// handler runs. The raw token never reaches the LLM, tools, logs, or session
// store. This package never logs the token value, at any level.
//
// # Deny codes
//
// Every denial returns a short JSON body, {"error":"<code>",
// "error_description":"<text>"}, with a stable code:
//
//	missing_token             401  no Authorization header
//	malformed_authorization   401  not exactly one Bearer header
//	unsupported_algorithm     401  alg is not ES256 or RS256
//	invalid_token             401  bad signature, unparseable, or unknown kid
//	token_expired             401  exp in the past beyond the clock skew
//	token_not_yet_valid       401  nbf or iat in the future beyond the skew
//	invalid_issuer            401  iss mismatch
//	invalid_audience          403  aud does not contain the configured audience
//	insufficient_scope        403  none of the required scopes are present
//	caller_not_allowed        403  azp (or client_id) is not an allowed caller
//	jwks_unavailable          503  JWKS unreachable and no cached key matches
//
// A 401 response carries WWW-Authenticate: Bearer realm="docsclaw",
// error="<code>". An insufficient_scope response also carries
// scope="<required scopes>".
//
// # Usage
//
//	h, err := inboundauth.Middleware(inboundauth.Config{
//		Issuer:   "http://token-issuer.zt-demo.svc:8080",
//		JWKSURL:  "http://token-issuer.zt-demo.svc:8080/.well-known/jwks.json",
//		Audience: "engineering-analyst",
//	}, mux)
//
// # Library choice
//
// Compared on 2026-11-06 against github.com/golang-jwt/jwt/v5 v5.3.1 (2026-01-28)
// with github.com/MicahParks/keyfunc/v3 v3.9.1 (2026-11-01):
//
//	                     jwx/v3 v3.4.0   jwt/v5 + keyfunc
//	latest release       2026-10-23      2026-01-28 + 2026-11-01
//	dependency edges     30              40
//	typed JWK parsing    built in        via keyfunc/jwkset
//	JWKS refresh         caller-driven   built in
//	algorithm pinning    WithKey         WithValidMethods
//	license              MIT             MIT + Apache-2.0
//
// jwx/v3 was chosen: one dependency instead of two, fewer transitive edges, and
// explicit control over the JWKS cache and refresh rate limit that the deny
// codes above require.
package inboundauth
