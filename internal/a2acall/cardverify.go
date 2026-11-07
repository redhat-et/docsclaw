package a2acall

import (
	"bytes"
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
	payloads, err := cardPayloads(body)
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
		ok, err := verifySignature(sig, alg, pub, payloads)
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

// cardPayloads returns the canonical payload candidates a signature may cover.
//
// The A2A specification keeps optional fields that were explicitly set, even
// at their default value, so the as-served form is the first candidate. But a
// server that re-marshals a card through a struct can emit defaults the signer
// never set; the second candidate therefore strips default-valued optional
// fields (and empty non-required arrays/objects) before canonicalization.
// Accepting either form cannot weaken the binding: a signature must still
// verify over the exact byte string that was signed.
func cardPayloads(body []byte) ([][]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCardFormat, err)
	}
	delete(raw, "signatures")

	var candidates [][]byte
	for _, form := range []map[string]json.RawMessage{
		raw,
		stripEmptyCollections(raw),
		stripAllDefaults(raw),
		dropEmptyRequired(stripAllDefaults(raw)),
	} {
		canonical, err := canonicalize(form)
		if err != nil {
			return nil, err
		}
		duplicate := false
		for _, existing := range candidates {
			if bytes.Equal(existing, canonical) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidates = append(candidates, canonical)
		}
	}
	return candidates, nil
}

// dropEmptyRequired removes required properties that arrive empty. It is a
// compatibility form for servers whose Go marshaling re-emits required
// struct fields (capabilities, skills, modes) that the signer omitted; the
// signature still has to verify over the exact bytes that were signed.
func dropEmptyRequired(raw map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		if requiredCardFields[key] && isZeroJSON(value) {
			continue
		}
		out[key] = value
	}
	return out
}

func canonicalize(raw map[string]json.RawMessage) ([]byte, error) {
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

// requiredCardFields are always kept, even when empty, per A2A §8.4.
var requiredCardFields = map[string]bool{
	"name":                true,
	"description":         true,
	"version":             true,
	"capabilities":        true,
	"skills":              true,
	"supportedInterfaces": true,
	"defaultInputModes":   true,
	"defaultOutputModes":  true,
}

// stripEmptyCollections removes empty strings, arrays, and objects, but keeps
// explicitly-present false booleans and zero numbers. This is the form the
// A2A worked example signs: it drops an empty capabilities.extensions array
// while keeping explicitly-set streaming/pushNotifications false flags.
func stripEmptyCollections(raw map[string]json.RawMessage) map[string]json.RawMessage {
	return stripValues(raw, isZeroCollection)
}

// stripAllDefaults additionally removes false booleans and zero numbers. It is
// the form a signer produces when it strips every default-valued property, and
// the form a struct-based server can diverge from by re-emitting them.
func stripAllDefaults(raw map[string]json.RawMessage) map[string]json.RawMessage {
	return stripValues(raw, isZeroJSON)
}

func stripValues(raw map[string]json.RawMessage, zero func(json.RawMessage) bool) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		if requiredCardFields[key] {
			out[key] = value
			continue
		}
		if !zero(value) {
			out[key] = value
		}
	}

	if capRaw, ok := out["capabilities"]; ok {
		var caps map[string]json.RawMessage
		if err := json.Unmarshal(capRaw, &caps); err == nil {
			for key, value := range caps {
				if zero(value) {
					delete(caps, key)
				}
			}
			if encoded, err := json.Marshal(caps); err == nil {
				out["capabilities"] = encoded
			}
		}
	}
	return out
}

func isZeroCollection(raw json.RawMessage) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

func isZeroJSON(raw json.RawMessage) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch v := value.(type) {
	case nil:
		return true
	case bool:
		return !v
	case string:
		return v == ""
	case float64:
		return v == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

// verifySignature checks one AgentCardSignature against each payload candidate.
// It returns (false, nil) when the entry is well-formed but does not verify,
// and an error only when the entry itself is malformed.
func verifySignature(sig a2a.AgentCardSignature, alg jwa.SignatureAlgorithm, pub crypto.PublicKey, payloads [][]byte) (bool, error) {
	headerJSON, err := base64.RawURLEncoding.DecodeString(sig.Protected)
	if err != nil {
		return false, fmt.Errorf("%w: protected header is not base64url: %v", ErrCardFormat, err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return false, fmt.Errorf("%w: protected header is not JSON: %v", ErrCardFormat, err)
	}
	if header.Alg != alg.String() {
		// An algorithm we cannot use with this key (including none/HS*).
		return false, fmt.Errorf("%w: unsupported alg %q", ErrCardFormat, header.Alg)
	}
	if header.Typ != "JOSE" {
		return false, fmt.Errorf("%w: protected header typ %q, want JOSE", ErrCardFormat, header.Typ)
	}
	if header.Kid == "" {
		return false, fmt.Errorf("%w: protected header has no kid", ErrCardFormat)
	}
	if _, err := base64.RawURLEncoding.DecodeString(sig.Signature); err != nil {
		return false, fmt.Errorf("%w: signature is not base64url: %v", ErrCardFormat, err)
	}

	for _, payload := range payloads {
		compact := sig.Protected + "." + base64.RawURLEncoding.EncodeToString(payload) + "." + sig.Signature
		if _, err := jws.Verify([]byte(compact), jws.WithKey(alg, pub)); err == nil {
			return true, nil
		}
	}
	return false, nil
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
