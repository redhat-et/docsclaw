# WP13 (optional): Verify signed AgentCards implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` first. Start only after WP12 is green and the user
> confirms there is time. This WP is not a demo blocker.

**Goal:** Before the first call to a peer, `a2a_call` fetches the peer's
AgentCard and verifies its JWS signature against a configured public key.
On failure, the call is refused.

**Architecture:** An optional `card_public_key` (PEM file path) per
`a2a_peers` entry. Verification happens once per peer per process, and the
result is cached. Read `internal/bridge/signedcard.go` first to see what
signature format DocsClaw serves, and verify that same format.

**Spec:** parent plan section 2.4.

## Review focus

- The card URL is derived from the peer URL. A redirect to another host
  must fail verification.
- The verification cache must not cache failures forever. A peer that was
  fixed is retried on the next call.

---

### Task 1: Card verification

**Files:**

- Create: `internal/a2acall/cardverify.go`, `internal/a2acall/cardverify_test.go`
- Modify: `internal/cmd/agentconfig.go` (`A2APeer.CardPublicKey string`,
  `yaml:"card_public_key"`), `internal/a2acall/a2acall.go`

**Interfaces:**

- Produces: `func verifyCard(ctx context.Context, client *http.Client,
  peerURL string, pub crypto.PublicKey) error`. `Peer` gains `CardKey
  crypto.PublicKey` (nil means skip verification).

- [ ] **Step 1: Write the failing table test** `TestVerifyCard`: valid
  signature; tampered card; wrong key; unsigned card; redirect to another
  host. Generate the signed fixture the same way `signedcard.go` expects
  its input.
- [ ] **Step 2:** Implement, wire it into `a2a_call` before the first
  invoke, and add the tool result `peer <name>: agent card signature
  invalid`.
- [ ] **Step 3:** Run `make fmt lint test`. Commit:
  `feat(a2acall): verify signed peer agent cards`.
