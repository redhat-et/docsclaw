# Skill-puller sidecar design

**Status:** Draft
**Date:** 2026-05-30

## Problem

When DocsClaw runs on OpenShift, skills mounted via ConfigMaps or
OCI image volumes are read-only and safe from tampering. But if
the agent needs to **discover and pull skills dynamically** at
runtime, we need a mechanism that:

1. Lets the agent request new skills after the pod has started
1. Ensures pulled skills cannot be modified by the agent process
   (or by a malicious skill running inside it)
1. Verifies skill authenticity before making them available

## Approach

A lightweight sidecar container (`skill-puller`) runs alongside
the DocsClaw agent in the same pod. It owns the skills directory
and exposes a local HTTP API for search, verification, and pull.
The agent mounts the same directory **read-only**.

### Why a sidecar

We considered alternatives:

| Approach | Verdict |
| -------- | ------- |
| Landlock (kernel self-sandboxing) | Self-imposed — a compromised agent could skip it |
| SELinux MCS policies | Fragile on OpenShift, designed for inter-pod isolation |
| Agent-side signature check only | Detects tampering but doesn't prevent writes |
| CSI ephemeral driver | Heavy infrastructure investment for a simple problem |

The sidecar with a read-only volume mount is the simplest
Kubernetes-native solution. The enforcement is at the VFS mount
level — the agent kernel-enforced cannot write to the skills
path, regardless of what code runs inside it.

## Architecture

```text
┌─────────────────────────────────────────────┐
│ Pod                                         │
│                                             │
│  ┌──────────────┐     ┌──────────────────┐  │
│  │ docsclaw     │     │ skill-puller     │  │
│  │ (agent)      │     │ (sidecar)        │  │
│  │              │     │                  │  │
│  │ POST /skills │────▶│  verify (cosign) │  │
│  │  /pull       │     │  pull (multi-src)│  │
│  │              │     │                  │  │
│  │  reads from  │     │  writes to       │  │
│  │  /skills/dyn │     │  /skills/dyn     │  │
│  │  (read-only) │     │  (read-write)    │  │
│  └──────┬───────┘     └────────┬─────────┘  │
│         │                      │            │
│         ▼                      ▼            │
│     ┌──────────────────────────────┐        │
│     │ emptyDir: dynamic-skills     │        │
│     └──────────────────────────────┘        │
└─────────────────────────────────────────────┘
```

### Volume mounts

The sidecar and the agent share an `emptyDir` volume. The agent
mounts it read-only, the sidecar mounts it read-write. The mount
paths are independent — the agent's `--skills-dir` flag
determines where it looks for skills, and the Deployment spec
wires the volume to a subdirectory under that path. The sidecar
writes to its own mount path and does not need to know the
agent's configuration.

```yaml
volumes:
  - name: dynamic-skills
    emptyDir: {}
containers:
  - name: agent
    args: ["serve", "--skills-dir", "/skills"]
    volumeMounts:
      - name: dynamic-skills
        mountPath: /skills/dynamic
        readOnly: true
  - name: skill-puller
    volumeMounts:
      - name: dynamic-skills
        mountPath: /data/skills
        readOnly: false
```

### Directory layout

The agent's `--skills-dir` points to `/skills` in this example.
Static and dynamic skills coexist as sibling subdirectories.
Most agents (including DocsClaw and Claude Code) scan the skills
directory recursively, looking for subdirectories containing a
`SKILL.md` file. The sidecar writes into the shared volume and
the files appear alongside operator-mounted skills automatically:

```text
/skills/
├── hr/                          # ConfigMap or image volume mount
│   └── resume-reviewer/
│       └── SKILL.md
├── finance/                     # another static mount
│   └── expense-policy/
│       └── SKILL.md
└── dynamic/                     # emptyDir, written by sidecar
    ├── doc-summarizer/
    │   └── SKILL.md
    └── calendar-scheduler/
        └── SKILL.md
```

The `/skills/dynamic/` mount is read-only in the agent container
and read-write in the sidecar. All other mounts under `/skills/`
are read-only by their nature (ConfigMap, image volume).

### Skill hot-reload

The sidecar's responsibility ends at writing the skill files to
the shared volume. **Discovering newly available skills is the
agent's responsibility.** Agent runtimes vary in how they handle
this:

- Some agents watch the skills directory and pick up new skills
  immediately (inotify, polling)
- Some agents require a signal (e.g. SIGHUP) to re-scan
- Some agents only load skills at startup and require a restart

The sidecar does not attempt to notify the agent — it has no
knowledge of the agent's reload mechanism. If the agent runtime
supports a reload trigger, the operator can configure a
post-pull webhook in the sidecar or handle it via a shared
signal mechanism. This is intentionally left to the agent side.

## Sidecar API

The sidecar listens on `localhost:9100` (pod-internal only).

| Method | Path | Description |
| ------ | ---- | ----------- |
| POST | `/skills/pull` | Pull, verify, and install a skill |
| GET | `/skills/list` | List currently available dynamic skills |
| GET | `/healthz` | Liveness probe |

