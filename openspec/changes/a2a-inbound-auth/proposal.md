## Why

OpenShell's `bearer-passthrough` service exposure forwards one bearer token
unchanged, but it also admits requests that carry no token at all. An agent
exposed this way has no authentication unless the application performs it, so
any client that can reach the gateway can drive the agent and read its session.

Phase 2 makes DocsClaw authenticate incoming A2A and `/v1` calls itself, bind
the verified identity to the session, and call peer agents through the
supervisor without ever holding a credential.

## What Changes

- Add `pkg/inboundauth`, a standalone `net/http` middleware that verifies JWTs
  against a JWKS, enforces issuer, audience, scope, and caller rules, strips the
  raw token, and returns a stable error code for every denial. (Already
  implemented; this change records its contract.)
- Add an `inbound_auth` block to `agent-config.yaml` and apply the middleware
  to the A2A JSON-RPC endpoint and the OpenAI-compatible `/v1` endpoint, exempting
  only the agent card and health paths.
- **BREAKING** — when inbound auth is enabled, `serve` binds to `127.0.0.1` by
  default instead of `0.0.0.0`. An explicit `service.host` still wins.
- Isolate sessions by caller: the session key becomes `sub|azp|x-session-id`
  when inbound auth is on, so one authorized caller cannot read another
  caller's conversation by guessing a session ID.
- Add an `a2a_call` tool that invokes a configured peer agent by name over A2A.
  The agent sends no bearer token; the OpenShell supervisor injects it.
- Log the verified identity (`sub`, `azp`, `scope`, `aud`) per request at info
  level, and never log the token or any header.
- Prove with an end-to-end test that the raw token is absent from the LLM input.

## Capabilities

### New Capabilities

- `inbound-auth`: verification of incoming bearer tokens, the stable deny
  codes, token stripping, and the configuration surface.
- `a2a-call-tool`: the `a2a_call` tool, peer configuration, and how supervisor
  denials and A2A task states are surfaced to the LLM.
- `serve-binding`: how `serve` chooses its listen address and applies the
  middleware to the A2A and `/v1` endpoints, including session isolation.

### Modified Capabilities

None. The existing workspace specs are unaffected.

## Impact

- `pkg/inboundauth/` (new, already merged), `go.mod`, `go.sum`.
- `internal/cmd/agentconfig.go`, `internal/config/config.go`,
  `internal/cmd/runtime.go`, `internal/cmd/serve.go`.
- `internal/bridge/executor.go` (identity from claims instead of the
  `Authorization` header), `internal/bridge/client.go`.
- `internal/a2acall/` (new).
- `deploy/README.md` and the demo `agent-config.yaml` files.
- The issuer's JWKS host must be reachable through the sandbox network policy,
  because the JWKS fetch is egress like any other request.
