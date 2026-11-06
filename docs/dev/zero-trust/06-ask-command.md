# WP06: One-shot `ask` command implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` first. WP03-WP05 must be merged.

**Goal:** `docsclaw ask --config-dir DIR "prompt"` runs the same agent setup
as `serve`, prints the final answer to stdout, and exits non-zero on
failure.

**Architecture:** Extract everything in `runServe` up to "Build HTTP mux"
into `buildRuntime`, which returns an `agentRuntime`. `serve` and `ask` both
call it. `--trace` is a `tools.Hook` that writes to stderr. WP10 and WP12
build on `newServeHandler`.

**Tech stack:** Go, Cobra/Viper, `pkg/tools.Hook`.

**Spec:** parent plan sections 1.5 and 1.6.

## Review focus

- No LLM key configured: `ask` must exit non-zero with `LLM API key not
  configured` on stderr. Mock text on stdout would look like success to a
  demo script.
- Logs must go to stderr, never stdout. A script does `answer=$(docsclaw ask ...)`.
- `LLM_API_KEY=openshell-placeholder-xyz` (no `sk-` prefix, odd length)
  must be accepted by both providers without format validation.
- Ctrl-C during the loop: the context is cancelled, the exit code is
  non-zero, and no partial answer goes to stdout.
- An empty prompt argument, or more than one positional argument: usage
  error, exit code 2.

---

### Task 1: Extract `buildRuntime` (refactor, no behavior change)

**Files:**

- Create: `internal/cmd/runtime.go`, `internal/cmd/runtime_test.go`
- Modify: `internal/cmd/serve.go`

**Interfaces:**

- Produces:

```go
type runtimeOptions struct {
    Provider  llm.Provider // optional override (tests); nil means build from cfg.LLM
    LogWriter io.Writer    // default os.Stderr
}

type agentRuntime struct {
    Cfg          Config
    AgentCfg     *AgentConfig      // nil in single-shot mode
    Provider     llm.Provider      // nil when no API key and no override
    Registry     *tools.Registry   // nil in single-shot mode
    LoopConfig   tools.LoopConfig
    SystemPrompt string            // includes workspace context and skills summary
    AgentCard    *a2a.AgentCard
    Log          *logger.Logger
    closers      []func() error
}

func buildRuntime(ctx context.Context, cfg Config, opts runtimeOptions) (*agentRuntime, error)
func (r *agentRuntime) Close() error
// Run executes one conversation: tool loop if Registry != nil, else a single Complete.
func (r *agentRuntime) Run(ctx context.Context, messages []llm.Message, hook tools.Hook) (string, error)
func loadConfig() (Config, error) // config.Load plus LLM env fallbacks (moved from runServe)
```

- Produces: `func newServeHandler(rt *agentRuntime) (http.Handler, func() error, error)`.
  It builds the mux (health, agent card, A2A, `/v1`) and the session store.
  The returned func closes the session store and reaper. `runServe` becomes:
  `loadConfig`, then `buildRuntime`, `newServeHandler`, telemetry, the
  servers, and shutdown.
- `executor.ProcessMessage` and `openaiapi.Handler` both call `rt.Run`, so
  there is one code path for the loop.

- [ ] **Step 1: Write failing tests** in `runtime_test.go` with a fake
  `llm.Provider` (define `fakeProvider` in the test file. It records every
  `Complete`/`Chat` input and returns scripted responses. WP12 reuses it, so
  put it in `internal/cmd/fakeprovider_test.go`):
  - `TestBuildRuntime_SingleShot`: use `testdata/standalone` (no
    `agent-config.yaml`). Assert `Registry == nil`, and that `Run` calls
    `Complete` once with the system prompt.
  - `TestBuildRuntime_ToolMode`: use `testdata/research-agent`, with the
    workspace set to `t.TempDir()`. Assert `Registry != nil` and that
    `fetch_document` is registered when `document_services` is set.
  - `TestNewServeHandler_Routes`: use `httptest`. `GET
    /.well-known/agent-card.json` returns 200; `GET /health` returns 200;
    `POST /a2a` with a `message/send` JSON-RPC body returns the fake
    provider's text.
- [ ] **Step 2:** Run `go test ./internal/cmd/ -run 'BuildRuntime|NewServeHandler' -v`.
  Expected: FAIL.
- [ ] **Step 3:** Move the code. Keep the startup log lines in `runServe`.
  Remove the now-redundant `startAgent` helper if only tests use it, and
  update those tests.
