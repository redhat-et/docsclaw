package inboundauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Claims is the verified identity of a caller. Raw holds every claim in the
// token; it never contains the token itself.
type Claims struct {
	Subject  string
	Issuer   string
	Audience []string
	AZP      string
	Scopes   []string
	Expiry   time.Time
	Raw      map[string]any
}

type claimsKey struct{}

// ContextWithClaims stores verified claims in ctx.
func ContextWithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFrom returns the verified claims stored by the middleware.
func ClaimsFrom(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(Claims)
	return c, ok
}

// verifyToken checks algorithm, key, signature, time, issuer, audience,
// scope, and caller in that order, so the first failing check decides the
// deny code.
func verifyToken(ctx context.Context, cfg Config, ks *keySource, raw string) (Claims, *authError) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Claims{}, errInvalidToken
	}

	header, ok := decodeSegment(parts[0])
	if !ok {
		return Claims{}, errInvalidToken
	}
	alg, _ := header["alg"].(string)
	if alg != "ES256" && alg != "RS256" {
		return Claims{}, errUnsupportedAlgorithm
	}
	kid, _ := header["kid"].(string)
	if kid == "" {
		return Claims{}, errInvalidToken
	}

	key, err := ks.key(ctx, kid)
	switch {
	case errors.Is(err, errJWKSFetch):
		return Claims{}, errJWKSUnavailable
	case err != nil:
		return Claims{}, errInvalidToken
	}

	tok, err := jwt.Parse([]byte(raw), jwt.WithKey(keyAlgorithm(alg), key), jwt.WithValidate(false))
	if err != nil {
		return Claims{}, errInvalidToken
	}

	now := time.Now()
	skew := cfg.ClockSkew

	exp, ok := tok.Expiration()
	if !ok {
		return Claims{}, errInvalidToken
	}
	if now.After(exp.Add(skew)) {
		return Claims{}, errTokenExpired
	}
	if nbf, ok := tok.NotBefore(); ok && nbf.After(now.Add(skew)) {
		return Claims{}, errTokenNotYetValid
	}
	if iat, ok := tok.IssuedAt(); ok && iat.After(now.Add(skew)) {
		return Claims{}, errTokenNotYetValid
	}

	issuer, _ := tok.Issuer()
	if issuer != cfg.Issuer {
		return Claims{}, errInvalidIssuer
	}

	audience, _ := tok.Audience()
	if !containsString(audience, cfg.Audience) {
		return Claims{}, errInvalidAudience
	}

	scopes := tokenScopes(tok)
	if len(cfg.RequiredScopes) > 0 && !containsAny(scopes, cfg.RequiredScopes) {
		return Claims{}, &authError{
			status: http.StatusForbidden,
			code:   "insufficient_scope",
			desc:   "token is missing a required scope",
			scopes: cfg.RequiredScopes,
		}
	}

	azp := tokenCaller(tok)
	if len(cfg.AllowedCallers) > 0 && !callerAllowed(azp, cfg.AllowedCallers) {
		return Claims{}, errCallerNotAllowed
	}

	claims, ok := decodeSegment(parts[1])
	if !ok {
		return Claims{}, errInvalidToken
	}

	return Claims{
		Subject:  claimsString(claims, "sub"),
		Issuer:   issuer,
		Audience: audience,
		AZP:      azp,
		Scopes:   scopes,
		Expiry:   exp,
		Raw:      claims,
	}, nil
}

func keyAlgorithm(alg string) jwa.KeyAlgorithm {
	if alg == "RS256" {
		return jwa.RS256()
	}
	return jwa.ES256()
}

func decodeSegment(segment string) (map[string]any, bool) {
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false
	}
	return out, true
}

func claimsString(claims map[string]any, name string) string {
	v, _ := claims[name].(string)
	return v
}

func tokenScopes(tok jwt.Token) []string {
	var out []string

	var single string
	if err := tok.Get("scope", &single); err == nil {
		out = append(out, strings.Fields(single)...)
	}

	if err := tok.Get("scp", &single); err == nil {
		out = append(out, strings.Fields(single)...)
		return out
	}
	var list []string
	if err := tok.Get("scp", &list); err == nil {
		return append(out, list...)
	}
	var generic []any
	if err := tok.Get("scp", &generic); err == nil {
		for _, v := range generic {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}

	return out
}

func tokenCaller(tok jwt.Token) string {
	var caller string
	if err := tok.Get("azp", &caller); err == nil && caller != "" {
		return caller
	}
	if err := tok.Get("client_id", &caller); err == nil {
		return caller
	}
	return ""
}

func callerAllowed(caller string, allowed []string) bool {
	if caller == "" {
		return false
	}
	for _, entry := range allowed {
		if strings.HasSuffix(entry, "/") {
			if strings.HasPrefix(caller, entry) {
				return true
			}
		} else if caller == entry {
			return true
		}
	}
	return false
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func containsAny(haystack, needles []string) bool {
	for _, n := range needles {
		if containsString(haystack, n) {
			return true
		}
	}
	return false
}
