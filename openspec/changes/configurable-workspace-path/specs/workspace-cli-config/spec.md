## ADDED Requirements

### Requirement: Workspace path configurable via CLI flag

The `serve` command SHALL accept a `--workspace` flag that sets
the agent workspace directory path.

#### Scenario: Flag sets workspace when no config file value

- **WHEN** `docsclaw serve --workspace /data/agent` is run and
  `agent-config.yaml` has no `tools.workspace` field
- **THEN** the workspace directory SHALL be `/data/agent`

#### Scenario: Config file takes precedence over flag

- **WHEN** `docsclaw serve --workspace /data/agent` is run and
  `agent-config.yaml` has `tools.workspace: /custom`
- **THEN** the workspace directory SHALL be `/custom`

### Requirement: Workspace path configurable via env var

The `serve` command SHALL accept a `DOCSCLAW_WORKSPACE` env var
that sets the workspace directory path, with the same precedence
as the `--workspace` flag.

#### Scenario: Env var sets workspace

- **WHEN** `DOCSCLAW_WORKSPACE=/data/agent docsclaw serve` is run
  and no `--workspace` flag is set and `agent-config.yaml` has no
  `tools.workspace` field
- **THEN** the workspace directory SHALL be `/data/agent`

#### Scenario: Flag and env var both set

- **WHEN** `DOCSCLAW_WORKSPACE=/env/path docsclaw serve --workspace /flag/path`
  is run
- **THEN** the workspace directory SHALL be `/flag/path`
  (explicit flag wins over env var, standard Viper behavior)

### Requirement: Default workspace unchanged

When neither the flag, env var, nor config file sets a workspace,
the system SHALL use `/workspace` as the default.

#### Scenario: No workspace configuration

- **WHEN** `docsclaw serve` is run with no `--workspace` flag, no
  `DOCSCLAW_WORKSPACE` env var, and no `tools.workspace` in
  `agent-config.yaml`
- **THEN** the workspace directory SHALL be `/workspace`

### Requirement: Workspace used consistently

The resolved workspace path SHALL be used for all workspace
purposes: tool registration (`read_file`, `write_file`), OpenClaw
context loading, system prompt injection, and directory creation.

#### Scenario: All consumers use same path

- **WHEN** the workspace is resolved to `/data/agent`
- **THEN** `read_file` and `write_file` tools SHALL use
  `/data/agent` as their root, OpenClaw files SHALL be loaded from
  `/data/agent`, and the system prompt SHALL reference
  `/data/agent`
