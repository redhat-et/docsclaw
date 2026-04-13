# OCI-based skill distribution design

## Overview

Enable skills to be packaged, signed, and distributed as OCI artifacts.
This gives enterprises a trust chain for skill provenance — only
approved, signed skills from trusted registries can be deployed to
agents.

**Core paradigm:** agents are operating systems, skills are
applications. Like RPM packages on RHEL, skills are versioned,
signed, and installed from trusted repositories.

**Related issue:** #4

## Prior art and community alignment

This design does not exist in a vacuum. The [Agent Skills OCI Artifacts
Specification][vitale-spec] (v0.1.0, draft) by Thomas Vitale defines a
comprehensive, registry-agnostic standard for packaging skills as OCI
artifacts. The accompanying [blog post][vitale-blog] explains the
motivation and provides worked examples. A Go-based reference
implementation ([skills-oci][skills-oci-go]) by Mauricio Salatino uses
the ORAS Go client, which aligns with our technology choice.

[vitale-spec]: https://github.com/ThomasVitale/agents-skills-oci-artifacts-spec
[vitale-blog]: https://www.thomasvitale.com/agent-skills-as-oci-artifacts/
[skills-oci-go]: https://github.com/salaboy/skills-oci

The community spec is developer-tool-oriented: skills are installed
into project directories like npm packages (`skills.json` +
`skills.lock.json`). Our design is platform-oriented: skills are
deployed into running agent containers by operations teams on
Kubernetes/OpenShift. Both perspectives are valid and complementary.

### Where we align

- OCI artifacts as the distribution primitive (oras-go SDK)
- Cosign/sigstore for signing and verification
- Standard OCI annotations (`org.opencontainers.image.*`)
- Agent Skills annotation namespace (`io.agentskills.*`)
- Config blob with structured metadata accessible without unpacking
- Semantic versioning for skill releases

### Where we diverge (and why)

| Area | Community spec | Our design | Rationale |
| ---- | -------------- | ---------- | --------- |
| Content layer | Single `tar+gzip` | Two layers (SkillCard + SKILL.md) | Enables metadata-only pulls for lazy loading and catalog browsing on the platform |
| Skill metadata | OCI config JSON mirroring SKILL.md frontmatter | Separate `skill.yaml` (SkillCard) with K8s-style schema | Adds resource hints, tool dependency declarations, and tool pack refs not in the community spec |
| Verification policy | "Use cosign" (no policy format) | `SkillPolicy` resource with enforce/warn/skip modes | Deployable as a ConfigMap; platform-level enforcement |
| Deployment model | `skills install` into project dirs | Init container pulling into shared volume | K8s-native; skills are platform-managed, not developer-managed |
| Resource awareness | Not addressed | `estimatedMemory`/`estimatedCPU` in SkillCard | Enables quota enforcement on OpenShift |
| Tool dependencies | `allowedTools` (what skill may invoke) | `tools.required`/`optional` + `toolPacks` | Distinguishes what a skill *needs from the platform* vs what it *is allowed to do* |
| Lock file | `skills.lock.json` in project root | Deployment audit log in ConfigMap (see below) | Adapted for K8s; audit trail per pod, not per project |

### Interoperability strategy

To maximize compatibility with the emerging community standard:

1. **Use shared media types.** We adopt the `application/vnd.agentskills.*`
   namespace for artifact type and config, matching the community spec.
   Our SkillCard layer uses a DocsClaw-specific media type since it is
   an extension not present in the community spec.
1. **Use standard OCI annotations.** We emit both `org.opencontainers.image.*`
   and `io.agentskills.*` annotations on our manifests, so community
   tools can read our artifacts and vice versa.
1. **Keep content extractable.** Once pulled and extracted, our skill
   directories are standard Agent Skills directories — any conforming
   tool can consume them.
1. **Contribute upstream.** Where our extensions prove useful (resource
   hints, tool dependencies, verification policies), propose them to
   the community spec.

## Goals

