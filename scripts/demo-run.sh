#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEMO_DIR="$SCRIPT_DIR/../demo/batch"
WORKER_COUNT=${WORKER_COUNT:-10}

echo "=== DocsClaw Batch Processing Demo ==="
echo ""

# --- Act 0: Setup ---
echo "--- Act 0: Namespace & infrastructure setup ---"

oc apply -f "$DEMO_DIR/namespace-setup.yaml"

# Create LLM secret in each namespace
for ns in demo-hr demo-security demo-finance; do
    if ! oc get secret llm-secret -n "$ns" >/dev/null 2>&1; then
        echo "Creating LLM secret in $ns..."
        oc create secret generic llm-secret \
            --from-literal=LLM_API_KEY="$LLM_API_KEY" \
            -n "$ns"
    fi
done

echo ""
echo "--- Deploying document-service ---"
echo "NOTE: Deploy document-service in each namespace manually"
echo "      or via your existing deployment scripts."
echo ""

# --- Act 1: HR Resume Screening ---
echo "--- Act 1: HR Resume Screening ---"
echo ""

# Generate worker manifests from template
echo "Generating $WORKER_COUNT worker manifests..."
for i in $(seq 1 "$WORKER_COUNT"); do
    sed "s/WORKER_INDEX/$i/g" \
        "$DEMO_DIR/hr/deployment.yaml" | \
        oc apply -f -
done

echo "Waiting for workers to be ready..."
for i in $(seq 1 "$WORKER_COUNT"); do
    oc rollout status "deployment/hr-worker-$i" -n demo-hr \
        --timeout=120s
done

echo ""
echo "Pod status:"
oc get pods -n demo-hr
echo ""
echo "Resource usage:"
oc top pods -n demo-hr 2>/dev/null || echo "(metrics not yet available)"
echo ""

# Seed HR data
echo "Seeding HR documents..."
HR_DOC_SVC=$(oc get svc document-service -n demo-hr \
    -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
if [[ -n "$HR_DOC_SVC" ]]; then
    "$SCRIPT_DIR/seed-demo-data.sh" \
        --scenario hr \
        --url "http://$HR_DOC_SVC:8080"
fi

# Build agent list
AGENTS=""
for i in $(seq 1 "$WORKER_COUNT"); do
    [[ -n "$AGENTS" ]] && AGENTS="$AGENTS,"
    AGENTS="${AGENTS}http://hr-worker-${i}.demo-hr.svc:8080"
done

# Build document list
DOC_LIST=$(python3 -c "print(','.join(f'DOC-R{i:03d}' for i in range(1, 101)))")

echo ""
echo "Starting batch processing..."
echo "  Agents: $WORKER_COUNT"
echo "  Documents: 100 resumes"
echo ""

docsclaw batch \
    --agents "$AGENTS" \
    --documents "$DOC_LIST" \
    --context-doc DOC-JD001 \
    --prompt "Fetch the job description (context document). Then fetch and evaluate each resume. Score each candidate 1-10 against the job requirements. Return a JSON array with document_id, candidate_name, score, strengths, weaknesses, and recommendation for each resume." \
    --output /tmp/hr-results.csv

echo ""
echo "Top 10 candidates:"
head -11 /tmp/hr-results.csv | column -t -s,
echo ""

# --- Act 2: Security Vulnerability Triage ---
echo "--- Act 2: Security Vulnerability Triage ---"
echo ""

oc apply -f "$DEMO_DIR/security/deployment.yaml"
oc rollout status deployment/security-analyst -n demo-security \
    --timeout=120s

echo "Pod status:"
oc get pods -n demo-security
echo ""

# --- Act 3: Finance Invoice Anomaly ---
echo "--- Act 3: Finance Invoice Anomaly Detection ---"
echo ""

oc apply -f "$DEMO_DIR/finance/deployment.yaml"
oc rollout status deployment/finance-analyst -n demo-finance \
    --timeout=120s

echo "Pod status:"
oc get pods -n demo-finance
echo ""

# --- Finale ---
echo "=== Finale: The Numbers ==="
echo ""
echo "All pods across demo namespaces:"
for ns in demo-hr demo-security demo-finance; do
    echo ""
    echo "--- $ns ---"
    oc top pods -n "$ns" 2>/dev/null || echo "(metrics pending)"
done

echo ""
echo "Demo complete."
