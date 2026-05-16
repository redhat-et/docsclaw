#!/usr/bin/env bash
set -euo pipefail

WEAVIATE_URL="${WEAVIATE_URL:-http://localhost:8080}"
OLLAMA_URL="${OLLAMA_URL:-http://localhost:11434}"

prettyjson() {
  jq . 2>/dev/null || python3 -m json.tool 2>/dev/null || cat
}

echo "==> Pulling embedding model (first run only, ~274 MB)..."
curl -s -X POST "${OLLAMA_URL}/api/pull" \
  -H "Content-Type: application/json" \
  -d '{"name": "nomic-embed-text"}' \
  | while IFS= read -r line; do
    status=$(echo "$line" | jq -r '.status // empty' 2>/dev/null || echo "")
    if [ -n "$status" ]; then
      printf "\r   %s" "$status"
    fi
  done
echo ""

echo "==> Creating Docs collection..."
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${WEAVIATE_URL}/v1/schema" \
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
  }')
if [ "$status" = "200" ]; then
  echo "   Collection created."
elif [ "$status" = "422" ]; then
  echo "   Collection already exists, continuing."
else
  echo "   ERROR: unexpected status $status"
  exit 1
fi

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

count=0
for doc in "${docs[@]}"; do
  payload=$(jq -n --arg content "$doc" '{class: "Docs", properties: {content: $content}}')
  response=$(curl -s -w "\n%{http_code}" -X POST "${WEAVIATE_URL}/v1/objects" \
    -H "Content-Type: application/json" \
    -d "$payload")
  http_code=$(echo "$response" | tail -1)
  if [ "$http_code" != "200" ]; then
    body=$(echo "$response" | sed '$d')
    echo "   FAILED (HTTP ${http_code}): ${doc:0:60}..."
    echo "   Response: ${body}"
    exit 1
  fi
  count=$((count + 1))
  echo "   [$count/${#docs[@]}] ingested"
done
echo "==> Ingested ${count} documents into Docs collection"

echo "==> Verifying search..."
curl -s -X POST "${WEAVIATE_URL}/v1/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ Get { Docs(nearText: {concepts: [\"agent identity\"]}, limit: 2) { content _additional { distance } } } }"}' \
  | prettyjson
echo ""
echo "==> Done. Weaviate is ready for DocsClaw."
