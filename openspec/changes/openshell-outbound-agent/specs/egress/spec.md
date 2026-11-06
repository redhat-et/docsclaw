## ADDED Requirements

### Requirement: Outbound clients are proxy-aware

Every outbound `http.Client` used by the agent SHALL be built from
`internal/egress`. The transport SHALL be a clone of
`http.DefaultTransport` with a non-nil `Proxy` function and MUST NOT set
a custom `DialContext`, `Dial`, or `DialTLSContext`.

#### Scenario: Client honors a configured proxy

- **WHEN** `HTTPS_PROXY` is set and a client is built with
  `egress.NewClient`
- **THEN** the transport's proxy function SHALL return the configured
  proxy for an `https` URL

#### Scenario: Raw transports are rejected

- **WHEN** any non-test Go file outside `internal/egress` contains an
  `http.Transport` composite literal
- **THEN** the source-scan test SHALL fail and name the file and line

### Requirement: Denials are classified

`egress.DescribeResponse` SHALL read at most 4 KiB of a response body
and return `"<status> <reason>"`, preferring the JSON `error` field.
`egress.DescribeError` SHALL return `blocked by sandbox network policy`
for OpenShell transport-level denial shapes and `request failed: <err>`
otherwise.

#### Scenario: JSON error code is surfaced

- **WHEN** the response is `403 {"error":"invalid_audience"}`
- **THEN** `DescribeResponse` SHALL return `403 invalid_audience`

#### Scenario: Oversized body does not hang

- **WHEN** the response body is 10 KiB
- **THEN** `DescribeResponse` SHALL return within its read cap and fall
  back to the status text

#### Scenario: HTTPS policy denial is mapped

- **WHEN** `DescribeError` receives an error containing
  `http: server gave HTTP response to HTTPS client`
- **THEN** it SHALL return `blocked by sandbox network policy`

### Requirement: Outbound calls are logged without secrets

`egress.LogCall` SHALL log `tool`, `host`, `status`, and `reason` at info
level. It MUST NOT log request headers, bearer tokens, API keys, or URL
query strings.

#### Scenario: Log excludes the authorization header

- **WHEN** a tool call carries `Authorization: Bearer leak` and the
  response is `200`
- **THEN** the captured log SHALL contain the host and status but MUST
  NOT contain `leak` or `Authorization`