- [ ] **Step 4:** Run `go test ./internal/cmd/ -v` and also run the existing
  `serve_test.go` and `serve_tools_test.go`. Expected: PASS.
- [ ] **Step 5:** Commit: `refactor(cmd): extract agent runtime from serve`.

### Task 2: `ask` command and `--trace`

**Files:**

- Create: `internal/cmd/ask.go`, `internal/cmd/ask_test.go`,
  `internal/cmd/trace.go`

**Interfaces:**

- Produces: the `askCmd` Cobra command `ask [flags] PROMPT`, with
  `cobra.ExactArgs(1)`. It shares the persistent LLM/config/workspace flags
  with `serve`. Move these flag definitions to a helper
  `addAgentFlags(cmd *cobra.Command)` that both commands call, and bind them
  once.
- Produces: `type traceHook struct{ w io.Writer }`, which implements
  `tools.Hook`. `BeforeToolCall` writes `→ <tool> <compact JSON args>` and
  returns `true, ""`. `AfterToolCall` writes `← <tool> ok|error: <first 200
  bytes of output, newlines collapsed>`.
- Behavior: the prompt becomes a user message after the system message. The
  answer goes to `cmd.OutOrStdout()` with a trailing newline. Errors are
  returned from `RunE`, so Cobra exits 1. If `rt.Provider == nil`, return
  `errors.New("LLM API key not configured")`. Use `signal.NotifyContext`
  for SIGINT/SIGTERM.

- [ ] **Step 1: Write failing tests** in `ask_test.go`. Run the command via
  `rootCmd.SetArgs`, `SetOut`, and `SetErr`, with the provider injected
  through a package-level `askRuntimeOptions` variable that tests override:
  - `TestAsk_PrintsAnswerOnly`: stdout equals `"answer\n"` exactly, and
    stderr has no answer text.
  - `TestAsk_NoProviderFails`: the error is non-nil, the message is `LLM
    API key not configured`, and stdout is empty.
  - `TestAsk_TraceWritesToolCalls`: the fake provider first returns a
    `fetch_document` tool call (against an `httptest` doc server), then
    text. With `--trace`, stderr contains `→ fetch_document` and `←
    fetch_document ok`, and stdout contains only the final text.
  - `TestAsk_ArgCount`: zero args and two args both give errors.
  - `TestAsk_LoopErrorNonZero`: the provider returns an error. Expect a
    non-nil error and empty stdout.
- [ ] **Step 2:** Run `go test ./internal/cmd/ -run TestAsk -v`. Expected:
  FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the tests. Expected: PASS. Then run manually:
  `go run ./cmd/docsclaw ask --config-dir testdata/standalone "hi"` with no
  key. Expected: exit code 1 and the message on stderr.
- [ ] **Step 5:** Commit: `feat(cmd): add one-shot ask command with --trace`.

### Task 3: Placeholder keys and OpenAI-compatible endpoint (step 1.6)

**Files:**

- Test: `internal/openai/openai_test.go`, `internal/anthropic/anthropic_test.go`
- Modify: `deploy/README.md`

- [ ] **Step 1: Write tests** `TestProvider_AcceptsPlaceholderKey` for each
  provider: build it with `APIKey: "openshell-placeholder-xyz"` and
  `BaseURL` set to an `httptest` server. The server asserts it received
  `Authorization: Bearer openshell-placeholder-xyz` (OpenAI) or `x-api-key:
  openshell-placeholder-xyz` (Anthropic) and returns a minimal valid
  completion. Assert there is no constructor error and the text comes back.
  For OpenAI, also assert the request path is `<base>/chat/completions`
  (MaaS compatibility).
- [ ] **Step 2:** Run the tests. If one fails because of key-format
  validation, remove that validation. Only "non-empty" may be checked.
- [ ] **Step 3:** In `deploy/README.md`, add a "Running in an OpenShell
  sandbox" section: a table of `LLM_PROVIDER`, `LLM_API_KEY` (placeholder,
  substituted by the proxy), `LLM_BASE_URL`, `LLM_MODEL`, and the proxy env
  vars from WP01. Add an `ask` example with `--trace`.
- [ ] **Step 4:** Run `make fmt lint test` and the `markdown-lint` skill.
  Expected: PASS.
- [ ] **Step 5:** Commit: `test(llm): accept opaque placeholder keys; document sandbox env`.
