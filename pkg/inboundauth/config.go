package inboundauth

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultClockSkew          = 30 * time.Second
	defaultMinRefreshInterval = 60 * time.Second
)

// Config describes the token issuer and the policy this middleware enforces.
type Config struct {
	Issuer   string
	JWKSURL  string
	Audience string

	// RequiredScopes requires at least one of the listed scopes. Empty
	// disables the scope check.
	RequiredScopes []string

	// AllowedCallers matches azp (or client_id when azp is absent). An
	// entry ending in "/" matches as a prefix; any other entry must match
	// exactly. Empty disables the caller check.
	AllowedCallers []string

	ClockSkew          time.Duration
	ExemptPaths        []string
	MinRefreshInterval time.Duration

	HTTPClient *http.Client
	Logger     *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.ClockSkew == 0 {
		c.ClockSkew = defaultClockSkew
	}
	if c.MinRefreshInterval == 0 {
		c.MinRefreshInterval = defaultMinRefreshInterval
	}
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

func (c Config) validate() error {
	if c.Issuer == "" {
		return fmt.Errorf("inboundauth: issuer is required")
	}
	if c.JWKSURL == "" {
		return fmt.Errorf("inboundauth: jwks_url is required")
	}
	u, err := url.Parse(c.JWKSURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("inboundauth: jwks_url must be an absolute http or https URL")
	}
	if c.Audience == "" {
		return fmt.Errorf("inboundauth: audience is required")
	}
	return nil
}
