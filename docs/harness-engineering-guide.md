# Agentic Harness Engineering Guide for DocsClaw

Condensed from *AI Harness Engineering Interview Preparation
Handbook* (2026 Edition) by AI Engineering Insider. Adapted for
the DocsClaw codebase.

## Core equation

```
Agent = Model + Harness
```

The model is a commodity you buy by the token. The harness is the
engineering you build, own, and differentiate on. A smarter model
does not save you from missing harness properties: determinism on
demand, auditability, permission enforcement, cost bounds,
adversarial robustness, and composition.

## The seven harness layers

Every production incident traces to a failure in one of these.

| #   | Layer              | What it owns                                  | DocsClaw location                                    |
| --- | ------------------ | --------------------------------------------- | ---------------------------------------------------- |
| 1   | Instruction        | System prompt, Skills, prompt versioning      | `system-prompt.txt`, `skills/`, `prompts.json`       |
| 2   | Tools              | Schemas, registry, dispatch, safety           | `pkg/tools/`, `internal/exec/`, `internal/webfetch/` |
| 3   | Memory & retrieval | Scratch, episodic, semantic memory; RAG       | `pkg/rag/`, context assembly in the loop             |
| 4   | Execution          | Sandbox, isolation, credential scoping        | `internal/exec/`, `internal/workspace/`              |
| 5   | Policy & approval  | Guardrails, permissions, approval gates       | `pkg/tools/hooks.go`, `agent-config.yaml`            |
| 6   | Observability      | Traces, cost tracking, dashboards             | `internal/metrics/`, slog in loop                    |
| 7   | Evaluation         | Golden datasets, regression gates, LLM judges | Not yet implemented                                  |

### Build order for new features

1. **Observability first** --- you cannot improve what you cannot
   see. Add tracing before adding capabilities.
2. **Execution** --- sandbox and isolation. DocsClaw already has
   workspace restrictions and exec timeouts.
3. **Tools + Instruction + Memory** --- iterate together. Start
   with two tools, a paragraph system prompt, and a scratchpad.
4. **Policy** --- dry-run defaults, approval gates on writes.
5. **Evaluation** --- by month two. Twenty golden cases, a
   regression gate in CI.

---

## Layer 1: Instruction engineering

### System prompt structure (8 components)

1. **Identity** --- one sentence: who is this agent
2. **Scope** --- what is in scope, what is out
3. **Inputs and outputs** --- shape of what agent receives and
   produces; output format is the most important line
4. **Tools** --- brief description of available tools, even though
   schemas are separate
5. **Refusal and escalation** --- what to do when out of scope or
   uncertain
6. **Constraints** --- what the agent must NOT do; concrete, paired
   with fallback, grounded with why
7. **Style** --- tone, length, citation format
8. **Failure-aware guidance** --- top 10 failure modes with
   prescribed responses

### Checklist

- [ ] System prompt versioned in source control, reviewed like code
- [ ] Structured-output contract specified (JSON schema), enforced
      at harness layer
- [ ] Skills declared with name, version, owner, scope, tool
      permissions
- [ ] Failure-aware sections listing top failure modes with
      prescribed responses
- [ ] Cached prompt prefix established; cache hit ratio
      instrumented
- [ ] Three context tiers separated: system (high trust), task
      (mixed), retrieved (low trust)
- [ ] Critical constraints repeated near end of context (counters
      lost-in-the-middle effect)
- [ ] Spotlighting applied to all untrusted/retrieved content:

```
<UNTRUSTED_DOCUMENT>
{{ retrieved_document_body }}
</UNTRUSTED_DOCUMENT>
```

### DocsClaw action items

- `system-prompt.txt` already serves as the instruction layer.
  Add the 8-component structure as a template.
- `pkg/skills/loader.go` handles Skill discovery. Add versioning
  and eval-suite metadata to `SKILL.md` frontmatter.
- Context assembly in the agentic loop should order: cached prefix
  -> task -> retrieved -> recent steps -> constraint reminders.

---

## Layer 2: Tool design

### Trust ladder (4 rungs)

