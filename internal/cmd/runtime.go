package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/redhat-et/docsclaw/internal/bridge"
	"github.com/redhat-et/docsclaw/internal/config"
	"github.com/redhat-et/docsclaw/internal/egress"
	"github.com/redhat-et/docsclaw/internal/exec"
	"github.com/redhat-et/docsclaw/internal/fetchdoc"
	"github.com/redhat-et/docsclaw/internal/logger"
	"github.com/redhat-et/docsclaw/internal/mcpclient"
	"github.com/redhat-et/docsclaw/internal/memorytool"
	"github.com/redhat-et/docsclaw/internal/openaiapi"
	"github.com/redhat-et/docsclaw/internal/ragsearch"
	"github.com/redhat-et/docsclaw/internal/readfile"
	"github.com/redhat-et/docsclaw/internal/session"
	"github.com/redhat-et/docsclaw/internal/webfetch"
	"github.com/redhat-et/docsclaw/internal/writefile"
	"github.com/redhat-et/docsclaw/pkg/agentcontext"
	"github.com/redhat-et/docsclaw/pkg/inboundauth"
	"github.com/redhat-et/docsclaw/pkg/llm"
	"github.com/redhat-et/docsclaw/pkg/memory"
	"github.com/redhat-et/docsclaw/pkg/rag"
	"github.com/redhat-et/docsclaw/pkg/skills"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// runtimeOptions controls how buildRuntime assembles an agentRuntime.
type runtimeOptions struct {
	// Provider overrides the LLM provider (tests). Nil builds one from
	// cfg.LLM when an API key is configured.
	Provider llm.Provider
	// LogWriter sends structured logs to a custom writer (ask uses
	// stderr). Nil keeps the default logger's destination.
	LogWriter io.Writer
}

// agentRuntime holds everything serve and ask need to run a conversation.
type agentRuntime struct {
	Cfg          Config
	AgentCfg     *AgentConfig
	Provider     llm.Provider
	Registry     *tools.Registry
	LoopConfig   tools.LoopConfig
	SystemPrompt string
	AgentCard    *a2a.AgentCard
	Log          *logger.Logger

	promptVariants map[string]string
	basePrompt     string
	skillsSummary  string
	closers        []func() error
}

// loadConfig reads configuration and applies the LLM environment fallbacks.
func loadConfig() (Config, error) {
	var cfg Config
	if err := config.Load(v, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.ConfigDir == "" {
		cfg.ConfigDir = "/config/agent"
	}

	// Load LLM env var fallbacks (same pattern as existing agents)
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = os.Getenv("LLM_PROVIDER")
	}
	if cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = os.Getenv("LLM_API_KEY")
		if cfg.LLM.APIKey == "" {
			cfg.LLM.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = os.Getenv("LLM_BASE_URL")
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = os.Getenv("LLM_MODEL")
	}

	return cfg, nil
}

