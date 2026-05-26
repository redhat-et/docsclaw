# NPS Explorer agent

A DocsClaw agent that answers questions about U.S. national parks
using the [National Park Service REST API][nps-api].
This example demonstrates three common patterns:

- **ConfigMap-driven personality** — system prompt, A2A agent card,
  and tool configuration in a single ConfigMap
- **Skill as a separate ConfigMap** — domain knowledge (API endpoints,
  authentication, request patterns) mounted into the agent
- **Secret management** — API keys for the LLM provider and the
  external service injected via Kubernetes Secrets

## Prerequisites

- An OpenShift cluster (or Kubernetes with an Ingress controller)
- `kubectl` or `oc` CLI
- An LLM provider API key (Anthropic or OpenAI)
- A free NPS API key — [register here][nps-register]

## Files

| File | Description |
| ---- | ----------- |
| `configmap.yaml` | Agent personality: system prompt, A2A card, tool config |
| `skill-configmap.yaml` | NPS API skill with endpoint docs and examples |
| `llm-secret.yaml` | LLM provider API key (placeholder) |
| `nps-secret.yaml` | NPS API key (placeholder) |
| `deployment.yaml` | Deployment, Service, and Route |

## Deploy

1. Edit the secrets with your actual API keys:

   ```bash
   # In llm-secret.yaml, replace REPLACE_WITH_YOUR_LLM_API_KEY
   # In nps-secret.yaml, replace REPLACE_WITH_YOUR_NPS_API_KEY
   ```

1. Apply all resources:

   ```bash
   kubectl apply -f configmap.yaml \
                 -f skill-configmap.yaml \
                 -f llm-secret.yaml \
                 -f nps-secret.yaml \
                 -f deployment.yaml
   ```

1. Wait for the pod to become ready:

   ```bash
   kubectl wait --for=condition=ready pod -l app=nps-explorer --timeout=60s
   ```

1. Get the route URL (OpenShift):

   ```bash
   echo "https://$(oc get route nps-explorer -o jsonpath='{.spec.host}')"
   ```

## Test

Send a request using curl or the [a2a CLI][a2a-cli]:

```bash
# Using curl
AGENT_URL="https://$(oc get route nps-explorer -o jsonpath='{.spec.host}')"
curl -s "$AGENT_URL/tasks/send" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tasks/send",
    "id": "1",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"text": "What national parks are in Utah?"}]
      }
    }
  }' | jq .
```

```bash
# Using a2a CLI
a2a send --url "$AGENT_URL" "What national parks are in Utah?"
```

## How it works

The agent receives a user question and uses the `exec` tool to run
`curl` commands against the NPS API. The skill ConfigMap teaches the
agent which endpoints exist, what parameters they accept, and how to
authenticate — so it can construct correct requests without hardcoded
logic. The LLM typically pipes responses through `jq` for parsing.

Both `curl` and `jq` are included in the DocsClaw container image.
You can verify this locally:

```bash
podman run --rm --entrypoint curl ghcr.io/redhat-et/docsclaw:latest -V
podman run --rm --entrypoint jq ghcr.io/redhat-et/docsclaw:latest --version
```

```text
User question → DocsClaw → reads NPS skill → exec(curl) → NPS API
                                                ↓
                         formats response ← JSON response
```

## Customization

- **Add more skills**: create another ConfigMap with a `SKILL.md` and
  mount it at `/config/agent/skills/<skill-name>`
- **Change the LLM**: set `LLM_PROVIDER` and `LLM_MODEL` environment
  variables in the Deployment
- **Adjust tool limits**: edit `agent-config.yaml` in the ConfigMap to
  change timeouts, allowed tools, or max loop iterations

[nps-api]: https://www.nps.gov/subjects/developer/api-documentation.htm
[nps-register]: https://www.nps.gov/subjects/developer/get-started.htm
[a2a-cli]: https://github.com/a2aserver/a2a-go
