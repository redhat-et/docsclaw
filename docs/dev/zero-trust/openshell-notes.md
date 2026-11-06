# OpenShell sandbox behavior: spike notes

Research for the DocsClaw zero-trust v2 migration (WP01).

- OpenShell source: `github.com/NVIDIA/OpenShell`, commit
  `9f1cdb4ef6ed6be9fd86507580f9656b61b9c754` (`git describe`:
  `sdk/go/v0.1.6-11-g9f1cdb4ef`, 2026-10-28), branch `main`.
- Method: source and documentation reading only. No sandbox was reachable
  (`openshell sandbox list` returned `connect error: transport error`), so
  every runtime observation below is marked **unverified**. Anything cited to
  a file path and line is read from that commit.
- Line numbers are from the commit above and will drift.

## Summary for DocsClaw

1. Egress mediation is transparent. OpenShell does not set `HTTP_PROXY` for
   the workload; it redirects the sandbox network namespace to its proxy. A Go
   client using `http.DefaultTransport` works unchanged and needs no proxy
   configuration.
1. Policy denial is an HTTP `403` with a JSON body whose `error` field is
   `policy_denied`, `ssrf_denied`, or a middleware code, plus
   `Content-Type: application/json` and `Connection: close`. For plain HTTP
   destinations a Go client can read it. For HTTPS destinations the plaintext
   denial arrives before the TLS handshake, so Go reports a TLS error instead
   and the body is not readable. **Unverified** at runtime.
1. A failed dynamic token exchange is a `502` with
   `error: token_grant_failed` and an `x-openshell-request-id` header. It is
   distinguishable from a policy denial.
1. `bearer-passthrough` forwards zero or one `Authorization` header unchanged.
   Requests without a token reach the application. The application must
   authenticate callers itself.
1. The application may listen on `127.0.0.1:<target_port>`; the supervisor
   connects through the isolation backend's loopback connector.
1. A sandbox image must be a standard Linux base with a non-root `USER` and a
   writable `/sandbox`. `sandbox exec` defaults to `bash -lc`, so either ship
   `bash` or pass `--no-login-shell`.
1. Provider credentials are opaque `openshell:resolve:env:<suffix>`
   placeholders in the credential's declared environment variable. The proxy
   resolves them in header values (including `Authorization: Bearer` and
   `x-api-key`), Basic auth, query parameters, URL path segments, and
   opt-in request bodies.

## Task 1: Egress interception and denial shape

### Interception model

Interception is transparent at the network layer, not through proxy
environment variables.

- Every outbound connection goes through the CONNECT proxy. The sandbox runs
  in its own network namespace with a veth pair; all traffic routes through
  the host-side veth IP `10.200.0.1`, where the proxy listens. A process that
  ignores proxy environment variables can still only reach the proxy
  (`docs/security/best-practices.mdx:52-62`).
- The supervisor removes ambient proxy routing from every mediated child. The
  list covers `ALL_PROXY`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`,
  `all_proxy`, `http_proxy`, `https_proxy`, `no_proxy`, `grpc_proxy`, and
  `NODE_USE_ENV_PROXY`
  (`crates/openshell-sandbox/src/process.rs:125-137`), and
  `strip_proxy_env` removes them before the child starts
  (`crates/openshell-sandbox/src/process.rs:293-295`). The test
  `transparent_mediation_removes_ambient_proxy_routing` asserts exactly that
  (`crates/openshell-sandbox/src/process.rs:2065-2090`).
- The only proxy configuration the supervisor accepts is an operator-owned
  upstream corporate proxy, delivered on the supervisor's command line
  (`--upstream-proxy` / `--upstream-no-proxy`), never through the sandbox
  environment
  (`crates/openshell-driver-podman/src/config.rs:130-150`,
  `crates/openshell-driver-podman/src/container.rs:3040-3090`).

**Answer:** interception is transparent (network namespace plus veth
redirect), not `HTTP_PROXY`-based. The sandbox sets no proxy environment
variables for the workload; it removes any it inherits. There is no
`NO_PROXY` value to account for in the default mode.

### Go `ProxyFromEnvironment` and loopback

Because no proxy variables are present, `http.ProxyFromEnvironment` returns
`nil` and Go dials the destination directly. The network namespace redirects
that connection to the proxy. The classic "Go never proxies localhost"
behavior is therefore irrelevant to egress: there is nothing to proxy
through.

Two loopback consequences remain:

1. Loopback egress destinations are always blocked by policy. `127.0.0.0/8`,
   `169.254.0.0/16`, and `0.0.0.0` cannot be authorized even with
   `allowed_ips` (`docs/security/best-practices.mdx:131-136`). A config that
   names `http://127.0.0.1:PORT` for an external service will be denied. Use
   the service's real hostname, or `host.openshell.internal` for a host-local
   service (`docs/how-it-works/inference.mdx:294-298`).
