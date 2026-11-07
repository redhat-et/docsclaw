package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		authOn     bool
		want       string
	}{
		{name: "auth on defaults to loopback", configured: "", authOn: true, want: "127.0.0.1"},
		{name: "auth off defaults to all interfaces", configured: "", authOn: false, want: "0.0.0.0"},
		{name: "explicit host wins", configured: "10.0.0.5", authOn: true, want: "10.0.0.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveHost(tt.configured, tt.authOn); got != tt.want {
				t.Fatalf("resolveHost(%q, %v) = %q, want %q", tt.configured, tt.authOn, got, tt.want)
			}
		})
	}
}

func TestInboundAuthCfgValidate(t *testing.T) {
	valid := func() *InboundAuthCfg {
		return &InboundAuthCfg{
			Enabled:  true,
			Issuer:   "https://issuer.test",
			JWKSURL:  "https://issuer.test/jwks.json",
			Audience: "engineering-analyst",
		}
	}
	tests := []struct {
		name    string
		cfg     *InboundAuthCfg
		wantErr string
	}{
		{name: "valid", cfg: valid()},
		{name: "nil is disabled", cfg: nil},
		{name: "disabled with empty fields", cfg: &InboundAuthCfg{}},
		{
			name: "missing issuer",
			cfg: func() *InboundAuthCfg {
				c := valid()
				c.Issuer = ""
				return c
			}(),
			wantErr: "inbound_auth.issuer",
		},
		{
			name: "missing audience",
			cfg: func() *InboundAuthCfg {
				c := valid()
				c.Audience = ""
				return c
			}(),
			wantErr: "inbound_auth.audience",
		},
		{
			name: "missing jwks url",
			cfg: func() *InboundAuthCfg {
				c := valid()
				c.JWKSURL = ""
				return c
			}(),
			wantErr: "inbound_auth.jwks_url",
		},
		{
			name: "bad clock skew",
			cfg: func() *InboundAuthCfg {
				c := valid()
				c.ClockSkew = "soon"
				return c
			}(),
			wantErr: "inbound_auth.clock_skew",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestInboundAuthCfgToConfig(t *testing.T) {
	c := &InboundAuthCfg{
		Enabled:        true,
		Issuer:         "https://issuer.test",
		JWKSURL:        "https://issuer.test/jwks.json",
		Audience:       "engineering-analyst",
		RequiredScopes: []string{"engineering"},
		AllowedCallers: []string{"spiffe://openshell.local/sandbox/"},
		ClockSkew:      "45s",
	}
	got, err := c.toInboundAuth([]string{"/health"}, nil, nil)
	if err != nil {
		t.Fatalf("toInboundAuth: %v", err)
	}
	if got.Issuer != c.Issuer || got.JWKSURL != c.JWKSURL || got.Audience != c.Audience {
		t.Fatalf("basic fields not copied: %+v", got)
	}
	if got.ClockSkew.String() != "45s" {
		t.Fatalf("ClockSkew = %v, want 45s", got.ClockSkew)
	}
	if len(got.ExemptPaths) != 1 || got.ExemptPaths[0] != "/health" {
		t.Fatalf("ExemptPaths = %v", got.ExemptPaths)
	}
}

func TestLoadAgentConfig_DocumentServices(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantErr    string
		wantURL    string
		wantAbsent bool
	}{
		{
			name: "valid map",
			config: `document_services:
  finance: http://document-finance:8080
  hr: https://document-hr:8443
`,
			wantURL: "http://document-finance:8080",
		},
		{
			name: "no document services",
			config: `tools:
  allowed: []
`,
			wantAbsent: true,
		},
		{
			name: "invalid key",
			config: `document_services:
  Eng: http://document-eng:8080
`,
			wantErr: "Eng",
		},
		{
			name: "non-http scheme",
			config: `document_services:
  finance: ftp://document-finance:8080
`,
			wantErr: "finance",
		},
		{
			name: "relative url",
			config: `document_services:
  finance: /documents
`,
			wantErr: "finance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"), []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := loadAgentConfig(dir)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantAbsent {
				if len(cfg.DocumentServices) != 0 {
					t.Fatalf("DocumentServices = %v, want empty", cfg.DocumentServices)
				}
				return
			}
			if got := cfg.DocumentServices["finance"]; got != tt.wantURL {
				t.Fatalf("finance URL = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestLoadAgentConfig_A2APeers(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
		want    []A2APeer
	}{
		{
			name: "valid peers",
			config: `a2a_peers:
  - name: engineering-analyst
    url: https://engineering.example/a2a
    description: Analyzes engineering documents
    timeout_seconds: 30
  - name: finance-analyst
    url: http://finance.example:8080
`,
			want: []A2APeer{
				{Name: "engineering-analyst", URL: "https://engineering.example/a2a",
					Description: "Analyzes engineering documents", TimeoutSeconds: 30},
				{Name: "finance-analyst", URL: "http://finance.example:8080"},
			},
		},
		{name: "no peers", config: "tools:\n  allowed: []\n"},
		{
			name: "duplicate name",
			config: `a2a_peers:
  - name: analyst
    url: http://a.example
  - name: analyst
    url: http://b.example
`,
			wantErr: "duplicate",
		},
		{
			name: "bad name",
			config: `a2a_peers:
  - name: Analyst
    url: http://a.example
`,
			wantErr: "Analyst",
		},
		{
			name: "relative url",
			config: `a2a_peers:
  - name: analyst
    url: /a2a
`,
			wantErr: "analyst",
		},
		{
			name: "negative timeout",
			config: `a2a_peers:
  - name: analyst
    url: http://a.example
    timeout_seconds: -1
`,
			wantErr: "timeout_seconds",
		},
		{
			name: "oversized timeout",
			config: `a2a_peers:
  - name: analyst
    url: http://a.example
    timeout_seconds: 3601
`,
			wantErr: "timeout_seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"), []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := loadAgentConfig(dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.A2APeers) != len(tt.want) {
				t.Fatalf("peers = %+v, want %+v", cfg.A2APeers, tt.want)
			}
			for i := range tt.want {
				if cfg.A2APeers[i] != tt.want[i] {
					t.Fatalf("peer[%d] = %+v, want %+v", i, cfg.A2APeers[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoadAgentConfig_CardPublicKey(t *testing.T) {
	writeKey := func(t *testing.T, dir, name string) {
		t.Helper()
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		if err != nil {
			t.Fatalf("marshal key: %v", err)
		}
		data := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
	}
	writeConfig := func(t *testing.T, dir, keyPath string) {
		t.Helper()
		cfg := "a2a_peers:\n  - name: peer\n    url: http://peer.example\n    card_public_key: " + keyPath + "\n"
		if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"), []byte(cfg), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	t.Run("relative path resolves against config dir", func(t *testing.T) {
		dir := t.TempDir()
		writeKey(t, dir, "peer.pub")
		writeConfig(t, dir, "peer.pub")

		cfg, err := loadAgentConfig(dir)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if len(cfg.A2APeers) != 1 || cfg.A2APeers[0].CardKey == nil {
			t.Fatalf("card key not loaded: %+v", cfg.A2APeers)
		}
	})

	t.Run("missing key file fails at load", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, "missing.pub")
		if _, err := loadAgentConfig(dir); err == nil || !strings.Contains(err.Error(), "card_public_key") {
			t.Fatalf("err = %v, want a card_public_key error", err)
		}
	})

	t.Run("invalid PEM fails at load", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "bad.pub"), []byte("nope"), 0o600); err != nil {
			t.Fatalf("write bad key: %v", err)
		}
		writeConfig(t, dir, "bad.pub")
		if _, err := loadAgentConfig(dir); err == nil || !strings.Contains(err.Error(), "card_public_key") {
			t.Fatalf("err = %v, want a card_public_key error", err)
		}
	})
}
