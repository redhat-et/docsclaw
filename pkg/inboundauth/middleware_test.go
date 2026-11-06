package inboundauth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func baseConfig(ti *testIssuer) Config {
	return Config{
		Issuer:         testIssuerURL,
		JWKSURL:        ti.jwksURL(),
		Audience:       testAudience,
		RequiredScopes: []string{"engineering"},
		AllowedCallers: []string{"spiffe://openshell.local/openshell/sandbox/"},
		ExemptPaths:    []string{"/health"},
	}
}

func requestWithToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func noTokenRequest(path string) *http.Request {
	return httptest.NewRequest(http.MethodGet, path, nil)
}

func mustMiddleware(t *testing.T, cfg Config, next http.Handler) http.Handler {
	t.Helper()
	h, err := Middleware(cfg, next)
	if err != nil {
		t.Fatalf("Middleware() error: %v", err)
	}
	return h
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) (code, desc string) {
	t.Helper()
	var body struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error, body.Description
}

func tamperPayload(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		panic("tamperPayload: token does not have three parts")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		panic("tamperPayload: " + err.Error())
	}
	payload = bytes.Replace(payload, []byte(`"alice"`), []byte(`"mallory"`), 1)
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	return strings.Join(parts, ".")
}

func TestMiddleware(t *testing.T) {
	type row struct {
		name        string
		configure   func(*Config)
		request     func(*testIssuer) *http.Request
		wantStatus  int
		wantCode    string
		wantNext    bool
		wantClaims  bool
		checkClaims func(*testing.T, Claims)
		checkHeader func(*testing.T, http.Header)
	}

	rows := []row{
		{
			name:       "valid ES256",
			request:    func(ti *testIssuer) *http.Request { return requestWithToken(ti.mint("ES256", nil)) },
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
			checkClaims: func(t *testing.T, c Claims) {
				t.Helper()
				if c.Subject != "alice" {
					t.Errorf("Subject = %q, want alice", c.Subject)
				}
				if c.AZP != testAZP {
					t.Errorf("AZP = %q, want %q", c.AZP, testAZP)
				}
				if !containsScope(c.Scopes, "engineering") {
					t.Errorf("Scopes = %v, want engineering", c.Scopes)
				}
			},
		},
		{
			name:       "valid RS256",
			request:    func(ti *testIssuer) *http.Request { return requestWithToken(ti.mint("RS256", nil)) },
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name: "expired",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["exp"] = time.Now().Add(-time.Hour)
				}))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "token_expired",
		},
		{
			name: "exp within skew is accepted",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["exp"] = time.Now().Add(-20 * time.Second)
				}))
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name: "nbf in the future",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["nbf"] = time.Now().Add(time.Hour)
				}))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "token_not_yet_valid",
		},
		{
			name: "iat in the future",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["iat"] = time.Now().Add(time.Hour)
				}))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "token_not_yet_valid",
		},
		{
			name: "wrong issuer",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["iss"] = "https://other.test"
				}))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_issuer",
		},
		{
			name: "wrong audience",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["aud"] = []string{"finance-analyst"}
				}))
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "invalid_audience",
		},
		{
			name: "aud as plain string",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["aud"] = testAudience
				}))
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name: "missing required scope",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["scope"] = "read"
				}))
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "insufficient_scope",
			checkHeader: func(t *testing.T, h http.Header) {
				t.Helper()
				if got := h.Get("WWW-Authenticate"); !strings.Contains(got, `scope="engineering"`) {
					t.Errorf("WWW-Authenticate = %q, want scope=\"engineering\"", got)
				}
			},
		},
		{
			name: "scp array form",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					delete(c, "scope")
					c["scp"] = []string{"engineering"}
				}))
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name: "disallowed caller",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["azp"] = "spiffe://evil.example/sandbox/x"
				}))
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "caller_not_allowed",
		},
		{
			name: "azp absent but client_id allowed",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					delete(c, "azp")
					c["client_id"] = "spiffe://openshell.local/openshell/sandbox/bob"
				}))
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name: "azp and client_id absent",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					delete(c, "azp")
				}))
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "caller_not_allowed",
		},
		{
			name: "prefix rule allows sandbox path",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["azp"] = "spiffe://openshell.local/openshell/sandbox/bob"
				}))
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name: "prefix without trailing slash does not match sandboxes",
			configure: func(c *Config) {
				c.AllowedCallers = []string{"spiffe://openshell.local/openshell/sandbox"}
			},
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mint("ES256", func(c map[string]any) {
					c["azp"] = "spiffe://openshell.local/openshell/sandboxes/x"
				}))
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "caller_not_allowed",
		},
		{
			name:       "alg none",
			request:    func(ti *testIssuer) *http.Request { return requestWithToken(ti.mint("none", nil)) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unsupported_algorithm",
		},
		{
			name:       "alg HS256",
			request:    func(ti *testIssuer) *http.Request { return requestWithToken(ti.mint("HS256", nil)) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unsupported_algorithm",
		},
		{
			name: "unknown kid",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(ti.mintWithKid("ES256", "unknown-kid", nil))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_token",
		},
		{
			name: "tampered payload",
			request: func(ti *testIssuer) *http.Request {
				return requestWithToken(tamperPayload(ti.mint("ES256", nil)))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_token",
		},
		{
			name:       "missing header",
			request:    func(*testIssuer) *http.Request { return noTokenRequest("/a2a") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "missing_token",
			checkHeader: func(t *testing.T, h http.Header) {
				t.Helper()
				if got := h.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer ") {
					t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
				}
			},
		},
		{
			name: "duplicate authorization headers",
			request: func(ti *testIssuer) *http.Request {
				token := ti.mint("ES256", nil)
				r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
				r.Header.Add("Authorization", "Bearer "+token)
				r.Header.Add("Authorization", "Bearer "+token)
				return r
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "malformed_authorization",
		},
		{
			name: "basic scheme",
			request: func(*testIssuer) *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
				r.Header.Set("Authorization", "Basic xyz")
				return r
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "malformed_authorization",
		},
		{
			name: "empty bearer token",
			request: func(*testIssuer) *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
				r.Header.Set("Authorization", "Bearer ")
				return r
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "malformed_authorization",
		},
		{
			name: "lowercase bearer scheme",
			request: func(ti *testIssuer) *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
				r.Header.Set("Authorization", "bearer "+ti.mint("ES256", nil))
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantClaims: true,
		},
		{
			name:       "exempt path",
			request:    func(*testIssuer) *http.Request { return noTokenRequest("/health") },
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "exempt path strips a supplied token",
			request: func(ti *testIssuer) *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/health", nil)
				r.Header.Set("Authorization", "Bearer "+ti.mint("ES256", nil))
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "configured exempt entry is not cleaned",
			configure:  func(c *Config) { c.ExemptPaths = []string{"/health/"} },
			request:    func(*testIssuer) *http.Request { return noTokenRequest("/health") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "missing_token",
		},
		{
			name:       "exempt path does not cover healthz",
			request:    func(*testIssuer) *http.Request { return noTokenRequest("/healthz") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "missing_token",
		},
		{
			name:       "exempt path does not cover cleaned traversal",
			request:    func(*testIssuer) *http.Request { return noTokenRequest("/health/../a2a") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "missing_token",
		},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			ti := newTestIssuer(t)
			cfg := baseConfig(ti)
			if tc.configure != nil {
				tc.configure(&cfg)
			}

			var nextCalled bool
			var got Claims
			var gotClaims bool
			h := mustMiddleware(t, cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				got, gotClaims = ClaimsFrom(r.Context())
				if r.Header.Get("Authorization") != "" {
					t.Error("Authorization header reached the next handler")
				}
				w.WriteHeader(http.StatusOK)
			}))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tc.request(ti))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("next called = %v, want %v", nextCalled, tc.wantNext)
			}
			if tc.wantNext && gotClaims != tc.wantClaims {
				t.Fatalf("claims in context = %v, want %v", gotClaims, tc.wantClaims)
			}
			if tc.wantCode != "" {
				code, _ := decodeErrorBody(t, rec)
				if code != tc.wantCode {
					t.Fatalf("error code = %q, want %q", code, tc.wantCode)
				}
				if rec.Code == http.StatusUnauthorized {
					if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, `error="`+tc.wantCode+`"`) {
						t.Errorf("WWW-Authenticate = %q, want error=%q", got, tc.wantCode)
					}
				}
			}
			if tc.checkClaims != nil {
				tc.checkClaims(t, got)
			}
			if tc.checkHeader != nil {
				tc.checkHeader(t, rec.Header())
			}
		})
	}
}

func containsScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func TestJWKS_RefreshOnUnknownKid(t *testing.T) {
	ti := newTestIssuer(t)
	cfg := baseConfig(ti)
	// A negative interval disables rate limiting deterministically; the
	// in-flight flag still serializes concurrent refreshes.
	cfg.MinRefreshInterval = -1
	h := mustMiddleware(t, cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithToken(ti.mint("ES256", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec.Code)
	}
	if got := ti.hitCount(); got != 1 {
		t.Fatalf("JWKS hits = %d, want 1", got)
	}

	ti.rotate()

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithToken(ti.mint("ES256", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("rotated request status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if got := ti.hitCount(); got != 2 {
		t.Fatalf("JWKS hits = %d, want 2", got)
	}
}

// TestCachedKidServedDuringRefresh proves the JWKS mutex is not held across
// the network call: an unauthenticated unknown-kid request cannot stall
// verification for callers whose key is already cached.
func TestCachedKidServedDuringRefresh(t *testing.T) {
	ti := newTestIssuer(t)

	resp, err := http.Get(ti.jwksURL())
	if err != nil {
		t.Fatalf("fetch test JWKS: %v", err)
	}
	jwks, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read test JWKS: %v", err)
	}

	var hits atomic.Int64
	blocked := make(chan struct{})
	release := make(chan struct{})
	var signalOnce sync.Once
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) > 1 {
			signalOnce.Do(func() { close(blocked) })
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	defer jwksServer.Close()

	cfg := Config{
		Issuer:             testIssuerURL,
		JWKSURL:            jwksServer.URL,
		Audience:           testAudience,
		MinRefreshInterval: -1,
	}
	h := mustMiddleware(t, cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Prime the cache with es-1.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithToken(ti.mint("ES256", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("priming request status = %d, want 200", rec.Code)
	}

	// An unknown kid starts a refresh that blocks inside the JWKS handler.
	unknownDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requestWithToken(ti.mintWithKid("ES256", "unknown-kid", nil)))
		unknownDone <- rec.Code
	}()

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("unknown-kid request never started a JWKS refresh")
	}

	// A cached kid must be served while that refresh is in flight.
	cachedDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requestWithToken(ti.mint("ES256", nil)))
		cachedDone <- rec.Code
	}()

	select {
	case code := <-cachedDone:
		if code != http.StatusOK {
			t.Fatalf("cached-kid request status = %d, want 200", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cached-kid request blocked behind an in-flight refresh")
	}

	close(release)

	select {
	case code := <-unknownDone:
		if code != http.StatusUnauthorized {
			t.Fatalf("unknown-kid request status = %d, want 401", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unknown-kid request did not finish after the refresh was released")
	}
}

func TestJWKS_RefreshRateLimited(t *testing.T) {
	ti := newTestIssuer(t)
	cfg := baseConfig(ti)
	cfg.MinRefreshInterval = time.Hour
	h := mustMiddleware(t, cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithToken(ti.mint("ES256", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec.Code)
	}

	for i := 0; i < 5; i++ {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, requestWithToken(ti.mintWithKid("ES256", "random-"+string(rune('a'+i)), nil)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d status = %d, want 401", i, rec.Code)
		}
		code, _ := decodeErrorBody(t, rec)
		if code != "invalid_token" {
			t.Fatalf("request %d error = %q, want invalid_token", i, code)
		}
	}
	if got := ti.hitCount(); got > 2 {
		t.Fatalf("JWKS hits = %d, want at most 2", got)
	}
}

func TestJWKS_Unavailable(t *testing.T) {
	ti := newTestIssuer(t)
	cfg := baseConfig(ti)
	ti.server.Close()

	nextCalled := false
	h, err := Middleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
	}))
	if err != nil {
		t.Fatalf("Middleware() error = %v, want nil", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithToken(ti.mint("ES256", nil)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body %s)", rec.Code, rec.Body.String())
	}
	code, _ := decodeErrorBody(t, rec)
	if code != "jwks_unavailable" {
		t.Fatalf("error = %q, want jwks_unavailable", code)
	}
	if nextCalled {
		t.Fatal("next handler ran on a JWKS failure")
	}
}

func TestErrorBodyShape(t *testing.T) {
	ti := newTestIssuer(t)
	h := mustMiddleware(t, baseConfig(ti), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, noTokenRequest("/a2a"))

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("body has %d keys, want exactly error and error_description: %v", len(body), body)
	}
	if _, ok := body["error"]; !ok {
		t.Error("body is missing the error key")
	}
	if _, ok := body["error_description"]; !ok {
		t.Error("body is missing the error_description key")
	}
}
