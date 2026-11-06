# Plan: make DocsClaw a zero-trust citizen in OpenShell sandboxes

## Context

DocsClaw (`github.com/redhat-et/docsclaw`) is moving from the v1 Zero
Trust demo (Kagenti, AuthBridge, OPA sidecars) to the v2 demo
(`github.com/redhat-et/zero-trust-agents-v2`), where **every agent runs
in an NVIDIA OpenShell sandbox**. In v2:

- The sandbox has no direct egress. The OpenShell supervisor intercepts
  every outbound request, performs RFC 8693 token exchanges using the
  sandbox's SPIFFE JWT-SVID, and injects the final bearer token. The
  agent never holds credentials.
- Model API keys reach the sandbox only as opaque placeholders. The
  proxy substitutes the real key at the network boundary.
- Inbound traffic reaches a sandbox only through
  `openshell service expose`. With `--authorization-mode
  bearer-passthrough`, OpenShell forwards one bearer token unchanged,
  but **requests without a token also pass through**. The application
  must authenticate callers itself (see OpenShell RFC 0017).

The v2 demo will run one DocsClaw sandbox per user and agent type
("Alice's assistant", "Alice's engineering analyst"). Lean agents make
this affordable. Shared multi-user agents are explicitly out of scope.

This plan has two phases:

1. **Phase 1** (blocks v2 Stage A): DocsClaw runs in a sandbox as an
   outbound-only agent that fetches documents and answers a prompt.
1. **Phase 2** (blocks v2 Stage B): DocsClaw can call other agents over
   A2A and authenticates incoming A2A calls itself.

## Working agreements

- Analyze before coding. For each step, read the relevant code first,
  write a short design note, and confirm it before implementing.
- Use OpenSpec as the repo already does: one change per phase under
  `openspec/changes/` (proposal, design, tasks, specs).
- Use the latest stable Go and latest versions of dependencies. Keep
  `github.com/a2aproject/a2a-go/v2` current.
- Run `make fmt lint test` before every commit. Add table-driven tests
  for every security decision (allow and every deny reason).
- Clean break: no backward compatibility for v1-era settings. The
  project has no production consumers (same decision as the
  `rename-spiffe-and-fix-workspace` change).
- Markdown follows strict linting: blank lines around headings, lists,
  and code blocks; sentence-case headings; `1.` for ordered lists.

## Phase 1: outbound-only agent in an OpenShell sandbox

### 1.1 Remove v1 identity remnants

v1 relied on infrastructure in front of the agent. In v2 those code
paths are either dead or unsafe (unsigned headers that anyone can
forge).

Remove or rework:

- `service.mock_spiffe` / `--mock-spiffe`, the `spiffe.*` and `opa.*`
  config keys, and the `--opa-host` / `--opa-port` flags in
  `internal/config/config.go`. Grep for any remaining readers first.
- `internal/bridge/delegation.go` (`DelegationContext`,
  `DelegationTransport`) and the `X-Delegation-User` /
  `X-Delegation-Agent` handling in `internal/bridge/executor.go` and
  `internal/bridge/client.go` (`UserSPIFFEID`, `AgentSPIFFEID`).
  Check `internal/cmd/batch.go` and `demo/batch/dashboard/` callers and
  update them.
- The `fetch_document` description in `internal/fetchdoc/fetchdoc.go`
  that mentions "delegation and OPA authorization". Replace with a
  neutral description.
- Metrics and logger helpers that only exist for SPIFFE/OPA
  (`metrics.AuthorizationDecisions`, `logger.SVID`) if nothing uses
  them after the removal.

Acceptance: `grep -ri 'spiffe\|opa\|x-delegation' --include=*.go .`
returns nothing outside tests that you deliberately kept, and all
tests pass.

### 1.2 Guarantee egress goes through the supervisor

OpenShell brokers egress. A Go client that bypasses proxy settings or
dials directly will be blocked, and the failure looks confusing during
a live demo.

- Audit every `http.Client` and `http.Transport` (`internal/cmd/serve.go`,
  `internal/webfetch`, `internal/openai`, `internal/anthropic`,
  `internal/mcpclient`, `internal/skillpuller`). Each must use
  `http.DefaultTransport` or a clone of it that keeps
  `Proxy: http.ProxyFromEnvironment`. No custom dialers.
- Add a unit test that fails if any constructor returns a transport
  with a nil `Proxy` function.
- Find out how the OpenShell supervisor intercepts traffic (transparent
  interception or `HTTP_PROXY` environment). Read
  `docs/how-it-works/sandboxes/overview.mdx` in the OpenShell repo and
  record the answer in the design note. The code must work in both
  cases.

