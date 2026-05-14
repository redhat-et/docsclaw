# Batch processing demo

Conference demo showcasing DocsClaw's lightweight agent runtime
through three business scenarios running on OpenShift.

## What this shows

- **HR resume screening:** 10 parallel agents score 100 resumes
  against a job description via A2A fan-out
- **Security vulnerability triage:** Single agent cross-references
  findings against asset inventory and SLA tiers
- **Finance invoice anomaly detection:** 5 parallel agents each
  analyze one vendor's contract and invoices to flag discrepancies

Same binary, three purpose-built images, three namespaces. Total
cluster footprint: under 200 MiB.

## Purpose-built images

| Image | Tools | Purpose |
| ----- | ----- | ------- |
| `docsclaw-hr` | pdftotext | Resume PDF extraction |
| `docsclaw-security` | jq, csvtool | Vuln report parsing |
| `docsclaw-finance` | ssconvert | Spreadsheet conversion |

## Prerequisites

- OpenShift 4.20+ cluster with `oc` CLI
- An LLM API key (set `LLM_API_KEY` env var)
- `docsclaw` binary (for `batch` subcommand)
- `python3` (for data generation)
- document-service deployed in each namespace

## Quick start

```bash
# 1. Generate synthetic test data
python3 scripts/generate-demo-data.py

# 2. Run the full demo
export LLM_API_KEY=sk-ant-...
scripts/demo-run.sh
```

## Manual step-by-step

See `scripts/demo-run.sh` for the full sequence, or run each
act individually:

```bash
# Act 1: HR (parallel fan-out)
oc apply -f demo/batch/namespace-setup.yaml
scripts/seed-demo-data.sh --scenario hr --url http://...
docsclaw batch --agents ... --documents ... --output results.csv

# Act 2: Security (single agent)
oc apply -f demo/batch/security/deployment.yaml
a2a send http://security-analyst.demo-security.svc:8080 "..."

# Act 3: Finance (5-agent fan-out)
oc apply -f demo/batch/finance/deployment.yaml
demo/batch/finance/finance-batch.sh
```

## Cleanup

```bash
oc delete namespace demo-hr demo-security demo-finance
```

## Architecture

See `docs/superpowers/specs/2026-05-09-batch-demo-design.md`
for the full design spec.
