## Context

DocsClaw Phase 1 runs each agent in an OpenShell sandbox as an outbound-only
process. Phase 2 exposes agents to each other through
`openshell service expose` and lets them call peers over A2A.

The OpenShell spike (WP01) established the constraint that shapes this
design: with `--authorization-mode bearer-passthrough`, the supervisor
forwards zero or one `Authorization` header unchanged. A request without a
token reaches the application with no header at all, and the supervisor adds
no trustworthy identity headers — gateway and edge identity headers are
stripped. The application is therefore the only authentication boundary.

`pkg/inboundauth` already implements the verification core. This change
records its contract and specifies how `serve` and the new `a2a_call` tool use
it. WP09 through WP13 implement the change.

## Goals / Non-Goals

**Goals:**

- Authenticate every inbound A2A and `/v1` request when inbound auth is on.
- Return a stable error code for every denial so demo scripts can assert on it.
- Guarantee the raw token never reaches the LLM, tools, logs, or session store.
- Isolate sessions by verified caller identity.
- Let the LLM call configured peer agents by name, with the supervisor
  injecting the outbound token.
- Bind to loopback by default when authentication is enabled.

**Non-Goals:**

- Shared agents serving multiple users.
- Delegation through a peer with the caller's token (nested `act` claims).
  This needs an OpenShell request-bound grant feature and is tracked in the
  v2 demo roadmap.
- Message buses, A2A push notifications, and streaming.
- An A2A adapter for non-DocsClaw agents; `pkg/inboundauth` stays reusable
  for that later project.
- Signed AgentCard verification (WP13) is optional and does not block merge.

## Decisions

### Library: `github.com/lestrrat-go/jwx/v3` v3.4.0

Compared on 2026-11-06:

| Criterion | jwx/v3 v3.4.0 | jwt/v5 v5.3.1 + keyfunc/v3 v3.9.1 |
| --- | --- | --- |
| Latest release | 2026-10-23 | 2026-01-28 + 2026-11-01 |
| Dependency edges added | 30 | 40 |
| Typed JWK parsing | built in | via keyfunc/jwkset |
| JWKS refresh | caller-driven | built in |
| Algorithm pinning | `jwt.WithKey` | `WithValidMethods` |
| License | MIT | MIT + Apache-2.0 |

`jwx/v3` was chosen: one dependency instead of two, fewer transitive edges, and
explicit control over the JWKS cache and refresh rate limit that the deny codes
require.

*Alternative considered:* `jwt/v5` plus `keyfunc`. Rejected because the
built-in refresh policy could not express the rate-limit and in-flight
semantics below without bypassing the library.

### Verification order and deny codes

The middleware checks, in order: exactly one `Authorization` header, the
algorithm allowlist, key lookup and signature, `exp`, `nbf`, `iat`, `iss`,
`aud`, scope, and caller. The first failing check decides the code, so a
caller can never learn more than the first failure.

| Condition | Status | `error` |
| --- | --- | --- |
| No `Authorization` header | 401 | `missing_token` |
| More than one header, not `Bearer`, or empty token | 401 | `malformed_authorization` |
| `alg` not ES256/RS256 (including `none`, `HS*`) | 401 | `unsupported_algorithm` |
| Bad signature, unparseable, unknown `kid` after refresh | 401 | `invalid_token` |
| `exp` in the past (beyond skew) | 401 | `token_expired` |
| `nbf` or `iat` in the future (beyond skew) | 401 | `token_not_yet_valid` |
| `iss` mismatch | 401 | `invalid_issuer` |
| `aud` does not contain configured audience | 403 | `invalid_audience` |
| None of `RequiredScopes` present | 403 | `insufficient_scope` |
| `azp` (fallback `client_id`) not in `AllowedCallers` | 403 | `caller_not_allowed` |
| JWKS cannot be fetched and no cached key matches | 503 | `jwks_unavailable` |

Body: `{"error":"<code>","error_description":"<short text>"}` with
`Content-Type: application/json`. A 401 response carries
`WWW-Authenticate: Bearer realm="docsclaw", error="<code>"`. An
`insufficient_scope` response also carries `scope="<required scopes>"`.

*Alternative considered:* a single `401 access_denied`. Rejected because the
demo's value is showing why a call was refused.

### Algorithm and key rules

- Only ES256 and RS256 are accepted. The allowlist is checked before key
  selection, so an `alg: none` or `HS*` token never reaches the key path.
- A token must carry `kid`. Tokens without one are `invalid_token` even when
  the JWKS has a single key, because positional selection makes rotation
  unsafe.
- The HS256 confusion attack is covered by a test that keys HMAC with the PKIX
  RSA public key bytes.

### JWKS caching and refresh

- The first request for an unknown `kid` triggers a fetch. Keys are cached by
  `kid` and replaced atomically on refresh; the map does not grow unbounded.
- The mutex is never held across the HTTP fetch. Cached `kid`s are served while
  a refresh is in flight, and only one refresh runs at a time.
- Refresh is rate-limited by `MinRefreshInterval` (default 60s). The
  rate-limit decision and the attempt timestamp are updated in the same
  critical section, so concurrent unknown-`kid` callers cannot both fetch.
