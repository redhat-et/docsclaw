# DocsClaw harness comparison and gap analysis

## Overview

This document compares DocsClaw's agent harness architecture against
three other agentic systems analyzed in the companion document
*AI Agent Harness Architecture*:

- **sandbox_agent** (Kagenti/LangGraph) — structured plan-execute-reflect
  loop with three-tier permissions and Landlock isolation
- **deepagents** (LangChain) — middleware-composable framework with
  swappable sandbox backends
- **Claude Code Harness** (Anthropic) — reactive agentic loop with
  five-layer permissions and context budget management

DocsClaw occupies a distinct position: a **lightweight, Kubernetes-native
agent runtime** with ConfigMap-driven personality. It is not a
developer framework (deepagents), not a CLI productivity tool (Claude
Code), and not a multi-tenant platform (sandbox_agent). It is an
**deployable agent workload** — the smallest useful unit that can
receive work via A2A, execute it with tools, and return results.

## Architecture comparison

### Agentic loop

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Loop structure | Explicit graph: router-planner-executor-reflector-reporter | Implicit — LLM decides when done (recursion_limit=9999) | Three blended phases; LLM steers reactively | Bounded tool-use loop (`RunToolLoop`, max 10 iterations) |
| Planning | Versioned structured plan with replan counter | TodoListMiddleware injects write_todos tool | Emergent from reactive tool calls | None — direct tool execution per request |
| Reflection | Explicit reflector node decides continue/replan/done | None | Implicit via test output | None |
| Interruptibility | HITL interrupts via A2A input_required | HumanInTheLoopMiddleware | User can steer mid-execution | Not interruptible mid-loop |

**DocsClaw's position:** The simplest loop of the four — intentionally.
`RunToolLoop()` in `pkg/tools/loop.go` calls `CompleteWithTools()`,
executes returned tool calls, feeds results back, and repeats up to
`MaxIterations`. No planning, no reflection, no state graph. This
simplicity is a feature for phase 2's target use case: short,
focused tasks (summarize a document, fetch a URL, run a command).

**Gap:** For complex multi-step tasks, the lack of planning and
reflection means the agent may exhaust its iteration budget without
completing work. The 10-iteration default is conservative.

### Permission and safety

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Tool gating | allow/deny/HITL rules | FilesystemPermission path globs | Five-layer pipeline (hooks-deny-mode-allow-callback) | Tool allowlist in agent-config.yaml |
| Command filtering | Three-tier: settings.json + sources.json + Landlock | Not implemented for shell | Bash treated as atomic; deny rules absolute | Regex denylist: 20+ dangerous patterns (rm -rf, sudo, docker run, etc.) |
| Workspace isolation | Per-context directory on shared PVC | Backend-dependent | Working directory scoping + checkpoints | Symlink-aware prefix matching via `workspace.IsInsideWorkspace()` |
| HITL | Yes — A2A input_required | Yes — HumanInTheLoopMiddleware | Yes — permission mode prompts | No |
| Deny override | Deny always beats allow | N/A | bypassPermissions does NOT override deny | Regex denylist is hardcoded, cannot be bypassed |
| Web access control | Not described | None | None | AllowedHosts with dot-boundary validation |

**DocsClaw's position:** Two-tier security that is simple but effective:

1. **Tool allowlist** — `NewRegistry(allowedTools)` in
   `pkg/tools/registry.go` hides non-allowed tools entirely. The
   LLM never sees them in tool definitions.
1. **Command denylist** — `internal/exec/exec.go` applies regex
   matching against 20+ dangerous patterns before execution. Blocks
   `rm -rf /`, `sudo`, `docker run`, fork bombs, `git push --hard`,
   `mkfs`, `bash -i`, etc. This is not overridable.

Additionally, `web_fetch` enforces an optional host allowlist, and
all file operations validate workspace boundaries with symlink
resolution.

**Gap — no HITL:** DocsClaw has no mechanism for pausing execution
to ask a human for approval. For autonomous agent workloads on K8s
this is acceptable (the operator pre-approves via tool allowlist),
but it limits interactive use cases.

**Gap — no hooks:** A `Hook` interface exists in `pkg/tools/hooks.go`
with `BeforeToolCall()` but is not wired into the default execution
path. Hooks would enable external policy enforcement without code
changes — a pattern proven by Claude Code's hook architecture.

**Strength — hardened defaults:** DocsClaw's security posture is
stronger than deepagents (which has no shell permission enforcement)
and comparable to sandbox_agent for the patterns it covers. The
read-only root filesystem, dropped capabilities, and
non-privileged container in the K8s deployment manifests add a
defense-in-depth layer that the other systems lack at the
deployment level.

