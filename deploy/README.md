# Deployment

## LLM provider configuration

DocsClaw supports three LLM providers. All configuration is carried in
a single Kubernetes Secret named `llm-secret`, injected via `envFrom`.

| Provider | `LLM_PROVIDER` | `LLM_BASE_URL` required? | Example models |
|----------|-----------------|--------------------------|----------------|
| Anthropic | `anthropic` | No | `claude-sonnet-4-6`, `claude-opus-4-6`, `claude-haiku-4-5` |
| OpenAI | `openai` | Yes (defaults to `https://api.openai.com/v1`) | `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.4-nano` |
| MaaS / LiteLLM | `litellm` | Yes | `qwen3-14b`, `deepseek-r1-distill-qwen-14b` |

Environment variables in the secret:

| Variable | Required | Description |
|----------|----------|-------------|
| `LLM_API_KEY` | Yes | API key for the provider |
| `LLM_PROVIDER` | Yes | `anthropic`, `openai`, or `litellm` |
| `LLM_MODEL` | No | Model name (each provider has a default) |
| `LLM_BASE_URL` | OpenAI/LiteLLM only | API endpoint URL |

To configure, copy the example for your provider and fill in the key:

```bash
# Pick one:
cp secrets/anthropic_example.yaml secrets/anthropic.yaml
cp secrets/openai_example.yaml    secrets/openai.yaml
cp secrets/maas_example.yaml      secrets/maas.yaml

# Edit to set LLM_API_KEY (and LLM_BASE_URL for MaaS), then:
oc apply -f secrets/<provider>.yaml
```

All three examples create the same `llm-secret` name, so only apply
one. To switch providers, delete the old secret and apply the new one.

## Local development

Source an environment file to set LLM provider variables:

```bash
cp environments/anthropic_example.env environments/anthropic.env
# Edit anthropic.env with your API key
source environments/anthropic.env

./bin/docsclaw serve --config-dir testdata/standalone
```

## OpenShift / Kubernetes

1. Create a namespace and the LLM secret (see above):

```bash
oc new-project docsclaw
oc apply -f secrets/anthropic.yaml   # or openai.yaml / maas.yaml
```

1. Deploy the agent:

```bash
oc apply -f standalone-agent.yaml        # basic agent
oc apply -f agent-with-skills.yaml       # agent with skills
```

## Egress in OpenShell sandboxes

OpenShell mediates egress transparently: the sandbox runs in its own
network namespace, every connection is routed through the supervisor's
proxy, and the supervisor strips `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`,
and their lowercase variants from the workload. A stock `http.Transport`
therefore reaches the supervisor with no code change.

Build outbound clients with `internal/egress`:

```go
client := egress.NewClient(30 * time.Second)
```

The package returns a clone of `http.DefaultTransport`, so the stock dialer
and connection pool are preserved. A source-scan test fails the build if
production code under `internal/` or `pkg/` contains a raw `http.Transport`
literal or assigns `Dial`, `DialContext`, or `DialTLSContext`. A raw
transport silently drops `Proxy`; a custom dialer changes DNS and TLS
behavior and diverges from the stock connection pool.

