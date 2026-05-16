# Add RAG support to DocsClaw (Weaviate)

## Goal

Add a `rag_search` tool that lets the agent retrieve the top-k semantically
similar chunks from a Weaviate collection. The query phrase is sent as plain
text — Weaviate handles vectorization internally via its `text2vec-ollama`
module. DocsClaw does no embedding work.

This is a demo of what's possible. One backend, one tool, minimum code.
If it lands well, other connectors (Milvus, pgvector) follow the same pattern.

---

## Design decisions

- One interface (`rag.Client`) with a single `Search` method. This makes future
  connectors drop-in additions.
- One implementation: `WeaviateClient`.
- Config lives in `agent-config.yaml` alongside MCP servers. The `rag` block is
  optional — if absent, no tool is registered.
- The operator owns the Weaviate instance and its data. DocsClaw only reads.

---

## Config schema

```yaml
rag:
  backend: weaviate
  url: http://weaviate:8080
  collection: Docs          # Weaviate collection name (case-sensitive)
  text_field: content       # the object property that holds the chunk text
  default_limit: 5
  max_limit: 20
```

---

## Implementation

### Step 1: Define the interface

Create `pkg/rag/rag.go`:

```go
package rag

import "context"

// Client is the interface every RAG backend must implement.
type Client interface {
    Search(ctx context.Context, query string, limit int) ([]Chunk, error)
}

// Chunk is a single retrieved document fragment.
type Chunk struct {
    ID       string
    Text     string
    Score    float64
    Metadata map[string]any
}
```

### Step 2: Implement the Weaviate client

> **Implementation note:** The shipped code uses raw HTTP/GraphQL
> (`net/http` + `encoding/json`) instead of the Weaviate Go SDK. This
> keeps the binary lean (zero new dependencies) and the approach
> generalizes to any GraphQL-speaking vector database.

See `pkg/rag/weaviate.go` for the implementation. `WeaviateClient`
sends a GraphQL `nearText` query to Weaviate's `/v1/graphql` endpoint,
parses the response, and converts distance to similarity score
(`Score = 1 - distance`, assumes cosine distance metric).

The tool implementation is in `internal/ragsearch/ragsearch.go`.

### Step 3: Config struct and factory

`pkg/rag/config.go` (or inline in the config package):

```go
type RAGConfig struct {
    Backend      string `yaml:"backend"`
    URL          string `yaml:"url"`
    Collection   string `yaml:"collection"`
    TextField    string `yaml:"text_field"`
    DefaultLimit int    `yaml:"default_limit"`
    MaxLimit     int    `yaml:"max_limit"`
}
```

Apply defaults after unmarshalling: `DefaultLimit: 5`, `MaxLimit: 20`,
`TextField: "content"` if empty.

`pkg/rag/factory.go`:

```go
func NewClient(cfg *config.RAGConfig) (Client, error) {
    switch cfg.Backend {
    case "weaviate":
        host := strings.TrimPrefix(cfg.URL, "http://")
        host = strings.TrimPrefix(host, "https://")
        return NewWeaviateClient(host, cfg.Collection, cfg.TextField)
    default:
        return nil, fmt.Errorf("unsupported RAG backend: %q", cfg.Backend)
    }
}
```

### Step 4: Register the tool

Alongside `read_file`, `fetch_doc`, etc.:

```go
if cfg.RAG != nil {
    ragClient, err := rag.NewClient(cfg.RAG)
    if err != nil {
        return fmt.Errorf("rag: %w", err)
    }
    registry.Register(tools.NewRAGSearchTool(ragClient, cfg.RAG))
}
```

### Step 5: Implement the tool

Create `pkg/tools/rag_search.go`. Tool name: `rag_search`.

Tool description for the LLM:

```text
Search the document store for chunks semantically related to the query.
Use this when the user's question requires information from indexed documents.
Returns the top-k most relevant text chunks.
```

Input schema:

```json
{
  "type": "object",
  "properties": {
    "query": { "type": "string" },
    "limit": { "type": "integer", "description": "Number of chunks. Default: 5." }
  },
  "required": ["query"]
}
```

Handler logic:

1. Use `cfg.DefaultLimit` if `limit` is absent from the tool call.
1. Cap at `cfg.MaxLimit` before calling the client.
1. Format returned chunks as numbered blocks for the LLM:

```text
[1] (score: 0.91)
DocsClaw is a lightweight A2A-compatible agent runtime written in Go.

[2] (score: 0.87)
The A2A protocol defines how agents communicate task requests and results.
```

---

## Local test deployment

Create `testdata/rag/docker-compose.yaml`:

```yaml
services:
  weaviate:
    image: cr.weaviate.io/semitechnologies/weaviate:1.37.2
    ports:
      - "8080:8080"
      - "50051:50051"
    volumes:
      - weaviate_data:/var/lib/weaviate
    environment:
      AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED: "true"
      PERSISTENCE_DATA_PATH: /var/lib/weaviate
      ENABLE_MODULES: text2vec-ollama
      CLUSTER_HOSTNAME: node1
      OLLAMA_API_ENDPOINT: http://ollama:11434

  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama

volumes:
  weaviate_data:
  ollama_data:
```

