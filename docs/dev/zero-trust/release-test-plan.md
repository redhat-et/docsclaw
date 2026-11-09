# Release test plan: v0.20.0 (Phase 1) and v0.21.0 (Phase 2)

Test plan for the zero-trust v2 line, from commit `db32804` (2026-11-07). The
automated suite proves the security contracts in process; this plan covers what
it cannot: real provider wire formats, the real sandbox boundary, real release
artifacts, and regression of everything the zero-trust work touched on the way.

## Scope

| Item | Value |
| ---- | ----- |
| Code under test | `db32804` — WP01–WP13 plus follow-ups #120, #121 |
| Phase 1 tag target | `v0.20.0` — outbound-only sandbox agent |
| Phase 2 tag target | `v0.21.0` — inbound auth and agent-to-agent calls |
| Last v1 release | `v0.16.0` (2026-11-06) — no `-openshell` image is published |
| Authoritative specs | `docs/dev/2026-11-06-docsclaw-zero-trust-plan.md`, `openspec/changes/` |
| Target environment | `~/work/zero-trust-agents-v2` — the v2 stack is live on this Mac |
| Open follow-ups | #122 (egress allowlist), #124 (test hardening), #125 (docs/CI/image) |

## What the automated suite already proves

Do not re-test these by hand; re-run them and check they are still green.

| Test | Guarantee |
| ---- | --------- |
| `pkg/inboundauth/*_test.go` | Every deny code, ES256/RS256 only, `alg: none` and `HS*` rejected, JWKS caching and refresh, `Authorization` stripped before `next` |
| `internal/cmd/serve_auth_test.go` | Middleware wired to A2A and the whole `/v1` tree; card and health exempt; loopback bind default |
| `internal/cmd/e2e_a2a_test.go` | A→B over a stand-in supervisor; six deny codes reach A; raw token absent from B's prompt, messages, logs and session store |
| `internal/a2acall/cardverify_test.go` | Signed-card verify, tampered card, wrong key, unsigned card, cross-host redirect, failure not cached forever |
| `internal/egress/*_test.go` | Denial classification (`policy_denied`, `ssrf_denied`, 403/502 bodies), source-scan guard on transports |
| `internal/fetchdoc/*_test.go` | Named services, unknown service error, denial passthrough |
| `internal/cmd/ask_test.go`, `runtime_test.go` | `ask` exit codes (2 for usage), stdout/stderr separation, `--trace` |
| `internal/session/*_test.go` | Per-caller session key `sub\|azp\|session-id` |

Baseline: `go test ./...` passes on this checkout (verified 2026-11-07, exit 0).

## Environment

The sandbox tiers run against the **Zero Trust Agents v2** stack at
`~/work/zero-trust-agents-v2`, not against the standalone local gateway. That
stack is up right now and its harness defines how the CLI is invoked:

```bash
OS=(openshell --gateway zt2 --gateway-endpoint http://127.0.0.1:18083)
"${OS[@]}" sandbox list
```

`scripts/drivers/lib-podman.sh` sets exactly this array. The stored `zt2`
metadata points at a ROSA endpoint and is unreachable; the `--gateway-endpoint`
override is what makes it work locally, so never rely on the stored endpoint.

### The v2 stack as verified on 2026-11-07

| Component | Container | Publish |
| --------- | --------- | ------- |
| Gateway | `zt2-gateway` (`nvidia/openshell/gateway:0.1.6`) | `127.0.0.1:18083` HTTP, `:18084` |
| Token issuer | `zt2-token-issuer` (alias `token-issuer.zt-demo.svc`) | `:8080` exchange, `:8090` demo subject tokens |
| Document services | `zt2-document-{engineering,finance,admin,hr,public}` | `:18101`–`:18105` |
| SPIFFE | `zt2-spire-server`, `zt2-spire-agent`, `zt2-spire-oidc` | OIDC + JWKS on `:18081` |
| Sandboxes | podman containers plus one supervisor sidecar each | e.g. `alice-s1` Ready |

Read-only probes that answered: `$OS sandbox list` → `alice-s1`;
`$OS profile list` → four `zt2-*` profiles; `$OS provider list` → providers
created by earlier scenario runs; `$OS service list` → nothing exposed yet;
`GET :8090/demo-subject-token?user=alice` → an HS256 token with
`iss=http://token-issuer.zt-demo.svc:8080`, `sub=alice`,
`departments=[engineering,finance]`, `scope=openid profile email`.

Host tooling: Podman 6.1.4 (`linux/arm64` machine), OpenShell CLI 0.1.6, `a2a`
at `~/go/bin/a2a`, Podman connection `rhel` (`panni@rhel01.internal`) for the
x86_64 tier. The `-openshell` image is **not published**, so it must be built
locally; `make image-openshell` defaults to arm64, which matches the machine.

### How a sandbox is put together here

