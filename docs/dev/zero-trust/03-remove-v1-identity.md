# WP03: Remove v1 identity remnants implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` first (especially decision 1). Steps use checkbox syntax.

**Goal:** Delete every v1 SPIFFE, OPA, and delegation code path, plus the A2A
document mode that forwards the caller's bearer token.

**Architecture:** This WP only removes code. The behavior that survives:
every A2A message goes through `AgentExecutor.ProcessMessage`. Without
`agent-config.yaml`, that function makes one LLM `Complete` call.

**Tech stack:** Go, Cobra/Viper, a2a-go v2.

**Spec:** parent plan section 1.1; `00-index.md` decision 1.

## Review focus

- An A2A message without `agent-config.yaml` (single-shot mode) still gets
  an answer. Before this WP it went through document mode or failed.
- The `authorization` service param is no longer read anywhere in
  `internal/bridge` (a test asserts that the executor ignores it).
- `docsclaw batch` still works: it uses `bridge.InvokeRequest` without the
  removed fields.

---

### Task 1: Remove delegation from the bridge

**Files:**

- Delete: `internal/bridge/delegation.go`
- Delete: `internal/bridge/message.go` (`ExtractDocumentID`), and its test
  if one exists
- Modify: `internal/bridge/executor.go`, `internal/bridge/client.go`
- Test: `internal/bridge/executor_test.go` (create if missing)

**Interfaces:**

- Produces: `bridge.AgentExecutor{Log, ProcessMessage, Sessions,
  SystemPrompt}`. The `FetchDocument` and `ProcessLLM` fields and the
  `DocumentFetcher`/`LLMProcessor` types are deleted. `ProcessMessage` is
  now required: `Execute` yields a failed event `"agent not configured"` if
  it is nil.
- Produces: `bridge.InvokeRequest{AgentURL, MessageText, SessionID?}`, with
  `BearerToken`, `UserSPIFFEID`, and `AgentSPIFFEID` removed. Keep any other
  existing fields.

- [ ] **Step 1: Write failing tests** in `executor_test.go`:
  - `TestExecute_IgnoresAuthorizationServiceParam`: `ServiceParams` holds
    `authorization: Bearer secret-token`. A recording `ProcessMessage`
    captures `messages`. Assert that no message content contains
    `secret-token`, and that the final event is `TaskStateCompleted`.
  - `TestExecute_DocumentIDMessageGoesToProcessMessage`: the message text
    is `DOC-001`. Assert that `ProcessMessage` was called once with a user
    message `DOC-001`.
  - `TestExecute_NilProcessMessageFails`: assert a failed status with the
    reason `agent not configured`.
- [ ] **Step 2:** Run `go test ./internal/bridge/ -run TestExecute -v`.
  Expected: compile failure or FAIL.
- [ ] **Step 3:** Edit `executor.go`. Remove the bearer and `x-delegation-*`
  extraction, `WithDelegation`, and the document-mode branch. Keep
  `x-session-id` and trace-context extraction. Edit `client.go`: remove the
  three fields and their `sp.Append` lines.
- [ ] **Step 4:** Run `go test ./internal/bridge/ -v`. Expected: PASS.
- [ ] **Step 5:** Commit: `refactor(bridge): remove delegation headers and document mode`.

### Task 2: Remove v1 wiring from serve and batch

**Files:**

- Modify: `internal/cmd/serve.go`, `internal/cmd/batch.go` (only if it no
  longer compiles), `internal/fetchdoc/fetchdoc.go`
- Test: `internal/cmd/serve_test.go`

**Interfaces:**

- Produces: `fetchdoc.DocumentFetcher func(ctx context.Context, documentID
  string) (map[string]any, error)`, with the `bearerToken` parameter removed.
  WP05 replaces this type again; keep the change minimal here.

- [ ] **Step 1:** In `runServe`, replace the `DelegationTransport` client
  with `&http.Client{Timeout: 30 * time.Second}`. Delete the
  `Authorization` header line in `fetchDocument`. Stop passing
  `FetchDocument`/`ProcessLLM` to the executor. Always set
  `executor.ProcessMessage`: in single-shot mode (`toolRegistry == nil`), it
  calls `llmProvider.Complete(ctx, systemPrompt, lastUserContent)`, or
  returns the existing mock text when `llmProvider == nil`. Delete the
  `processLLM` closure if nothing else uses it.
- [ ] **Step 2:** Change the `fetch_document` description to: `"Fetch a
  document from the document service by ID (e.g., DOC-001). Returns the
  document title and content."`
- [ ] **Step 3:** Add `TestSingleShotProcessMessage_UsesComplete` to
  `serve_test.go` if the closure can be tested in isolation. If the closure
  is inline, note the gap in the PR description; WP06 will cover it.
- [ ] **Step 4:** Run `go build ./... && go test ./internal/...`. Expected:
  PASS.
- [ ] **Step 5:** Commit: `refactor(serve): drop delegation transport and token forwarding`.

### Task 3: Remove SPIFFE/OPA config, metrics, and logger helpers

**Files:**

- Modify: `internal/config/config.go`, `internal/metrics/metrics.go`,
  `internal/logger/logger.go`, plus any test that references removed symbols

- [ ] **Step 1:** In `config.go`, delete `ServiceConfig.MockSPIFFE`,
  `SPIFFEConfig`, `OPAConfig` (and `Addr`), the `CommonConfig.SPIFFE`/`OPA`
  fields, their defaults, the `--mock-spiffe`/`--opa-host`/`--opa-port`
  flags and bindings, and the `"opa"` entry in `GetServiceEndpoints`. First
  grep for readers: `grep -rn 'GetServiceEndpoints\|\.SPIFFE\|\.OPA\|MockSPIFFE' --include='*.go' .`
- [ ] **Step 2:** In `metrics.go`, delete `AuthorizationDecisions`,
  `AuthorizationDuration`, `DelegationsTotal`, and any metric whose label set
  includes `spiffe_id`, if nothing references them after Step 1. In
  `logger.go`, delete the `ComponentOPA*` constants and their colors.
- [ ] **Step 3:** Run the acceptance check:

```bash
grep -rniE 'spiffe|\bopa\b|x-delegation|delegation' --include='*.go' . | grep -v '^./demo/'
```

  Expected: no output. If a `demo/batch/dashboard` match remains, remove it
  too. The dashboard sends its own Kubernetes `Authorization` header
  (`k8s.go`); that line is unrelated, so keep it.

- [ ] **Step 4:** Run `make fmt lint test`. Expected: all green.
- [ ] **Step 5:** Also grep the docs and deploy manifests for `mock-spiffe`,
  `MOCK_SPIFFE`, and `OPA_` env vars. Remove them in the same commit.
- [ ] **Step 6:** Commit: `refactor: remove SPIFFE and OPA config, metrics, and logger remnants`.
