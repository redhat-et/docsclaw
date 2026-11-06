# Zero-trust work packages: index

This directory splits
[`../2026-11-06-docsclaw-zero-trust-plan.md`](../2026-11-06-docsclaw-zero-trust-plan.md)
(the "parent plan") into work packages (WPs) that you can hand to
individual agents. Each WP is self-contained: it lists the files, the
interfaces it consumes and produces, tests with exact assertions, and
commit points.

Every agent must read this index (global constraints and decisions)
and its own WP file. Agents read the parent plan only for background.

## Dependency graph

```text
Phase 1                                   Phase 2
WP01 spike ──► WP02 openspec-p1           WP08 openspec-p2
   │                │                        │
   │           WP03 remove-v1 ──► WP04 egress ──► WP05 docservices ──► WP06 ask
   │                                                   │                  │
   └──────────► WP07 image                             │   WP09 inboundauth (no deps)
                                                       │        │         │
                                                       └──► WP11 a2a_call  WP10 wire-auth ◄─ WP06
                                                                 │         │
                                                                 └──► WP12 e2e ◄┘
                                                                       WP13 signed cards (optional)
```

| WP | File | Parent step | Depends on | Can run in parallel with |
| --- | --- | --- | --- | --- |
| 01 | `01-openshell-spike.md` | 1.2, 1.4, 1.7 research; open questions 1-2 | none | 03, 09 |
| 02 | `02-phase1-openspec.md` | working agreements | 01 | 03, 09 |
| 03 | `03-remove-v1-identity.md` | 1.1 (+ document mode) | none | 01, 02, 09 |
| 04 | `04-egress-clients.md` | 1.2 | 03 | 07, 09 |
| 05 | `05-document-services.md` | 1.3, 1.4 | 01, 04 | 07, 09 |
| 06 | `06-ask-command.md` | 1.5, 1.6 | 05 | 07, 09 |
| 07 | `07-openshell-image.md` | 1.7 | 01 | 03-06, 09 |
| 08 | `08-phase2-openspec.md` | working agreements | 02 | 03-07, 09 |
| 09 | `09-inboundauth-package.md` | 2.1 | none | everything |
| 10 | `10-wire-inbound-auth.md` | 2.2 | 06, 08, 09 | 11 |
| 11 | `11-a2a-call-tool.md` | 2.3 | 05, 08 | 10 |
| 12 | `12-two-agent-e2e.md` | Phase 2 DoD | 10, 11 | 13 |
| 13 | `13-signed-cards.md` | 2.4 (optional) | 11 | 12 |

WP03 through WP06 all edit `internal/cmd/serve.go`. Run them one after
another, never in parallel. WP09 touches only `pkg/inboundauth/` and
`go.mod`, so it can start on day one.

## Global constraints (apply to every WP)

- Branch per WP, using the user's naming scheme. Examples:
  `refactor/remove-v1-identity`, `feature/inboundauth`,
  `docs/zero-trust-spike`. Never commit to `main`. Open a draft PR with
  `gh pr create --draft`.
- Go: `go 1.26.0` floor in `go.mod` (local toolchain is 1.27.3). Use the
  latest stable versions of new dependencies. Keep
  `github.com/a2aproject/a2a-go/v2` current (now `v2.4.0`; run
  `go list -m -u github.com/a2aproject/a2a-go/v2` before starting a
  Phase 2 WP).
- Follow `~/.claude/go-conventions.md`: stdlib first, `log/slog`,
  minimal dependencies.
- Run `make fmt lint test` before every commit. Commit with `git commit -s`
  and the trailer `Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>`.
- Every security decision gets a table-driven test with one row for
  allow and one row per deny reason.
- Clean break: no compatibility shims, deprecated flags, or aliases for
  v1-era settings.
- Markdown follows `~/.claude/markdown-style.md`: blank lines around
  headings, lists, and code blocks; sentence-case headings; `1.` for
  ordered lists. Run the `markdown-lint` skill on changed `.md` files.
- Never log or return HTTP headers, bearer tokens, or API keys. Never
  pass a raw inbound token to the LLM, tools, or session store.
- If the code you find disagrees with your WP (a file moved, or a
  function has a different signature), stop and report the
  disagreement. Do not improvise a different design.

## Decisions made while splitting the plan

The user should confirm these before WP02. Each is marked
**DECISION** where it is used.

1. **Remove A2A "document mode".** `internal/bridge/executor.go`
   currently detects a document ID in the incoming message, reads the
   caller's `Authorization` value from `ServiceParams`, and forwards
   that raw token to the document service (`FetchDocument(ctx, id,
   bearerToken)`). That is token passthrough, which v2 forbids. WP03
   deletes document mode (`FetchDocument`, `ProcessLLM`,
   `ExtractDocumentID`). Every A2A message goes through
   `ProcessMessage`. Without `agent-config.yaml`, `ProcessMessage` makes
   one `Complete` call; with it, `ProcessMessage` runs the tool loop.
1. **Keep the tool name `fetch_document`** (open question 3). Ten demo
   configs under `demo/batch/` and `testdata/` already list it in
   `tools.allowed`, and the name is still accurate. Only the parameters
   change: `service` and `document_id`.
1. **New package `internal/egress`.** It owns every outbound HTTP
   concern for tools: client construction (WP04), and denial
   description and call logging (WP05). WP11 reuses it.
1. **Delete `--document-service-url`** and `document_service_url`. The
   `document_services` map in `agent-config.yaml` replaces them.
   WP05 updates the demo YAML that referenced them.
1. **Session isolation under inbound auth.** When inbound auth is on,
   the session key becomes `sub|azp|x-session-id`. Without this, one
   authorized caller could read another caller's conversation by
   guessing a session ID (WP10).
1. **Loopback default.** `service.host` defaults to `""`. `serve`
   resolves it to `127.0.0.1` when inbound auth is enabled and to
   `0.0.0.0` otherwise. An explicit value always wins. The health
   server uses the same host (WP10).

## Stable inbound-auth error codes (used by WP09, WP10, WP12)

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
`WWW-Authenticate: Bearer realm="docsclaw", error="<code>"`.

## Execution order recommendation

1. Day one, in parallel: WP01, WP03, WP09.
1. WP02 after WP01; the user reviews the OpenSpec design (this is the
   "confirm before implementing" gate from the parent plan).
1. WP04, WP05, WP06 in order; WP07 in parallel once WP01 is done.
1. WP08, then WP10 and WP11 in parallel, then WP12. WP13 only if time
   allows.
