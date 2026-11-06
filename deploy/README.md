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

2. Deploy the agent:

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
