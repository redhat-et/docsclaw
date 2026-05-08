# MCP client configuration guide

Connect DocsClaw agents to external
[MCP](https://modelcontextprotocol.io/) tool servers. MCP tools
integrate transparently into the agentic loop — the LLM sees them
alongside built-in tools and uses them the same way.

## Supported transports

| Transport | Use case | Config fields |
| --------- | -------- | ------------- |
| `streamable_http` | Remote MCP servers accessible over HTTP | `url` |
| `stdio` | MCP servers co-located in the same container or host | `command`, `args`, `env` |

## Configuration

Add MCP servers to `agent-config.yaml` under `tools.mcp`:

```yaml
tools:
  allowed:
    - exec
    - read_file
  mcp:
    - name: weather
      transport: streamable_http
      url: "http://weather-tool:8000/mcp"
    - name: localtools
      transport: stdio
      command: python
      args: ["-m", "local_mcp_server"]
      env:
        LOG_LEVEL: debug

loop:
  max_iterations: 10
```

### Field reference

| Field | Required | Transport | Description |
| ----- | -------- | --------- | ----------- |
| `name` | yes | both | Unique server name, used as tool name prefix |
| `transport` | yes | both | `streamable_http` or `stdio` |
| `url` | yes | streamable_http | Server endpoint URL |
| `command` | yes | stdio | Executable to run |
| `args` | no | stdio | Command arguments |
| `env` | no | stdio | Environment variables for the subprocess |

### Tool naming

Tool names are prefixed with the server name to prevent collisions.
If a server named `weather` exposes a tool called `get_forecast`,
the LLM sees it as `weather_get_forecast`.

### Allowlist behavior

MCP tools bypass the `tools.allowed` list. Configuring a server
implies trust — all its tools are available to the LLM.

## Examples

### Filesystem access via stdio

Use the official MCP filesystem server to give the agent read/write
access to a directory:

```yaml
tools:
  mcp:
    - name: fs
      transport: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
```

Requires Node.js. The server exposes tools like `fs_read_file`,
`fs_write_file`, `fs_list_directory`, and others.

### Remote MCP server

Connect to an MCP server running as a separate service (e.g., in a
sidecar container or on the network):

```yaml
tools:
  mcp:
    - name: db
      transport: streamable_http
      url: "http://db-tools.svc.cluster.local:8080/mcp"
```

### Multiple servers

You can configure as many MCP servers as needed. Each must have a
unique `name`:

```yaml
tools:
  allowed:
    - exec
  mcp:
    - name: fs
      transport: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    - name: weather
      transport: streamable_http
      url: "http://weather-api:8000/mcp"
```

## Lifecycle

MCP servers connect on agent startup. If any configured server
is unreachable, the agent fails to start with a clear error
message naming the server. During operation, tool call failures
surface as errors to the LLM, which can decide how to proceed.

Connections are closed when the agent shuts down.

## Testing locally

1. Create a config directory:

```bash
mkdir -p /tmp/mcp-test-config
```

2. Write a system prompt:

```bash
echo "You are a helpful assistant with filesystem tools." \
  > /tmp/mcp-test-config/system-prompt.txt
```

3. Write the agent config:

```bash
cat > /tmp/mcp-test-config/agent-config.yaml << 'EOF'
tools:
  mcp:
    - name: fs
      transport: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

loop:
  max_iterations: 5
EOF
```

4. Start the agent:

```bash
docsclaw serve \
  --config-dir /tmp/mcp-test-config \
  --llm-provider anthropic \
  --llm-api-key $ANTHROPIC_API_KEY \
  --llm-model claude-sonnet-4-6 \
  --port 8888 --listen-plain-http
```

Look for `connected to MCP server name=fs tools=14` in the output.

5. Send a test request:

```bash
curl -s http://localhost:8888/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"docsclaw","messages":[
    {"role":"user","content":"List files in /tmp/mcp-test-config"}
  ]}' | jq -r '.choices[0].message.content'
```

## Troubleshooting

| Symptom | Cause | Fix |
| ------- | ----- | --- |
| Agent fails to start with "failed to connect" | MCP server unreachable | Check the URL or command path |
| "unsupported transport" error | Typo in transport field | Use `streamable_http` or `stdio` |
| "duplicate name" error | Two servers share a name | Use unique names |
| Tool call returns error to LLM | Server-side failure | Check MCP server logs |
| Open WebUI can't reach the agent | Docker networking | Use `host.docker.internal` instead of `localhost` |