1. A sandbox is a podman container from an image (`--from`), plus a supervisor
   container that mediates its network. `alice-s1` today runs
   `curlimages/curl`.
1. Capability comes from a **provider profile**: credentials, `endpoints`
   (host/port), and `binaries` (which executables may use the credential and
   reach those endpoints).
1. On a request to a bound endpoint the supervisor exchanges the sandbox
   SVID for an access token (RFC 8693) and injects `Authorization: Bearer`.
   The workload never holds a token. `deploy/podman/profiles/*.yaml.in` show
   the shape, including `audience_overrides` per host.
1. The sandbox's SPIFFE ID must be registered in SPIRE
   (`deploy/podman/spire/register-sandbox.sh`, wrapped by `register_sandbox` in
   `scripts/lib.sh`). Without it the exchange fails and every request is
   refused.

### Two verified integration facts that constrain Phase 2

1. **v2 access tokens are HS256 over a shared secret, and the issuer serves no
   JWKS.** `internal/tokens/tokens.go` signs HS256; `:8080` exposes only
   `/healthz` and `POST /token`, and `:8090` only `/demo-subject-token`.
   `pkg/inboundauth` accepts ES256/RS256 and rejects every `HS*` by design, so
   **DocsClaw cannot verify a v2-issued token today**. This blocks T4b, not
   T3: on the outbound path DocsClaw is only an HTTP client — the supervisor
   injects and the document service verifies.
1. **The claims models differ.** v2 puts departments in a `departments` claim
   with `scope=openid profile email` and no `azp`; DocsClaw checks
   `required_scopes` and `allowed_callers` (azp or client_id). A mapping
   decision is needed before the two systems can share an issuer.

### Test matrix

| Tier | Where | Container | ~Time | Risk covered |
| ---- | ----- | --------- | ----- | ------------ |
| T0 | macOS | No | 3 min | Regression, clean break, static guards |
| T1 | macOS | No | 45 min | Real LLM wire format, inbound auth over HTTP, two-process A2A |
| T2 | macOS | Podman | 15 min | Image contract (UID, paths, shell, `sleep`, read-only rootfs) |
| T3 | macOS + v2 stack | OpenShell | 60 min | DocsClaw as the agent in scenarios 1–6: exchange injection, policy denial, LLM placeholder |
| T4a | macOS | No | 45 min | Phase 2 contract with a test issuer: exposure, inbound auth, `a2a_call`, deny codes |
| T4b | macOS + v2 stack | OpenShell | 30 min | v2-issuer integration — **blocked**, requirements listed |
| T5 | RHEL homelab | Podman + OpenShell | 60 min | x86_64 artifacts, same v2 stack, host-OS differences |
| T6 | macOS | No | 60 min | Non-zero-trust features (skills, MCP, RAG, batch, `/v1`, K8s) |
| T7 | macOS | Podman | 20 min | Release artifacts, multi-arch manifests |

Gate: T0 and T7 block the tag. T1 and T3 prove DocsClaw's own contracts and its
fit in the v2 stack; T4a proves Phase 2 end to end; T4b blocks the integrated
two-agent claim. T5 blocks the x86_64 demo deployment. T6 blocks a "no
regressions" statement.

## T0 — Static and unit

```bash
make fmt lint test
go vet ./...
go test -race -count=1 ./pkg/inboundauth/ ./internal/egress/ ./internal/a2acall/ ./internal/cmd/
```

Clean-break acceptance (plan section 1.1). Use word boundaries: a bare `opa`
pattern also matches `otel/propagation` in `internal/bridge`.

```bash
grep -rniE '\bspiffe\b|\bopa\b|x-delegation' --include='*.go' . | grep -v '_test.go'
```

Expected: no output. The only remaining hits anywhere are `spiffe://` caller
identities in tests, which are intentional.

Removed surfaces must fail loudly, not silently (verified on `db32804`):

```bash
./bin/docsclaw serve --mock-spiffe;              echo $?   # Error: unknown flag, 1
./bin/docsclaw serve --opa-host x;               echo $?   # Error: unknown flag, 1
./bin/docsclaw serve --document-service-url x;   echo $?   # Error: unknown flag, 1
```

Note the asymmetry: unknown flags exit **1**, while arg-count mistakes in `ask`
exit **2** (the WP06 contract). Both must print to stderr and never start a
server.

Coverage check on the security packages — the deny paths matter more than the
happy path:

```bash
go test -cover ./pkg/inboundauth/ ./internal/egress/ ./internal/a2acall/
```

## T1 — Local processes, no containers

### T1.1 `ask` against a stub document service and a real provider

This tier and T3 are the only ones that use a real LLM provider; every automated
test uses a fake one. That makes T1 the first check of the provider tool-call
wire format.