Search is **out of scope** for the initial implementation. Skill
search depends heavily on the registry backend (OCI catalog API,
Quay REST API, GitHub search, etc.) and there is no universal
search interface across these sources. For now, the agent or
operator provides the exact reference to pull. Search can be
added later per source type.

### Skill sources

The sidecar supports pulling skills from multiple source types.
The agent specifies the source and reference in the pull request:

```json
{"source": "url", "ref": "https://example.com/skills/summarizer/SKILL.md"}
{"source": "github", "ref": "org/repo/path/to/skill", "version": "v1.2.0"}
{"source": "oci", "ref": "quay.io/redhat-et/skills/summarizer:1.2.0"}
```

| Source | How it works |
| ------ | ------------ |
| `url` | HTTP GET to fetch a single `SKILL.md` file |
| `github` | Fetch from GitHub raw content API, with optional branch/tag |
| `oci` | Pull and unpack an OCI image containing the skill |

### Pull flow

1. Fetch the skill content from the specified source
1. Write to `/skills/dynamic/<skill-name>/`
1. Return skill metadata (name, description, source) to the agent

## Relationship to skillimage

The sibling project `skillimage` provides OCI-specific tooling
for skill images (build, push, pull, catalog indexing). The
sidecar is **not** part of skillimage — it lives in docsclaw
as a demo of the deployment pattern.

The sidecar is source-agnostic by design. For the `oci` source
type, it may reuse skillimage's pull logic (ORAS client, path
traversal protection, credential chain) either as a Go module
import or by vendoring the relevant code. The `url` and `github`
source types are simple HTTP fetches with no skillimage
dependency.

The sidecar does not use skillimage's `SkillCard` schema. Skills
are identified by the standard `SKILL.md` file, not by any
registry-specific metadata format.

## Future work

### Signature verification

The sidecar should eventually verify OCI image signatures before
unpacking (cosign/sigstore). This is deferred from the initial
implementation to focus on the core pull-and-serve pattern.

### Authentication

Private registries and GitHub repos require auth tokens. The
credential handling is straightforward (Docker/Podman credential
chain for OCI, GitHub token for `github` source) but is deferred
from v1 to keep the demo focused on the concept.

### Search

Skill search depends on the registry backend and has no universal
interface. Can be added per source type as needed.

## Agent-side skill for sidecar interaction

Some skill ecosystems use a "paste a prompt" installation
pattern where the agent downloads and writes skill files
directly. For example, Red Hat's [agentic skills page][rh-skills]
provides a bootstrap prompt that the user pastes into their
agent chat. The agent fetches a bootstrap skill from GitHub,
which then pulls the remaining skills. This works well on
developer workstations where the user is present and the agent
has filesystem write access.

In production on OpenShift, the agent cannot (and should not)
write skill files — the skills directory is read-only. Instead,
we provide a **`skill-pull` skill** that teaches the agent how
to interact with the sidecar's HTTP API. The agent uses `curl`
(or the built-in HTTP tool) to call the sidecar on localhost:

```text
# Pull a skill from a URL
curl -X POST http://localhost:9100/skills/pull \
  -d '{"source": "url", "ref": "https://example.com/skills/summarizer/SKILL.md"}'

# Pull from GitHub
curl -X POST http://localhost:9100/skills/pull \
  -d '{"source": "github", "ref": "org/repo/path/to/skill", "version": "v1.2.0"}'

# Pull from an OCI registry
curl -X POST http://localhost:9100/skills/pull \
  -d '{"source": "oci", "ref": "quay.io/redhat-et/skills/summarizer:1.2.0"}'

# List available dynamic skills
curl http://localhost:9100/skills/list
```

This skill can be included in the agent's static skill set
(ConfigMap mount) or baked into the agent container image. It
replaces the "agent writes files" pattern with a safe
delegation to the sidecar, preserving the read-only enforcement.

The flow becomes:

1. Agent determines it needs a capability it doesn't have
1. Agent (or operator) identifies the skill reference to pull
1. Agent uses the `skill-pull` skill to request the sidecar
   to fetch and verify the skill
1. Skill appears in `/skills/dynamic/` (read-only to the agent)
1. Agent discovers and loads the new skill (runtime-dependent;
   see "Skill hot-reload" above)

[rh-skills]: https://www.redhat.com/en/agentic-skills

## Deployment

The sidecar is added to the agent's Deployment spec. It should
integrate with the existing `docsclaw deploy` command — when
`agent-manifest.yaml` includes a `dynamicSkills` section,
the generated Kubernetes manifest includes the sidecar container
and the shared volume.

Estimated resource footprint: ~15–20 MiB memory, minimal CPU
(idle most of the time, brief spikes during pulls).

## Open questions

- Should skill pulls be allowed from any source or restricted
  to an allow-list in the sidecar config?
- Should the sidecar support skill eviction (removing a dynamic
  skill to free space)?
- How does the agent discover the sidecar endpoint — hardcoded
  localhost:9100, env var, or DNS?
