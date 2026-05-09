# Batch processing demo design

## Overview

A conference-ready demo showcasing DocsClaw's lightweight agent
runtime through three business scenarios: HR resume screening
(parallel fan-out), security vulnerability triage, and finance
invoice anomaly detection. The demo targets a mixed audience of
platform architects, business stakeholders, and developers.

All three scenarios run on an OpenShift cluster. The HR scenario
demonstrates parallel orchestration across 10 worker agents via
A2A. The security and finance scenarios demonstrate personality
swap via ConfigMap changes. Total cluster footprint for all 16
agents: under 200 MiB.

## Architecture

### Approach: hybrid orchestration

A new `docsclaw batch` CLI subcommand handles deterministic
fan-out (splitting documents across worker agents, collecting
results, aggregating output). Worker agents handle the
LLM-powered evaluation. This separates mechanical orchestration
from intelligent analysis — no LLM tokens wasted on logistics.

```text
                    OpenShift Cluster
  ┌──────────────────────────────────────────────────┐
  │                                                  │
  │  demo-hr namespace                               │
  │  ┌──────────────┐    ┌────────────────────────┐  │
  │  │ document-    │◄───│ hr-worker-1..10        │  │
  │  │ service      │    │ (DocsClaw pods)        │  │
  │  │ (100 resumes)│    │ fetch_document tool    │  │
  │  └──────────────┘    └──────────▲─────────────┘  │
  │                                 │ A2A            │
  │                     ┌───────────┴─────────────┐  │
  │                     │ docsclaw batch          │  │
  │                     │ (K8s Job)               │  │
  │                     └─────────────────────────┘  │
  │                                                  │
  │  demo-security namespace                         │
  │  ┌──────────────┐    ┌────────────────────────┐  │
  │  │ document-    │◄───│ security-analyst       │  │
  │  │ service      │    │ (DocsClaw pod)         │  │
  │  └──────────────┘    └────────────────────────┘  │
  │                                                  │
  │  demo-finance namespace                          │
  │  ┌──────────────┐    ┌────────────────────────┐  │
  │  │ document-    │◄───│ finance-analyst        │  │
  │  │ service      │    │ (DocsClaw pod)         │  │
  │  └──────────────┘    └────────────────────────┘  │
  └──────────────────────────────────────────────────┘
```

### Namespace layout

| Namespace | Pods | Purpose |
| --------- | ---- | ------- |
| `demo-hr` | 10 workers + document-service + batch Job | Resume screening fan-out |
| `demo-security` | 1 analyst + document-service | Vulnerability triage |
| `demo-finance` | 1 analyst + document-service | Invoice anomaly detection |

Each worker pod has its own ClusterIP Service for addressability
within the cluster. No Routes or Ingresses — all communication
is internal.

### Purpose-built agent images

Each scenario uses a purpose-built container image: same
DocsClaw binary, different preprocessing tools. This
demonstrates token efficiency (mechanical format conversion
before LLM sees the data) and minimal attack surface (no shell,
no package manager, only the specific tools the admin approved).

| Image | Base | Added tools | Purpose | Size |
| ----- | ---- | ----------- | ------- | ---- |
| `docsclaw-hr` | scratch + docsclaw | `pdftotext` (poppler) | Resume PDF extraction | ~15-20 MiB |
| `docsclaw-security` | scratch + docsclaw | `jq`, `csvtool` | Vuln report JSON/CSV parsing | ~12-15 MiB |
| `docsclaw-finance` | scratch + docsclaw | `xlsx2csv` | Spreadsheet conversion | ~15-20 MiB |

Images are pre-built by the cluster admin and stored in the
registry. Users select an image when deploying an agent — no
build step at request time.

### Document-service integration

Documents (resumes, reports, invoices) are served by the
existing document-service from the zero-trust-agent-demo
project. It acts as a generic document repository — the storage
backend (S3, filesystem, mock) is irrelevant to the demo
audience.

Workers retrieve documents using the existing `fetch_document`
tool via `GET /documents/{id}`. The batch CLI sends document IDs
in the A2A message payload — workers fetch content themselves.

## The `docsclaw batch` subcommand

### Usage

