# Testing the OpenAI-Compatible API

DocsClaw exposes an OpenAI-compatible API on the same port as the
A2A endpoint (default 8000). This lets users connect existing chat
UIs like Open WebUI instead of building a custom frontend.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/chat/completions` | Chat completions (streaming and non-streaming) |
| GET | `/v1/models` | Lists the configured model |
| GET | `/v1/skills` | Lists agent skills |

## Quick verification

Replace `$ROUTE` with your DocsClaw URL (local or OpenShift route):

```bash
export ROUTE=localhost:8000
# or
export ROUTE=my-agent-namespace.apps.cluster.example.com
```

### Check the model

```bash
curl -s https://$ROUTE/v1/models | jq .
```

### Check available skills

```bash
curl -s https://$ROUTE/v1/skills | jq .
```

### Non-streaming chat

```bash
curl -s https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "messages": [
      {"role": "user", "content": "Say hello in one sentence."}
    ]
  }' | jq .
```

### Streaming chat

```bash
curl -s -N https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "stream": true,
    "messages": [
      {"role": "user", "content": "Say hello in one sentence."}
    ]
  }'
```

### Document summarization (with document-summarizer skill)

```bash
curl -s -N https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "stream": true,
    "messages": [
      {
        "role": "user",
        "content": "Summarize this document:\n\nTitle: Kubernetes Pod Security Standards\n\nKubernetes defines three Pod Security Standards to cover the security spectrum. These standards are cumulative and range from highly permissive to highly restrictive.\n\nPrivileged: Unrestricted policy, providing the widest possible level of permissions. This policy allows for known privilege escalations.\n\nBaseline: Minimally restrictive policy which prevents known privilege escalations. Allows the default Pod configuration. Prohibits hostNetwork, hostPID, hostIPC, hostPorts, privileged containers, and adding capabilities beyond a default set.\n\nRestricted: Heavily restricted policy, following current Pod hardening best practices. Requires containers to run as non-root, drops all capabilities, and sets a read-only root filesystem."
      }
    ]
  }'
```

### Error handling

```bash
# Invalid JSON → 400
curl -s https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d 'not json' | jq .

# Empty messages → 400
curl -s https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages": []}' | jq .
```

## Connecting Open WebUI

Open WebUI is the easiest way to get a full chat interface.

### With Podman (or Docker)

```bash
podman run -d -p 3000:8080 \
  -e OPENAI_API_BASE_URL=https://$ROUTE/v1 \
  -e OPENAI_API_KEY=unused \
  --name open-webui \
  ghcr.io/open-webui/open-webui:main
```

Open http://localhost:3000, create a local account (first user
becomes admin), and select "docsclaw" from the model dropdown.

**Networking notes:**
- If DocsClaw is on OpenShift, the container reaches it directly
  via the public route — no special networking needed.
- If DocsClaw runs on localhost, use `host.containers.internal`
  (Podman) or `host.docker.internal` (Docker) instead of
  `localhost` in the URL.

### Cleanup

```bash
podman stop open-webui && podman rm open-webui
```

## System prompt considerations

The OpenAI API injects DocsClaw's configured system prompt on
every request. The client can send additional system messages,
but the server's prompt always takes priority (client messages
are appended after it).

This means the agent's behavior is controlled entirely by the
ConfigMap — different deployments can have different guardrails.

### Guardrail patterns

Without guardrails, the agent will answer any question (write
code, chat about weather, etc.). To constrain it to its intended
role, add boundaries to the system prompt:

**Scope restriction:**

```
You are a document summarization assistant. Your role is to
summarize documents that users provide.

If a user asks you to do something outside document
summarization (write code, answer general questions, etc.),
politely redirect them: "I'm configured to help with document
summarization. Please provide a document you'd like me to
summarize."
```

**Output format control:**

```
Always structure your summaries with these sections:
- Summary (2-3 sentences)
- Key decisions
- Open questions
- Action items
- Risk factors

Omit sections that don't apply to the document.
```

**Tone and audience:**

```
Write for a technical audience familiar with cloud-native
technologies. Be concise — prefer bullet points over prose.
Do not use emojis.
```

**Skill-aware prompting** (when skills are loaded):

```
You have access to specialized skills. Use them when the
user's request matches a skill's purpose. Do not attempt
tasks outside your available skills.
```

### Testing guardrails

After updating the system prompt, test with both in-scope and
out-of-scope requests:

```bash
# In-scope: should produce a structured summary
curl -s https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "messages": [
      {"role": "user", "content": "Summarize: The Q1 report shows revenue of $2.4M, up 18% from Q4, driven by Project Atlas ($400K) and Project Beacon ($200K). Baseline spend is flat at $1.65M."}
    ]
  }' | jq -r '.choices[0].message.content'

# Out-of-scope: should be redirected
curl -s https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "messages": [
      {"role": "user", "content": "Write a Python script to sort a list."}
    ]
  }' | jq -r '.choices[0].message.content'
```

## Comparing A2A vs OpenAI API

Both interfaces reach the same LLM and tools. The difference is
protocol and audience:

| Aspect | A2A | OpenAI API |
|--------|-----|------------|
| Protocol | JSON-RPC | REST |
| Streaming | A2A events (task lifecycle) | SSE (token deltas) |
| Session | Server-side (x-session-id) | Client-side (full history) |
| Tools | Visible in task events | Server-side only (v1) |
| Audience | Agent-to-agent | Human via chat UI |
| Client | `a2a` CLI, other agents | Open WebUI, curl, any OpenAI client |

The same request via both interfaces:

```bash
# A2A
a2a send --timeout 120s "https://$ROUTE" "Summarize: ..."

# OpenAI API
curl -s https://$ROUTE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"docsclaw","messages":[{"role":"user","content":"Summarize: ..."}]}'
```
