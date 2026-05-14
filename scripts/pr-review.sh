#!/usr/bin/env bash
set -euo pipefail

# pr-review.sh — generate an OpenCode-ready PR review block
# Usage: ./scripts/pr-review.sh <PR_NUMBER>
#
# Requires: gh CLI authenticated and in PATH
# Outputs: a ready-to-paste block for OpenCode using the requesting-code-review skill

if [ $# -lt 1 ]; then
    echo "Usage: $0 <PR_NUMBER>" >&2
    exit 1
fi

PR_NUMBER="$1"

# Verify gh is available
if ! command -v gh &>/dev/null; then
    echo "Error: 'gh' CLI not found in PATH." >&2
    echo "Install from: https://cli.github.com" >&2
    exit 1
fi

# Fetch PR metadata
PR_JSON=$(gh pr view "$PR_NUMBER" --json number,title,body,author,additions,deletions,changedFiles,baseRefName,headRefName,mergeCommit 2>/dev/null) || {
    echo "Error: Could not fetch PR #$PR_NUMBER. Check the number and your gh auth status." >&2
    exit 1
}

# Extract fields using jq or fall back to gh --template
if command -v jq &>/dev/null; then
    TITLE=$(echo "$PR_JSON" | jq -r '.title')
    BODY=$(echo "$PR_JSON" | jq -r '.body // empty')
    AUTHOR=$(echo "$PR_JSON" | jq -r '.author.login')
    ADDITIONS=$(echo "$PR_JSON" | jq -r '.additions')
    DELETIONS=$(echo "$PR_JSON" | jq -r '.deletions')
    FILES=$(echo "$PR_JSON" | jq -r '.changedFiles')
    BASE_REF=$(echo "$PR_JSON" | jq -r '.baseRefName')
    HEAD_REF=$(echo "$PR_JSON" | jq -r '.headRefName')
    MERGE_COMMIT=$(echo "$PR_JSON" | jq -r '.mergeCommit.oid // empty')
else
    # Fallback: use gh template for plain text
    TITLE=$(gh pr view "$PR_NUMBER" --json title --jq '.title')
    AUTHOR=$(gh pr view "$PR_NUMBER" --json author --jq '.author.login')
    ADDITIONS=$(gh pr view "$PR_NUMBER" --json additions --jq '.additions')
    DELETIONS=$(gh pr view "$PR_NUMBER" --json deletions --jq '.deletions')
    FILES=$(gh pr view "$PR_NUMBER" --json changedFiles --jq '.changedFiles')
    BASE_REF=$(gh pr view "$PR_NUMBER" --json baseRefName --jq '.baseRefName')
    HEAD_REF=$(gh pr view "$PR_NUMBER" --json headRefName --jq '.headRefName')
    MERGE_COMMIT=$(gh pr view "$PR_NUMBER" --json mergeCommit --jq '.mergeCommit.oid // empty')
    BODY=$(gh pr view "$PR_NUMBER" --json body --jq '.body // empty')
fi

echo "========================================"
echo "Copy the block below into your OpenCode session"
echo "========================================"
echo ""

echo "Use the \`requesting-code-review\` skill to review this PR."
echo ""
echo "## PR Details"
echo ""
echo "**PR:** #$PR_NUMBER — $TITLE by @$AUTHOR"
echo ""
echo "**Branch:** \`$HEAD_REF\` → \`$BASE_REF\`"
echo ""
echo "**Stats:** +$ADDITIONS / -$DELETIONS in $FILES files"
echo ""

if [ -n "$MERGE_COMMIT" ]; then
    echo "**Merge commit:** \`$MERGE_COMMIT\`"
    echo ""
fi

if [ -n "$BODY" ] && [ "$BODY" != "null" ]; then
    echo "**Description:**"
    echo "$BODY"
    echo ""
fi

echo "## Full Diff"
echo '```diff'
gh pr diff "$PR_NUMBER"
echo '```'

echo ""
echo "========================================"
echo "End of review block"
echo "========================================"
