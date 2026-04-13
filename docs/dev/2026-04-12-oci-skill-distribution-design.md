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
  metadata:                       # pass-through from SKILL.md frontmatter
    category: document-tools
spec:
  tools:
    required: [exec]              # what the skill needs from the platform
    optional: [read_file]
  allowedTools: "Bash(pandoc:*)"  # from SKILL.md allowed-tools (community field)
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
    environment: "Requires pandoc 3.x"  # from SKILL.md compatibility field
```

### Field reference

| Field | Required | Description |
| ----- | -------- | ----------- |
| `apiVersion` | yes | Schema version (`docsclaw.io/v1alpha1`) |
| `kind` | yes | Always `SkillCard` |
| `metadata.name` | yes | Skill identifier; must follow Agent Skills naming rules (lowercase, hyphens, 1-64 chars, must match directory name) |
| `metadata.namespace` | yes | Ownership scope (e.g., `official`, `research`) |
| `metadata.ref` | yes | Canonical OCI reference (without tag) |
| `metadata.version` | yes | Semver version |
| `metadata.description` | yes | Human-readable description (max 1024 chars per Agent Skills spec) |
| `metadata.author` | yes | Skill author or organization |
| `metadata.license` | no | SPDX license identifier |
| `metadata.metadata` | no | Arbitrary key-value pairs; mirrors SKILL.md `metadata` field |
| `spec.tools.required` | no | Tools the skill needs from the agent platform |
| `spec.tools.optional` | no | Tools the skill can use but does not require |
| `spec.allowedTools` | no | Pre-approved tool patterns; mirrors SKILL.md `allowed-tools` field |
| `spec.dependencies.skills` | no | Other skills that must be loaded first |
| `spec.dependencies.toolPacks` | no | External binary dependencies (OCI refs) |
| `spec.resources.estimatedMemory` | no | Resource hint for quota enforcement |
| `spec.resources.estimatedCPU` | no | Resource hint for quota enforcement |
| `spec.compatibility.minAgentVersion` | no | Minimum agent version |
| `spec.compatibility.environment` | no | Free-text environment requirements; mirrors SKILL.md `compatibility` field |

### Agent Skills spec compatibility

The SkillCard maps every SKILL.md frontmatter field defined by the
[Agent Skills Specification](https://agentskills.io/specification):

| SKILL.md field | SkillCard location | Notes |
| -------------- | ------------------ | ----- |
| `name` | `metadata.name` | Same constraints enforced |
| `description` | `metadata.description` | Same 1024-char limit |
| `license` | `metadata.license` | Direct mapping |
| `compatibility` | `spec.compatibility.environment` | Free text, same semantics |
| `metadata` | `metadata.metadata` | Pass-through key-value map |
| `allowed-tools` | `spec.allowedTools` | Also populated in OCI config blob for community tool compatibility |

The `docsclaw skill pack` command validates that `metadata.name`
matches the skill directory name, as required by the Agent Skills
spec. The `pack` command also validates naming constraints: lowercase,
hyphens only, no leading/trailing/consecutive hyphens, max 64 chars.

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

## Deployment models

Two deployment models are supported, depending on the target
platform version. Image volumes are the preferred approach;
init containers are the fallback for older clusters.

### Image volumes (preferred)

Kubernetes 1.33+ and OpenShift 4.20+ support [image volumes][k8s-image-vol]:
OCI images (and artifacts) can be mounted directly as read-only
volumes in a pod without an init container, sidecar, or extra
storage.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: docsclaw-agent
spec:
  containers:
    - name: docsclaw
      image: ghcr.io/redhat-et/docsclaw:latest
      volumeMounts:
        - name: skill-code-review
          mountPath: /skills/code-review
        - name: skill-doc-prep
          mountPath: /skills/doc-prep
  volumes:
    - name: skill-code-review
      image:
        reference: quay.io/docsclaw/official/skill-code-review:1.0.0
        pullPolicy: IfNotPresent
    - name: skill-doc-prep
      image:
        reference: quay.io/docsclaw/official/skill-doc-prep:1.2.0
        pullPolicy: IfNotPresent
```

**How it works:** The kubelet pulls the OCI object via the container
runtime (CRI-O on OpenShift), unpacks the layers into a directory,
and bind-mounts it into the container — read-only, no emptyDir, no
node ephemeral storage consumed. The container runtime's existing
image store and garbage collection manage the cache.

**Advantages over init containers:**

