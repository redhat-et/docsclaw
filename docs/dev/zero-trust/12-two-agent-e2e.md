# WP12: Two-agent end-to-end test implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` first. WP10 and WP11 must be merged.

**Goal:** Prove the Phase 2 definition of done in one automated test:
agent A calls agent B through `a2a_call`, a stand-in supervisor injects a
token from a test issuer, B logs the verified claims, every deny code
surfaces to A's LLM, and the raw token never reaches B's LLM.

**Architecture:** One in-process Go test. B is a real `newServeHandler`
behind `httptest`, with inbound auth on. The "supervisor" is an
`httputil.ReverseProxy` that adds `Authorization: Bearer <minted>`. A is a
real `buildRuntime` whose `a2a_call` peer URL points at the supervisor.
Both use scripted fake providers that record their input.

**Tech stack:** Go `httptest`, `net/http/httputil`, the WP06 fake provider,
and the WP10 test issuer.

**Spec:** parent plan, "Phase 2 definition of done".

## Review focus

- The token appearing in **any** recorded B provider input: system prompt,
  user messages, tool results, or session history. Search all of them, not
  just the last message.
- The token appearing in B's log output or session store contents.

---

### Task 1: E2E test

**Files:**

- Create: `internal/cmd/e2e_a2a_test.go`

- [ ] **Step 1:** Write `TestE2E_AgentACallsAgentB`:
  1. Start the test issuer. B's config: `inbound_auth` (issuer, JWKS URL,
     `audience: engineering-analyst`, `required_scopes: [engineering]`,
     `allowed_callers: [spiffe://openshell.local/openshell/sandbox/]`),
     `tools.allowed: []`. B's fake provider returns `"B says hi"`.
  1. Supervisor proxy: target B, inject a fresh token per request.
  1. A's config: `a2a_peers: [{name: engineering-analyst, url:
     <supervisor URL>}]`, `tools.allowed: [a2a_call]`. A's fake provider
     returns a tool call `a2a_call{peer: engineering-analyst, message:
     "hello"}`, then echoes the tool result as the final text.
  1. Run `rtA.Run(...)`. Assert the final text contains `[completed] B
     says hi`.
  1. Assert B's log buffer contains `sub=alice` and
     `azp=spiffe://openshell.local/openshell/sandbox/alice-assistant`.
  1. Assert the token string does not appear in: any recorded B provider
     input (iterate all messages and the system prompt), B's log buffer,
     A's provider input, or the memory session store snapshot for that
     session.
- [ ] **Step 2:** Write `TestE2E_DenyCodesReachCaller` (table). Each row
  changes the minted claims or B's config, and asserts that A's
  `a2a_call` tool result contains `peer engineering-analyst: <status>
  <code>`. Rows: expired (`401 token_expired`), wrong audience (`403
  invalid_audience`), missing scope (`403 insufficient_scope`), disallowed
  `azp` (`403 caller_not_allowed`), the supervisor injects nothing (`401
  missing_token`), and the JWKS server is stopped (`503 jwks_unavailable`).
  Use fresh B instances so JWKS caching does not leak between rows.
- [ ] **Step 3:** Run `go test -race ./internal/cmd/ -run TestE2E -v`.
  Expected: PASS. If a row fails, fix the owning WP's code in this branch
  and mention it in the PR description.
- [ ] **Step 4:** Add a "Local two-agent demo" section to `deploy/README.md`
  with the manual equivalent: two `docsclaw serve` processes, and a
  `curl`-based token from any test issuer. Keep it under 15 lines and
  point at the test for the authoritative setup.
- [ ] **Step 5:** Run `make fmt lint test`. Commit:
  `test(e2e): prove A-to-B auth, deny codes, and token isolation`.
