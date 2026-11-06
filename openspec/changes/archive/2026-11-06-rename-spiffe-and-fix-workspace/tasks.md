## 1. Rename SPIFFE_DEMO references

- [x] 1.1 Change env prefix from `SPIFFE_DEMO` to `DOCSCLAW` in `internal/config/config.go`
- [x] 1.2 Change config path from `/etc/spiffe-demo/` to `/etc/docsclaw/` in `internal/config/config.go`
- [x] 1.3 Rename `SPIFFE_DEMO_LOG_FORMAT` to `DOCSCLAW_LOG_FORMAT` in `internal/logger/logger.go` (env var and comment)
- [x] 1.4 Rename Prometheus namespace from `spiffe_demo` to `docsclaw` in `internal/metrics/metrics.go` (7 metrics) and update package doc comment
- [x] 1.5 Update `SPIFFE_DEMO_SERVICE_PORT` to `DOCSCLAW_SERVICE_PORT` in `deploy/agent-with-skills.yaml` and `deploy/standalone-agent.yaml`

## 2. Fix workspace default and prompt injection

- [x] 2.1 Change default workspace from `/tmp/agent-workspace` to `/workspace` in `internal/cmd/serve.go`
- [x] 2.2 Inject workspace path into system prompt after loading system-prompt.txt in `internal/cmd/serve.go`
- [x] 2.3 Add emptyDir volume mount at `/workspace` in deployment manifests

## 3. Verify and clean up

- [x] 3.1 Grep project-wide for remaining `spiffe` or `SPIFFE` references and fix any found
- [x] 3.2 Run `make lint` and `make test` to verify no regressions
- [x] 3.3 Update CLAUDE.md if any documented commands or env vars changed