1. Connections to loopback inside the sandbox (the app talking to its own
   listener) do not leave the namespace and are not mediated.

### TLS termination and trust

The proxy auto-detects TLS by peeking the first bytes. On a TLS ClientHello it
terminates TLS with a per-sandbox ephemeral CA, which enables credential
injection and L7 inspection
(`docs/security/best-practices.mdx:118-126`).

The CA is injected into the workload's trust stores:

| Variable | Value |
| --- | --- |
| `NODE_EXTRA_CA_CERTS` | CA certificate |
| `DENO_CERT` | CA certificate |
| `SSL_CERT_FILE` | combined CA bundle |
| `REQUESTS_CA_BUNDLE` | combined CA bundle |
| `CURL_CA_BUNDLE` | combined CA bundle |
| `GIT_SSL_CAINFO` | combined CA bundle |

Source: `crates/openshell-sandbox/src/child_env.rs:167-181`. The paths in the
unit test are `/etc/openshell-tls/openshell-ca.pem` and
`/etc/openshell-tls/ca-bundle.pem`
(`crates/openshell-sandbox/src/child_env.rs:207-209`).

Go's `crypto/x509` reads `SSL_CERT_FILE` on Unix, so a Go client picks up the
per-sandbox CA without code changes. This matters for the LLM endpoint and for
the Phase 2 JWKS fetch.

`tls: skip` on an endpoint disables termination, credential rewriting, token
grant injection, and L7 inspection. Do not use it for DocsClaw endpoints that
need credential substitution.

### Policy denial shape

The CONNECT path evaluates OPA policy before `200 Connection Established` and
before any TLS handshake. A denial writes an HTTP response with a JSON body
(`crates/openshell-supervisor-network/src/proxy.rs:2795-2835`).

The response builder
(`crates/openshell-supervisor-network/src/proxy.rs:6978-7045`) produces:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
Content-Length: <n>
Connection: close

{"error":"policy_denied","detail":"CONNECT api.example.com:443 not permitted by policy","reason":"<policy detail>"}
```

The `reason` field is present only when the policy engine supplies one; it
names the binary or rule that caused the denial
(`docs/observability/logging.mdx:228-260`).

Error codes a Go client can observe:

| Code | Status | Meaning |
| --- | --- | --- |
| `policy_denied` | 403 | No policy rule allowed this host, port, or binary |
| `ssrf_denied` | 403 | Destination resolved to a blocked internal address |
| `middleware_denied` | 403 | A supervisor middleware denied the request |
| `middleware_failed` | 403 | A middleware failed closed |
| `credential_endpoint_mismatch` | 403 | A provider placeholder was used outside its bound endpoint |
| `upstream_unreachable` | 502 | The proxy could not reach the upstream (includes DNS failure) |
| `token_grant_failed` | 502 | A dynamic token grant failed |
| `unsupported_l7_protocol` | 403 | The endpoint demanded inspection an unsupported protocol cannot provide |

For L7 REST rule denials the body also carries `method`, `path`,
`rule_missing`, and `next_steps`
(`docs/observability/logging.mdx:228-260`).

**What a Go client sees, by scheme:**

- `http://` destination: a normal `403` response. Go parses it; the JSON body
  is readable. This is the path the v2 document services use
  (`http://document-<dept>...:8080`).
