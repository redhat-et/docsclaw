# WP10: Wire inbound auth into `serve` implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` (decisions 5-6) first. WP06, WP08 (approved), and WP09
> must be merged.

**Goal:** When `inbound_auth.enabled` is set, every A2A and `/v1` request
must carry a valid token. Identity is logged per request, sessions are
isolated per caller, and `serve` binds to loopback by default.

**Architecture:** `newServeHandler` (WP06) wraps the whole main mux with
`inboundauth.Middleware`, exempting only the agent card and `/health`. The
executor reads identity from `inboundauth.ClaimsFrom(ctx)`. The JWKS client
is `egress.NewClient`.

**Tech stack:** Go, `pkg/inboundauth`, `internal/egress`.

**Spec:** parent plan section 2.2.

## Review focus

- `inbound_auth.enabled: true` with an empty `audience` or `issuer`:
  startup fails with an error naming the key. It must not silently run
  without auth.
- `enabled: false` (or the block absent): behavior is unchanged, but log a
  warning at startup, `inbound auth disabled`, so a demo operator notices.
- Two callers using the same `x-session-id` get separate histories.
- `GET /v1/models` without a token gives 401 (the whole `/v1` tree is
  protected, not just chat completions).
- The health server port (`/metrics`, `/ready`) binds to the same resolved
  host.

---

### Task 1: Config and binding

**Files:**

- Modify: `internal/cmd/agentconfig.go`, `internal/config/config.go`
  (`service.host` default becomes `""`), `internal/cmd/serve.go`
- Test: `internal/cmd/agentconfig_test.go`, `internal/cmd/serve_test.go`

**Interfaces:**

- Produces: `AgentConfig.InboundAuth *InboundAuthCfg` (`yaml:"inbound_auth"`),
  with the fields `Enabled bool`, `Issuer`, `JWKSURL` (`yaml:"jwks_url"`),
  `Audience string`, `RequiredScopes []string` (`yaml:"required_scopes"`),
  `AllowedCallers []string` (`yaml:"allowed_callers"`), and `ClockSkew
  string` (a Go duration, optional).
- Produces: `func (c *InboundAuthCfg) toInboundAuth(exempt []string, client
  *http.Client, log *slog.Logger) (inboundauth.Config, error)`.
- Produces: `func resolveHost(configured string, authEnabled bool) string`.

- [ ] **Step 1: Write failing tests:**
  - `TestResolveHost` table: `("", true)` gives `127.0.0.1`; `("", false)`
    gives `0.0.0.0`; `("10.0.0.5", true)` gives `10.0.0.5`.
  - `TestInboundAuthCfg_Validate` table: ok; enabled with no issuer;
    enabled with no audience; a bad `clock_skew`; disabled with empty
    fields gives no error.
- [ ] **Step 2:** Run the tests. Expected: FAIL. Implement. Then make
  `Service.Addr()` and `HealthAddr()` use the resolved host (set it on
  `cfg.Service.Host` once in `runServe`, after `buildRuntime`).
- [ ] **Step 3:** Run the tests. Expected: PASS. Commit:
  `feat(serve): inbound_auth config and loopback default`.

### Task 2: Middleware, identity logging, and session isolation

**Files:**

- Modify: `internal/cmd/runtime.go` (`newServeHandler`),
  `internal/bridge/executor.go`
- Test: `internal/cmd/serve_auth_test.go`, `internal/bridge/executor_test.go`

**Interfaces:**

- Consumes: `inboundauth.Middleware`, `inboundauth.ClaimsFrom`, and
  `inboundauth.ContextWithClaims` (WP09).
- Produces: `func sessionKey(ctx context.Context, sessionID string) string`
  in `internal/bridge`. It returns `sub + "|" + azp + "|" + sessionID` when
  claims are present, and `sessionID` otherwise. It returns `""` if
  `sessionID == ""`.
- Exempt paths: `/.well-known/agent-card.json` and `/health`. Nothing else.

- [ ] **Step 1: Write failing tests** in `serve_auth_test.go`. Build the
  runtime with the fake provider from WP06. The test issuer is a minimal
  copy of WP09's `testIssuer`: ES256 only, in
  `internal/cmd/testissuer_test.go`.
  - `TestServeAuth_Routes` table:
    - `POST /a2a` with no token gives `401 missing_token`
    - `POST /a2a` with a valid token gives 200
    - `POST /` (JSON-RPC root) with no token gives 401
    - `POST /v1/chat/completions` with no token gives 401
    - `GET /v1/models` with no token gives 401
    - `GET /.well-known/agent-card.json` with no token gives 200
    - `GET /health` with no token gives 200
  - `TestServeAuth_LogsIdentity`: capture the logger output. A valid
    request logs `sub=alice`, `azp=`, `scope=`, and `aud=`. The output does
    not contain the token string.
  - `TestServeAuth_DisabledWarns`: with no `inbound_auth` block, `POST
    /a2a` with no token gives 200, and the log contains `inbound auth
    disabled`.
- [ ] **Step 2: Write failing tests** in `executor_test.go`:
  - `TestSessionKey` table: claims plus an ID gives `alice|azp1|s1`; no
    claims gives `s1`; an empty ID gives `""`.
  - `TestExecute_SessionsIsolatedByCaller`: use a memory store and two
    contexts with different `sub`s and the same `x-session-id: s1`. The
    second caller's `ProcessMessage` input contains only its own user
    message.
- [ ] **Step 3:** Run the tests. Expected: FAIL. Implement. In the executor,
  log `"A2A request authenticated", "sub", "azp", "scope", "aud"` at info
  when claims are present, and use `sessionKey` for all session store calls.
  Apply the 64-character truncation to the raw ID before building the key.
- [ ] **Step 4:** Run `go test -race ./internal/...`. Expected: PASS.
- [ ] **Step 5:** Update `deploy/README.md` with an "Inbound
  authentication" section: a 10-line config example, a note that the JWKS
  host must be in the sandbox network policy (it is egress through the
  supervisor), and the error codes table (link to the `pkg/inboundauth`
  docs rather than copying it).
- [ ] **Step 6:** Run `make fmt lint test`. Commit:
  `feat(serve): enforce inbound auth on A2A and /v1 with per-caller sessions`.
