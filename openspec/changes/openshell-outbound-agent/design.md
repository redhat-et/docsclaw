## Context

DocsClaw currently targets the v1 Zero Trust demo, where Kagenti,
AuthBridge, and OPA sidecars sit in front of the agent. The v2 demo runs
every agent in an NVIDIA OpenShell sandbox: the supervisor brokers
egress, performs token exchanges, and injects credentials at the network
boundary. Inbound traffic reaches a sandbox only through
`openshell service expose`, and `bearer-passthrough` forwards one bearer
token unchanged while also admitting token-less requests.

The v1 identity code is therefore dead or unsafe. In particular, A2A
document mode reads the caller's `Authorization` value from
`ServiceParams` and forwards it to the document service, which is token
passthrough. Phase 1 removes that code and prepares the agent for an
outbound-only sandbox. Phase 2, in a separate change, adds inbound
authentication and A2A calls between sandboxes.

## Goals / Non-Goals

**Goals:**

- Remove every v1 SPIFFE, OPA, delegation, and document-mode code path.
- Make every outbound HTTP client proxy-aware and enforcement-friendly.
- Let one agent address several departmental document services by name.
- Report denials with the supervisor's reason, not a generic message.
- Provide a one-shot `ask` command that shares the `serve` runtime.
- Publish an image the v2 harness can start with `-- sleep infinity`.

**Non-Goals:**

- Inbound authentication, session isolation, or `a2a_call` (Phase 2).
- A shared agent serving multiple users.
- An agent-side allowlist for document services; OpenShell policy is the
  enforcement point, and a config that names a forbidden service is a
  deliberate teaching scenario.
- Message buses, streaming, or A2A push notifications.

## Decisions

### Index decision 1: remove A2A document mode

Delete `FetchDocument`, `ProcessLLM`, `ExtractDocumentID`, the
`DocumentFetcher` and `LLMProcessor` types, and the `Authorization`
service-param read. Every A2A message goes through `ProcessMessage`.
Without `agent-config.yaml`, `ProcessMessage` makes one
`llmProvider.Complete` call; with it, it runs the tool loop.

*Alternative considered:* keep document mode but swap the forwarded
token for a supervisor-injected one. Rejected: the agent has no token to
forward, and a second code path through the executor duplicates the tool
loop.

### Index decision 2: keep the tool name `fetch_document`

Only the parameters change, to `service` and `document_id`. Ten demo and
testdata configs already list `fetch_document` in `tools.allowed`, and
the name is still accurate.

*Alternative considered:* rename to `get_document`. Rejected as churn
with no behavioral gain.

### Index decision 3: one `internal/egress` package

`internal/egress` owns client construction, denial description, and
outbound call logging. WP11 reuses it for `a2a_call`. Production code
must not build `http.Transport` literals or custom dialers; a source-scan
test enforces this.

*Alternative considered:* keep per-package clients. Rejected because a
single new `&http.Transport{TLSClientConfig: ...}` would silently drop
`Proxy` and bypass the supervisor's model, and no central place would
notice.

### Index decision 4: delete `--document-service-url`

The `document_services` map replaces both the flag and the
`document_service_url` config key. This is a clean break with no alias,
consistent with the project having no production consumers.

### Egress: transparent interception, stock transport

WP01 found that OpenShell redirects the sandbox network namespace to its
proxy through a veth pair and strips `ALL_PROXY`, `HTTP_PROXY`,
`HTTPS_PROXY`, `NO_PROXY` (and lowercase and gRPC variants) from every
mediated child. `http.ProxyFromEnvironment` therefore returns nil and Go
dials directly into the namespace redirect. The correct client is a
clone of `http.DefaultTransport`, which keeps `Proxy` populated for the
case where an operator configures an upstream proxy, and the stock
dialer. A per-sandbox CA reaches Go through `SSL_CERT_FILE`, so no code
change is needed for TLS trust.

The unit test asserts the proxy function is non-nil, and the scan test
fails any raw `http.Transport` literal or `DialContext` assignment
outside `internal/egress`.

### Denial reporting: two shapes, one vocabulary

For `http://` targets the supervisor returns `403` with
`{"error":"policy_denied","detail":...,"reason":...}` and
`Content-Type: application/json`, which Go can read. Other codes are
`ssrf_denied`, `middleware_denied`, `middleware_failed`,
`credential_endpoint_mismatch`, and the `502` codes
`upstream_unreachable` and `token_grant_failed`.

