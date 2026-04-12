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
│   ├── config: application/vnd.docsclaw.skill.config+json
│   └── layers:
│       ├── [0] application/vnd.docsclaw.skill.card+yaml    → skill.yaml
│       └── [1] application/vnd.docsclaw.skill.content+md   → SKILL.md
│
└── cosign signature (tag: sha256-<digest>.sig)
```

### Design decisions

- **Two layers, not one tarball.** Separating the SkillCard from the
  content enables "pull metadata only" for catalog browsing and lazy
  loading. The agent can pull layer 0 to check dependencies and
  resources without downloading the full skill content.
- **Config blob.** A JSON blob with the skill name, version, and
  creation timestamp. Registries like quay.io display this in their UI.
- **Cosign-compatible signatures.** Attached using the standard tag
  convention. Verifiable with both `docsclaw skill verify` and
  `cosign verify` — no lock-in.

### Media types

| Media type | Content |
| ---------- | ------- |
| `application/vnd.docsclaw.skill.config+json` | Config (name, version, timestamp) |
| `application/vnd.docsclaw.skill.card+yaml` | SkillCard metadata |
| `application/vnd.docsclaw.skill.content+md` | SKILL.md content |

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

**`inspect`** — pulls only layer 0 (SkillCard), verifies the signature,
and displays metadata: name, description, version, required tools,
dependencies, resource estimates. Like `skopeo inspect` for skills.

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
- **Catalog / "mart" UI** for browsing available skills
