# Agent manifest and container tooling design

## Overview

DocsClaw agents run in minimal hardened container images. When
skills need OS-level tools (curl, jq, pandoc, python3), those
tools must be explicitly installed into the image. Today this is
manual — editing Containerfiles by hand with no validation that
skills will have what they need at runtime. This design introduces
a declarative agent manifest, a curated tool catalog with
risk scoring, and build/deploy tooling that generates container
images and K8s manifests from that manifest.

The core formula: **agent = base image + installed tools + system
prompt + skills (mounted)**.

## Goals

- Declare a complete agent (image, tools, prompt, skills, secrets)
  in a single versionable YAML file
- Curate allowed tools in a tiered catalog with risk scoring so
  admins can enforce security policy
- Check skill/tool compatibility at both build time and runtime
- Embed tool metadata in OCI annotations for inspection without
  pulling the image
- Generate Containerfiles, K8s manifests, and secrets from the
  manifest
- Provide a shopping cart UI for assembling agents visually

## User model

A layered model separates admin and developer concerns:

| Role | Responsibilities |
|------|-----------------|
| Cluster admin | Maintains the tool catalog and build policy. Builds approved base images with tool tiers. Sets max risk thresholds. |
| Developer | Picks a base image, adds skills, writes the system prompt. Gets compatibility warnings but cannot add OS-level tools beyond what the admin allows. |

Admins provide approved base images with curated tool sets.
Developers add skills on top and get compatibility warnings.

## Agent manifest format

The manifest is the single source of truth. Everything else
(Containerfile, K8s YAML, OCI annotations) is generated from it.

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: AgentManifest
metadata:
  name: nps-assistant
  version: 1.0.0
