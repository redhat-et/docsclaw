# Agent manifest guide

Build and deploy custom agent images from a declarative manifest.

## Overview

An agent manifest defines everything needed to build and deploy
an agent: base image, OS tools, system prompt, skills, secrets,
and deployment config. The formula:

**agent = base image + installed tools + system prompt + skills**

The workflow has three steps:

1. **Write** the manifest (YAML)
2. **Build** the container image (`make agent-push`)
3. **Deploy** to OpenShift (`docsclaw deploy | oc apply -f -`)

## Step 1: Write the manifest

Create an `agent-manifest.yaml`:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: AgentManifest
metadata:
  name: nps-assistant
  version: 1.0.0
spec:
  base:
    image: registry.access.redhat.com/hi/core-runtime:latest
    toolBuilder: registry.access.redhat.com/hi/core-runtime:latest-builder

  tools:
    - curl
    - jq
    - git

  prompt:
    text: |
      You are a national parks assistant.
      Use the NPS API skill to answer questions about parks.

  skills:
    - name: nps-api
      image: quay.io/docsclaw/skill-nps-api:1.0.0-image

  runtime:
    tools:
      allowed:
        - exec
        - read_file
        - web_fetch
        - load_skill
      exec:
        timeout: 30
        maxOutput: 50000
    loop:
      maxIterations: 15

  secrets:
    - name: NPS_API_KEY
      description: "API key for developer.nps.gov"
      required: true
    - name: LLM_API_KEY
      description: "LLM provider API key"
      required: true

  deploy:
    image: ghcr.io/redhat-et/nps-agent:1.0.0
    replicas: 1
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
      limits:
        cpu: 500m
        memory: 256Mi
```

### Key fields

| Field | Purpose |
|-------|---------|
| `spec.base.image` | Hardened runtime base image |
| `spec.base.toolBuilder` | Builder image with dnf (for installing tools) |
| `spec.base.goBuilder` | Go compiler image (defaults to `hi/go:latest`) |
| `spec.tools` | OS-level tools to install (from the tool catalog) |
| `spec.prompt.text` | System prompt defining the agent's personality |
| `spec.skills` | OCI image volumes mounted at `/config/agent/skills/` |
| `spec.runtime` | Agent-config.yaml settings (tool allowlist, loop) |
| `spec.secrets` | Required environment variables (values never stored) |
| `spec.deploy.image` | The built agent image used in the K8s Deployment |

### Tool catalog

Tools are selected from a curated catalog organized by security
tier. Core tools (curl, jq) are always included.

| Tier | Tools | Risk | Auto-included |
|------|-------|------|---------------|
| core | curl, jq | 1-2 | Yes |
| standard | git, openssh-client | 4 | No |
| extended | pandoc, poppler-utils, imagemagick | 3-5 | No |
| runtime | python3, nodejs | 8 | No |

Each tool carries a risk score (1-10). The image-level score is
the max of all installed tools. Use `--max-risk` to enforce a
ceiling:

```bash
docsclaw build --manifest agent-manifest.yaml --max-risk 6
```

## Step 2: Build and push the image

### Validate first (dry run)

```bash
docsclaw build --manifest agent-manifest.yaml --dry-run
```

This prints the tool inventory, tier, and risk score without
building anything.

### Build and push

```bash
make agent-push \
  MANIFEST=agent-manifest.yaml \
  TAG=ghcr.io/redhat-et/nps-agent:1.0.0
```

This runs three chained targets:

1. `agent-build` — generates the Containerfile and tools.json,
   copies them to the project root as the build context
2. `agent-image` — runs `podman build` with the two-stage
   Containerfile (Go compilation + hardened runtime)
3. `agent-push` — pushes the image to the registry

### Remote builds (RHEL host)

If you develop on macOS and need x86_64 images, use a remote
podman connection:

```bash
make agent-push \
  MANIFEST=agent-manifest.yaml \
  TAG=ghcr.io/redhat-et/nps-agent:1.0.0 \
  PODMAN_CONNECTION=rhel
```

The two-stage Containerfile compiles the Go binary on the remote
host natively — no cross-compilation needed.

### Inspect the generated artifacts

```bash
docsclaw build --manifest agent-manifest.yaml --output build/agent

ls build/agent/
# Containerfile      — two-stage build
# tools.json         — embedded runtime metadata
# agent-config.yaml  — generated from spec.runtime
# system-prompt.txt  — from spec.prompt.text
# k8s/               — K8s manifests
```

## Step 3: Deploy to OpenShift

### Create the namespace and secrets

```bash
oc new-project my-agents

