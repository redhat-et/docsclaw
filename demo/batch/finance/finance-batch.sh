#!/usr/bin/env bash
# Finance analyst batch demo — fan-out five contracts to five agents.
#
# Usage:
#   ./finance-batch.sh [namespace]
#
# Prerequisites:
#   - finance-analyst-config ConfigMap already applied
#   - document-service running with seed data
#   - llm-secret exists in namespace
#
# The script:
#   1. Deploys five finance-analyst instances
#   2. Waits for all pods to be ready
#   3. Sends one contract to each agent in parallel
#   4. Collects results
#   5. Optionally cleans up the instances

set -euo pipefail

NAMESPACE="${1:-panni-docsclaw}"
TEMPLATE_DIR="$(cd "$(dirname "$0")" && pwd)"
CONTRACTS=(DOC-CTR-V001 DOC-CTR-V002 DOC-CTR-V003 DOC-CTR-V004 DOC-CTR-V005)
TIMEOUT="120s"

echo "=== Finance Analyst Batch Demo ==="
echo "Namespace: $NAMESPACE"
echo "Contracts: ${CONTRACTS[*]}"
echo ""

# --- Step 1: Deploy agents ---
echo "--- Deploying ${#CONTRACTS[@]} finance analyst agents ---"
for i in "${!CONTRACTS[@]}"; do
    num=$(printf "%03d" $((i + 1)))
    name="finance-analyst-${num}"
    echo "  Creating $name..."
    sed -e "s/INSTANCE_NAME/${name}/g" \
        -e "s/NAMESPACE/${NAMESPACE}/g" \
        "$TEMPLATE_DIR/deployment-template.yaml" | oc apply -f -
done
echo ""

# --- Step 2: Wait for readiness ---
echo "--- Waiting for pods to be ready ---"
for i in "${!CONTRACTS[@]}"; do
    num=$(printf "%03d" $((i + 1)))
    name="finance-analyst-${num}"
    echo -n "  Waiting for $name... "
    oc wait --for=condition=available --timeout=60s \
        "deployment/$name" -n "$NAMESPACE" 2>/dev/null
done
echo ""

# --- Step 3: Send contracts in parallel ---
echo "--- Sending contracts to agents ---"
RESULTS_DIR=$(mktemp -d)
PIDS=()

for i in "${!CONTRACTS[@]}"; do
    num=$(printf "%03d" $((i + 1)))
    name="finance-analyst-${num}"
    contract="${CONTRACTS[$i]}"
    route=$(oc get route "$name" -n "$NAMESPACE" -o jsonpath='{.spec.host}')

    echo "  $name <- $contract (https://$route)"
    a2a send --timeout "$TIMEOUT" "https://$route" \
        "run analysis for document $contract" \
        > "$RESULTS_DIR/${name}.txt" 2>&1 &
    PIDS+=($!)
done
echo ""

# --- Step 4: Collect results ---
echo "--- Waiting for results ---"
FAILED=0
for i in "${!CONTRACTS[@]}"; do
    num=$(printf "%03d" $((i + 1)))
    name="finance-analyst-${num}"
    contract="${CONTRACTS[$i]}"
    pid="${PIDS[$i]}"

    if wait "$pid"; then
        echo "  ✓ $name ($contract) — completed"
    else
        echo "  ✗ $name ($contract) — failed"
        FAILED=$((FAILED + 1))
    fi
done
echo ""

# --- Step 5: Print results ---
echo "--- Results ---"
for i in "${!CONTRACTS[@]}"; do
    num=$(printf "%03d" $((i + 1)))
    name="finance-analyst-${num}"
    contract="${CONTRACTS[$i]}"
    echo ""
    echo "=============================="
    echo "$name ($contract)"
    echo "=============================="
    cat "$RESULTS_DIR/${name}.txt"
done

echo ""
echo "Results saved in: $RESULTS_DIR"
echo "Failed: $FAILED / ${#CONTRACTS[@]}"
echo ""

# --- Step 6: Cleanup prompt ---
read -r -p "Delete the ${#CONTRACTS[@]} agent instances? [y/N] " answer
if [[ "$answer" =~ ^[Yy]$ ]]; then
    echo "Cleaning up..."
    for i in "${!CONTRACTS[@]}"; do
        num=$(printf "%03d" $((i + 1)))
        name="finance-analyst-${num}"
        oc delete deployment,service,route "$name" -n "$NAMESPACE"
    done
    echo "Done."
else
    echo "Agents left running. Clean up with:"
    echo "  oc delete deployment,service,route -l batch-role=finance-analyst -n $NAMESPACE"
fi
