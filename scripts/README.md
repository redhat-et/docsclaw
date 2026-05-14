# Scripts

## `pr-review.sh` — OpenCode PR Review Context Generator

Generates a ready-to-paste review block for OpenCode using the **requesting-code-review** skill.

### Usage

```bash
./scripts/pr-review.sh <PR_NUMBER>
```

Example:

```bash
./scripts/pr-review.sh 42
```

### Requirements

- [GitHub CLI (`gh`)](https://cli.github.com) installed and authenticated
- `jq` (optional — script works without it, but slower due to extra `gh` calls)

### Workflow

1. **Run the script** in your terminal while your OpenCode session is open:
   ```bash
   ./scripts/pr-review.sh 42
   ```

2. **Copy the output block** (from `===` to `===`).

3. **Paste into OpenCode.** The block starts with:
   ```
   Use the `requesting-code-review` skill to review this PR.
   ```
   This triggers the skill automatically — you don't need to paste the template yourself.

4. **Review the results.** OpenCode will produce a structured review with:
   - **Strengths** — what's well done
   - **Issues** categorized as Critical / Important / Minor
   - **Assessment** — Ready to merge? Yes / No / With fixes

### Why This Works

The `requesting-code-review` skill contains `code-reviewer.md`, which defines:

- The full review checklist (Code Quality, Architecture, Testing, Production Readiness)
- The categorized output format
- The "Ready to merge?" verdict structure

By saying `Use the \`requesting-code-review\` skill`, you delegate the prompt engineering to the skill. You only provide the raw data (PR metadata + diff).
