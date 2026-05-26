#!/usr/bin/env bash
# redeploy.sh — Update the draft skill ConfigMap and optionally restart
#
# Usage:
#   ./redeploy.sh <skill-file>              # content-only (auto-sync)
#   ./redeploy.sh <skill-file> --restart    # full restart
#
# The auto-sync path updates the ConfigMap and lets the kubelet
# propagate the change to the mounted volume (typically seconds).
# The agent's load_skill tool reads from disk on each call, so new
# content is picked up on the next request after sync completes.
#
# The --restart path additionally triggers a rollout restart, which
# creates a new pod that reads the updated ConfigMap immediately
# (~10-15s). Use this when you change the skill's name or description
# in the YAML frontmatter.

set -euo pipefail

CONFIGMAP_NAME="skill-playground-draft-skill"
DEPLOYMENT_NAME="skill-playground"
NAMESPACE=$(kubectl config view --minify --output 'jsonpath={..namespace}' 2>/dev/null)
NAMESPACE="${NAMESPACE:-default}"

usage() {
    echo "Usage: $0 <skill-file> [--restart] [-n NAMESPACE]"
    echo ""
    echo "  <skill-file>   Path to the SKILL.md file"
    echo "  --restart       Trigger a rollout restart (for metadata changes)"
    echo "  -n NAMESPACE    Kubernetes namespace (default: current context)"
    exit 1
}

if [[ $# -lt 1 ]]; then
    usage
fi

SKILL_FILE="$1"
shift
RESTART=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --restart)
            RESTART=true
            shift
            ;;
        -n)
            NAMESPACE="$2"
            shift 2
            ;;
        *)
            usage
            ;;
    esac
done

if [[ ! -f "$SKILL_FILE" ]]; then
    echo "Error: file not found: $SKILL_FILE"
    exit 1
fi

echo "Namespace: ${NAMESPACE}"
START=$(date +%s)

echo "Updating ConfigMap ${CONFIGMAP_NAME}..."
kubectl -n "$NAMESPACE" create configmap "$CONFIGMAP_NAME" \
    --from-file="SKILL.md=${SKILL_FILE}" \
    --dry-run=client -o yaml | kubectl -n "$NAMESPACE" apply -f -

if [[ "$RESTART" == "true" ]]; then
    echo "Triggering rollout restart..."
    kubectl -n "$NAMESPACE" rollout restart deployment/"$DEPLOYMENT_NAME"
    echo "Waiting for rollout to complete..."
    kubectl -n "$NAMESPACE" rollout status deployment/"$DEPLOYMENT_NAME" --timeout=60s
else
    echo "ConfigMap updated. The kubelet will sync the volume shortly."
    echo "The agent will pick up new content on the next request after sync."
fi

END=$(date +%s)
ELAPSED=$((END - START))
echo "Done in ${ELAPSED}s."
