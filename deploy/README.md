# Deployment

## Local development

Source an environment file to set LLM provider variables:

```bash
cp environments/anthropic_example.env environments/anthropic.env
# Edit anthropic.env with your API key
source environments/anthropic.env

./bin/docsclaw serve --config-dir testdata/standalone --listen-plain-http
```

## OpenShift/Kubernetes

1. Create a secret from the example:

```bash
cp secrets/anthropic_example.yaml secrets/anthropic.yaml
# Edit anthropic.yaml with your API key
oc apply -f secrets/anthropic.yaml
```

2. Deploy the agent:

```bash
oc apply -f standalone-agent.yaml
```

The deployment references `llm-secret` via `envFrom` — all three
provider examples create the same secret name, so only apply one.

## Files

| Path | Tracked | Purpose |
|------|---------|---------|
| `environments/*_example.env` | Yes | Template env files for local dev |
| `environments/*.env` | No | Your local env files (gitignored) |
| `secrets/*_example.yaml` | Yes | Template K8s secrets |
| `secrets/*.yaml` | No | Your real secrets (gitignored) |
| `standalone-agent.yaml` | Yes | Full deployment (ConfigMap + Deployment + Service + Route) |