```bash
mkdir -p /tmp/dc-stub/documents
printf '%s' '{"title":"Engineering Onboarding","content":"Step 1: read the runbook."}' \
  > /tmp/dc-stub/documents/DOC-001
python3 -m http.server 9090 --directory /tmp/dc-stub &
```

Config dir `/tmp/dc-a` with `system-prompt.txt` and:

```yaml
document_services:
  engineering: http://127.0.0.1:9090
tools:
  allowed: [fetch_document]
loop:
  max_iterations: 10
```

```bash
export LLM_PROVIDER=anthropic LLM_API_KEY=... LLM_MODEL=...
./bin/docsclaw ask --config-dir /tmp/dc-a --trace \
  "Summarize DOC-001 from engineering"
```

Pass: stdout is the answer only; stderr shows `→ fetch_document` and
`← fetch_document ok:`; the answer mentions the title. Repeat with
`LLM_PROVIDER=openai` and `LLM_BASE_URL` against the MaaS endpoint used by the
demo — the placeholder path in T3 reuses this exact config.

### T1.2 `ask` failure contract

| Invocation | Expected |
| ---------- | -------- |
| `ask` with no prompt | exit 2, usage on stderr |
| `ask "x"` with `LLM_API_KEY` unset | exit 1, `LLM API key not configured` |
| `ask --config-dir /nonexistent "x"` | exit 1, clear config error |
| Stub server killed mid-run | exit 1, tool failure surfaced, no hang |

### T1.3 Error vocabulary from the tool layer

```bash
# Unknown service: the enumeration is in the tool result the LLM sees
./bin/docsclaw ask --config-dir /tmp/dc-a --trace "Fetch DOC-001 from finance"
```

Pass: result contains `unknown document service "finance"; available: engineering`.

### T1.4 Document-service denial is verbatim

Replace the stub with one that answers `403` and `{"error":"invalid_audience"}`
(any small reverse proxy or a Python handler), then re-run T1.1. Pass: the tool
result and the info log carry `403 invalid_audience`, not "access denied".

### T1.5 `serve` with inbound auth — token needed

The live HTTP tests need an issuer. Two options:

1. Lift `pkg/inboundauth/testissuer_test.go` into `hack/testissuer` (recommended,
   ~60 lines, no new dependencies — it already mints ES256 tokens and serves
   JWKS). Expose `GET /token?sub=&azp=&aud=&scope=&ttl=` plus `/jwks.json`.
1. Skip T1.5–T1.9 and rely on `go test -race -run TestE2E -v ./internal/cmd`,
   which covers the same contracts in process. Do not skip T3: the sandbox
   boundary cannot be tested in process.

With option 1, agent config `/tmp/dc-b/agent-config.yaml`:

```yaml
inbound_auth:
  enabled: true
  issuer: http://127.0.0.1:9099
  jwks_url: http://127.0.0.1:9099/jwks.json
  audience: engineering-analyst
  required_scopes: [engineering]
  allowed_callers: [spiffe://openshell.local/openshell/sandbox/]
```

```bash
./bin/docsclaw serve --config-dir /tmp/dc-b -p 8081 &
TOKEN=$(curl -s 'http://127.0.0.1:9099/token?sub=alice&azp=spiffe://openshell.local/openshell/sandbox/alice-assistant&aud=engineering-analyst&scope=engineering&ttl=300' | jq -r .token)
```

| Probe | Expected |
| ----- | -------- |
| `curl -s localhost:8081/.well-known/agent-card.json` | 200, no token |
| `curl -s localhost:8180/health` | 200, no token (separate health server) |
| `curl -s -X POST localhost:8081/a2a -H 'content-type: application/json' -d '{}'` | 401 `missing_token`, `WWW-Authenticate` present |
| same with `-H "Authorization: Bearer $TOKEN"` | not 401 — JSON-RPC error about the empty body is fine |
| `curl -s localhost:8081/v1/models` | 401 `missing_token` (the whole `/v1` tree is protected) |
| `curl -s localhost:8081/v1/models -H "Authorization: Bearer $TOKEN"` | 200 |
| `ss -ltn` / `lsof -iTCP:8081` | bound to `127.0.0.1` only |
| stderr log for an authenticated request | `request authenticated sub=alice azp=… scope=engineering aud=engineering-analyst`, no token |
| `grep -r "$TOKEN" /tmp/dc-b` | no match |

### T1.6 Deny matrix over HTTP

Mint one token per row and assert the exact code. Rows: expired (`401
token_expired`), `nbf` in the future (`401 token_not_yet_valid`), wrong `iss`
(`401 invalid_issuer`), wrong `aud` (`403 invalid_audience`), no required scope
(`403 insufficient_scope`), `azp` outside the allowlist (`403
caller_not_allowed`), two `Authorization` headers (`401 invalid_token`), and
stop the issuer then mint from cache-absent state (`503 jwks_unavailable`).

