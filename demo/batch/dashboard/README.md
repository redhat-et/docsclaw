# Batch demo dashboard

Web UI for the DocsClaw batch processing demo. Deploys agents,
tracks progress, and displays results with live resource metrics.

## Scenarios

| Scenario | Agents | What it shows |
|----------|--------|---------------|
| Finance | 5 | Fan-out: each agent analyzes one vendor's contract + invoices |
| Security | 1 | Cross-reference: 50 CVEs × 8 assets × SLA deadlines |
| HR | 10 | Batch: 100 shuffled resumes scored against a job description |

## Prerequisites

- ConfigMaps for each scenario applied to the namespace
  (`finance-analyst-config`, `security-analyst-config`,
  `hr-screener-config`)
- `document-service` running with seed data (see
  `demo/batch/document-service/`)
- `llm-secret` with LLM provider credentials

## Local development

```bash
go build -o dashboard .
./dashboard -kubeconfig ~/.kube/config -namespace panni-docsclaw
# Open http://localhost:8090
```

In local mode the dashboard creates Routes for each agent and
communicates via HTTPS. Agents are cleaned up with the Cleanup
button.

## Deploy to OpenShift

```bash
# Build and push
podman build -t ghcr.io/redhat-et/docsclaw-dashboard:latest .
podman push ghcr.io/redhat-et/docsclaw-dashboard:latest

# Deploy (creates ServiceAccount, Role, RoleBinding, Deployment,
# Service, Route)
oc apply -f deployment.yaml -n <namespace>

# Get the dashboard URL
oc get route dashboard -o jsonpath='{.spec.host}'
```

In-cluster mode uses the ServiceAccount token for K8s API access
and talks to agents via Service DNS (no Routes needed).

## Architecture

Standalone Go binary (~10 MiB) with embedded static files
(`embed.FS`). No external dependencies beyond `gopkg.in/yaml.v3`
for kubeconfig parsing.

```
Browser → Route → Dashboard ─┬→ K8s API (deploy/delete/metrics/logs)
                              ├→ Agent Services (A2A JSON-RPC)
                              └→ Document Service (content preview)
```

The dashboard manages agent lifecycle: creates Deployments and
Services on "Run", sends A2A tasks, polls for completion, and
deletes resources on "Cleanup". Metrics (memory, CPU) come from
the K8s metrics API; token counts are parsed from agent pod logs.

## Demo flow

1. Open the dashboard in a browser
2. Pick a scenario (Finance, Security, or HR)
3. Click document IDs to preview input data
4. Click **Run** — watch agents deploy and process in parallel
5. When agents show **Ready**, click **View Report** to see results
6. Point out the totals bar: memory, CPU, tokens
7. Click **Cleanup** to remove agent pods
