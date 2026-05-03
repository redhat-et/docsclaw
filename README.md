# DocsClaw

A ConfigMap-driven agentic runtime that turns any LLM into a
tool-using, [A2A](https://a2a-protocol.org/latest/)-compatible agent.
Deploy with a system prompt, agent card, and tool config — no code
changes needed. Runs at ~5 MiB per pod on OpenShift.

## Quick start

```bash
make build

ANTHROPIC_API_KEY=sk-... ./bin/docsclaw serve \
  --config-dir testdata/standalone \
  --listen-plain-http
```

Install the [A2A CLI](https://github.com/a2aproject/a2a-go/tree/main/cmd)
to discover and interact with agents:

```bash
go install github.com/a2aproject/a2a-go/v2/cmd/a2a@latest

a2a discover http://localhost:8000
a2a send http://localhost:8000 "Summarize https://go.dev/blog/go1.24"
```

Or use the built-in interactive chat:

```bash
./bin/docsclaw chat --agent-url http://localhost:8000
```

The agent uses `web_fetch` to retrieve the page, then summarizes
it. Replace the text with any task — the agent decides which tools
to call.

## How it works

DocsClaw reads its personality from a config directory:

```text
my-agent/
├── system-prompt.txt       # Who the agent is
├── agent-card.json         # A2A metadata (name, skills, URL)
├── agent-config.yaml       # Which tools to allow, loop settings
└── skills/                 # Optional SKILL.md instructions
    └── url-summary/
        └── SKILL.md
```

Same binary, different config = different agent.

Without `agent-config.yaml`, DocsClaw runs in **single-shot mode**
(no tools, prompt-in/response-out). With it, the **agentic loop**
is enabled and the LLM can call tools iteratively until the task
is complete.

## Deploy on OpenShift/Kubernetes

```bash
make image-push

oc new-project docsclaw
oc create secret generic llm-secret --from-literal=LLM_API_KEY=sk-...
oc apply -f deploy/standalone-agent.yaml
```

The manifest bundles ConfigMap + Deployment + Service + Route in
a single file. Secret examples for Anthropic, OpenAI, and MaaS
are in [`deploy/`](deploy/README.md). Test via the Route:

```bash
curl -X POST https://$(oc get route docsclaw -o jsonpath='{.spec.host}')/a2a \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"message/send","id":"1",
       "params":{"message":{"messageId":"m1","role":"user",
       "parts":[{"kind":"text","text":"Run uname -a and describe this system"}]}}}'
```

## Skills

Skills are markdown files that teach the agent specialized
behaviors. The agent discovers them at startup and can load them
on demand via the `load_skill` tool.

To add a skill, create a directory under `skills/` with a
`SKILL.md` file:

```markdown
---
name: my-skill
description: What this skill does (shown to the LLM at startup)
---

# My skill

Step-by-step instructions for the LLM to follow...
```

See `testdata/standalone/skills/` for examples (url-summary,
code-review).

### Automatic AgentCard skill injection

Discovered skills are automatically added to the AgentCard served
at `/.well-known/agent-card.json`. If a skill includes a
`skill.yaml` with richer metadata (namespace, author, tools), the
AgentCard entry gets derived tags. Static skills in
`agent-card.json` take precedence on ID conflict.

### OCI skill distribution

Skills can be packaged as OCI artifacts and distributed via
registries (quay.io, GHCR, Harbor, Zot) using the
[skillctl](https://github.com/redhat-et/skillimage) CLI. On
OpenShift 4.20+ they mount directly as image volumes — no init
container needed.

```bash
skillctl pack examples/skills/resume-screener
skillctl push --as-image examples/skills/resume-screener \
  quay.io/myorg/skill-resume-screener:1.0.0
skillctl inspect quay.io/myorg/skill-resume-screener:1.0.0
```

See the [OCI skills guide](docs/oci-skills-guide.md) for the full
workflow including signing, verification, and deployment manifests.

## Configuration

### system-prompt.txt

The agent's personality. Plain text, no special format.

### agent-card.json

A2A protocol metadata — name, description, capabilities.
Discovered skills are merged into the card at startup; you only
need to hardcode skills here if you want static entries that
override discovery. See the
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

| Tool         | Description                                          |
| ------------ | ---------------------------------------------------- |
| `exec`       | Run shell commands (dangerous commands blocked)      |
| `web_fetch`  | HTTP GET with SSRF protection                        |
| `read_file`  | Read files (workspace-restricted)                    |
| `write_file` | Write files (workspace-restricted)                   |
| `load_skill` | Load a SKILL.md (always available when skills exist) |

### LLM providers

Set via environment variables:

| Variable                             | Description                                |
| ------------------------------------ | ------------------------------------------ |
| `LLM_API_KEY` or `ANTHROPIC_API_KEY` | API key                                    |
| `LLM_PROVIDER`                       | `anthropic` (default), `openai`, `litellm` |
| `LLM_MODEL`                          | Model name (default: `claude-sonnet-4-6`)  |
| `LLM_BASE_URL`                       | Base URL for OpenAI-compatible APIs        |

## Architecture

```text
┌─────────────────────────────────────────────────┐
│  docsclaw serve                                 │
│                                                 │
│  ┌──────────┐   ┌──────────┐   ┌────────────┐   │
│  │ A2A      │──▶│ Agentic  │──▶│ LLM        │   │
│  │ Endpoint │   │ Loop     │   │ Provider   │   │
│  └──────────┘   └────┬─────┘   └────────────┘   │
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

## License

Apache License 2.0
