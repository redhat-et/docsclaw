package a2acall

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log/slog"
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
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jws"

	"github.com/redhat-et/docsclaw/internal/egress"
)

var testCard = []byte(`{"name":"peer","description":"test peer","version":"1.0.0","skills":[]}`)

func newTestKey(t *testing.T) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return priv, &priv.PublicKey
}

func signTestCard(t *testing.T, priv *ecdsa.PrivateKey, payload []byte) []byte {
	t.Helper()
	signed, err := jws.Sign(payload, jws.WithKey(jwa.ES256(), priv))
	if err != nil {
		t.Fatalf("sign card: %v", err)
	}
	return signed
}

// tamper replaces the payload of a compact JWS while keeping the signature.
func tamper(t *testing.T, signed, payload []byte) []byte {
	t.Helper()
	parts := strings.Split(string(signed), ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected compact JWS shape: %d parts", len(parts))
	}
	return []byte(parts[0] + "." + base64.RawURLEncoding.EncodeToString(payload) + "." + parts[2])
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
	priv, pub := newTestKey(t)
	_, otherPub := newTestKey(t)
	valid := signTestCard(t, priv, testCard)

	tests := []struct {
		name    string
		card    []byte
		pub     crypto.PublicKey
		wantErr bool
	}{
		{name: "valid signature", card: valid, pub: pub},
		{name: "tampered card", card: tamper(t, valid, []byte(`{"name":"evil"}`)), pub: pub, wantErr: true},
		{name: "wrong key", card: valid, pub: otherPub, wantErr: true},
		{name: "unsigned card", card: testCard, pub: pub, wantErr: true},
		{name: "nil key skips verification", card: testCard, pub: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := cardOnlyServer(t, tc.card)
			err := verifyCard(context.Background(), egress.NewClient(5*time.Second), srv.URL, tc.pub)
			if tc.wantErr && err == nil {
				t.Fatal("expected verification error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected verification error: %v", err)
			}
		})
	}
}

func TestVerifyCard_RejectsCrossHostRedirect(t *testing.T) {
	priv, pub := newTestKey(t)
	valid := signTestCard(t, priv, testCard)
	target := cardOnlyServer(t, valid)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	if err := verifyCard(context.Background(), egress.NewClient(5*time.Second), redirector.URL, pub); err == nil {
		t.Fatal("expected a cross-host redirect to be rejected")
	}
}

func TestLoadCardPublicKey(t *testing.T) {
	_, pub := newTestKey(t)
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	dir := t.TempDir()
	good := filepath.Join(dir, "card.pub")
	if err := os.WriteFile(good, pemBytes, 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}
	got, err := LoadCardPublicKey(good)
	if err != nil {
		t.Fatalf("load public key: %v", err)
	}
	gotEC, ok := got.(*ecdsa.PublicKey)
	if !ok || !gotEC.Equal(pub) {
		t.Fatalf("loaded key does not match: %T", got)
	}

	if _, err := LoadCardPublicKey(filepath.Join(dir, "missing.pub")); err == nil {
		t.Fatal("expected an error for a missing key file")
	}

	bad := filepath.Join(dir, "bad.pub")
	if err := os.WriteFile(bad, []byte("not a pem key"), 0o600); err != nil {
		t.Fatalf("write bad pem: %v", err)
	}
	if _, err := LoadCardPublicKey(bad); err == nil {
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
	priv, pub := newTestKey(t)
	var card atomic.Value
	card.Store(signTestCard(t, priv, testCard))
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
	priv, pub := newTestKey(t)
	var card atomic.Value
	card.Store(testCard) // unsigned: verification fails
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

	card.Store(signTestCard(t, priv, testCard))
	res = tool.Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
	if res.Error {
		t.Fatalf("fixed card should be retried and succeed: %s", res.Output)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("card fetches = %d, want 2 (failure must not be cached)", got)
	}
}
