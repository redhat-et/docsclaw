package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/internal/config"
	_ "github.com/redhat-et/docsclaw/internal/metrics"
	"github.com/redhat-et/docsclaw/internal/telemetry"
	"github.com/redhat-et/docsclaw/pkg/llm"
	"github.com/redhat-et/docsclaw/pkg/manifest"
)

// loadSystemPrompt reads the system prompt from config-dir/system-prompt.txt.
// Returns an error if the file does not exist.
func loadSystemPrompt(configDir string) (string, error) {
	path := filepath.Join(configDir, "system-prompt.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read system prompt: %w", err)
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("system prompt is empty: %s", path)
	}
	return prompt, nil
}

// loadAgentCard reads the agent card from config-dir/agent-card.json.
// If the file does not exist, returns a minimal fallback card.
func loadAgentCard(configDir string) (*a2a.AgentCard, error) {
	path := filepath.Join(configDir, "agent-card.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &a2a.AgentCard{
				Name:               "docsclaw",
				Description:        "DocsClaw Agent",
				Version:            "1.0.0",
				Capabilities:       a2a.AgentCapabilities{},
				DefaultInputModes:  []string{"application/json"},
				DefaultOutputModes: []string{"text/plain"},
			}, nil
		}
		return nil, fmt.Errorf("failed to read agent card: %w", err)
	}

	var card a2a.AgentCard
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, fmt.Errorf("failed to parse agent card: %w", err)
	}
	return &card, nil
}

// loadPromptVariants reads optional prompt variants from
// config-dir/prompts.json. Returns an empty map if the file
// does not exist.
func loadPromptVariants(configDir string) (map[string]string, error) {
	path := filepath.Join(configDir, "prompts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("failed to read prompt variants: %w", err)
	}

	var variants map[string]string
	if err := json.Unmarshal(data, &variants); err != nil {
		return nil, fmt.Errorf("failed to parse prompt variants: %w", err)
	}
	return variants, nil
}

// loadToolsJSON reads the OS tool inventory from /etc/docsclaw/tools.json.
// Returns nil if the file does not exist (backward compatible).
func loadToolsJSON(path string) (*manifest.ToolsJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tools.json: %w", err)
	}
	var tj manifest.ToolsJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse tools.json: %w", err)
	}
	return &tj, nil
}

const defaultWorkspace = "/workspace"

var toolNameAllowed = regexp.MustCompile(`[^a-zA-Z0-9 _-]`)

func sanitizeToolName(name string) string {
	return toolNameAllowed.ReplaceAllString(name, "")
}

// selectPrompt picks the appropriate prompt based on message content.
// If a keyword from the variants map appears in the message
// (case-insensitive), the variant prompt is returned. Otherwise,
// the default prompt is returned.
func selectPrompt(defaultPrompt string, variants map[string]string, message string) string {
	if len(variants) == 0 {
		return defaultPrompt
	}
	lower := strings.ToLower(message)
	for keyword, prompt := range variants {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return prompt
		}
	}
	return defaultPrompt
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the docsclaw service",
	Long:  "Start the docsclaw service with ConfigMap-driven personality.",
	RunE:  runServe,
}

// addAgentFlags defines the flags shared by serve and ask.
func addAgentFlags(cmd *cobra.Command) {
	cmd.Flags().String("config-dir", "/config/agent",
		"Directory containing system-prompt.txt, agent-card.json, and optional prompts.json")
	cmd.Flags().String("skills-dir", "",
		"Directory containing skill subdirectories (default: <config-dir>/skills)")
	cmd.Flags().String("llm-provider", "", "LLM provider (anthropic, openai, litellm)")
	cmd.Flags().String("llm-api-key", "", "LLM API key")
	cmd.Flags().String("llm-base-url", "", "Base URL for OpenAI-compatible APIs")
	cmd.Flags().String("llm-model", "", "LLM model to use")
	cmd.Flags().Int("llm-max-tokens", 4096, "Max tokens for LLM response")
	cmd.Flags().Int("llm-timeout", 45, "LLM request timeout in seconds")
	cmd.Flags().String("workspace", "",
		"Workspace directory path (default: /workspace)")
	cmd.Flags().String("workspace-profile", "docsclaw",
		"Workspace context profile: docsclaw, openclaw, or hermes")
	cmd.Flags().String("session-db", "",
		"Session database backend ('memory' for in-memory, or a file path for SQLite; default: memory)")
}