For `https://` targets the plaintext `403` precedes the TLS handshake,
so Go reports `http: server gave HTTP response to HTTPS client` and the
body is unreadable. `DescribeError` maps that string, and the
`proxyconnect` / connection-refused shapes, to
`blocked by sandbox network policy`. `DescribeResponse` reads at most
4 KiB and prefers the JSON `error` field, falling back to
`http.StatusText`.

`LogCall` logs `tool`, `host`, `status`, and `reason` at info level and
takes no header or query arguments by design.

### `document_services` schema and validation

```yaml
document_services:
  engineering: http://document-engineering.zt-demo.svc.cluster.local:8080
  finance: http://document-finance.zt-demo.svc.cluster.local:8080
```

Keys must match `^[a-z0-9][a-z0-9-]*$`; values must be absolute `http`
or `https` URLs. `fetch_document` exposes `service` as an `enum` of the
sorted keys and `document_id` as a string. The request path is
`<base>/documents/<url.PathEscape(id)>`, with trailing slashes trimmed.

### `agentRuntime` extraction

`runServe` is split so `serve` and `ask` share one setup path:

```go
type agentRuntime struct {
    Cfg          Config
    AgentCfg     *AgentConfig
    Provider     llm.Provider
    Registry     *tools.Registry
    LoopConfig   tools.LoopConfig
    SystemPrompt string
    AgentCard    *a2a.AgentCard
    Log          *logger.Logger
}

func buildRuntime(ctx context.Context, cfg Config, opts runtimeOptions) (*agentRuntime, error)
func (r *agentRuntime) Run(ctx context.Context, messages []llm.Message, hook tools.Hook) (string, error)
func newServeHandler(rt *agentRuntime) (http.Handler, func() error, error)
```

`executor.ProcessMessage` and `openaiapi.Handler` both call `rt.Run`, so
the tool loop has one implementation. `ask` uses an injected provider in
tests, prints only the answer to stdout, sends logs and `--trace` output
to stderr, and returns an error when no provider is configured.

### Sandbox image layout

Use a UBI minimal base because `sandbox exec` defaults to `bash -lc` and
UBI micro has no shell. The image provides:

| Element | Value |
| --- | --- |
| Binary | `/usr/local/bin/docsclaw`, mode 0755, root-owned, no symlink |
| User | UID 1001 |
| Workspace | `/sandbox`, writable by UID 1001 |
| Command | `sleep infinity` |
| Labels | OCI `source`, `description`, `licenses` |

The provider profile must list `/usr/local/bin/docsclaw` in `binaries`,
because OpenShell binds credentials to binary paths and pins the first
observed digest.

## Risks / Trade-offs

- [HTTPS denial is unreadable] The Go error string is the only signal, and
  it is source-derived and unverified. Mitigation: also match
  `proxyconnect` and connection-refused; keep the mapping in one function.
- [Proxy environment regression] A future raw transport silently drops
  `Proxy`. Mitigation: the scan test fails the build with file:line.
- [Demo configs drift] Removing `--document-service-url` breaks any
  manifest not updated in the same change. Mitigation: the acceptance
  grep in WP05 must return nothing, and the demo YAMLs are updated in the
  same commit.
- [Base image choice] UBI micro would be smaller but strands `sandbox
  exec`. Mitigation: use minimal and document the path.
- [Streaming output] `ask` prints the final answer only. Mitigation: not
  required by the demo; `--trace` shows tool progress.

## Migration Plan

1. WP03 removes v1 identity and document mode; no config changes needed.
1. WP04 introduces `internal/egress` and routes existing clients through it.
1. WP05 changes `fetch_document`, deletes the old flag, and updates demo
   and testdata configs in the same commit.
1. WP06 extracts the runtime and adds `ask`.
1. WP07 adds the image and release workflow.
1. Rollback is a revert per WP; there are no data migrations.

## Open Questions

1. The exact Go error for an HTTPS policy denial is unverified until a
   sandbox is reachable. WP05 must keep the mapping narrow and tested at
   the string level.
1. The public service URL is deployment-specific; the v2 harness must
   read it from `openshell service list --output json`.
