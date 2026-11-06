## ADDED Requirements

### Requirement: One-shot ask command

`docsclaw ask` SHALL accept exactly one positional prompt, run the same
runtime setup as `serve`, print only the final answer to stdout, and
exit non-zero when the run fails.

#### Scenario: Answer goes to stdout

- **WHEN** `docsclaw ask --config-dir DIR "hi"` succeeds
- **THEN** stdout SHALL equal the answer plus a trailing newline, and
  stderr SHALL contain no answer text

#### Scenario: Missing provider fails loudly

- **WHEN** no LLM API key or provider is configured
- **THEN** the command SHALL exit non-zero with `LLM API key not
  configured` on stderr and SHALL print nothing to stdout

#### Scenario: Wrong argument count is a usage error

- **WHEN** `ask` is given zero or two positional arguments
- **THEN** the command SHALL return a usage error

### Requirement: Trace mode reports tool calls on stderr

With `--trace`, the command SHALL write one line before each tool call
and one line after it, both to stderr. The final answer SHALL still be
the only content on stdout.

#### Scenario: Tool calls appear in the trace

- **WHEN** the model calls `fetch_document` and then returns text with
  `--trace`
- **THEN** stderr SHALL contain `→ fetch_document` and
  `← fetch_document ok`, and stdout SHALL contain only the final text

### Requirement: Placeholder API keys are accepted

Both LLM providers SHALL accept any non-empty API key value without
format validation and SHALL send it unchanged to the configured base
URL.

#### Scenario: OpenAI-compatible endpoint with a placeholder

- **WHEN** the OpenAI provider is built with
  `APIKey: "openshell-placeholder-xyz"` and a base URL
- **THEN** the request SHALL carry `Authorization: Bearer
  openshell-placeholder-xyz` and SHALL use the `<base>/chat/completions`
  path

#### Scenario: Anthropic-compatible endpoint with a placeholder

- **WHEN** the Anthropic provider is built with a placeholder key
- **THEN** the request SHALL carry `x-api-key` with that value
