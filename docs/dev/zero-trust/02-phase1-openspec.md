# WP02: Phase 1 OpenSpec change implementation plan

> **For agentic workers:** documentation WP. Read `00-index.md` first. Use
> the `openspec-propose` skill for structure. Steps use checkbox syntax.

**Goal:** Create the OpenSpec change `openshell-outbound-agent` that captures
Phase 1 (WP03-WP07), so the user can review one design before code lands.

**Architecture:** Follow the layout of the existing changes in
`openspec/changes/` (proposal, design, tasks, specs).

**Spec:** `../2026-11-06-docsclaw-zero-trust-plan.md` Phase 1, the decisions
in `00-index.md`, and `openshell-notes.md` (WP01).

---

## Tasks

### Task 1: Archive the finished change

`openspec/changes/rename-spiffe-and-fix-workspace/tasks.md` has 11/11 tasks
checked but has not been archived.

- [ ] **Step 1:** Run the `openspec-archive-change` skill for
  `rename-spiffe-and-fix-workspace`. Check `configurable-workspace-path` and
  `openclaw-workspace-compat` the same way (shipped in #98 and #97) and
  archive them if all their tasks are checked.
- [ ] **Step 2:** Commit: `chore: archive completed openspec changes`.

### Task 2: Write the change

- [ ] **Step 1:** Create `openspec/changes/openshell-outbound-agent/` with:
  - `proposal.md`: why (v2 sandboxes), what changes (bullets for
    WP03-WP07, with **BREAKING** marks on the removed flags and config keys,
    document mode, and the `fetch_document` parameters).
  - `design.md`: one section per WP. Include the egress answers and the
    denial marker from `openshell-notes.md`, the `internal/egress` package,
    the `document_services` schema, the `agentRuntime` extraction (WP06
    Interfaces), and the image layout. List index decisions 1-4 explicitly.
  - `specs/`: requirement deltas, using scenarios in the existing specs'
    WHEN/THEN style, for `fetch-document`, `egress`, `ask-command`, and
    `sandbox-image`.
  - `tasks.md`: one numbered group per WP (WP03-WP07), mirroring the task
    titles in those WP files.
- [ ] **Step 2:** Lint all files with the `markdown-lint` skill.
- [ ] **Step 3:** Commit on branch `docs/openspec-openshell-outbound`:
  `docs: propose openshell-outbound-agent change`.
- [ ] **Step 4:** Stop. Ask the user to review `design.md`. This is the
  "confirm before implementing" gate. WP04 onward waits for approval. WP03
  may already be in flight.
