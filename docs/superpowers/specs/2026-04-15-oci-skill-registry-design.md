# OCI Skill Registry — Design Spec

**Date:** 2026-04-15
**Status:** Draft
**Audience:** OCTO internal (Red Hat)
**Working project name:** oci-skill-registry

## Problem

AI agent skills need a distribution and lifecycle management layer
built on open standards. Existing solutions either store only
metadata (agent-registry) or are tied to specific agent frameworks
(DocsClaw OCI skills). Enterprises require content distribution
with provenance guarantees, versioning, and lifecycle governance
accessible to both technical and non-technical users.

## Vision

Skills are applications. Apply the full SDLC: develop, test,
version, distribute, update, deprecate. OCI is the distribution
layer because it already has signing, verification, and registry
infrastructure that enterprises trust.

## Scope

An OCI-based registry that **stores actual skill content** as OCI
artifacts and manages their lifecycle. This is a companion to
[agentoperations/agent-registry](https://github.com/agentoperations/agent-registry),
which handles metadata and governance for agents, skills, and MCP
servers. Integration is via shared OCI registries (loosely coupled).

### In scope

- Go library (`pkg/`) for OCI skill operations
- `skillctl` CLI for developers and CI/CD
- REST API server with OpenAPI 3.1 spec
- Skill authoring UI support (API endpoints for visual editor)
- SkillCard schema compatible with
  [Agent Skills spec](https://agentskills.io/specification)
- Lifecycle state machine with semver-aware promotion
- Signature verification (cosign/sigstore)
- Podman, Kubernetes, and OpenShift delivery targets

### Out of scope (planned for future phases)

- RBAC and customizable approval workflows
- Trust score computation (separate concern)
- gRPC interface (deferred until real consumer exists)
- UI implementation (separate project by UI developer)

### Not in scope

- Agent or MCP server registry (handled by agent-registry)
- Runtime execution of skills (handled by agent runtimes)

## Architecture

### Library-first design

The core is a Go library (`pkg/`). CLI and server are thin
consumers. This allows agent runtimes (DocsClaw, OpenClaw, any
framework), CI/CD pipelines, and agent-registry to import the
library directly.

```
┌─────────────────────────────────────────────────┐
│  Consumers: skillctl CLI, REST API server,      │
│  agent runtimes, CI/CD pipelines                │
├─────────────────────────────────────────────────┤
│  pkg/ — Go library (public API)                 │
│  ├── skillcard/   SkillCard parse/validate      │
│  ├── oci/         pack/push/pull/inspect        │
│  ├── verify/      signature verification        │
│  ├── lifecycle/   state machine, versioning     │
│  └── diff/        version comparison            │
├─────────────────────────────────────────────────┤
│  Storage: OCI registries (quay.io, ghcr.io,     │
│  Zot) + local SQLite/Postgres for metadata      │
└─────────────────────────────────────────────────┘
```

### Two operating modes

| Operation | CLI + OCI registry (no server) | Server + UI |
|---|---|---|
| Create/edit skill | Edit files locally | Visual editor: metadata form + prompt textarea + templates |
| Pack + push | `skillctl pack && push` | "Create skill" / "Save" button |
| Pull | `skillctl pull` | Download button |
| Inspect | `skillctl inspect` | Skill detail page |
| Diff | `skillctl diff` | Side-by-side visual diff |
| Verify signature | `skillctl verify` | Trust badge |
| Promote | — | Promote button with confirmation |
| Search/browse | — | Search, filters, categories |
| Eval results | — | Eval dashboard |
| Dependency graph | — | Interactive visualization |

The server uses the same `pkg/` library. CLI and UI execute
identical code paths.

## Skill lifecycle

### State machine

```
draft → testing → published → deprecated → archived
```

### Transitions

| Transition | Gate |
|---|---|
| draft → testing | Schema validation passes, required SkillCard fields present |
| testing → published | Major version: full review. Minor/patch: lightweight review (diff-only) |
| published → deprecated | Author or admin decision. Skill still pullable, consumers warned |
| deprecated → archived | Retention policy or manual. Skill no longer pullable, metadata retained |

### Updates

When a published skill gets an update, a new version enters as
`draft` and proceeds through the pipeline independently. The
previous version stays `published` until the new one is ready.
This mirrors standard software release management.

### OCI tags by state

| State | Tag pattern | Example |
|---|---|---|
| draft | `<version>-draft` | `1.2.0-draft` |
| testing | `<version>-rc` | `1.2.0-rc` |
| published | `<version>` + `latest` | `1.2.0`, `latest` |
| deprecated | `<version>` (no `latest`) | `1.2.0` |
| archived | tag removed, digest-only | — |

The SkillCard `status` field is the authoritative source of truth.
Tags are a convenience. `skillctl promote` updates both atomically.

## SkillCard schema

Compatible with the
[Agent Skills spec](https://agentskills.io/specification),
extended for OCI distribution and lifecycle management.

```yaml
apiVersion: skills.octo.ai/v1
kind: SkillCard
metadata:
  name: hr-onboarding
  display-name: "HR Onboarding Guide"
  namespace: acme
  version: 1.2.0
  status: published
  description: >
    Guides new employees through onboarding steps.
    Use when a new hire asks about first-day procedures.
  license: Apache-2.0
  compatibility: "Requires network access"
  tags:
    - hr
    - onboarding
  authors:
    - name: Jane Smith
      email: jsmith@acme.com
  allowed-tools: "exec webfetch"
provenance:
  source: https://github.com/acme/hr-skills
  commit: a1b2c3d
  path: skills/onboarding/
spec:
  prompt: system-prompt.txt
  examples:
    - input: "I'm starting next Monday"
      output: "Welcome! Let me walk you through..."
  dependencies:
    - name: acme/company-policies
      version: ">=1.0.0"
```

### Field mapping from Agent Skills spec

| Agent Skills spec | SkillCard field | Notes |
|---|---|---|
| `name` | `metadata.name` | Same constraints |
| `description` | `metadata.description` | Same constraints |
| `license` | `metadata.license` | Direct adoption |
| `compatibility` | `metadata.compatibility` | Direct adoption |
| `metadata.author` | `metadata.authors[]` | Structured list |
| `metadata.version` | `metadata.version` | Required, semver |
| `allowed-tools` | `metadata.allowed-tools` | Space-separated per spec (experimental) |

### Extensions beyond the spec

| Field | Purpose |
|---|---|
| `apiVersion` + `kind` | Schema versioning, K8s-style |
| `display-name` | Human-readable name for UI |
| `namespace` | Multi-tenant registry scoping |
| `status` | Lifecycle state |
| `tags` | Search and filtering |
| `provenance` | Optional source linkage |
| `spec.prompt` | File reference for prompt content |
| `spec.examples` | Structured example interactions |
| `spec.dependencies` | Skill composition with semver ranges |

### OCI artifact contents

```
skill.yaml          # The SkillCard
system-prompt.txt   # Main prompt content
examples/           # Optional example files
assets/             # Optional supporting files
```

### Interoperability

`skillctl import --from-skill ./SKILL.md` converts Agent Skills
format to SkillCard. `skillctl export --format skill-md` produces
the reverse. This keeps the project compatible without being locked
to the Agent Skills schema.

## Naming model

Three layers of naming support both technical and non-technical
users.

| Layer | Rules | Example |
|---|---|---|
| Display name | Free-form UTF-8, max 128 chars | `Resume Reviewer (Strict)` |
| Skill identifier | `namespace/name`, lowercase + hyphens, max 64 each | `docsclaw/hr-resume-reviewer` |
| OCI reference | `registry/namespace/name:version` | `quay.io/docsclaw/hr-resume-reviewer:1.2.0` |

### Auto-conversion

The UI auto-generates identifiers from display names: lowercase,
spaces to hyphens, strip special characters, collapse consecutive
hyphens. Users can override before saving.

### In agent configs

```yaml
skills:
  - docsclaw/hr-resume-reviewer:1.2.0          # short form
  - quay.io/docsclaw/hr-resume-reviewer:1.2.0  # full OCI ref
```

The default registry is configurable via `skillctl config`.

### Nested namespaces

OCI registries support multi-level paths. The namespace is
everything between the registry host and the skill name.
Recommended: keep it shallow (org/team at most).

### Validation

| Field | Constraint |
|---|---|
| `name` | 1-64 chars, `[a-z0-9-]`, no leading/trailing/consecutive hyphens |
| `namespace` | 1-128 chars, `[a-z0-9-/]`, each segment follows name rules |
| `display-name` | 1-128 chars, UTF-8 |
| `version` | Valid semver |

### Agent runtime integration

How skills surface to end users (slash commands, menus, etc.) is
the agent runtime's responsibility. The SkillCard provides `name`,
`display-name`, `description`, and `tags` — sufficient metadata for
any runtime to present skills in its own style.

## REST API

**Base path:** `/api/v1`

### Skill CRUD and content

| Method | Path | Description |
|---|---|---|
| POST | `/skills` | Create skill (SkillCard + content) |
| GET | `/skills` | List (filter by namespace, tags, status) |
| GET | `/skills/{ns}/{name}/versions` | List versions |
| GET | `/skills/{ns}/{name}/versions/{ver}` | Get SkillCard |
| PUT | `/skills/{ns}/{name}/versions/{ver}` | Update draft |
| DELETE | `/skills/{ns}/{name}/versions/{ver}` | Delete draft only |
| GET | `/skills/{ns}/{name}/versions/{ver}/content` | Get prompt content |
| PUT | `/skills/{ns}/{name}/versions/{ver}/content` | Update prompt content |

### Lifecycle

| Method | Path | Description |
|---|---|---|
| POST | `/skills/{ns}/{name}/versions/{ver}/promote` | Promote |
| GET | `/skills/{ns}/{name}/versions/{ver}/history` | Promotion history |

### Discovery

| Method | Path | Description |
|---|---|---|
| GET | `/search?q=...&status=...&tags=...` | Full-text search |
| GET | `/skills/{ns}/{name}/versions/{ver}/diff/{ver2}` | Diff |
| GET | `/skills/{ns}/{name}/versions/{ver}/dependencies` | Dependencies |

### Eval signals

| Method | Path | Description |
|---|---|---|
| POST | `/skills/{ns}/{name}/versions/{ver}/evals` | Attach eval |
| GET | `/skills/{ns}/{name}/versions/{ver}/evals` | List evals |

### System

| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Health check |
| GET | `/openapi.yaml` | OpenAPI spec |

### Response format

```json
{
  "data": { },
  "_meta": { "request_id": "..." },
  "pagination": { "total": 42, "page": 1, "per_page": 20 }
}
```

Errors follow RFC 7807. OpenAPI spec served for UI client
generation.

## CLI (`skillctl`)

### Standalone (no server needed)

```
skillctl pack <dir>                          Pack into OCI artifact
skillctl push <oci-ref>                      Push to registry
skillctl pull <oci-ref> -o <dir>             Pull to local directory
skillctl inspect <oci-ref>                   Show SkillCard + manifest
skillctl verify <oci-ref>                    Verify cosign signature
skillctl diff <oci-ref> <ver1> <ver2>        Diff two versions
skillctl diff <oci-ref> --local <dir>        Diff against local
skillctl validate <dir>                      Validate SkillCard schema
skillctl import --from-skill <SKILL.md>      Import Agent Skills format
skillctl export <oci-ref> --format skill-md  Export as SKILL.md
```

### Server commands

```
skillctl serve --port 8080                   Start API server
skillctl search <query> --status published
skillctl promote <ns/name> <ver> --to <state>
skillctl history <ns/name> <ver>
skillctl eval attach <ns/name> <ver> --category <cat> --score <n>
skillctl eval list <ns/name> <ver>
```

### Config

```
skillctl config init                         Interactive setup
skillctl config set registry quay.io
skillctl config set server http://localhost:8080

Config path: ~/.config/skillctl/config.yaml
```

## Project structure

```
oci-skill-registry/
├── cmd/skillctl/           Entry point
├── internal/
│   ├── cli/                Cobra commands
│   ├── handler/            HTTP handlers
│   ├── service/            Business logic
│   ├── store/              Storage interface + SQLite
│   └── server/             Router, middleware
├── pkg/
│   ├── skillcard/          Parse, validate, serialize
│   ├── oci/                Pack/push/pull (oras-go)
│   ├── verify/             Sigstore verification
│   ├── lifecycle/          State machine
│   └── diff/               Version comparison
├── schemas/                JSON Schema for SkillCard
├── api/openapi.yaml        OpenAPI 3.1 spec
├── deploy/
│   ├── Dockerfile
│   └── k8s/                Kustomize overlays (k8s, openshift)
├── examples/               Sample skills
├── docs/                   Architecture, ADRs
└── Makefile
```

## Delivery targets

| Target | Mechanism |
|---|---|
| Local dev (Podman) | `skillctl pull` → mount into container |
| K8s 1.33+ / OpenShift 4.20+ | Image volumes (preferred) |
| Older K8s | Init container fallback |
| CI/CD | `skillctl pack && skillctl push` in pipeline |
| UI developer | OpenAPI spec → generated typed client |

## Key dependencies

| Dependency | Purpose |
|---|---|
| oras-go | OCI artifact operations |
| cosign | Signature verification |
| cobra/viper | CLI and config |
| chi | HTTP router |
| SQLite | Metadata storage (Postgres swap path) |

## Integration with agent-registry

Both projects share OCI registries as the common layer.
agent-registry stores metadata and governance signals;
oci-skill-registry stores actual skill content. Integration
is loosely coupled — no direct API dependency.

Future convergence: `skillctl` commands may become
`agentctl push skills`, `agentctl promote skills`, etc.
This depends on collaboration with azaalouk (TBD).

## Findings from landscape research

Competitive analysis (see companion document
`2026-04-15-oci-skill-registry-landscape.md`) identified five
gaps worth addressing. Items marked with priority are recommended
for inclusion in the first release; the rest are future work.

### Trust tiers (future)

Microsoft's Agent Governance Toolkit uses trust-tiered capability
gating: skills at different trust levels get different runtime
permissions. A "draft" skill from an unknown author should not
get the same tool access as a "published" skill signed by a
trusted org. This extends our lifecycle states with a
permission model.

### Shadow skill detection (future)

JFrog AI Catalog detects unauthorized AI usage across an
enterprise. Applied to skills: what skills are agents actually
loading at runtime vs. what's approved in the registry? This
requires runtime telemetry integration and is a future concern,
but the API should support querying "who is using this skill."

### Skill collections and bundles (priority)

Thomas Vitale's OCI Skills spec proposes "collections" — OCI
Image Indexes that group related skills into discoverable
bundles. Example: an "HR Skills Pack" containing onboarding,
resume-review, and policy-lookup skills. This is useful for
enterprise distribution (install a curated set, not individual
skills) and aligns with our OCI-native approach.

**Action:** Support collections as a first-class concept.
A collection is an OCI Image Index referencing multiple skill
artifacts. `skillctl` should support `pack --collection`,
`push`, and `pull` for collections.

### Security scanning integration (future)

Tessl integrates Snyk for vulnerability scanning of skill
content. The ClawHavoc incident (341 malicious skills on ClawHub
in February 2026) validates that skill content should be scanned.
Our registry should support pluggable security scanners as eval
signal providers, reusing the eval attachment API.

### Dependency resolution with lock files (priority)

Thomas Vitale's spec introduces `skills.json` (declarative
dependencies) and `skills.lock.json` (resolved digests), mirroring
npm's package management model. This is more mature than our
current `spec.dependencies` with semver ranges.

**Action:** Adopt a similar two-file model. `skills.json` declares
what skills an agent needs with version ranges. `skills.lock.json`
pins exact OCI digests for reproducible deployments. `skillctl`
resolves and locks.

### Alignment with Thomas Vitale's OCI Skills spec (priority)

Vitale's spec is gaining community traction and has reference
implementations (Arconia CLI in Java, skills-oci in Go). He is
a known contact. We should align our SkillCard schema and OCI
artifact layout with his spec rather than diverge.

**Action:** Engage Thomas Vitale early. Align on artifact type
identifiers, media types, and layer layout. Contribute our
lifecycle and signing extensions back to the community spec.

## Future work

- **RBAC and workflow customization:** configurable policies for
  who can author, approve, and deprecate skills
- **gRPC interface:** when programmatic consumers justify it
- **Sidecar puller:** live skill upgrades without pod restart
- **Full sigstore integration:** end-to-end signing and
  verification (requires signing infrastructure)
- **Trust score computation:** separate service, potentially
  integrated with agent-registry eval signals
- **Trust tiers:** permission gating based on skill trust level
- **Shadow skill detection:** runtime telemetry for skill usage
  visibility
- **Security scanning:** pluggable scanners as eval signal
  providers