### Tool execution

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Shell execution | Sandboxed with timeout | Backend-dependent | Universal Bash tool | exec: 30s timeout, 50KB output cap, regex filter |
| File operations | file_read, file_write, grep, glob | Backend Protocol | Read, Write, Edit, Glob, Grep | read_file, write_file (workspace-scoped) |
| Web access | web_fetch | None built-in | WebSearch, WebFetch | web_fetch: host allowlist, 30s timeout, 100KB limit |
| Code intelligence | None | None | LSP-backed go-to-def, find-refs | None |
| Search | grep, glob | grep, glob | Glob, Grep | None built-in (available via exec) |
| Agent coordination | explore + delegate (four isolation modes) | task tool + async remote | Agent + SendMessage | fetch_document (A2A bridge to peer agents) |

**DocsClaw's position:** Six focused tools with hard resource limits.
Every tool has explicit timeouts and output caps, preventing runaway
resource consumption — important for shared K8s clusters.

**Gap — no file search tools:** DocsClaw lacks built-in `grep` and
`glob` tools. The agent can use `exec` with `grep`/`find` commands,
but this bypasses structured output and is subject to the command
denylist. Dedicated search tools would be more reliable and safer.

**Gap — no file edit tool:** DocsClaw has `read_file` and `write_file`
but no `edit` (patch/diff-based modification). The agent must read
the entire file, modify it, and write it back — error-prone for
large files and wasteful of context tokens.

### Context management

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Compaction | LLM Budget Proxy (HTTP 402) | Auto-compact at 85% context | Clear old outputs, then summarize | None |
| Durable instructions | N/A | AGENTS.md via MemoryMiddleware | CLAUDE.md survives compaction | system-prompt.txt (always prepended) |
| Token tracking | Budget enforcement | SummarizationMiddleware | First-class concern | None |
| Session persistence | A2A context ID + PostgreSQL | LangGraph thread state | JSONL, resumable | Chat TUI: string history prepended to each message |

**DocsClaw's position:** The most basic context management of the four.
The chat TUI (`internal/chat/model.go`) simply prepends prior turns
as a `[Conversation history]` text block. No token counting, no
compaction, no summarization. For short A2A request-response
interactions this is adequate. For longer conversations via the chat
TUI, context will eventually exceed the model's window.

**Gap — no context compaction:** This is the most significant
architectural gap for any future "long-running agent" use case.
When conversation memory features are added (as planned), context
management becomes critical.

**Strength — durable system prompt:** DocsClaw's `system-prompt.txt`
is always prepended, never compacted — functionally equivalent to
Claude Code's CLAUDE.md durability guarantee, though without the
explicit compaction-survival semantics since there is no compaction.

### Multi-agent architecture

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Protocol | A2A + in-process sub-graphs | LangGraph SDK | CLI sessions + Agent SDK | A2A (native) |
| Sub-agent spawning | explore + delegate (four modes) | Inline, pre-compiled, async remote | Agent tool + Agent Teams | None |
| Peer-to-peer | No | No | Yes (Agent Teams) | Yes (A2A message passing) |
| Isolation modes | in-process, shared-PVC, isolated pod, sidecar | Backend-dependent | Worktree isolation | Each agent is an independent pod |
| Coordination | Shared filesystem | State reducer | File-locked task list + mailbox | A2A JSON-RPC message/send |

**DocsClaw's position:** A2A-native from the start. Each DocsClaw
agent is an independent pod with its own identity, and agents
communicate via the A2A protocol (`internal/bridge/`). This is
the most Kubernetes-native multi-agent model of the four — no
shared filesystem, no in-process sub-graphs, just network
messages between pods.

The `DelegationContext` in `internal/bridge/delegation.go` carries
SPIFFE IDs for user/agent delegation, enabling identity-aware
agent-to-agent communication — a feature none of the other three
systems have.

**Gap — no sub-agent spawning:** DocsClaw cannot spawn child agents
dynamically. It can communicate with pre-deployed peer agents via
A2A, but it cannot create new agent instances to parallelize work.
sandbox_agent's four delegation modes and Claude Code's Agent tool
are more capable here.

**Strength — SPIFFE identity:** The delegation context with SPIFFE
IDs enables zero-trust agent-to-agent communication on service
meshes. This is production infrastructure that the other systems
don't address.

### Configuration and extension

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Configuration | Python code + settings.json + sources.json | deepagents.toml + AGENTS.md | CLAUDE.md + .claude/ directory tree | ConfigMap: system-prompt.txt + agent-card.json + agent-config.yaml |
| Personality | Hardcoded in graph | System prompt in AGENTS.md | CLAUDE.md | system-prompt.txt (ConfigMap-mounted) |
| Skill system | N/A | SKILL.md via SkillsMiddleware | SKILL.md in .claude/skills/ | SKILL.md in skills/ directory, loaded via load_skill tool |
| Tool extension | Add graph nodes/tools | Middleware composition | MCP servers, hooks | Register Tool interface in serve.go |
| LLM providers | LangChain model zoo | LangChain model zoo | Claude only (Anthropic) | Factory pattern: Anthropic + OpenAI-compatible (init-time registration) |
| Hot-reload | No | No | MCP servers can be added mid-session | No |

