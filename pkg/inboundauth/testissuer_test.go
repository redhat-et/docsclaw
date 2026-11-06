package inboundauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	testIssuerURL = "https://issuer.test"
	testAudience  = "engineering-analyst"
	testAZP       = "spiffe://openshell.local/openshell/sandbox/alice-assistant"
)

type testIssuer struct {
	t      *testing.T
	server *httptest.Server
	hits   atomic.Int64

	mu      sync.Mutex
	keys    map[string]jwk.Key
	priv    map[string]any
	byAlg   map[string][]string
	hmacKey []byte
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()

	esKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ES256 key: %v", err)
	}
	rsKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RS256 key: %v", err)
	}
	rsPub, err := x509.MarshalPKIXPublicKey(&rsKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal RSA public key: %v", err)
	}

	ti := &testIssuer{
		t:       t,
		keys:    make(map[string]jwk.Key),
		priv:    make(map[string]any),
		byAlg:   make(map[string][]string),
		hmacKey: rsPub,
	}
	ti.addKey("es-1", "ES256", esKey)
	ti.addKey("rs-1", "RS256", rsKey)

	mux := http.NewServeMux()
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		ti.hits.Add(1)
		set := jwk.NewSet()
		ti.mu.Lock()
		for _, kid := range ti.sortedKids() {
			if err := set.AddKey(ti.keys[kid]); err != nil {
				ti.mu.Unlock()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		ti.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(set); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	ti.server = httptest.NewServer(mux)
	t.Cleanup(ti.server.Close)
	return ti
}

func (ti *testIssuer) sortedKids() []string {
	kids := make([]string, 0, len(ti.keys))
	for kid := range ti.keys {
		kids = append(kids, kid)
	}
	return kids
}

func (ti *testIssuer) addKey(kid, alg string, raw any) {
	ti.t.Helper()

	privJWK, err := jwk.Import(raw)
	if err != nil {
		ti.t.Fatalf("import %s key: %v", kid, err)
	}
	pubJWK, err := jwk.PublicKeyOf(privJWK)
	if err != nil {
		ti.t.Fatalf("public part of %s: %v", kid, err)
	}
	if err := pubJWK.Set(jwk.KeyIDKey, kid); err != nil {
		ti.t.Fatalf("set kid: %v", err)
	}
	if err := pubJWK.Set(jwk.AlgorithmKey, alg); err != nil {
		ti.t.Fatalf("set alg: %v", err)
	}

	ti.mu.Lock()
	defer ti.mu.Unlock()
	ti.keys[kid] = pubJWK
	ti.priv[kid] = raw
	ti.byAlg[alg] = append(ti.byAlg[alg], kid)
}

// rotate adds a new ES256 key to the JWKS without touching the old one.
func (ti *testIssuer) rotate() {
	ti.t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		ti.t.Fatalf("generate rotated key: %v", err)
	}
	ti.addKey("es-2", "ES256", key)
}

func (ti *testIssuer) jwksURL() string {
	return ti.server.URL + "/jwks.json"
}

func (ti *testIssuer) hitCount() int {
	return int(ti.hits.Load())
}

func (ti *testIssuer) currentKid(alg string) string {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	return ti.currentKidLocked(alg)
}

func (ti *testIssuer) currentKidLocked(alg string) string {
	kids := ti.byAlg[alg]
	if len(kids) == 0 {
		ti.t.Fatalf("no key for algorithm %s", alg)
	}
	return kids[len(kids)-1]
}

func (ti *testIssuer) defaultClaims() map[string]any {
	now := time.Now()
	return map[string]any{
		"iss":   testIssuerURL,
		"aud":   []string{testAudience},
		"sub":   "alice",
		"azp":   testAZP,
		"scope": "engineering read",
		"iat":   now,
		"nbf":   now,
		"exp":   now.Add(5 * time.Minute),
	}
}

// mint signs a token with the most recently added key for alg. alg may also be
// "none" or "HS256", which produce tokens for the algorithm-confusion tests.
func (ti *testIssuer) mint(alg string, mutate func(map[string]any)) string {
	ti.t.Helper()
	return ti.mintWithKid(alg, "", mutate)
}

// mintWithKid sets the JWS header kid to kid while signing with the current
// key for alg, which is how an unknown-kid token is produced.
func (ti *testIssuer) mintWithKid(alg, kid string, mutate func(map[string]any)) string {
	ti.t.Helper()

	claims := ti.defaultClaims()
	if mutate != nil {
		mutate(claims)
	}

	switch alg {
	case "ES256", "RS256":
		headerKid := kid
		if headerKid == "" {
			headerKid = ti.currentKid(alg)
		}
		tok := jwt.New()
		for name, value := range claims {
			if err := tok.Set(name, value); err != nil {
				ti.t.Fatalf("set claim %s: %v", name, err)
			}
		}
		hdrs := jws.NewHeaders()
		if err := hdrs.Set(jws.KeyIDKey, headerKid); err != nil {
			ti.t.Fatalf("set kid header: %v", err)
		}
		ti.mu.Lock()
		priv := ti.priv[ti.currentKidLocked(alg)]
		ti.mu.Unlock()
		signed, err := jwt.Sign(tok, jwt.WithKey(algorithm(alg), priv, jws.WithProtectedHeaders(hdrs)))
		if err != nil {
			ti.t.Fatalf("sign %s token: %v", alg, err)
		}
		return string(signed)
	case "none":
		return ti.manualToken(alg, "es-1", claims, nil)
	case "HS256":
		return ti.manualToken(alg, "es-1", claims, ti.hmacKey)
	default:
		ti.t.Fatalf("unsupported test algorithm %s", alg)
		return ""
	}
}

func (ti *testIssuer) manualToken(alg, kid string, claims map[string]any, hmacKey []byte) string {
	ti.t.Helper()

	header := map[string]any{"alg": alg, "typ": "JWT", "kid": kid}
	hb, err := json.Marshal(header)
	if err != nil {
		ti.t.Fatalf("marshal header: %v", err)
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		ti.t.Fatalf("marshal claims: %v", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)

	var sig []byte
	if hmacKey != nil {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write([]byte(signingInput))
		sig = mac.Sum(nil)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func algorithm(alg string) jwa.KeyAlgorithm {
	switch alg {
	case "ES256":
		return jwa.ES256()
	case "RS256":
		return jwa.RS256()
	default:
		return jwa.NoSignature()
	}
}
