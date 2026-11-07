package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/redhat-et/docsclaw/internal/mcpclient"
	"github.com/redhat-et/docsclaw/pkg/inboundauth"
	"github.com/redhat-et/docsclaw/pkg/rag"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// documentServiceName matches the allowed document_services keys.
var documentServiceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// AgentConfig holds the agent-config.yaml configuration.
type AgentConfig struct {
	Tools            ToolsConfig       `yaml:"tools"`
	Loop             LoopCfg           `yaml:"loop"`
	RAG              *rag.Config       `yaml:"rag"`
	SkillsDir        string            `yaml:"skills_dir"`
	DocumentServices map[string]string `yaml:"document_services"`
	InboundAuth      *InboundAuthCfg   `yaml:"inbound_auth"`
}

// InboundAuthCfg configures verification of incoming bearer tokens.
type InboundAuthCfg struct {
	Enabled        bool     `yaml:"enabled"`
	Issuer         string   `yaml:"issuer"`
	JWKSURL        string   `yaml:"jwks_url"`
	Audience       string   `yaml:"audience"`
	RequiredScopes []string `yaml:"required_scopes"`
	AllowedCallers []string `yaml:"allowed_callers"`
	ClockSkew      string   `yaml:"clock_skew"`
}

// validate rejects an enabled block that would silently run without auth.
func (c *InboundAuthCfg) validate() error {
	if c == nil || !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Issuer) == "" {
		return fmt.Errorf("inbound_auth.issuer is required when inbound auth is enabled")
	}
	if strings.TrimSpace(c.Audience) == "" {
		return fmt.Errorf("inbound_auth.audience is required when inbound auth is enabled")
	}
	if strings.TrimSpace(c.JWKSURL) == "" {
		return fmt.Errorf("inbound_auth.jwks_url is required when inbound auth is enabled")
	}
	if c.ClockSkew != "" {
		if _, err := time.ParseDuration(c.ClockSkew); err != nil {
			return fmt.Errorf("inbound_auth.clock_skew: %w", err)
		}
	}
	return nil
}

// toInboundAuth converts the YAML block into the middleware's config.
func (c *InboundAuthCfg) toInboundAuth(exempt []string, client *http.Client,
	log *slog.Logger) (inboundauth.Config, error) {
	if c == nil {
		return inboundauth.Config{}, fmt.Errorf("inbound auth config is nil")
	}
	cfg := inboundauth.Config{
		Issuer:         c.Issuer,
		JWKSURL:        c.JWKSURL,
		Audience:       c.Audience,
		RequiredScopes: c.RequiredScopes,
		AllowedCallers: c.AllowedCallers,
		ExemptPaths:    exempt,
		HTTPClient:     client,
		Logger:         log,
	}
	if c.ClockSkew != "" {
		skew, err := time.ParseDuration(c.ClockSkew)
		if err != nil {
			return inboundauth.Config{}, fmt.Errorf("inbound_auth.clock_skew: %w", err)
		}
		cfg.ClockSkew = skew
	}
	return cfg, nil
}

// resolveHost picks the bind address. An explicit host always wins; an empty
// host means loopback when inbound auth is on and all interfaces otherwise.
func resolveHost(configured string, authEnabled bool) string {
	if configured != "" {
		return configured
	}
	if authEnabled {
		return "127.0.0.1"
	}
	return "0.0.0.0"
}

// ToolsConfig configures tool availability and behavior.
type ToolsConfig struct {
	Allowed   []string                    `yaml:"allowed"`
	Exec      ExecToolConfig              `yaml:"exec"`
	WebFetch  WebFetchCfg                 `yaml:"web_fetch"`
	Workspace string                      `yaml:"workspace"`
	MCP       []mcpclient.MCPServerConfig `yaml:"mcp"`
}

// ExecToolConfig configures the exec tool.
type ExecToolConfig struct {
	Timeout   int `yaml:"timeout"`
	MaxOutput int `yaml:"max_output"`
}

// WebFetchCfg configures the web_fetch tool.
type WebFetchCfg struct {
	AllowedHosts []string `yaml:"allowed_hosts"`
}

// LoopCfg configures the agentic loop.
type LoopCfg struct {
	MaxIterations int `yaml:"max_iterations"`
}

// loadAgentConfig reads agent-config.yaml from the config directory.
// Returns nil if the file does not exist (phase 1 mode).
func loadAgentConfig(configDir string) (*AgentConfig, error) {
	path := filepath.Join(configDir, "agent-config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Phase 1 mode
		}
		return nil, fmt.Errorf("failed to read agent config: %w", err)
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse agent config: %w", err)
	}
	if err := validateDocumentServices(cfg.DocumentServices); err != nil {
		return nil, err
	}
	if err := cfg.InboundAuth.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateDocumentServices(services map[string]string) error {
	for name, rawURL := range services {
		if !documentServiceName.MatchString(name) {
			return fmt.Errorf("document_services[%q]: name must match %s", name, documentServiceName)
		}
		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("document_services[%q]: invalid URL: %w", name, err)
		}
		if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("document_services[%q]: must be an absolute http or https URL", name)
		}
	}
	return nil
}

// toLoopConfig converts AgentConfig to the tools.LoopConfig.
func (c *AgentConfig) toLoopConfig() tools.LoopConfig {
	cfg := tools.DefaultLoopConfig()
	if c.Loop.MaxIterations > 0 {
		cfg.MaxIterations = c.Loop.MaxIterations
	}
	return cfg
}
