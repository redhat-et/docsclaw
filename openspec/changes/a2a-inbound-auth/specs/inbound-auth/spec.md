## ADDED Requirements

### Requirement: Every inbound request is authenticated

When inbound auth is enabled, the middleware SHALL require exactly one
`Authorization: Bearer` header before any protected handler runs. The scheme
SHALL be matched case-insensitively and the token trimmed of surrounding
whitespace.

#### Scenario: Missing token is rejected

- **WHEN** a request has no `Authorization` header
- **THEN** the response SHALL be `401` with `error` `missing_token` and a
  `WWW-Authenticate: Bearer realm="docsclaw", error="missing_token"` header

#### Scenario: Malformed authorization is rejected

- **WHEN** a request has two `Authorization` headers, a non-Bearer scheme, or
  an empty token
- **THEN** the response SHALL be `401` with `error` `malformed_authorization`

#### Scenario: Lowercase bearer scheme is accepted

- **WHEN** a valid token is sent with the scheme `bearer`
- **THEN** the request SHALL be authenticated

### Requirement: Only ES256 and RS256 are accepted

The middleware SHALL check the `alg` header against an allowlist before
selecting a key. `none` and every `HS*` algorithm SHALL be rejected.

#### Scenario: Unsigned token is rejected

- **WHEN** a token carries `alg: none`
- **THEN** the response SHALL be `401` with `error` `unsupported_algorithm`

#### Scenario: HMAC confusion is rejected

- **WHEN** a token carries `alg: HS256` and is signed with the issuer's RSA
  public key bytes as the HMAC key
- **THEN** the response SHALL be `401` with `error` `unsupported_algorithm`

### Requirement: Signatures are verified against the issuer JWKS

The middleware SHALL resolve the signing key by `kid`, verify the signature,
and accept a token only when the key is present in the cached or freshly
fetched JWKS. A token without `kid` SHALL be rejected even when the JWKS has a
single key.

#### Scenario: Invalid signature is rejected

- **WHEN** a token's payload is tampered with or its signature does not verify
- **THEN** the response SHALL be `401` with `error` `invalid_token`

#### Scenario: Unknown key id is rejected

- **WHEN** a token names a `kid` that is absent after a permitted refresh
- **THEN** the response SHALL be `401` with `error` `invalid_token`

#### Scenario: Unavailable JWKS fails closed

- **WHEN** the JWKS cannot be fetched and no cached key matches
- **THEN** the response SHALL be `503` with `error` `jwks_unavailable` and the
  protected handler SHALL NOT run

### Requirement: JWKS refresh does not block verification

The middleware SHALL serve cached keys while a refresh is in flight, SHALL run
at most one refresh at a time, and SHALL limit refreshes to one per
`MinRefreshInterval` (default 60 seconds). An unknown `kid` received during an
in-flight refresh MAY fail closed rather than wait.

#### Scenario: Cached key is served during refresh

- **WHEN** a refresh is in flight and a request presents a cached `kid`
- **THEN** the request SHALL be verified without waiting for the refresh

#### Scenario: Refresh is rate-limited

- **WHEN** several unknown-`kid` tokens arrive within `MinRefreshInterval`
- **THEN** at most one JWKS fetch SHALL occur

### Requirement: Time claims are validated with skew

The middleware SHALL require `exp` and SHALL reject a token whose `exp` is in
the past beyond `ClockSkew` (default 30 seconds), or whose `nbf` or `iat` is in
the future beyond the skew.

#### Scenario: Expired token is rejected

- **WHEN** `exp` is earlier than now minus the skew
- **THEN** the response SHALL be `401` with `error` `token_expired`

#### Scenario: Not-yet-valid token is rejected

- **WHEN** `nbf` or `iat` is later than now plus the skew
- **THEN** the response SHALL be `401` with `error` `token_not_yet_valid`

#### Scenario: Small clock skew is tolerated