// buildRuntime loads the agent personality, tools, skills, and provider.
func buildRuntime(ctx context.Context, cfg Config, opts runtimeOptions) (rt *agentRuntime, err error) {
	var closers []func() error
	defer func() {
		if err != nil && rt == nil {
			for i := len(closers) - 1; i >= 0; i-- {
				_ = closers[i]()
			}
		}
	}()
	systemPrompt, err := loadSystemPrompt(cfg.ConfigDir)
	if err != nil {
		return nil, err
	}

	agentCard, err := loadAgentCard(cfg.ConfigDir)
	if err != nil {
		return nil, err
	}

	promptVariants, err := loadPromptVariants(cfg.ConfigDir)
	if err != nil {
		return nil, err
	}

	// Load agent config (optional — nil means phase 1 mode)
	agentCfg, err := loadAgentConfig(cfg.ConfigDir)
	if err != nil {
		return nil, err
	}

	// Resolve workspace path: agent-config.yaml > --workspace flag/env > default
	var cfgWorkspace string
	if agentCfg != nil {
		cfgWorkspace = agentCfg.Tools.Workspace
	}
	workspace := resolveWorkspace(cfgWorkspace, cfg.Workspace)

	var toolRegistry *tools.Registry
	var loopCfg tools.LoopConfig
	var skillsSummary string

	if agentCfg != nil {
		toolRegistry = tools.NewRegistry(agentCfg.Tools.Allowed)

		if err := os.MkdirAll(workspace, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspace: %w", err)
		}
		systemPrompt += fmt.Sprintf(
			"\n\nYour workspace directory is %s. Always write files there.", workspace)

		toolRegistry.Register(exec.NewExecTool(exec.ExecConfig{
			Timeout:   agentCfg.Tools.Exec.Timeout,
			MaxOutput: agentCfg.Tools.Exec.MaxOutput,
		}))
		toolRegistry.Register(webfetch.NewWebFetchTool(webfetch.WebFetchConfig{
			AllowedHosts: agentCfg.Tools.WebFetch.AllowedHosts,
		}))
		toolRegistry.Register(readfile.NewReadFileTool(workspace))
		toolRegistry.Register(writefile.NewWriteFileTool(workspace))
		toolRegistry.Register(memorytool.NewRememberTool(
			memory.NewFileStore(filepath.Join(workspace, "MEMORY.md")),
		))

		loopCfg = agentCfg.toLoopConfig()

		if len(agentCfg.Tools.MCP) > 0 {
			mcpMgr, err := mcpclient.NewManager(ctx, agentCfg.Tools.MCP)
			if err != nil {
				return nil, fmt.Errorf("failed to connect to MCP servers: %w", err)
			}
			closers = append(closers, mcpMgr.Close)

			for _, t := range mcpMgr.Tools() {
				toolRegistry.RegisterAlwaysAllowed(t)
			}
		}
	}

	log := newRuntimeLogger(opts.LogWriter)
	slog.SetDefault(log.Logger)

	// Load workspace context using the selected profile (works in both phases)
	profile, known := agentcontext.ResolveProfile(cfg.WorkspaceProfile)
	if cfg.WorkspaceProfile != "" && !known {
		log.Warn("unknown workspace profile, using docsclaw",
			"profile", cfg.WorkspaceProfile)
	}
	systemPrompt += agentcontext.NewLoader().Load(workspace, profile)

	// Load OS tool inventory and inject into system prompt
	const toolsJSONPath = "/etc/docsclaw/tools.json"
	tj, err := loadToolsJSON(toolsJSONPath)
	if err != nil {
		log.Warn("failed to load tools.json", "error", err)
	}
	if tj != nil {
		var names []string
		for _, t := range tj.Tools {
			clean := sanitizeToolName(t.Name)
			if clean != "" {
				names = append(names, clean)
			}
		}
		systemPrompt += fmt.Sprintf(
			"\n\nAvailable OS tools: %s\nDo NOT attempt to use tools not in this list.",
			strings.Join(names, ", "),
		)
		log.Info("loaded OS tool inventory", "tools", names, "risk_score", tj.RiskScore)
	}

	// Initialize LLM provider
	var llmProvider llm.Provider
	if opts.Provider != nil {
		llmProvider = opts.Provider
		log.Info("LLM provider initialized",
			"provider", llmProvider.ProviderName(),
			"model", llmProvider.Model())
	} else if cfg.LLM.APIKey != "" {
		llmProvider, err = llm.NewProvider(cfg.LLM)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM provider: %w", err)
		}
		log.Info("LLM provider initialized",
			"provider", llmProvider.ProviderName(),
			"model", llmProvider.Model())
	} else {
		log.Warn("LLM API key not configured - will use mock responses")
	}

	// Discover skills (runs in both phase 1 and phase 2)
	// Priority: CLI flag → agent-config.yaml → default (<config-dir>/skills)
	skillsDir := cfg.SkillsDir
	if skillsDir == "" && agentCfg != nil {
		skillsDir = agentCfg.SkillsDir
	}
	if skillsDir == "" {
		skillsDir = filepath.Join(cfg.ConfigDir, "skills")
	}
	discoveredSkills, err := skills.Discover(skillsDir)
	if err != nil {
		log.Warn("Failed to load skills", "error", err)
	}
	if len(discoveredSkills) > 0 {
		agentSkills := skills.ToAgentSkills(discoveredSkills)
		agentCard.Skills = skills.MergeSkills(agentCard.Skills, agentSkills)

		log.Info("Skills discovered",
			"count", len(discoveredSkills),
			"names", skillNames(discoveredSkills))
	}

	// Register additional tools and skills when in phase 2 mode
	if toolRegistry != nil {
		if agentCfg.RAG != nil {
			ragClient, err := rag.NewClient(agentCfg.RAG)
			if err != nil {
				return nil, fmt.Errorf("rag: %w", err)
			}
			toolRegistry.RegisterAlwaysAllowed(ragsearch.NewRAGSearchTool(
				ragClient, agentCfg.RAG))
			log.Info("RAG search enabled",
				"backend", agentCfg.RAG.Backend,
				"collection", agentCfg.RAG.Collection)
		}

		// Register fetch_document tool for configured services
		if len(agentCfg.DocumentServices) > 0 {
			services := make([]string, 0, len(agentCfg.DocumentServices))
			for name := range agentCfg.DocumentServices {
				services = append(services, name)
			}
			sort.Strings(services)

			toolRegistry.Register(fetchdoc.New(
				agentCfg.DocumentServices,
				egress.NewClient(30*time.Second),
				log.Logger,
			))
			log.Info("fetch_document enabled", "services", services)
		}

		// Register skill loading tool in phase 2
		if len(discoveredSkills) > 0 {
			skillsSummary = skills.BuildSummary(discoveredSkills)

			toolRegistry.RegisterAlwaysAllowed(&loadSkillTool{
				skillsDir: skillsDir,
			})
		}

		log.Info("Tools enabled",
			"allowed", len(toolRegistry.Definitions()),
			"max_iterations", loopCfg.MaxIterations)
	}

	return &agentRuntime{
		Cfg:            cfg,
		AgentCfg:       agentCfg,
		Provider:       llmProvider,
		Registry:       toolRegistry,
		LoopConfig:     loopCfg,
		SystemPrompt:   systemPrompt + skillsSummary,
		AgentCard:      agentCard,
		Log:            log,
		promptVariants: promptVariants,
		basePrompt:     systemPrompt,
		skillsSummary:  skillsSummary,
		closers:        closers,
	}, nil
}

