# OCI skill distribution guide

Package, sign, and distribute skills as OCI artifacts using the
[skillctl](https://github.com/redhat-et/skillimage) CLI. Skills
can be pushed to any OCI-compliant registry (quay.io, GHCR,
Harbor, Zot) and consumed in two ways:

- **Individual users** pull skills with `oras` or `skillctl` CLI
- **Platform operators** mount skills as image volumes on
  OpenShift 4.20+ — no init container needed

DocsClaw discovers mounted skills at startup and automatically
populates the AgentCard's `skills` array so the A2A endpoint
reflects runtime-available capabilities.

## Prerequisites

- [skillctl](https://github.com/redhat-et/skillimage) CLI
  (`go install github.com/redhat-et/skillimage/cmd/skillctl@latest`)
- An OCI-compliant registry (Zot recommended for local testing)
- Optional: [oras CLI](https://oras.land/) for pulling skills
  without skillctl
- Optional: cosign key pair for signing

## Skill structure

Each OCI-distributed skill has two files:

```text
my-skill/
├── SKILL.md        # Instructions (Agent Skills spec format)
└── skill.yaml      # SkillCard metadata (resource hints, deps)
```

The `SKILL.md` follows the [Agent Skills Specification][agentskills]
with YAML frontmatter (name, description) and markdown instructions.

The `skill.yaml` adds platform-aware metadata:

```yaml
apiVersion: docsclaw.io/v1alpha1
kind: SkillCard
metadata:
  name: resume-screener
  namespace: official
  ref: quay.io/docsclaw/skill-resume-screener
  version: 1.0.0
  description: >-
    Screen resumes against a job description. Use when HR
    uploads resumes for a job opening.
  author: Red Hat ET
  license: Apache-2.0
spec:
  tools:
    required: [read_file]
    optional: [write_file]
  resources:
    estimatedMemory: 32Mi
    estimatedCPU: 100m
```

See `examples/skills/` for complete examples (resume-screener,
policy-comparator, checklist-auditor).

## Two OCI formats

Skills are pushed in two formats for different audiences. Both
live in the same registry, typically with different tags.

| | Artifact (default) | Image (`--as-image`) |
|-|-------------------|---------------------|
| **Audience** | Individual users, personal agents | Platform deployments on K8s |
| **Pull tool** | `oras pull` or `skillctl pull` | Kubelet (image volume mount) |
| **Format** | Each file is a separate OCI layer | Single tar+gzip layer |
| **Result** | Files extracted directly | Mounted as read-only volume |

**Publishing workflow:** push both formats for each skill release:

```bash
# Artifact format — for oras pull / skillctl pull
skillctl push examples/skills/resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0

# Image format — for OpenShift image volume mounting
skillctl push --as-image examples/skills/resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0-image
```

See [ADR-0001](adr/0001-oci-skill-dual-format.md) for the design
rationale behind the dual format.

## CLI commands

### Pack a skill

Package a skill directory into a local OCI layout:

```bash
skillctl pack examples/skills/resume-screener
```

Use `--as-image` for the image format, `-o` for output directory,
`--force` to overwrite an existing layout:

```bash
skillctl pack --as-image --force -o /tmp/layout \
  examples/skills/resume-screener
```

### Push a skill to a registry

```bash
# Artifact format (default)
skillctl push examples/skills/resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0

# Image format
skillctl push --as-image examples/skills/resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0-image
```

For local registries without TLS:

```bash
skillctl push --tls-verify=false \
  examples/skills/resume-screener \
  localhost:5000/skill-resume-screener:1.0.0
```

### Pull a skill

With DocsClaw:

```bash
skillctl pull -o /tmp/skills \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

With oras (no DocsClaw needed):

```bash
oras pull -o resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

Both produce the same result:

```text
resume-screener/
├── SKILL.md
└── skill.yaml
```

### Inspect a skill

Show SkillCard metadata without pulling content:

```bash
skillctl inspect \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

### List and delete local skills

```bash
docsclaw skill list  # or: skillctl list /tmp/skills
docsclaw skill delete resume-screener --dir /tmp/skills
```

### Verify a skill signature

```bash
skillctl verify --key cosign.pub \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

## Deploy on OpenShift 4.20+ with image volumes

Use skills pushed with `--as-image`. The kubelet pulls and caches
them using the container runtime's image store.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hr-agent
spec:
  containers:
    - name: docsclaw
      image: ghcr.io/redhat-et/docsclaw:latest
      securityContext:
        runAsNonRoot: true
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
      volumeMounts:
        - name: skill-resume-screener
          mountPath: /skills/resume-screener
  volumes:
    - name: skill-resume-screener
      image:
        reference: quay.io/docsclaw/skill-resume-screener:1.0.0-image
        pullPolicy: IfNotPresent
```

Note the `-image` tag — image volumes require the `--as-image`
format. The default artifact format cannot be mounted by the
kubelet.

For private registries, add `imagePullSecrets`:

```yaml
spec:
  imagePullSecrets:
    - name: skill-registry-creds
```

## Deploy on older clusters with init container

For Kubernetes < 1.33 or OpenShift < 4.20, use an init container
with `skillctl` and a PVC. This works with both artifact and image
formats.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hr-agent
spec:
  initContainers:
    - name: skill-puller
      image: ghcr.io/redhat-et/skillctl:latest
      command: ["skillctl", "pull",
                "-o", "/skills",
                "quay.io/docsclaw/skill-resume-screener:1.0.0"]
      securityContext:
        runAsNonRoot: true
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
      volumeMounts:
        - mountPath: /skills
          name: skills-pvc
  containers:
    - name: docsclaw
      image: ghcr.io/redhat-et/docsclaw:latest
      securityContext:
        runAsNonRoot: true
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
      volumeMounts:
        - mountPath: /skills
          name: skills-pvc
  volumes:
    - name: skills-pvc
      persistentVolumeClaim:
        claimName: skill-cache
```

Use a PVC (not emptyDir) to persist the skill cache across pod
restarts and avoid filling node ephemeral storage.

## Local testing with Zot

[Zot][zot] is a lightweight OCI-native registry ideal for local
development:

```bash
# Run Zot locally
docker run -d -p 5000:5000 ghcr.io/project-zot/zot-linux-amd64:latest

# Push both formats
skillctl push --tls-verify=false \
  examples/skills/resume-screener \
  localhost:5000/skill-resume-screener:1.0.0

skillctl push --as-image --tls-verify=false \
  examples/skills/resume-screener \
  localhost:5000/skill-resume-screener:1.0.0-image

# Test artifact pull with oras
oras pull --plain-http -o resume-screener \
  localhost:5000/skill-resume-screener:1.0.0

# Test image volume mount on OpenShift 4.20+
# (use the -image tag in your pod manifest)
```

## Further reading

- [ADR-0001: Dual OCI format](adr/0001-oci-skill-dual-format.md) —
  why two formats, trade-offs, alternatives considered
- [OCI skill distribution design spec](dev/2026-04-12-oci-skill-distribution-design.md) —
  full design with SkillCard schema, media types, annotations,
  signature verification, and community alignment
- [Agent Skills Specification][agentskills] — upstream skill format
- [Agent Skills OCI Artifacts Specification][vitale-spec] — community
  standard for OCI-based skill distribution

[agentskills]: https://agentskills.io/specification
[vitale-spec]: https://github.com/ThomasVitale/agents-skills-oci-artifacts-spec
[zot]: https://zotregistry.dev/
