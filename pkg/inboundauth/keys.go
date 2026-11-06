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
//
// The mutex is never held across the network call. Cached kids are served
// while a refresh is in flight, and only one refresh runs at a time.
type keySource struct {
	cfg Config

	mu          sync.Mutex
	keys        map[string]jwk.Key
	attempted   bool
	lastFetch   time.Time
	lastErr     error
	refreshing  bool
	refreshDone chan struct{}
}

func newKeySource(cfg Config) *keySource {
	return &keySource{cfg: cfg, keys: make(map[string]jwk.Key)}
}

func (k *keySource) key(ctx context.Context, kid string) (jwk.Key, error) {
	k.mu.Lock()
	if key, ok := k.keys[kid]; ok {
		k.mu.Unlock()
		return key, nil
	}

	// The rate-limit decision and the attempt timestamp are updated under the
	// same lock hold, so concurrent callers cannot both start a refresh.
	if k.attempted && time.Since(k.lastFetch) < k.cfg.MinRefreshInterval {
		lastErr := k.lastErr
		k.mu.Unlock()
		if lastErr != nil {
			return nil, errJWKSFetch
		}
		return nil, errKeyUnknown
	}

	if k.refreshing {
		done := k.refreshDone
		k.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, errJWKSFetch
		}
		k.mu.Lock()
		key, ok := k.keys[kid]
		lastErr := k.lastErr
		k.mu.Unlock()
		if ok {
			return key, nil
		}
		if lastErr != nil {
			return nil, errJWKSFetch
		}
		return nil, errKeyUnknown
	}

	k.refreshing = true
	k.attempted = true
	k.lastFetch = time.Now()
	done := make(chan struct{})
	k.refreshDone = done
	k.mu.Unlock()

	keys, err := k.fetch(ctx)

	k.mu.Lock()
	if err == nil {
		k.keys = keys
	}
	k.lastErr = err
	k.refreshing = false
	k.refreshDone = nil
	close(done)
	var key jwk.Key
	var ok bool
	if err == nil {
		key, ok = k.keys[kid]
	}
	k.mu.Unlock()

	if err != nil {
		k.cfg.Logger.Warn("inboundauth: jwks fetch failed", "error", err)
		return nil, errJWKSFetch
	}
	if ok {
		return key, nil
	}
	return nil, errKeyUnknown
}

func (k *keySource) fetch(ctx context.Context) (map[string]jwk.Key, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.cfg.JWKSURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := k.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	set, err := jwk.Parse(body)
	if err != nil {
		return nil, err
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
	return keys, nil
}
