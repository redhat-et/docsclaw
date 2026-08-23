# DocsClaw — Agent Instructions

Compact guidance for OpenCode sessions working in this repo. Verify claims against `Makefile`, `go.mod`, and `internal/cmd/` before relying on them.

## Quick local run

```bash
make build
ANTHROPIC_API_KEY=sk-... ./bin/docsclaw serve \
  --config-dir testdata/standalone \
  --listen-plain-http
```

- `--listen-plain-http` disables TLS/SPIFFE expectations; omit only when SPIRE/SPIFFE is wired.
- Default ports: main `8000`, health/metrics `8100`.
- Main endpoints: `/.well-known/agent-card.json`, `/a2a`, `/v1/chat/completions`, `/v1/models`, `/v1/skills`.
- Health/metrics: `/health`, `/ready`, `/metrics` on the health port.

## Build and verify

```bash
make build              # main binary -> bin/docsclaw
make build-skill-puller # second binary -> bin/skill-puller
make test               # go test ./...
make lint               # golangci-lint run ./... (no config file, defaults)
make fmt                # gofmt -w .
```

- Go 1.26+ is required.
- No hidden test harnesses or integration services are needed for `make test`; it runs pure Go tests.

## Project layout

| Path | What lives here |
| ---- | --------------- |
| `cmd/docsclaw/main.go` | Entrypoint; blank-imports `internal/anthropic` and `internal/openai` to trigger provider registration via `init()`. |
| `cmd/skill-puller/main.go` | Sidecar entrypoint for pulling OCI skills. |
| `internal/cmd/` | Cobra commands (`serve`, `batch`, `build`, `deploy`, `skill`). |
| `internal/bridge/` | A2A protocol bridge: executor, client, delegation, dynamic/signed agent cards. |
| `internal/openaiapi/` | OpenAI-compatible `/v1/*` handlers. |
| `internal/telemetry/` | OpenTelemetry tracing setup. |
| `pkg/llm/` | Provider interface, factory, and registration pattern. |
| `pkg/tools/` | Tool interface, registry, and agentic loop. |
| `pkg/skills/` | Skill discovery and loading. |
| `pkg/manifest/` | Agent manifest parsing, Containerfile/K8s generation, risk scoring. |
| `testdata/` | Agent configs and prompts used by tests and examples. |

## Agent modes

- **Phase 1 (single-shot):** only `system-prompt.txt` required. No tools.
- **Phase 2 (agentic loop):** add `agent-config.yaml` to enable tools and `tools.RunToolLoop`.

Key config files in the config directory:

- `system-prompt.txt` — required.
- `agent-card.json` — optional; fallback card is generated.
- `agent-config.yaml` — optional; enables phase 2.
- `prompts.json` — optional keyword-to-prompt variants.
- `skills/<name>/SKILL.md` — optional on-demand skills.

## Container images

```bash
make image              # dev image from Dockerfile (Alpine)
make image-push         # push to ghcr.io/redhat-et/docsclaw
make image-security     # hardened image from containers/Containerfile.security
```

- Default engine is `podman`; override with `CONTAINER_ENGINE=docker`.
- Default tag is the short Git SHA; override with `DEV_TAG` or `REGISTRY`.
- Release builds use `Dockerfile.release` (Red Hat `core-runtime` base) via GoReleaser.

## Custom agent manifests

```bash
# Validate only
docsclaw build --manifest agent-manifest.yaml --dry-run

# Build and push via Makefile
make agent-build MANIFEST=examples/.../agent.yaml
make agent-image MANIFEST=... TAG=ghcr.io/org/agent:1.0.0
make agent-push MANIFEST=... TAG=ghcr.io/org/agent:1.0.0

# Generate K8s manifests with secrets resolved
docsclaw deploy --manifest agent-manifest.yaml --secret API_KEY=... | oc apply -f -
```

- `docsclaw build` validates tools against the embedded catalog, computes a risk score/tier, and emits `Containerfile` + `tools.json` (+ K8s if requested).
- `make agent-build` writes generated files to `build/agent/` and copies `Containerfile.agent` + `tools.json` to the repo root for the image build.

## ConfigMap generation for OpenShift

```bash
make configmap-gen CONFIG_DIR=testdata/standalone
make configmap-apply CONFIG_DIR=testdata/standalone
```

- Defaults to `oc`; override with `KUBECTL=kubectl`.
- Generates one ConfigMap for agent config files and one per skill subdirectory containing `SKILL.md`.

## Release

GoReleaser runs on pushes to `v*` tags:

```bash
git tag v0.1.0
git push --tags
```

- Requires the repo secret `HOMEBREW_TAP_TOKEN` for the Homebrew tap.
- Produces cross-platform binaries, multi-arch images (`ghcr.io/redhat-et/docsclaw:<version>`), and a Homebrew formula.

## Workspace context

When a workspace directory (default `/workspace`) contains Markdown files, they are appended to the system prompt as `## Project Context`. Use `--workspace-profile` to select which files are loaded and in what order:

| Profile | Files loaded |
| ------- | ------------ |
| `docsclaw` (default) | `AGENTS.md`, `SOUL.md`, `USER.md`, `IDENTITY.md`, `TOOLS.md` |
| `openclaw` | `AGENTS.md`, `SOUL.md`, `USER.md`, `MEMORY.md`, `IDENTITY.md`, `TOOLS.md` |
| `hermes` | `AGENTS.md`, `SOUL.md`, `USER.md`, `MEMORY.md` |

- `MEMORY.md` is loaded only by the `openclaw` and `hermes` profiles.
- In Phase 2 mode, the `remember` tool appends entries to `MEMORY.md`, giving the agent file-backed long-term memory that survives restarts.
- Each file is capped at 20,000 characters; total context is capped at 60,000 characters.
- Do not confuse `/workspace/AGENTS.md` with this repo-level `AGENTS.md`.

## Environment variables agents should know

| Variable | Purpose |
| -------- | ------- |
| `LLM_API_KEY` / `ANTHROPIC_API_KEY` | LLM API key (fallback chain). |
| `LLM_PROVIDER` | `anthropic` (default), `openai`, `litellm`. |
| `LLM_MODEL` | Model override; default is `claude-sonnet-4-6`. |
| `LLM_BASE_URL` | Base URL for OpenAI-compatible endpoints. |
| `DOCSCLAW_SESSION_DB` | `memory` (default) or SQLite file path. |
| `AGENT_CARD_SIGNED_PATH` | Optional path to a pre-signed agent card. |

## Site / docs

- `site/` deploys to GitHub Pages on every push to `main` that changes `site/**` or `.github/workflows/pages.yaml`.
- See `docs/` for longer guides (agent config, manifests, OCI skills, etc.).

## Common mistakes to avoid

- Adding a new LLM provider: register it in `pkg/llm/factory.go` and blank-import it in `cmd/docsclaw/main.go`.
- Adding a new tool: implement the `tools.Tool` interface and register it in `internal/cmd/serve.go` inside the `toolRegistry != nil` block.
- Running `make lint` without `golangci-lint` installed; there is no repo-local linter config.
- Forgetting `--listen-plain-http` in local dev and wondering why TLS/SPIFFE fails.