```bash
docsclaw batch \
  --agents http://hr-worker-1.demo-hr.svc:8080,...  \
  --documents DOC-R001,DOC-R002,...,DOC-R100 \
  --context-doc DOC-JD001 \
  --prompt "Score this resume against the job description..." \
  --output results.csv
```

### Behavior

The subcommand is purely deterministic Go code — no LLM
involvement:

1. **Load** — read document ID list and agent endpoint list
1. **Split** — divide document IDs into equal batches across
   agents
1. **Fan out** — send each batch to a worker via A2A
   `message/send`, including the context document ID and
   scoring prompt
1. **Collect** — wait for all responses with timeout and retry
1. **Aggregate** — parse results, merge into ranked list
1. **Output** — write CSV, print summary stats

### Worker message format

Each worker receives a message containing:

- The context document ID (job description, asset inventory, or
  contracts)
- A list of document IDs to evaluate
- The scoring prompt with output format instructions

The worker uses its `fetch_document` tool in the agentic loop
to retrieve each document, evaluates it, and returns structured
results.

### Error handling

- Failed or timed-out workers: batch is redistributed to
  remaining workers
- Partial results are output with a warning listing
  unprocessed documents
- Summary includes per-worker status (success, retry, failed)

### Summary output

```text
Batch complete.
  Documents processed: 100/100
  Agents used:         10
  Wall-clock time:     3m42s
  Total LLM tokens:    287,400 (input: 241,200 / output: 46,200)
  Tokens saved by preprocessing: ~120,000 (pdf_to_text)
  Peak cluster memory:  142 MiB (all pods)
```

## Three demo scenarios

### Act 1: HR resume screening (parallel fan-out)

- **Input:** 100 synthetic resumes + 1 job description for
  Senior Product Manager
- **Agents:** 10 workers using `docsclaw-hr` image
- **System prompt:** HR analyst persona specializing in resume
  screening with consistent, fair scoring criteria
- **Agent config:** `fetch_document` tool only
- **Output:** CSV: rank, document_id, candidate_name, score
  (1-10), strengths, weaknesses, recommendation
- **Talking point:** "10 agents, 100 resumes, under 200 MiB
  total, done in N minutes"

### Act 2: Security vulnerability triage (personality swap)

- **Input:** 1 vulnerability scan report (~50 findings) + 1
  asset inventory with SLA tiers
- **Agent:** 1 analyst using `docsclaw-security` image
- **System prompt:** Security analyst persona, triage by
  business impact and SLA requirements
- **Agent config:** `fetch_document` tool only
- **Output:** Prioritized remediation list grouped by team, SLA
  breaches highlighted
- **Talking point:** "Same runtime, different image, different
  personality — now it's a security analyst"

### Act 3: Finance invoice anomaly detection (another swap)

- **Input:** ~20 invoices from 5 vendors + 5 contracts
- **Agent:** 1 analyst using `docsclaw-finance` image
- **System prompt:** Procurement analyst persona, compare
  invoices against contracted rates
- **Agent config:** `fetch_document` tool only
- **Output:** Anomaly report: rate deviations, duplicate
  charges, unexplained patterns
- **Talking point:** "Third domain, third image, still under
  200 MiB total"

## Synthetic test data

### HR resumes (100 documents)

Generated by a script with controlled distribution:

| Tier | Count | Profile |
| ---- | ----- | ------- |
| Strong match | 20 | 7+ years PM, relevant industry, leadership |
| Moderate match | 30 | PM experience, missing some requirements |
| Weak match | 30 | Adjacent roles (project manager, BA) |
| Poor match | 20 | Unrelated backgrounds |

Each resume: name, contact, summary, work history, education,
skills. Realistic but obviously synthetic (fictional companies).

The job description specifies required qualifications, preferred
experience, responsibilities, and a scoring rubric.

### Security data

- Vulnerability report: ~50 findings with CVE IDs, severity
  levels, affected hosts, discovery dates (JSON/CSV)
- Asset inventory: hosts mapped to teams and SLA tiers (Tier 1:
  24h, Tier 2: 7d, Tier 3: 30d)
- Several findings planted past SLA deadlines

### Finance data

- ~20 invoices from 5 fictional vendors (XLSX format)
- 5 contracts with agreed rates
- Planted anomalies: 30% rate increase without amendment, one
  duplicate charge, line items not in contract

