## 1. Remove v1 identity (WP03)

- [ ] 1.1 Remove delegation from the bridge: delete `delegation.go` and
  `message.go`, drop document mode and the bearer/`x-delegation-*` reads
  in `executor.go`, and drop `UserSPIFFEID`/`AgentSPIFFEID` from
  `client.go`, with the three executor tests from WP03
- [ ] 1.2 Remove v1 wiring from serve and batch; always set
  `ProcessMessage`; replace the delegation transport with a plain client;
  neutral `fetch_document` description
- [ ] 1.3 Remove SPIFFE/OPA config, metrics, and logger helpers; run the
  acceptance grep

## 2. Egress through the supervisor (WP04)

- [ ] 2.1 Add `internal/egress` `NewClient`/`NewTransport` with proxy,
  dialer, and timeout tests
- [ ] 2.2 Route every sandbox-side client through `internal/egress`
  (`serve`, webfetch, openai, anthropic, mcpclient, skillpuller, rag)
- [ ] 2.3 Add the source-scan guard test, prove it catches a raw
  transport, and document sandbox egress in `deploy/README.md`

## 3. Named document services and precise denials (WP05)

- [ ] 3.1 Add `DescribeResponse`, `DescribeError`, and `LogCall` to
  `internal/egress` with table tests
- [ ] 3.2 Add `document_services` config validation and rewrite
  `fetch_document` to take `service` and `document_id`, with path-escape
  and no-header-log tests
- [ ] 3.3 Wire the tool into `serve`, delete `--document-service-url`,
  and migrate demo, testdata, and seed-script uses

## 4. One-shot `ask` command (WP06)

- [ ] 4.1 Extract `buildRuntime`, `agentRuntime.Run`, and
  `newServeHandler` from `serve`, with fake-provider tests
- [ ] 4.2 Add the `ask` command, shared agent flags, and the `--trace`
  hook, with stdout/stderr and exit-code tests
- [ ] 4.3 Add placeholder-key tests for both providers and document the
  sandbox environment variables in `deploy/README.md`

## 5. OpenShell sandbox image (WP07)

- [ ] 5.1 Add `Containerfile.openshell` and a `make image-openshell`
  target; verify UID, binary path, and `sleep infinity`
- [ ] 5.2 Add the GoReleaser `-openshell` image entries and manifest;
  document the tag, binary path, UID, and workspace mount