spec:
  base:
    image: registry.access.redhat.com/hi/core-runtime:latest
    builder: registry.access.redhat.com/hi/core-runtime:latest-builder

  tools:
    - curl
    - jq
    - git

  prompt:
    # Option A: inline text
    text: |
      You are a national parks assistant...
    # Option B: from a Git repo (mutually exclusive with text)
    # source:
    #   git: https://github.com/org/prompts.git
    #   path: agents/nps-assistant/system-prompt.txt
    #   ref: main

  skills:
    - name: nps-api
      image: quay.io/docsclaw/skill-nps-api:1.0.0-image
    - name: document-summarizer
      image: quay.io/docsclaw/skill-document-summarizer:1.0.0-image

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
    replicas: 1
    resources:
      requests: { cpu: 100m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
```

Key distinctions:

- `spec.tools` — OS-level packages installed in the container
  (curl, jq, git)
- `spec.runtime.tools.allowed` — DocsClaw internal tools the LLM
  can call (exec, read_file, etc.)
- `spec.secrets` — declares what the agent needs without storing
  values; resolved from environment variables at deploy time

### Secret resolution

The manifest declares required secrets but never stores values.
Resolution order at deploy time:

1. `--secret NAME=value` CLI flag (highest priority)
2. Environment variable with matching name
3. Fail with error if `required: true` and no value found

This keeps the manifest safe to commit while reducing friction
for local development (`export NPS_API_KEY=xxx` before deploy).

## Tool catalog

A curated list of tools that DocsClaw knows how to install into
hardened images. Each tool carries metadata for the UI,
compatibility checking, and security warnings.

### Tier system

| Tier | Description | Auto-include | Warning |
|------|-------------|-------------|---------|
| core | Required for basic agent operation | Yes | None |
| standard | Low risk, small footprint | No | None |
| extended | Significant capabilities and attack surface | No | "Review with your security team" |
| runtime | Full language runtimes, large footprint | No | "Adds full scripting runtime" |

### Risk scoring

Each tool carries a composite risk score (1-10) computed from
four factors:

| Factor | Condition | Points |
|--------|-----------|--------|
| Code execution | Can run arbitrary code | +3 |
| Network capable | Can make outbound connections | +2 |
| Dependencies | Per 5 transitive packages | +1 |
| CVE history | low / medium / high | +1 / +2 / +3 |

The image-level risk score is the max of all installed tool
scores (not the sum). One python3 puts the image at 8/10;
adding nodejs does not make it 16.

### Catalog format

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: ToolCatalog
metadata:
  name: docsclaw-default
  version: 1.0.0

tiers:
  core:
    description: "Always included"
    autoInclude: true
  standard:
    description: "Low risk, small footprint"
  extended:
    description: "Medium risk, larger footprint"
    warning: "These tools expand the image attack surface."
  runtime:
    description: "High risk, full language runtimes"
    warning: "Adds a full scripting runtime (~40-100 MB)."

tools:
  curl:
    tier: core
    package: { dnf: curl, apk: curl }
    size: ~1MB
    description: "HTTP client for API requests"
    risk:
      score: 2
      factors:
        codeExecution: false
        networkCapable: true
        dependencies: 2
        cveHistory: low
      rationale: "Network-capable but single-purpose"

  jq:
    tier: core
    package: { dnf: jq, apk: jq }
    size: ~1MB
    description: "JSON processor for parsing API responses"
    risk:
      score: 1
      factors:
        codeExecution: false
        networkCapable: false
        dependencies: 1
        cveHistory: low
      rationale: "Single-purpose binary, minimal surface"

  git:
    tier: standard
    package: { dnf: git, apk: git }
    size: ~15MB
    description: "Version control for code-related workflows"
    risk:
      score: 4
      factors:
        codeExecution: false
        networkCapable: true
        dependencies: 8
        cveHistory: medium
      rationale: "Network-capable, moderate dependency tree"

  openssh-client:
    tier: standard
    package: { dnf: openssh-clients, apk: openssh-client }
    size: ~5MB
    description: "SSH client for remote operations"
    risk:
      score: 4
      factors:
        codeExecution: false
        networkCapable: true
        dependencies: 5
        cveHistory: medium
      rationale: "Network-capable, handles credentials"

  pandoc:
    tier: extended
    package: { dnf: pandoc, apk: pandoc }
    size: ~25MB
    description: "Universal document converter"
    risk:
      score: 3
      factors:
        codeExecution: false
        networkCapable: false
        dependencies: 6
        cveHistory: low
      rationale: "Parses complex formats, moderate deps"

  poppler-utils:
    tier: extended
    package: { dnf: poppler-utils, apk: poppler-utils }
    size: ~10MB
    description: "PDF text extraction"
    risk:
      score: 3
      factors:
        codeExecution: false
        networkCapable: false
        dependencies: 8
        cveHistory: medium
      rationale: "Parses untrusted PDF input"

  imagemagick:
    tier: extended
    package: { dnf: ImageMagick, apk: imagemagick }
    size: ~20MB
    description: "Image conversion and manipulation"
    risk:
      score: 5
      factors:
        codeExecution: false
        networkCapable: false
        dependencies: 15
        cveHistory: high
      rationale: "Large dependency tree, extensive CVE history"

  python3:
    tier: runtime
    package: { dnf: python3, apk: python3 }
    size: ~45MB
    description: "Python interpreter for scripting"
    risk:
      score: 8
      factors:
        codeExecution: true
        networkCapable: false
        dependencies: 12
        cveHistory: high
      rationale: "Full interpreter enables arbitrary code execution"

  nodejs:
    tier: runtime
    package: { dnf: nodejs, apk: nodejs }
    size: ~60MB
    description: "Node.js runtime for JavaScript tools"
    risk:
      score: 8
      factors:
        codeExecution: true
        networkCapable: false
        dependencies: 10
        cveHistory: high
      rationale: "Full interpreter enables arbitrary code execution"
```

The `package` field has per-distro mappings so the Containerfile
generator picks `dnf` for hardened images, `apk` for Alpine.

Organizations can maintain their own catalog with custom tools or
override tier assignments via `--catalog custom-catalog.yaml`.

## Image metadata

Two layers: OCI annotations for quick inspection without pulling,
and an embedded manifest for full detail at runtime.

### OCI annotations

Set as LABELs in the generated Containerfile, visible via
`skopeo inspect`:

```
io.docsclaw.tools/installed: "curl,jq,git"
io.docsclaw.tools/tier: "standard"
io.docsclaw.tools/risk-score: "4"
io.docsclaw.tools/base: "hi/core-runtime:latest"
io.docsclaw.tools/agent-name: "nps-assistant"
```

The tier label reflects the highest tier of any installed tool.

### Embedded tools.json

Generated during build at `/etc/docsclaw/tools.json`:

```json
{
  "manifestVersion": "1.0.0",
  "agentName": "nps-assistant",
  "base": "registry.access.redhat.com/hi/core-runtime:latest",
  "highestTier": "standard",
  "riskScore": 4,
  "tools": [
    {
      "name": "curl",
      "package": "curl",
      "version": "8.5.0",
      "tier": "core",
      "risk": { "score": 2, "codeExecution": false, "networkCapable": true }
    },
    {
      "name": "jq",
      "package": "jq",
      "version": "1.7.1",
      "tier": "core",
      "risk": { "score": 1, "codeExecution": false, "networkCapable": false }
    },
    {
      "name": "git",
      "package": "git",
      "version": "2.43.0",
      "tier": "standard",
      "risk": { "score": 4, "codeExecution": false, "networkCapable": true }
    }
  ]
}
```

Version numbers are captured at build time by querying installed
packages (`rpm -q` for hardened images, `apk info` for Alpine).

### Usage by layer

| Layer | When | By whom | Example |
|-------|------|---------|---------|
| OCI annotations | Before pulling | Registry UI, `skopeo`, admission webhooks | Block images with risk > 6 from production |
| `/etc/docsclaw/tools.json` | At runtime | Agent startup, skill checker | Verify skills have required tools |

OCI annotations enable OPA/Gatekeeper policies: a cluster admin
can enforce "no runtime-tier images in production" at the
admission controller level.

## Skill compatibility checking

Skills declare tool requirements in `skill.yaml` (the SkillCard
format). The existing `spec.tools` field has `required` and
`optional` lists.

### Build-time checking

When `docsclaw build` processes a manifest:

1. Resolve all skills listed in `spec.skills`
2. Fetch each skill's `skill.yaml` from OCI registry or local path
3. Compare `spec.tools.required` against the manifest's
   `spec.tools` list
4. Required tool missing → build fails with actionable message
5. Optional tool missing → warning with risk information

```
Checking skill compatibility...

  ✔ nps-api
      required: curl ✔, jq ✔
      optional: python3 ✗ (tier: runtime, risk: 8)

  ✘ doc-converter
      required: curl ✔, pandoc ✗
      ERROR: 'doc-converter' requires 'pandoc' (tier: extended)
      → Add 'pandoc' to spec.tools or remove this skill

Build failed: 1 unsatisfied skill requirement
```

### Runtime checking

On agent startup, DocsClaw reads `/etc/docsclaw/tools.json` and
validates each loaded skill:

- Required tool missing → disable skill, log ERROR
- Optional tool missing → log WARN, skill remains active
- No `skill.yaml` → log INFO, skip validation

### Known limitation

Skills with only `SKILL.md` (no `skill.yaml`) have no
machine-readable tool declarations. Build-time checking is
skipped; runtime failures surface as `command not found` errors
in the agentic loop. The build command emits a warning:

```
WARN  skill 'nps-api' has no skill.yaml — tool requirements
      unknown. Runtime tool errors may occur.
```

This nudges skill authors toward declaring dependencies without
blocking anyone.

## Build pipeline

### `docsclaw build`

Reads the agent manifest and produces container images and/or
generated files.

```bash
# Build image locally
docsclaw build --manifest agent-manifest.yaml

# Generate files to directory without building
docsclaw build --manifest agent-manifest.yaml --output ./build/

# Generate only specific artifacts
docsclaw build --manifest agent-manifest.yaml --output ./build/ \
  --only containerfile
docsclaw build --manifest agent-manifest.yaml --output ./build/ \
  --only k8s

# Dry run — compatibility report and risk score only
docsclaw build --manifest agent-manifest.yaml --dry-run

# Build and push (future work — use make agent-push for now)
# docsclaw build --manifest agent-manifest.yaml --push \
#   --tag ghcr.io/org/nps-agent:1.0.0

# Enforce risk policy
docsclaw build --manifest agent-manifest.yaml --max-risk 6
```

Output directory structure when using `--output`:

```
build/
├── Containerfile
├── tools.json
├── agent-config.yaml
├── system-prompt.txt
└── k8s/
    ├── configmap.yaml
    ├── deployment.yaml
    ├── service.yaml
    ├── serviceaccount.yaml
    └── secret.yaml
```

### Build steps

1. Parse and validate manifest against schema
2. Load tool catalog (built-in + optional org catalog)
3. Validate all tools exist in catalog
4. Fetch skill metadata from OCI registries
5. Run compatibility check (skills vs tools)
6. Compute risk score; enforce `--max-risk` threshold
7. Generate Containerfile with dnf/apk install, LABELs, tools.json
8. Build image via podman/docker (unless `--output` or `--dry-run`)
9. Push to registry (if `--push`)

### `docsclaw deploy`

Generates K8s manifests with secrets resolved from environment:

```bash
export NPS_API_KEY=abc123
export LLM_API_KEY=sk-xxx
docsclaw deploy --manifest agent-manifest.yaml | oc apply -f -
```

### Admin build policy

Organizations can enforce constraints via a policy file:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: BuildPolicy
metadata:
  name: production
spec:
  maxRiskScore: 6
  blockedTools:
    - nodejs
  blockedTiers:
    - runtime
  requiredBase: "registry.access.redhat.com/hi/"
```

## Runtime integration

The agent startup sequence gains tool awareness without changing
the core agentic loop.

### Startup sequence

1. Load `agent-config.yaml` (unchanged)
2. **New:** read `/etc/docsclaw/tools.json` for OS tool inventory
3. Discover skills in skills directory (unchanged)
4. **New:** validate skill tool requirements against inventory
5. Register internal tools based on `tools.allowed` (unchanged)
6. **New:** inject tool list into system prompt context

The injected context tells the LLM exactly what's available:

```
Available OS tools: curl, jq, git
Do NOT attempt to use tools not in this list.
```

This directly prevents the wasted-iteration problem where LLMs
try tools that don't exist (python3, pip, node) and burn through
the agentic loop limit.

## Shopping cart UI

Extends the existing skillimage stepper wizard from 3 steps to 4.

### Step 1: Base Image

Pick the container runtime. Cards show image name, registry,
whether it's hardened, and pre-installed core tools. Initially
one option (DocsClaw on `hi/core-runtime`); extensible to other
base images from the Red Hat image catalog
(https://images.redhat.com/).

### Step 2: Tools (new)

Pick OS-level tools, grouped by tier. Core tools (curl, jq) are
pre-selected and locked. Key UI elements:

- **Risk gauge** in sidebar — green (1-3), yellow (4-6), red (7-10)
- **Running size estimate** — "Estimated image size: +47 MB"
- **Tier headers** with warning text for extended and runtime
- **Confirmation banner** when selecting runtime-tier tools

### Step 3: System Prompt

Pick a persona or paste custom text. Same as existing UI.

### Step 4: Skills

Same as existing UI plus compatibility checking:

- Skills with `skill.yaml`: check required tools against step 2
  selections. Missing tool → inline banner with "Add tool" button
- Skills without `skill.yaml`: info badge "Tool requirements
  unknown"
- Recommended skills pre-selected based on persona

### Output tabs

| Tab | Content |
|-----|---------|
| Agent Manifest | Primary output — copy or download YAML |
| Containerfile | Generated from manifest + catalog |
| K8s Manifests | ConfigMap, Deployment, Service, Secret |
| CLI Commands | `docsclaw build` and `docsclaw deploy` one-liners |

The manifest is the artifact users version-control. Generated
files are ephemeral — regenerate anytime.

## Summary

| Component | Format | Purpose |
|-----------|--------|---------|
| Agent Manifest | `agent-manifest.yaml` | Single source of truth |
| Tool Catalog | `tool-catalog.yaml` | Curated tools with risk scores |
| Build Policy | `build-policy.yaml` | Org-level guardrails |
| OCI annotations | Image labels | Pre-pull inspection |
| tools.json | `/etc/docsclaw/tools.json` | Runtime compatibility |
| Shopping cart UI | HTML | Visual agent assembly |