- **WHEN** `exp` is 20 seconds in the past and the skew is 30 seconds
- **THEN** the request SHALL be authenticated

### Requirement: Issuer and audience are enforced

The middleware SHALL require `iss` to equal the configured issuer and `aud` to
contain the configured audience. `aud` MAY be a string or an array.

#### Scenario: Wrong issuer is rejected

- **WHEN** `iss` does not equal the configured issuer
- **THEN** the response SHALL be `401` with `error` `invalid_issuer`

#### Scenario: Wrong audience is rejected

- **WHEN** `aud` does not contain the configured audience
- **THEN** the response SHALL be `403` with `error` `invalid_audience`

#### Scenario: Audience as a string is accepted

- **WHEN** `aud` is a plain string equal to the configured audience
- **THEN** the request SHALL be authenticated

### Requirement: Scope is enforced

The middleware SHALL accept scopes from a space-delimited `scope` string or an
`scp` array, and SHALL require at least one configured scope.

#### Scenario: Missing scope is rejected

- **WHEN** none of `RequiredScopes` is present
- **THEN** the response SHALL be `403` with `error` `insufficient_scope` and a
  `WWW-Authenticate` header containing the required scopes

#### Scenario: Array scope is accepted

- **WHEN** `scp` is an array containing a required scope
- **THEN** the request SHALL be authenticated

### Requirement: Caller is enforced

The middleware SHALL use `azp`, falling back to `client_id`, and compare it to
`AllowedCallers`. An entry ending in `/` SHALL match as a prefix; every other
entry SHALL match exactly.

#### Scenario: Disallowed caller is rejected

- **WHEN** `azp` is not permitted by a non-empty `AllowedCallers`
- **THEN** the response SHALL be `403` with `error` `caller_not_allowed`

#### Scenario: Prefix entry matches a sandbox identity

- **WHEN** `AllowedCallers` contains `spiffe://openshell.local/openshell/sandbox/`
  and `azp` is `spiffe://openshell.local/openshell/sandbox/bob`
- **THEN** the request SHALL be authenticated

#### Scenario: Prefix without slash does not match

- **WHEN** an entry is `spiffe://openshell.local/openshell/sandbox` and `azp`
  is `spiffe://openshell.local/openshell/sandboxes/x`
- **THEN** the response SHALL be `403` with `error` `caller_not_allowed`

### Requirement: Tokens never leave the middleware

On success, the middleware SHALL delete `Authorization`, store the verified
claims in the request context, and then call the next handler. Exempt paths
SHALL skip verification but MUST also delete `Authorization`. The package MUST
NOT log the token at any level and MUST NOT place it in `Claims.Raw`.

#### Scenario: Protected handler sees no authorization header

- **WHEN** a valid token is accepted
- **THEN** the next handler SHALL see an empty `Authorization` header and the
  verified claims in its context

#### Scenario: Exempt path still strips the token

- **WHEN** a request to an exempt path carries the header `Authorization: Bearer secret`
- **THEN** the handler SHALL see an empty `Authorization` header

#### Scenario: Exempt paths match exactly

- **WHEN** `/health` is exempt and a request targets `/healthz` or
  `/health/../a2a`
- **THEN** the request SHALL require authentication

### Requirement: Configuration is validated at construction

`Middleware` SHALL return an error at construction when the issuer, JWKS URL,
or audience is missing or when the JWKS URL is not an absolute `http`/`https`
URL. An unreachable JWKS SHALL NOT cause a construction error.

#### Scenario: Relative JWKS URL is rejected

- **WHEN** `JWKSURL` is `/.well-known/jwks.json`
- **THEN** `Middleware` SHALL return an error

#### Scenario: Unreachable JWKS still constructs

- **WHEN** `JWKSURL` is valid but unreachable
- **THEN** `Middleware` SHALL return a handler and requests SHALL fail with
  `503 jwks_unavailable`
