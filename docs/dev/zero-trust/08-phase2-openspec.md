# WP08: Phase 2 OpenSpec change implementation plan

> **For agentic workers:** documentation WP. Read `00-index.md` first. Use
> the `openspec-propose` skill.

**Goal:** Create the OpenSpec change `a2a-inbound-auth` for WP09-WP13, so
the user can approve the auth design (library choice, error codes, session
isolation) before code lands.

**Spec:** parent plan Phase 2; `00-index.md` decisions 5-6 and the error
code table; `openshell-notes.md` Task 2.

---

## Tasks

### Task 1: Write the change

- [ ] **Step 1:** Create `openspec/changes/a2a-inbound-auth/`:
  - `proposal.md`: why (bearer passthrough lets unauthenticated requests
    through), and what changes (`pkg/inboundauth`, the `inbound_auth`
    config, loopback default, `a2a_call`, session isolation). Mark the
    loopback default as **BREAKING**.
  - `design.md`: the library choice with the justification from WP09 Task
    1 (if WP09 has not run yet, list the candidates and the criteria); the
    full error-code table copied from `00-index.md`; JWKS caching and the
    refresh rate limit; `AllowedCallers` matching rules (a trailing `/`
    means prefix match, otherwise exact); `azp` with `client_id` as the
    fallback; scope parsing (space-delimited `scope` string or `scp`
    array); the session key `sub|azp|x-session-id`; the token-stripping
    guarantee and how WP12 proves it.
  - `specs/`: `inbound-auth`, `a2a-call-tool`, and `serve-binding`
    requirement deltas, with one scenario per error code.
  - `tasks.md`: groups mirroring WP09-WP13.
- [ ] **Step 2:** Run the `markdown-lint` skill. Commit on
  `docs/openspec-a2a-inbound-auth`: `docs: propose a2a-inbound-auth change`.
- [ ] **Step 3:** Stop and ask the user to review `design.md`. WP10 and
  WP11 wait for approval. WP09 may run in parallel, but must adopt any
  design changes.
