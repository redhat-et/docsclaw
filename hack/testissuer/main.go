// Command testissuer serves a JWKS and mints ES256/RS256 tokens for manual
// testing of inbound auth. It is the standalone form of the fixture in
// pkg/inboundauth/testissuer_test.go, and exists because the Zero Trust
// Agents v2 issuer signs HS256 and publishes no JWKS.
//
// Usage:
//
//	testissuer -addr 127.0.0.1:9099
//	curl -s 'http://127.0.0.1:9099/token?sub=alice&scope=engineering' | jq -r .token
//
// Query parameters for /token (all optional):
//
//	sub    subject claim (default alice)
//	azp    authorized party (default spiffe://openshell.local/openshell/sandbox/alice-assistant)
//	aud    audience; comma-separated for several (default engineering-analyst)
//	scope  space-separated scopes (default "engineering read")
//	ttl    seconds from now until exp, may be negative (default 300)
//	nbf    seconds from now until nbf, may be positive (default 0)
//	iss    issuer claim override (default the -issuer flag)
//	jti    token ID, useful as a leak marker
//	alg    ES256 (default), RS256, wrongkey, none, or HS256
//	kid    JWS header kid override; kid= (empty) omits the header entirely
//
// alg=wrongkey signs with an ES256 key that is not in the JWKS while keeping a
// published kid, which produces a token that fails signature verification.
// alg=none and alg=HS256 produce algorithm-confusion probes.
//
// The in-process fixture in pkg/inboundauth/testissuer_test.go mints the same
// deny shapes for the automated suite. This command is the standalone form;
// when a new verifier deny path is added, extend it here too.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	defaultSub   = "alice"
	defaultAZP   = "spiffe://openshell.local/openshell/sandbox/alice-assistant"
	defaultAud   = "engineering-analyst"
	defaultScope = "engineering read"
)

// signingKey pairs a private key with the JWS algorithm and published kid it
// signs under. The "wrongkey" entry deliberately reuses the published ES256
// kid, so key lookup succeeds and signature verification is what fails.
type signingKey struct {
	priv any
	alg  jwa.KeyAlgorithm
	kid  string
}

type issuer struct {
	jwks    jwk.Set
	hmacKey []byte
	signers map[string]signingKey
}

// newIssuer generates the signing keys and publishes the public halves.
func newIssuer() (*issuer, error) {
	esPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ES256 key: %w", err)
	}
	rsPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RS256 key: %w", err)
	}
	wrongPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate wrong key: %w", err)
	}
	hmacKey := make([]byte, 32)
	if _, err := rand.Read(hmacKey); err != nil {
		return nil, fmt.Errorf("generate hmac key: %w", err)
	}

	it := &issuer{
		jwks:    jwk.NewSet(),
		hmacKey: hmacKey,
		signers: map[string]signingKey{
			"ES256":    {priv: esPriv, alg: jwa.ES256(), kid: "es-1"},
			"RS256":    {priv: rsPriv, alg: jwa.RS256(), kid: "rs-1"},
			"wrongkey": {priv: wrongPriv, alg: jwa.ES256(), kid: "es-1"},
		},
	}
	for _, name := range []string{"ES256", "RS256"} {
		sk := it.signers[name]
		pub, err := publicJWK(sk.priv, sk.kid, name)
		if err != nil {
			return nil, err
		}
		if err := it.jwks.AddKey(pub); err != nil {
			return nil, fmt.Errorf("add %s to JWKS: %w", sk.kid, err)
		}
	}
	return it, nil
}

// publicJWK returns the public part of priv with kid and alg set, ready for
// the published JWKS set.
func publicJWK(priv any, kid, alg string) (jwk.Key, error) {
	privJWK, err := jwk.Import(priv)
	if err != nil {
		return nil, fmt.Errorf("import %s key: %w", kid, err)
	}
	pubJWK, err := jwk.PublicKeyOf(privJWK)
	if err != nil {
		return nil, fmt.Errorf("public part of %s: %w", kid, err)
	}
	if err := pubJWK.Set(jwk.KeyIDKey, kid); err != nil {
		return nil, fmt.Errorf("set kid %s: %w", kid, err)
	}
	if err := pubJWK.Set(jwk.AlgorithmKey, alg); err != nil {
		return nil, fmt.Errorf("set alg %s: %w", kid, err)
	}
	return pubJWK, nil
}

type mintRequest struct {
	alg string
	// kidSet reports that the caller supplied the kid parameter, so kid=""
	// means "omit the header" rather than "use the default".
	kidSet bool
	kid    string
	claims map[string]any
}

