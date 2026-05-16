# RAG Search Local Testing Guide

Test the `rag_search` tool end-to-end with a local Weaviate instance
and Ollama for embeddings.

## Prerequisites

- Docker and Docker Compose
- Go toolchain
- An LLM API key (Anthropic or OpenAI-compatible)

## 1. Start the databases

```bash
cd testdata/rag
docker compose up -d
```

Wait for Weaviate to be ready:

```bash
curl -s http://localhost:8080/v1/meta | jq .version
```

Expected: a version string like `"1.37.2"`. If you get a connection
error, wait a few seconds and retry.

## 2. Seed sample data

The seeder script pulls the embedding model (first run only), creates
a `Docs` collection, ingests 8 sample documents, and verifies search
works:

```bash
cd testdata/rag
./seed.sh
```

The script uses HTTP APIs only — no `docker compose exec` — so it
works with Docker, Podman, or any setup where the services are
reachable. Override the defaults with environment variables:

```bash
WEAVIATE_URL=http://localhost:8080 OLLAMA_URL=http://localhost:11434 ./seed.sh
```

Expected output:

```text
==> Pulling embedding model (first run only, ~274 MB)...
   pulling manifest... pulling sha256:... success
==> Creating Docs collection...
   Collection created.
==> Ingesting sample documents...
   [1/8] ingested
   ...
==> Ingested 8 documents into Docs collection
==> Verifying search...
{
    "data": { ... ranked results ... }
}
==> Done. Weaviate is ready for DocsClaw.
```

The first run takes a minute or two while Ollama downloads the
`nomic-embed-text` model (~274 MB). Subsequent runs are instant.

## 3. Verify search directly (optional)

Before involving DocsClaw, confirm Weaviate returns ranked results:

```bash
curl -s -X POST http://localhost:8080/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ Get { Docs(nearText: {concepts: [\"agent identity\"]}, limit: 3) { content _additional { distance } } } }"
  }' | jq '.data.Get.Docs[] | {content, distance: ._additional.distance}'
```

You should see results ranked by semantic similarity, with lower
distance values indicating closer matches.

## 4. Configure DocsClaw for RAG

Uncomment the `rag:` block in `testdata/standalone/agent-config.yaml`:

```yaml
rag:
  backend: weaviate
  url: http://localhost:8080
  collection: Docs
  text_field: content
  default_limit: 5
  max_limit: 20
```

Or create a dedicated config directory by copying the standalone
config and enabling RAG:

```bash
cp -r testdata/standalone testdata/rag-demo
```

Then edit `testdata/rag-demo/agent-config.yaml` to add the `rag:`
block above, and update `system-prompt.txt` to mention RAG:

```
You are a helpful research assistant with access to tools.

You can:
- Execute shell commands (exec)
- Fetch web pages (web_fetch)
- Read and write files in your workspace (read_file, write_file)
- Search indexed documents for relevant information (rag_search)

When asked a question, first search the document store using rag_search
to find relevant context. Use the retrieved chunks to inform your answer.
Be concise in your responses. Show your work.
```

## 5. Start DocsClaw

```bash
go run ./cmd/docsclaw serve \
  --config-dir testdata/rag-demo \
  --llm-provider anthropic \
  --llm-model claude-sonnet-4-20250514
```

Look for this log line confirming RAG is wired up:

```
RAG search enabled  backend=weaviate collection=Docs
```

## 6. Test via chat

In another terminal:

```bash
go run ./cmd/docsclaw chat --agent-url http://localhost:8088
```

Try these queries (the sample data covers DocsClaw, A2A, MCP, SPIFFE,
OPA, Weaviate, and RAG):

| Query | Expected behavior |
|-------|-------------------|
| "What is DocsClaw?" | Calls `rag_search`, retrieves the DocsClaw chunk |
| "How do agents communicate?" | Retrieves A2A protocol chunk |
| "What is RAG?" | Retrieves the RAG definition chunk |
| "Tell me about security" | Retrieves SPIFFE and OPA chunks |

The agent should call `rag_search` automatically and include the
retrieved chunks in its answer. In the server logs, you'll see the
tool call and its results.

## 7. Test via curl (A2A protocol)

```bash
curl -s -X POST http://localhost:8088/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "message/send",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"kind": "text", "text": "What tools can agents use?"}]
      }
    }
  }' | jq '.result.artifacts[].parts[].text'
```

## 8. Clean up

```bash
cd testdata/rag
docker compose down -v
```

The `-v` flag removes the named volumes (Weaviate data and Ollama
models). Omit it if you want to keep the data for next time.

## Troubleshooting

**Weaviate not ready:** The container may take 10-20 seconds to
initialize. Check `docker compose logs weaviate` for startup progress.

**Embedding model slow:** The first `seed.sh` run downloads
`nomic-embed-text` (~274 MB). Check progress with
`docker compose logs ollama`.

**"class Docs already exists":** The seeder is idempotent for ingestion
but not for schema creation. If you see this, the collection already
exists from a previous run. Data will be appended. To start fresh:
`docker compose down -v && docker compose up -d`, then re-run
`seed.sh`.

**No search results in DocsClaw:** Verify the `rag.url` in your config
matches the Weaviate endpoint (default: `http://localhost:8080`). Check
the server logs for `rag search failed` errors.