- While a refresh is in flight, an unknown `kid` that hits the rate-limit
  branch fails closed with `401 invalid_token` instead of waiting. This keeps
  an unauthenticated caller from turning the JWKS into a latency lever.
- If the fetch fails and no cached key matches, the response is
  `503 jwks_unavailable`. Construction never fails because the issuer is
  unreachable.
- The fetch uses the configured `HTTPClient` (default `http.DefaultClient`,
  which is proxy-aware) and a 10s timeout. In a sandbox the fetch is ordinary
  egress, so the issuer's JWKS host must be in the sandbox network policy.

### Scope and caller matching

- Scopes come from a space-delimited `scope` string or an `scp` array or
  space-delimited string. Both forms are accepted together.
- At least one `RequiredScopes` entry must be present. An empty list disables
  the scope check.
- The caller is `azp`, falling back to `client_id`.
- `AllowedCallers` entries ending in `/` match as a prefix. All other entries
  must match exactly. A missing caller is `caller_not_allowed` when the list is
  non-empty.

### Token stripping

On success the middleware deletes `Authorization`, stores the verified claims
in the request context, and calls the next handler. Exempt paths (agent card
and health) skip verification but still delete `Authorization`. The token is
never logged, never placed in `Claims.Raw`, and never passed to the LLM, tools,
or session store.

WP12 proves the guarantee with a fake LLM provider that records every message
it receives; the test asserts the raw token string does not appear in that
input.

### Inbound configuration

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

The middleware wraps the A2A JSON-RPC endpoint and the OpenAI-compatible `/v1`
endpoint. Only the agent card and health endpoints are exempt. When inbound
auth is on, `/v1` is protected too; it is not left as an unauthenticated side
door.

The verified identity (`sub`, `azp`, `scope`, `aud`) is logged at info level
per request. This is the line the demo shows on screen.

### Session isolation

When inbound auth is on, the session key becomes `sub|azp|x-session-id`.
Without it, one authorized caller could read another caller's conversation by
guessing a session ID. The executor reads identity from `ClaimsFrom(ctx)`
instead of extracting the `Authorization` header, which Phase 1 removed.

*Alternative considered:* a random session key per caller. Rejected because
the demo needs a stable key across turns, and `x-session-id` is already the
caller-visible session selector.

### Loopback binding

When inbound auth is enabled and `service.host` is empty, `serve` binds to
`127.0.0.1`; otherwise it binds to `0.0.0.0`. An explicit value always wins,
and the health server uses the same host. WP01 confirmed that loopback is
sufficient because the supervisor reaches the sandbox through the isolation
backend's loopback connector.

**BREAKING**: an existing deployment that relied on the implicit `0.0.0.0`
default while enabling inbound auth must set `service.host` explicitly.

### `a2a_call` tool

```yaml
a2a_peers:
  - name: engineering-analyst
    url: https://<service-url-from-openshell>
    description: Analyzes engineering documents
```

- Parameters: `peer` (an enum of configured names) and `message`.
- The tool sends no bearer token. The supervisor injects it at the network
  boundary.
- The peer's public URL is the one `openshell service list --output json`
  prints; the harness must not compose it from workspace and sandbox names.
- The tool returns the peer's text and final task state. `input-required` and
  `auth-required` are terminal for now and are reported to the LLM.
- `401` and `403` responses are surfaced with their `error` code, as in Phase 1
  denial reporting.
- The tool is registered only when `a2a_call` appears in `tools.allowed`.
- The supervisor's network policy enforces the same peer list again. A config
  that names a forbidden peer is a deliberate demo scenario.

## Risks / Trade-offs

- [HTTPS denial body is unreadable] → Match the Go error string in one place,
  keep the mapping narrow, and test it at the string level.
- [JWKS endpoint unreachable] → Fail closed with `503 jwks_unavailable` and
  show the code on screen; do not fall back to unauthenticated serving.
- [Token leaks through a log or tool input] → Strip at the edge, never log
  headers, and prove absence in the WP12 fake-provider test.
- [Session collision across callers] → Include `sub` and `azp` in the key, not
  only `x-session-id`.
- [Peer URL drift] → Record the URL printed by `openshell service list`; do not
  compose it.
- [Loopback default breaks a deployment] → Document the **BREAKING** change and
  make an explicit `service.host` win.

## Migration Plan

1. WP09 ships `pkg/inboundauth` (already merged).
1. WP10 adds config, middleware wiring, identity logging, and session
   isolation, and changes the loopback default.
1. WP11 adds `a2a_call` and peer configuration.
1. WP12 adds the two-process end-to-end test.
1. WP13 optionally verifies signed AgentCards before the first peer call.
1. Rollback is a revert per WP. Inbound auth is off unless configured, so a
   revert restores the previous binding default without data migration.

## Open Questions

1. The exact public service URL format is deployment-specific; the harness
   must read it from `openshell service list --output json`.
1. Whether the v2 token issuer exposes the JWKS on the same host as the
   issuer. The design requires both to be in the sandbox network policy.
1. Whether signed AgentCard verification (WP13) ships in this change or is
   deferred; it is optional.
