# WP01: OpenShell spike implementation plan

> **For agentic workers:** this is a research WP. It produces no Go code.
> Read `00-index.md` first. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Answer the open OpenShell questions that later WPs depend on, and
record the answers in one note.

**Architecture:** Read the OpenShell repository (and run a sandbox if one is
available) to find out how egress is intercepted, what a policy denial looks
like to a Go HTTP client, and what a bring-your-own-container image needs.

**Tech stack:** OpenShell docs and source; `curl` or a tiny Go probe inside a
sandbox, if one is available.

**Spec:** `../2026-11-06-docsclaw-zero-trust-plan.md`, sections 1.2, 1.4, 1.7,
and "Open questions".

## Deliverable

`docs/dev/zero-trust/openshell-notes.md`, with one section per question
below. Each answer cites a file path and commit SHA in the OpenShell repo, or
shows observed output. Mark anything you inferred rather than observed as
**unverified**.

## Review focus

- An answer that is copied from docs but contradicted by source code: when
  they disagree, trust the source and say so.
- HTTPS peers: whether the supervisor can inject a bearer token into a
  request that a Go client sends through `CONNECT` (it can only do that if it
  terminates TLS).

---

### Task 1: Egress interception and denial shape

- [ ] **Step 1:** Clone or locate the OpenShell repo. Read
  `docs/how-it-works/sandboxes/overview.mdx` plus the supervisor/proxy source
  it points to.
- [ ] **Step 2:** Answer each of these in the notes:
  1. Is interception transparent (iptables/netns redirect), or does it rely
     on `HTTP_PROXY`/`HTTPS_PROXY`? Which env vars does the sandbox set,
     including `NO_PROXY`?
  1. Go's `http.ProxyFromEnvironment` never proxies `localhost`/loopback
     targets and honors `NO_PROXY`. Does that matter for any demo path?
  1. For `https://` targets: does the supervisor terminate TLS (MITM with a
     sandbox-trusted CA)? If so, where is the CA bundle mounted, and does Go's
     `crypto/x509` pick it up via `SSL_CERT_FILE` or the system pool?
  1. What exactly does a Go client see on a policy denial: connection
     refused, an HTTP 403 from the proxy (which headers and body?), a `CONNECT`
     failure (`proxyconnect` error text), or a DNS failure? Give a distinctive
     marker that WP05 can match on, such as a response header, body field, or
     error substring.
  1. How does the supervisor report a failed token exchange (for example, the
     IdP refused)? Is that distinguishable from a policy denial?
- [ ] **Step 3:** If a sandbox is available, verify the answer to question 4
  with `curl -v` to a denied host and paste the trimmed output. If no sandbox
  is available, tell the user what is missing (see the global CLAUDE.md
  "Environment setup preferences"). Do not simulate one.

### Task 2: Inbound service exposure

- [ ] **Step 1:** Read OpenShell RFC 0017 and the `openshell service expose`
  implementation.
- [ ] **Step 2:** Record:
  1. The address the sandbox app must listen on. Is loopback enough?
  1. Whether `bearer-passthrough` strips or keeps other headers, and whether
     it adds any identifying headers (it must not be trusted if it does).
  1. The public URL format peers will use (input for WP11 `a2a_peers.url`).

### Task 3: Sandbox image requirements

- [ ] **Step 1:** Read `examples/bring-your-own-container` in the OpenShell
  repo.
- [ ] **Step 2:** Record required binaries (shell, `sleep`, others),
  user/UID constraints, filesystem layout, entrypoint expectations, and how
  provider profiles bind credentials to binary paths (exact path matching
  rules, and whether symlinks count).

### Task 4: LLM provider placeholders

- [ ] **Step 1:** Find how OpenShell injects provider placeholders. Record
  the env var names, the placeholder format, and which request locations it
  substitutes (`Authorization` header, `x-api-key`, query string).
- [ ] **Step 2:** State whether the Anthropic SDK header (`x-api-key`) and
  the OpenAI header (`Authorization: Bearer`) are both supported.

### Task 5: Commit

- [ ] **Step 1:** Lint the notes with the `markdown-lint` skill.
- [ ] **Step 2:** Commit on branch `docs/openshell-spike`:

```bash
git add docs/dev/zero-trust/openshell-notes.md
git commit -s -m "docs: record OpenShell sandbox behavior for zero-trust work"
```