// newRuntimeLogger keeps serve's existing logger while letting ask send
// logs to stderr.
func newRuntimeLogger(w io.Writer) *logger.Logger {
	if w == nil {
		return logger.New(logger.ComponentAgent)
	}
	return logger.NewWithWriter(logger.ComponentAgent, w, false)
}

// Run executes one conversation: the tool loop when a registry is
// configured, otherwise a single completion.
func (r *agentRuntime) Run(ctx context.Context, messages []llm.Message,
	hook tools.Hook) (string, error) {

	if r.Registry != nil {
		if r.Provider == nil {
			return "", errors.New("LLM provider required for tool-use mode")
		}
		loopCfg := r.LoopConfig
		loopCfg.Hook = hook
		result, err := tools.RunToolLoop(ctx, r.Provider, messages, r.Registry, loopCfg)
		if err != nil {
			return "", fmt.Errorf("agentic loop failed: %w", err)
		}
		r.Log.Success("Agentic processing completed")
		return result, nil
	}

	userText := lastUserContent(messages)
	if r.Provider == nil {
		return "## Result\n\nMock response. Configure LLM_API_KEY to enable AI processing.", nil
	}
	selectedPrompt := selectPrompt(r.basePrompt, r.promptVariants, userText) + r.skillsSummary
	result, err := r.Provider.Complete(ctx, selectedPrompt, userText)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	r.Log.Success("LLM processing completed")
	return result, nil
}