// bindAgentFlags binds the executed command's agent flags to viper. serve
// binds once at init; ask binds at execution time so the two commands do
// not fight over the same viper keys.
func bindAgentFlags(cmd *cobra.Command) {
	_ = v.BindPFlag("config_dir", cmd.Flags().Lookup("config-dir"))
	_ = v.BindPFlag("workspace", cmd.Flags().Lookup("workspace"))
	_ = v.BindPFlag("workspace_profile", cmd.Flags().Lookup("workspace-profile"))
	_ = v.BindPFlag("skills_dir", cmd.Flags().Lookup("skills-dir"))
	_ = v.BindPFlag("llm.provider", cmd.Flags().Lookup("llm-provider"))
	_ = v.BindPFlag("llm.api_key", cmd.Flags().Lookup("llm-api-key"))
	_ = v.BindPFlag("llm.base_url", cmd.Flags().Lookup("llm-base-url"))
	_ = v.BindPFlag("llm.model", cmd.Flags().Lookup("llm-model"))
	_ = v.BindPFlag("llm.max_tokens", cmd.Flags().Lookup("llm-max-tokens"))
	_ = v.BindPFlag("llm.timeout_seconds", cmd.Flags().Lookup("llm-timeout"))
	_ = v.BindPFlag("session_db", cmd.Flags().Lookup("session-db"))
}

func init() {
	rootCmd.AddCommand(serveCmd)
	addAgentFlags(serveCmd)
	bindAgentFlags(serveCmd)
}

// Config holds docsclaw configuration.
type Config struct {
	config.CommonConfig `mapstructure:",squash"`
	ConfigDir           string     `mapstructure:"config_dir"`
	SkillsDir           string     `mapstructure:"skills_dir"`
	Workspace           string     `mapstructure:"workspace"`
	WorkspaceProfile    string     `mapstructure:"workspace_profile"`
	LLM                 llm.Config `mapstructure:"llm"`
	SessionDB           string     `mapstructure:"session_db"`
}

func resolveWorkspace(cfgWorkspace, flagWorkspace string) string {
	if v := strings.TrimSpace(cfgWorkspace); v != "" {
		return v
	}
	if v := strings.TrimSpace(flagWorkspace); v != "" {
		return v
	}
	return defaultWorkspace
}

// startAgent loads config from configDir and validates it.
// This function is separated from runServe for testability.
func startAgent(configDir string, _ llm.Provider) error {
	_, err := loadSystemPrompt(configDir)
	if err != nil {
		return err
	}
	return nil
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	rt, err := buildRuntime(context.Background(), cfg, runtimeOptions{
		LogWriter: os.Stdout,
	})
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close() }()
	log := rt.Log

	// Initialize OpenTelemetry tracing (no-op when disabled)
	otelShutdown, err := telemetry.Init(context.Background(), telemetry.Config{
		Enabled:           cfg.OTel.Enabled,
		CollectorEndpoint: cfg.OTel.CollectorEndpoint,
		StdoutExporter:    cfg.OTel.StdoutExporter,
		ServiceVersion:    version,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
	}

	handler, closeHandler, err := newServeHandler(rt)
	if err != nil {
		return err
	}
	defer func() { _ = closeHandler() }()

	// Main server — WriteTimeout is 0 because SSE streaming
	// (OpenAI chat completions) and agentic tool loops can exceed
	// any fixed deadline. Request-scoped contexts handle cancellation.
	h := handler
	if cfg.OTel.Enabled {
		h = telemetry.HTTPMiddleware(h)
	}
	server := &http.Server{
		Addr:        cfg.Service.Addr(),
		Handler:     h,
		ReadTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	done := make(chan bool)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Info("Shutting down docsclaw...")
		serverCtx, serverCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer serverCancel()
		if err := server.Shutdown(serverCtx); err != nil {
			log.Error("Shutdown error", "error", err)
		}
		otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer otelCancel()
		if err := otelShutdown(otelCtx); err != nil {
			log.Error("OTel shutdown error", "error", err)
		}
		close(done)
	}()

	// Startup logging
	log.Section("STARTING DOCSCLAW")
	log.Info("Agent", "name", rt.AgentCard.Name)
	log.Info("Listening", "addr", cfg.Service.Addr())
	log.Info("Health server", "addr", cfg.Service.HealthAddr())
	log.Info("Config dir", "path", cfg.ConfigDir)
	if len(rt.promptVariants) > 0 {
		keys := make([]string, 0, len(rt.promptVariants))
		for k := range rt.promptVariants {
			keys = append(keys, k)
		}
		log.Info("Prompt variants loaded", "variants", strings.Join(keys, ", "))
	}
	if rt.Provider != nil {
		log.Info("LLM enabled",
			"provider", rt.Provider.ProviderName(),
			"model", rt.Provider.Model())
	}
	log.Info("OpenAI-compatible API enabled",
		"endpoints", "/v1/chat/completions, /v1/models, /v1/skills")

	// Health server (plain HTTP for K8s probes)
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", healthHandler)
	healthMux.HandleFunc("/ready", healthHandler)
	healthMux.Handle("/metrics", promhttp.Handler())
	healthServer := &http.Server{
		Addr:         cfg.Service.HealthAddr(),
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Health server error", "error", err)
		}
	}()

	serverErr := server.ListenAndServe()
	if serverErr != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", serverErr)
	}

	<-done
	log.Info("docsclaw stopped")
	return nil
}