Also assert the negative: a token signed with a different key, and a token with
`alg: none` or `alg: HS256`, all `401 invalid_token`.

### T1.7 Session isolation

Two tokens with different `sub`, same `x-session-id`, same message twice. Use
`curl` (the `a2a` CLI cannot set arbitrary HTTP headers; `--svc-param` does not
reach `ServiceParams` for transport headers):

```bash
SID=$(uuidgen)
A_TOKEN=$(curl -s 'http://127.0.0.1:9099/token?...&sub=alice&ttl=300' | jq -r .token)
B_TOKEN=$(curl -s 'http://127.0.0.1:9099/token?...&sub=bob&ttl=300' | jq -r .token)
```

Send "remember the number 7" as Alice, then "what number did I say?" as Bob with
the same `-H "x-session-id: $SID"`. Pass: Bob does not see Alice's turn; with
`--session-db /tmp/dc-b/sessions.db` the store contains two keys and the file is
mode `0600`.

### T1.8 Two-process A2A (manual WP12)

B as in T1.5. A with `a2a_peers: [{name: engineering-analyst, url:
http://127.0.0.1:7000}]`, `tools.allowed: [a2a_call]`, plus a request logger in
front of B that injects `Authorization: Bearer $TOKEN`. Pass: A's final answer
contains B's text; B logs verified claims; neither A's log nor B's log contains
the token.

### T1.9 Signed AgentCard (WP13)

```bash
openssl ecparam -name prime256v1 -genkey -noout -out /tmp/card.key
openssl ec -in /tmp/card.key -pubout -out /tmp/card.pub
# Produce the A2A detached-JWS card file (reuse the fixture builder in
# internal/a2acall/cardverify_test.go or write ~40 lines with jwx), then:
AGENT_CARD_SIGNED_PATH=/tmp/dc-b/agent-card.signed.json ./bin/docsclaw serve --config-dir /tmp/dc-b
```

A side: peer gets `card_public_key: /tmp/card.pub`. Pass: first call succeeds;
tampering with the card file body gives `peer engineering-analyst: agent card
signature invalid`; deleting the file and restarting gives the unsigned-card
fallback and, with `card_public_key` set, a signature failure — not a crash.

## T2 — OpenShell image

```bash
make image-openshell
podman image inspect docsclaw:dev-openshell \
  --format '{{.Config.User}} | {{.Config.WorkingDir}} | {{json .Config.Cmd}} | {{json .Config.Entrypoint}}'
```

