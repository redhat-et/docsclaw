#!/usr/bin/env bash
# redeploy.sh — Update the draft skill ConfigMap and optionally restart
#
# Usage:
#   ./redeploy.sh <skill-file>              # content-only (auto-sync)
#   ./redeploy.sh <skill-file> --restart    # full restart
#
# The auto-sync path updates the ConfigMap and lets the kubelet
# propagate the change to the mounted volume (~30-60s). The agent's
# load_skill tool reads from disk on each call, so new content is
# picked up on the next request after sync completes.
#
# The --restart path additionally triggers a rollout restart, which
# creates a new pod that reads the updated ConfigMap immediately
# (~10-15s). Use this when you change the skill's name or description
# in the YAML frontmatter.

set -euo pipefail

CONFIGMAP_NAME="skill-playground-draft-skill"
DEPLOYMENT_NAME="skill-playground"

usage() {
    echo "Usage: $0 <skill-file> [--restart]"
    echo ""
    echo "  <skill-file>   Path to the SKILL.md file"
    echo "  --restart       Trigger a rollout restart (for metadata changes)"
    exit 1
}

if [[ $# -lt 1 ]]; then
    usage
fi

SKILL_FILE="$1"
RESTART=false

if [[ $# -ge 2 && "$2" == "--restart" ]]; then
    RESTART=true
fi

if [[ ! -f "$SKILL_FILE" ]]; then
    echo "Error: file not found: $SKILL_FILE"
    exit 1
fi

START=$(date +%s)

echo "Updating ConfigMap ${CONFIGMAP_NAME}..."
kubectl create configmap "$CONFIGMAP_NAME" \
    --from-file="SKILL.md=${SKILL_FILE}" \
    --dry-run=client -o yaml | kubectl apply -f -

if [[ "$RESTART" == "true" ]]; then
    echo "Triggering rollout restart..."
    kubectl rollout restart deployment/"$DEPLOYMENT_NAME"
    echo "Waiting for rollout to complete..."
    kubectl rollout status deployment/"$DEPLOYMENT_NAME" --timeout=60s
else
    echo "ConfigMap updated. Kubelet will sync the volume in ~30-60s."
    echo "The agent will pick up new content on the next request after sync."
fi

END=$(date +%s)
ELAPSED=$((END - START))
echo "Done in ${ELAPSED}s."