| Rung | Type        | Example                  | Required safety                                               |
| ---- | ----------- | ------------------------ | ------------------------------------------------------------- |
| 1    | Pure        | String manipulation      | Input validation                                              |
| 2    | Read        | `web_fetch`, `read_file` | Scoped credentials, rate limits                               |
| 3    | Write       | `write_file`             | Idempotency, dry-run, approval gates                          |
| 4    | Destructive | `exec rm`, delete ops    | Dry-run default, mandatory idempotency, approval gates, audit |

### Schema design rules

1. **Verb-noun naming** --- `get_pipeline`, not `pipelineManager`
2. **Descriptions for the model** --- what it does (1 sentence),
   when to use it (1 sentence), critical caveats
3. **Enums where possible** --- closes off hallucinated arguments
4. **Mark all required fields** --- never ambiguous
5. **Describe every field** --- include expected shape, not just
   "the path"
6. **Safe defaults** --- `dry_run=true`, `timeout=30`,
   `recursive=false`
7. **No free-form string fields** --- constrain the type whenever
   possible

### Error messages for LLM recovery

```json
{
  "error": {
    "code": "file_not_found",
    "message": "No file at path 'src/main.go'.",
    "hint": "Call read_file with workspace-relative path. Try listing the directory first.",
    "retryable": false,
    "suggested_tools": ["read_file"]
  }
}
```

Fields: `code` (stable), `message` (readable), `hint` (recovery
action), `retryable`, `suggested_tools`.

### Separation principle

Never merge read and write in one tool. No `get_or_create`.
Reasons: auditability collapses, permission scope blurs, the
model abuses it speculatively.

### Checklist

- [ ] Tools classified by trust ladder (pure/read/write/destructive)
- [ ] Schemas with enums, patterns, required fields
- [ ] Descriptions written for LLM consumption
- [ ] Read and write operations separated
- [ ] Idempotency keys on every write tool
- [ ] Dry-run default true on destructive tools
- [ ] Structured error responses with `code`, `message`, `hint`,
      `retryable`, `suggested_tools`
- [ ] Approval gates for destructive and high-risk operations
- [ ] Per-call audit logging with principal, args, result, trace ID
- [ ] Tool result output truncated with summary and continuation
      token when above size threshold

### DocsClaw action items

- `pkg/tools/tool.go`: `ToolResult` currently has `Output` and
  `Error` (bool). Extend with structured error fields.
- `internal/exec/exec.go`: classify as destructive-rung tool. Add
  a deny-list check and structured errors.
- `internal/webfetch/`: classify as read-rung. Add SSRF
  allowlist (partially done via `allowed_hosts`).
- Add `dry_run` parameter to `write_file` tool with
  `default=true`.

---

## Layer 3: Memory and retrieval

### Three memory types

| Type     | Lifetime     | What it stores                               | Eviction                                      |
| -------- | ------------ | -------------------------------------------- | --------------------------------------------- |
| Scratch  | One run      | User request, tool results, agent reasoning  | Sliding window + summarization                |
| Episodic | Weeks-months | Past run outcomes, decisions, reflections    | TTL (90 days), archive + down-weight          |
| Semantic | Indefinite   | Domain facts, conventions, codebase topology | Change-detection hooks, re-validation on read |

### Context packing order

1. Cached prefix (system prompt, tool schemas, Skill template)
2. Task context (user request, attachments)
3. Retrieved context (documents, episodic memories) --- middle
4. Recent steps (last 3-5 tool calls, full fidelity) --- near end
5. Critical constraint reminders --- very end

### Eviction strategies

- **Sliding window** --- keep last N steps; drop older
- **Summarization chain** --- replace old steps with summaries
- **Selective retention** --- tag load-bearing results; keep
  verbatim
- **Map-reduce on tool results** --- summarize large outputs at
  retrieval time

### Memory poisoning defenses

- Write-time quality filters (reject malformed reflections)
- Source-of-truth preference (live platform data beats remembered
  claims)
- Provenance tags on every entry (authored/extracted/learned)
- Periodic memory audits
- Aggressive TTLs

### Checklist

- [ ] Three memory types distinguished with clear ownership
- [ ] Eviction strategy explicit per type
- [ ] Episodes structured (identity, outcome, decisions, evidence,
      reflection)
