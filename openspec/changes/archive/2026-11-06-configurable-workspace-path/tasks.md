## 1. Core implementation

- [x] 1.1 Add `Workspace` field to `Config` struct in `serve.go`
- [x] 1.2 Add `--workspace` flag to the `serve` command and bind to Viper (`workspace` key)
- [x] 1.3 Create `resolveWorkspace(cfgWorkspace, flagWorkspace string) string` function that returns the first non-empty value, falling back to `defaultWorkspace`
- [x] 1.4 Replace the inline workspace resolution in the `if agentCfg != nil` block with a call to `resolveWorkspace`
- [x] 1.5 Replace the inline workspace resolution in the OpenClaw context block with the same resolved value

## 2. Testing

- [x] 2.1 Add unit test: `resolveWorkspace` returns config value when all three are set
- [x] 2.2 Add unit test: `resolveWorkspace` returns flag value when config is empty
- [x] 2.3 Add unit test: `resolveWorkspace` returns default when both config and flag are empty

## 3. Documentation and deployment

- [x] 3.1 Add commented-out `DOCSCLAW_WORKSPACE` example to `deploy/standalone-agent.yaml` env section
- [x] 3.2 Document `--workspace` flag in `docs/agent-config-guide.md` workspace section
- [x] 3.3 Run `make lint` and `make test` to verify no regressions
