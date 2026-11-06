## ADDED Requirements

### Requirement: Workspace path injected into system prompt

The system SHALL append the configured workspace directory path to the
system prompt so that the LLM knows where to read and write files.

#### Scenario: Workspace path appears in system prompt

- **WHEN** the agent starts with workspace configured to `/workspace`
- **THEN** the system prompt SHALL contain text directing the LLM to
  use `/workspace` for all file operations

#### Scenario: Default workspace is container-safe

- **WHEN** the agent starts without an explicit workspace in
  `agent-config.yaml`
- **THEN** the default workspace SHALL be `/workspace` (not `/tmp`)

### Requirement: DOCSCLAW env prefix replaces SPIFFE_DEMO

The system SHALL use `DOCSCLAW` as the Viper env var prefix for all
configuration. The legacy `SPIFFE_DEMO` prefix SHALL no longer be
recognized.

#### Scenario: New env prefix is used

- **WHEN** a user sets `DOCSCLAW_OTEL_ENABLED=true`
- **THEN** the agent SHALL read the value and enable OTel

#### Scenario: Old env prefix is ignored

- **WHEN** a user sets `SPIFFE_DEMO_OTEL_ENABLED=true` without the
  corresponding `DOCSCLAW_*` var
- **THEN** the agent SHALL NOT read the value (breaking change)

### Requirement: Prometheus namespace uses docsclaw

All Prometheus metrics SHALL use the namespace `docsclaw` instead
of `spiffe_demo`.

#### Scenario: Metrics use new namespace

- **WHEN** Prometheus scrapes the `/metrics` endpoint
- **THEN** all metric names SHALL start with `docsclaw_`
  (e.g. `docsclaw_http_requests_total`)

### Requirement: Config file path uses docsclaw

The system SHALL search for config files in `/etc/docsclaw/` instead
of `/etc/spiffe-demo/`.

#### Scenario: Config loaded from new path

- **WHEN** a config file exists at `/etc/docsclaw/config.yaml`
- **THEN** the agent SHALL load it

### Requirement: Logger env var uses DOCSCLAW prefix

The legacy log format env var SHALL be renamed from
`SPIFFE_DEMO_LOG_FORMAT` to `DOCSCLAW_LOG_FORMAT`.

#### Scenario: New logger env var controls format

- **WHEN** `DOCSCLAW_LOG_FORMAT=json` is set
- **THEN** the logger SHALL output JSON format
