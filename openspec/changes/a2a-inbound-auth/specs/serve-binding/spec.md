## ADDED Requirements

### Requirement: Inbound auth protects both public endpoints

When `inbound_auth.enabled` is true, the middleware SHALL wrap the A2A
JSON-RPC endpoint and the OpenAI-compatible `/v1` endpoint. Only
`/.well-known/agent-card.json` and `/health` SHALL be exempt. Exemption SHALL
match the request path exactly after `path.Clean`, so `/healthz` and
`/health/../a2a` SHALL require authentication. `/v1` SHALL be protected by the
same middleware, not disabled.

#### Scenario: A2A request without a token is rejected

- **WHEN** a request reaches the A2A endpoint with no token
- **THEN** the response SHALL be `401` with `error` `missing_token`

#### Scenario: OpenAI-compatible request is protected

- **WHEN** a request reaches `/v1` with no token and inbound auth is enabled
- **THEN** the response SHALL be `401` with `error` `missing_token`

#### Scenario: Agent card and health are exempt

- **WHEN** a request targets `/.well-known/agent-card.json` or `/health` with
  no token
- **THEN** the request SHALL be served without authentication

### Requirement: Loopback is the default when authentication is enabled

When inbound auth is enabled and `service.host` is empty, `serve` SHALL bind to
`127.0.0.1`. Otherwise it SHALL bind to `0.0.0.0`. An explicit `service.host`
SHALL always win, and the health server SHALL use the same host.

#### Scenario: Enabled with no host binds loopback

- **WHEN** `inbound_auth.enabled` is true and `service.host` is empty
- **THEN** `serve` SHALL listen on `127.0.0.1`

#### Scenario: Explicit host wins

- **WHEN** `service.host` is set
- **THEN** `serve` SHALL listen on that value regardless of inbound auth

### Requirement: Sessions are isolated by caller

When inbound auth is enabled, the session key SHALL be
`sub|azp|x-session-id`. The raw `x-session-id` SHALL be truncated to 64
characters before composition. An absent or empty `x-session-id` SHALL yield an
empty session key and stateless handling for that request. When inbound auth is
disabled, the existing session key SHALL be unchanged.

#### Scenario: Two callers with the same session id do not share a session

- **WHEN** two requests carry the same `x-session-id` but different verified
  `sub` or `azp` values
- **THEN** the agent SHALL use two different sessions

#### Scenario: Same caller keeps its session

- **WHEN** the same verified identity sends two requests with the same
  `x-session-id`
- **THEN** the agent SHALL reuse the session

#### Scenario: Absent session id is stateless

- **WHEN** an authenticated request has no `x-session-id`
- **THEN** the agent SHALL handle it without a session

#### Scenario: Long session id is truncated before composition

- **WHEN** `x-session-id` is longer than 64 characters
- **THEN** only the first 64 characters SHALL be composed into the session key

### Requirement: Verified identity is logged without secrets

`serve` SHALL log `sub`, `azp`, `scope`, and `aud` for each authenticated
request at info level. It MUST NOT log the raw token or any request header.

#### Scenario: Identity is logged

- **WHEN** a valid token is accepted
- **THEN** the log SHALL contain the verified `sub`, `azp`, `scope`, and `aud`

#### Scenario: Token is absent from the log

- **WHEN** a valid token is accepted
- **THEN** the log SHALL NOT contain the raw token value

### Requirement: The executor uses verified claims

The A2A executor SHALL read identity from the request context via
`ClaimsFrom`. It MUST NOT read or forward the `Authorization` header.

#### Scenario: Identity comes from claims

- **WHEN** the executor needs the caller identity
- **THEN** it SHALL use the claims stored by the middleware
