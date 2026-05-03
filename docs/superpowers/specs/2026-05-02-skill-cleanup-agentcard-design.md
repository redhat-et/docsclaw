# Skill Cleanup and AgentCard Injection

**Issues:** #38 (remove OCI code), #39 (populate AgentCard skills)
**Approach:** Single combined PR on `refactor/skill-cleanup-and-agentcard`

## Problem

DocsClaw contains OCI skill pack/push/pull/inspect code that has moved
to the standalone [skillimage](https://github.com/redhat-et/skillimage)
project. This dead code should be removed. Additionally, the AgentCard
served at `/.well-known/agent.json` never includes discovered skills,
even though `pkg/skills/Discover()` already finds them at runtime.

## Part 1: OCI Code Removal

### Files to delete

| Category | Files |
|----------|-------|
| OCI package | `internal/oci/` (all 7 files) |
| Skill commands | `internal/cmd/skill_pack.go`, `skill_push.go`, `skill_pull.go`, `skill_inspect.go`, `skill_verify.go` |
| Verify package | `internal/verify/` (all 3 files) |

### Files to modify

- `internal/cmd/skill.go` — update description from
  "Manage OCI-distributed skills" to "Manage locally available skills";
  remove pack/push/pull/verify references from long description
- `go.mod` / `go.sum` — `go mod tidy` to drop `oras.land/oras-go/v2`
  and transitively-unused deps

### Files to keep

- `internal/cmd/skill_delete.go`, `skill_list.go` — local management
- `pkg/skills/` — runtime discovery and loading
- `pkg/skills/card/` — `skill.yaml` parsing (used by list and new
  AgentSkill conversion)
- `examples/openshift/`, `docs/demo/` — already reference `skillctl`

## Part 2: AgentCard Skill Injection

### New file: `pkg/skills/a2a.go`

Conversion function:

```go
func ToAgentSkills(metas []SkillMeta) []a2a.AgentSkill
```

For each `SkillMeta`:

1. Attempt `card.Parse(meta.Dir + "/skill.yaml")`
2. If `skill.yaml` exists, map `SkillCard` fields:
   - `ID` = SkillCard `Name`
   - `Name` = SkillCard `Name`
   - `Description` = SkillCard `Description`
   - `Tags` = derived from namespace, author, required tools
3. If no `skill.yaml`, fall back to `SkillMeta`:
   - `ID` = `meta.Name`
   - `Name` = `meta.Name`
   - `Description` = `meta.Description`
   - `Tags` = empty

### New file: `pkg/skills/a2a_test.go`

Test cases:
- Skill with `skill.yaml` produces full mapping with derived tags
- Skill with only `SKILL.md` produces fallback mapping, empty tags
- Empty input returns empty output
- Mixed skills (some with `skill.yaml`, some without)

### Modified: `internal/cmd/serve.go`

After `skills.Discover()` (line ~312), call `skills.ToAgentSkills()`
and merge results into the `agentCard` loaded by `loadAgentCard()`.
The injection point is between skill discovery and
`DynamicCardHandler(agentCard, ...)` at line ~429. Discovered skills
append to any skills already present in `agentCard.Skills` (from
`agent-card.json`). Deduplicate by ID; static entries win on conflict.

`BuildAgentCard` in `internal/bridge/agentcard.go` is not used in the
serve flow — `loadAgentCard()` reads from JSON instead. No changes
needed to `agentcard.go`.

## Verification

- `make build` — compiles after OCI removal
- `make test` — all tests pass (including new `a2a_test.go`)
- `make lint` — clean
- `go mod tidy` — oras dependency removed