| Check | Command | Expected |
| ----- | ------- | -------- |
| User | inspect | a numeric pair with **non-zero UID and GID** |
| Workdir | inspect | `/workspace` |
| CMD | inspect | `["sleep","infinity"]`, no ENTRYPOINT |
| Binary path | `podman run --rm docsclaw:dev-openshell ls -l /usr/local/bin/docsclaw` | real file, not a symlink |
| Shell | `podman run --rm docsclaw:dev-openshell bash -lc 'echo ok'` | `ok` |
| Version | `podman exec … docsclaw --version` | the injected version (a local build reports `dev`) |
| Config error path (#125) | `docsclaw ask --config-dir /nonexistent x` | exit 1, clear message |
| Writable workspace | `podman run --rm docsclaw:dev-openshell bash -lc 'touch /workspace/x && echo ok'` | `ok` |
| Read-only rootfs | `podman run --rm --read-only -v dc-ws:/workspace docsclaw:dev-openshell bash -lc 'touch /workspace/x'` | `ok` |
| Non-root euid | `podman run --rm docsclaw:dev-openshell id` | `uid=1001 gid=1001` |
| `tar` present | `podman run --rm docsclaw:dev-openshell tar --version` | needed by `sandbox upload` |

**Defect found by this plan, now fixed (2026-11-09):** the image shipped
`USER 1001:0` and OpenShell refused to start the sandbox with
`descriptor error: workload identity must not contain UID or GID zero`. Fixed in
PR #126 (`9c7c2a3`) with `USER 1001:1001`. The GID assertion above is the guard
that catches a regression.

**Second gap, now fixed:** UBI minimal has no `tar`, so `openshell sandbox
upload` failed with `tar: command not found`. Fixed in PR #127 (`b390422`), which
installs `tar` and documents the upload destination semantics.

Also confirm `oras`/`curl` absence is acceptable: the image is a runtime, not a
toolbox. If the demo scripts need `curl` for health checks, add it to the
Containerfile or use `docsclaw ask`.

## T3 — DocsClaw as the agent in the v2 scenarios

This is the headline test: replace the harness's `curl` with `docsclaw ask` and
show the documented scenarios still hold. Scenario 1 is the shape to mirror:

```bash
# scripts/scenario1.sh, the part DocsClaw replaces
sandbox_curl alice-s1 "$ENGINEERING_URL/documents/DOC-001" "Engineering Roadmap"
```

### T3.0 Make the image visible to the gateway

```bash
make image-openshell                    # arm64 by default; matches the podman machine
$OS sandbox create --name dc-probe --from docsclaw:dev-openshell -- sleep infinity
$OS sandbox exec --name dc-probe -- bash -lc 'id; ls -l /usr/local/bin/docsclaw'
$OS sandbox exec --name dc-probe -- bash -lc 'env | grep -iE "proxy|LLM_"; echo "SSL_CERT_FILE=$SSL_CERT_FILE"'
```

Record the driver, whether `/sandbox` exists next to `/workspace`, whether proxy
variables are stripped, and whether a CA file is injected. If the gateway cannot
resolve a locally built image, fall back to a throwaway `registry:2` on
`127.0.0.1:5000` and `--from 127.0.0.1:5000/docsclaw:dev-openshell`.

### T3.1 Capability and model profiles

Two profiles: the document capability (the shipped template, plus the DocsClaw
binary) and the model endpoint.

```bash
# Same RFC 8693 grant as the demo, with the DocsClaw binary authorized too.
sed -e "s/@DOC_HOST@/$(doc_endpoint_host engineering)/" \
    -e "s/@DOC_PORT@/$(doc_endpoint_port engineering)/" \
    -e "$(doc_allowed_ips_subst)" \
    -e 's#^  - /usr/bin/curl$#  - /usr/bin/curl\n  - /usr/local/bin/docsclaw#' \
    deploy/podman/profiles/alice-engineering.yaml.in > /tmp/dc-alice-eng.yaml
$OS profile lint -f /tmp/dc-alice-eng.yaml
$OS profile import -f /tmp/dc-alice-eng.yaml
```

Model profile, retargeted from `docs/OPENCLAW_ON_OPENSHELL.md`:

```yaml
id: dc-model
display_name: DocsClaw model
category: inference
inference_capable: true
credentials:
  - name: api_key
    description: MaaS key
    env_vars: [LLM_API_KEY]
    required: true
    auth_style: bearer
    header_name: authorization
endpoints:
  - host: api.enmaas.devshift.net
    port: 443
    protocol: rest
    access: read-write
    enforcement: enforce
binaries: [/usr/local/bin/docsclaw]
```

```bash
$OS profile lint -f /tmp/dc-model.yaml
$OS profile import -f /tmp/dc-model.yaml
$OS provider create --name dc-model --type dc-model --credential api_key=...
```

The endpoint host must equal `LLM_BASE_URL`'s host, and `binaries` must name the
image's real binary path. OpenShell pins the first observed digest per
authorized path, so a symlink or a binary replaced inside a running sandbox
breaks authorization until the sandbox restarts.

### T3.2 Sandbox and config

```yaml
# /tmp/dc-sandbox-config/agent-config.yaml
document_services:
  engineering: http://<doc_endpoint_host(engineering)>:<doc_port>
tools:
  allowed: [fetch_document]
loop:
  max_iterations: 10
```

```bash
# Mint a fresh subject token per run: they expire in 30 minutes (see T3.7).
SUBJECT=$(curl -fsS "http://127.0.0.1:8090/demo-subject-token?user=alice" | jq -r .access_token)
$OS provider create --name dc-alice-eng --type zt2-alice-engineering-v2 \
  --credential "subject_token=$SUBJECT"
$OS sandbox create --name dc-alice --from docsclaw:dev-openshell \
  --provider dc-alice-eng --provider dc-model \
  --env LLM_PROVIDER=openai --env LLM_BASE_URL=... --env LLM_MODEL=... \
  -- sleep infinity
ID=$($OS sandbox get dc-alice | awk '/Id:/ {print $2; exit}')
deploy/podman/spire/register-sandbox.sh "$ID"     # no SPIFFE entry, no exchange

# DEST is the parent directory, so this lands as /workspace/config/.
$OS sandbox upload --no-git-ignore dc-alice /tmp/dc-sandbox-config /workspace
```

`/config` is not writable (the root filesystem is root-owned, the process is
1001), so the config lives under `/workspace`. Before #127 added `tar` to the
image, the same files had to be written over stdin.

### T3.3 The scenarios, with DocsClaw as the agent

```bash
$OS sandbox exec --name dc-alice -- bash -lc \
  'docsclaw ask --config-dir /workspace/config --trace "Summarize DOC-001 from engineering"'
```

| Scenario | Setup | Expected |
| -------- | ----- | -------- |
| 1 | Alice + engineering capability, engineering doc | answer mentions `Engineering Roadmap`; supervisor log shows the exchange |
| 2 | Alice + hr capability, engineering doc | `502 Bad Gateway` in the tool result, `Token grant failed` in the supervisor log |
| 3 | no delegation (no subject token) | same shape as scenario 2 |
| 5 | sandbox SPIFFE ID not registered | the exchange fails; no document content reaches the answer |
| 6 | — | not a DocsClaw case: the replay is staged out-of-band with registered SVIDs, and DocsClaw never holds a token to replay. Leave it with the harness |

The denial vocabulary is the point, and it differs by who refused:

- The **document service** returns `{"error":"invalid_audience"}`, or
  `insufficient_scope`, or `invalid_token`. `internal/egress` keeps those
  verbatim (`403 invalid_audience`).
- The **supervisor** returns a **bare `502`** for a refused exchange: the body
  carries no `error` field, so `internal/egress` falls back to
  `502 Bad Gateway` and the reason exists only in the supervisor log
  (`Token grant failed`). Observed 2026-11-09 with an expired subject token;
  this corrects the source-derived expectation of `502 token_grant_failed` in
  `openshell-notes.md`. Whether the body is empty or unparseable is worth
  confirming in T3.4.
- A **policy** denial becomes `blocked by sandbox network policy` (T3.4).

An answer that says only "access denied", or a 502 flattened into a policy
block, is a regression.

Also assert the credential never leaks: `env` inside the sandbox shows
`LLM_API_KEY=openshell:resolve:env:…`, and neither `$OS logs dc-alice` nor the
supervisor log carries an `Authorization` value.

### T3.4 Denial shape probes — close the four unverified items

Put a second config in the same sandbox pointing `fetch_document` at hosts the
profile does not bind, one `http://` and one `https://`. This settles items 1–3
of "Unverified items" in `openshell-notes.md`; item 4 is settled by
`$OS service list -o json` in T3.5.

| Target | Expected tool result | Record |
| ------ | -------------------- | ------ |
| unbound `http://` host | `403 policy_denied` → `blocked by sandbox network policy` | status, body, headers |
| unbound `https://` host | `blocked by sandbox network policy` via the Go TLS string | exact Go error text |
| bound but down host | `502 upstream_unreachable` | status reaches the tool result |

### T3.5 Exposure: OpenShell admits, DocsClaw refuses

```bash
$OS sandbox exec --name dc-alice -- bash -lc \
  'docsclaw serve --config-dir /config/agent-b -p 8080 &'
$OS service expose dc-alice 8080 agent --authorization-mode bearer-passthrough
$OS service list -o json                        # settle unverified item 4
```

Against the exposed URL, from the Mac:

| Probe | Expected |
| ----- | -------- |
| `GET /.well-known/agent-card.json`, no token | 200 (exempt) |
| `POST /a2a`, no token | **401 `missing_token`** — the reason `pkg/inboundauth` exists |
| `GET /v1/models`, no token | 401 |
| `GET :8180/health` inside the sandbox | 200, no token |
| `$OS logs dc-alice` | no token, no `Authorization` value |

The authenticated 200 case needs a token DocsClaw can verify, which the v2
issuer cannot mint — that belongs to T4a (test issuer) and T4b (integration).

### T3.6 Session store inside a sandbox

`--session-db /workspace/sessions.db`; two execs share history; file mode
`0600`; `$OS sandbox stop` then `start` preserves it.

### T3.7 First run: scenario 1 result (2026-11-07)

Run on the live zt2 stack, twice: first with a hand-built GID-fixed variant
(2026-11-07), then with the image built from merged `main` after #126 and #127
(2026-11-09), config delivered by `sandbox upload`:

```console
→ fetch_document {"document_id":"DOC-001","service":"engineering"}
outbound call tool=fetch_document host=192.168.127.254.nip.io:18101 status=200 reason=ok
← fetch_document ok: **Document:** Engineering Roadmap ...
```

**PASS** on both. The agent reached the document service through the
supervisor, the answer names `Engineering Roadmap` as scenario 1 asserts, and
the model was reached with the placeholder key. Verified alongside:

| Check | Result |
| ----- | ------ |
| `LLM_API_KEY` in the workload | `openshell:resolve:env:v13319767025953453690_LLM_API_KEY` (revision-prefixed, as the notes predicted) |
| JWT-shaped strings under `/workspace` | none |
| JWT-shaped strings in supervisor logs | none |
| DocsClaw stderr / `--trace` output | tool name, host, status; no `Authorization` value |
| `docsclaw --version` in a local build | `dev` — expected without ldflags |

Both image defects above were found here, not in T2: T2 as originally written
would have passed the broken image. That is why T2 now asserts a non-zero GID.

**New finding — the demo subject token expires in 30 minutes.** Re-running the
scenario against a sandbox whose provider was created two days earlier failed
with a bare `502 Bad Gateway` while the LLM path kept working:

```console
outbound call tool=fetch_document host=...:18101 status=502 reason=502 Bad Gateway
# supervisor log: WARN openshell_supervisor_network::l7::token_grant_injection: Token grant failed
```

`GET :8090/demo-subject-token` returns a token with `exp - iat = 1800`. Mint a
fresh subject token and a fresh provider per run, as the v2 harness does
(`provider create --name ...-$$` per scenario); reusing a provider silently ages
the whole sandbox out of its capability. The document service itself was
healthy (`401` without a token on `:18101`).

## T4a — Phase 2 contract with an ES256 test issuer

Not the v2 issuer: `pkg/inboundauth` needs ES256/RS256 plus a JWKS, and the v2
issuer offers neither. This tier proves the DocsClaw contract — verification,
deny codes, `a2a_call`, token isolation — with the test issuer from T1.5.

Note on injection: a sandbox never needs a placeholder to get a token. The
supervisor injects `Authorization: Bearer` on **any** request to a bound
endpoint, which is why `sandbox_curl` in the v2 harness sends no credentials.
`a2a_call` therefore works the moment A's profile binds B's host — no agent-side
credential handling at all.

Two shapes, cheapest first:

1. **Two local processes** (T1.8): B behind an injecting proxy, A with an
   `a2a_peers` entry. Proves the agent path, not the sandbox boundary.
1. **Sandbox to container**: B as a plain podman container from
   `docsclaw:dev-openshell` with `inbound_auth`, published on the host; an
   injecting proxy in front; A in an OpenShell sandbox whose profile binds the
   proxy host. Proves supervisor egress, real HTTP, real verification.

Assertions, in both:

- A's final text contains B's answer; B logs
  `sub=alice azp=spiffe://openshell.local/openshell/sandbox/…`.
- Each deny row from T1.6 arrives in A's tool result as
  `peer engineering-analyst: <status> <code>`.
- The token appears in no log, no session store, and no provider input on
  either side (`$OS logs`, `podman logs`, `/workspace/sessions.db`).

## T4b — v2 issuer integration (blocked)

Blocked on the v2 side; do not attempt it before T4a is green. To make A→B work
inside the v2 stack:

1. **Signing**: the token issuer must sign access tokens with ES256 or RS256 and
   serve `GET /.well-known/jwks.json`. The shared-secret HS256 mint
   (`internal/tokens/tokens.go`) cannot be verified by DocsClaw, and should not
   be by anything else either. The issuer already verifies ES256 SVIDs against
   the SPIRE OIDC JWKS, so the key handling exists — the gap is signing plus the
   endpoint.
1. **Claims mapping**: decide how DocsClaw reads a caller's capability. Today
   DocsClaw checks `required_scopes` against `scope` and `allowed_callers`
   against `azp`; the v2 token has `scope=openid profile email`, a `departments`
   array, and no `azp`. Either the issuer emits department scopes and an `azp`,
   or DocsClaw gains a `departments` check. Prefer the former.
1. **Profile**: a grant with `audience` equal to B's `inbound_auth.audience` and
   an `audience_override` for B's exposed host, `binaries:
   [/usr/local/bin/docsclaw]`.
1. **Exposure**: `$OS service expose <B> 8080 agent --authorization-mode
   bearer-passthrough`, then bind the URL from `$OS service list -o json` in A's
   profile.

Until all four land, A→B inside the v2 stack is unproven and the demo readiness
note must say so. T4a is the substitute evidence for the v0.21.0 tag.

## T5 — x86_64 on the RHEL homelab

Same v2 stack, its Fedora/RHEL quick start (`README.md`): rootless podman with
`podman.socket` enabled, OpenShell CLI 0.1.6 from upstream releases, and the
host OS detected automatically (`ZT2_HOST_OS` overrides).

```bash
make build && deploy/podman/scripts/build-images.sh && scripts/setup.sh
make image-openshell OPENARCH=amd64          # or PODMAN_CONNECTION=rhel on the host
```

Make the image visible to that gateway, then re-run T2, T3 and T4a. Additional
checks:

- `setup.sh` host-OS detection: the issuer `.svc` alias is a host `/etc/hosts`
  entry here, a podman VM entry on macOS.
- `doc_endpoint_host` differs (nip.io host vs host IP), so the
  `document_services` URL must match what the profile binds.
- The deny probes again: DNS and loopback behave differently on Linux, and the
  expected shape is still `upstream_unreachable` rather than `connection
  refused` for a bound-but-down host, because the proxy is the only reachable
  endpoint.

## T6 — Regression beyond zero-trust

The clean break touched config schema, serve wiring, `internal/bridge` and the
deploy manifests. Sweep the features that share those seams.

| Area | Fixture | Pass |
| ---- | ------- | ---- |
| Skills (local, `SKILL.md`) | `testdata/research-agent` | skills listed, injected into the prompt |
| OCI skills, `oras` | `testdata/skill-puller` | pull by reference, verify dual format |
| MCP client | `testdata/mcp-agent` | tool from the MCP server is callable |
| RAG / Weaviate | `testdata/rag` | search returns hits; skipped if no Weaviate |
| Exec deny patterns, `web_fetch.allowed_hosts` | `testdata/standalone` | allowed/denied as configured |
| Workspace profiles | `--workspace-profile openclaw\|hermes` | context file loaded, no `/workspace` assumptions broken |
| Batch mode + dashboard | `demo/batch` | `docsclaw batch` runs; dashboard renders |
| OpenAI `/v1` chat | Open WebUI pointed at `:8081/v1` | chat completes; with inbound auth on, the UI needs a token |
| K8s deploy | `deploy/standalone-agent.yaml` | applies, pod ready, probes green; no v1 SPIRE/OPA wording (#125) |
| ConfigMap generation | `make configmap-gen CONFIG_DIR=testdata/standalone` | renders |
| Metrics/health | `:8180/metrics`, `/health`, `/ready` | 200; no auth required |
| A2A interop | `a2a discover <url>`, `a2a send <url> "hi"` | card parsed, message answered |

## T7 — Release artifacts

```bash
goreleaser release --snapshot --clean
ls dist/ | head
podman manifest inspect ghcr.io/redhat-et/docsclaw:<ver>-openshell   # after push
```

| Check | Expected |
| ----- | -------- |
| Binaries | linux/darwin/windows × amd64/arm64 |
| Images | `<ver>-amd64`, `<ver>-arm64`, plus the `<ver>-openshell` manifest with both |
| Homebrew formula | generated, points at the darwin binaries |
| Version injection | `docsclaw --version` = the tag, not `dev` |
| Tag flow | `v0.21.0-rc.1` first; promote after T3 and T4a pass |
| CI gate | **none today** (#125) — run T0 locally before tagging |

## Triage

| Symptom | Likely cause | Look at |
| ------- | ------------ | ------- |
| `ask` hangs inside a sandbox | egress to the LLM host not allowed | supervisor log, model profile endpoint |
| `blocked by sandbox network policy` for a bound host | `binaries` path or digest mismatch | provider profile, `/usr/local/bin/docsclaw` |
| `502 Bad Gateway` on document fetch while the LLM works | expired demo subject token (30 min), or unregistered SPIFFE ID | mint a fresh provider per run, `register-sandbox.sh`, supervisor `Token grant failed` |
| `403 credential_endpoint_mismatch` | key used against a host it is not bound to | model profile `endpoints` |
| `403 insufficient_scope` with the right department | v2 `departments` claim vs DocsClaw `scope` check | T4b, claims mapping |
| `401 missing_token` when a token was sent | exposure used `--authorization-mode strip` | `service expose` flag, T3.5 |
| gateway cannot resolve `docsclaw:dev-openshell` | image built in a different engine than the gateway's | fallback registry in T3.0 |
| `503 jwks_unavailable` everywhere | issuer host not reachable from the sandbox | T4a |
| `401 missing_token` locally | caller omitted `Authorization` — expected when inbound auth is on | T1.5 |
| `403 invalid_audience` | `audience` differs between agent config and minted token | T1.6 |
| A sees `peer X: 403 caller_not_allowed` | `azp` not covered by `allowed_callers` prefix | T1.6 |
| `agent card signature invalid` | card re-signed with a different key, or canonicalization mismatch | `internal/a2acall/cardverify.go` |
| Serve won't start: address in use | loopback bind default and a stale process | `lsof -iTCP:8080` |
| `/workspace` not writable in sandbox | image `WORKDIR` created but volume remounted | T2 read-only case |

## Exit criteria

- [ ] T0 green on the release commit; no clean-break grep hits.
- [ ] T1.5–T1.7 pass or are explicitly substituted by the in-process e2e run.
- [ ] T2 image checks pass on arm64 and amd64.
- [ ] `Containerfile.openshell` runs as a non-zero UID **and GID**, and a sandbox
      created from the built image reaches Ready (T3.0). Satisfied by #126;
      keep the check as a regression guard.
- [ ] Config delivery into a sandbox is settled: `tar` in the image, a baked-in
      config, or the stdin path documented in T3.2. Satisfied by #127.
- [ ] T3.3 runs at least scenarios 1, 2 and 5 through `docsclaw ask` inside the
      v2 stack, with the real refusal code visible in the tool result.
- [ ] T3.4 records the real HTTPS denial shape and updates `openshell-notes.md`.
- [ ] T3.5 proves unauthenticated requests are refused by DocsClaw, not by
      OpenShell, and records the service URL from `service list -o json`.
- [ ] T4a passes: A→B with the test issuer, deny codes, no token in any log,
      session store, or provider input.
- [ ] T4b is either done or written into the demo readiness note as blocked,
      with the four requirements from T4b listed.
- [ ] T7 artifacts verified; #125's image check closed.
- [ ] T6 regressions filed as issues before the tag, not after.
