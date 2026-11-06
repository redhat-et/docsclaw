# WP09: `pkg/inboundauth` implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:test-driven-development.
> Read `00-index.md` (the error-code table is normative) first. No
> dependency on other WPs.

**Goal:** A standalone, framework-neutral `net/http` middleware that
verifies incoming JWT bearer tokens against a JWKS, enforces issuer,
audience, scope, and caller rules, puts the claims in the request context,
and strips the token.

**Architecture:** One package, no `internal/` imports. The files are split
by responsibility: config validation, JWKS key source, token verification,
HTTP middleware, and errors. The same package will later ship in an A2A
adapter for other agents, so the exported API stays at `Config`, `Claims`,
`Middleware`, `ClaimsFrom`, plus `ContextWithClaims` for tests.

**Tech stack:** Go stdlib plus one JWT/JWKS library (chosen in Task 1).

**Spec:** parent plan section 2.1.

## Global constraints for this WP

- `go list -deps ./pkg/inboundauth | grep docsclaw/internal` must print
  nothing.
- Accept ES256 and RS256 only. Check the algorithm against this allowlist
  **before** selecting a key.
- Never log the token, even at debug level. The package takes an optional
  `*slog.Logger` and logs only error codes and `kid`.

## Review focus

- JWKS unreachable at startup: `Middleware` still returns a handler (lazy
  fetch), and requests fail closed with `503 jwks_unavailable`, not 401.
  The demo uses this to show "issuer not in network policy".
- `kid` flooding: an attacker sends tokens with random `kid`s. Refresh the
  JWKS at most once per `MinRefreshInterval` (default 60s); between
  refreshes an unknown `kid` returns `401 invalid_token` without a fetch.
- `aud` as a string vs an array, and `scope` as a space-delimited string vs
  `scp` as an array: both forms are accepted.
- Lowercase `bearer` scheme: RFC 7235 says schemes are case-insensitive, so
  accept it. A token with surrounding whitespace is trimmed.
- An `ExemptPaths` entry must match exactly. `/health` must not exempt
  `/healthz` or `/health/../a2a`. Compare against `r.URL.Path` after
  `path.Clean`.

---

### Task 1: Library choice and package skeleton

**Files:**

- Create: `pkg/inboundauth/doc.go`, `pkg/inboundauth/config.go`,
  `pkg/inboundauth/config_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**

- Produces:

```go
type Config struct {
    Issuer             string
    JWKSURL            string
    Audience           string
    RequiredScopes     []string      // at least one must be present; empty = no scope check
    AllowedCallers     []string      // exact match, or prefix match when entry ends with "/"
    ClockSkew          time.Duration // default 30s
    ExemptPaths        []string
    MinRefreshInterval time.Duration // default 60s
    HTTPClient         *http.Client  // default http.DefaultClient (proxy-aware)
    Logger             *slog.Logger  // default slog.Default()
}
func (c Config) validate() error // Issuer, JWKSURL (absolute http/https), Audience required
```

- [ ] **Step 1:** Compare `github.com/lestrrat-go/jwx/v3` against
  `github.com/golang-jwt/jwt/v5` plus `github.com/MicahParks/keyfunc/v3` on
  these criteria: latest release date, a JWKS cache with refresh on unknown
  `kid` and a rate limit, explicit algorithm allowlisting, transitive
  dependency count (`go mod graph | wc -l` delta), and license. Write the
  table and the choice into `doc.go`'s design comment (and WP08's
  `design.md` if it exists).
- [ ] **Step 2:** Write the failing test `TestConfigValidate` (table): ok;
  missing issuer; missing JWKS URL; relative JWKS URL; missing audience;
  defaults applied (skew is 30s, refresh is 60s).
- [ ] **Step 3:** Implement `config.go` and `go get` the chosen library at
  its latest version. Run the tests. Expected: PASS.
- [ ] **Step 4:** Commit on `feature/inboundauth`:
  `feat(inboundauth): add config and choose JWT library`.

### Task 2: Test fixtures (issuer and JWKS server)

**Files:**

- Create: `pkg/inboundauth/testissuer_test.go`

**Interfaces:**

- Produces (test-only, `package inboundauth`):
  - `newTestIssuer(t) *testIssuer`, which holds an ES256 key and an RS256
    key with `kid`s `es-1` and `rs-1`, and an `httptest.Server` serving
    `/jwks.json` that counts hits.
  - `(*testIssuer).mint(t, alg string, mutate func(map[string]any)) string`.
    Default claims: `iss=https://issuer.test`, `aud=["engineering-analyst"]`,
    `sub=alice`, `azp=spiffe://openshell.local/openshell/sandbox/alice-assistant`,
    `scope="engineering read"`, `iat=now`, `nbf=now`, `exp=now+5m`.
    `alg="none"` and `alg="HS256"` produce unsigned and HMAC tokens (HMAC
    keyed with the RSA public key bytes, the classic confusion attack).
  - `(*testIssuer).rotate(t)`, which adds a key `es-2` to the JWKS.

WP10 and WP12 need the same issuer: keep it in a `_test.go` file here; WP10
copies the minimal parts into `internal/cmd`. Do not export test helpers.

- [ ] **Step 1:** Implement. There is no separate test for the fixture; the
  Task 3 tests exercise it.

### Task 3: Verification and middleware

