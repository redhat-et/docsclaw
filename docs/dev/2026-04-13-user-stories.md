# User stories for skill-equipped agents

## Overview

This document captures user stories for two distinct audiences:

- **Skill contributors** — developers who create, test, and publish
  skills for enterprise agents (based on input from teammate review)
- **Business users** — non-technical staff who consume agents and
  skills to accomplish domain-specific tasks without touching a
  terminal

The skill contributor stories are included for completeness. The
business user stories are the primary focus — these represent the
users who will drive adoption volume and whose needs should shape
the agent "mart" experience.

## Skill contributor stories

These stories describe the developer workflow for creating and
publishing enterprise skills. They assume familiarity with version
control and CI/CD but should not require specific tools like Cursor
or Claude Code.

> **Friction note:** Requiring contributors to install Cursor, Claude
> Code, and git before they can contribute a skill is a significant
> barrier. The contribution workflow should be accessible through a
> web-based UX with git and CI/CD handled behind the scenes.

### Discovery

- As a skill contributor, I need to see all skills currently in prod,
  pre-prod, and dev environments so that I do not replicate existing
  functionality. This should support interactive search and an API
  that agents (Cursor, Claude Code) can query programmatically.

- As a skill contributor, I need to see all tools (MCP servers)
  available in each environment so that I know what capabilities my
  skill can use. If the tool does not exist, I need to follow the
  "contribute a tool" workflow first.

### Ideation and validation

- As a skill contributor, I need a "potential skill" UX that accepts
  my ad-hoc description of a proposed skill and checks it against the
  existing catalog. The result is either a new skill project or a
  recommendation to modify an existing skill.

### Development and testing

- As a skill contributor, I need a runtime dev environment to iterate
  on a SKILL.md and its associated assets (scripts, references,
  sub-agents) with interactive testing. This should feel like vibe
  coding — fast feedback without deploy cycles.

- As a skill contributor, I need a dashboard showing detailed logs and
  stack traces for my skill's execution during automated evals and
  pre-prod runs so that I can quickly diagnose failures.

- As a skill contributor, I need to provide one or more automated
  evals (test cases) that validate the skill performs as expected.

### Contribution and review

- As a skill contributor, I need a "prepare for contribution" workflow
  that checks for overlap with existing skills (re-checked since
  elapsed time may have introduced new ones), verifies test coverage,
  runs security and supply chain scans, and validates metadata
  quality — prompting me where there is ambiguity.

- As a skill contributor, I need a "make contribution" workflow that
  bundles the skill, creates the branch and commit, pushes, creates
  the merge request, and triggers the CI pipeline — without requiring
  me to use git directly.

### Lifecycle management

- As a skill contributor, I need a "retire skill" workflow to craft a
  deprecation message and sunset date so that users are informed about
  upcoming removal or replacement.

- As a skill contributor, I need a "transfer ownership" workflow in
  case another person should take over maintenance. The organization
  needs to handle the case where a contributor has left the company.

### Feedback and monitoring

- As a skill contributor, I need a dashboard showing: contribution
  status (pipeline stage, approved/rejected, reviewer comments),
  usage metrics (daily active users), and user feedback (thumbs
  up/down with comments).

- As a skill contributor, I need Slack notifications for: pipeline
  errors, completion/success, reviewer comments, and end-user
  feedback.

## Business user stories

These stories describe non-technical users who interact with agents
through a web portal, chat interface, or internal application — never
through a terminal. They think in terms of tasks and outcomes, not
tools and APIs. The agent and its skills are invisible infrastructure;
what matters is that the work gets done correctly, securely, and with
an audit trail.

### Human resources

**Resume screening and candidate ranking**

As an HR recruiter, I have received 150 resumes for a senior product
manager position. I want to upload the resumes and the job description,
and have the agent grade each candidate on alignment with the required
qualifications, preferred experience, and cultural fit criteria. I need
the results as a ranked spreadsheet with scores and a one-paragraph
justification per candidate so that I can focus my time on the top 20.

