#!/usr/bin/env bash
set -euo pipefail

WEAVIATE_URL="${WEAVIATE_URL:-http://localhost:8080}"

echo "==> Pulling embedding model (first run only)..."
docker compose exec ollama ollama pull nomic-embed-text 2>/dev/null || true

echo "==> Creating Docs collection..."
curl -sf -X POST "${WEAVIATE_URL}/v1/schema" \
  -H "Content-Type: application/json" \
  -d '{
    "class": "Docs",
    "vectorizer": "text2vec-ollama",
    "moduleConfig": {
      "text2vec-ollama": {
        "apiEndpoint": "http://ollama:11434",
        "model": "nomic-embed-text"
      }
    },
    "properties": [
      {"name": "content", "dataType": ["text"]}
    ]
  }' && echo " OK" || echo " (may already exist)"

echo "==> Ingesting sample documents..."
docs=(
  "DocsClaw is a lightweight A2A-compatible agent runtime written in Go."
  "The A2A protocol defines how agents communicate task requests and results."
  "MCP servers expose tools that agents can discover and call dynamically."
  "SPIFFE provides workload identity using X.509 SVIDs in zero-trust environments."
  "OPA enforces policy decisions at runtime using the Rego policy language."
  "Weaviate is an open-source vector database written in Go with built-in vectorization."
  "RAG stands for Retrieval-Augmented Generation: retrieve relevant chunks, then generate."
  "The rag_search tool sends a plain text query and receives ranked document chunks."
)

for doc in "${docs[@]}"; do
  curl -sf -X POST "${WEAVIATE_URL}/v1/objects" \
    -H "Content-Type: application/json" \
    -d "{\"class\": \"Docs\", \"properties\": {\"content\": \"${doc}\"}}" > /dev/null
done
echo "==> Ingested ${#docs[@]} documents into Docs collection"

echo "==> Verifying search..."
curl -sf -X POST "${WEAVIATE_URL}/v1/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ Get { Docs(nearText: {concepts: [\"agent identity\"]}, limit: 2) { content _additional { distance } } } }"}' \
  | python3 -m json.tool 2>/dev/null || jq . 2>/dev/null || cat
echo ""
echo "==> Done. Weaviate is ready for DocsClaw."