### 1.3 Multiple named document services

v2 runs one document service per department (engineering, finance, hr,
and so on). The current `fetch_document` tool supports only one
`--document-service-url`.

- Add a `document_services` map to `agent-config.yaml`:

  ```yaml
  document_services:
    engineering: http://document-engineering.zt-demo.svc.cluster.local:8080
    finance: http://document-finance.zt-demo.svc.cluster.local:8080
  ```

- Change the tool to take `service` and `document_id`. Unknown service
  names return a clear error from the agent.
- Keep `--document-service-url` only if something still needs it;
  otherwise remove it (clean break).
- Do not add an agent-side host allowlist for document services. In
  the demo, the OpenShell policy is the enforcement point. The DocsClaw
  config may list services the platform will refuse; that is a
  deliberate teaching scenario.

### 1.4 Report denials precisely

The demo's value is in showing **why** a request was refused.

- On non-2xx responses, return the HTTP status and the JSON `error`
  field (for example `403 invalid_audience`,
  `403 insufficient_department_permissions`) to the LLM as the tool
  result, instead of a generic "access denied".
- Detect supervisor policy denials (connection refused, proxy 403, or
  whatever OpenShell returns; check this during the spike) and report
  them as `blocked by sandbox network policy`.
- Log each outbound tool call at info level: tool, target host, status,
  and denial reason. Never log headers.

### 1.5 One-shot `ask` command

The v2 scenario scripts drive sandboxes with `openshell sandbox exec`.
A long-running server plus an extra A2A CLI in the image adds moving
parts. Add a one-shot command instead:

```console
docsclaw ask --config-dir /config/agent "Summarize DOC-001 from engineering"
```

- Runs the same agentic loop as `serve` (same config loading, tools,
  skills, and LLM provider), prints the final answer to stdout, and
  exits non-zero on loop failure.
- `--trace` prints each tool call and its result summary to stderr, so
  scripts can show what the agent tried.
- Reuse the code in `internal/cmd/serve.go`. Extract the shared setup
  into a function rather than duplicating it.

### 1.6 LLM access through an OpenShell provider

- Confirm that DocsClaw accepts a placeholder value in `LLM_API_KEY`
  and does not validate its format. The OpenShell proxy substitutes the
  real key.
- Confirm `--llm-base-url` / `--llm-model` work with an
  OpenAI-compatible endpoint (the demo uses a MaaS endpoint).
- Document the required environment variables for sandbox use in
  `deploy/README.md`.

### 1.7 Sandbox image

The v2 harness creates sandboxes with `-- sleep infinity` and execs
commands into them. `Dockerfile.release` uses a minimal runtime image
that may lack a shell and `sleep`.

- Read `examples/bring-your-own-container` in the OpenShell repo for
  sandbox image requirements.
- Add `Containerfile.openshell`: a small base (UBI micro or minimal)
  with `/usr/local/bin/docsclaw`, `sleep`, and a non-root user. The
  binary path matters: OpenShell provider profiles bind credentials to
  binary paths.
- Publish as `ghcr.io/redhat-et/docsclaw:<version>-openshell` from the
  release workflow.

### Phase 1 definition of done

- `docsclaw ask` works locally against a stub document service and an
  OpenAI-compatible endpoint.
- No v1 identity code remains.
- Proxy test passes; denial reporting is covered by tests.
- The OpenShell image is published and its binary path is documented.

## Phase 2: A2A between sandboxes

### 2.1 `pkg/inboundauth`: incoming token verification

Write this as a **standalone, reusable package** with no imports from
`internal/`. The same package will later go into an A2A adapter that
wraps mainstream agents (Codex, Pi, and so on), so keep its API small
and framework-neutral.

API sketch:

```go
type Config struct {
    Issuer         string
    JWKSURL        string
    Audience       string   // this agent's audience, e.g. "engineering-analyst"
    RequiredScopes []string // at least one must be present
    AllowedCallers []string // azp allowlist (supervisor SPIFFE IDs or prefixes)
    ClockSkew      time.Duration
    ExemptPaths    []string // e.g. /.well-known/agent-card.json, /health
}

func Middleware(cfg Config, next http.Handler) (http.Handler, error)
func ClaimsFrom(ctx context.Context) (Claims, bool)
```

Behavior:

- Require exactly one `Authorization: Bearer` header. Missing or
  malformed means `401` with a `WWW-Authenticate` header.
  This matters because OpenShell's bearer passthrough lets
  unauthenticated requests through.
