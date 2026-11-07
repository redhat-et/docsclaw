package a2acall

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jws"

	"github.com/redhat-et/docsclaw/internal/egress"
)

const (
	cardPath     = "/.well-known/agent-card.json"
	maxCardBytes = 1 << 20
)

// LoadCardPublicKey reads a PEM-encoded public key from path. PKIX (the usual
// "PUBLIC KEY" block) and PKCS#1 RSA keys are accepted; EC and RSA keys are the
// only types a signed card can be verified with.
func LoadCardPublicKey(path string) (crypto.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read card public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("card public key is not PEM-encoded")
	}

	var key any
	if key, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		if rsaKey, rsaErr := x509.ParsePKCS1PublicKey(block.Bytes); rsaErr == nil {
			key = rsaKey
		} else {
			return nil, fmt.Errorf("parse card public key: %w", err)
		}
	}

	switch key.(type) {
	case *ecdsa.PublicKey, *rsa.PublicKey:
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported card public key type %T", key)
	}
}

// verifyCard fetches the peer's agent card from the well-known path on the
// peer's own origin and verifies its JWS signature against pub. The card file
// is the JWS itself (compact or JSON serialization), whose payload is the agent
// card JSON — the same file SignedCardHandler serves verbatim. A nil pub skips
// verification. Redirects to a different host are rejected.
func verifyCard(ctx context.Context, client *http.Client, peerURL string, pub crypto.PublicKey) error {
	if pub == nil {
		return nil
	}

	u, err := url.Parse(peerURL)
	if err != nil {
		return fmt.Errorf("parse peer URL: %w", err)
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("peer URL %q is not an absolute http or https URL", peerURL)
	}
	cardURL := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: cardPath}).String()

	if client == nil {
		client = egress.NewClient(0)
	}
	// Copy so the redirect policy is per-fetch and the shared client is never
	// mutated.
	fetchClient := *client
	fetchClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != u.Host {
			return fmt.Errorf("agent card redirect to different host %q rejected", req.URL.Host)
		}
		if len(via) >= 5 {
			return errors.New("too many agent card redirects")
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return fmt.Errorf("build agent card request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := fetchClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch agent card: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch agent card: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCardBytes+1))
	if err != nil {
		return fmt.Errorf("read agent card: %w", err)
	}
	if len(body) > maxCardBytes {
		return fmt.Errorf("agent card exceeds %d bytes", maxCardBytes)
	}

	msg, err := jws.Parse(body)
	if err != nil {
		return fmt.Errorf("agent card is not a JWS: %w", err)
	}
	if len(msg.Signatures()) != 1 {
		return fmt.Errorf("agent card must carry exactly one signature, got %d", len(msg.Signatures()))
	}

	alg, err := verifyAlgorithm(pub)
	if err != nil {
		return err
	}
	if _, err := jws.Verify(body, jws.WithKey(alg, pub)); err != nil {
		return fmt.Errorf("agent card signature invalid: %w", err)
	}
	return nil
}

// verifyAlgorithm maps the configured key type to the only algorithm accepted
// for it. Anything else (HS*, none, unsupported curves) is refused.
func verifyAlgorithm(pub crypto.PublicKey) (jwa.SignatureAlgorithm, error) {
	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		if key.Curve != elliptic.P256() {
			return jwa.SignatureAlgorithm{}, errors.New("agent card key must use the P-256 curve")
		}
		return jwa.ES256(), nil
	case *rsa.PublicKey:
		return jwa.RS256(), nil
	default:
		return jwa.SignatureAlgorithm{}, fmt.Errorf("unsupported agent card key type %T", pub)
	}
}
