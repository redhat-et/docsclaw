## Why

The Zero Trust for Agents v2 demo runs every agent inside an NVIDIA
OpenShell sandbox, where egress is brokered by the supervisor and the
agent never holds credentials. DocsClaw still carries v1 identity code
(SPIFFE, OPA, delegation headers) that is dead in v2 and unsafe in
places: the A2A document mode forwards the caller's raw bearer token to
the document service, which is token passthrough. Phase 1 makes
DocsClaw a well-behaved outbound-only sandbox citizen before Phase 2
adds inbound A2A authentication.

## What Changes

- **BREAKING**: Remove the A2A document mode. `FetchDocument`,
  `ProcessLLM`, and `ExtractDocumentID` are deleted; every A2A message
  goes through `ProcessMessage`.
- **BREAKING**: Remove `--mock-spiffe`, `--opa-host`, `--opa-port`, the
  `spiffe.*` and `opa.*` config keys, `X-Delegation-User` /
  `X-Delegation-Agent` handling, and the SPIFFE/OPA metrics and logger
  helpers.
- **BREAKING**: Remove `--document-service-url` and
  `document_service_url`. Fetching a document now requires `service` and
  `document_id`, resolved through a `document_services` map in
  `agent-config.yaml`.
- Route every tool's outbound HTTP through a new `internal/egress`
  package: a proxy-aware client constructor, denial description, and
  call logging (tool, host, status, denial reason; never headers).
- Report denials precisely: non-2xx document-service responses return
  the HTTP status and JSON `error` field; OpenShell network-policy
  denials are reported as `blocked by sandbox network policy`.
- Add a one-shot `docsclaw ask --config-dir DIR "prompt"` command with
  an optional `--trace`, sharing one extracted runtime setup with
  `serve`.
- Add `Containerfile.openshell` and a GoReleaser image variant
  `ghcr.io/redhat-et/docsclaw:<version>-openshell`: non-root, writable
  `/sandbox`, `bash` and `sleep`, binary at `/usr/local/bin/docsclaw`.

## Capabilities

### New Capabilities

- `fetch-document`: selecting a named document service and reporting its
  allow/deny outcome to the LLM.
- `egress`: proxy-aware outbound HTTP construction, denial classification,
  and outbound call logging.
- `ask-command`: one-shot agent invocation that shares the `serve` runtime.
- `sandbox-image`: OpenShell-compatible container image layout and
  constraints.

### Modified Capabilities

(none -- v1 identity removal and the `fetch_document` parameter change are
captured as new capability requirements, not as changes to an existing
main spec)

## Impact

- `internal/bridge/` -- `executor.go`, `client.go`; delete `delegation.go`
  and `message.go`
- `internal/cmd/` -- `serve.go`, `agentconfig.go`; new `runtime.go`,
  `ask.go`
- `internal/config/config.go`, `internal/metrics/metrics.go`,
  `internal/logger/logger.go` -- remove SPIFFE/OPA wiring
- `internal/fetchdoc/fetchdoc.go` -- `service` + `document_id`
- New `internal/egress/`
- `internal/webfetch`, `internal/openai`, `internal/anthropic`,
  `internal/mcpclient`, `internal/skillpuller` -- use the shared egress
  client
- `Containerfile.openshell`, `Makefile`, `.goreleaser.yaml`,
  `CLAUDE.md`, `deploy/README.md`, `testdata/` and `demo/batch/` configs
- No production consumers; no compatibility shims for v1 settings