// parseMintRequest turns the /token query into the claims and signing options
// for one token.
func parseMintRequest(q map[string][]string, issuerURL string) (*mintRequest, error) {
	get := func(name, fallback string) string {
		if vs, ok := q[name]; ok && len(vs) > 0 {
			return vs[0]
		}
		return fallback
	}
	has := func(name string) bool { _, ok := q[name]; return ok }

	now := time.Now()
	ttl, err := parseSeconds(get("ttl", "300"), "ttl")
	if err != nil {
		return nil, err
	}
	nbfOffset, err := parseSeconds(get("nbf", "0"), "nbf")
	if err != nil {
		return nil, err
	}

	claims := map[string]any{
		"iss": get("iss", issuerURL),
		"sub": get("sub", defaultSub),
		"iat": now,
		"nbf": now.Add(nbfOffset),
		"exp": now.Add(ttl),
	}
	if has("azp") {
		if v := get("azp", ""); v != "" {
			claims["azp"] = v
		}
	} else {
		claims["azp"] = defaultAZP
	}
	if has("aud") {
		claims["aud"] = splitList(get("aud", ""), ",")
	} else {
		claims["aud"] = []string{defaultAud}
	}
	claims["scope"] = get("scope", defaultScope)
	if v := get("jti", ""); v != "" {
		claims["jti"] = v
	}

	return &mintRequest{
		alg:    get("alg", "ES256"),
		kidSet: has("kid"),
		kid:    get("kid", ""),
		claims: claims,
	}, nil
}

// numericClaims converts time.Time claims to Unix seconds, which is how a JWT
// encodes dates. Without it, encoding/json would emit RFC3339 strings and the
// /token echo would not match the decoded token.
func numericClaims(claims map[string]any) map[string]any {
	out := make(map[string]any, len(claims))
	for name, value := range claims {
		if t, ok := value.(time.Time); ok {
			out[name] = t.Unix()
			continue
		}
		out[name] = value
	}
	return out
}

// parseSeconds parses an integer number of seconds from a query value.
func parseSeconds(raw, name string) (time.Duration, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer number of seconds", name)
	}
	return time.Duration(n) * time.Second, nil
}

// splitList splits raw on sep and drops whitespace-only elements.
func splitList(raw, sep string) []string {
	var out []string
	for _, part := range strings.Split(raw, sep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// mint signs the requested token, including the deny shapes the verifier is
// meant to reject.
func (it *issuer) mint(req *mintRequest) (string, error) {
	sk, ok := it.signers[req.alg]
	if !ok {
		switch req.alg {
		case "none", "HS256":
			return it.manualToken(req.alg, req.kid, req.kidSet, req.claims), nil
		default:
			return "", fmt.Errorf("unsupported alg %q", req.alg)
		}
	}

	kid := sk.kid
	if req.kidSet {
		kid = req.kid
	}

	tok := jwt.New()
	for name, value := range req.claims {
		if err := tok.Set(name, value); err != nil {
			return "", fmt.Errorf("set claim %s: %w", name, err)
		}
	}
	hdrs := jws.NewHeaders()
	if kid != "" {
		if err := hdrs.Set(jws.KeyIDKey, kid); err != nil {
			return "", fmt.Errorf("set kid header: %w", err)
		}
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(sk.alg, sk.priv, jws.WithProtectedHeaders(hdrs)))
	if err != nil {
		return "", fmt.Errorf("sign %s token: %w", req.alg, err)
	}
	return string(signed), nil
}

// manualToken builds an unsigned or HMAC-signed JWS by hand, which jwx will not
// do for algorithms the verifier is meant to reject.
func (it *issuer) manualToken(alg, kid string, kidSet bool, claims map[string]any) string {
	if !kidSet {
		kid = "es-1"
	}
	header := map[string]any{"alg": alg, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(numericClaims(claims))
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(cb)

	var sig []byte
	if alg == "HS256" {
		mac := hmac.New(sha256.New, it.hmacKey)
		mac.Write([]byte(signingInput))
		sig = mac.Sum(nil)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// main serves the JWKS and token endpoints until the process is stopped.
func main() {
	addr := flag.String("addr", "127.0.0.1:9099", "listen address")
	issuerURL := flag.String("issuer", "", "issuer claim URL (default http://<addr>)")
	flag.Parse()

	if *issuerURL == "" {
		host, port, err := net.SplitHostPort(*addr)
		if err != nil {
			log.Fatalf("invalid -addr %q: %v", *addr, err)
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		*issuerURL = "http://" + net.JoinHostPort(host, port)
	}

	it, err := newIssuer()
	if err != nil {
		log.Fatalf("init issuer: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(it.jwks); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		req, err := parseMintRequest(r.URL.Query(), *issuerURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		token, err := it.mint(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Token  string         `json:"token"`
			Claims map[string]any `json:"claims"`
		}{token, numericClaims(req.claims)})
	})

	log.Printf("test issuer listening on %s (iss=%s, jwks=%s/jwks.json)",
		*addr, *issuerURL, *issuerURL)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