**Files:**

- Create: `pkg/inboundauth/keys.go`, `pkg/inboundauth/verify.go`,
  `pkg/inboundauth/middleware.go`, `pkg/inboundauth/errors.go`,
  `pkg/inboundauth/middleware_test.go`

**Interfaces:**

- Produces:

```go
type Claims struct {
    Subject  string
    Issuer   string
    Audience []string
    AZP      string   // azp, or client_id if azp absent
    Scopes   []string
    Expiry   time.Time
    Raw      map[string]any // all claims, for callers needing extras; never contains the token
}
func Middleware(cfg Config, next http.Handler) (http.Handler, error) // error only from cfg.validate()
func ClaimsFrom(ctx context.Context) (Claims, bool)
func ContextWithClaims(ctx context.Context, c Claims) context.Context
```

- Behavior: exact codes, statuses, body, and `WWW-Authenticate` per
  `00-index.md`. For `403 insufficient_scope`, add `scope="<required
  scopes, space-joined>"` to `WWW-Authenticate`. On success:
  `r.Header.Del("Authorization")`, put the claims in the context, and call
  `next`.

- [ ] **Step 1: Write the failing table test** `TestMiddleware`. Each row
  has a request builder, the expected status, the expected `error`, and
  whether `next` ran. Rows:
  - valid ES256 gives 200, and `next` sees claims `sub=alice` and **no**
    `Authorization` header
  - valid RS256 gives 200
  - expired gives `401 token_expired`
  - `exp` 20s ago with 30s skew gives 200
  - `nbf` in the future gives `401 token_not_yet_valid`
  - `iat` in the future gives `401 token_not_yet_valid`
  - wrong issuer gives `401 invalid_issuer`
  - wrong audience gives `403 invalid_audience`
  - `aud` as a plain string gives 200
  - no required scope gives `403 insufficient_scope`, with `WWW-Authenticate`
    containing `scope="engineering"`
  - `scp` array form gives 200
  - disallowed caller gives `403 caller_not_allowed`
  - `azp` absent but `client_id` allowed gives 200
  - `azp` absent and no `client_id`, with `AllowedCallers` non-empty, gives
    `403 caller_not_allowed`
  - prefix rule: `.../sandbox/` allows `.../sandbox/bob`
  - prefix rule: `.../sandbox` (no slash) does not allow `.../sandboxes/x`
  - `alg: none` gives `401 unsupported_algorithm`
  - HS256 gives `401 unsupported_algorithm`
  - unknown `kid` gives `401 invalid_token`
  - tampered payload gives `401 invalid_token`
  - missing header gives `401 missing_token` and a `WWW-Authenticate` header
  - two `Authorization` headers give `401 malformed_authorization`
  - `Basic xyz` gives `401 malformed_authorization`
  - `Bearer` with an empty token gives `401 malformed_authorization`
  - lowercase `bearer <valid>` gives 200
  - exempt path `/health` with no token gives 200
  - `/healthz` with no token gives 401
  - `/health/../a2a` with no token gives 401
- [ ] **Step 2: Write failing tests** for the JWKS behavior:
  - `TestJWKS_RefreshOnUnknownKid`: `rotate`, mint with `es-2`. Expect
    200 and a hit count of 2.
  - `TestJWKS_RefreshRateLimited`: use a `MinRefreshInterval` of 1h, then
    send 5 tokens with random `kid`s. Expect all to be 401 and the hit
    count to be at most 2.
  - `TestJWKS_Unavailable`: the JWKS server is closed before the first
    request. Expect `503 jwks_unavailable`, `next` not called, and
    `Middleware` returning no error at construction.
  - `TestErrorBodyShape`: the body is valid JSON with exactly the keys
    `error` and `error_description`, and the content type is
    `application/json`.
- [ ] **Step 3:** Run `go test ./pkg/inboundauth/ -v`. Expected: FAIL.
- [ ] **Step 4:** Implement `errors.go` (an unexported `authError{status,
  code, desc}` plus `write(w)`), `keys.go` (the JWKS source with the rate
  limit), `verify.go` (algorithm allowlist, then key lookup, then signature,
  then time, issuer, audience, scope, caller, in that order, so the first
  failing check decides the code), and `middleware.go`.
- [ ] **Step 5:** Run `go test -race ./pkg/inboundauth/ -v`. Expected: PASS.
- [ ] **Step 6:** Commit: `feat(inboundauth): verify bearer JWTs against JWKS with stable deny codes`.

### Task 4: Package docs and the isolation check

**Files:**

- Modify: `pkg/inboundauth/doc.go`
- Create: `pkg/inboundauth/example_test.go`

- [ ] **Step 1:** Write `ExampleMiddleware` (a compiling example: build the
  config, wrap a mux, read `ClaimsFrom` in a handler). In `doc.go`, document
  the threat model (bearer passthrough), the error codes table, and the
  "token is stripped" guarantee.
- [ ] **Step 2:** Add `TestNoInternalImports`. Use `go/build` or
  `golang.org/x/tools/go/packages` (prefer `go list` via `os/exec` if you
  want to avoid a new dependency) and assert that no import path contains
  `/internal/`.
- [ ] **Step 3:** Run `make fmt lint test`. Expected: PASS.
- [ ] **Step 4:** Commit: `docs(inboundauth): add package docs and usage example`.