**Policy document comparison**

As an HR policy analyst, we are updating our parental leave policy and
I have collected policies from five peer companies. I want to upload
all five documents and our current policy, and have the agent produce
a side-by-side comparison table highlighting where we are above, at,
or below market. I need the output in a format I can paste into a
slide deck for the VP review.

**Employee onboarding checklist audit**

As an HR operations manager, I want to verify that our 30-60-90 day
onboarding checklists are consistent across all twelve departments. I
want to upload all checklists and have the agent identify gaps,
inconsistencies, and missing steps compared to our corporate standard.

### Sales

**Proposal response generation**

As a sales engineer, I have received an RFP with 200 questions from
a prospective government customer. I want to upload the RFP and our
product documentation library, and have the agent draft responses to
each question, flagging any that require SME input because the
documentation does not cover them. I need the output formatted to
match the RFP's numbering so I can paste it into the response
template.

**Competitive analysis from earnings calls**

As a sales strategist, I want to upload the last four quarterly
earnings call transcripts from our three main competitors and have
the agent extract product announcements, pricing changes, and
strategic shifts. I need a trend summary per competitor and a
table of threats and opportunities for our portfolio.

**Deal qualification scoring**

As a sales manager, I want to upload a set of meeting notes and
email threads from a prospect engagement and have the agent score
the deal against our MEDDPICC qualification framework. I need the
output to show which criteria are met, which are at risk, and what
questions I should ask in the next call.

### Marketing

**Vendor proposal evaluation**

As a marketing director, we have received four proposals from
agencies for a website redesign. I want to upload all four proposals
and our brief, and have the agent produce a structured evaluation
matrix scoring each proposal on cost, timeline, creative approach,
technical capabilities, and alignment with our brand guidelines.
I need a recommendation with rationale that I can present to the
CMO.

**Content audit and gap analysis**

As a content marketing manager, I want the agent to analyze our
blog archive (300+ posts) against our current product positioning
and target personas. I need a report showing which topics are
over-covered, which have gaps, and a prioritized list of 10 new
articles that would fill the gaps.

**Campaign performance narrative**

As a demand gen analyst, I have a CSV export of campaign
performance data from the last quarter. I want the agent to
generate an executive summary with key trends, anomalies, and
recommendations — not just charts, but a narrative explanation of
what happened and why, suitable for the monthly marketing review.

### Security and compliance

**Vulnerability report triage**

As a security analyst, I receive a weekly vulnerability scan report
with 500+ findings across our infrastructure. I want to upload the
report and have the agent cross-reference each finding against our
asset inventory and SLA tiers, then produce a prioritized
remediation list grouped by team ownership. Critical findings with
SLA breaches should be highlighted at the top.

**Compliance evidence collection**

As a compliance officer preparing for a SOC 2 audit, I need to
collect evidence for 80 control points. I want to describe each
control to the agent and have it search our documentation
repository for matching evidence — policies, procedures, runbooks,
screenshots. It should produce a checklist showing which controls
have evidence, which are partial, and which have gaps that need
attention before the audit.

**Incident post-mortem analysis**

As a security operations lead, after a security incident I have a
collection of log excerpts, timeline notes, and Slack threads from
the response. I want to upload everything and have the agent
produce a structured post-mortem document following our template:
timeline of events, root cause analysis, impact assessment, and
remediation actions. I need the draft ready for the review meeting
within an hour.

### IT operations

**Runbook validation**

As an IT operations engineer, I want to verify that our runbooks
are still accurate after a platform upgrade. I want to upload the
runbook set and the release notes from the upgrade, and have the
agent identify which runbook steps reference changed components,
deprecated commands, or modified configuration paths. I need a
change list I can hand to the runbook owners for update.

**Change request risk assessment**

