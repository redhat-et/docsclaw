## ADDED Requirements

### Requirement: OpenShell-compatible image layout

The published OpenShell image SHALL place the binary at
`/usr/local/bin/docsclaw` with no symlink, run as a non-root user, and
provide a writable workspace at `/sandbox`. The image SHALL include
`bash` and a `sleep` implementation that accepts `infinity`.

#### Scenario: Harness can exec into the image

- **WHEN** the v2 harness starts the image with `-- sleep infinity` and
  execs `docsclaw --version` through the default login shell
- **THEN** the command SHALL run as a non-root UID and print a version

#### Scenario: Provider profile path matches the binary

- **WHEN** an OpenShell provider profile lists `/usr/local/bin/docsclaw`
  in its `binaries`
- **THEN** the binary SHALL be a real file at that path, not a symlink

### Requirement: Release publishes the OpenShell variant

The release workflow SHALL publish a multi-architecture image tagged
`ghcr.io/redhat-et/docsclaw:<version>-openshell`.

#### Scenario: Release builds both architectures

- **WHEN** a version tag is released
- **THEN** the workflow SHALL build the amd64 and arm64 variants and a
  manifest tagged `<version>-openshell`