### Seeding

A `seed-demo-data.sh` script loads all documents into
document-service via POST endpoints. Runs as a deployment step
or standalone.

## Demo script

```text
Act 0: Setup (pre-done or quick show)
  ├── Show three images in registry
  ├── Show a Dockerfile: scratch + binary + pdftotext
  └── "The admin prepared these. Users never see a Dockerfile."

Act 1: HR resume screening
  ├── oc apply -f demo-hr/
  ├── oc get pods -n demo-hr
  ├── oc top pods -n demo-hr          # ~10 MiB each
  ├── docsclaw batch --agents ... --output hr-results.csv
  ├── Watch progress in terminal
  ├── cat hr-results.csv | head -20
  └── Recap: agents, time, memory, tokens

Act 2: Security vulnerability triage
  ├── oc apply -f demo-security/
  ├── oc get pods -n demo-security
  ├── Send vuln report via A2A
  ├── Show prioritized remediation output
  └── "Same runtime, different image, different personality"

Act 3: Finance invoice anomaly
  ├── oc apply -f demo-finance/
  ├── Send invoices + contracts
  ├── Show anomaly report
  └── "Third domain, third image, one runtime"

Finale: The numbers
  ├── oc top pods across all namespaces
  ├── "175 MiB for 16 agents vs ~5 GiB in Python"
  └── Token savings from preprocessing
```

### Metrics captured

| Metric | Source | Shown |
| ------ | ------ | ----- |
| Pod memory per agent | `oc top pods` | After deployment |
| Total cluster memory | `oc top pods` across namespaces | Finale |
| Wall-clock time | `docsclaw batch` output | After HR |
| Tokens used per worker | Agentic loop logs | After HR |
| Tokens saved by preprocessing | Calculated | Finale |
| Pod startup time | `oc get pods` timestamps | After deployment |

## Deployment manifests

Each agent type gets a YAML file following the existing pattern
in `docs/demo/`:

- `ConfigMap` — system-prompt.txt + agent-config.yaml
- `Deployment` — DocsClaw pod with resource limits
- `Service` — ClusterIP, no Route

For the HR scenario, a script or template generates 10 worker
manifests with different names but identical ConfigMaps.

### Resource budget

| Component | Pods | Memory each | Total |
| --------- | ---- | ----------- | ----- |
| HR workers | 10 | ~10 MiB | ~100 MiB |
| Security analyst | 1 | ~10 MiB | ~10 MiB |
| Finance analyst | 1 | ~10 MiB | ~10 MiB |
| Document services | 3 | ~15 MiB | ~45 MiB |
| Batch Job | 1 | ~10 MiB | ~10 MiB |
| **Total** | **16** | | **~175 MiB** |

## What needs to be built

### Existing (reuse)

| Component | Location |
| --------- | -------- |
| DocsClaw runtime | `cmd/docsclaw/` |
| `fetch_document` tool | `internal/fetchdoc/` |
| Agentic loop | `pkg/tools/loop.go` |
| A2A bridge | `internal/bridge/` |
| Document-service | `zero-trust-agent-demo/document-service` |
| Demo manifest pattern | `docs/demo/hr-analyst.yaml` |

### New work

| Component | Description | Effort |
| --------- | ----------- | ------ |
| `docsclaw batch` subcommand | Cobra command: fan-out, collect, aggregate, output CSV | Medium |
| HR worker ConfigMap | System prompt + agent-config for resume scoring | Small |
| Security analyst ConfigMap | System prompt + agent-config for vuln triage | Small |
| Finance analyst ConfigMap | System prompt + agent-config for invoice analysis | Small |
| Three Dockerfiles | `Dockerfile.hr`, `Dockerfile.security`, `Dockerfile.finance` | Small |
| Deployment manifests | Per-namespace YAML, templated for HR workers | Small |
| Test data generator | Script: 100 resumes, vuln report, invoices | Medium |
| Seed script | Load documents into document-service | Small |
| Demo runner script | End-to-end deployment and execution | Small |

### Future extensions (not in scope)

- Web UI for launching scenarios
- OCI-packaged skills for each persona
- Sigstore signing of agent images
