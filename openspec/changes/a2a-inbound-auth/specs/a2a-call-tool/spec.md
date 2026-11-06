## ADDED Requirements

### Requirement: Peers are configured by name

The agent configuration SHALL accept an `a2a_peers` list, each entry with a
unique `name`, an absolute `url`, and an optional `description`. The
`a2a_call` tool SHALL expose `peer` as an enum of the configured names.

#### Scenario: Unknown peer is rejected

- **WHEN** the LLM calls `a2a_call` with a peer name that is not configured
- **THEN** the tool SHALL return an error naming the unknown peer and the call
  SHALL NOT be made

### Requirement: The agent sends no credential

`a2a_call` MUST NOT set an `Authorization` header. The OpenShell supervisor
injects the outbound token at the network boundary.

#### Scenario: Outbound request has no authorization header

- **WHEN** `a2a_call` invokes a configured peer
- **THEN** the request observed by the peer SHALL contain no `Authorization`
  header contributed by the agent

### Requirement: Results and task states are surfaced

The tool SHALL return the peer's text and final task state to the LLM.
`input-required` and `auth-required` SHALL be treated as terminal states and
reported as such.

#### Scenario: Completed call returns text and state

- **WHEN** the peer completes the task
- **THEN** the tool result SHALL contain the peer's text and the terminal state

#### Scenario: Input-required is reported

- **WHEN** the peer returns `input-required`
- **THEN** the tool result SHALL report that state to the LLM as terminal

### Requirement: Auth denials keep their error code

The tool SHALL surface `401` and `403` peer responses with the peer's `error`
code, using the same denial vocabulary as outbound document fetches.

#### Scenario: Insufficient scope is surfaced

- **WHEN** the peer returns `403 {"error":"insufficient_scope"}`
- **THEN** the tool result SHALL contain `403 insufficient_scope`

### Requirement: Registration is gated by the tool allowlist

The runtime SHALL register `a2a_call` only when `a2a_call` appears in
`tools.allowed`.

#### Scenario: Tool is absent when not allowed

- **WHEN** `tools.allowed` does not list `a2a_call`
- **THEN** the LLM tool definitions SHALL NOT contain `a2a_call`

### Requirement: Peer URLs come from the supervisor

The `url` value SHALL be the service URL printed by
`openshell service list --output json`. The agent MUST NOT compose it from
workspace, sandbox, and service names.

#### Scenario: Configured URL is used verbatim

- **WHEN** `a2a_peers[].url` is set
- **THEN** the request SHALL target that exact URL