- [ ] Provenance tags on every memory entry
- [ ] Decay policy documented (TTL, change-detection, re-validation)
- [ ] Write-time quality filter rejects malformed reflections
- [ ] Source-of-truth preference: live data beats remembered claims
- [ ] Chunking strategy chosen per corpus type
- [ ] Hybrid retrieval (BM25 + dense) where corpus has identifiers
- [ ] Reranking applied before context injection
- [ ] Citations validated by output guardrail
- [ ] Retrieved content spotlighted as untrusted
- [ ] Access control enforced at retrieval layer

### DocsClaw action items

- `pkg/rag/` provides the semantic memory substrate (Weaviate).
  Add episodic memory: store structured episodes after each run.
- Implement context packing in the agentic loop
  (`pkg/tools/loop.go`): order messages per the packing strategy.
- Add summarization for old tool results when context grows long.

---

## Layer 4: Execution and sandboxing

### Trust ladder for sandboxing

| Rung | Isolation              | Use when                  |
| ---- | ---------------------- | ------------------------- |
| 1    | In-process             | Pure tools only           |
| 2    | Docker (default)       | Trusted code, development |
| 3    | gVisor (`runsc`)       | Semi-trusted code         |
| 4    | Firecracker microVM    | Untrusted code            |
| 5    | Separate cloud account | Maximum isolation         |

### Sandbox requirements

- Read-only root filesystem, tmpfs scratch, bind-mounted worktree
- Network egress: DNS sinkhole + proxy + kernel policy
- Short-lived credentials scoped to the run
- Per-sandbox budgets: wall-clock, CPU, memory, scratch space,
  output size, egress
- Kill-switch API with sub-second effect
- Per-sandbox traces emitted to observability

### Checklist

- [ ] Trust-ladder rung chosen against threat model, documented
- [ ] Untrusted code on Firecracker or equivalent; not default
      Docker
- [ ] Three-layer egress enforcement
- [ ] Filesystem: read-only root, tmpfs scratch, bind-mounted
      worktree, no host-sensitive mounts
- [ ] Short-lived credentials scoped to the run
- [ ] Per-sandbox budgets enforced
- [ ] Kill-switch API available
- [ ] Per-sandbox traces emitted

### DocsClaw action items

- `internal/exec/exec.go`: already has timeout and dangerous
  command blocking. Add output-size budget enforcement.
- `internal/workspace/workspace.go`: already restricts
  read/write to workspace. Document the threat model rung.
- For OpenShift deployments: configure `SecurityContext`
  (`runAsNonRoot`, `readOnlyRootFilesystem`, drop capabilities).

---

## Layer 5: Policy and guardrails

### Three guardrail positions

**Input guardrails** (before agent sees content):
- PII detection and redaction
- Prompt injection classifier
- Malicious-intent classifier
- Language and scope check
- Rate and quota check

**Inline guardrails** (between agent steps):
- Tool-call policy check
- Scope-drift detection
- Loop-break detection (duplicate calls)
- Budget enforcement (token, step, time)

**Output guardrails** (before user sees response):
- PII leakage check
- Secret-value detection
- Citation validation against retrieved set
- Cross-tenant content check
- Harmful-content classifier

### Prompt injection defenses (layered)

1. Spotlighting (wrap untrusted content in markers)
2. Instruction-hierarchy training (system > user > retrieved)
3. Injection classifiers (separate scanner)
4. Output guardrails for exfiltration patterns
5. Scope restriction on sensitive tools
6. Architectural isolation (exfiltration tools never see untrusted
   content)

### Approval triggers

- Tool class: any destructive tool
- Scope: any action affecting prod
- Scale: batch operations above threshold
- Amount: monetary actions above limit
- Novelty: first call with these args this session
- Low confidence: agent self-reported confidence below threshold

### Checklist

- [ ] Input guardrails: PII detection, injection classifier,
      scope check, rate limit
- [ ] Inline guardrails: tool-call policy, scope-drift, loop-break,
      budget enforcement
- [ ] Output guardrails: PII leakage, secret detection, citation
      validation, cross-tenant check
- [ ] Spotlighting on all retrieved content and tool results
- [ ] Architectural isolation: exfiltration tools separated from
      untrusted content
- [ ] Refusal calibrated --- over-refusal and under-refusal
      measured separately
- [ ] Policy engine for declarative rules (vary by tenant/region)
- [ ] Secrets: never in prompts, named references only, detection
      on output, rotation on suspected leak

