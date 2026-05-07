# Testing the OpenAI-Compatible API

DocsClaw exposes an OpenAI-compatible API on the same port as the
A2A endpoint (default 8000). This lets users connect existing chat
UIs like Open WebUI instead of building a custom frontend.

## Endpoints

| Method | Path                   | Description                                    |
| ------ | ---------------------- | ---------------------------------------------- |
| POST   | `/v1/chat/completions` | Chat completions (streaming and non-streaming) |
| GET    | `/v1/models`           | Lists the configured model                     |
| GET    | `/v1/skills`           | Lists agent skills                             |

## Quick verification

Replace `$BASE` with your DocsClaw base URL:

```bash
# Local development
export BASE=http://localhost:8000

# OpenShift route
export BASE=https://my-agent-namespace.apps.cluster.example.com
```

### Check the model

```bash
curl -s $BASE/v1/models | jq .
```

### Check available skills

```bash
curl -s $BASE/v1/skills | jq .
```

### Non-streaming chat

```bash
curl -s $BASE/v1/chat/completions \
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
curl -s -N $BASE/v1/chat/completions \
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
curl -s -N $BASE/v1/chat/completions \
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
curl -s $BASE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d 'not json' | jq .

# Empty messages → 400
curl -s $BASE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages": []}' | jq .
```

## Connecting Open WebUI (recommended)

Open WebUI is the easiest way to get a full chat interface — a
single container with zero configuration. Recommended for demos
and quick testing.

### With Podman (or Docker)

```bash
podman run -d -p 3000:8080 \
  -e OPENAI_API_BASE_URL=${BASE}/v1 \
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

## Connecting LibreChat

LibreChat is a heavier alternative — it runs five containers
(MongoDB, Redis, MeiliSearch, RAG pipeline, API) and offers
features like multi-model routing, RAG, and agent builders.
Use it when you need DocsClaw as one of several endpoints in a
shared chat portal. For simple demos, Open WebUI is faster to
set up.

LibreChat requires a cloned repo with `docker-compose` (or
`podman-compose`).

### Setup

1. Clone LibreChat:

   ```bash
   git clone https://github.com/danny-avila/LibreChat.git
   cd LibreChat
   cp .env.example .env
   ```

2. Create `docker-compose.override.yml` to mount the config and
   fix MongoDB on Apple Silicon (M1-M4):

   ```yaml
   services:
     mongodb:
       image: mongo:4.4.18
     api:
       volumes:
         - type: bind
           source: ./librechat.yaml
           target: /app/librechat.yaml
   ```

3. Create `librechat.yaml` with DocsClaw as the only endpoint:

   ```yaml
   version: 1.3.9
   cache: true

   endpoints:
     custom:
       - name: 'DocsClaw'
         apiKey: 'unused'
         baseURL: 'https://your-agent.apps.cluster.example.com/v1'
         models:
           default: ['docsclaw']
           fetch: true
         titleConvo: true
         titleModel: 'current_model'
         summarize: false
         modelDisplayLabel: 'DocsClaw'
   ```

   Replace the `baseURL` with your DocsClaw route.

4. If using Podman, update `.env` to use the Podman host address:

   ```bash
   sed -i '' 's|host.docker.internal|host.containers.internal|g' .env
   ```

5. Start LibreChat:

   ```bash
   docker compose up -d   # or: podman-compose up -d
   ```

6. Open http://localhost:3080, create an account, and select
   "DocsClaw" from the endpoint dropdown.

### Cleanup

```bash
docker compose down   # or: podman-compose down
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

```text
You are a document summarization assistant. Your role is to
summarize documents that users provide.

If a user asks you to do something outside document
summarization (write code, answer general questions, etc.),
politely redirect them: "I'm configured to help with document
summarization. Please provide a document you'd like me to
summarize."
```

**Output format control:**

```text
Always structure your summaries with these sections:
- Summary (2-3 sentences)
- Key decisions
- Open questions
- Action items
- Risk factors

Omit sections that don't apply to the document.
```

**Tone and audience:**

```text
Write for a technical audience familiar with cloud-native
technologies. Be concise — prefer bullet points over prose.
Do not use emojis.
```

**Skill-aware prompting** (when skills are loaded):

```text
You have access to specialized skills. Use them when the
user's request matches a skill's purpose. Do not attempt
tasks outside your available skills.
```

### Testing guardrails

After updating the system prompt, test with both in-scope and
out-of-scope requests:

```bash
# In-scope: should produce a structured summary
curl -s $BASE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "messages": [
      {"role": "user", "content": "Summarize: The Q1 report shows revenue of $2.4M, up 18% from Q4, driven by Project Atlas ($400K) and Project Beacon ($200K). Baseline spend is flat at $1.65M."}
    ]
  }' | jq -r '.choices[0].message.content'

# Out-of-scope: should be redirected
curl -s $BASE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "docsclaw",
    "messages": [
      {"role": "user", "content": "Write a Python script to sort a list."}
    ]
  }' | jq -r '.choices[0].message.content'
```

## Client compatibility

DocsClaw's OpenAI API is designed for **chat UIs** that let the
server control personality and skills. **Agent clients** that have
their own agentic loop bypass the server-side controls.

| Client      | Type    | Works with DocsClaw? | Notes                                                                      |
| ----------- | ------- | -------------------- | -------------------------------------------------------------------------- |
| Open WebUI  | Chat UI | Yes                  | Recommended — single container, zero config                                |
| LibreChat   | Chat UI | Yes                  | Heavier (5 containers), but has RAG and multi-model routing                |
| TypingMind  | Chat UI | Untested             | Proprietary (not open source)                                              |
| Goose       | Agent   | No                   | Sends its own system prompt and tools, treats DocsClaw as a dumb LLM proxy |
| Claude Code | Agent   | No                   | Same issue — own agentic loop                                              |

The key distinction: chat UIs are a **window** into the agent.
Agent clients **replace** the agent with their own logic and use
the endpoint as a model backend.

This is a feature, not a limitation — it means the admin controls
the agent's behavior entirely through the ConfigMap (system prompt + skills).
Users connecting via a chat UI can't install extra
tools or override the personality. The guardrails are server-side.

## Comparing A2A vs OpenAI API

Both interfaces reach the same LLM and tools. The difference is
protocol and audience:

| Aspect    | A2A                         | OpenAI API                          |
| --------- | --------------------------- | ----------------------------------- |
| Protocol  | JSON-RPC                    | REST                                |
| Streaming | A2A events (task lifecycle) | SSE (token deltas)                  |
| Session   | Server-side (x-session-id)  | Client-side (full history)          |
| Tools     | Visible in task events      | Server-side only (v1)               |
| Audience  | Agent-to-agent              | Human via chat UI                   |
| Client    | `a2a` CLI, other agents     | Open WebUI, curl, any OpenAI client |

The same request via both interfaces:

```bash
# A2A
a2a send --timeout 120s "$BASE" "Summarize: ..."

# OpenAI API
curl -s $BASE/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"docsclaw","messages":[{"role":"user","content":"Summarize: ..."}]}'
```
