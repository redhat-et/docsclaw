# AI Agent Harness Architecture

## A Comparative Analysis of sandbox\_agent, deepagents, and Claude Code Harness

*April 2026*

This document analyzes three distinct approaches to building agentic AI systems: the **sandbox\_agent** from the Kagenti platform (kagenti/agent-examples\#126), **deepagents** from LangChain (langchain-ai/deepagents), and the **Claude Code Harness** from Anthropic. Each system targets a different point in the spectrum from infrastructure safety to developer ergonomics to human-AI collaboration.

## Part 1: sandbox\_agent (kagenti/agent-examples\#126)

### Overview

The **sandbox\_agent** is a LangGraph-based A2A agent built for the Kagenti platform. It is designed to safely execute arbitrary shell commands in multi-tenant Kubernetes environments, with hard per-context filesystem isolation, a three-tier permission system, and a structured plan-execute-reflect reasoning loop. The PR (by @Ladas) adds 17,684 lines across 47 files and includes 68 unit tests.

**Key capabilities:**

* Sandboxed shell execution with timeout enforcement  
* Per-context workspace isolation on a shared RWX PVC  
* Human-in-the-Loop (HITL) interrupts via A2A `input_required`  
* Structured plan → step → execute → reflect loop  
* Sub-agent delegation in 4 isolation modes  
* Per-node LLM overrides (different model per graph node)

### Architecture

The agent is structured as a LangGraph `StateGraph` with six named nodes:

| `router → planner → step_selector → executor ⇄ tools                         ↑                         ↓                     [replan]              reflector ⇄ tools                                                ↓                                          reporter → END` |
| :---- |

**`SandboxState`** carries: `context_id`, `workspace_path`, versioned `plan` / `plan_steps` / `plan_status` / `plan_version`, `current_step`, `step_results`, `iteration`, `replan_count`, `done`, and `skill_instructions`.

**Node responsibilities:**

| Node | Role | Tools available |
| :---- | :---- | :---- |
| router | Decides resume vs new task | — |
| planner | Produces versioned step-by-step plan | file\_read, grep, glob, file\_write, respond\_to\_user |
| step\_selector | Writes executor brief per step | — |
| executor | Runs the actual work | shell, file\_read, file\_write, grep, glob, web\_fetch, explore, step\_done |
| reflector | Verifies step outcome: continue/replan/done | file\_read, grep, glob (read-only) |
| reporter | Writes final answer | read-only tools |

### Three-Tier Permission System

Every shell command is evaluated against three independent enforcement layers before execution:

**Layer 1 — `settings.json` (runtime policy, operator-configurable):**

Rules take the form `type(prefix:glob)`. Evaluation order: **deny beats allow**; unmatched \= **HITL**.

Default deny rules block: `rm -rf /`, `sudo`, `mount`, `umount`, `chroot`, `nsenter`, `chmod 777`, `nc`, `ncat`, writes to `/etc/**`, reads of `/etc/shadow` and `/proc/**`.

The checker also handles compound operators (`&&`, `||`, `;`, `|`) — each segment is checked independently — and detects interpreter bypass patterns (`bash -c "..."` unwrapped and re-checked).

**Layer 2 — `sources.json` (capability manifest, baked into container image):**

Declares what the container *can* do: which package managers are enabled, blocked packages (e.g. `subprocess32`, `pyautogui`), allowed git remotes (fnmatch patterns), allowed/blocked web domains, and runtime limits (`max_execution_time_seconds`, `max_memory_mb`).

**Layer 3 — Linux Landlock (optional, `SANDBOX_LANDLOCK=true`):**

Kernel-level filesystem restriction via `landlock_ctypes`. No fallback — if Landlock is requested and unavailable, execution fails hard.

When a command returns **HITL**, the executor raises `HitlRequired`, the graph calls LangGraph's `interrupt()`, and the A2A turn returns `input_required` to the caller. The human approves/denies; the turn resumes without losing state.

### Per-Context Workspace Isolation

`WorkspaceManager` maps each A2A `context_id` to an isolated directory:

| `/workspace/<context_id>/   scripts/   data/   repos/   output/   .context.json` |
| :---- |

`.context.json` stores agent name, namespace, TTL, disk usage, and timestamps. `cleanup_expired()` removes directories past `created_at + ttl_days`. Parallel A2A contexts never share filesystem state.

### Sub-Agent Delegation

Two tools provide sub-agent capabilities:

* **`explore`** — read-only in-process sub-graph (grep, read\_file, list\_files). Shares parent workspace. Best for codebase research.  
* **`delegate`** — four isolation modes:

| Mode | Isolation | Use case |
| :---- | :---- | :---- |
| in-process | Shared graph, shared filesystem | Fast parallel tasks |
| shared-pvc | Separate pod, parent's PVC mounted | Parallel tasks sharing data |
| isolated | Separate pod via SandboxClaim | Full isolation |
| sidecar | New container in parent pod | Co-located work |

The LLM auto-selects the mode, or the caller can specify it.

### Budget Enforcement

`AgentBudget` enforces multi-scope resource limits:

| Scope | Parameter | Default |
| :---- | :---- | :---- |
| Per-message | `SANDBOX_MAX_ITERATIONS` | 100 |
| Per-message | `SANDBOX_MAX_WALL_CLOCK_S` | 3600s |
| Per-step | `SANDBOX_MAX_TOOL_CALLS_PER_STEP` | 10 |
| Per-session | LLM Budget Proxy (HTTP 402 on exhaustion) | `SANDBOX_MAX_TOKENS` |

The HITL interval (`SANDBOX_HITL_INTERVAL=50`) forces a human checkpoint every N iterations.

## Part 2: deepagents (langchain-ai/deepagents)

### Overview

**deepagents** (20,500+ stars, MIT) is a developer-facing framework from LangChain that makes it easy to build capable, long-running agents without implementing tool loops, context management, or sub-agent coordination from scratch. It is explicitly self-described as "primarily inspired by Claude Code." The core design decision: **"trust the LLM"** — the framework does not attempt to use the model as a security boundary; isolation is the backend's responsibility.

**Key capabilities:**

* Composable middleware stack over LangChain's `create_agent`  
* Swappable sandbox backends (local shell, LangSmith, Modal, Runloop, Daytona)  
* Three sub-agent forms: inline sync, pre-compiled, async remote  
* Automatic context compaction via `SummarizationMiddleware`  
* Provider profiles with per-model customizations  
* deepagents\_acp server — a custom compatibility layer for Claude Code and Cursor clients (deepagents' own protocol, not an Anthropic standard)

### Architecture

The single public entry point is `create_deep_agent(...)` in `deepagents/graph.py`, which assembles an ordered middleware stack and delegates to LangChain's `create_agent`. The resulting graph runs at `recursion_limit=9999` — the LLM decides when it is done.

The middleware contract exposes two hook points:

* `wrap_model_call` — intercept LLM requests (e.g. inject system prompt additions)  
* `wrap_tool_call` — intercept tool execution (e.g. check permissions)

**Default middleware stack (in evaluation order):**

| Position | Middleware | Purpose |
| :---- | :---- | :---- |
| 1 | `TodoListMiddleware` | Plan/todo tracking via `write_todos` tool |
| 2 | `SkillsMiddleware` (opt) | Load [SKILL.md](http://SKILL.md) files into system prompt |
| 3 | `FilesystemMiddleware` | Inject ls/read/write/edit/glob/grep/execute tools |
| 4 | `SubAgentMiddleware` | Inject `task` tool, compile sub-graphs |
| 5 | `SummarizationMiddleware` | Auto-compact context at 85% window |
| 6 | `PatchToolCallsMiddleware` | Normalize malformed tool calls |
| 7 | `AsyncSubAgentMiddleware` (opt) | Remote/background subagent tools |
| … | user middleware | Inserted here |
| n-3 | `_ToolExclusionMiddleware` | Strip provider-excluded tools |
| n-2 | `AnthropicPromptCachingMiddleware` | Unconditional (no-ops on non-Anthropic) |
| n-1 | `MemoryMiddleware` (opt) | Load [AGENTS.md](http://AGENTS.md) into system prompt |
| n-0 | `HumanInTheLoopMiddleware` | Pause on named tools for approval |
| last | `_PermissionMiddleware` | Filesystem ACL enforcement (must see all tools) |

### Backend Abstraction

Sandboxing is handled entirely by the backend, not the graph. Two protocols define the contract:

* **`BackendProtocol`** — file operations only (`ls`, `read`, `write`, `edit`, `glob`, `grep`, `upload_files`, `download_files`)  
* **`SandboxBackendProtocol`** — extends `BackendProtocol` with `execute(command, timeout)` for shell commands

All file operations in `BaseSandbox` are implemented by running Python scripts through `execute()`, so subclasses only need to implement `execute()`, `upload/download_files`, and `id`.

**Available backends:**

| Backend | Isolation | Notes |
| :---- | :---- | :---- |
| `StateBackend` | None — ephemeral LangGraph state | Default; no execution |
| `FilesystemBackend` | None — direct disk access | No execution |
| `LocalShellBackend` | **None** — raw subprocess on host | Dev/CLI only; documented as dangerous |
| `LangSmithSandbox` | Remote container | Production-grade |
| `ModalSandbox` | Modal container | Partner integration |
| `DaytonaSandbox` | Daytona workspace | Partner integration |
| `RunloopSandbox` | Runloop devbox | Partner integration |
| `CompositeBackend` | Mixed | Routes by path prefix |

A **QuickJS sandbox** provides an in-process JavaScript REPL (via `quickjs` Python binding) with no filesystem/network access unless Python callables are explicitly bridged in.

### Permission Model

`_PermissionMiddleware` is always **last** in the stack so it intercepts tools injected by all other middleware.

| `@dataclass class FilesystemPermission:     operations: list[FilesystemOperation]   # "read" | "write"     paths: list[str]                         # wcmatch glob patterns, must start with /     mode: Literal["allow", "deny"] = "allow"` |
| :---- |

Rule evaluation: **declaration order, first match wins, permissive default** (allow if no match). Path patterns use `wcmatch` with `BRACE | GLOBSTAR` flags. Canonicalization blocks traversal bypasses.

Two-phase enforcement:

* **Pre-check**: extract path from tool args, evaluate rules, return `permission_denied` ToolMessage on deny  
* **Post-filter**: after `ls`, `glob`, `grep`, strip denied paths from returned results before the model sees them

**Important gap:** `_PermissionMiddleware` raises `NotImplementedError` when the backend implements `SandboxBackendProtocol`. Shell `execute` is explicitly out of scope — documented as "not yet implemented."

Subagents **inherit** parent permissions by default. A subagent's `permissions` list **replaces** (does not merge) the parent's list.

### Multi-Agent Architecture

Three sub-agent forms:

**Inline sync (`SubAgent`):** A `TypedDict` spec compiled into a `CompiledStateGraph` at agent creation time. The `task` tool invokes it synchronously — parent blocks until the subagent returns. Shares the parent `backend` unless overridden. State keys `todos`, `structured_response`, `skills_metadata`, `memory_contents` are excluded from cross-agent state transfer.

**Pre-compiled (`CompiledSubAgent`):** Supply a pre-built `Runnable` directly. Must have `messages` in its state schema.

**Async remote (`AsyncSubAgent`):** Connects via `langgraph_sdk` to a remote Agent Protocol server. Exposes tools: launch, check, update, cancel, list. Tasks tracked in agent state via `AsyncTask` dict with `_tasks_reducer` merging.

A **default general-purpose subagent** is always present unless the user supplies a subagent named `general-purpose`.

### Context Management

`SummarizationMiddleware` auto-compacts the conversation when token usage exceeds a configurable fraction (default: 85%). Evicted messages are stored as markdown at `/conversation_history/{thread_id}.md` in the backend. A companion `SummarizationToolMiddleware` exposes a `compact_conversation` tool for demand-triggered compaction.

Output truncation: `FilesystemMiddleware` tracks aggregate tool output tokens against context window size; very large outputs are saved to files and summarized.

### Deployment

* **Local CLI** (`deepagents-cli`): Textual TUI application with `LocalShellBackend` (no sandboxing). Includes HITL approval UI.  
* **`deepagents deploy`**: Bundles `deepagents.toml` \+ `AGENTS.md` \+ `skills/` \+ `mcp.json` into a LangSmith Deployment with `LangSmithSandbox`.  
* **deepagents\_acp server: deepagents ships a package called deepagents\_acp which they label the "Agent Client Protocol" — this is deepagents' own terminology, not a protocol Anthropic documents publicly. AgentServerACP wraps a CompiledStateGraph and implements a streaming tool-call wire format (start\_tool\_call, update\_tool\_call, start\_edit\_tool\_call) that makes deepagents agents appear as compatible backends to Claude Code and Cursor clients. This is likely a reverse-engineered or inferred interface based on how the Claude Code CLI communicates with external agent processes over stdio/JSON.**

No Kubernetes support. No Helm charts. Infrastructure is entirely LangSmith / LangGraph Platform managed or self-hosted.

## Part 3: Claude Code Harness (Anthropic)

### Overview

The **Claude Code Harness** is Anthropic's agentic execution framework, the reference design that both `sandbox_agent` and `deepagents` explicitly cite as inspiration. The central metaphor is precise: Claude Code is the *harness* around the LLM — the LLM is the reasoning engine; the harness owns tool execution, context management, permission enforcement, and agent lifecycle. Users interact with it as a CLI, a desktop app, or via the Agent SDK.

**Key capabilities:**

* Universal Bash tool (any shell command) plus file, search, web, and LSP tools  
* Five-layer permission pipeline with unconditional deny rules  
* Checkpoint-based undo for all file edits  
* Subagents with per-agent model, tools, memory, and git worktree isolation  
* Agent Teams: experimental peer-to-peer multi-agent coordination  
* [CLAUDE.md](http://CLAUDE.md) as the durable instruction layer that survives context compaction  
* Lifecycle hooks for external policy enforcement

### The Agentic Loop

The loop blends three phases — **gather context**, **take action**, **verify results** — but the phases are not explicit graph nodes. The LLM decides, turn by turn, which phase it is in. The loop is reactive: each tool result informs the next decision, producing emergent planning from feedback rather than upfront structured plans.

The loop is also **interruptible** at any point. The user can steer mid-execution. This is not a batch system.

A simple question may only trigger gather-context. A bug fix cycles through all three phases repeatedly (read → edit → run tests → read test output → edit again → run tests → verify).

### Tool Execution Model

| Category | Tools |
| :---- | :---- |
| File operations | Read, Write, Edit |
| Search | Glob, Grep |
| Execution | Bash (any shell command: git, tests, servers, build tools) |
| Web | WebSearch, WebFetch |
| Code intelligence | LSP-backed: go-to-definition, find-references, type errors |
| Agent coordination | Agent (spawn subagent), SendMessage (teammate messaging) |

**Bash is a universal escape hatch.** "Any command you could run from the command line, Claude can too." The tool set expands via MCP and is augmented by LSP plugins for language-server-backed intelligence.

### Permission and Safety Architecture

Every tool call is evaluated through a **five-layer pipeline, in strict order:**

1. **Hooks** — custom code; exit code 2 blocks the operation and sends feedback to the model  
2. **Deny rules** (`disallowed_tools`) — absolute; cannot be bypassed even by `bypassPermissions`  
3. **Permission mode** — global gate: `bypass`, `acceptEdits`, `plan`, `dontAsk`, `auto`  
4. **Allow rules** (`allowed_tools`) — pre-approved tool list  
5. **`canUseTool` callback** — runtime programmatic approval

**Critical design invariants:**

* **Deny rules are unconditional.** `bypassPermissions` does not override `disallowed_tools`. Safety is not a single switch.  
* **`acceptEdits` mode** auto-approves filesystem mutations *within the working directory* only — other Bash commands still require approval. Path scoping is enforced.  
* **Plan mode** is a read-only gate: Claude can analyze and plan but cannot make changes. Enables a two-phase explore-then-execute workflow.  
* **Protected directories** (`/.git`, `/.claude`, `/.vscode`, `/.idea`, `/.husky`) always prompt for confirmation even in `bypassPermissions` mode.  
* **Permission mode is dynamically adjustable** mid-session via `set_permission_mode()`.

**Checkpoints** provide a second safety layer independent of permissions: every file edit is snapshotted before execution, enabling `Esc+Esc` rewind. Checkpoints are session-local and do not cover external side effects (databases, APIs, git pushes).

### Context Management

The context window is the central resource constraint. The harness manages it with several mechanisms:

**What loads into context at session start:**

* Conversation history (JSONL in `~/.claude/projects/`)  
* `CLAUDE.md` (project instructions — survives compaction)  
* Auto-memory (`MEMORY.md`, first 200 lines or 25KB)  
* Skill descriptions  
* MCP tool definitions (deferred — only names load; full schemas load on demand)

**Compaction:** when context fills, the harness clears older tool outputs first, then summarizes the conversation. Critical design insight: *"detailed instructions from early in the conversation may be lost during compaction"* — persistent rules belong in `CLAUDE.md`, not conversation history.

**Subagents as context garbage collection:** "Subagents get their own fresh context, completely separate from your main conversation. Their work doesn't bloat your context. When done, they return a summary. This isolation is *why* subagents help with long sessions."

**Session persistence:** JSONL in `~/.claude/projects/`, resumable via `--continue`, forkable via `--fork-session`.

### Multi-Agent Architecture

Three tiers of agent coordination:

**Tier 1 — Subagents (within a session):**

Each subagent has its own context window, a custom system prompt (`.claude/agents/<name>.md` with YAML frontmatter), a restricted tool set, an independent permission mode, optional persistent memory, and optional git worktree isolation (`isolation: worktree`).

Built-in subagents reveal model routing intent:

| Built-in | Model | Tools | Purpose |
| :---- | :---- | :---- | :---- |
| Explore | Haiku (fast/cheap) | Read-only | Codebase search |
| Plan | Default | Read-only | Planning without writes |
| General-purpose | Default | All | Complex multi-step work |

Model resolution order: `CLAUDE_CODE_SUBAGENT_MODEL` env var → per-invocation parameter → subagent frontmatter → parent model.

**Explicit constraint: subagents cannot spawn other subagents.**

**Tier 2 — Agent Teams (experimental, cross-session):**

True peer-to-peer coordination. Each teammate is a fully independent Claude Code session.

| Dimension | Subagents | Agent Teams |
| :---- | :---- | :---- |
| Communication | Results back to main only | Teammates message each other directly |
| Coordination | Main agent manages all work | Shared task list with self-coordination |
| Context | Within parent's lifetime | Independent sessions |
| Token cost | Lower (results summarized) | Higher (full instances) |

Infrastructure: **team lead** (main session), **teammates** (independent sessions), **shared task list** (file-locked to prevent race conditions), **mailbox** (async inter-agent messaging).

**Explicit constraints: no nested teams; only the lead can manage the team.**

**Tier 3 — MCP (external tool expansion):**

MCP servers connect the harness to external systems. Session-wide via `.mcp.json`; subagent-scoped via inline definition (connected at spawn, disconnected at finish). Context budget tip from the docs: *"To keep an MCP server out of the main conversation entirely, define it inline in a subagent rather than in `.mcp.json`."*

### Extension Model: Hooks

Hooks are event-driven shell callbacks in the agent lifecycle:

| Hook event | Trigger |
| :---- | :---- |
| `PreToolUse` | Before any tool call |
| `PostToolUse` | After any tool call |
| `Stop` | Agent finishes |
| `SessionStart` / `SessionEnd` | Session lifecycle |
| `UserPromptSubmit` | Before user message is processed |
| `TeammateIdle`, `TaskCreated`, `TaskCompleted` | Agent team events |

Hooks receive structured JSON on stdin. **Exit code 2 blocks the operation and sends feedback to the model.** Hooks are evaluated at layer 1 — before all other permission gates — making them the most powerful external policy mechanism. Hooks can be scoped to individual subagents via frontmatter.

### Filesystem-First Configuration

All configuration is file-system-native and markdown-readable:

| Artifact | Location |
| :---- | :---- |
| Project instructions | `CLAUDE.md`, `.claude/CLAUDE.md` |
| User memory | `~/.claude/MEMORY.md` |
| Subagent definitions | `.claude/agents/*.md` (YAML frontmatter \+ markdown) |
| Skills | `.claude/skills/*/SKILL.md` |
| Slash commands | `.claude/commands/*.md` |
| MCP servers | `.mcp.json` |
| Session history | `~/.claude/projects/` (JSONL) |

Scope hierarchy: org managed settings → CLI flag → project `.claude/agents/` → user `~/.claude/agents/` → plugin.

Agent definitions are **version-controllable** and human-readable — the docs explicitly recommend checking subagents into source control for team sharing.

### Deployment Environments

The agentic loop, tools, and permission model are **identical** across all three environments:

| Environment | Where code runs | Use case |
| :---- | :---- | :---- |
| Local | User's machine | Default, full tool access |
| Cloud | Anthropic-managed VMs | Offload tasks, work on remote repos |
| Remote Control | User's machine, controlled via browser | Web UI with local execution |

## Part 4: Three-Way Comparison

### 1\. Core Abstraction

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Primary abstraction | Explicit LangGraph graph (nodes, edges, state shape) | Middleware stack over opaque `create_agent()` | Harness is the agent — LLM is a component inside |
| User's entry point | Python class, deploy as A2A microservice | `create_deep_agent(...)` → `CompiledStateGraph` | CLI / Agent SDK `query()` |
| Graph visibility | Fully exposed and modifiable | Encapsulated — only hooks visible | Fully encapsulated |
| Customization model | Edit graph source directly | Middleware composition | Files: [CLAUDE.md](http://CLAUDE.md), skills, subagent `.md`, hooks |

### 2\. Agentic Loop Design

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Loop structure | Explicit nodes: router→planner→step\_selector→executor→reflector→reporter | Implicit — LLM decides when done (`recursion_limit=9999`) | Three blended phases (gather/act/verify); LLM steers reactively |
| Planning | Versioned, structured `plan_steps` with `replan_count` | `TodoListMiddleware` injects `write_todos` tool | No upfront plan — emergent from reactive tool calls |
| Reflection | Explicit `reflector` node decides continue/replan/done | None — LLM decides implicitly | Implicit — verified via Bash/test output |
| Step control | `step_selector` writes brief per step | None | None |
| Iteration cap | `SANDBOX_MAX_ITERATIONS=100` | Effectively none (9999) | Not published; compaction kicks in at context limit |

`sandbox_agent` has the most *structured* reasoning loop. Claude Code is the most *fluid* and interruptible. `deepagents` trusts the LLM to manage its own loop.

### 3\. Permission and Safety Architecture

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Permission model | allow/deny/HITL rules in `settings.json` | `FilesystemPermission` path globs (read/write only) | 5-layer pipeline: Hooks → Deny → Mode → Allow → canUseTool |
| Execute permissions | Three-tier rule evaluation per command | **Not implemented** (documented gap) | Permission mode gates all Bash; deny rules are absolute |
| Deny override | Deny always beats allow | N/A | `bypassPermissions` does NOT override deny rules |
| Runtime changeability | Operator-configured at deploy time | Defined per-agent at construction | Dynamically adjustable mid-session |
| Two-layer policy | `settings.json` (runtime) \+ `sources.json` (image baked) | None | `disallowed_tools` (absolute) \+ permission mode (contextual) |
| Command-level analysis | Splits compound operators; detects interpreter bypass | Dangerous-pattern checker (display only, not enforcement) | Bash treated as atomic |

`sandbox_agent` has the most granular *command-level* policy. Claude Code has the most *layered* safety architecture with unconditional deny rules. `deepagents` delegates safety entirely to the backend container.

### 4\. Sandboxing and Isolation

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Isolation unit | Per-A2A-context directory on shared PVC | Swappable backend (local shell, LangSmith, Modal, Runloop…) | Worktree isolation for subagents |
| Default isolation | Filesystem directory per context | None | Working directory scoping |
| Strong isolation | Linux Landlock (kernel-level) | Remote container (LangSmith/Modal/Runloop) | Anthropic-managed cloud VMs |
| Multi-tenancy | Built-in — context ID → workspace | Not a framework concern | Per-session/per-subagent scoping |
| Package/remote policy | `sources.json`: pip packages, npm, git remotes | None at framework level | None at framework level |

### 5\. Context Management

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Compaction strategy | LLM Budget Proxy (HTTP 402), iteration cap, wall-clock limit | SummarizationMiddleware auto-compacts at 85% context | Clear old tool outputs first, then summarize |
| Durable instructions | N/A (single-context per A2A turn) | [AGENTS.md](http://AGENTS.md) via MemoryMiddleware | `CLAUDE.md` — the canonical durable layer, survives compaction |
| Subagent as context GC | Sub-agents run isolated in-process or pods | `_EXCLUDED_STATE_KEYS` prevents state leakage | Primary motivation for subagents is explicitly context isolation |
| MCP tool context cost | N/A | Full tool schemas always load | Deferred loading — only names load, full schemas on demand |
| Session persistence | A2A context ID \+ plan store (PostgreSQL) | LangGraph Platform thread state | JSONL in `~/.claude/projects/`, resumable via `--continue` |

### 6\. Multi-Agent Architecture

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Sub-agent forms | explore (read-only), delegate (4 isolation modes) | Inline sync, pre-compiled, async remote | Subagents (within session), Agent Teams (cross-session) |
| Nesting | Not restricted | Not restricted | Explicitly disallowed at both tiers |
| Communication model | Task tool \+ shared filesystem | `task` tool; results as `Command(update=...)` | Subagents return summaries; teammates use shared task list \+ mailbox |
| Peer-to-peer agents | No | No | Yes — Agent Teams with direct inter-agent messaging |
| Model routing | Per-node (planner/executor/reflector can differ) | Per-subagent at construction | Per-subagent via frontmatter \+ env var |
| Coordination primitives | Workspace directories | State reducer merging | File-locked shared task list, async mailbox |

Claude Code is the only system with true peer-to-peer multi-agent coordination (Agent Teams). `sandbox_agent` has the richest *isolation modes* for spawning. `deepagents` has the cleanest async remote delegation model.

### 7\. Extension Model

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Extension mechanism | Add/replace graph nodes and tools | Middleware stack composition | Hooks, MCP servers, skills, subagent `.md` files |
| Hook events | N/A | `wrap_model_call` / `wrap_tool_call` per middleware | PreToolUse, PostToolUse, Stop, SessionStart/End, TeammateIdle, TaskCreated/Completed |
| Policy via hooks | N/A | First-class in middleware | Yes — hooks run before all permission gates; exit code 2 blocks |
| Config as files | `settings.json`, `sources.json` | `deepagents.toml`, `AGENTS.md`, `skills/` | `CLAUDE.md`, `.claude/agents/*.md`, `.claude/skills/`, `.mcp.json` |

### 8\. Deployment Target

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Target runtime | Kubernetes (Kagenti platform, Helm) | Local CLI, LangSmith managed, or LangGraph Platform self-hosted | Local, Cloud (Anthropic VMs), Remote Control |
| Protocol | A2A (Google Agent-to-Agent) | ACP (Claude Code/Cursor), MCP, A2A | CLI, Agent SDK |
| Infrastructure owned | Yes — PVC, pods, Istio, Keycloak | No — delegates to LangSmith or partner sandboxes | Partially — local is user-owned; cloud is Anthropic-managed |
| Multi-tenant by design | Yes | No | No (per-user sessions) |

### 9\. Design Philosophy

|  | sandbox\_agent | deepagents | Claude Code Harness |
| :---- | :---- | :---- | :---- |
| Security model | Defense-in-depth: deny rules \+ sources policy \+ Landlock | "Trust the LLM" — isolation is the backend's job | Layered with unconditional deny rules; plan mode for read-only exploration |
| Transparency | Graph is open source and editable | Graph opaque; middleware open | Loop internals opaque; configuration model fully open |
| Primary concern | Safe multi-tenant execution in shared K8s infra | Developer ergonomics; build capable agents quickly | Developer productivity; context budget management; safety for tool-executing AI |
| Planning style | Upfront structured plan with reflection | Reactive with todo list | Reactive — emergent from tool feedback |
| Target user | Platform operators deploying agent infrastructure | Individual developers building agentic apps | Software engineers using AI assistance day-to-day |

## Summary

These three systems sit at different points in a spectrum from *infrastructure safety* to *developer ergonomics* to *human-AI collaboration*:

**`sandbox_agent`** solves the *infrastructure* problem: how do you run arbitrary-code agents safely in a shared Kubernetes cluster, with hard execution boundaries, per-tenant workspace isolation, and a structured reasoning loop that doesn't go off the rails. It is the most opinionated about *what the agent does* (plan → execute → reflect) and *how it does it safely*. It is the only system with kernel-level Landlock isolation, command-segment analysis, and a two-file policy split between runtime and build-time capabilities.

**`deepagents`** solves the *developer ergonomics* problem: how do you build capable agents without implementing tool loops, context management, or sub-agent coordination from scratch. Safety is explicitly the backend's job; the framework focuses on composability. Its middleware model is the cleanest of the three for adding cross-cutting features (HITL, memory, summarization) without touching graph topology.

**Claude Code Harness** solves the *human-AI collaboration* problem: how do you make an LLM a useful engineering partner that can be trusted with real tools on a real machine. The central innovations are the layered permission model (especially unconditional deny rules), the `CLAUDE.md` durable instruction layer that survives context compaction, and the explicit treatment of context budget as a first-class architectural concern — not a workaround. Both `sandbox_agent` and `deepagents` describe being "inspired by Claude Code"; the harness is the reference design they are converging toward.

*Sources: kagenti/agent-examples\#126, langchain-ai/deepagents (GitHub), [code.claude.com/docs](http://code.claude.com/docs), [anthropic.com/engineering/managed-agents](http://anthropic.com/engineering/managed-agents)*

