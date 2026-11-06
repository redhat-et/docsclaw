## ADDED Requirements

### Requirement: Document fetch selects a named service

The `fetch_document` tool SHALL require a `service` argument and a
`document_id` argument. `service` SHALL be an enum of the configured
`document_services` keys. An unknown service name SHALL return an error
that lists the available services and SHALL NOT perform network I/O.

#### Scenario: Known service fetches the document

- **WHEN** the LLM calls `fetch_document` with `service: engineering` and
  `document_id: DOC-001`
- **THEN** the agent SHALL GET
  `<engineering-base>/documents/DOC-001` and return the document title
  and content

#### Scenario: Unknown service is rejected before the network

- **WHEN** the LLM calls `fetch_document` with a service not present in
  `document_services`
- **THEN** the tool SHALL return an error naming the available services,
  and no HTTP request SHALL be made

#### Scenario: Document ID cannot change the request path

- **WHEN** `document_id` is `../x?y`
- **THEN** the request path SHALL be `/documents/..%2Fx%3Fy`

### Requirement: Document fetch reports denials precisely

On a non-2xx response the tool SHALL return `"<id>: <status> <reason>"`,
where `<reason>` is the JSON `error` field when present and
`http.StatusText` otherwise. The tool MUST NOT return an empty or
generic "access denied" message, and MUST NOT include request headers in
its result.

#### Scenario: Service denies with a reason code

- **WHEN** the document service returns `403`
  `{"error":"invalid_audience"}`
- **THEN** the tool result SHALL contain `403 invalid_audience`

#### Scenario: Service returns a non-JSON error

- **WHEN** the document service returns a `502` with an HTML body
- **THEN** the tool result SHALL contain `502 Bad Gateway`

#### Scenario: Sandbox policy blocks the request

- **WHEN** the request is blocked by OpenShell network policy
- **THEN** the tool result SHALL contain
  `blocked by sandbox network policy`

### Requirement: `document_services` config is validated

`agent-config.yaml` SHALL accept a `document_services` map whose keys
match `^[a-z0-9][a-z0-9-]*$` and whose values are absolute `http` or
`https` URLs. Loading a config that violates either rule SHALL fail with
an error that names the offending key or value.

#### Scenario: Valid map loads

- **WHEN** `document_services` maps `engineering` to an absolute HTTP URL
- **THEN** the agent SHALL load the config and register `fetch_document`

#### Scenario: Invalid service key fails to load

- **WHEN** a `document_services` key is `Eng`
- **THEN** config loading SHALL fail with an error naming `Eng`