- `https://` destination: the client has already sent a TLS ClientHello when
  the denial is written. The proxy sends the plaintext `403` where a TLS
  ServerHello is expected. Go's `crypto/tls` raises a record-header error and
  `net/http` surfaces `http: server gave HTTP response to HTTPS client`. The
  JSON body is not readable on this path. **Unverified** (source-derived; no
  sandbox). Verify with the probe in "Unverified items".
- The proxy is the only reachable endpoint in the namespace, so a policy
  denial does not present as `connection refused` or a `proxyconnect` error.
  The `proxyconnect` wording only appears when a client is configured with an
  explicit proxy and speaks CONNECT to it.

### Failed token exchange

A failed dynamic token grant returns `502 Bad Gateway` with
`error: token_grant_failed`, `detail: dynamic token grant failed`, and an
`x-openshell-request-id` response header
(`crates/openshell-supervisor-network/src/proxy.rs:6999-7018`,
`crates/openshell-core/src/token_grant_failure.rs:23`). The body never states
the cause; the request ID correlates the response with supervisor and gateway
logs (`docs/how-it-works/providers/profiles.mdx:661-690`).

This is distinguishable from a policy denial by status (`502` versus `403`)
and by the `error` code. It is also distinguishable from `upstream_unreachable`
despite the shared status.

## Task 2: Inbound service exposure

### Listen address

The application must listen on the declared target port, and loopback is
sufficient. The documentation's example is
`python -m http.server 8080 --bind 127.0.0.1`
(`docs/how-it-works/sandboxes/overview.mdx:495-505`). The supervisor reaches
the sandbox through the isolation backend's loopback connector (RFC 0017), and
`build_upstream_request` rewrites the `Host` header to
`127.0.0.1:<target_port>` before forwarding
(`crates/openshell-server/src/service_routing.rs:973-1013`).

Binding `0.0.0.0` also works but exposes the port to every interface inside
the sandbox. Bind `127.0.0.1`.

### Authorization modes and header handling

Default mode strips `Authorization`. `--authorization-mode bearer-passthrough`
forwards one valid bearer credential unchanged
(`docs/how-it-works/sandboxes/overview.mdx:580-620`).

Validation, read from `crates/openshell-server/src/service_routing.rs`:

1. Zero `Authorization` headers: allowed, and the request reaches the
   application with no `Authorization` header. The application must return
   its own authentication response.
1. Exactly one `Authorization` header: it must consist of the Bearer scheme
   (case-insensitive) and a non-empty token68 value. Otherwise the request
   fails with `400 Bad Request` (`validate_application_authorization`,
   `:1094-1135`).
1. More than one `Authorization` header: `400 Bad Request`.

Header handling when forwarding:

- `Authorization` is forwarded unchanged only in bearer-passthrough mode. In
  strip mode it is removed (`is_gateway_auth_header`, `:1137-1147`).
- Gateway and edge identity headers are always removed:
  `cf-access-jwt-assertion`, `x-forwarded-client-cert`, `x-ssl-client-cert`,
  `x-client-cert` (`:1137-1147`).
- Gateway authentication cookies are removed (`sanitize_cookie_header`,
  `:1148-1160`).
- Hop-by-hop headers are removed; WebSocket upgrade headers are preserved for
  WebSocket requests (`:977-1005`).
- The proxy adds no identifying headers of its own. It rewrites `Host` to
  `127.0.0.1:<target_port>` and otherwise forwards the request line and
  headers.

The practical consequence for DocsClaw: never trust any header other than the
one `Authorization` value the application itself validated. Any identity
header that a client sends has either been stripped or is client-controlled.

### Service URLs

The documented URL shape is
`https://<workspace>--<sandbox>--<service>.<gateway-base-domain>/`, for example
`https://default--sandbox-a--web.svc.openshell.internal/`
(`rfc/0017-sandbox-service-exposure/README.md`, "SNI routing and policy").

Two variations matter:

1. A loopback gateway returns local `openshell.localhost` URLs.
1. A gateway configured with a service routing name returns opaque hashed
   hostnames such as `https://<hash>.<base-domain>/`.