### DocsClaw action items

- `pkg/tools/hooks.go`: `Hook` interface already provides
  `BeforeToolCall`/`AfterToolCall`. Extend to implement:
  - Duplicate-call detection (loop-break)
  - Tool-call policy checks (trust-ladder classification)
  - Budget enforcement per step
- Add PII redaction as a pre-processing step before context
  assembly.
- `agent-config.yaml`: add a `policy:` section for declarative
  guardrail rules.

---

## Layer 6: Observability

### Agent run as a trace

```
Root span: agent run
  ├── Context assembly
  ├── Step 1
  │   ├── LLM call (model, tokens, cost, cache hit)
  │   ├── Tool call: exec (args, result, duration)
  │   └── Guardrail evaluation
  ├── Step 2
  │   ├── LLM call
  │   └── Tool call: web_fetch
  └── Final outcome (completion/escalation/error)
```

### GenAI OpenTelemetry attributes

- `gen_ai.system`, `gen_ai.request.model`, `gen_ai.response.model`
- `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`
- `gen_ai.usage.cached_tokens`
- `gen_ai.tool.name`, `gen_ai.tool.input`, `gen_ai.tool.output`

### Cost metrics to track

- Cost per run (distribution, not just average; p95 is what kills
  budgets)
- Cost per successful outcome
- Cost per Skill
- Cost per user/tenant
- Cache hit ratio

### Anomaly detection

- Loop pattern (unusual tool-call ratio, repeating calls)
- Cost spike (top 0.1% of runs)
- Latency spike (top 1%)
- Guardrail cluster (burst of trips)
- Tool error clustering
- Model-behavior drift after version change

### Sampling and retention

- 100% capture, full fidelity, 7 days
- 100% capture, truncated, 30 days
- 1-5% sample, full fidelity, 12 months
- Failure-biased: all escalations/errors kept 90+ days

### Checklist

- [ ] OpenTelemetry GenAI semantic conventions emitted
- [ ] Trace per agent run with spans for each step, LLM call,
      tool call, guardrail event
- [ ] Prompt and completion captured with PII redaction; secrets
      dropped entirely
- [ ] Cost dashboards: per run, per success, cache hit ratio
- [ ] Anomaly detection: loops, cost spikes, latency spikes,
      guardrail clusters, tool errors, model drift
- [ ] User feedback signals linked to traces
- [ ] Retention tiered per policy

### DocsClaw action items

- `internal/metrics/metrics.go`: already has Prometheus metrics.
  Add OpenTelemetry trace exporter.
- `pkg/tools/loop.go`: already logs token usage per iteration.
  Emit as OTel spans with `gen_ai.*` attributes.
- Add per-run cost tracking: accumulate `input_tokens` and
  `output_tokens` across iterations, compute dollar cost.
- Add trace ID to all tool execution logs.

---

## Layer 7: Evaluation

### Eval suite components

1. **Golden dataset** --- 200-500 cases from production, experts,
   adversarial generation
2. **Scorers** --- exact match, LLM-as-judge, structured-field
   equality, test-suite pass rate
3. **Regression gates** --- thresholds below which a version cannot
   deploy
4. **Slice analysis** --- performance by Skill, user segment, task
   type; ship gate respects worst slice
5. **Adversarial cases** --- prompt injection, edge-case phrasings
6. **Confidence intervals** --- reported on every score

### Eval cadence

- **Smoke on PR** --- small subset, fast
- **Full nightly** --- complete golden dataset
- **Daily model-version check** --- detect provider updates
- **Production sampling** --- 1-5% scored by LLM judge

### Eval-driven debugging workflow

1. Failure surfaces in production
2. Reproduce in sandboxed trace replay
3. Diagnose root cause (nine failure categories below)
4. Fix
5. Add eval case that catches this failure

### Nine failure categories

1. Hallucinated actions (invents tool name/param/path)
2. Wrong tool choice
3. Tool misuse (right tool, wrong args)
4. Broken handoffs (multi-agent context loss)
5. Context poisoning (false info in memory/retrieval)
6. Timeout/latency failures
7. Cost blowouts
8. Scope creep
9. Policy violations (PII leak, unauthorized action)

