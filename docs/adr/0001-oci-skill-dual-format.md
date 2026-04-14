# ADR-0001: Dual OCI format for skill distribution

## Status

Accepted

## Date

2026-04-14

## Context

DocsClaw distributes skills as OCI artifacts via registries like
quay.io. Two distinct audiences consume these artifacts:

**Individual users** use personal agents (Claude Code, OpenCode,
Cursor) on their workstations. They want to browse a registry,
pull a skill, and have it ready to use with standard tools — no
DocsClaw installation required. The `oras` CLI is the natural tool
for this audience.

**Platform operators** deploy agents on Kubernetes/OpenShift. They
want skills mounted into pods without init containers or extra
storage. OpenShift 4.20+ supports image volumes — OCI images
mounted directly as read-only pod volumes by the kubelet.

These two use cases have conflicting requirements:

- `oras pull` works best with **individual file layers** — each
  file becomes a separate layer with a title annotation, and
  `oras pull` extracts them directly as named files.
- Kubernetes image volumes require **OCI images** — a single
  tar+gzip layer with a valid image config including
  `rootfs.diff_ids`. CRI-O cannot mount pure OCI artifacts.

A single tarball format serves neither audience well: `oras pull`
gives users a tarball they must manually extract, and a pure
artifact with individual layers cannot be mounted by the kubelet.

## Decision

Support two OCI formats via the `--as-image` flag on
`docsclaw skill push`:

### Artifact mode (default)

Each file in the skill directory is pushed as a separate layer
with an `org.opencontainers.image.title` annotation. The config
blob uses the community `application/vnd.agentskills.skill.config.v1+json`
media type with skill metadata.

```text
manifest
├── config: agentskills.skill.config.v1+json  → config.json
└── layers:
    ├── SKILL.md        (individual file)
    ├── skill.yaml      (individual file)
    ├── scripts/foo.sh  (individual file, if present)
    └── ...
```

Pull workflow (no DocsClaw needed):

```bash
oras pull -o resume-screener quay.io/docsclaw/skill-resume-screener:1.0.0
ls resume-screener/
# SKILL.md  skill.yaml  config.json
```

### Image mode (`--as-image`)

The entire skill directory is packed as a single tar+gzip layer
with a valid OCI image config. This produces a kubelet-mountable
image.

```text
manifest
├── config: oci.image.config.v1+json  → {rootfs, diff_ids}
└── layers:
    └── application/vnd.oci.image.layer.v1.tar+gzip  → all files
```

Mount workflow:

```yaml
volumes:
  - name: skill
    image:
      reference: quay.io/docsclaw/skill-resume-screener:1.0.0-image
```

### How `docsclaw skill pull` handles both

The `pull` command detects the format by inspecting layer media
types. For artifacts with individual file layers, it downloads
each layer and writes the file to the destination. For image-mode
tarballs, it extracts the tarball. Both produce the same result:
a skill directory with `SKILL.md` and `skill.yaml`.

## Alternatives considered

### Single tarball for both modes

Use a tar+gzip layer for artifact mode too (with different config
media type). This was the initial implementation.

Rejected because `oras pull` gives users a tarball they must
manually extract. Skills are small text files (typically under
10KB) — the tar overhead is unnecessary and the UX is poor.

### Single file-per-layer for both modes

Push individual files in both modes.

Rejected because Kubernetes image volumes (CRI-O) cannot mount
OCI artifacts with custom layer media types. The kubelet requires
a valid image with standard `application/vnd.oci.image.layer.v1.tar+gzip`
layers and an image config with `rootfs.diff_ids`.

### Different tags for different formats

Push both formats under the same repository with different tags
(e.g., `:1.0.0` for artifact, `:1.0.0-image` for image).

This is supported but not required. Users can push the same skill
with both flags to the same repository. The tag convention is a
user choice, not enforced by the tool.

## Consequences

- Two code paths in `Pack()` — one for file-per-layer, one for
  tarball. Maintained but separate.
- `Pull()` must handle both formats — detect by layer media type.
- `Inspect()` works the same for both — reads config blob
  annotations and metadata.
- Users without DocsClaw can consume skills via `oras pull`.
- Platform operators get kubelet-mountable images via `--as-image`.
- The community spec uses a single tarball; our artifact mode
  diverges here for better UX but the skill directory structure
  on disk is identical after extraction.