- Package skills as OCI artifacts with structured metadata (SkillCard)
- Sign skills using sigstore and verify signatures at pull time
- Provide CLI commands for the full lifecycle: pack, push, pull,
  verify, inspect
- Maintain backward compatibility with existing filesystem-based skills
- Demonstrate end-to-end trust chain in a demoable PoC

## Non-goals (future work)

| Item | Reason deferred |
| ---- | --------------- |
| Sidecar puller / hot-reload | Production feature, not needed for PoC |
| Tool packs (binary deps) | Separate artifact format, adds complexity |
| Keyless (Fulcio) verification | Adds OIDC dependency |
| Kubernetes operator / CRD | Platform-level enforcement, phase 2 |
| Quota enforcement | Requires operator |
| Skill dependency resolution | No cross-dependent skills yet |
| Skill catalog UI | UX concern, not runtime |

## SkillCard schema

The SkillCard is a YAML file (`skill.yaml`) that lives alongside
`SKILL.md` in each skill directory. It is the machine-readable metadata
for a skill — what `agent-card.json` is to agents.

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: document-prep
  namespace: official
  ref: quay.io/docsclaw/official/skill-document-prep
  version: 1.2.0
  description: Convert documents between formats using pandoc
  author: Red Hat ET
  license: Apache-2.0
spec:
  tools:
    required: [exec]
    optional: [read_file]
  dependencies:
    skills: []
    toolPacks:
      - name: pandoc
        ref: quay.io/docsclaw/toolpack-pandoc:3.1
  resources:
    estimatedMemory: 64Mi
    estimatedCPU: 100m
  compatibility:
    minAgentVersion: "0.5.0"