### Checklist

- [ ] Golden dataset of 200-500 cases
- [ ] Slice analysis by Skill, user segment, task type
- [ ] Per-slice thresholds; ship gate respects worst slice
- [ ] LLM judges validated against human-scored set
- [ ] Pairwise scoring where possible
- [ ] Regression suite gating CI; threshold breaches block merge
- [ ] Cost per eval run tracked; batch APIs for non-realtime
- [ ] Tiered evals: smoke on PR, full nightly
- [ ] Confidence intervals on every score

### DocsClaw action items

- Create `testdata/golden/` with 20-50 initial eval cases.
- Add an `eval` subcommand that runs golden cases and reports
  scores.
- Wire eval into CI as a regression gate.

---

## Agent loop anti-patterns

### Loop detection (3 layers)

1. **Hard limits** (floor) --- step budget, token budget,
   wall-clock budget. On exceed: escalate with trace, never
   silently complete.
2. **Behavioral detection** --- duplicate tool calls (exact or
   near-duplicate), no-state-progress detection. Inject reflection
   prompt or escalate.
3. **Structural prevention** --- idempotency keys on writes,
   state-changing actions must advance state, forced exploration
   after N failed attempts.

### DocsClaw status

- `pkg/tools/loop.go` has `MaxIterations` (hard limit). Missing:
  token budget, wall-clock budget, duplicate-call detection.

---

## Verification loop (Guides + Sensors)

### Guides (feed-forward, before action)

- Spec files, conventions documents
- Type systems, schemas, linter configs
- Few-shot examples

### Sensors (feedback, after action)

- **Computational** (cheap, deterministic): linters, type
  checkers, schema validators, test runners
- **Inferential** (expensive, semantic): LLM-as-judge, AI code
  review, semantic lint

### Self-correction loop

1. Agent reads guides, produces output
2. Output passes computational sensors (lint, validate)
3. If passed: output passes inferential sensors (LLM review)
4. If sensors flag issues: agent re-plans with structured feedback
5. Bounded by retry budget; exhaustion escalates

### Approved-fixtures pattern

Tests and golden files are frozen. Agent can read, not modify.
Guard in pipeline fails if agent's diff touches frozen paths.
Prevents "fix by changing the test" failure mode.

### Checker messages for LLM consumption

```json
{
  "file": "main.go",
  "line": 42,
  "code": "E501",
  "severity": "error",
  "message": "Line exceeds limit.",
  "fix_hint": "Extract to a named variable.",
  "context": "result = someFunction(arg1, arg2, arg3, arg4)"
}
```

---

## Skills architecture

### Skill anatomy

- **Identifier** --- verb-noun: `pipeline.generate`, `pr.review`
- **Version** --- semver; breaking changes bump major
- **Input/output schema** --- typed, validated
- **Instruction template** --- prompt scaffold with parameters
- **Retrieved context policy** --- which index, reranker, top-k
- **Tool preferences** --- which tools this Skill needs
- **Examples** --- curated input/output pairs
- **Eval suite** --- regression tests specific to this Skill
- **Policy variables** --- inherited from harness
- **Deprecation state** --- active / deprecated / removed

### Skill discovery (two-stage)

1. Coarse classification: cheap model narrows 50 Skills to 3-5
2. Structured selection: full descriptions loaded, agent picks one

### Skill drift detection

- Daily re-runs of golden eval on current model version
- Production sampling: 1-5% scored by LLM judge
- Outcome signals: did user accept? Did PR land?

### DocsClaw status

- `pkg/skills/loader.go` discovers Skills from `SKILL.md` files.
- `SKILL.md` frontmatter has `name` and `description`. Add:
  `version`, `tools_expected`, `eval_suite`.

---

## MCP integration

### Three primitives

1. **Tools** --- agent-initiated actions with schemas
2. **Resources** --- server-owned data the client can read
3. **Prompts** --- server-registered prompt templates

### Checklist

- [ ] Servers connected via standard transports (stdio/SSE/HTTP)
- [ ] Capability manifest logged at session start
- [ ] Tokens scoped per tenant, per Skill, with short expiry
- [ ] Resource access subject to spotlighting before context
      injection
- [ ] Tool surface size monitored; per-Skill subsetting where
      surface exceeds 30+ tools

