#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="$SCRIPT_DIR/../demo/batch/data"

usage() {
    echo "Usage: $0 --scenario <hr|security|finance|all> --url <document-service-url>"
    echo ""
    echo "Load demo data into document-service."
    echo ""
    echo "Options:"
    echo "  --scenario    Which scenario data to load (hr, security, finance, all)"
    echo "  --url         Document-service base URL (e.g., http://localhost:8084)"
    exit 1
}

SCENARIO=""
BASE_URL=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --scenario) SCENARIO="$2"; shift 2 ;;
        --url)      BASE_URL="$2"; shift 2 ;;
        *)          usage ;;
    esac
done

[[ -z "$SCENARIO" || -z "$BASE_URL" ]] && usage

load_documents() {
    local file="$1"
    local count=0
    local total

    total=$(python3 -c "import json; print(len(json.load(open('$file'))))")
    echo "Loading $total documents from $(basename "$file")..."

    python3 -c "
import json, sys, urllib.request

docs = json.load(open('$file'))
url = '${BASE_URL}/documents'
success = 0
for doc in docs:
    body = json.dumps(doc).encode()
    req = urllib.request.Request(url, data=body,
        headers={'Content-Type': 'application/json'},
        method='POST')
    try:
        urllib.request.urlopen(req)
        success += 1
    except Exception as e:
        print(f'  WARN: {doc[\"id\"]}: {e}', file=sys.stderr)
print(f'  Loaded {success}/{len(docs)} documents')
"
}

case "$SCENARIO" in
    hr)
        load_documents "$DATA_DIR/hr-documents.json"
        ;;
    security)
        load_documents "$DATA_DIR/security-documents.json"
        ;;
    finance)
        load_documents "$DATA_DIR/finance-documents.json"
        ;;
    all)
        load_documents "$DATA_DIR/hr-documents.json"
        load_documents "$DATA_DIR/security-documents.json"
        load_documents "$DATA_DIR/finance-documents.json"
        ;;
    *)
        echo "ERROR: Unknown scenario: $SCENARIO"
        usage
        ;;
esac

echo "Done."