As a change manager, I receive 30-40 change requests per week
for review. I want to upload each change request along with our
change risk matrix and have the agent pre-score the risk level
(low/medium/high) based on scope, blast radius, rollback
complexity, and time of implementation. I need the pre-scored
list before the Change Advisory Board meeting so we can focus
discussion on the high-risk items.

**Capacity planning report**

As an infrastructure analyst, I have exported six months of
resource utilization data from our monitoring platform. I want
the agent to analyze the trends, identify clusters approaching
capacity thresholds, and project when we will need to add capacity
at current growth rates. I need the output as a summary with
charts that I can include in the quarterly infrastructure review.

**Alert fatigue reduction**

As an SRE, I want to upload a month of alert history (thousands of
alerts) and have the agent identify patterns: which alerts always
fire together (and should be consolidated), which alerts never
result in action (and should be silenced or re-thresholded), and
which alerts have high false-positive rates. I need actionable
recommendations I can implement in our alerting rules.

### Finance and procurement

**Invoice anomaly detection**

As a procurement analyst, I want to upload the last quarter's
invoices from our top 20 vendors alongside the corresponding
contracts. I need the agent to flag invoices that deviate from
contracted rates, identify duplicate charges, and highlight
unusual patterns — for example, a vendor whose charges increased
30% without a contract amendment.

**Budget variance explanation**

As a financial analyst, I have the monthly budget-vs-actuals
report and I need to prepare variance explanations for every line
item over 10% deviation. I want to upload the report and have the
agent draft variance explanations based on known context
(headcount changes, project delays, one-time purchases). I need
the output in our standard format so I can review and submit
without reformatting.

## Common patterns across business user stories

Several patterns emerge from these stories that should inform the
agent and skill design:

1. **Document in, document out.** Almost every story involves
   uploading documents and receiving a structured output. The agent
   needs robust document ingestion (PDF, DOCX, XLSX, CSV, plain
   text) and formatted output generation (tables, spreadsheets,
   slide-ready content).

1. **No terminal, no git, no CLI.** Business users interact through
   a web portal or chat interface. The agent mart, skill selection,
   and task submission must be fully graphical. File uploads replace
   command-line arguments.

1. **Audit trail and explainability.** Business users in regulated
   functions (HR, security, finance) need to explain and defend the
   agent's output. Every analysis should include justification, not
   just results. The audit trail (which skills ran, which documents
   were processed, which model was used) must be accessible.

1. **Output format matters.** "A ranked spreadsheet," "a table I can
   paste into slides," "our standard template." Business users have
   existing workflows and tools; the agent's output must fit into
   them, not replace them.

1. **Domain-specific scoring frameworks.** MEDDPICC for sales, SOC 2
   controls for compliance, change risk matrices for IT ops. Skills
   need to be parameterizable with domain-specific rubrics that the
   business user provides or selects from a library.

1. **Batch processing.** Many stories involve processing tens or
   hundreds of documents in a single request. The agent needs to
   handle batch workloads without hitting context limits — likely
   by processing documents individually and aggregating results.

## Implications for DocsClaw

These business user stories reinforce several design decisions and
highlight new requirements:

| Implication | Design impact |
| ----------- | ------------- |
| Web portal needed | Agents need an HTTP frontend beyond A2A; a thin web UI or integration with an existing portal |
| Document ingestion skills | Need PDF, DOCX, XLSX, CSV parsing skills — prime candidates for OCI-distributed skill packs with tool dependencies (e.g., pandoc, Apache Tika) |
| Batch processing | Context compaction becomes critical; or a pattern where the orchestrating agent delegates per-document work to sub-agents |
| Template-driven output | Skills need configurable output templates; the SkillCard could declare supported output formats |
| Audit trail | The deployment audit trail (skills-installed.json) extends to execution logs: which skill processed which document with what result |
| Domain rubric library | Scoring frameworks (MEDDPICC, SOC 2 controls, change risk matrix) are a form of skill configuration — parameterizable via the SkillCard or uploaded reference documents |
