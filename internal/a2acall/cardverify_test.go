package a2acall

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/gowebpki/jcs"

	"github.com/redhat-et/docsclaw/internal/egress"
)

func testCardMap() map[string]any {
	return map[string]any{
		"name":        "peer",
		"description": "test peer",
		"version":     "1.0.0",
		"skills":      []any{},
		"capabilities": map[string]any{
			"streaming": false,
		},
	}
}

func newECKey(t *testing.T) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate EC key: %v", err)
	}
	return priv, &priv.PublicKey
}

func newRSAKey(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return priv, &priv.PublicKey
}

// signCard builds an A2A signed card using the procedure documented in
// verifyCard: JCS(card without "signatures") as a detached JWS payload, signed
// with a hand-rolled ES256/RS256 signer independent of the verifier.
func signCard(t *testing.T, card map[string]any, priv crypto.Signer, alg string) []byte {
	t.Helper()
	unsigned := maps.Clone(card)
	delete(unsigned, "signatures")
	raw, err := json.Marshal(unsigned)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatalf("canonicalize card: %v", err)
	}

	protected := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"` + alg + `","typ":"JOSE"}`))
	payload := base64.RawURLEncoding.EncodeToString(canonical)
	sum := sha256.Sum256([]byte(protected + "." + payload))

	var signature []byte
	switch key := priv.(type) {
	case *ecdsa.PrivateKey:
		r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
		if err != nil {
			t.Fatalf("sign with EC key: %v", err)
		}
		signature = make([]byte, 64)
		r.FillBytes(signature[:32])
		s.FillBytes(signature[32:])
	case *rsa.PrivateKey:
		signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
		if err != nil {
			t.Fatalf("sign with RSA key: %v", err)
		}
	default:
		t.Fatalf("unsupported signer %T", priv)
	}

	signed := maps.Clone(card)
	signed["signatures"] = []any{map[string]any{
		"protected": protected,
		"signature": base64.RawURLEncoding.EncodeToString(signature),
	}}
	body, err := json.Marshal(signed)
	if err != nil {
		t.Fatalf("marshal signed card: %v", err)
	}
	return body
}

func tamperCard(t *testing.T, signed []byte) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(signed, &m); err != nil {
		t.Fatalf("unmarshal signed card: %v", err)
	}
	m["name"] = "evil"
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal tampered card: %v", err)
	}
	return body
}

func cardOnlyServer(t *testing.T, card []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cardPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(card)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestVerifyCard(t *testing.T) {
	ecPriv, ecPub := newECKey(t)
	_, otherPub := newECKey(t)
	rsaPriv, rsaPub := newRSAKey(t)
	card := testCardMap()

	hs256Protected := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	hs256Card := maps.Clone(card)
	hs256Card["signatures"] = []any{map[string]any{
		"protected": hs256Protected,
		"signature": base64.RawURLEncoding.EncodeToString([]byte("nope")),
	}}
	hs256Body, _ := json.Marshal(hs256Card)

	noneProtected := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	noneCard := maps.Clone(card)
	noneCard["signatures"] = []any{map[string]any{
		"protected": noneProtected,
		"signature": "",
	}}
	noneBody, _ := json.Marshal(noneCard)

	unsigned, _ := json.Marshal(card)

	tests := []struct {
		name    string
		card    []byte
		pub     crypto.PublicKey
		wantErr error
	}{
		{name: "valid ES256", card: signCard(t, card, ecPriv, "ES256"), pub: ecPub},
		{name: "valid RS256", card: signCard(t, card, rsaPriv, "RS256"), pub: rsaPub},
		{name: "tampered card", card: tamperCard(t, signCard(t, card, ecPriv, "ES256")), pub: ecPub, wantErr: ErrCardSignature},
		{name: "wrong key", card: signCard(t, card, ecPriv, "ES256"), pub: otherPub, wantErr: ErrCardSignature},
		{name: "unsigned card", card: unsigned, pub: ecPub, wantErr: ErrCardSignature},
		{name: "HS256 rejected", card: hs256Body, pub: ecPub, wantErr: ErrCardFormat},
		{name: "alg none rejected", card: noneBody, pub: ecPub, wantErr: ErrCardFormat},
		{name: "nil key skips verification", card: unsigned, pub: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := cardOnlyServer(t, tc.card)
			err := verifyCard(context.Background(), egress.NewClient(5*time.Second), srv.URL, tc.pub)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected verification error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyCard_FormatInvalid(t *testing.T) {
	_, pub := newECKey(t)
	srv := cardOnlyServer(t, []byte("not json"))
	if err := verifyCard(context.Background(), egress.NewClient(5*time.Second), srv.URL, pub); !errors.Is(err, ErrCardFormat) {
		t.Fatalf("err = %v, want %v", err, ErrCardFormat)
	}
}

func TestVerifyCard_RejectsCrossHostRedirect(t *testing.T) {
	priv, pub := newECKey(t)
	valid := signCard(t, testCardMap(), priv, "ES256")
	target := cardOnlyServer(t, valid)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	if err := verifyCard(context.Background(), egress.NewClient(5*time.Second), redirector.URL, pub); err == nil {
		t.Fatal("expected a cross-host redirect to be rejected")
	}
}

func pemFor(t *testing.T, pub crypto.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestLoadCardPublicKey(t *testing.T) {
	_, ecPub := newECKey(t)
	_, rsaPub := newRSAKey(t)
	dir := t.TempDir()

	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	ecPath := write("ec.pub", pemFor(t, ecPub))
	got, err := LoadCardPublicKey(ecPath)
	if err != nil {
		t.Fatalf("load EC key: %v", err)
	}
	if gotEC, ok := got.(*ecdsa.PublicKey); !ok || !gotEC.Equal(ecPub) {
		t.Fatalf("loaded EC key does not match: %T", got)
	}

	if _, err := LoadCardPublicKey(write("rsa.pub", pemFor(t, rsaPub))); err != nil {
		t.Fatalf("load RSA 2048 key: %v", err)
	}

	smallRSA, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate 1024-bit RSA key: %v", err)
	}
	if _, err := LoadCardPublicKey(write("small.pub", pemFor(t, &smallRSA.PublicKey))); err == nil {
		t.Fatal("expected a 1024-bit RSA key to be rejected")
	}

	if _, err := LoadCardPublicKey(filepath.Join(dir, "missing.pub")); err == nil {
		t.Fatal("expected an error for a missing key file")
	}
	if _, err := LoadCardPublicKey(write("bad.pub", []byte("not a pem key"))); err == nil {
		t.Fatal("expected an error for a non-PEM key file")
	}
}

// peerServer serves both the agent card and the A2A JSON-RPC endpoint, so the
// card URL derived from the peer URL lands on the same origin.
func peerServer(t *testing.T, card *atomic.Value, cardHits *atomic.Int32) *httptest.Server {
	t.Helper()
	a2aHandler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(
		&scriptedExecutor{state: a2a.TaskStateCompleted, text: "peer answer"},
	))
	mux := http.NewServeMux()
	mux.HandleFunc(cardPath, func(w http.ResponseWriter, _ *http.Request) {
		cardHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(card.Load().([]byte))
	})
	mux.Handle("/", a2aHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCardVerificationCachedOnSuccess(t *testing.T) {
	priv, pub := newECKey(t)
	var card atomic.Value
	card.Store(signCard(t, testCardMap(), priv, "ES256"))
	var hits atomic.Int32
	srv := peerServer(t, &card, &hits)

	tool := New([]Peer{{Name: "peer", URL: srv.URL, CardKey: pub}}, nil, slog.Default())
	for i := range 2 {
		res := tool.Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
		if res.Error {
			t.Fatalf("call %d failed: %s", i, res.Output)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("card fetches = %d, want 1 (success must be cached)", got)
	}
}

func TestCardVerificationRetriedAfterFailure(t *testing.T) {
	priv, pub := newECKey(t)
	var card atomic.Value
	card.Store([]byte(`{"name":"peer"}`)) // unsigned: verification fails
	var hits atomic.Int32
	srv := peerServer(t, &card, &hits)

	tool := New([]Peer{{Name: "peer", URL: srv.URL, CardKey: pub}}, nil, slog.Default())
	res := tool.Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
	if !res.Error {
		t.Fatalf("expected verification failure, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "peer peer: agent card signature invalid") {
		t.Fatalf("unexpected error text: %q", res.Output)
	}

	card.Store(signCard(t, testCardMap(), priv, "ES256"))
	res = tool.Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
	if res.Error {
		t.Fatalf("fixed card should be retried and succeed: %s", res.Output)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("card fetches = %d, want 2 (failure must not be cached)", got)
	}
}

func TestCardVerificationErrorClasses(t *testing.T) {
	_, pub := newECKey(t)

	t.Run("format invalid", func(t *testing.T) {
		var card atomic.Value
		card.Store([]byte("not json"))
		var hits atomic.Int32
		srv := peerServer(t, &card, &hits)
		tool := New([]Peer{{Name: "peer", URL: srv.URL, CardKey: pub}}, nil, slog.Default())
		res := tool.Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
		if !strings.Contains(res.Output, "agent card format invalid") {
			t.Fatalf("output = %q", res.Output)
		}
	})

	t.Run("HTTP error keeps status", func(t *testing.T) {
		var hits atomic.Int32
		a2aHandler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(
			&scriptedExecutor{state: a2a.TaskStateCompleted, text: "peer answer"},
		))
		mux := http.NewServeMux()
		mux.HandleFunc(cardPath, func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			http.Error(w, `{"error":"invalid_audience"}`, http.StatusForbidden)
		})
		mux.Handle("/", a2aHandler)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		tool := New([]Peer{{Name: "peer", URL: srv.URL, CardKey: pub}}, nil, slog.Default())
		res := tool.Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
		if !strings.Contains(res.Output, "403 invalid_audience") {
			t.Fatalf("output = %q", res.Output)
		}
	})
}
