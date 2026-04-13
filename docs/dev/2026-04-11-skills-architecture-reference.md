# Skills Architecture Reference

## Source

**Talk:** "Don't Build Agents, Build Skills Instead"
**Speakers:** Barry Zhang & Mahesh Murag, Anthropic
**Event:** AI Engineer conference (2025)
**Video:** https://www.youtube.com/watch?v=CEvIs9y1uog

## Core thesis

A single general-purpose agent with code execution (bash + file system)
is sufficient for any domain. Domain specialization comes from
**skills** (organized folders of procedural knowledge), not from
building separate agents. MCP provides connectivity to external
systems; skills provide the expertise for using those connections
effectively.

## Emerging general agent architecture

| Layer | Role | DocsClaw equivalent |
|-------|------|---------------------|
| Agent loop | Manages context, token flow | `pkg/tools/` agentic loop |
| Runtime environment | File system, code execution | `internal/exec/`, `internal/readfile/`, `internal/writefile/` |
| MCP servers | External tools and data | A2A bridge (`internal/bridge/`), tool registry |
| Skills library | On-demand procedural knowledge | `pkg/skills/`, ConfigMap-driven config |

## Computing analogy (from the talk)

| Computing | AI equivalent |
|-----------|---------------|
| CPU | LLM model |
| Operating system | Agent runtime |
| Applications | Skills |

DocsClaw positions itself as the **OS layer**: a universal runtime that
orchestrates models, tools, and skills.

## What DocsClaw already implements

- **Progressive disclosure**: `Discover()` reads only frontmatter
  metadata; `BuildSummary()` shows a compact list; `LoadContent()`
  loads full content on demand via `load_skill` tool
- **Skills as folders**: each skill is a subdirectory with `SKILL.md`
  containing YAML frontmatter (`name`, `description`) and markdown
  instructions
- **Universal agent**: single binary configured via `system-prompt.txt`,
  `agent-card.json`, `agent-config.yaml`, and a skills directory
- **ConfigMap packaging**: `configmap-gen` generates Kubernetes
  ConfigMaps for both agent config and skills

## Enhancement opportunities from the talk

### 1. Scripts as tools within skills

The talk describes skills that include executable scripts (Python,
bash) as reusable tools. Currently DocsClaw skills are markdown-only.
Adding support for scripts inside skill directories would let skills
package their own tooling.

**Relevant code:** `pkg/skills/loader.go` — `LoadContent()` could
discover and register scripts in the skill directory.

### 2. Skill dependencies

Skills that explicitly declare dependencies on other skills or MCP
servers. This makes agent behavior more predictable across different
runtime environments.

**Possible approach:** extend `SKILL.md` frontmatter with `depends:`
field listing required skills or tools.

### 3. Skill testing and evaluation

Treat skills like software: automated testing that a skill triggers
correctly and produces expected output quality.

**Possible approach:** `SKILL_TEST.md` or test fixtures within skill
directories, run via `make test-skills`.

### 4. Skill versioning

Track how a skill evolves over time and how that affects agent
behavior. Git already provides versioning for the skill files;
the gap is surfacing version info at runtime.

**Possible approach:** add `version:` to frontmatter, expose in
`BuildSummary()`.

### 5. Agent-created skills

The talk emphasizes that agents should create skills for themselves
during use, making learning transferable across sessions. This is
described as "concrete steps towards continuous learning."

**Possible approach:** a `save_skill` tool that packages a successful
multi-step workflow into a new skill directory.

## Key quotes

> "We used to think agents in different domains will look very
> different. [...] The agent underneath is actually more universal
> than we thought."

> "MCP is providing the connection to the outside world while skills
> are providing the expertise."

> "Our goal is that Claude on day 30 of working with you is going to
> be a lot better than Claude on day one."
