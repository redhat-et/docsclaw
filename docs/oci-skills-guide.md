# OCI skill distribution guide

Package, sign, and distribute skills as OCI artifacts. Skills can
be pushed to any OCI-compliant registry (quay.io, GHCR, Harbor, Zot)
and mounted into agent pods as image volumes on OpenShift 4.20+ or
pulled via init containers on older clusters.

## Prerequisites

- DocsClaw binary (`make build`)
- An OCI-compliant registry (Zot recommended for local testing)
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

## CLI commands

### Pack a skill

Package a skill directory into a local OCI layout:

```bash
docsclaw skill pack examples/skills/resume-screener

# Output:
# Packed skill to examples/skills/resume-screener/oci-layout
# Digest: sha256:65af81ce...
# Size: 1226 bytes
```

Use `--as-image` to produce a kubelet-mountable image (required
for image volumes on OpenShift 4.20+):

```bash
docsclaw skill pack --as-image examples/skills/resume-screener
```

Use `-o` to specify the output directory:

```bash
docsclaw skill pack -o /tmp/my-layout examples/skills/resume-screener
```

### Push a skill to a registry

Pack and push in one step:

```bash
docsclaw skill push examples/skills/resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

Push as a mountable image:

```bash
docsclaw skill push --as-image examples/skills/resume-screener \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

### Pull a skill from a registry

```bash
docsclaw skill pull quay.io/docsclaw/skill-resume-screener:1.0.0
```

Skills are extracted to `~/.docsclaw/skills/` by default. Use `-o`
to specify a different directory:

```bash
docsclaw skill pull -o /skills \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

Pull with signature verification:

```bash
docsclaw skill pull --verify --key cosign.pub \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

### Inspect a skill

Show SkillCard metadata without pulling the full content:

```bash
docsclaw skill inspect \
  quay.io/docsclaw/skill-resume-screener:1.0.0

# Output:
# Name:        resume-screener
# Namespace:   official
# Version:     1.0.0
# Description: Screen resumes against a job description...
# Author:      Red Hat ET
# License:     Apache-2.0
# Tools:       [read_file]
# Memory:      32Mi
# CPU:         100m
```

### Verify a skill signature

```bash
docsclaw skill verify --key cosign.pub \
  quay.io/docsclaw/skill-resume-screener:1.0.0
```

## Deploy on OpenShift 4.20+ with image volumes

Skills pushed with `--as-image` can be mounted directly as pod
volumes — no init container needed:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hr-agent
spec:
  containers:
    - name: docsclaw
      image: ghcr.io/redhat-et/docsclaw:latest
      volumeMounts:
        - name: skill-resume-screener
          mountPath: /skills/resume-screener
        - name: skill-policy-comparator
          mountPath: /skills/policy-comparator
  volumes:
    - name: skill-resume-screener
      image:
        reference: quay.io/docsclaw/skill-resume-screener:1.0.0
        pullPolicy: IfNotPresent
    - name: skill-policy-comparator
      image:
        reference: quay.io/docsclaw/skill-policy-comparator:1.0.0
        pullPolicy: IfNotPresent
```

The kubelet pulls and caches the skill images using the container
runtime's existing image store. No emptyDir, no node ephemeral
storage consumed.

For private registries, add `imagePullSecrets`:

```yaml
spec:
  imagePullSecrets:
    - name: skill-registry-creds
```

## Deploy on older clusters with init container

For Kubernetes < 1.33 or OpenShift < 4.20, use an init container
with a PVC:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hr-agent
spec:
  initContainers:
    - name: skill-puller
      image: ghcr.io/redhat-et/docsclaw:latest
      command: ["docsclaw", "skill", "pull", "--verify",
                "--key", "/etc/docsclaw/keys/cosign.pub",
                "-o", "/skills",
                "quay.io/docsclaw/skill-resume-screener:1.0.0"]
      volumeMounts:
        - mountPath: /skills
          name: skills-pvc
        - mountPath: /etc/docsclaw/keys
          name: signing-keys
  containers:
    - name: docsclaw
      image: ghcr.io/redhat-et/docsclaw:latest
      volumeMounts:
        - mountPath: /skills
          name: skills-pvc
  volumes:
    - name: skills-pvc
      persistentVolumeClaim:
        claimName: skill-cache
    - name: signing-keys
      secret:
        secretName: docsclaw-signing-keys
```

Use a PVC (not emptyDir) to persist the skill cache across pod
restarts and avoid filling node ephemeral storage.

## Local testing with Zot

[Zot][zot] is a lightweight OCI-native registry ideal for local
development:

```bash
# Run Zot locally
docker run -d -p 5000:5000 ghcr.io/project-zot/zot-linux-amd64:latest

# Push a skill
docsclaw skill push --as-image examples/skills/resume-screener \
  localhost:5000/skills/resume-screener:1.0.0

# Inspect it
docsclaw skill inspect localhost:5000/skills/resume-screener:1.0.0

# Pull it
docsclaw skill pull -o /tmp/skills \
  localhost:5000/skills/resume-screener:1.0.0
```

## Further reading

- [OCI skill distribution design spec](dev/2026-04-12-oci-skill-distribution-design.md) —
  full design with SkillCard schema, media types, annotations,
  signature verification, and community alignment
- [Agent Skills Specification][agentskills] — upstream skill format
- [Agent Skills OCI Artifacts Specification][vitale-spec] — community
  standard for OCI-based skill distribution

[agentskills]: https://agentskills.io/specification
[vitale-spec]: https://github.com/ThomasVitale/agents-skills-oci-artifacts-spec
[zot]: https://zotregistry.dev/