Start and pull the embedding model (only needed once — Ollama caches it in
the named volume):

```bash
cd testdata/rag
docker compose up -d   # or: podman-compose up -d
```

Check Weaviate is ready:

```bash
curl -s http://localhost:8080/v1/meta | jq .version
```

### Seed sample documents

Run the seeder script (`testdata/rag/seed.sh`). It uses the Ollama
and Weaviate HTTP APIs directly — no `docker compose exec` — so it
works with any container runtime:

```bash
cd testdata/rag
./seed.sh
```

The script pulls the embedding model, creates the `Docs` collection,
ingests 8 sample documents, and verifies search. Override endpoints
with `WEAVIATE_URL` and `OLLAMA_URL` environment variables if needed.

### Verify search before wiring DocsClaw

```bash
curl -s -X POST http://localhost:8080/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ Get { Docs(nearText: {concepts: [\"agent identity\"]}, limit: 3) { content _additional { distance } } } }"
  }' | jq '.data.Get.Docs[] | {content, distance: ._additional.distance}'
```

Ranked results mean Weaviate is working and DocsClaw can connect to it.

### Wire DocsClaw

Add to `testdata/standalone/agent-config.yaml`:

```yaml
rag:
  backend: weaviate
  url: http://localhost:8080
  collection: Docs
  text_field: content
  default_limit: 5
  max_limit: 20
```

Start DocsClaw and ask it something covered by the sample documents. It should
call `rag_search` and include the retrieved chunks in its answer.

---

## Kubernetes and OpenShift deployment

### Option A: minimal manifest (demo / no persistence)

Good enough to show the integration in a shared dev cluster. Data is lost on
pod restart, which is acceptable for a demo.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: weaviate-env
  namespace: docsclaw
data:
  AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED: "true"
  PERSISTENCE_DATA_PATH: /var/lib/weaviate
  ENABLE_MODULES: text2vec-ollama
  CLUSTER_HOSTNAME: weaviate-0
  OLLAMA_API_ENDPOINT: http://ollama:11434
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: weaviate
  namespace: docsclaw
spec:
  replicas: 1
  selector:
    matchLabels:
      app: weaviate
  template:
    metadata:
      labels:
        app: weaviate
    spec:
      containers:
        - name: weaviate
          image: cr.weaviate.io/semitechnologies/weaviate:1.37.2
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: weaviate-env
---
apiVersion: v1
kind: Service
metadata:
  name: weaviate
  namespace: docsclaw
spec:
  selector:
    app: weaviate
  ports:
    - port: 8080
      targetPort: 8080
```

Deploy Ollama the same way, then run `setup.go` against the port-forwarded
service to seed the collection.

### Option B: Helm chart (persistent, closer to production)

```bash
helm repo add weaviate https://weaviate.github.io/weaviate-helm
helm repo update

helm upgrade --install weaviate weaviate/weaviate \
  --namespace docsclaw \
  --create-namespace \
  --set authentication.anonymous_access.enabled=true \
  --set modules.text2vec-ollama.enabled=true \
  --set "env.OLLAMA_API_ENDPOINT=http://ollama.docsclaw.svc.cluster.local:11434"
```

The current chart version is 17.8.0. It deploys Weaviate as a StatefulSet
with a PVC — data survives pod restarts. The chart also sets up internode
authentication automatically when running multi-replica.

### OpenShift-specific notes

Weaviate runs as a non-root user by default, which is compatible with
OpenShift's restricted SCC. No `securityContext` overrides are needed for
a basic deployment.

Do not expose Weaviate via a `LoadBalancer` service on OpenShift. Instead,
keep it as a `ClusterIP` service for internal traffic and expose it with a
`Route` only if external access is needed for seeding or inspection:

```bash
oc expose service weaviate --name=weaviate-ext --namespace=docsclaw
```

DocsClaw's `rag.url` in its ConfigMap should always point to the internal
service (`http://weaviate.docsclaw.svc.cluster.local:8080`) regardless of
whether an external Route exists. Keep internal traffic internal.

### Seeding data on Kubernetes

The simplest approach is a local port-forward:

```bash
kubectl port-forward svc/weaviate 8080:8080 -n docsclaw &
kubectl port-forward svc/ollama 11434:11434 -n docsclaw &
./testdata/rag/seed.sh
# kill the port-forwards when done
```

For a more automated setup, package `seed.sh` into a minimal container
image and run it as a Kubernetes `Job` in the same namespace. The Job
runs once, seeds the collection, and completes. This is the right
approach when the demo environment is rebuilt frequently.

---

## Adding more backends later

The interface is the only extension point anyone needs to touch:

1. A new `pkg/rag/<backend>.go` implementing `rag.Client`.
1. A new `case` in `NewClient()` (`config.go`).
1. Any backend-specific config fields in `Config`, clearly commented.

The tool, the agent loop, and the config schema do not change.
