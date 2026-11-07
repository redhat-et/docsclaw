package a2acall

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/gowebpki/jcs"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jws"

	"github.com/redhat-et/docsclaw/internal/egress"
)

// Agent card verification failures are classified so a2a_call can tell a
// format problem apart from a forged card or an unreachable peer.
var (
	ErrCardFormat    = errors.New("agent card format invalid")
	ErrCardSignature = errors.New("agent card signature invalid")
)

// cardHTTPError carries a non-2xx response from the card endpoint so the tool
// can report it with the same vocabulary as any other outbound denial.
type cardHTTPError struct {
	status int
	body   []byte
}

func (e *cardHTTPError) Error() string {
	return fmt.Sprintf("agent card endpoint returned HTTP %d", e.status)
}

const (
	cardPath       = "/.well-known/agent-card.json"
	maxCardBytes   = 1 << 20
	maxCardErrBody = 4 << 10
	minRSAKeyBits  = 2048
)

// LoadCardPublicKey reads a PEM-encoded public key from path. PKIX (the usual
// "PUBLIC KEY" block) and PKCS#1 RSA keys are accepted. EC keys must use
// P-256; RSA keys must be at least 2048 bits.
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

	switch k := key.(type) {
	case *ecdsa.PublicKey:
		if k.Curve != elliptic.P256() {
			return nil, errors.New("card public key must use the P-256 curve")
		}
		return k, nil
	case *rsa.PublicKey:
		if k.N.BitLen() < minRSAKeyBits {
			return nil, fmt.Errorf("card public key is %d bits; at least %d required", k.N.BitLen(), minRSAKeyBits)
		}
		return k, nil
	default:
		return nil, fmt.Errorf("unsupported card public key type %T", key)
	}
}

// verifyCard fetches the peer's agent card from the well-known path on the
// peer's own origin and verifies one of its detached JWS signatures.
//
// The wire format is the A2A AgentCard JSON with a "signatures" array
// (AgentCardSignature: base64url "protected" header and "signature"). Per the
// A2A specification, the signed payload is the RFC 8785 (JCS) canonical form
// of the card with the "signatures" field removed; the JWS input is
// ASCII(protected + "." + base64url(payload)).
//
// A nil pub skips verification. Redirects to a different scheme or host are
// rejected.
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
	body, err := fetchCard(ctx, client, u)
	if err != nil {
		return err
	}

	var card a2a.AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return fmt.Errorf("%w: %v", ErrCardFormat, err)
	}
	canonical, err := canonicalCard(body)
	if err != nil {
		return err
	}

	alg, err := verifyAlgorithm(pub)
	if err != nil {
		return err
	}

	sawFormatErr := false
	sawSignatureMismatch := false
	for _, sig := range card.Signatures {
		ok, err := verifySignature(sig, alg, pub, canonical)
		if err != nil {
			sawFormatErr = true
			continue
		}
		if ok {
			return nil
		}
		sawSignatureMismatch = true
	}

	if len(card.Signatures) == 0 {
		return fmt.Errorf("%w: card has no signatures", ErrCardSignature)
	}
	if sawFormatErr && !sawSignatureMismatch {
		return fmt.Errorf("%w: no usable signature entry", ErrCardFormat)
	}
	return ErrCardSignature
}

func fetchCard(ctx context.Context, client *http.Client, peer *url.URL) ([]byte, error) {
	cardURL := (&url.URL{Scheme: peer.Scheme, Host: peer.Host, Path: cardPath}).String()

	if client == nil {
		client = egress.NewClient(0)
	}
	// Copy so the redirect policy is per-fetch and the shared client is never
	// mutated. A redirect may not downgrade the scheme or change the host.
	fetchClient := *client
	fetchClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != peer.Host || req.URL.Scheme != peer.Scheme {
			return fmt.Errorf("agent card redirect to %s://%s rejected", req.URL.Scheme, req.URL.Host)
		}
		if len(via) >= 5 {
			return errors.New("too many agent card redirects")
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build agent card request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch agent card: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, maxCardErrBody))
		return nil, &cardHTTPError{status: resp.StatusCode, body: preview}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCardBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read agent card: %w", err)
	}
	if len(body) > maxCardBytes {
		return nil, fmt.Errorf("%w: card exceeds %d bytes", ErrCardFormat, maxCardBytes)
	}
	return body, nil
}

// canonicalCard produces the JCS canonical form of the card with the
// "signatures" field removed. json.RawMessage preserves nested values exactly,
// so numbers are not reformatted between the wire and the canonical form.
func canonicalCard(body []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCardFormat, err)
	}
	delete(raw, "signatures")
	trimmed, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCardFormat, err)
	}
	canonical, err := jcs.Transform(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize: %v", ErrCardFormat, err)
	}
	return canonical, nil
}

// verifySignature checks one AgentCardSignature. It returns (false, nil) when
// the entry is well-formed but does not verify, and an error only when the
// entry itself is malformed.
func verifySignature(sig a2a.AgentCardSignature, alg jwa.SignatureAlgorithm, pub crypto.PublicKey, canonical []byte) (bool, error) {
	headerJSON, err := base64.RawURLEncoding.DecodeString(sig.Protected)
	if err != nil {
		return false, fmt.Errorf("%w: protected header is not base64url: %v", ErrCardFormat, err)
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return false, fmt.Errorf("%w: protected header is not JSON: %v", ErrCardFormat, err)
	}
	if header.Alg != alg.String() {
		// An algorithm we cannot use with this key (including none/HS*).
		return false, fmt.Errorf("%w: unsupported alg %q", ErrCardFormat, header.Alg)
	}
	if _, err := base64.RawURLEncoding.DecodeString(sig.Signature); err != nil {
		return false, fmt.Errorf("%w: signature is not base64url: %v", ErrCardFormat, err)
	}

	compact := sig.Protected + "." + base64.RawURLEncoding.EncodeToString(canonical) + "." + sig.Signature
	if _, err := jws.Verify([]byte(compact), jws.WithKey(alg, pub)); err != nil {
		return false, nil
	}
	return true, nil
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