export NPS_API_KEY=your-key-here
export LLM_API_KEY=your-llm-key
```

### Generate and apply manifests

```bash
docsclaw deploy --manifest agent-manifest.yaml | oc apply -f -
```

The deploy command:

1. Reads the manifest
2. Resolves secrets from environment variables (or `--secret`
   flags)
3. Generates ServiceAccount, ConfigMap, Secret, Service, and
   Deployment YAML
4. Prints to stdout for piping to `oc apply`

Alternatively, write to files and apply separately:

```bash
docsclaw deploy --manifest agent-manifest.yaml \
  --output deploy/nps-agent/

oc apply -f deploy/nps-agent/k8s/
```

### Secret resolution

Secrets are resolved at deploy time, never stored in the
manifest. Resolution order:

1. `--secret NAME=value` flag (highest priority)
2. Environment variable with matching name
3. Error if `required: true` and no value found

```bash
# From environment (recommended for CI/CD)
export LLM_API_KEY=sk-...
docsclaw deploy --manifest agent-manifest.yaml | oc apply -f -

# From flags (quick local testing)
docsclaw deploy --manifest agent-manifest.yaml \
  --secret NPS_API_KEY=abc123 \
  --secret LLM_API_KEY=sk-test \
  | oc apply -f -
```

### Verify the deployment

```bash
# Check pod status
oc get pods -l app=nps-assistant

# Check logs
oc logs -l app=nps-assistant

# Verify tools are installed
oc exec deploy/nps-assistant -- curl --version
oc exec deploy/nps-assistant -- jq --version

# Check embedded tool metadata
oc exec deploy/nps-assistant -- cat /etc/docsclaw/tools.json

# Test the agent
a2a discover http://nps-assistant:8000
a2a send http://nps-assistant:8000 "List parks in Montana"
```

## Image metadata

Built images carry OCI annotations inspectable without pulling:

```bash
skopeo inspect docker://ghcr.io/redhat-et/nps-agent:1.0.0 \
  | jq '.Labels | with_entries(select(.key | startswith("io.docsclaw")))'
```

| Annotation | Example |
|------------|---------|
| `io.docsclaw.tools/installed` | `curl,git,jq` |
| `io.docsclaw.tools/tier` | `standard` |
| `io.docsclaw.tools/risk-score` | `4` |
| `io.docsclaw.tools/agent-name` | `nps-assistant` |

At runtime, `/etc/docsclaw/tools.json` provides the full
inventory with versions and risk scores. The agent reads this at
startup and injects the tool list into the system prompt so the
LLM knows exactly which OS tools are available.

## Cluster requirements

### Skill image volumes

Skills listed in `spec.skills` are mounted as OCI image volumes
using the Kubernetes `image:` volume source (KEP-4639). This
requires:

- **OpenShift 4.20+** with the `ImageVolumeSource` feature gate
  enabled, or
- **Kubernetes 1.31+** with the `ImageVolume` feature gate
  enabled (alpha)

If your cluster does not support image volumes, deploy skills
via ConfigMaps or init containers instead. See the
[OCI skills guide](oci-skills-guide.md) for alternative
deployment methods.

## Skill compatibility

Skills that include a `skill.yaml` with tool requirements are
checked at build time:

```text
Checking skill compatibility...

  ✔ nps-api
      required: curl ✔, jq ✔

  ✘ doc-converter
      required: pandoc ✗
      → Add 'pandoc' to spec.tools or remove this skill
```

Skills without `skill.yaml` are flagged as unknown — no build
failure, but tool errors may occur at runtime.

## Quick reference

```bash
# Validate manifest
docsclaw build --manifest agent-manifest.yaml --dry-run

# Generate artifacts without building
docsclaw build --manifest agent-manifest.yaml --output ./build/

# Build and push image
make agent-push MANIFEST=agent-manifest.yaml TAG=registry/image:tag

# Build on remote RHEL host
make agent-push MANIFEST=... TAG=... PODMAN_CONNECTION=rhel

# Deploy with env secrets
export LLM_API_KEY=sk-...
docsclaw deploy --manifest agent-manifest.yaml | oc apply -f -

# Deploy with flag secrets
docsclaw deploy --manifest agent-manifest.yaml \
  --secret LLM_API_KEY=sk-... | oc apply -f -
```
