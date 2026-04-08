# DocsClaw

A ConfigMap-driven agentic runtime that turns any LLM into a
tool-using, [A2A](https://google.github.io/A2A/)-compatible agent.
Deploy with a system prompt, agent card, and tool config — no code
changes needed.

DocsClaw provides:

- **Agentic tool-use loop** — the LLM iteratively calls tools until
  the task is complete
- **Built-in tools** — exec, web_fetch, read_file, write_file
- **SKILL.md loading** — teach the agent new behaviors via markdown
- **A2A protocol** — standard agent-to-agent communication
- **Zero-trust ready** — designed for SPIFFE/OPA architectures

## Quick start

```bash
# Build
make build

# Run (requires an Anthropic API key)
ANTHROPIC_API_KEY=sk-... ./bin/docsclaw serve \
  --config-dir testdata/standalone \
  --listen-plain-http
```

Test it:

```bash
curl -X POST http://localhost:8000/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "1",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "user",
        "parts": [
          {"kind": "text", "text": "Run uname -a and describe this system"}
        ]
      }
    }
  }'
```

## How it works

DocsClaw reads its personality from a config directory:

```
my-agent/
├── system-prompt.txt       # Who the agent is
├── agent-card.json         # A2A metadata (name, skills, URL)
├── agent-config.yaml       # Which tools to allow, loop settings
└── skills/                 # Optional SKILL.md instructions
    ├── url-summary/
    │   └── SKILL.md
    └── code-review/
        └── SKILL.md
```

Same binary, different config = different agent. No code changes.

Without `agent-config.yaml`, DocsClaw runs in single-shot mode
(no tools, just prompt-in/response-out). With it, the agentic
loop is enabled and the LLM can call tools iteratively.

## Scenarios

### 1. Standalone local agent

The simplest setup — run locally, send tasks via curl.

```bash
ANTHROPIC_API_KEY=sk-... ./bin/docsclaw serve \
  --config-dir testdata/standalone \
  --listen-plain-http
```

Summarize a web page:

```bash
curl -X POST http://localhost:8000/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "1",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "user",
        "parts": [{
          "kind": "text",
          "text": "Fetch https://go.dev/blog/go1.24 and summarize the key changes"
        }]
      }
    }
  }'
```

The agent will use `web_fetch` to retrieve the page, then
summarize it.

### 2. Deploy on OpenShift/Kubernetes

Build and push the image, then deploy with a ConfigMap:

```bash
# Build and push
make image-push

# Create namespace and API key secret
oc new-project docsclaw
oc create secret generic llm-secret \
  --from-literal=LLM_API_KEY=sk-...

# Apply manifests
oc apply -f deploy/standalone-agent.yaml
```

The manifest bundles ConfigMap + Deployment + Service + Route.
See `deploy/standalone-agent.yaml` for the full example.

Test via the Route:

```bash
curl -X POST \
  https://$(oc get route docsclaw -o jsonpath='{.spec.host}')/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "1",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "user",
        "parts": [{
          "kind": "text",
          "text": "Run date and tell me the current time"
        }]
      }
    }
  }'
```

### 3. Agent with skills

Skills are markdown files that teach the agent specialized
behaviors. The agent discovers them at startup and can load them
on demand via the `load_skill` tool.

The `testdata/standalone/skills/` directory includes two example
skills:

**url-summary** — fetch a URL and produce a structured summary:

```bash
curl -X POST http://localhost:8000/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "1",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "user",
        "parts": [{
          "kind": "text",
          "text": "Use the url-summary skill to summarize https://kubernetes.io/blog/2025/01/13/kubernetes-1.32-release-interview/"
        }]
      }
    }
  }'
```

**code-review** — review a file for bugs and style:

```bash
curl -X POST http://localhost:8000/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "1",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "user",
        "parts": [{
          "kind": "text",
          "text": "Use the code-review skill to review /app/docsclaw (the main.go file)"
        }]
      }
    }
  }'
```

To add your own skill, create a directory under `skills/` with a
`SKILL.md` file:

```markdown
---
name: my-skill
description: What this skill does (shown to the LLM at startup)
---

# My skill

Step-by-step instructions for the LLM to follow...
```

## Configuration

### system-prompt.txt

The agent's personality. Plain text, no special format.

### agent-card.json

A2A protocol metadata — name, description, capabilities. See the
[A2A spec](https://google.github.io/A2A/) for the full schema.

### agent-config.yaml

Controls which tools are available and loop behavior:

```yaml
tools:
  allowed:            # Only these tools are available to the LLM
    - exec
    - web_fetch
    - read_file
    - write_file
  exec:
    timeout: 30       # Max seconds per command
    max_output: 50000 # Truncate output after this many chars
  web_fetch:
    allowed_hosts:    # SSRF protection (empty = all allowed)
      - go.dev
      - kubernetes.io
  workspace: /tmp/agent-workspace  # read_file/write_file root

loop:
  max_iterations: 10  # Max tool-use rounds before giving up
```

### Built-in tools

| Tool | Description |
|------|-------------|
| `exec` | Run shell commands (dangerous commands blocked) |
| `web_fetch` | HTTP GET with SSRF protection |
| `read_file` | Read files (workspace-restricted) |
| `write_file` | Write files (workspace-restricted) |
| `load_skill` | Load a SKILL.md (always available when skills exist) |

### LLM providers

Set via environment variables:

| Variable | Description |
|----------|-------------|
| `LLM_API_KEY` or `ANTHROPIC_API_KEY` | API key |
| `LLM_PROVIDER` | `anthropic` (default), `openai`, `litellm` |
| `LLM_MODEL` | Model name (default: `claude-sonnet-4-20250514`) |
| `LLM_BASE_URL` | Base URL for OpenAI-compatible APIs |

## Build

```bash
make build          # Build binary to bin/docsclaw
make test           # Run tests
make lint           # Run golangci-lint
make image          # Build container image
make image-push     # Build and push to GHCR
```

Override defaults:

```bash
make image REGISTRY=my-registry.io/docsclaw DEV_TAG=v1.0
make image CONTAINER_ENGINE=docker
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│  docsclaw serve                                 │
│                                                 │
│  ┌──────────┐   ┌──────────┐   ┌────────────┐  │
│  │ A2A      │──▶│ Agentic  │──▶│ LLM        │  │
│  │ Endpoint │   │ Loop     │   │ Provider   │  │
│  └──────────┘   └────┬─────┘   └────────────┘  │
│                      │                          │
│            ┌─────────┼─────────┐                │
│            ▼         ▼         ▼                │
│       ┌────────┐ ┌────────┐ ┌────────┐          │
│       │  exec  │ │  web   │ │  read  │ ...      │
│       │        │ │  fetch │ │  file  │          │
│       └────────┘ └────────┘ └────────┘          │
│                                                 │
│  Config: system-prompt.txt + agent-config.yaml  │
└─────────────────────────────────────────────────┘
```

## License

Apache License 2.0