### DocsClaw status

- `internal/mcpclient/` implements MCP client with stdio and SSE
  transports. Logs capabilities at connect time. Add: token
  scoping, spotlighting on resource content, tool surface
  monitoring.

---

## Multi-agent design

### When to use multi-agent (need 2+ triggers)

- Roles genuinely separable (planner vs executor)
- Parallel exploration helps
- Permission boundaries are load-bearing
- Scale exceeds one context window
- Different subtasks benefit from different models

### Default: single agent with multiple Skills

For most tasks, a single well-harnessed agent outperforms
multi-agent on cost, latency, and debuggability.

### Handoff protocol requirements

- Structured context payload (not free text)
- Receiving agent acknowledges before proceeding
- Handoff depth limit; loop detection
- Authority declared per subtask
- Correlation ID across all agents

### DocsClaw status

- `internal/bridge/delegation.go` handles A2A agent delegation.
  Add: handoff depth limit, correlation ID propagation.

---

## Failure handling

### Recovery strategies (order of preference)

1. **Retry with reflection** --- structured feedback, re-plan,
   different approach
2. **Fallback to simpler approach** --- explicitly designed, not
   emergent
3. **Escalate to human** --- structured artifact with task,
   trajectory, failure mode
4. **Safe no-op** --- "I cannot confidently do this; consult X"

### Blast radius minimization

**Reversibility hierarchy:**
1. Draft human can discard (low risk)
2. Commit to branch, can roll back (medium)
3. Merged PR, production deploy (high)
4. Sent email, dropped database (catastrophic)

Design prefers earlier rungs. Drafts before commits. Commits
before deploys.

### Partially-applied state mitigations

- All-or-nothing coordination where possible
- Explicit rollback plans recorded before multi-step actions
- Checkpointing per step
- Escalation on partial state (not blind continuation)

### Checklist

- [ ] Retry with structured reflection as primary recovery
- [ ] Fallback paths explicitly designed
- [ ] Escalation as first-class action with structured artifact
- [ ] Safe no-op available when uncertainty exceeds threshold
- [ ] Loop-break detection at agent layer
- [ ] Reversibility ordering in planning
- [ ] Rollback plan recorded before multi-step actions
- [ ] Chaos-engineering harness with bad/truncated/delayed/poisoned
      injections

---

## Runtime engineering

### Cost control

- **Model routing** --- per-Skill default, tiered escalation
  (cheap for classification, expensive for reasoning)
- **Prompt caching** --- stable prefixes cached; cache hit ratio
  tracked
- **Batch APIs** --- for non-realtime workloads (evals, bulk
  enrichment)
- **Token budgets** --- per-step, per-run, per-tenant per-day
- **Concurrency caps** --- per-tenant, per-provider, per-tool

### Latency optimization

- Streaming responses
- Parallel tool execution (DocsClaw already does this in
  `pkg/tools/loop.go` via `errgroup`)
- Smaller model in critical path
- Prefetched context

### Fallback and degradation

- Fallback routing for provider outages (eval-tested)
- Graceful degradation modes: retrieval-degraded, read-only
- Provider health checks

---

## Production readiness gate

Use this as a gate before any DocsClaw deployment to production.

### Minimum viable (week one)

- [ ] System prompt versioned in source control
- [ ] At least 2 tools with proper schemas
- [ ] Observability: trace every LLM call and tool call
- [ ] PII redaction on input and output
- [ ] Step budget + token budget enforced
- [ ] 20-50 golden eval cases with regression gate
- [ ] Ship behind "draft mode" --- human reviews every output

### Production-ready (month three)

- [ ] Full 8-component system prompt
- [ ] All tools classified on trust ladder with appropriate safety
- [ ] Structured error messages on all tools
- [ ] Episodic memory with quality filters
- [ ] Semantic memory with decay/re-validation
- [ ] Input + inline + output guardrails
- [ ] Prompt injection defenses (spotlighting + classifier)
- [ ] OpenTelemetry traces with GenAI conventions
- [ ] Cost dashboards and budget alerts
- [ ] 200+ golden eval cases with slice analysis
- [ ] Chaos testing schedule
- [ ] On-call runbook for common incident classes
- [ ] Approval gates on all destructive operations