The documentation is explicit: consume the URL the CLI or SDK prints rather
than constructing hostnames from workspace, sandbox, and service names
(`docs/how-it-works/sandboxes/overview.mdx:668-690`). For the v2 demo, record
the printed URL in `a2a_peers.url` rather than composing it.

## Task 3: Sandbox image requirements

### Runtime requirements

From `examples/bring-your-own-container/README.md` and
`docs/how-it-works/sandboxes/overview.mdx`:

| Requirement | Detail |
| --- | --- |
| Base image | A standard Linux base. Distroless and `FROM scratch` are not supported. |
| User | Declare a non-root OCI `USER`. Use a named account, a numeric UID with a passwd entry for its primary GID, or a numeric pair such as `1500:1500`. An image without `USER` runs as UID and GID 1000. Policy can override with `process.run_as_user` and `process.run_as_group`. |
| Workspace | `/sandbox` is created by the driver for the selected identity. An image `WORKDIR` other than `/` or `/sandbox` becomes the workspace and must exist and be writable. |
| Entrypoint | The image `CMD` / `ENTRYPOINT` is replaced by the sandbox supervisor. Pass the main process explicitly on the create command. |
| Networking tools | None are required. OpenShell handles networking, including the CA trust injection. |
| Shell | `sandbox exec` runs the command through a login shell (`bash -lc`) by default, so the image needs `bash` for the default path. Use `--no-login-shell -- <binary>` to exec a command directly in an image without bash (`docs/how-it-works/sandboxes/overview.mdx:379-384`). |
| `sleep` | Needed if the harness uses the `-- sleep infinity` pattern to keep the sandbox alive. It is a coreutils/busybox binary, not provided by a shell alone. |

UBI micro has no shell, so either use a minimal base that includes `bash` and
`sleep`, or have the harness use `openshell sandbox exec --no-login-shell --
/usr/local/bin/docsclaw ...`. The image should place the binary at a stable
absolute path; `/usr/local/bin/docsclaw` matches the path provider profiles
will declare.

### Provider credential binding to binaries

Provider profiles authorize endpoints per binary
(`docs/security/best-practices.mdx:78-91`):

- Every `network_policies` entry requires a `binaries` list. Only listed
  binaries can reach the associated endpoints.
- Paths support glob patterns: `*` matches one path component, `**` matches
  recursively.
- Authorization may use the connection-owning executable or one of its
  executable ancestors.
- OpenShell pins every authorization-capable path to the first observed
  SHA256 digest (trust on first use). A later digest mismatch, missing
  integrity evidence, or conflicting evidence fails closed.
- Command-line paths are diagnostic only and never authorize access.

Two consequences for the DocsClaw image:

1. The binary path must match the provider profile's `binaries` entry exactly,
   or fall under its glob. A symlinked or wrapper path may authorize only if
   the pinned digest and ancestor rules admit it. Prefer a real path, not a
   symlink.
1. Replacing the binary at an authorized path invalidates the pin until the
   sandbox runtime restarts. Build the binary into the image rather than
   updating it inside a running sandbox.

The shipped example profiles list only `/usr/bin/curl` and
`/usr/local/bin/curl` (`providers/anthropic.yaml`,
`providers/nvidia.yaml`). A DocsClaw profile must list the DocsClaw binary
path, for example `/usr/local/bin/docsclaw`.

## Task 4: LLM provider placeholders

### Environment variables and placeholder format

Provider profiles declare the credential environment variable names. The
shipped profiles use:

| Provider | Env var | Placement |
| --- | --- | --- |
| Anthropic | `ANTHROPIC_API_KEY` | `auth_style: header`, `header_name: x-api-key` |
| OpenAI-compatible | `OPENAI_API_KEY` (profile-dependent) | `auth_style: bearer`, `header_name: authorization` |
| NVIDIA | `NVIDIA_API_KEY` | `auth_style: bearer`, `header_name: authorization` |

Sources: `providers/anthropic.yaml`, `providers/nvidia.yaml`;
placement semantics in `docs/how-it-works/providers/profiles.mdx:486`.