- Verify the signature against the JWKS, with caching and refresh on
  unknown `kid`. Accept only ES256 and RS256; reject `none` and all
  `HS*` algorithms.
- Check `iss`, `aud` (must contain the configured audience), `exp`,
  `nbf`, and `iat` with skew. Check scopes. Check `azp` against
  `AllowedCallers` when the list is non-empty (`403` with reason
  `caller_not_allowed`).
- On success, put the verified claims in the request context and
  **remove the `Authorization` header** before calling `next`. The
  raw token must never reach the LLM, tools, logs, or session store.
- Return a short JSON error body with a stable `error` code for each
  deny reason, so demo scripts can assert on it.
- Pick a maintained JWT/JWKS library at its latest version (for
  example `github.com/lestrrat-go/jwx/v3` or
  `github.com/golang-jwt/jwt/v5` with a JWKS helper). Justify the
  choice in the design note.

Tests: a table with a locally generated key pair and a JWKS test
server covering valid, expired, wrong issuer, wrong audience, missing
scope, disallowed caller, `alg: none`, HS256, unknown `kid`, missing
header, duplicate header.

### 2.2 Wire it into `serve`

- Config block in `agent-config.yaml`:

  ```yaml
  inbound_auth:
    enabled: true
    issuer: http://token-issuer.zt-demo.svc:8080
    jwks_url: http://token-issuer.zt-demo.svc:8080/.well-known/jwks.json
    audience: engineering-analyst
    required_scopes: [engineering]
    allowed_callers:
      - spiffe://openshell.local/openshell/sandbox/
  ```

- Apply the middleware to the A2A JSON-RPC endpoint **and** the
  OpenAI-compatible `/v1` endpoint (or disable `/v1` when inbound auth
  is on). Exempt only the agent card and health endpoints.
- The JWKS fetch is itself egress, so it goes through the supervisor.
  Note this in the docs: the issuer's JWKS host must be in the
  sandbox's network policy.
- `serve` binds to loopback by default when inbound auth is enabled
  (OpenShell reaches the service through the sandbox loopback).
- Log verified identity per request at info level: `sub`, `azp`,
  `scope`, `aud`. This is what the demo shows on screen.
- Remove the old bearer-token extraction from
  `internal/bridge/executor.go`; read identity from `ClaimsFrom(ctx)`
  instead.

### 2.3 `a2a_call` tool

- Config in `agent-config.yaml`:

  ```yaml
  a2a_peers:
    - name: engineering-analyst
      url: https://<service-url-from-openshell>
      description: Analyzes engineering documents
  ```

- Tool parameters: `peer` (one of the configured names) and `message`.
  Unknown peers return an error. The supervisor enforces the same list
  again through network policy (defense in depth, and a demo
  scenario).
- Use the existing `internal/bridge/client.go`. Send **no** bearer
  token from the agent: the supervisor injects it.
- Return the peer's text and final task state. Treat `input-required`
  and `auth-required` as terminal for now and report them to the LLM.
- Surface `401` / `403` responses with their `error` code, as in
  step 1.4.
- Register the tool only when `a2a_call` is in `tools.allowed`.

### 2.4 Optional: verify signed AgentCards

DocsClaw can already serve a pre-signed card
(`internal/bridge/signedcard.go`). If time allows, make `a2a_call`
fetch the peer card and verify its JWS signature against a configured
key before the first call. Skip this if Phase 2 runs late. It is a
nice-to-have, not a demo blocker.

### Phase 2 definition of done

- Two local DocsClaw processes: A calls B through `a2a_call` with a
  token minted by a test issuer; B logs verified claims.
- Every deny reason returns the documented error code.
- The raw token is provably absent from B's LLM input (test with a
  fake LLM provider that records its input).
- `pkg/inboundauth` has no `internal/` imports and has package docs
  with a usage example.

## Out of scope

- Shared agents serving multiple users.
- Delegation through B with the caller's token (nested `act` claims).
  This needs an OpenShell feature (request-bound token grants) and is
  tracked as a roadmap item in the v2 demo.
- Message buses, A2A push notifications, streaming.
- An A2A adapter for mainstream agents (a later project that reuses
  `pkg/inboundauth`).

## Open questions to resolve during Phase 1

1. How exactly does the OpenShell supervisor intercept egress, and
   what does a policy denial look like to a Go HTTP client?
1. Does OpenShell require anything specific in a sandbox image beyond
   a shell and `sleep`?
1. Should `fetch_document` be renamed now that it takes a service
   parameter (for example `get_document`)? Decide once, clean break.