| Concern | Image volume | Init container |
| ------- | ------------ | -------------- |
| Node storage | Uses container runtime image store (managed, GC'd) | Writes to emptyDir or PVC (fills node temp storage) |
| Complexity | Native K8s primitive, no extra containers | Extra container, extra code, extra failure mode |
| Pull caching | Kubelet image cache with standard pull policies | Must implement own caching or re-pull on every restart |
| Security | Platform-level image signature verification | Application-level verification in init container |

**Signature verification with image volumes:** Since the kubelet
pulls the image (not our code), signature verification happens at
the platform level. OpenShift already supports this via
`containers-policy.json` and [image signature verification
policies][ocp-sig-verify]. This is actually a stronger trust model:
the platform enforces trust, not the agent — consistent with the
principle that the agent should not decide what to trust.

**Skill artifact packaging for image volumes:** The skill OCI
artifact must be mountable as a filesystem by the container runtime.
This means packaging the skill directory as a `FROM scratch` image
with the content layer, rather than a pure OCI artifact with custom
media types. Concretely, the `docsclaw skill push` command would
produce a minimal container image:

```dockerfile
FROM scratch
COPY skill.yaml SKILL.md scripts/ references/ assets/ /
```

The community-compatible content layer
(`application/vnd.agentskills.skill.content.v1.tar+gzip`) already
packages the skill as a tarball, which aligns with how container
image layers work. The SkillCard (layer 0) would be included in the
same image as a separate file rather than a separate OCI layer, so
it is accessible after mount.

[k8s-image-vol]: https://kubernetes.io/docs/concepts/storage/volumes/#image
[ocp-sig-verify]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/container-image-signatures

### Init container (fallback)

For clusters that do not support image volumes (K8s < 1.33,
OpenShift < 4.20), skills are pulled via an init container. Based
on feedback from the ORAS maintainer team, the init container
should use a **PVC** (not emptyDir) to avoid filling node ephemeral
storage and to persist the skill cache across pod restarts.

```text
Pod
├── init: skill-puller
│   ├── runs: docsclaw skill pull --verify <ref1> <ref2> ...
│   ├── writes to: /skills/ (PVC)
│   └── reads keys from: /etc/docsclaw/keys/ (Secret volume)
│
└── container: docsclaw-agent
    ├── mounts: /skills/ (PVC)
    └── discovers skills via: skills.Discover("/skills/")
```

The init container image is the same `docsclaw` binary. The skill
refs and verification key are configured via environment variables or
a ConfigMap. Signature verification happens at the application level
via sigstore-go.

**When to use init containers:**

- Clusters without image volume support
- When application-level signature verification is required (e.g.,
  the cluster does not have image signature policies configured)
- When skills need to be pulled from registries that require custom
  authentication not available to the kubelet

### Agent Sandbox compatibility

[Agent Sandbox][agent-sandbox] is a Kubernetes SIG Apps project that
provides a `Sandbox` CRD (`agents.x-k8s.io/v1alpha1`) for managing
isolated, stateful, singleton pods. It offers stable identity,
persistent storage, lifecycle management (shutdown timers,
pause/resume), network isolation via NetworkPolicies, and a
`SandboxTemplate` + `SandboxClaim` pattern for templated provisioning
with pre-warmed pools.

Agent Sandbox has no opinion about skills, agents, or AI workloads
specifically — it is a generic "managed singleton pod" abstraction.
Our OCI skill distribution design is fully compatible because the two
projects operate at different layers:

| Concern | Agent Sandbox | DocsClaw OCI Skills |
| ------- | ------------- | ------------------- |
| Pod lifecycle | Manages it | Not involved |
| Container image | User provides | DocsClaw agent image |
| Skill delivery | Not addressed | Image volume or init container |
| Network isolation | NetworkPolicy | Not addressed |
| Persistent storage | PVC management | Could persist skill cache |
| Signature verification | Not addressed | Platform policy or sigstore |

A DocsClaw agent with OCI-distributed skills deployed as an Agent
Sandbox using image volumes:

```yaml
apiVersion: agents.x-k8s.io/v1alpha1
kind: Sandbox
metadata:
  name: docsclaw-agent
spec:
  podTemplate:
    spec:
      containers:
        - name: docsclaw
          image: ghcr.io/redhat-et/docsclaw:latest
          volumeMounts:
            - name: skill-code-review
              mountPath: /skills/code-review
      volumes:
        - name: skill-code-review
          image:
            reference: quay.io/docsclaw/official/skill-code-review:1.0.0
            pullPolicy: IfNotPresent
  lifecycle:
    shutdownPolicy: Retain
```

This is cleaner than the init container approach — no extra
containers, no emptyDir, no PVC. The skill image is pulled and
mounted by the kubelet like any other volume.

**Future opportunity:** Agent Sandbox's `SandboxTemplate` +
`SandboxClaim` pattern could serve as the foundation for skill
assignment at scale. A `SandboxTemplate` could define the base agent
image + default skill set as image volumes, and `SandboxClaim`
handles provisioning for individual users. This may reduce or
eliminate the need for a custom operator.

**Future opportunity:** Agent Sandbox's `SandboxTemplate` +
`SandboxClaim` pattern could serve as the foundation for skill
assignment at scale. A `SandboxTemplate` could define the base agent
image + default skill set, and `SandboxClaim` handles provisioning
for individual users. This may reduce or eliminate the need for a
custom operator.

[agent-sandbox]: https://github.com/kubernetes-sigs/agent-sandbox

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
1. `docsclaw skill push --sign` — push to a local Zot registry, sign
   with cosign key
1. `docsclaw skill pull --verify` — pull on a clean environment,
   signature verified
1. `docsclaw skill pull` an unsigned skill with `mode: enforce` —
   rejected with clear error
1. Mount the pushed skill as an image volume in a pod on
   OpenShift 4.20 — show the skill files appearing at `/skills/`
   without an init container
1. Show the agent discovering and using the mounted skill

## Future work

This PoC establishes the packaging format and trust chain. Future
phases build on it:

- **Sidecar puller** for live skill upgrades on long-running agents
- **Tool packs** as separate OCI artifacts for binary dependencies
- **Keyless verification** via Fulcio for identity-based trust
- **Kubernetes operator** with CRD for skill assignment and policy
  enforcement, potentially built on [Agent Sandbox][agent-sandbox]
  `SandboxTemplate` + `SandboxClaim` instead of a custom CRD
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
- [Agent Sandbox][agent-sandbox] — Kubernetes SIG Apps project for
  isolated, stateful, singleton agent pods; compatible deployment
  target for DocsClaw with OCI skills
- [Image Volumes KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source) —
  Kubernetes enhancement for mounting OCI images/artifacts as pod
  volumes; beta in K8s 1.33+, available in OpenShift 4.20+
- [Zot Registry](https://zotregistry.dev/) — lightweight OCI-native
  registry; recommended for local development and testing
