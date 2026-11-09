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
//	kid    JWS header kid override
//
// alg=wrongkey signs with an ES256 key that is not in the JWKS while keeping a
// published kid, which produces a token that fails signature verification.
// alg=none and alg=HS256 produce algorithm-confusion probes.
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

type issuer struct {
	esKid, rsKid string
	esPriv       *ecdsa.PrivateKey
	rsPriv       *rsa.PrivateKey
	wrongPriv    *ecdsa.PrivateKey
	jwks         jwk.Set
	hmacKey      []byte
}

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
		esKid: "es-1", rsKid: "rs-1",
		esPriv: esPriv, rsPriv: rsPriv, wrongPriv: wrongPriv,
		jwks: jwk.NewSet(), hmacKey: hmacKey,
	}
	for _, k := range []struct {
		kid  string
		alg  string
		priv any
	}{
		{it.esKid, "ES256", it.esPriv},
		{it.rsKid, "RS256", it.rsPriv},
	} {
		pub, err := publicJWK(k.priv, k.kid, k.alg)
		if err != nil {
			return nil, err
		}
		if err := it.jwks.AddKey(pub); err != nil {
			return nil, fmt.Errorf("add %s to JWKS: %w", k.kid, err)
		}
	}
	return it, nil
}

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
	alg    string
	kid    string
	claims map[string]any
}

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
		kid:    get("kid", ""),
		claims: claims,
	}, nil
}

func parseSeconds(raw, name string) (time.Duration, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer number of seconds", name)
	}
	return time.Duration(n) * time.Second, nil
}

func splitList(raw, sep string) []string {
	var out []string
	for _, part := range strings.Split(raw, sep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (it *issuer) mint(req *mintRequest) (string, error) {
	switch req.alg {
	case "ES256", "RS256", "wrongkey":
		var (
			priv any
			alg  jwa.KeyAlgorithm
			kid  string
		)
		switch req.alg {
		case "ES256":
			priv, alg, kid = it.esPriv, jwa.ES256(), it.esKid
		case "RS256":
			priv, alg, kid = it.rsPriv, jwa.RS256(), it.rsKid
		case "wrongkey":
			// Keep a published kid so key lookup succeeds and the signature
			// check is what fails.
			priv, alg, kid = it.wrongPriv, jwa.ES256(), it.esKid
		}
		if req.kid != "" {
			kid = req.kid
		}

		tok := jwt.New()
		for name, value := range req.claims {
			if err := tok.Set(name, value); err != nil {
				return "", fmt.Errorf("set claim %s: %w", name, err)
			}
		}
		hdrs := jws.NewHeaders()
		if err := hdrs.Set(jws.KeyIDKey, kid); err != nil {
			return "", fmt.Errorf("set kid header: %w", err)
		}
		signed, err := jwt.Sign(tok, jwt.WithKey(alg, priv, jws.WithProtectedHeaders(hdrs)))
		if err != nil {
			return "", fmt.Errorf("sign %s token: %w", req.alg, err)
		}
		return string(signed), nil
	case "none", "HS256":
		return it.manualToken(req.alg, req.kid, req.claims), nil
	default:
		return "", fmt.Errorf("unsupported alg %q", req.alg)
	}
}

// manualToken builds an unsigned or HMAC-signed JWS by hand, which jwx will not
// do for algorithms the verifier is meant to reject.
func (it *issuer) manualToken(alg, kid string, claims map[string]any) string {
	if kid == "" {
		kid = it.esKid
	}
	header := map[string]any{"alg": alg, "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(header)
	// encoding/json renders time.Time as RFC3339; JWT numeric dates are Unix
	// seconds, so convert before marshaling.
	numeric := make(map[string]any, len(claims))
	for name, value := range claims {
		if t, ok := value.(time.Time); ok {
			numeric[name] = t.Unix()
			continue
		}
		numeric[name] = value
	}
	cb, _ := json.Marshal(numeric)
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
		}{token, req.claims})
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
