# DocsClaw + SkillImage demo

End-to-end demo of specialized agents powered by DocsClaw with
skills delivered as OCI images from the
[skillimage](https://github.com/redhat-et/skillimage) registry.

## What this shows

Two agents, same runtime, different personalities and skills —
deployed side-by-side in a single namespace:

| Agent | Skill image | Role | User stories |
| ----- | ----------- | ---- | ------------ |
| Executive Assistant | `quay.io/skillimage/document-summarizer:1.0.0-testing` | Summarizes reports, meeting notes, budgets for leadership | Campaign narrative, Budget variance, Capacity planning |
| HR Analyst | `quay.io/skillimage/document-reviewer:1.0.0-draft` | Reviews resumes, policies, onboarding checklists | Resume screening, Policy comparison, Onboarding audit |

Skills are mounted as OCI image volumes directly into
`/config/agent/skills/` — no init containers, no ConfigMap
duplication. The agent discovers them at startup.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  OpenShift namespace: panni-docsclaw                    │
│                                                         │
│  ┌──────────────────────┐  ┌──────────────────────────┐ │
│  │  executive-assistant  │  │  hr-analyst              │ │
│  │                       │  │                          │ │
│  │  ConfigMap:           │  │  ConfigMap:              │ │
│  │   system-prompt.txt   │  │   system-prompt.txt      │ │
│  │   agent-card.json     │  │   agent-card.json        │ │
│  │   agent-config.yaml   │  │   agent-config.yaml      │ │
│  │                       │  │                          │ │
│  │  Image volume:        │  │  Image volume:           │ │
│  │   document-summarizer │  │   document-reviewer      │ │
│  │   → /config/agent/    │  │   → /config/agent/       │ │
│  │     skills/document-  │  │     skills/document-     │ │
│  │     summarizer/       │  │     reviewer/            │ │
│  └──────────┬────────────┘  └────────────┬─────────────┘ │
│             │                            │               │
│        Route/Service              Route/Service          │
│             │                            │               │
└─────────────┼────────────────────────────┼───────────────┘
              │                            │
         a2a CLI / curl                a2a CLI / curl
```

## Prerequisites

- OpenShift 4.20+ (for image volume support)
- `oc` CLI logged in to the cluster
- Cluster-admin access (one-time SCC setup)
- An Anthropic API key
- `a2a` CLI (from [a2a-go](https://github.com/a2a-go/a2a-go))

## Deploy

### One-time setup: SCC for image volumes

OpenShift's default `restricted` SCC does not allow the `image`
volume type. Create a custom SCC that extends `restricted-v2`
with image volume support. This step requires cluster-admin:

```bash
oc apply -f image-volume-scc.yaml
```

This creates:

| Resource | Name | Scope |
| -------- | ---- | ----- |
| SecurityContextConstraints | `restricted-with-image-volumes` | Cluster |
| ServiceAccount | `docsclaw-agent` | Namespace |
| ClusterRole | `use-image-volume-scc` | Cluster |
| RoleBinding | `docsclaw-image-volume-scc` | Namespace |

### Deploy the agents

```bash
# 1. Switch to the demo namespace
oc project panni-docsclaw

# 2. Create the LLM API key secret
oc create secret generic llm-secret \
  --from-literal=LLM_API_KEY=sk-ant-...

# 3. Deploy both agents
oc apply -f executive-assistant.yaml
oc apply -f hr-analyst.yaml

# 4. Wait for rollout
oc rollout status deployment/executive-assistant
oc rollout status deployment/hr-analyst

# 5. Get the routes
oc get routes
```

## Demo scenario 1: Executive briefing

The executive assistant summarizes a quarterly report for a VP
meeting — demonstrating the "Campaign performance narrative" and
"Budget variance explanation" user stories.

```bash
EXEC_ROUTE=$(oc get route executive-assistant \
  -o jsonpath='{.spec.host}')

# Summarize a quarterly cloud spend report
a2a send --timeout 120s "https://$EXEC_ROUTE" \
  "Summarize this for the VP infrastructure review:

   Q1 Cloud Spend Report — Total: \$2.4M (up 18% from Q4)

   Three projects drove the increase:
   - Project Atlas: \$400K (database migration to Aurora, completing
     in Q2 — costs will drop to \$50K/quarter steady state)
   - Project Beacon: \$200K (new GPU instances for ML pipeline,
     expected to generate \$1.2M in efficiency gains by Q3)
   - DR failover event (Feb 14): \$150K unplanned, triggered by
     us-east-1 degradation. Post-mortem identified missing
     auto-scaling rules.

   Remaining \$1.65M is baseline (compute, storage, networking),
   in line with Q4. Q2 forecast: \$2.1M as Atlas completes.
   Action needed: approve Beacon GPU reservation for 1-year RI
   (saves 35%, \$70K/quarter)."
```

Expected output: a structured summary with key decisions, risks,
and action items — not a wall of text.

## Demo scenario 2: Resume screening

The HR analyst reviews a resume against job requirements —
demonstrating the "Resume screening and candidate ranking" user
story.

```bash
HR_ROUTE=$(oc get route hr-analyst \
  -o jsonpath='{.spec.host}')

# Review a candidate resume against job requirements
a2a send --timeout 120s "https://$HR_ROUTE" \
  "Review this resume for a Senior Product Manager role.

   Job requirements:
   - 5+ years product management experience
   - B2B SaaS background
   - Data-driven decision making with metrics
   - Cross-functional leadership (engineering, design, sales)
   - Experience with API or platform products (preferred)

   Resume — Jane Chen:
   Experience:
   - Datadog, Group Product Manager (2022-present): Leads API
     platform team of 12 engineers. Shipped usage-based billing
     that grew ARR 40%. Defined 3-year platform roadmap.
   - Datadog, Senior Product Manager (2020-2022): Owned developer
     integrations. Launched 15 new integrations, drove 25% increase
     in weekly active users.
   - Stripe, Associate Product Manager (2018-2020): Payments API
     team. Led developer documentation redesign that reduced
     support tickets 30%.
   Education: MS Computer Science, Stanford (2018)

   Gaps to evaluate:
   - No mention of user research or customer discovery methods
   - No B2B enterprise sales cycle experience listed
   - Short tenure at Stripe (2 years)"
```

Expected output: structured review with severity levels
(Critical/Important/Suggestion), fit score, and specific
feedback on each gap.

## Demo scenario 3: Policy review

The HR analyst reviews an updated policy for completeness —
demonstrating the "Policy document comparison" user story.

```bash
a2a send --timeout 120s "https://$HR_ROUTE" \
  "Review this updated remote work policy draft for completeness
   and consistency:

   REMOTE WORK POLICY v2.0 (Draft)

   1. Eligibility: All full-time employees after 90-day
      probationary period. Contractors are excluded.

   2. Schedule: Employees may work remotely up to 3 days per
      week. Core hours are 10am-3pm local time for meetings.

   3. Equipment: Company provides laptop and monitor. Employees
      are responsible for internet (minimum 50 Mbps). See the
      equipment policy for reimbursement details.

   4. Security: All work must be done on company-managed devices.
      VPN required for accessing internal systems.

   5. Performance: Managers will evaluate remote employees using
      the same criteria as on-site employees.

   6. Termination of remote work: The company reserves the right
      to require on-site presence with 2 weeks notice.

   Questions to address in your review:
   - Does this cover international remote work?
   - Is the equipment reimbursement section sufficient?
   - Are there any compliance gaps for US labor law?"
```

## Verifying skill discovery

After deployment, confirm the agent discovered the mounted skill:

```bash
# Check the pod logs for skill discovery
oc logs deployment/executive-assistant | grep -i skill

# Expected output:
#   level=INFO msg="discovered skill" name=document-summarizer ...

# Inspect what's mounted in the skills directory
oc exec deployment/executive-assistant -- ls /config/agent/skills/
oc exec deployment/executive-assistant -- \
  ls /config/agent/skills/document-summarizer/
```

## Cleanup

```bash
oc delete -f executive-assistant.yaml
oc delete -f hr-analyst.yaml
oc delete secret llm-secret

# Remove SCC resources (cluster-admin)
oc delete -f image-volume-scc.yaml
```

## How skills inform agent config

Each skill image contains a `skill.yaml` (SkillCard) that declares
the tools it needs and resource hints. You can inspect a skill
before deploying to ensure your `agent-config.yaml` allows the
right tools:

```bash
skillctl inspect quay.io/skillimage/document-summarizer:1.0.0-testing
```

The SkillCard `tools` field (e.g. `[read_file]`) should be a
subset of the tools listed in `agent-config.yaml`. The `resources`
field provides CPU/memory hints for pod sizing.

See [skillimage.dev](https://skillimage.dev) for the full
SkillCard schema and CLI reference. The site provides
`/llms.txt` and `/llms-full.txt` for agent-friendly access.

## Fallback: init container pull

If running on OpenShift < 4.20 (no image volume support), replace
the image volume with an init container that uses
[skillctl](https://github.com/redhat-et/skillimage) to pull the
skill at startup:

```yaml
initContainers:
  - name: skill-puller
    image: ghcr.io/redhat-et/skillctl:latest
    command:
      - skillctl
      - pull
      - --verify
      - -o
      - /config/agent/skills
      - quay.io/skillimage/document-summarizer:1.0.0-testing
    volumeMounts:
      - name: skills-vol
        mountPath: /config/agent/skills
```

See `deploy/agent-with-skills.yaml` for the ConfigMap approach.