```

### Field reference

| Field | Required | Description |
| ----- | -------- | ----------- |
| `apiVersion` | yes | Schema version (`docsclaw.io/v1alpha1`) |
| `kind` | yes | Always `SkillCard` |
| `metadata.name` | yes | Short name used with `load_skill` |
| `metadata.namespace` | yes | Ownership scope (e.g., `official`, `research`) |
| `metadata.ref` | yes | Canonical OCI reference (without tag) |
| `metadata.version` | yes | Semver version |
| `metadata.description` | yes | Human-readable description |
| `metadata.author` | yes | Skill author or organization |
| `metadata.license` | no | SPDX license identifier |
| `spec.tools.required` | no | Tools the skill needs from the agent |
| `spec.tools.optional` | no | Tools the skill can use but does not require |
| `spec.dependencies.skills` | no | Other skills that must be loaded first |
| `spec.dependencies.toolPacks` | no | External binary dependencies (OCI refs) |
| `spec.resources.estimatedMemory` | no | Resource hint for quota enforcement |
| `spec.resources.estimatedCPU` | no | Resource hint for quota enforcement |
| `spec.compatibility.minAgentVersion` | no | Minimum agent version |

### Naming convention

Skills use the OCI registry's native path hierarchy for namespacing:

```text
quay.io/<org>/<namespace>/skill-<name>:<version>
```

Examples:

```text
quay.io/docsclaw/official/skill-code-review:1.0.0
quay.io/redhat-et/research/skill-code-review:2.1.0
quay.io/acme-corp/internal/skill-document-prep:1.0.0
```

When two skills share the same short name but different namespaces, the
agent uses the qualified form: `load_skill official/code-review` vs
`load_skill research/code-review`. If there is no ambiguity, the short
name works.

## OCI artifact layout

Each skill is packaged as an OCI artifact with two layers, each
identified by media type:

```text
quay.io/docsclaw/official/skill-code-review:1.0.0
├── manifest.json
│   ├── artifactType: application/vnd.agentskills.skill.v1
│   ├── config: application/vnd.agentskills.skill.config.v1+json
│   ├── layers:
│   │   ├── [0] application/vnd.docsclaw.skill.card.v1+yaml  → skill.yaml
│   │   └── [1] application/vnd.agentskills.skill.content.v1.tar+gzip → skill dir
│   └── annotations:
│       ├── org.opencontainers.image.title: code-review
│       ├── org.opencontainers.image.version: 1.0.0
│       ├── io.agentskills.skill.name: code-review
│       └── io.docsclaw.skill.resources.memory: 32Mi
│
└── cosign signature (via Referrers API or tag fallback)
```

### Design decisions

- **Two layers instead of one tarball.** The community spec uses a
  single `tar+gzip` layer containing the entire skill directory. We
  add layer 0 (SkillCard) as a separate, lightweight YAML blob. This
  enables "pull metadata only" for catalog browsing, lazy loading, and
  resource-aware scheduling — the platform can check dependencies and
  resource hints without downloading skill content. Layer 1 contains
  the full skill directory as `tar+gzip`, matching the community spec
  content format for interoperability.
- **Shared artifact type and config.** We use the community
  `application/vnd.agentskills.skill.v1` artifact type and config
  media type so that community tools recognize our artifacts. The
  SkillCard layer uses a DocsClaw-specific media type since it is an
  extension. Tools that don't understand it simply skip layer 0 and
  extract layer 1 as usual.
- **Standard + extended annotations.** We emit standard OCI annotations
  and `io.agentskills.*` annotations for community compatibility, plus
  `io.docsclaw.*` annotations for platform-specific metadata (resource
  hints, tool dependencies).
- **Cosign-compatible signatures.** We prefer the OCI v1.1 Referrers
  API for attaching signatures (the modern approach), with fallback to
  the `sha256-<digest>.sig` tag convention for older registries.
  Verifiable with both `docsclaw skill verify` and `cosign verify`.

### Media types

| Media type | Source | Content |
| ---------- | ------ | ------- |
| `application/vnd.agentskills.skill.v1` | community | Artifact type identifier |
| `application/vnd.agentskills.skill.config.v1+json` | community | Config blob (name, version, description) |
| `application/vnd.docsclaw.skill.card.v1+yaml` | ours | SkillCard with resource hints and deps |
| `application/vnd.agentskills.skill.content.v1.tar+gzip` | community | Skill directory as tarball |

### Annotations

| Annotation | Source | Description |
| ---------- | ------ | ----------- |
| `org.opencontainers.image.title` | OCI standard | Human-readable skill name |
| `org.opencontainers.image.version` | OCI standard | Semver version |
| `org.opencontainers.image.description` | OCI standard | Short description |
| `org.opencontainers.image.source` | OCI standard | Source repository URL |
| `org.opencontainers.image.licenses` | OCI standard | SPDX license |
| `io.agentskills.skill.name` | community | Machine-readable skill identifier |
| `io.docsclaw.skill.resources.memory` | ours | Estimated memory (e.g., `64Mi`) |
| `io.docsclaw.skill.resources.cpu` | ours | Estimated CPU (e.g., `100m`) |
| `io.docsclaw.skill.tools.required` | ours | Comma-separated required tools |

## Signature verification

### Trust configuration

The agent or init container reads a verification policy from a
ConfigMap:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: SkillPolicy
verification:
  mode: enforce
  trustedKeys:
    - name: redhat-et
      publicKey: /etc/docsclaw/keys/redhat-et.pub
  keyless:                    # not implemented in PoC
    enabled: false
    allowedIssuers:
      - https://accounts.google.com
    allowedIdentities:
      - "*@redhat.com"
```

### Verification modes

| Mode | Behavior | Use case |
| ---- | -------- | -------- |
| `enforce` | Reject unsigned or untrusted skills | Production |
| `warn` | Log warning, allow loading | Migration / testing |
| `skip` | No verification | Local development |

### Pull-time verification flow

1. Pull the OCI manifest for the skill ref
1. Look up the cosign signature (tag `sha256-<digest>.sig`)
1. If `mode: enforce` and no signature found — reject with clear error
1. Verify signature against configured public keys using sigstore-go
1. If verification passes — pull layers and cache
1. If verification fails — reject, log the signer identity mismatch