**DocsClaw's position:** The ConfigMap-driven personality model is
DocsClaw's defining characteristic. The same binary serves as a
code reviewer, document processor, or research assistant based
entirely on what text files are mounted. This is operationally
powerful: personality changes are a ConfigMap update and pod
restart, not a code change and rebuild.

**Strength — LLM provider flexibility:** DocsClaw supports multiple
LLM providers via the factory pattern, including OpenAI-compatible
endpoints (vLLM, LiteLLM, Ollama). Claude Code is locked to
Anthropic; sandbox_agent and deepagents use LangChain's model zoo
but require code changes to switch.

### Deployment

| Dimension | sandbox_agent | deepagents | Claude Code | DocsClaw |
| --------- | ------------- | ---------- | ----------- | -------- |
| Target | Kubernetes (Kagenti, Helm) | Local CLI, LangSmith, self-hosted | Local, Anthropic cloud, remote | Kubernetes/OpenShift |
| Protocol | A2A | ACP, MCP, A2A | CLI, Agent SDK | A2A |
| Container image | Multi-image (agent + gateway) | Not containerized by default | Not containerized | Single binary, 9Mi memory footprint |
| Multi-tenant | Yes (per-context workspace) | No | No | Yes (per-pod isolation, A2A context) |
| Resource footprint | Not published | 487Mi (OpenCode comparison) | Not published (desktop app) | 9Mi memory |

**DocsClaw's position:** The most lightweight and deployment-ready
of the four. A single Go binary in a minimal container image,
running at 9Mi memory. Kubernetes manifests ship with the project.
A2A protocol support means it integrates into any A2A-compatible
orchestrator without adapters.

## Summary of gaps and recommendations

### Critical gaps (address before production use)

| Gap | Impact | Recommendation |
| --- | ------ | -------------- |
| No context compaction | Long conversations will exceed model window | Implement token-aware summarization, similar to deepagents' SummarizationMiddleware; preserve system-prompt.txt as the durable layer |
| No HITL mechanism | Cannot pause for human approval on sensitive operations | Implement A2A `input_required` flow (sandbox_agent pattern); DocsClaw already speaks A2A, so the protocol support is there |

### Important gaps (address for feature parity)

| Gap | Impact | Recommendation |
| --- | ------ | -------------- |
| No planning/reflection | Agent may exhaust iterations on complex tasks | Add optional plan-execute-reflect mode in agent-config.yaml; keep the simple loop as the default for focused tasks |
| No file search tools (grep/glob) | Agent must use exec for search, which is less safe and structured | Add dedicated grep and glob tools following the Tool interface pattern |
| No file edit tool | Full file rewrites are error-prone and token-heavy | Add a patch-based edit tool that applies targeted changes |
| Hooks not wired | Hook interface exists but is unused | Wire `BeforeToolCall()` into `RunToolLoop()` to enable external policy enforcement |

### Nice-to-have gaps (future consideration)

| Gap | Impact | Recommendation |
| --- | ------ | -------------- |
| No sub-agent spawning | Cannot parallelize work dynamically | Consider an A2A-native delegation model: spawn a new Sandbox (via Agent Sandbox CRD) for sub-tasks |
| No code intelligence | No LSP-backed navigation | Low priority for server-side agents; more relevant if DocsClaw gains interactive use cases |
| No checkpoint/undo | File writes are not reversible | Consider git-based checkpointing within the workspace volume |

### DocsClaw strengths (preserve and leverage)

| Strength | Why it matters |
| -------- | -------------- |
| 9Mi memory footprint | Order of magnitude smaller than alternatives; critical for dense K8s deployments |
| ConfigMap-driven personality | Operational flexibility without code changes; enables fleet management |
| A2A-native with SPIFFE delegation | Production-ready zero-trust agent communication |
| Hardened K8s deployment | Read-only root, drop ALL capabilities, non-root user — defense in depth at the container level |
| Multi-provider LLM support | Not locked to a single vendor; supports on-prem models via OpenAI-compatible API |
| Skills as OCI artifacts (planned) | Enterprise trust chain for skill provenance — unique advantage over all three comparators |
| Agent Sandbox compatibility | Fits into the emerging K8s SIG standard for agent workloads |

## Positioning

DocsClaw is not trying to be a general-purpose agent framework.
Its value proposition is:

> **The smallest, most secure, most operationally flexible agent
> runtime you can deploy on Kubernetes.**

The gaps identified above should be addressed in priority order,
but always within this design philosophy. Adding a planning node
should not turn DocsClaw into a LangGraph clone. Adding context
compaction should not require a database. The right approach is
incremental enhancement of the bounded tool-use loop, keeping the
single-binary, ConfigMap-driven, A2A-native model intact.
