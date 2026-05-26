# Skill playground

An interactive environment for developing and testing DocsClaw skills.
Deploy a generic agent, edit a skill in a text editor, push the update,
and test it in a chat — without redeploying the entire stack.

This example is designed as a backend for a skill-editing frontend
where users iterate on skill content through a browser-based workflow:
edit skill text, click "Save and redeploy", test in chat, repeat.

## Prerequisites

- An OpenShift cluster (or Kubernetes with an Ingress controller)
- `kubectl` or `oc` CLI
- An LLM provider API key (Anthropic or OpenAI)

## Files

| File | Description |
| ---- | ----------- |
| `configmap.yaml` | Generic "skill tester" agent personality |
| `skill-configmap.yaml` | Draft skill with placeholder content |
| `llm-secret.yaml` | LLM provider API key (placeholder) |
| `deployment.yaml` | Deployment, Service, and Route |
| `redeploy.sh` | Update script: recreate ConfigMap, optionally restart |

## Deploy

1. Edit `llm-secret.yaml` with your LLM provider API key.

1. Apply all resources:

   ```bash
   kubectl apply -f configmap.yaml \
                 -f skill-configmap.yaml \
                 -f llm-secret.yaml \
                 -f deployment.yaml
   ```

1. Wait for the pod to become ready:

   ```bash
   kubectl wait --for=condition=ready pod -l app=skill-playground --timeout=60s
   ```

1. Get the route URL (OpenShift):

   ```bash
   echo "https://$(oc get route skill-playground -o jsonpath='{.spec.host}')"
   ```

## Edit and redeploy

There are two update paths depending on what changed in the skill.

### Content-only changes (auto-sync, no restart)

When you only change the skill body (not the `name:` or `description:`
in the YAML frontmatter), the update propagates automatically:

1. Edit your SKILL.md file
1. Update the ConfigMap:

   ```bash
   ./redeploy.sh my-skill.md
   ```

1. Wait ~30-60 seconds for the kubelet to sync the ConfigMap volume
1. Send a test message — the agent reads skill content from disk on
   each request, so it picks up the new version

**How it works**: Kubernetes periodically syncs ConfigMap data to
mounted volumes (default kubelet sync period is ~60 seconds). The
DocsClaw `load_skill` tool reads the SKILL.md file from disk on
every invocation, so once the kubelet syncs, the next request gets
the updated content.

### Full restart (for metadata changes)

When you change the skill's `name:` or `description:` in the YAML
frontmatter, the agent needs a restart because skill metadata is
loaded once at startup:

1. Edit your SKILL.md file (including frontmatter changes)
1. Update the ConfigMap and restart:

   ```bash
   ./redeploy.sh my-skill.md --restart
   ```

1. Wait ~10-15 seconds for the new pod to become ready
1. Send a test message

## Expected timing

| Operation | Delay |
| --------- | ----- |
| ConfigMap update (`kubectl apply`) | Instant |
| Kubelet volume sync (auto-sync path) | ~30-60s |
| Rollout restart + readiness (restart path) | ~10-15s |
| Full restart cycle (update + restart + ready) | ~15-20s |

The rollout restart is faster than waiting for the kubelet sync.
For rapid iteration during development, use `--restart` even for
content-only changes.

## Frontend integration

For a web frontend that wraps this workflow, the backend needs to
make these Kubernetes API calls:

1. **Update the skill ConfigMap**

   ```text
   PUT /api/v1/namespaces/{ns}/configmaps/skill-playground-draft-skill
   ```

   Set `data["SKILL.md"]` to the new skill text.

1. **Trigger a rollout restart** (optional, for metadata changes)

   ```text
   PATCH /apis/apps/v1/namespaces/{ns}/deployments/skill-playground
   ```

   Set `spec.template.metadata.annotations["kubectl.kubernetes.io/restartedAt"]`
   to the current timestamp.

1. **Poll for readiness**

   ```text
   GET /apis/apps/v1/namespaces/{ns}/deployments/skill-playground
   ```

   Check `status.readyReplicas == spec.replicas`.

1. **Send a test message** via the agent's A2A endpoint

   ```text
   POST https://{route-host}/tasks/send
   ```

## How it works

```text
                    ┌─────────────┐
                    │ Edit skill  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Update     │
                    │  ConfigMap  │
                    └──────┬──────┘
                           │
              ┌────────────┴────────────┐
              │                         │
     ┌────────▼────────┐     ┌──────────▼──────────┐
     │  Auto-sync      │     │  Rollout restart     │
     │  (~30-60s)      │     │  (~10-15s)           │
     │                 │     │                      │
     │  kubelet syncs  │     │  new pod reads       │
     │  volume, agent  │     │  ConfigMap on        │
     │  reads on next  │     │  startup             │
     │  load_skill     │     │                      │
     └────────┬────────┘     └──────────┬───────────┘
              │                         │
              └────────────┬────────────┘
                           │
                    ┌──────▼──────┐
                    │ Test in     │
                    │ chat        │
                    └─────────────┘
```
