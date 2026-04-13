---
name: checklist-auditor
description: >-
  Audit checklists for consistency against a corporate standard.
  Identifies gaps, inconsistencies, and missing steps. Use when
  verifying process documents across departments.
license: Apache-2.0
metadata:
  author: Red Hat ET
  category: operations
---

# Checklist auditor skill

When asked to audit checklists against a standard:

1. Use `read_file` to read the corporate standard checklist — this
   is the reference against which all others are compared
2. Extract the canonical list of steps, grouping them by phase or
   category if the standard uses phases (e.g., "Day 1-7",
   "Week 2-4", "Day 60-90")
3. For each department checklist:
   - Use `read_file` to read the checklist
   - Compare each step against the standard
   - Classify each step as:
     - **Present**: matches the standard (exact or equivalent)
     - **Modified**: present but differs in wording, scope, or timing
     - **Missing**: not present in this checklist
     - **Extra**: present in this checklist but not in the standard
4. Produce an audit report with two sections:

   **Summary table:**

   | Department | Present | Modified | Missing | Extra | Score |
   |------------|---------|----------|---------|-------|-------|

   **Detail per department** (only for departments with issues):
   - List each Missing step with the standard's wording
   - List each Modified step with both versions side by side
   - List each Extra step — these may be legitimate additions that
     should be considered for the standard

5. If requested, provide recommendations:
   - Steps that should be added to the standard (extras that appear
     in multiple departments)
   - Steps that may be outdated (missing from most departments)
6. Use `write_file` to save the audit report if requested

## Important guidelines

- Treat equivalent steps as matches even if wording differs — focus
  on intent, not exact text
- When a checklist uses different phase names but covers the same
  time period, align by timing rather than labels
- Flag contradictions (e.g., department A says "notify within 24h"
  but the standard says "notify within 48h") as Modified, not
  Present
- Do not assume Missing steps are wrong — the department may have a
  valid reason. Present findings, let the reviewer decide