Inside a sandbox, `ProxyFromEnvironment` is effectively a no-op: the
supervisor strips `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, and their
lowercase variants from the workload, and an operator-configured upstream
proxy (`--upstream-proxy`) is applied by the supervisor independently of the
workload's environment. Keeping `Proxy: http.ProxyFromEnvironment` in the
constructor matters for deployments that are not sandboxed, where the process
environment is the only proxy signal.

## Files

| Path | Tracked | Purpose |
|------|---------|---------|
| `environments/*_example.env` | Yes | Template env files for local dev |
| `environments/*.env` | No | Your local env files (gitignored) |
| `secrets/*_example.yaml` | Yes | Template K8s secrets |
| `secrets/*.yaml` | No | Your real secrets (gitignored) |
| `standalone-agent.yaml` | Yes | Basic agent (ConfigMap + Deployment + Service + Route) |
| `agent-with-skills.yaml` | Yes | Agent with skills (adds skill ConfigMaps) |

## OpenShell sandbox image

The `-openshell` image variant is built for the v2 zero-trust demo and is
not the same image as the default agent image.

| Property | Value |
|----------|-------|
| Tag | `ghcr.io/redhat-et/docsclaw:<version>-openshell` |
| Binary | `/usr/local/bin/docsclaw` (no symlink) |
| User | UID 1001, GID 0 |
| Working directory | `/workspace` (writable) |
| Base | UBI 10 minimal (includes `bash` and `sleep`) |

OpenShell's `sandbox exec` runs commands through `bash -lc`, and the
harness keeps the sandbox alive with `sleep infinity`, so the image ships
both. Provider profiles bind credentials to the binary path, so the
binary must stay at `/usr/local/bin/docsclaw`.

`serve` defaults to `/workspace`. OpenShell treats a `WORKDIR` other than
`/` or `/sandbox` as the sandbox workspace, so it must exist and be
writable; the image creates it for UID 1001.

For a read-only root filesystem, mount a volume at the workspace path:

```bash
podman run -d --name dc-os -v docsclaw-ws:/workspace docsclaw:dev-openshell
```

Build it locally:

```bash
make image-openshell
podman run -d --name dc-os docsclaw:dev-openshell
podman exec dc-os docsclaw --version
podman rm -f dc-os
```

## Running in an OpenShell sandbox

The supervisor resolves provider credentials at the network boundary. The
workload only sees an opaque placeholder, so DocsClaw validates that the key
is non-empty and nothing else.

| Variable | Value in a sandbox |
|----------|--------------------|
| `LLM_PROVIDER` | `openai` (MaaS) or `anthropic` |
| `LLM_API_KEY` | `openshell:resolve:env:...` placeholder; the proxy substitutes the real key |
| `LLM_BASE_URL` | The MaaS or provider endpoint allowed by policy |
| `LLM_MODEL` | Model name served by that endpoint |

Do not set `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, or their lowercase
variants for the workload: the supervisor strips them. The per-sandbox CA is
injected through `SSL_CERT_FILE`, which Go reads automatically.

Run a single prompt without starting a server:

```bash
docsclaw ask --config-dir /config/agent --trace "Summarize DOC-001 from engineering"
```

The answer goes to stdout and every log line and trace entry goes to stderr,
so `answer=$(docsclaw ask ...)` captures only the answer. `--trace` prints
`→ fetch_document {...}` before the call and `← fetch_document ok: ...` after
it.

## Inbound authentication

OpenShell's `bearer-passthrough` forwards one bearer token but also admits
requests without one, so DocsClaw verifies the token itself when
`inbound_auth` is enabled:

```yaml
inbound_auth:
  enabled: true
  issuer: https://token-issuer.zt-demo.svc:8080
  jwks_url: https://token-issuer.zt-demo.svc:8080/.well-known/jwks.json
  audience: engineering-analyst
  required_scopes: [engineering]
  allowed_callers:
    - spiffe://openshell.local/openshell/sandbox/
```

With this block the A2A endpoints and the whole `/v1` tree require a valid
token; only `/.well-known/agent-card.json` and `/health` are exempt. `serve`
binds to `127.0.0.1` by default when inbound auth is enabled, because OpenShell
reaches the service through the sandbox loopback. Set `service.host`
explicitly to override the bind address.

Sessions are keyed by verified identity and the caller's `x-session-id`, so
two callers that reuse the same session ID cannot read each other's history.

The JWKS fetch is ordinary egress and goes through the supervisor: the issuer
host must be allowed by the sandbox network policy, or startup succeeds but
every request fails closed with `503 jwks_unavailable`.

The deny codes, token-stripping guarantee, and JWKS refresh behavior are
documented in `pkg/inboundauth/doc.go`.

## Calling peer agents

The `a2a_call` tool lets the LLM call another agent by name. Configure the
peers in `agent-config.yaml` and list the tool in `tools.allowed`:

```yaml
tools:
  allowed: [fetch_document, a2a_call]

a2a_peers:
  - name: engineering-analyst
    url: https://default--alice--engineering.svc.openshell.internal/
    description: Analyzes engineering documents
    timeout_seconds: 120
```

The agent sends no credentials; the OpenShell supervisor injects the bearer
token at the network boundary. The same peer list is enforced again by the
sandbox network policy, which is deliberate defense in depth: a typo in
`a2a_peers` fails with a policy denial rather than reaching an unintended
agent.
