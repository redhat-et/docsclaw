package inboundauth

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	valid := Config{
		Issuer:   "https://issuer.test",
		JWKSURL:  "https://issuer.test/jwks.json",
		Audience: "engineering-analyst",
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{
			name:   "ok",
			mutate: func(*Config) {},
		},
		{
			name:    "missing issuer",
			mutate:  func(c *Config) { c.Issuer = "" },
			wantErr: true,
		},
		{
			name:    "missing jwks url",
			mutate:  func(c *Config) { c.JWKSURL = "" },
			wantErr: true,
		},
		{
			name:    "relative jwks url",
			mutate:  func(c *Config) { c.JWKSURL = "/jwks.json" },
			wantErr: true,
		},
		{
			name:    "jwks url without host",
			mutate:  func(c *Config) { c.JWKSURL = "http:///jwks.json" },
			wantErr: true,
		},
		{
			name:    "missing audience",
			mutate:  func(c *Config) { c.Audience = "" },
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.mutate(&c)
			err := c.validate()
			if tc.wantErr && err == nil {
				t.Fatal("validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}.withDefaults()

	if c.ClockSkew != 30*time.Second {
		t.Errorf("ClockSkew = %v, want 30s", c.ClockSkew)
	}
	if c.MinRefreshInterval != 60*time.Second {
		t.Errorf("MinRefreshInterval = %v, want 60s", c.MinRefreshInterval)
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient = nil, want http.DefaultClient")
	}
	if c.Logger == nil {
		t.Error("Logger = nil, want a default logger")
	}
}

func TestConfigDefaultsDoNotOverride(t *testing.T) {
	c := Config{ClockSkew: time.Second, MinRefreshInterval: 2 * time.Second}.withDefaults()

	if c.ClockSkew != time.Second {
		t.Errorf("ClockSkew = %v, want 1s", c.ClockSkew)
	}
	if c.MinRefreshInterval != 2*time.Second {
		t.Errorf("MinRefreshInterval = %v, want 2s", c.MinRefreshInterval)
	}
}
