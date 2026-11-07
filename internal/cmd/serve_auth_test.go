package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	authTestIssuerURL = "https://issuer.test"
	authTestAudience  = "engineering-analyst"
	authTestAZPPrefix = "spiffe://openshell.local/openshell/sandbox/"
)

// authTestIssuer is a minimal ES256 issuer + JWKS server, copied from the
// inboundauth package's fixture so internal/cmd tests stay self-contained.
type authTestIssuer struct {
	server *httptest.Server
	key    *ecdsa.PrivateKey
}

func newAuthTestIssuer(t *testing.T) *authTestIssuer {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privJWK, err := jwk.Import(key)
	if err != nil {
		t.Fatalf("import key: %v", err)
	}
	pubJWK, err := jwk.PublicKeyOf(privJWK)
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	if err := pubJWK.Set(jwk.KeyIDKey, "es-1"); err != nil {
		t.Fatalf("set kid: %v", err)
	}
	if err := pubJWK.Set(jwk.AlgorithmKey, "ES256"); err != nil {
		t.Fatalf("set alg: %v", err)
	}
	set := jwk.NewSet()
	if err := set.AddKey(pubJWK); err != nil {
		t.Fatalf("add key: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(set); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &authTestIssuer{server: server, key: key}
}

func (ti *authTestIssuer) jwksURL() string { return ti.server.URL + "/jwks.json" }

func (ti *authTestIssuer) mint(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	now := time.Now()
	claims := map[string]any{
		"iss":   authTestIssuerURL,
		"aud":   []string{authTestAudience},
		"sub":   "alice",
		"azp":   authTestAZPPrefix + "alice-assistant",
		"scope": "engineering read",
		"iat":   now,
		"nbf":   now,
		"exp":   now.Add(5 * time.Minute),
	}
	if mutate != nil {
		mutate(claims)
	}

	tok := jwt.New()
	for name, value := range claims {
		if err := tok.Set(name, value); err != nil {
			t.Fatalf("set claim %s: %v", name, err)
		}
	}
	hdrs := jws.NewHeaders()
	if err := hdrs.Set(jws.KeyIDKey, "es-1"); err != nil {
		t.Fatalf("set kid header: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), ti.key,
		jws.WithProtectedHeaders(hdrs)))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

// writeAuthConfigDir builds a config dir with an optional inbound_auth block.
func writeAuthConfigDir(t *testing.T, issuer *authTestIssuer, enabled bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "system-prompt.txt"),
		[]byte("You are a test agent. Answer briefly."), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := fmt.Sprintf("tools:\n  allowed: []\n  workspace: %s\n",
		filepath.Join(dir, "workspace"))
	if enabled {
		cfg += fmt.Sprintf(
			"inbound_auth:\n"+
				"  enabled: true\n"+
				"  issuer: %s\n"+
				"  jwks_url: %s\n"+
				"  audience: %s\n"+
				"  required_scopes: [engineering]\n"+
				"  allowed_callers: [\"%s\"]\n",
			authTestIssuerURL, issuer.jwksURL(), authTestAudience, authTestAZPPrefix)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"),
		[]byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newAuthRuntime(t *testing.T, issuer *authTestIssuer, enabled bool,
	logWriter io.Writer) *agentRuntime {
	t.Helper()
	rt, err := buildRuntime(context.Background(), Config{
		ConfigDir: writeAuthConfigDir(t, issuer, enabled),
		Workspace: t.TempDir(),
	}, runtimeOptions{Provider: &fakeProvider{}, LogWriter: logWriter})
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

func a2aRequestBody(t *testing.T) []byte {
	t.Helper()
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hi"))
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "SendMessage",
		"params":  map[string]any{"message": msg},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return body
}

func TestServeAuth_Routes(t *testing.T) {
	issuer := newAuthTestIssuer(t)
	rt := newAuthRuntime(t, issuer, true, io.Discard)
	handler, closeHandler, err := newServeHandler(rt)
	if err != nil {
		t.Fatalf("newServeHandler: %v", err)
	}
	t.Cleanup(func() { _ = closeHandler() })

	token := issuer.mint(t, nil)
	body := a2aRequestBody(t)

	tests := []struct {
		name    string
		method  string
		path    string
		token   string
		want    int
		wantErr string
	}{
		{name: "a2a no token", method: "POST", path: "/a2a", want: 401, wantErr: "missing_token"},
		{name: "a2a valid token", method: "POST", path: "/a2a", token: token, want: 200},
		{name: "json-rpc root no token", method: "POST", path: "/", want: 401, wantErr: "missing_token"},
		{name: "v1 chat no token", method: "POST", path: "/v1/chat/completions", want: 401, wantErr: "missing_token"},
		{name: "v1 models no token", method: "GET", path: "/v1/models", want: 401, wantErr: "missing_token"},
		{name: "agent card exempt", method: "GET", path: "/.well-known/agent-card.json", want: 200},
		{name: "health exempt", method: "GET", path: "/health", want: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(body))
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.wantErr != "" {
				var payload struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
					t.Fatalf("error body is not JSON: %v (%s)", err, rec.Body.String())
				}
				if payload.Error != tt.wantErr {
					t.Fatalf("error = %q, want %q", payload.Error, tt.wantErr)
				}
			}
		})
	}
}

// syncBuffer is a concurrency-safe log sink: the A2A runtime may execute
// requests on several goroutines, and bytes.Buffer is not safe for that.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestServeAuth_LogsIdentity(t *testing.T) {
	issuer := newAuthTestIssuer(t)
	buf := &syncBuffer{}
	rt := newAuthRuntime(t, issuer, true, buf)
	handler, closeHandler, err := newServeHandler(rt)
	if err != nil {
		t.Fatalf("newServeHandler: %v", err)
	}
	t.Cleanup(func() { _ = closeHandler() })

	token := issuer.mint(t, nil)
	req := httptest.NewRequest("POST", "/a2a", bytes.NewReader(a2aRequestBody(t)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	out := buf.String()
	for _, want := range []string{
		"request authenticated",
		"sub=alice",
		"azp=" + authTestAZPPrefix,
		"scope=engineering read",
		"aud=" + authTestAudience,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q; body=%s; log=%s", want, rec.Body.String(), out)
		}
	}
	if strings.Contains(out, token) {
		t.Fatal("raw token appeared in the log output")
	}
}

// The OpenAI-compatible tree must log the same identity line as A2A.
func TestServeAuth_V1LogsIdentity(t *testing.T) {
	issuer := newAuthTestIssuer(t)
	buf := &syncBuffer{}
	rt := newAuthRuntime(t, issuer, true, buf)
	handler, closeHandler, err := newServeHandler(rt)
	if err != nil {
		t.Fatalf("newServeHandler: %v", err)
	}
	t.Cleanup(func() { _ = closeHandler() })

	token := issuer.mint(t, nil)
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	out := buf.String()
	if !strings.Contains(out, "request authenticated") ||
		!strings.Contains(out, "sub=alice") {
		t.Fatalf("identity line missing from /v1 request:\n%s", out)
	}
	if strings.Contains(out, token) {
		t.Fatal("raw token appeared in the log output")
	}
}

func TestServeAuth_DisabledWarns(t *testing.T) {
	issuer := newAuthTestIssuer(t)
	buf := &syncBuffer{}
	rt := newAuthRuntime(t, issuer, false, buf)
	handler, closeHandler, err := newServeHandler(rt)
	if err != nil {
		t.Fatalf("newServeHandler: %v", err)
	}
	t.Cleanup(func() { _ = closeHandler() })

	req := httptest.NewRequest("POST", "/a2a", bytes.NewReader(a2aRequestBody(t)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(buf.String(), "inbound auth disabled") {
		t.Fatalf("missing startup warning:\n%s", buf.String())
	}
}
