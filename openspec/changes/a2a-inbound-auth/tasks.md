## 1. Inbound auth package (WP09 — shipped in `cadc7d2`)

- [x] 1.1 Add the package skeleton and choose `github.com/lestrrat-go/jwx/v3`
  v3.4.0, recording the comparison in `doc.go`
- [x] 1.2 Add a test issuer with ES256 and RS256 keys and a JWKS server that
  counts hits
- [x] 1.3 Implement verification, the eleven deny codes, the error body, and
  `WWW-Authenticate`
- [x] 1.4 Implement the JWKS cache with cached-key serving, one in-flight
  refresh, and the `MinRefreshInterval` rate limit
- [x] 1.5 Add package docs, a compiling example, and the no-`internal/`-imports
  guard

## 2. Configuration and binding (WP10)

- [ ] 2.1 Add the `inbound_auth` block to `agent-config.yaml` parsing with
  validation for issuer, JWKS URL, audience, scopes, and callers
- [ ] 2.2 Resolve `service.host`: `127.0.0.1` when inbound auth is enabled and
  the host is empty, `0.0.0.0` otherwise, with an explicit value winning
- [ ] 2.3 Bind the health server to the same resolved host
- [ ] 2.4 Add table-driven tests for every host-resolution combination
- [ ] 2.5 Document the **BREAKING** loopback default in `deploy/README.md`

## 3. Middleware wiring and session isolation (WP10)

- [ ] 3.1 Build the middleware in the serve handler and wrap the A2A endpoint
  and `/v1`
- [ ] 3.2 Exempt only the agent card and health endpoints; confirm exempt paths
  still strip `Authorization`
- [ ] 3.3 Log `sub`, `azp`, `scope`, and `aud` at info level per authenticated
  request, and never the token or headers
- [ ] 3.4 Derive the session key as `sub|azp|x-session-id` when inbound auth is
  on
- [ ] 3.5 Read identity from `ClaimsFrom` in the executor and delete any
  remaining `Authorization` extraction
- [ ] 3.6 Add serve-level tests asserting each deny code over HTTP and the
  session-isolation behavior

## 4. A2A call tool (WP11)

- [ ] 4.1 Add the `a2a_peers` config with unique-name and absolute-URL
  validation
- [ ] 4.2 Implement `internal/a2acall` with `peer` and `message`, an enum of
  configured names, and an error for unknown peers
- [ ] 4.3 Send no `Authorization` header and return the peer's text and final
  task state
- [ ] 4.4 Report `input-required` and `auth-required` as terminal, and surface
  `401`/`403` with the peer's `error` code
- [ ] 4.5 Register the tool only when `a2a_call` is in `tools.allowed`
- [ ] 4.6 Add tests against a fake A2A peer covering success, unknown peer,
  auth denial, and terminal states

## 5. Two-agent end-to-end proof (WP12)

- [ ] 5.1 Run two DocsClaw processes with a test issuer and have A call B
  through `a2a_call`
- [ ] 5.2 Assert B logs the verified `sub`, `azp`, `scope`, and `aud`
- [ ] 5.3 Assert with a fake LLM provider that the raw token never appears in
  B's LLM input
- [ ] 5.4 Assert every documented deny code is returned for its triggering
  request

## 6. Signed AgentCards (WP13 — optional)

- [ ] 6.1 Fetch and verify the peer's AgentCard JWS before the first call
- [ ] 6.2 Add the card verification key to the peer configuration
- [ ] 6.3 Add tests for a valid signature, a bad signature, and a missing card
