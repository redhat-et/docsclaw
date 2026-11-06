# WP07: OpenShell sandbox image implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` and `openshell-notes.md` (Task 3 answers) first.

**Goal:** Publish `ghcr.io/redhat-et/docsclaw:<version>-openshell`, a small
non-root image with `/usr/local/bin/docsclaw` and `sleep`, so the v2 harness
can start it with `-- sleep infinity` and `exec` into it.

**Architecture:** A new `Containerfile.openshell` consumes the
GoReleaser-built binary (like `Dockerfile.release`) on a UBI minimal or
micro base. `.goreleaser.yaml` gains amd64/arm64 image entries and a
manifest.

**Tech stack:** Podman/Buildx, GoReleaser, UBI 9/10 minimal or micro.

**Spec:** parent plan section 1.7.

## Review focus

- The binary must sit at exactly `/usr/local/bin/docsclaw`, with no
  symlink, because provider profiles bind credentials to the path.
- `readOnlyRootFilesystem`: the default workspace must be writable. Use the
  path `serve` defaults to, and document how to mount it.
- `sleep infinity` must work. BusyBox `sleep` may reject `infinity`.

---

### Task 1: Containerfile

**Files:**

- Create: `Containerfile.openshell`
- Modify: `Makefile` (target `image-openshell` for local builds)

- [ ] **Step 1:** Choose the base from the WP01 requirements:
  `registry.access.redhat.com/ubi10/ubi-micro` if it ships coreutils
  `sleep` and `/bin/sh`, otherwise `ubi10/ubi-minimal`. Pin the digest.
  Record the choice and the measured image size in the PR description.
- [ ] **Step 2:** Write the file: copy the pre-built `docsclaw` to
  `/usr/local/bin/docsclaw` (mode 0755, owned by root), create the
  workspace directory owned by UID 1001, set `USER 1001`, and set the
  default `CMD ["sleep","infinity"]`. Add OCI labels (`source`,
  `description`, `licenses`).
- [ ] **Step 3:** Add a `make image-openshell` target. It builds the
  linux binary (`GOOS=linux CGO_ENABLED=0`) into the build context, then
  runs `podman build -f Containerfile.openshell`.
- [ ] **Step 4: Verify locally:**

```bash
make image-openshell
podman run -d --name dc-os docsclaw:dev-openshell
podman exec dc-os sh -c 'id -u; which docsclaw; readlink -f /usr/local/bin/docsclaw; docsclaw --version'
podman exec dc-os docsclaw ask --config-dir /nonexistent x; echo "exit=$?"
podman rm -f dc-os
```

  Expected: `1001`, `/usr/local/bin/docsclaw` twice, a version string, and
  a non-zero exit with a clear config-dir error.

- [ ] **Step 5:** Commit: `feat(image): add OpenShell sandbox Containerfile`.

### Task 2: Release workflow

**Files:**

- Modify: `.goreleaser.yaml`, `CLAUDE.md` (Release section: three
  Dockerfiles), `deploy/README.md`

- [ ] **Step 1:** Add two `dockers` entries using `Containerfile.openshell`
  with tags `{{ .Version }}-openshell-amd64` and `-openshell-arm64`, plus a
  `docker_manifests` entry `ghcr.io/redhat-et/docsclaw:{{ .Version }}-openshell`.
- [ ] **Step 2:** Run `goreleaser check`, then
  `goreleaser release --snapshot --clean --skip=publish`. Expected: both
  openshell images build.
- [ ] **Step 3:** Document the image tag, binary path, UID, and workspace
  mount in `deploy/README.md`. Add the new Containerfile to the
  "Dockerfiles" list in `CLAUDE.md`.
- [ ] **Step 4:** Commit: `ci(release): publish -openshell image variant`.
