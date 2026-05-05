# CLAUDE.md - DocsClaw Project Guide

## Overview

DocsClaw is a universal AI agent runtime with ConfigMap-driven
personality. It provides an A2A-compatible agent that can operate in
two modes: single-shot LLM processing (phase 1) and agentic tool-use
loops (phase 2).

## Build and test

```bash
make build       # Build binary to bin/docsclaw
make test        # Run all tests
make lint        # Run golangci-lint
make fmt         # Format code
make image       # Build container image (local dev)
make image-push  # Build and push to GHCR
```

## Release

Releases use [GoReleaser](https://goreleaser.com/) triggered by
version tags (`v*`). Config in `.goreleaser.yaml`, workflow in
`.github/workflows/release.yaml`.

```bash
git tag v0.1.0 && git push --tags
```

Produces binaries (linux/darwin/windows, amd64/arm64), multi-arch
container images on `ghcr.io/redhat-et/docsclaw`, and a Homebrew
formula. Version is injected via ldflags into
`internal/cmd.version`.

Two Dockerfiles:
- `Dockerfile` — local dev builds (multi-stage with Go builder)
- `Dockerfile.release` — GoReleaser builds (scratch, pre-built binary)

Required repo secret: `HOMEBREW_TAP_TOKEN` (write access to
`pavelanni/homebrew-tap`).

## Project structure

| Path | Description |
| ---- | ----------- |
| `cmd/docsclaw/` | Main entrypoint; registers LLM providers |
| `internal/cmd/` | Cobra commands (root, serve, chat, agentconfig) |
| `internal/chat/` | Bubble Tea interactive chat TUI |
| `internal/anthropic/` | Anthropic LLM provider (auto-registers via init) |
| `internal/openai/` | OpenAI-compatible provider (auto-registers via init) |
| `internal/exec/` | Shell command execution tool |
| `internal/webfetch/` | HTTP GET tool |
| `internal/readfile/` | File reading tool |
| `internal/writefile/` | File writing tool |
| `internal/fetchdoc/` | Document service fetch tool |
| `internal/workspace/` | Workspace path validation |
| `internal/bridge/` | A2A protocol bridge (executor, client, delegation) |
| `internal/session/` | In-memory session store for multi-turn conversations |
| `internal/config/` | Viper configuration |
| `internal/logger/` | Color-coded slog logger |
| `internal/metrics/` | Prometheus metrics |
| `pkg/llm/` | LLM provider interface, types, config, factory |
| `pkg/tools/` | Tool interface, registry, agentic loop |
| `pkg/skills/` | Skill discovery and loading |
| `testdata/` | Test fixtures (agent configs, prompts) |

## Architecture

LLM providers register themselves via `init()` functions using a
registration pattern in `pkg/llm/factory.go`. The main entrypoint
imports both provider packages as blank imports to trigger registration.

Tools are organized as separate internal packages, each exporting a
constructor that returns `tools.Tool`. The tool registry in `pkg/tools/`
manages allowed tools and generates LLM tool definitions.

## Configuration

The agent reads personality from a config directory:
- `system-prompt.txt` (required)
- `agent-card.json` (optional, fallback provided)
- `prompts.json` (optional prompt variants)
- `agent-config.yaml` (optional, enables phase 2 tool-use mode)

## Key technologies

- **Go 1.25+**
- **Cobra/Viper** for CLI and configuration
- **A2A (a2a-go)** for agent protocol
- **log/slog** for structured logging
- **Prometheus** for metrics
- **GoReleaser** for automated releases