### PoC scope

Key-based verification only (no keyless/Fulcio). One trusted key
configured via CLI flag or environment variable.

## CLI commands

The PoC adds a `skill` subcommand group to the `docsclaw` CLI:

```text
docsclaw skill pack <skill-dir>              # package into local OCI layout
docsclaw skill push <skill-dir> <ref>        # pack + push to registry
docsclaw skill pull <ref> [--verify]         # pull and verify signature
docsclaw skill verify <ref>                  # verify signature only
docsclaw skill inspect <ref>                 # show SkillCard metadata
```

### Command details

**`pack`** — reads `skill.yaml` and `SKILL.md` from the directory,
validates the SkillCard schema, produces a local OCI layout on disk.

**`push`** — packs and pushes to the registry. Optionally signs:

```bash
docsclaw skill push ./skills/code-review \
  quay.io/docsclaw/official/skill-code-review:1.0.0 \
  --sign --key cosign.key
```

**`pull`** — pulls to a local skills cache directory
(`~/.docsclaw/skills/` or configurable). With `--verify`, checks the
sigstore signature before accepting.

**`verify`** — standalone signature check without pulling content.
Useful for CI pipelines or policy enforcement scripts.

**`inspect`** — pulls the config blob and layer 0 (SkillCard), verifies
the signature, and displays metadata: name, description, version,
required tools, dependencies, resource estimates. Like
`skopeo inspect` for skills.

### Authentication

Uses the standard OCI credential chain: `~/.docker/config.json` or
environment variables. No custom auth mechanism.

## Package architecture

Three new packages following DocsClaw's existing pattern:

| Package | Location | Responsibility |
| ------- | -------- | -------------- |
| SkillCard types | `pkg/skills/card/` | Schema types, validation, parsing |
| OCI operations | `internal/oci/` | Artifact packing, pushing, pulling via oras-go |
| Verification | `internal/verify/` | Signature verification via sigstore-go |

### Why `pkg/skills/card/` is public

The SkillCard types belong in `pkg/` because external tooling (other
projects, CI scripts) will want to generate or read SkillCards without
importing DocsClaw internals.

### Key interfaces

**`pkg/skills/card/`:**

- `SkillCard` struct matching the YAML schema
- `Parse(path string) (SkillCard, error)` — read and validate
- `Validate(SkillCard) error` — check required fields, version format

**`internal/oci/`:**

- `Pack(skillDir string) (ocispec.Descriptor, error)`
- `Push(ctx, ref, skillDir string, opts PushOptions) error`
- `Pull(ctx, ref, destDir string, opts PullOptions) error`
- `FetchCard(ctx, ref string) (card.SkillCard, error)` — metadata only

**`internal/verify/`:**

- `Verify(ctx context.Context, ref string, policy Policy) error`
- `Policy` struct: signer identity, allowed issuers, public key path

### CLI layer

Commands live in `internal/cmd/skill_*.go` as Cobra subcommands under
a `skillCmd` group. They parse flags, call the packages above, and
format output. No business logic in the command layer.

### Backward compatibility

`pkg/skills/loader.go` gains the ability to read `skill.yaml` in
addition to `SKILL.md` frontmatter. When both exist, the SkillCard
takes precedence. Skills without `skill.yaml` continue to work as
before.

## Dependencies

| Library | Purpose | Justification |
| ------- | ------- | ------------- |
| `oras.land/oras-go/v2` | OCI artifact push/pull | Mature Go SDK, no CLI deps |
| `github.com/sigstore/sigstore-go` | Signature verification | What cosign is built on |

No CLI shelling out. Both libraries are embeddable and testable as
unit tests.

## Deployment model (PoC)

For the PoC, skills are pulled into the agent container via a
Kubernetes init container:

```text
Pod
├── init: skill-puller
│   ├── runs: docsclaw skill pull --verify <ref1> <ref2> ...
│   ├── writes to: /skills/ (emptyDir volume)
│   └── reads keys from: /etc/docsclaw/keys/ (Secret volume)
│
└── container: docsclaw-agent
    ├── mounts: /skills/ (emptyDir volume)
    └── discovers skills via: skills.Discover("/skills/")
```

The init container image is the same `docsclaw` binary. The skill
refs and verification key are configured via environment variables or
a ConfigMap.

## Deployment audit trail

The community spec defines a `skills.lock.json` lock file for
reproducible installs in developer projects. In our K8s deployment
model the equivalent concern is auditing: which exact skill digests
were running in a pod when an incident occurred?

The init container writes a `skills-installed.json` file after
pulling all skills. This file follows the community lock file schema
where applicable but is adapted for our deployment context:

```json
{
  "lockfileVersion": 1,
  "generatedAt": "2026-04-12T10:30:00Z",
  "podName": "docsclaw-agent-7b9f4",
  "skills": [
    {
      "name": "code-review",
      "source": {
        "registry": "quay.io",
        "repository": "docsclaw/official/skill-code-review",
        "tag": "1.0.0",
        "digest": "sha256:abc123...",
        "ref": "quay.io/docsclaw/official/skill-code-review:1.0.0@sha256:abc123..."
      },
      "verified": true,
      "verifiedBy": "redhat-et",
      "installedAt": "2026-04-12T10:30:00Z"
    }
  ]
}
```

Key differences from the community lock file:

- **`podName`** — identifies the deployment instance
- **`verified` / `verifiedBy`** — records whether the signature was
  checked and which trusted key matched
- **No `path` or `additionalPaths`** — skills always land in
  `/skills/` on the shared volume; no multi-vendor directory logic

For the PoC, this file is written to the shared volume alongside the
skills. In production, it could be emitted as a Kubernetes Event or
pushed to a central audit log.

## Demo scenario

1. Show an existing skill directory with `SKILL.md` and new `skill.yaml`
1. `docsclaw skill pack ./skills/code-review` — produce local OCI layout
1. `docsclaw skill inspect` the local layout — show SkillCard metadata
1. `docsclaw skill push --sign` — push to quay.io, sign with cosign key
1. `docsclaw skill pull --verify` — pull on a clean environment,
   signature verified
1. `docsclaw skill pull` an unsigned skill with `mode: enforce` —
   rejected with clear error
1. Show the agent discovering and using the pulled skill

## Future work

This PoC establishes the packaging format and trust chain. Future
phases build on it:

- **Sidecar puller** for live skill upgrades on long-running agents
- **Tool packs** as separate OCI artifacts for binary dependencies
- **Keyless verification** via Fulcio for identity-based trust
- **Kubernetes operator** with CRD for skill assignment and policy
  enforcement
- **Resource quotas** using SkillCard resource hints
- **Skill dependency resolution** when cross-dependent skills emerge
- **Skills Collection** using OCI Image Index for catalog browsing,
  following the community spec's collection artifact format
- **Catalog / "mart" UI** for browsing available skills
- **Upstream contributions** — propose resource hints, tool dependency
  declarations, and verification policy format to the [Agent Skills
  OCI Artifacts Specification][vitale-spec]

## References

- [Agent Skills OCI Artifacts Specification][vitale-spec] — community
  standard for OCI-based skill distribution (v0.1.0, draft)
- [Agent Skills as OCI Artifacts][vitale-blog] — blog post by Thomas
  Vitale explaining the motivation and architecture
- [Agent Skills Specification](https://agentskills.io/specification) —
  upstream skill format specification by Anthropic
- [ORAS](https://oras.land/) — OCI Registry As Storage (CNCF)
- [sigstore-go](https://github.com/sigstore/sigstore-go) — Go library
  for sigstore verification
- [Lola](https://github.com/RedHatProductSecurity/lola) — Red Hat
  Product Security skill manager (interactive CLI, RPM-inspired model)