At startup OpenShell replaces the credential value with an opaque placeholder
in the workload environment. The observed format is
`openshell:resolve:env:<suffix>`, for example
`openshell:resolve:env:ANTHROPIC_API_KEY`
(`crates/openshell-sandbox/src/process.rs:1348-1362`,
`crates/openshell-sandbox/src/process.rs:1800-1808`). Revision-scoped
placeholders use a `v<digits>_` prefix in the suffix namespace, for example
`openshell:resolve:env:v1_EXTERNAL_TOKEN`
(`crates/openshell-supervisor/src/config_runtime.rs:1939`).

The client library does not need to understand the value. It only needs to
treat it as an opaque non-empty string and send it where the profile says.
For a server that does not authenticate, the documentation explicitly allows
any non-empty placeholder (`docs/how-it-works/inference.mdx:176`).

### Substitution locations

The proxy resolves placeholders in these parts of an HTTP request
(`docs/how-it-works/providers/overview.mdx:464-473`):

| Location | Example |
| --- | --- |
| Header value | `Authorization: Bearer <placeholder>`, `x-api-key: <placeholder>` |
| Basic auth header | `Authorization: Basic <base64(user:<placeholder>)>`; the proxy decodes, resolves, and re-encodes |
| Query parameter value | `GET /api?key=<placeholder>` |
| URL path segment | `POST /bot<placeholder>/sendMessage` |
| Request body (opt-in) | `{"api_key":"<placeholder>"}` when the REST endpoint sets `request_body_credential_rewrite: true` |
| WebSocket text frame (opt-in) | When the endpoint sets `websocket_credential_rewrite: true` |
| AWS SigV4 signing | When the endpoint sets `credential_signing: sigv4` |

Cookies, response content, WebSocket binary frames, and unsupported request
bodies are not rewritten.

Both header shapes the DocsClaw providers use are supported: `x-api-key` is
just a header value, and `Authorization: Bearer` is the bearer form. Dynamic
token grants support `bearer` and `header` placement only, not query or path
(`docs/how-it-works/providers/profiles.mdx:563-599`).

Fail-closed behavior:

- An unresolved, malformed, expired, or unknown placeholder is rejected rather
  than forwarded (`docs/how-it-works/providers/overview.mdx:498-505`).
- A placeholder used outside its bound endpoint returns `403
  credential_endpoint_mismatch` (`docs/how-it-works/providers/profiles.mdx:152-161`).

## Unverified items

None of the following was observed; each is source- or documentation-derived.
A reachable gateway and sandbox would settle them.

1. The exact Go error and status for an `https://` policy denial. Expected:
   `http: server gave HTTP response to HTTPS client` with no readable body,
   because the plaintext `403` precedes the TLS handshake. Probe: a two-line Go
   program using `http.DefaultTransport` to `https://denied.invalid`, run with
   `openshell sandbox exec`.
1. Whether the proxy sets any response header other than `Content-Type`,
   `Content-Length`, and `Connection` on the denial path. Source shows only
   those three.
1. Whether `x-openshell-request-id` appears on responses other than
   `token_grant_failed`. Source shows it only on that path.
1. The exact public service-URL hostname in the v2 deployment. It depends on
   gateway configuration and must be read from `openshell service list
   --output json`.

## Implications for the work packages

- WP04: keep `http.DefaultTransport` or a clone with
  `Proxy: http.ProxyFromEnvironment`. There are no proxy environment
  variables in the default mode, but the transport still works and stays
  correct if an operator upstream proxy is configured. No custom dialer is
  needed; the network namespace is the enforcement point.
- WP05: report `403` bodies by `error` code (`policy_denied`, `ssrf_denied`,
  `credential_endpoint_mismatch`) and `502` bodies by `error`
  (`upstream_unreachable`, `token_grant_failed`). For HTTPS targets, also map
  the Go string `http: server gave HTTP response to HTTPS client` to
  `blocked by sandbox network policy`.
- WP07: the image needs a non-root user, writable `/sandbox`, `/bin/bash` (or
  a documented `--no-login-shell` exec), `sleep`, and the binary at
  `/usr/local/bin/docsclaw`. The provider profile must list that path in
  `binaries`.
- WP10: bind `127.0.0.1` when inbound auth is enabled. That is sufficient for
  service exposure.
