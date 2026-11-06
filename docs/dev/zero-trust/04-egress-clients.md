# WP04: Egress through the supervisor implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` (decision 3) and `openshell-notes.md` first.

**Goal:** Every outbound HTTP request the agent makes honors proxy
environment variables and uses the stock dialer, and a test keeps it that
way.

**Architecture:** A new package `internal/egress` owns HTTP client
construction. Production code under `internal/` and `pkg/` must not build
`http.Transport` literals or custom dialers. A source-scan test enforces
this, and a unit test checks the constructor.

**Tech stack:** Go stdlib (`net/http`, `go/parser`, `go/ast`).

**Spec:** parent plan section 1.2.

## Audit baseline (already done while writing this WP)

| Site | Current client | Proxy-safe today? |
| --- | --- | --- |
| `internal/cmd/serve.go` `runServe` | `&http.Client{Timeout: 30s}` after WP03 | yes (nil Transport means `DefaultTransport`) |
| `internal/webfetch/webfetch.go` `Execute` | `&http.Client{CheckRedirect: ...}` | yes |
| `internal/openai/openai.go` | `&http.Client{}` | yes |
| `internal/anthropic/anthropic.go` | SDK default client | verify in Task 2 |
| `internal/mcpclient/transport.go` | `mcp.StreamableClientTransport{Endpoint}` (nil `HTTPClient` means `http.DefaultClient`) | yes |
| `internal/skillpuller/server.go`, `source/url.go` | `&http.Client{Timeout: 30s}`, `http.DefaultClient` | yes |
| `pkg/rag/weaviate.go` | `&http.Client{Timeout: 30s}` | yes |
| `demo/batch/dashboard/*` | custom `http.Transport` with TLS | out of scope (does not run in a sandbox) |

Today the risk is regression, not current breakage. This WP makes the
choice explicit and adds a guard.

## Review focus

- Someone later adds `Transport: &http.Transport{TLSClientConfig: ...}` to
  pin a CA. That literal silently drops `Proxy`, and the scan test must
  catch it.
- `HTTPS_PROXY` set while `NO_PROXY` includes the cluster domain: document
  in the README that `NO_PROXY` bypasses the supervisor and is then blocked.
  Use the WP01 findings.

---

### Task 1: `internal/egress` client constructor

**Files:**

- Create: `internal/egress/client.go`, `internal/egress/client_test.go`,
  `internal/egress/doc.go`

**Interfaces:**

- Produces: `func NewClient(timeout time.Duration) *http.Client`. It
  returns a client whose `Transport` is `http.DefaultTransport.(*http.Transport).Clone()`
  and whose `Timeout` is the argument (0 means no timeout).
- Produces: `func NewTransport() *http.Transport`, the same clone, for
  callers that need to set `CheckRedirect` or wrap the transport.

- [ ] **Step 1: Write failing tests:**
  - `TestNewClient_ProxyFromEnvironment`: `t.Setenv("HTTPS_PROXY",
    "http://proxy.test:3128")`. Assert that `tr.Proxy != nil` and that, for
    `https://example.com`, `tr.Proxy(req)` returns `proxy.test:3128`. Note:
    `ProxyFromEnvironment` caches env on first use, so build the request URL
    and call `httpproxy.FromEnvironment().ProxyFunc()` as a cross-check, or
    run this test in its own process using `TestMain`. Choose one and
    comment why.
  - `TestNewClient_NoCustomDialer`: assert that `tr.DialContext` is the
    clone's (non-nil) and `tr.DialTLSContext == nil`.
  - `TestNewClient_Timeout`: `NewClient(5*time.Second).Timeout == 5*time.Second`.
- [ ] **Step 2:** Run `go test ./internal/egress/ -v`. Expected: FAIL
  (undefined).
- [ ] **Step 3:** Implement. In `doc.go`, explain why (OpenShell supervisor
  egress) and summarize the WP01 interception answer in two sentences.
- [ ] **Step 4:** Run the tests. Expected: PASS.
- [ ] **Step 5:** Commit: `feat(egress): add proxy-aware HTTP client constructor`.

### Task 2: Route all sandbox-side clients through `egress`

**Files:**

- Modify: `internal/cmd/serve.go`, `internal/webfetch/webfetch.go`,
  `internal/openai/openai.go`, `internal/skillpuller/server.go`,
  `internal/skillpuller/source/url.go`, `pkg/rag/weaviate.go`,
  `internal/mcpclient/transport.go`, `internal/anthropic/anthropic.go`

- [ ] **Step 1:** Replace each `&http.Client{...}` with `egress.NewClient(...)`,
  keeping the existing timeout. Then set the remaining fields
  (`CheckRedirect`) on the returned client. For MCP, set
  `StreamableClientTransport.HTTPClient = egress.NewClient(0)`. For
  Anthropic, pass `option.WithHTTPClient(egress.NewClient(0))`. `pkg/rag`
  is public, but `pkg` must not import `internal/`: leave
  `pkg/rag/weaviate.go` on a nil-Transport client and add a comment, since
  the scan test still covers it.
- [ ] **Step 2:** Run `go test ./...`. Expected: PASS.
- [ ] **Step 3:** Commit: `refactor: build all outbound clients via internal/egress`.

### Task 3: Source-scan guard test

**Files:**

- Create: `internal/egress/scan_test.go`

- [ ] **Step 1:** Write `TestNoRawTransportsOrDialers`. Walk the repo root
  (`../..`), skipping `demo/`, `.worktrees/`, `vendor/`, `_test.go` files,
  and `internal/egress/`. Parse each `.go` file with `go/parser` and fail on
  any composite literal of type `http.Transport`, or any selector named
  `DialContext`, `Dial`, or `DialTLSContext` assigned in a composite literal
  or assignment. The failure message names the file:line and says "use
  egress.NewTransport()".
- [ ] **Step 2:** Prove the test catches a violation: temporarily add
  `_ = &http.Transport{}` to `internal/webfetch/webfetch.go`, run
  `go test ./internal/egress/ -run TestNoRaw -v`, and confirm FAIL with that
  file:line. Then revert.
- [ ] **Step 3:** Run `make fmt lint test`. Expected: PASS.
- [ ] **Step 4:** Add a short "Egress in OpenShell sandboxes" subsection to
  `deploy/README.md`: which env vars the supervisor sets (from WP01), the
  `NO_PROXY` warning, and the rule that new clients use `internal/egress`.
- [ ] **Step 5:** Commit: `test(egress): forbid raw transports and custom dialers`.