// Close releases resources opened by buildRuntime.
func (r *agentRuntime) Close() error {
	var firstErr error
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i](); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// newServeHandler builds the HTTP mux and the session store.
func newServeHandler(rt *agentRuntime) (http.Handler, func() error, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	// A2A setup — ensure at least one interface is declared
	if len(rt.AgentCard.SupportedInterfaces) == 0 {
		rt.AgentCard.SupportedInterfaces = []*a2a.AgentInterface{
			a2a.NewAgentInterface(
				fmt.Sprintf("http://localhost:%d", rt.Cfg.Service.Port),
				a2a.TransportProtocolJSONRPC,
			),
		}
	}

	executor := &bridge.AgentExecutor{
		Log:          rt.Log,
		SystemPrompt: rt.SystemPrompt,
		ProcessMessage: func(ctx context.Context, messages []llm.Message) (string, error) {
			return rt.Run(ctx, messages, nil)
		},
	}

	var closers []func() error

	// In phase 2 mode, enable free-form message handling with sessions
	if rt.Registry != nil {
		sessionDB := rt.Cfg.SessionDB
		if sessionDB == "" {
			sessionDB = os.Getenv("DOCSCLAW_SESSION_DB")
		}
		if sessionDB == "" {
			sessionDB = "memory"
		}

		var sessions session.SessionStore
		if sessionDB == "memory" {
			sessions = session.NewMemoryStore(30 * time.Minute)
			rt.Log.Info("Session store", "backend", "memory")
		} else {
			if err := os.MkdirAll(filepath.Dir(sessionDB), 0700); err != nil {
				return nil, nil, fmt.Errorf("failed to create session db directory: %w", err)
			}
			sqliteStore, err := session.NewSQLiteStore(sessionDB, 30*time.Minute)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to open session database: %w", err)
			}
			if err := os.Chmod(sessionDB, 0600); err != nil {
				return nil, nil, fmt.Errorf("failed to set session db permissions: %w", err)
			}
			closers = append(closers, sqliteStore.Close)
			sessions = sqliteStore
			rt.Log.Info("Session store", "backend", "sqlite", "path", sessionDB)
		}

		reaperCtx, reaperCancel := context.WithCancel(context.Background())
		closers = append(closers, func() error {
			reaperCancel()
			return nil
		})
		go sessions.StartReaper(reaperCtx)

		executor.Sessions = sessions
	}

	a2aHandler := a2asrv.NewHandler(executor)
	jsonrpcHandler := a2asrv.NewJSONRPCHandler(a2aHandler)
	dynamicCardHandler := bridge.DynamicCardHandler(rt.AgentCard, rt.Log.Logger)
	cardHandler := bridge.SignedCardHandler(
		os.Getenv("AGENT_CARD_SIGNED_PATH"),
		dynamicCardHandler,
		rt.Log.Logger,
	)
	mux.Handle("GET /.well-known/agent-card.json", cardHandler)
	mux.Handle("POST /a2a", jsonrpcHandler)
	mux.Handle("POST /{$}", jsonrpcHandler)

	// OpenAI-compatible API
	openaiHandler := &openaiapi.Handler{
		Provider:     rt.Provider,
		SystemPrompt: rt.SystemPrompt,
		Registry:     rt.Registry,
		LoopConfig:   rt.LoopConfig,
		AgentCard:    rt.AgentCard,
		AgentName:    rt.AgentCard.Name,
	}
	mux.HandleFunc("POST /v1/chat/completions", openaiHandler.ChatCompletion)
	mux.HandleFunc("GET /v1/models", openaiHandler.Models)
	mux.HandleFunc("GET /v1/skills", openaiHandler.Skills)

	closeHandler := func() error {
		var firstErr error
		for i := len(closers) - 1; i >= 0; i-- {
			if err := closers[i](); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}

	var handler http.Handler = mux
	if auth := rt.AgentCfg; auth != nil && auth.InboundAuth != nil && auth.InboundAuth.Enabled {
		authCfg, err := auth.InboundAuth.toInboundAuth(
			[]string{"/.well-known/agent-card.json", "/health"},
			egress.NewClient(10*time.Second),
			rt.Log.Logger,
		)
		if err != nil {
			_ = closeHandler()
			return nil, nil, fmt.Errorf("inbound auth: %w", err)
		}
		// Log identity below the middleware so it covers A2A and the whole
		// /v1 tree; exempt paths carry no claims and stay silent.
		handler = identityLoggingHandler(rt.Log, mux)
		handler, err = inboundauth.Middleware(authCfg, handler)
		if err != nil {
			_ = closeHandler()
			return nil, nil, fmt.Errorf("inbound auth: %w", err)
		}
		rt.Log.Info("inbound auth enabled",
			"issuer", authCfg.Issuer, "audience", authCfg.Audience)
	} else {
		rt.Log.Warn("inbound auth disabled")
	}

	return handler, closeHandler, nil
}

// identityLoggingHandler logs the verified caller for every authenticated
// request, whichever endpoint it hits.
func identityLoggingHandler(log *logger.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, ok := inboundauth.ClaimsFrom(r.Context()); ok {
			log.Info("request authenticated",
				"sub", claims.Subject,
				"azp", claims.AZP,
				"scope", strings.Join(claims.Scopes, " "),
				"aud", strings.Join(claims.Audience, " "))
		}
		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// lastUserContent returns the text of the most recent user message.
func lastUserContent(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// loadSkillTool implements the load_skill tool.
type loadSkillTool struct {
	skillsDir string
}

func (t *loadSkillTool) Name() string { return "load_skill" }
func (t *loadSkillTool) Description() string {
	return "Load the full instructions for a skill by name. " +
		"Use this when you need specialized guidance for a task."
}
func (t *loadSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The skill name to load",
			},
		},
		"required": []string{"name"},
	}
}
func (t *loadSkillTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return tools.Errorf("skill name is required")
	}
	content, err := skills.LoadContent(t.skillsDir, name)
	if err != nil {
		return tools.Errorf("%s", err)
	}
	return tools.OK(content)
}

func skillNames(discovered []skills.SkillMeta) string {
	names := make([]string, len(discovered))
	for i, s := range discovered {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}
