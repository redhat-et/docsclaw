# WP05: Named document services and precise denials implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans.
> Read `00-index.md` (decisions 2-4) and `openshell-notes.md` (denial
> marker) first.

**Goal:** `fetch_document` takes `service` and `document_id`, resolves the
service from a `document_services` map, and tells the LLM exactly why a
request was refused.

**Architecture:** `internal/egress` gains denial description and call
logging, which WP11 reuses. `internal/fetchdoc` owns the tool and the HTTP
call. `serve.go` loses its inline `fetchDocument` closure.

**Tech stack:** Go stdlib, `log/slog`.

**Spec:** parent plan sections 1.3 and 1.4.

## Review focus

- A non-JSON error body (an HTML 502 from a proxy, or an empty 403): the
  result is still `403 <status text>`, never a decode panic or `""`.
- A huge error body: read at most 4 KiB before parsing.
- A `document_id` containing `/`, `..`, `?`, or `#` must not change the
  request path: use `url.PathEscape`, and assert this in a test.
- A service URL with a trailing slash produces no double slash.
- The `service` argument is missing or not a string, so the call is
  rejected before any network I/O.

---

### Task 1: Denial description in `internal/egress`

**Files:**

- Create: `internal/egress/denial.go`, `internal/egress/denial_test.go`

**Interfaces:**

- Produces: `func DescribeResponse(resp *http.Response) string`. It reads up
  to 4 KiB of the body and returns `"<code> <reason>"`, where `<reason>` is
  the JSON `error` field if present, else `http.StatusText(code)`. Examples:
  `"403 invalid_audience"`, `"502 Bad Gateway"`. If the response carries the
  sandbox-policy marker from WP01, it returns `"blocked by sandbox network
  policy"`.
- Produces: `func DescribeError(err error) string`. It returns `"blocked by
  sandbox network policy"` for the WP01 transport-level denial shapes
  (`proxyconnect` errors, connection refused to the proxy, or whatever WP01
  recorded), and `"request failed: <err>"` otherwise.
- Produces: `func LogCall(log *slog.Logger, tool, host string, status int,
  reason string)`. It logs at info with the keys `tool`, `host`, `status`,
  and `reason`. It takes no header or URL query arguments by design.

- [ ] **Step 1: Write a failing table test** `TestDescribeResponse` with
  these rows: 200 is not called (skip); `403 {"error":"invalid_audience"}`
  gives `403 invalid_audience`; `403 {"error":"insufficient_department_permissions"}`
  gives the same pattern; `404 {}` gives `404 Not Found`; `502` HTML body
  gives `502 Bad Gateway`; a 10 KiB body gives no hang and a status-text
  fallback; and a proxy-marker response gives `blocked by sandbox network
  policy`.
- [ ] **Step 2: Write a failing table test** `TestDescribeError`: a
  `*url.Error` wrapping a `proxyconnect` `*net.OpError` gives `blocked by
  sandbox network policy`; a plain DNS error gives a `request failed:`
  prefix; `context.DeadlineExceeded` gives a `request failed:` prefix.
- [ ] **Step 3:** Run `go test ./internal/egress/ -v`. Expected: FAIL.
- [ ] **Step 4:** Implement both functions and `LogCall`.
- [ ] **Step 5:** Run the tests. Expected: PASS. Commit:
  `feat(egress): describe HTTP and sandbox policy denials`.

### Task 2: `document_services` config and the new tool

**Files:**

- Modify: `internal/cmd/agentconfig.go` (add the field and validation)
- Rewrite: `internal/fetchdoc/fetchdoc.go`
- Create: `internal/fetchdoc/fetchdoc_test.go`

**Interfaces:**

- Produces: `AgentConfig.DocumentServices map[string]string`
  (`yaml:"document_services"`). `loadAgentConfig` rejects a key that does not
  match `^[a-z0-9][a-z0-9-]*$` or a value that is not an absolute
  `http`/`https` URL. The error names the key.
- Produces: `func fetchdoc.New(services map[string]string, client
  *http.Client, log *slog.Logger) tools.Tool`, with name `fetch_document`.
  Parameters: `service` (string, `enum` = sorted map keys) and `document_id`
  (string), both required. The description lists the service names.
- Behavior: `GET <base>/documents/<url.PathEscape(id)>` with `Accept:
  application/json`. On 200, decode and unwrap `document` as today. Results:
  OK `"**Document:** <title>\n\n<content>"`. Errors: `unknown document
  service "<name>"; available: a, b`, `<id>: <DescribeResponse>`, or `<id>:
  <DescribeError>`. Call `LogCall` once per network call.
- Removes: the `DocumentFetcher` type.

- [ ] **Step 1: Write failing tests** with `httptest.Server`:
  - `TestFetchDocument` table: ok; unknown service (assert the server got 0
    requests); missing `service`; missing `document_id`; `403
    invalid_audience` (result `IsError` and contains `403
    invalid_audience`); path-escape (`id = "../x?y"`; the server asserts
    `r.URL.EscapedPath() == "/documents/..%2Fx%3Fy"`); trailing slash in the base URL.
  - `TestFetchDocument_LogsWithoutHeaders`: a server that echoes. The test
    sets the client's request header `Authorization: Bearer leak` via a
    wrapping `RoundTripper`. Capture the slog output into a buffer and assert
    it contains `host=` and `status=200` but not `leak` and not
    `Authorization`.
  - `TestLoadAgentConfig_DocumentServices` table: valid; bad key `Eng`; bad
    URL `ftp://x`; relative URL.
- [ ] **Step 2:** Run the tests. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the tests. Expected: PASS. Commit:
  `feat(fetchdoc): named document services with precise denial reporting`.

### Task 3: Wire into serve, remove the old flag, update the demos

**Files:**

- Modify: `internal/cmd/serve.go`, `testdata/research-agent/agent-config.yaml`,
  `demo/batch/{finance,hr,security}/*.yaml`,
  `demo/batch/finance/deployment-template.yaml`,
  `demo/batch/dashboard/k8s.go`, `scripts/seed-demo-data.sh`

- [ ] **Step 1:** In `serve.go`, delete the `--document-service-url` flag,
  its binding, `Config.DocumentServiceURL`, its default, the startup log
  line, and the `fetchDocument` closure. Register
  `fetchdoc.New(agentCfg.DocumentServices, egress.NewClient(30*time.Second), log.Logger)`.
  Register it only when `len(DocumentServices) > 0`, and log
  `"fetch_document enabled", "services", <sorted keys>`.
- [ ] **Step 2:** Replace every `--document-service-url`/`DOCUMENT_SERVICE_URL`
  use in the listed demo and deploy files with a `document_services` entry
  in the matching agent config. Then confirm this prints nothing:

  ```bash
  grep -rn 'document-service-url\|DOCUMENT_SERVICE_URL\|document_service_url' \
    --exclude-dir=.git --exclude-dir=docs .
  ```

- [ ] **Step 3:** Run `make fmt lint test`. Expected: PASS.
- [ ] **Step 4:** Commit: `feat!: replace --document-service-url with document_services map`.
