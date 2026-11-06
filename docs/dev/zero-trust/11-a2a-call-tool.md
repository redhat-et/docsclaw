# WP11: `a2a_call` tool implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` first. WP05 and WP08 (approved) must be merged.

**Goal:** The LLM can call a configured peer agent over A2A by name. The
agent sends no credentials (the supervisor injects them), and the LLM sees
the peer's answer, the final task state, or the precise denial.

**Architecture:** A new package `internal/a2acall` holds the tool. It uses
`bridge.A2AClient` (cleaned up in WP03) with an `egress.NewClient` HTTP
client. Peers come from `a2a_peers` in `agent-config.yaml`.

**Tech stack:** Go, a2a-go v2, `internal/egress`.

**Spec:** parent plan section 2.3.

## Review focus

- A peer returns `401 caller_not_allowed`: the tool result must be
  `peer engineering-analyst: 401 caller_not_allowed`, not an a2a-go
  JSON-RPC parse error. Check how a2a-go surfaces non-2xx responses. If it
  hides the body, wrap the HTTP client's transport to capture the status
  and body for `egress.DescribeResponse`.
- A peer that hangs: a per-call timeout (default 120s, configurable per
  peer as `timeout_seconds`) gives `peer X: request failed: context
  deadline exceeded`.
- A very large peer reply: rely on the loop's `MaxResultBytes`, and test
  that it gets truncated, not dropped.
- No `Authorization` header leaves the agent, even if the context carries
  claims from an inbound request (proved in a test).

---

### Task 1: Config

**Files:**

- Modify: `internal/cmd/agentconfig.go`
- Test: `internal/cmd/agentconfig_test.go`

**Interfaces:**

- Produces: `AgentConfig.A2APeers []A2APeer` (`yaml:"a2a_peers"`), where
  `A2APeer{Name, URL, Description string; TimeoutSeconds int}`.
  Validation: the name matches `^[a-z0-9][a-z0-9-]*$`, names are unique,
  and the URL is absolute `http`/`https`.

- [ ] **Step 1:** Write the failing table test `TestLoadAgentConfig_A2APeers`:
  ok; duplicate name; bad name; relative URL. Implement, and run until PASS.

### Task 2: Tool

**Files:**

- Create: `internal/a2acall/a2acall.go`, `internal/a2acall/a2acall_test.go`
- Modify: `internal/bridge/client.go` (only if Task 2 needs `InvokeResult`
  to expose the task state)

**Interfaces:**

- Produces: `func New(peers []Peer, client *bridge.A2AClient, log *slog.Logger)
  tools.Tool`, with name `a2a_call`, where `Peer{Name, URL, Description
  string; Timeout time.Duration}`. Parameters: `peer` (string, `enum` of
  names) and `message` (string), both required. The description lists each
  peer as `name: description`.
- Produces (if missing): `bridge.InvokeResult.State a2a.TaskState`.
- Result format: OK, `"[<state>] <peer text>"` for `completed`. `IsError`
  for `failed`, `rejected`, and `canceled`. `input-required` and
  `auth-required` give a non-error result `"[<state>] peer needs more input:
  <text>"`. The loop treats these as terminal and does not continue the
  conversation. Unknown peer: `unknown peer "<name>"; available: a, b`, with
  no network call. HTTP denial: `peer <name>: <egress.DescribeResponse>`.
  Transport error: `peer <name>: <egress.DescribeError>`. Call
  `egress.LogCall` once per call with `tool=a2a_call` and `host=<peer
  host>`.

- [ ] **Step 1: Write failing tests.** Use an `httptest` A2A server built
  with `a2asrv.NewJSONRPCHandler` and a scripted executor. The server
  records request headers.
  - `TestA2ACall` table: completed; failed; input-required; auth-required;
    unknown peer (0 requests); missing message; `401
    {"error":"caller_not_allowed"}` from a plain handler in front of the
    JSON-RPC handler; `403 {"error":"insufficient_scope"}`; timeout (the
    server sleeps past a 100ms peer timeout).
  - `TestA2ACall_SendsNoAuthorization`: the tool context carries
    `inboundauth.ContextWithClaims(...)`. Assert that the recorded request
    has no `Authorization` header.
- [ ] **Step 2:** Run `go test ./internal/a2acall/ -v`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the tests. Expected: PASS. Commit:
  `feat(a2acall): add a2a_call tool for configured peers`.

### Task 3: Register in the runtime

**Files:**

- Modify: `internal/cmd/runtime.go`
- Test: `internal/cmd/runtime_test.go`

- [ ] **Step 1:** Write the failing test `TestBuildRuntime_A2ACallRegistration`
  (table): peers configured and `a2a_call` in `tools.allowed` gives it in
  `Registry.Definitions()`; peers configured but not allowed gives it absent;
  allowed but no peers gives it absent and logs a warning `a2a_call allowed
  but no a2a_peers configured`.
- [ ] **Step 2:** Implement with `Registry.Register`, not
  `RegisterAlwaysAllowed`, so `tools.allowed` gates the tool. Run the tests.
  Expected: PASS.
- [ ] **Step 3:** Add an `a2a_peers` example and a note about defense in
  depth (the supervisor policy enforces the same list) to
  `deploy/README.md`.
- [ ] **Step 4:** Run `make fmt lint test`. Commit:
  `feat(cmd): register a2a_call when allowed and peers configured`.
