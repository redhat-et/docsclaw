package inboundauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"
)

var (
	errKeyUnknown = errors.New("inboundauth: unknown key id")
	errJWKSFetch  = errors.New("inboundauth: jwks fetch failed")
)

// keySource caches the issuer's public keys and refreshes them at most once
// per MinRefreshInterval. It fails closed: if the JWKS cannot be fetched, no
// request is verified.
type keySource struct {
	cfg Config

	mu        sync.Mutex
	keys      map[string]jwk.Key
	attempted bool
	lastFetch time.Time
}

func newKeySource(cfg Config) *keySource {
	return &keySource{cfg: cfg, keys: make(map[string]jwk.Key)}
}

func (k *keySource) key(ctx context.Context, kid string) (jwk.Key, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if key, ok := k.keys[kid]; ok {
		return key, nil
	}

	// Unknown kid. Refresh at most once per interval; a flood of random kids
	// must not turn into a flood of JWKS requests.
	if k.attempted && time.Since(k.lastFetch) < k.cfg.MinRefreshInterval {
		if len(k.keys) == 0 {
			return nil, errJWKSFetch
		}
		return nil, errKeyUnknown
	}

	if err := k.fetchLocked(ctx); err != nil {
		k.cfg.Logger.Warn("inboundauth: jwks fetch failed", "error", err)
		return nil, errJWKSFetch
	}
	if key, ok := k.keys[kid]; ok {
		return key, nil
	}
	return nil, errKeyUnknown
}

func (k *keySource) fetchLocked(ctx context.Context) error {
	k.attempted = true
	k.lastFetch = time.Now()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := k.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	set, err := jwk.Parse(body)
	if err != nil {
		return err
	}

	keys := make(map[string]jwk.Key, set.Len())
	for i := 0; i < set.Len(); i++ {
		key, ok := set.Key(i)
		if !ok {
			continue
		}
		kid, ok := key.KeyID()
		if !ok || kid == "" {
			continue
		}
		keys[kid] = key
	}
	k.keys = keys
	return nil
}
