# Batch processing demo implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a conference demo that showcases DocsClaw's
lightweight agent runtime through three business scenarios (HR
resume screening with parallel fan-out, security vulnerability
triage, finance invoice anomaly detection) running on OpenShift.

**Architecture:** A new `docsclaw batch` CLI subcommand handles
deterministic fan-out of document IDs to N worker agents via A2A,
collects results, and produces a ranked CSV report. Worker agents
use `fetch_document` to retrieve documents from document-service
and score them. Three purpose-built container images (same binary,
different preprocessing tools) serve three business domains in
separate namespaces.

**Tech Stack:** Go 1.25+, Cobra, A2A (a2a-go), document-service
(existing), OpenShift 4.20+

**Spec:** `docs/superpowers/specs/2026-05-09-batch-demo-design.md`

---

## File structure

### New files

| File | Responsibility |
| ---- | -------------- |
| `internal/cmd/batch.go` | Cobra `batch` subcommand: parse flags, split documents, fan out via A2A, collect and aggregate results, write CSV |
| `internal/cmd/batch_test.go` | Tests for batch logic: splitting, aggregation, CSV output, error handling |
| `demo/batch/hr/system-prompt.txt` | HR resume scoring persona |
| `demo/batch/hr/agent-config.yaml` | Tools config for HR workers |
| `demo/batch/hr/agent-card.json` | Agent card for HR workers |
| `demo/batch/hr/deployment.yaml` | K8s manifests: ConfigMap + Deployment + Service for hr-worker-{1..10} and document-service |
| `demo/batch/security/system-prompt.txt` | Security vulnerability triage persona |
| `demo/batch/security/agent-config.yaml` | Tools config for security analyst |
| `demo/batch/security/agent-card.json` | Agent card for security analyst |
| `demo/batch/security/deployment.yaml` | K8s manifests for security-analyst + document-service |
| `demo/batch/finance/system-prompt.txt` | Finance invoice analysis persona |
| `demo/batch/finance/agent-config.yaml` | Tools config for finance analyst |
| `demo/batch/finance/agent-card.json` | Agent card for finance analyst |
| `demo/batch/finance/deployment.yaml` | K8s manifests for finance-analyst + document-service |
| `demo/batch/Dockerfile.hr` | scratch + docsclaw + pdftotext |
| `demo/batch/Dockerfile.security` | scratch + docsclaw + jq + csvtool |
| `demo/batch/Dockerfile.finance` | scratch + docsclaw + xlsx2csv |
| `scripts/generate-demo-data.py` | Generate 100 synthetic resumes, vuln report, invoices |
| `scripts/seed-demo-data.sh` | Load generated documents into document-service |
| `scripts/demo-run.sh` | End-to-end demo runner |
| `demo/batch/README.md` | Demo instructions |

### Modified files

| File | Change |
| ---- | ------ |
| `internal/cmd/root.go` | No change needed (batch.go registers itself via `init()`) |

---

## Task 1: Batch subcommand — document splitting logic

The core splitting function: takes a list of document IDs and a
number of agents, returns equal-sized batches. Pure function, no
I/O.

**Files:**

- Create: `internal/cmd/batch.go`
- Create: `internal/cmd/batch_test.go`

- [ ] **Step 1: Write the failing test for splitting**

```go
// internal/cmd/batch_test.go
package cmd

import (
	"testing"
)

func TestSplitDocuments(t *testing.T) {
	tests := []struct {
		name     string
		docs     []string
		agents   int
		expected [][]string
	}{
		{
			name:   "even split",
			docs:   []string{"D1", "D2", "D3", "D4"},
			agents: 2,
			expected: [][]string{
				{"D1", "D2"},
				{"D3", "D4"},
			},
		},
		{
			name:   "uneven split",
			docs:   []string{"D1", "D2", "D3", "D4", "D5"},
			agents: 2,
			expected: [][]string{
				{"D1", "D2", "D3"},
				{"D4", "D5"},
			},
		},
		{
			name:   "more agents than docs",
			docs:   []string{"D1", "D2"},
			agents: 5,
			expected: [][]string{
				{"D1"},
				{"D2"},
			},
		},
		{
			name:     "empty docs",
			docs:     []string{},
			agents:   3,
			expected: nil,
		},
		{
			name:   "single agent",
			docs:   []string{"D1", "D2", "D3"},
			agents: 1,
			expected: [][]string{
				{"D1", "D2", "D3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitDocuments(tt.docs, tt.agents)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d batches, got %d",
					len(tt.expected), len(result))
			}
			for i, batch := range result {
				if len(batch) != len(tt.expected[i]) {
					t.Fatalf("batch %d: expected %d docs, got %d",
						i, len(tt.expected[i]), len(batch))
				}
				for j, doc := range batch {
					if doc != tt.expected[i][j] {
						t.Fatalf("batch %d doc %d: expected %q, got %q",
							i, j, tt.expected[i][j], doc)
					}
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestSplitDocuments -v`
Expected: FAIL — `splitDocuments` undefined

- [ ] **Step 3: Write the splitDocuments function**

```go
// internal/cmd/batch.go
package cmd

func splitDocuments(docs []string, agents int) [][]string {
	if len(docs) == 0 {
		return nil
	}
	if agents > len(docs) {
		agents = len(docs)
	}
	batchSize := len(docs) / agents
	remainder := len(docs) % agents

	var batches [][]string
	start := 0
	for i := range agents {
		size := batchSize
		if i < remainder {
			size++
		}
		batches = append(batches, docs[start:start+size])
		start += size
	}
	return batches
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cmd/ -run TestSplitDocuments -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/batch.go internal/cmd/batch_test.go
git commit -s -m "feat: add document splitting for batch subcommand"
```

---

## Task 2: Batch subcommand — CSV result aggregation

Parse structured text results from workers into a ranked CSV
output. Each worker returns text containing one JSON object per
document with score, candidate name, strengths, weaknesses,
recommendation.

**Files:**

- Modify: `internal/cmd/batch.go`
- Modify: `internal/cmd/batch_test.go`

- [ ] **Step 1: Write the failing test for result parsing**

```go
// append to internal/cmd/batch_test.go

import (
	"encoding/json"
	"strings"
)

func TestParseBatchResult(t *testing.T) {
	input := `[
		{
			"document_id": "DOC-R001",
			"candidate_name": "Jane Chen",
			"score": 9,
			"strengths": "Strong PM background, API platform experience",
			"weaknesses": "No enterprise sales cycle experience",
			"recommendation": "Strong hire"
		},
		{
			"document_id": "DOC-R002",
			"candidate_name": "Bob Smith",
			"score": 4,
			"strengths": "Leadership experience",
			"weaknesses": "No PM experience, wrong industry",
			"recommendation": "Pass"
		}
	]`

	results, err := parseBatchResult(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].DocumentID != "DOC-R001" {
		t.Fatalf("expected DOC-R001, got %q", results[0].DocumentID)
	}
	if results[0].Score != 9 {
		t.Fatalf("expected score 9, got %d", results[0].Score)
	}
}

func TestParseBatchResultFromMarkdown(t *testing.T) {
	input := "Here are the results:\n\n```json\n" +
		`[{"document_id":"DOC-R001","candidate_name":"Jane",` +
		`"score":8,"strengths":"Good","weaknesses":"None",` +
		`"recommendation":"Hire"}]` +
		"\n```\n\nAll candidates evaluated."

	results, err := parseBatchResult(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestFormatCSV(t *testing.T) {
	results := []candidateResult{
		{
			DocumentID:     "DOC-R002",
			CandidateName:  "Bob Smith",
			Score:          4,
			Strengths:      "Leadership",
			Weaknesses:     "Wrong industry",
			Recommendation: "Pass",
		},
		{
			DocumentID:     "DOC-R001",
			CandidateName:  "Jane Chen",
			Score:          9,
			Strengths:      "Strong PM",
			Weaknesses:     "None",
			Recommendation: "Hire",
		},
	}

	var buf strings.Builder
	err := formatCSV(&buf, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "rank,") {
		t.Fatalf("expected CSV header starting with 'rank,', got %q",
			lines[0])
	}
	// First data row should be Jane (score 9, rank 1)
	if !strings.Contains(lines[1], "Jane Chen") {
		t.Fatalf("expected Jane Chen in first row (highest score), got %q",
			lines[1])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestParseBatchResult|TestFormatCSV" -v`
Expected: FAIL — types and functions undefined

- [ ] **Step 3: Implement result types, parsing, and CSV formatting**

```go
// append to internal/cmd/batch.go

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type candidateResult struct {
	DocumentID     string `json:"document_id"`
	CandidateName  string `json:"candidate_name"`
	Score          int    `json:"score"`
	Strengths      string `json:"strengths"`
	Weaknesses     string `json:"weaknesses"`
	Recommendation string `json:"recommendation"`
}

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(.*?)\\n```")

func parseBatchResult(text string) ([]candidateResult, error) {
	jsonStr := text

	// Try to extract JSON from markdown code blocks first
	if matches := jsonBlockRe.FindStringSubmatch(text); len(matches) > 1 {
		jsonStr = matches[1]
	}

	jsonStr = strings.TrimSpace(jsonStr)

	var results []candidateResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		// Try parsing as a single object
		var single candidateResult
		if err2 := json.Unmarshal([]byte(jsonStr), &single); err2 != nil {
			return nil, fmt.Errorf("failed to parse results: %w", err)
		}
		results = append(results, single)
	}
	return results, nil
}

func formatCSV(w io.Writer, results []candidateResult) error {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"rank", "document_id", "candidate_name", "score",
		"strengths", "weaknesses", "recommendation",
	}); err != nil {
		return err
	}

	for i, r := range results {
		if err := cw.Write([]string{
			strconv.Itoa(i + 1),
			r.DocumentID,
			r.CandidateName,
			strconv.Itoa(r.Score),
			r.Strengths,
			r.Weaknesses,
			r.Recommendation,
		}); err != nil {
			return err
		}
	}
	return cw.Error()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run "TestParseBatchResult|TestFormatCSV" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/batch.go internal/cmd/batch_test.go
git commit -s -m "feat: add result parsing and CSV formatting for batch"
```

---

## Task 3: Batch subcommand — A2A fan-out and Cobra wiring

Wire together the splitting, A2A invocation, result collection,
and CSV output into the full `docsclaw batch` Cobra command.

**Files:**

- Modify: `internal/cmd/batch.go`
- Modify: `internal/cmd/batch_test.go`

- [ ] **Step 1: Write the failing test for the batch runner**

```go
// append to internal/cmd/batch_test.go

import (
	"context"
	"sync/atomic"
)

func TestRunBatch(t *testing.T) {
	callCount := atomic.Int32{}

	// Mock invoker that returns scored results
	invoker := func(ctx context.Context, agentURL string,
		contextDoc string, docs []string, prompt string,
	) (string, error) {
		callCount.Add(1)
		var results []candidateResult
		for i, doc := range docs {
			results = append(results, candidateResult{
				DocumentID:     doc,
				CandidateName:  fmt.Sprintf("Candidate %s", doc),
				Score:          10 - i,
				Strengths:      "Good",
				Weaknesses:     "None",
				Recommendation: "Hire",
			})
		}
		data, _ := json.Marshal(results)
		return string(data), nil
	}

	cfg := batchConfig{
		agents:     []string{"http://agent-1:8080", "http://agent-2:8080"},
		documents:  []string{"D1", "D2", "D3", "D4"},
		contextDoc: "JD1",
		prompt:     "Score these resumes",
	}

	var buf strings.Builder
	stats, err := runBatch(context.Background(), cfg, invoker, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount.Load() != 2 {
		t.Fatalf("expected 2 agent calls, got %d", callCount.Load())
	}
	if stats.processed != 4 {
		t.Fatalf("expected 4 processed, got %d", stats.processed)
	}
	if stats.failed != 0 {
		t.Fatalf("expected 0 failed, got %d", stats.failed)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// header + 4 data rows
	if len(lines) != 5 {
		t.Fatalf("expected 5 CSV lines, got %d", len(lines))
	}
}

func TestRunBatchWithFailure(t *testing.T) {
	invoker := func(ctx context.Context, agentURL string,
		contextDoc string, docs []string, prompt string,
	) (string, error) {
		if agentURL == "http://agent-2:8080" {
			return "", fmt.Errorf("connection refused")
		}
		var results []candidateResult
		for _, doc := range docs {
			results = append(results, candidateResult{
				DocumentID:     doc,
				CandidateName:  "Test",
				Score:          5,
				Strengths:      "OK",
				Weaknesses:     "OK",
				Recommendation: "Maybe",
			})
		}
		data, _ := json.Marshal(results)
		return string(data), nil
	}

	cfg := batchConfig{
		agents:     []string{"http://agent-1:8080", "http://agent-2:8080"},
		documents:  []string{"D1", "D2", "D3", "D4"},
		contextDoc: "JD1",
		prompt:     "Score these",
	}

	var buf strings.Builder
	stats, err := runBatch(context.Background(), cfg, invoker, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.processed != 2 {
		t.Fatalf("expected 2 processed, got %d", stats.processed)
	}
	if stats.failed != 2 {
		t.Fatalf("expected 2 failed, got %d", stats.failed)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestRunBatch" -v`
Expected: FAIL — `batchConfig`, `runBatch` undefined

- [ ] **Step 3: Implement batchConfig, runBatch, and Cobra wiring**

```go
// append to internal/cmd/batch.go

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/redhat-et/docsclaw/internal/bridge"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type batchConfig struct {
	agents     []string
	documents  []string
	contextDoc string
	prompt     string
	timeout    time.Duration
}

type batchStats struct {
	processed int
	failed    int
	agents    int
	duration  time.Duration
}

type agentInvoker func(ctx context.Context, agentURL string,
	contextDoc string, docs []string, prompt string,
) (string, error)

func runBatch(ctx context.Context, cfg batchConfig,
	invoke agentInvoker, output io.Writer,
) (*batchStats, error) {
	start := time.Now()
	batches := splitDocuments(cfg.documents, len(cfg.agents))

	type workerResult struct {
		agentURL string
		results  []candidateResult
		err      error
	}

	results := make([]workerResult, len(batches))
	g, gCtx := errgroup.WithContext(ctx)

	for i, batch := range batches {
		g.Go(func() error {
			text, err := invoke(gCtx, cfg.agents[i], cfg.contextDoc,
				batch, cfg.prompt)
			if err != nil {
				results[i] = workerResult{
					agentURL: cfg.agents[i],
					err:      err,
				}
				return nil // don't cancel other workers
			}
			parsed, err := parseBatchResult(text)
			if err != nil {
				results[i] = workerResult{
					agentURL: cfg.agents[i],
					err:      fmt.Errorf("parse error: %w", err),
				}
				return nil
			}
			results[i] = workerResult{
				agentURL: cfg.agents[i],
				results:  parsed,
			}
			return nil
		})
	}

	_ = g.Wait()

	var allResults []candidateResult
	stats := &batchStats{agents: len(batches)}
	for _, wr := range results {
		if wr.err != nil {
			slog.Warn("worker failed",
				"agent", wr.agentURL,
				"error", wr.err)
			continue
		}
		allResults = append(allResults, wr.results...)
		stats.processed += len(wr.results)
	}
	stats.failed = len(cfg.documents) - stats.processed
	stats.duration = time.Since(start)

	if len(allResults) > 0 {
		if err := formatCSV(output, allResults); err != nil {
			return stats, fmt.Errorf("CSV output error: %w", err)
		}
	}

	return stats, nil
}

func newA2AInvoker(log *slog.Logger) agentInvoker {
	client := bridge.NewA2AClient(&http.Client{
		Timeout: 5 * time.Minute,
	}, log)

	return func(ctx context.Context, agentURL string,
		contextDoc string, docs []string, prompt string,
	) (string, error) {
		docList := strings.Join(docs, ", ")
		message := fmt.Sprintf(
			"%s\n\nContext document: %s\nDocuments to evaluate: %s",
			prompt, contextDoc, docList)

		result, err := client.Invoke(ctx, &bridge.InvokeRequest{
			AgentURL:    agentURL,
			MessageText: message,
		})
		if err != nil {
			return "", err
		}
		return result.Text, nil
	}
}

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Fan out document processing across multiple agents",
	Long: "Distribute document evaluation across N running " +
		"agents via A2A, collect results, and produce a " +
		"ranked CSV report.",
	RunE: runBatchCmd,
}

func init() {
	rootCmd.AddCommand(batchCmd)

	batchCmd.Flags().StringSlice("agents", nil,
		"Agent endpoint URLs (required)")
	batchCmd.Flags().StringSlice("documents", nil,
		"Document IDs to process (required)")
	batchCmd.Flags().String("context-doc", "",
		"Context document ID (e.g., job description)")
	batchCmd.Flags().String("prompt", "",
		"Prompt template for agents (required)")
	batchCmd.Flags().String("output", "",
		"Output file path (default: stdout)")
	batchCmd.Flags().Duration("timeout", 10*time.Minute,
		"Overall timeout for batch processing")

	_ = batchCmd.MarkFlagRequired("agents")
	_ = batchCmd.MarkFlagRequired("documents")
	_ = batchCmd.MarkFlagRequired("prompt")
}

func runBatchCmd(cmd *cobra.Command, _ []string) error {
	agents, _ := cmd.Flags().GetStringSlice("agents")
	documents, _ := cmd.Flags().GetStringSlice("documents")
	contextDoc, _ := cmd.Flags().GetString("context-doc")
	prompt, _ := cmd.Flags().GetString("prompt")
	outputPath, _ := cmd.Flags().GetString("output")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	log := slog.Default()
	log.Info("starting batch processing",
		"agents", len(agents),
		"documents", len(documents),
		"context_doc", contextDoc)

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	cfg := batchConfig{
		agents:     agents,
		documents:  documents,
		contextDoc: contextDoc,
		prompt:     prompt,
		timeout:    timeout,
	}

	var output io.Writer = os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		output = f
	}

	invoker := newA2AInvoker(log)
	stats, err := runBatch(ctx, cfg, invoker, output)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nBatch complete.\n")
	fmt.Fprintf(os.Stderr, "  Documents processed: %d/%d\n",
		stats.processed, len(documents))
	fmt.Fprintf(os.Stderr, "  Agents used:         %d\n",
		stats.agents)
	fmt.Fprintf(os.Stderr, "  Wall-clock time:     %s\n",
		stats.duration.Round(time.Second))
	if stats.failed > 0 {
		fmt.Fprintf(os.Stderr,
			"  WARNING: %d documents not processed\n",
			stats.failed)
	}

	return nil
}
```

- [ ] **Step 4: Fix imports at the top of batch.go**

The final import block for `internal/cmd/batch.go` should be:

```go
package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redhat-et/docsclaw/internal/bridge"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)
```

- [ ] **Step 5: Run all batch tests**

Run: `go test ./internal/cmd/ -run "TestSplit|TestParse|TestFormat|TestRunBatch" -v`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `make test`
Expected: PASS

- [ ] **Step 7: Run linter**

Run: `make lint`
Expected: PASS (fix any issues)

- [ ] **Step 8: Commit**

```bash
git add internal/cmd/batch.go internal/cmd/batch_test.go
git commit -s -m "feat: add docsclaw batch subcommand with A2A fan-out"
```

---

## Task 4: HR scenario — agent configuration

Create the system prompt, agent config, and agent card for the
HR resume screening worker agents.

**Files:**

- Create: `demo/batch/hr/system-prompt.txt`
- Create: `demo/batch/hr/agent-config.yaml`
- Create: `demo/batch/hr/agent-card.json`

- [ ] **Step 1: Create demo directory structure**

```bash
mkdir -p demo/batch/hr demo/batch/security demo/batch/finance
```

- [ ] **Step 2: Create HR system prompt**

Write `demo/batch/hr/system-prompt.txt`:

```text
You are an HR analyst specializing in resume screening for
technology companies. You evaluate candidates against job
requirements with consistent, fair scoring criteria.

When given a job description and resumes to evaluate, you must:

1. Fetch the job description document using the context document
   ID provided.
2. Fetch each resume document using the document IDs provided.
3. Score each candidate on a scale of 1-10 based on alignment
   with the job requirements.
4. Return your evaluation as a JSON array.

Scoring rubric:
- 9-10: Exceeds all requirements, strong hire signal
- 7-8: Meets most requirements, some preferred qualifications
- 5-6: Meets minimum requirements, gaps in preferred areas
- 3-4: Partially qualified, significant gaps
- 1-2: Does not meet minimum requirements

Your response MUST be a JSON array with this exact structure:

```json
[
  {
    "document_id": "DOC-R001",
    "candidate_name": "Full Name",
    "score": 8,
    "strengths": "Brief summary of strengths",
    "weaknesses": "Brief summary of gaps",
    "recommendation": "Strong hire / Hire / Maybe / Pass"
  }
]
```

Be objective. Evaluate based on evidence in the resume, not
assumptions. If information is missing, note it as a gap rather
than inferring.
```

- [ ] **Step 3: Create HR agent config**

Write `demo/batch/hr/agent-config.yaml`:

```yaml
tools:
  allowed:
    - fetch_document
  workspace: /tmp/agent-workspace

loop:
  max_iterations: 25
```

Note: `max_iterations` is set to 25 to allow the agent to fetch
up to 10 resumes + 1 job description + produce results within a
single agentic loop.

- [ ] **Step 4: Create HR agent card**

Write `demo/batch/hr/agent-card.json`:

```json
{
  "name": "hr-resume-screener",
  "description": "Screens resumes against job requirements and produces scored rankings",
  "version": "1.0.0",
  "protocolVersion": "0.3.0",
  "url": "http://hr-worker:8000",
  "skills": [
    {
      "id": "resume-screening",
      "name": "Resume Screening",
      "description": "Evaluates resumes against job requirements with consistent scoring",
      "tags": ["hr", "resume", "screening", "scoring"],
      "examples": [
        "Score these resumes against the job description",
        "Evaluate candidates for the Senior PM role"
      ]
    }
  ],
  "capabilities": {},
  "defaultInputModes": ["application/json"],
  "defaultOutputModes": ["text/plain"]
}
```

- [ ] **Step 5: Commit**

```bash
git add demo/batch/hr/
git commit -s -m "feat: add HR resume screening agent configuration"
```

---

## Task 5: Security and finance scenario configurations

Create the agent configurations for the remaining two demo
scenarios.

**Files:**

- Create: `demo/batch/security/system-prompt.txt`
- Create: `demo/batch/security/agent-config.yaml`
- Create: `demo/batch/security/agent-card.json`
- Create: `demo/batch/finance/system-prompt.txt`
- Create: `demo/batch/finance/agent-config.yaml`
- Create: `demo/batch/finance/agent-card.json`

- [ ] **Step 1: Create security analyst system prompt**

Write `demo/batch/security/system-prompt.txt`:

```text
You are a security analyst specializing in vulnerability
management and risk assessment. You triage vulnerability scan
findings by cross-referencing against asset inventory and SLA
requirements.

When given a vulnerability report and asset inventory, you must:

1. Fetch the vulnerability report document.
2. Fetch the asset inventory document.
3. Cross-reference each finding against the asset inventory to
   determine the owning team and SLA tier.
4. Prioritize findings by business impact: SLA breaches first,
   then critical severity, then high severity.
5. Group findings by team ownership.

Your response should be a structured remediation report in
markdown format:

## SLA Breaches (Immediate Action Required)
- CVE-XXXX-XXXX | Host: ... | Team: ... | Days overdue: ...

## Critical Findings
- CVE-XXXX-XXXX | Host: ... | Team: ... | SLA deadline: ...

## High Findings
- ...

## Summary
- Total findings: N
- SLA breaches: N
- By team: Team A (N), Team B (N), ...
- Recommended actions: ...

Be precise. Reference specific CVE IDs, hosts, and SLA
deadlines. Flag any findings where the asset is not in the
inventory.
```

- [ ] **Step 2: Create security agent config**

Write `demo/batch/security/agent-config.yaml`:

```yaml
tools:
  allowed:
    - fetch_document
  workspace: /tmp/agent-workspace

loop:
  max_iterations: 10
```

- [ ] **Step 3: Create security agent card**

Write `demo/batch/security/agent-card.json`:

```json
{
  "name": "security-analyst",
  "description": "Triages vulnerability scan findings against asset inventory and SLA requirements",
  "version": "1.0.0",
  "protocolVersion": "0.3.0",
  "url": "http://security-analyst:8000",
  "skills": [
    {
      "id": "vuln-triage",
      "name": "Vulnerability Triage",
      "description": "Prioritizes vulnerability findings by business impact and SLA compliance",
      "tags": ["security", "vulnerability", "triage", "compliance"],
      "examples": [
        "Triage the weekly vulnerability scan report",
        "Prioritize findings by SLA breach status"
      ]
    }
  ],
  "capabilities": {},
  "defaultInputModes": ["application/json"],
  "defaultOutputModes": ["text/plain"]
}
```

- [ ] **Step 4: Create finance analyst system prompt**

Write `demo/batch/finance/system-prompt.txt`:

```text
You are a procurement analyst specializing in invoice
verification and contract compliance. You compare invoices
against contracted rates to identify anomalies.

When given invoices and contracts, you must:

1. Fetch each document provided.
2. Compare each invoice line item against the corresponding
   contract's agreed rates.
3. Identify anomalies: rate deviations, duplicate charges,
   line items not in the contract, unusual patterns.

Your response should be a structured anomaly report in markdown
format:

## Rate Deviations
- Vendor: ... | Invoice: ... | Line item: ...
  Contracted rate: $X | Invoiced rate: $Y | Deviation: Z%

## Duplicate Charges
- Vendor: ... | Invoice: ... | Amount: $X | Appears in: ...

## Uncontracted Line Items
- Vendor: ... | Invoice: ... | Line item: ...
  (not found in contract)

## Unusual Patterns
- Vendor: ... | Observation: ...

## Summary
- Invoices reviewed: N
- Total anomalies found: N
- Estimated overcharge: $X
- Vendors requiring follow-up: ...

Be precise with dollar amounts and percentages. Reference
specific invoice numbers and contract sections.
```

- [ ] **Step 5: Create finance agent config**

Write `demo/batch/finance/agent-config.yaml`:

```yaml
tools:
  allowed:
    - fetch_document
  workspace: /tmp/agent-workspace

loop:
  max_iterations: 15
```

- [ ] **Step 6: Create finance agent card**

Write `demo/batch/finance/agent-card.json`:

```json
{
  "name": "finance-analyst",
  "description": "Compares invoices against contracts to detect anomalies and overcharges",
  "version": "1.0.0",
  "protocolVersion": "0.3.0",
  "url": "http://finance-analyst:8000",
  "skills": [
    {
      "id": "invoice-audit",
      "name": "Invoice Audit",
      "description": "Detects rate deviations, duplicates, and uncontracted charges",
      "tags": ["finance", "procurement", "invoice", "audit"],
      "examples": [
        "Review Q1 invoices against vendor contracts",
        "Flag any charges that deviate from contracted rates"
      ]
    }
  ],
  "capabilities": {},
  "defaultInputModes": ["application/json"],
  "defaultOutputModes": ["text/plain"]
}
```

- [ ] **Step 7: Commit**

```bash
git add demo/batch/security/ demo/batch/finance/
git commit -s -m "feat: add security and finance agent configurations"
```

---

## Task 6: Purpose-built Dockerfiles

Create the three Dockerfiles with scenario-specific
preprocessing tools.

**Files:**

- Create: `demo/batch/Dockerfile.hr`
- Create: `demo/batch/Dockerfile.security`
- Create: `demo/batch/Dockerfile.finance`

- [ ] **Step 1: Create HR Dockerfile**

Write `demo/batch/Dockerfile.hr`:

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o docsclaw ./cmd/docsclaw

FROM alpine:3.23

RUN apk add --no-cache poppler-utils ca-certificates \
    && rm -rf /var/cache/apk/*

COPY --from=builder /build/docsclaw /usr/local/bin/docsclaw

USER 65534:65534

EXPOSE 8000 8100

ENTRYPOINT ["docsclaw"]
CMD ["serve", "--config-dir", "/config/agent", "--listen-plain-http"]
```

- [ ] **Step 2: Create security Dockerfile**

Write `demo/batch/Dockerfile.security`:

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o docsclaw ./cmd/docsclaw

FROM alpine:3.23

RUN apk add --no-cache jq csvtool ca-certificates \
    && rm -rf /var/cache/apk/*

COPY --from=builder /build/docsclaw /usr/local/bin/docsclaw

USER 65534:65534

EXPOSE 8000 8100

ENTRYPOINT ["docsclaw"]
CMD ["serve", "--config-dir", "/config/agent", "--listen-plain-http"]
```

- [ ] **Step 3: Create finance Dockerfile**

Write `demo/batch/Dockerfile.finance`:

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o docsclaw ./cmd/docsclaw

FROM alpine:3.23

RUN apk add --no-cache gnumeric ca-certificates \
    && rm -rf /var/cache/apk/*

COPY --from=builder /build/docsclaw /usr/local/bin/docsclaw

USER 65534:65534

EXPOSE 8000 8100

ENTRYPOINT ["docsclaw"]
CMD ["serve", "--config-dir", "/config/agent", "--listen-plain-http"]
```

Note: `gnumeric` provides `ssconvert` for XLSX-to-CSV conversion
and is available in Alpine's package repository. The `xlsx2csv`
Python tool would require installing Python, defeating the
minimal image goal.

- [ ] **Step 4: Verify Dockerfiles build (local test)**

```bash
docker build -f demo/batch/Dockerfile.hr -t docsclaw-hr:test .
docker build -f demo/batch/Dockerfile.security -t docsclaw-security:test .
docker build -f demo/batch/Dockerfile.finance -t docsclaw-finance:test .
```

Check image sizes:

```bash
docker images | grep docsclaw
```

Expected: each image under 30 MiB (alpine base + tools + binary).

- [ ] **Step 5: Commit**

```bash
git add demo/batch/Dockerfile.*
git commit -s -m "feat: add purpose-built Dockerfiles for demo scenarios"
```

---

## Task 7: Kubernetes deployment manifests

Create deployment manifests for all three namespaces. The HR
namespace needs templated worker manifests.

**Files:**

- Create: `demo/batch/hr/deployment.yaml`
- Create: `demo/batch/security/deployment.yaml`
- Create: `demo/batch/finance/deployment.yaml`
- Create: `demo/batch/namespace-setup.yaml`

- [ ] **Step 1: Create namespace setup manifest**

Write `demo/batch/namespace-setup.yaml`:

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: demo-hr
---
apiVersion: v1
kind: Namespace
metadata:
  name: demo-security
---
apiVersion: v1
kind: Namespace
metadata:
  name: demo-finance
```

- [ ] **Step 2: Create HR deployment manifest**

This manifest includes the ConfigMap, a single worker Deployment
template (replicated by the demo script), and the Service. Write
`demo/batch/hr/deployment.yaml`:

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: hr-worker-config
  namespace: demo-hr
  labels:
    app: hr-worker
data:
  system-prompt.txt: |
    You are an HR analyst specializing in resume screening for
    technology companies. You evaluate candidates against job
    requirements with consistent, fair scoring criteria.

    When given a job description and resumes to evaluate, you must:

    1. Fetch the job description document using the context
       document ID provided.
    2. Fetch each resume document using the document IDs provided.
    3. Score each candidate on a scale of 1-10 based on alignment
       with the job requirements.
    4. Return your evaluation as a JSON array.

    Scoring rubric:
    - 9-10: Exceeds all requirements, strong hire signal
    - 7-8: Meets most requirements, some preferred qualifications
    - 5-6: Meets minimum requirements, gaps in preferred areas
    - 3-4: Partially qualified, significant gaps
    - 1-2: Does not meet minimum requirements

    Your response MUST be a JSON array with this exact structure:

    [
      {
        "document_id": "DOC-R001",
        "candidate_name": "Full Name",
        "score": 8,
        "strengths": "Brief summary of strengths",
        "weaknesses": "Brief summary of gaps",
        "recommendation": "Strong hire / Hire / Maybe / Pass"
      }
    ]

    Be objective. Evaluate based on evidence in the resume, not
    assumptions. If information is missing, note it as a gap
    rather than inferring.

  agent-config.yaml: |
    tools:
      allowed:
        - fetch_document
      workspace: /tmp/agent-workspace
    loop:
      max_iterations: 25

  agent-card.json: |
    {
      "name": "hr-resume-screener",
      "description": "Screens resumes against job requirements",
      "version": "1.0.0",
      "protocolVersion": "0.3.0",
      "url": "http://hr-worker:8000",
      "skills": [],
      "capabilities": {},
      "defaultInputModes": ["application/json"],
      "defaultOutputModes": ["text/plain"]
    }

---
# Template for hr-worker-N. The demo script creates 10 copies
# with sed: replace WORKER_INDEX with 1..10.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hr-worker-WORKER_INDEX
  namespace: demo-hr
  labels:
    app: hr-worker
    instance: hr-worker-WORKER_INDEX
spec:
  replicas: 1
  selector:
    matchLabels:
      instance: hr-worker-WORKER_INDEX
  template:
    metadata:
      labels:
        app: hr-worker
        instance: hr-worker-WORKER_INDEX
    spec:
      securityContext:
        runAsNonRoot: true
      containers:
        - name: docsclaw
          image: ghcr.io/redhat-et/docsclaw-hr:latest
          args:
            - serve
            - --config-dir
            - /config/agent
            - --listen-plain-http
            - --document-service-url
            - http://document-service.demo-hr.svc:8080
          ports:
            - containerPort: 8000
              name: http
            - containerPort: 8100
              name: health
          envFrom:
            - secretRef:
                name: llm-secret
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /health
              port: health
            initialDelaySeconds: 5
          readinessProbe:
            httpGet:
              path: /ready
              port: health
            initialDelaySeconds: 3
          volumeMounts:
            - name: agent-config
              mountPath: /config/agent
              readOnly: true
            - name: workspace
              mountPath: /tmp/agent-workspace
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
            readOnlyRootFilesystem: true
      volumes:
        - name: agent-config
          configMap:
            name: hr-worker-config
        - name: workspace
          emptyDir: {}

---
apiVersion: v1
kind: Service
metadata:
  name: hr-worker-WORKER_INDEX
  namespace: demo-hr
  labels:
    app: hr-worker
    instance: hr-worker-WORKER_INDEX
spec:
  selector:
    instance: hr-worker-WORKER_INDEX
  ports:
    - port: 8080
      targetPort: 8000
      name: http
```

- [ ] **Step 3: Create security deployment manifest**

Write `demo/batch/security/deployment.yaml`:

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: security-analyst-config
  namespace: demo-security
  labels:
    app: security-analyst
data:
  system-prompt.txt: |
    You are a security analyst specializing in vulnerability
    management and risk assessment. You triage vulnerability scan
    findings by cross-referencing against asset inventory and SLA
    requirements.

    When given a vulnerability report and asset inventory, you
    must:

    1. Fetch the vulnerability report document.
    2. Fetch the asset inventory document.
    3. Cross-reference each finding against the asset inventory
       to determine the owning team and SLA tier.
    4. Prioritize findings by business impact: SLA breaches
       first, then critical severity, then high severity.
    5. Group findings by team ownership.

    Your response should be a structured remediation report in
    markdown format with sections for SLA breaches, critical
    findings, high findings, and a summary.

    Be precise. Reference specific CVE IDs, hosts, and SLA
    deadlines.

  agent-config.yaml: |
    tools:
      allowed:
        - fetch_document
      workspace: /tmp/agent-workspace
    loop:
      max_iterations: 10

  agent-card.json: |
    {
      "name": "security-analyst",
      "description": "Triages vulnerability findings by impact and SLA",
      "version": "1.0.0",
      "protocolVersion": "0.3.0",
      "url": "http://security-analyst:8000",
      "skills": [],
      "capabilities": {},
      "defaultInputModes": ["application/json"],
      "defaultOutputModes": ["text/plain"]
    }

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: security-analyst
  namespace: demo-security
  labels:
    app: security-analyst
spec:
  replicas: 1
  selector:
    matchLabels:
      app: security-analyst
  template:
    metadata:
      labels:
        app: security-analyst
    spec:
      securityContext:
        runAsNonRoot: true
      containers:
        - name: docsclaw
          image: ghcr.io/redhat-et/docsclaw-security:latest
          args:
            - serve
            - --config-dir
            - /config/agent
            - --listen-plain-http
            - --document-service-url
            - http://document-service.demo-security.svc:8080
          ports:
            - containerPort: 8000
              name: http
            - containerPort: 8100
              name: health
          envFrom:
            - secretRef:
                name: llm-secret
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /health
              port: health
            initialDelaySeconds: 5
          readinessProbe:
            httpGet:
              path: /ready
              port: health
            initialDelaySeconds: 3
          volumeMounts:
            - name: agent-config
              mountPath: /config/agent
              readOnly: true
            - name: workspace
              mountPath: /tmp/agent-workspace
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
            readOnlyRootFilesystem: true
      volumes:
        - name: agent-config
          configMap:
            name: security-analyst-config
        - name: workspace
          emptyDir: {}

---
apiVersion: v1
kind: Service
metadata:
  name: security-analyst
  namespace: demo-security
  labels:
    app: security-analyst
spec:
  selector:
    app: security-analyst
  ports:
    - port: 8080
      targetPort: 8000
      name: http
```

- [ ] **Step 4: Create finance deployment manifest**

Write `demo/batch/finance/deployment.yaml`:

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: finance-analyst-config
  namespace: demo-finance
  labels:
    app: finance-analyst
data:
  system-prompt.txt: |
    You are a procurement analyst specializing in invoice
    verification and contract compliance. You compare invoices
    against contracted rates to identify anomalies.

    When given invoices and contracts, you must:

    1. Fetch each document provided.
    2. Compare each invoice line item against the corresponding
       contract's agreed rates.
    3. Identify anomalies: rate deviations, duplicate charges,
       line items not in the contract, unusual patterns.

    Your response should be a structured anomaly report in
    markdown format with sections for rate deviations, duplicate
    charges, uncontracted line items, unusual patterns, and a
    summary.

    Be precise with dollar amounts and percentages. Reference
    specific invoice numbers and contract sections.

  agent-config.yaml: |
    tools:
      allowed:
        - fetch_document
      workspace: /tmp/agent-workspace
    loop:
      max_iterations: 15

  agent-card.json: |
    {
      "name": "finance-analyst",
      "description": "Detects invoice anomalies against contracts",
      "version": "1.0.0",
      "protocolVersion": "0.3.0",
      "url": "http://finance-analyst:8000",
      "skills": [],
      "capabilities": {},
      "defaultInputModes": ["application/json"],
      "defaultOutputModes": ["text/plain"]
    }

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: finance-analyst
  namespace: demo-finance
  labels:
    app: finance-analyst
spec:
  replicas: 1
  selector:
    matchLabels:
      app: finance-analyst
  template:
    metadata:
      labels:
        app: finance-analyst
    spec:
      securityContext:
        runAsNonRoot: true
      containers:
        - name: docsclaw
          image: ghcr.io/redhat-et/docsclaw-finance:latest
          args:
            - serve
            - --config-dir
            - /config/agent
            - --listen-plain-http
            - --document-service-url
            - http://document-service.demo-finance.svc:8080
          ports:
            - containerPort: 8000
              name: http
            - containerPort: 8100
              name: health
          envFrom:
            - secretRef:
                name: llm-secret
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /health
              port: health
            initialDelaySeconds: 5
          readinessProbe:
            httpGet:
              path: /ready
              port: health
            initialDelaySeconds: 3
          volumeMounts:
            - name: agent-config
              mountPath: /config/agent
              readOnly: true
            - name: workspace
              mountPath: /tmp/agent-workspace
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
            readOnlyRootFilesystem: true
      volumes:
        - name: agent-config
          configMap:
            name: finance-analyst-config
        - name: workspace
          emptyDir: {}

---
apiVersion: v1
kind: Service
metadata:
  name: finance-analyst
  namespace: demo-finance
  labels:
    app: finance-analyst
spec:
  selector:
    app: finance-analyst
  ports:
    - port: 8080
      targetPort: 8000
      name: http
```

- [ ] **Step 5: Commit**

```bash
git add demo/batch/namespace-setup.yaml \
        demo/batch/hr/deployment.yaml \
        demo/batch/security/deployment.yaml \
        demo/batch/finance/deployment.yaml
git commit -s -m "feat: add Kubernetes deployment manifests for demo scenarios"
```

---

## Task 8: Synthetic test data generator

Python script to generate 100 resumes, a job description,
security data, and finance data as markdown documents ready for
seeding into document-service.

**Files:**

- Create: `scripts/generate-demo-data.py`

- [ ] **Step 1: Create the data generator script**

Write `scripts/generate-demo-data.py`:

```python
#!/usr/bin/env python3
"""Generate synthetic demo data for DocsClaw batch processing demo.

Outputs JSON files to demo/batch/data/ that can be loaded into
document-service via the seed script.

Usage:
    python3 scripts/generate-demo-data.py
"""

import json
import os
import random

OUTPUT_DIR = os.path.join(os.path.dirname(__file__),
                          "..", "demo", "batch", "data")

FIRST_NAMES = [
    "James", "Maria", "Chen", "Aisha", "Dmitri", "Priya",
    "Carlos", "Yuki", "Fatima", "Oleksandr", "Sarah", "Ravi",
    "Emma", "Kofi", "Ingrid", "Ahmed", "Lucia", "Hiroshi",
    "Zara", "Mikhail", "Ana", "Raj", "Sofia", "Kwame",
    "Elena", "Omar", "Linnea", "Takeshi", "Amara", "Viktor",
    "Isabella", "Jin", "Nadia", "Alejandro", "Anya", "Sanjay",
    "Clara", "Emeka", "Marta", "Kenji", "Leila", "Andrei",
    "Diana", "Hassan", "Freya", "Arjun", "Rosa", "Taro",
    "Amina", "Boris",
]

LAST_NAMES = [
    "Anderson", "Patel", "Kim", "Okafor", "Mueller", "Singh",
    "Rodriguez", "Tanaka", "Al-Hassan", "Petrov", "Johnson",
    "Gupta", "Williams", "Mensah", "Johansson", "Ibrahim",
    "Garcia", "Watanabe", "Osei", "Kozlov", "Martinez", "Kumar",
    "Brown", "Asante", "Lindberg", "Nakamura", "Diallo",
    "Volkov", "Rossi", "Chen", "Ahmadi", "Fernandez", "Sato",
    "Mwangi", "Larsson", "Sharma", "Costa", "Suzuki", "Bello",
    "Novak", "Lee", "Popov", "Santos", "Yamamoto", "Okonkwo",
    "Berg", "Reddy", "Silva", "Takahashi", "Ivanov",
]

COMPANIES = [
    "Acme Corp", "TechVista", "DataForge", "CloudPeak",
    "NexGen Labs", "Quantum Software", "SynergyTech",
    "Pinnacle AI", "CoreLogic", "Atlas Systems",
    "ByteWorks", "Streamline Inc", "FusionPoint",
    "Catalyst Digital", "Meridian Tech", "Elevate SaaS",
    "Vanguard Platform", "Horizon Computing", "Apex Solutions",
    "Nimbus Technologies",
]

PM_TITLES = [
    "Group Product Manager", "Senior Product Manager",
    "Product Manager", "Director of Product",
    "VP of Product", "Principal Product Manager",
]

ADJACENT_TITLES = [
    "Senior Project Manager", "Business Analyst",
    "Program Manager", "Scrum Master",
    "Technical Program Manager", "Product Marketing Manager",
    "Solutions Architect", "Engineering Manager",
]

UNRELATED_TITLES = [
    "Software Engineer", "UX Designer", "Data Scientist",
    "DevOps Engineer", "QA Engineer", "Frontend Developer",
    "Marketing Specialist", "Sales Representative",
    "Technical Writer", "Database Administrator",
]


def generate_job_description():
    return {
        "id": "DOC-JD001",
        "title": "Job Description - Senior Product Manager",
        "sensitivity": "public",
        "required_department": "hr",
        "content": """# Senior Product Manager - API Platform

## About the role

We are seeking an experienced Senior Product Manager to lead our
API platform team. You will define the product roadmap, work
cross-functionally with engineering, design, and sales, and
drive measurable business outcomes.

## Required qualifications

- 5+ years of product management experience
- B2B SaaS background with enterprise customers
- Data-driven decision making with defined metrics and KPIs
- Cross-functional leadership experience (engineering, design,
  sales)
- Track record of shipping products that drive revenue growth

## Preferred qualifications

- Experience with API or platform products
- Technical background (CS degree or engineering experience)
- Experience with usage-based pricing models
- Enterprise sales cycle knowledge
- Public speaking or thought leadership

## Responsibilities

- Define and execute the 3-year product roadmap for the API
  platform
- Lead a cross-functional team of 8-12 engineers and designers
- Drive product-led growth metrics (adoption, retention, revenue)
- Conduct customer research and translate insights into product
  requirements
- Present to executive leadership on product strategy and results

## Scoring rubric for evaluators

- 9-10: Exceeds all required qualifications, has most preferred
- 7-8: Meets all required, some preferred qualifications
- 5-6: Meets most required qualifications, few preferred
- 3-4: Meets some required qualifications, significant gaps
- 1-2: Does not meet minimum requirements
""",
    }


def generate_resume(doc_id, tier):
    first = random.choice(FIRST_NAMES)
    last = random.choice(LAST_NAMES)
    name = f"{first} {last}"

    if tier == "strong":
        years = random.randint(7, 15)
        title = random.choice(PM_TITLES[:3])
        prev_title = random.choice(PM_TITLES[2:])
        content = f"""# {name}

## Contact
Email: {first.lower()}.{last.lower()}@email.com

## Summary
{years} years of product management experience in B2B SaaS,
specializing in API and platform products. Proven track record
of driving revenue growth through data-driven product strategy.

## Experience

### {title}, {random.choice(COMPANIES)} (2021-present)
- Leads API platform team of {random.randint(8, 15)} engineers
- Shipped usage-based billing that grew ARR {random.randint(20, 50)}%
- Defined 3-year platform roadmap adopted by executive team
- Drove {random.randint(15, 40)}% increase in developer adoption
- Conducts quarterly customer advisory board sessions

### {prev_title}, {random.choice(COMPANIES)} (2018-2021)
- Owned developer integrations and partner API program
- Launched {random.randint(8, 20)} new integrations in 18 months
- Increased weekly active API users by {random.randint(20, 35)}%
- Led cross-functional team of engineering, design, and sales

### Associate Product Manager, {random.choice(COMPANIES)} (2016-2018)
- API team member, led documentation redesign
- Reduced support tickets {random.randint(20, 40)}% through
  improved developer experience

## Education
{random.choice(["MS Computer Science", "MBA", "BS Computer Science"])}, {random.choice(["Stanford", "MIT", "Carnegie Mellon", "Wharton", "Berkeley"])} ({2016 - random.randint(0, 3)})

## Skills
Product strategy, API design, data analytics, SQL, A/B testing,
cross-functional leadership, enterprise sales, agile/scrum
"""

    elif tier == "moderate":
        years = random.randint(4, 7)
        title = random.choice(PM_TITLES[1:4])
        content = f"""# {name}

## Contact
Email: {first.lower()}.{last.lower()}@email.com

## Summary
{years} years of product management experience. Background in
SaaS products with focus on user-facing features.

## Experience

### {title}, {random.choice(COMPANIES)} (2020-present)
- Manages product roadmap for customer-facing dashboard
- Grew user engagement metrics by {random.randint(10, 25)}%
- Works with engineering team of {random.randint(4, 8)}
- Introduced A/B testing framework for feature releases

### Product Manager, {random.choice(COMPANIES)} (2018-2020)
- Owned mobile app product line
- Launched {random.randint(3, 8)} major features
- Coordinated with design and QA teams

## Education
{random.choice(["BS Business", "BA Economics", "BS Computer Science", "MBA"])}, {random.choice(["UC Berkeley", "Michigan", "Georgetown", "Duke"])} ({2018 - random.randint(0, 3)})

## Skills
Product management, Jira, data analysis, user research, agile
"""

    elif tier == "weak":
        years = random.randint(3, 10)
        title = random.choice(ADJACENT_TITLES)
        content = f"""# {name}

## Contact
Email: {first.lower()}.{last.lower()}@email.com

## Summary
{years} years in {title.lower()} roles. Looking to transition
into product management.

## Experience

### {title}, {random.choice(COMPANIES)} (2019-present)
- Managed project timelines and deliverables for {random.randint(3, 8)} projects
- Coordinated cross-team dependencies
- Tracked budgets and resource allocation
- {random.choice(["Earned PMP certification", "Led agile transformation initiative", "Managed vendor relationships"])}

### {random.choice(ADJACENT_TITLES)}, {random.choice(COMPANIES)} (2016-2019)
- Supported product team with requirements gathering
- Created project documentation and status reports
- Facilitated stakeholder meetings

## Education
{random.choice(["BS Business Administration", "BA Communications", "BS Industrial Engineering", "MBA"])}, {random.choice(["Ohio State", "Arizona State", "Florida", "Penn State"])} ({2016 - random.randint(0, 4)})

## Skills
Project management, Jira, Confluence, Excel, stakeholder
management, agile/scrum
"""

    else:  # poor
        years = random.randint(2, 8)
        title = random.choice(UNRELATED_TITLES)
        content = f"""# {name}

## Contact
Email: {first.lower()}.{last.lower()}@email.com

## Summary
{years} years as a {title.lower()}. Interested in exploring
product management opportunities.

## Experience

### {title}, {random.choice(COMPANIES)} (2020-present)
- {random.choice([
    "Develops backend services in Go and Python",
    "Designs user interfaces for web applications",
    "Builds machine learning models for recommendation systems",
    "Manages CI/CD pipelines and cloud infrastructure",
    "Writes test automation frameworks",
    "Creates marketing campaign materials",
    "Manages client accounts and renewals",
    "Writes technical documentation",
])}
- Works on team of {random.randint(3, 10)}

### {random.choice(UNRELATED_TITLES)}, {random.choice(COMPANIES)} (2017-2020)
- Individual contributor role
- {random.choice(["Participated in hackathons", "Mentored junior team members", "Contributed to open source projects"])}

## Education
{random.choice(["BS Computer Science", "BFA Graphic Design", "BS Mathematics", "BA English"])}, {random.choice(["Community College", "State University", "Technical Institute"])} ({2017 - random.randint(0, 3)})

## Skills
{random.choice([
    "Python, Go, SQL, Docker, Kubernetes",
    "Figma, Sketch, Adobe Creative Suite, HTML/CSS",
    "Python, R, TensorFlow, statistics",
    "AWS, Terraform, Jenkins, Linux",
    "Selenium, pytest, JUnit, test planning",
])}
"""

    return {
        "id": doc_id,
        "title": f"Resume - {name}",
        "sensitivity": "medium",
        "required_department": "hr",
        "content": content.strip(),
    }


def generate_vuln_report():
    teams = ["platform", "networking", "app-services", "data",
             "security"]
    hosts = []
    for team in teams:
        for i in range(1, random.randint(4, 8)):
            hosts.append(
                {"host": f"{team}-srv-{i:02d}",
                 "team": team,
                 "sla_tier": random.choice([1, 2, 2, 3, 3, 3])})

    findings = []
    for i in range(50):
        host = random.choice(hosts)
        severity = random.choices(
            ["critical", "high", "medium", "low"],
            weights=[5, 15, 50, 30])[0]
        days_ago = random.randint(1, 45)
        findings.append({
            "cve": f"CVE-2026-{random.randint(10000, 99999)}",
            "severity": severity,
            "host": host["host"],
            "description": random.choice([
                "Remote code execution in OpenSSL",
                "SQL injection in web framework",
                "Privilege escalation via kernel vulnerability",
                "Cross-site scripting in admin panel",
                "Denial of service in HTTP parser",
                "Information disclosure via debug endpoint",
                "Authentication bypass in API gateway",
                "Buffer overflow in image processing library",
                "Insecure deserialization in message queue",
                "Path traversal in file upload handler",
            ]),
            "discovered": f"{days_ago} days ago",
        })

    content = "# Weekly Vulnerability Scan Report\n\n"
    content += f"**Scan date:** 2026-05-05\n"
    content += f"**Total findings:** {len(findings)}\n\n"
    content += "## Findings\n\n"
    content += "| # | CVE | Severity | Host | Description | Discovered |\n"
    content += "|---|-----|----------|------|-------------|------------|\n"
    for i, f in enumerate(findings, 1):
        content += (f"| {i} | {f['cve']} | {f['severity']} | "
                    f"{f['host']} | {f['description']} | "
                    f"{f['discovered']} |\n")

    asset_content = "# Asset Inventory\n\n"
    asset_content += "| Host | Team | SLA Tier | SLA Deadline |\n"
    asset_content += "|------|------|----------|-------------|\n"
    for h in hosts:
        sla_days = {1: "24 hours", 2: "7 days", 3: "30 days"}
        asset_content += (f"| {h['host']} | {h['team']} | "
                          f"Tier {h['sla_tier']} | "
                          f"{sla_days[h['sla_tier']]} |\n")

    return [
        {
            "id": "DOC-VULN001",
            "title": "Weekly Vulnerability Scan Report - 2026-05-05",
            "sensitivity": "high",
            "required_department": "security",
            "content": content.strip(),
        },
        {
            "id": "DOC-ASSET001",
            "title": "Infrastructure Asset Inventory",
            "sensitivity": "medium",
            "required_department": "security",
            "content": asset_content.strip(),
        },
    ]


def generate_finance_data():
    vendors = [
        {"name": "CloudHost Inc", "id": "V001"},
        {"name": "DataPipe Solutions", "id": "V002"},
        {"name": "SecureNet Pro", "id": "V003"},
        {"name": "DevTools Global", "id": "V004"},
        {"name": "InfraScale Corp", "id": "V005"},
    ]

    documents = []

    for vendor in vendors:
        rates = {
            "compute": round(random.uniform(0.05, 0.15), 3),
            "storage": round(random.uniform(0.02, 0.08), 3),
            "bandwidth": round(random.uniform(0.01, 0.05), 3),
            "support": round(random.uniform(500, 2000), 2),
        }

        contract_content = f"# Service Agreement - {vendor['name']}\n\n"
        contract_content += f"**Contract ID:** CTR-{vendor['id']}\n"
        contract_content += "**Effective:** 2026-01-01 to 2026-12-31\n\n"
        contract_content += "## Agreed Rates\n\n"
        contract_content += "| Service | Unit | Rate |\n"
        contract_content += "|---------|------|------|\n"
        contract_content += (f"| Compute | per vCPU-hour | "
                             f"${rates['compute']:.3f} |\n")
        contract_content += (f"| Storage | per GB-month | "
                             f"${rates['storage']:.3f} |\n")
        contract_content += (f"| Bandwidth | per GB | "
                             f"${rates['bandwidth']:.3f} |\n")
        contract_content += (f"| Support | monthly flat | "
                             f"${rates['support']:.2f} |\n")

        documents.append({
            "id": f"DOC-CTR-{vendor['id']}",
            "title": f"Contract - {vendor['name']}",
            "sensitivity": "high",
            "required_department": "finance",
            "content": contract_content.strip(),
        })

        for month_num, month in enumerate(["January", "February",
                                            "March"], 1):
            invoice_content = (f"# Invoice - {vendor['name']} - "
                               f"{month} 2026\n\n")
            invoice_content += (f"**Invoice #:** INV-{vendor['id']}-"
                                f"2026-{month_num:02d}\n\n")
            invoice_content += "| Service | Usage | Rate | Amount |\n"
            invoice_content += "|---------|-------|------|--------|\n"

            compute_usage = random.randint(5000, 20000)
            storage_usage = random.randint(100, 1000)
            bw_usage = random.randint(500, 5000)

            # Introduce anomalies for specific vendors
            compute_rate = rates["compute"]
            storage_rate = rates["storage"]
            bw_rate = rates["bandwidth"]
            support_rate = rates["support"]

            # V002: 30% rate increase in March
            if vendor["id"] == "V002" and month == "March":
                compute_rate = round(compute_rate * 1.3, 3)

            # V004: duplicate support charge in February
            if vendor["id"] == "V004" and month == "February":
                invoice_content += (
                    f"| Compute | {compute_usage} vCPU-hrs | "
                    f"${compute_rate:.3f} | "
                    f"${compute_usage * compute_rate:.2f} |\n"
                    f"| Storage | {storage_usage} GB-mo | "
                    f"${storage_rate:.3f} | "
                    f"${storage_usage * storage_rate:.2f} |\n"
                    f"| Bandwidth | {bw_usage} GB | "
                    f"${bw_rate:.3f} | "
                    f"${bw_usage * bw_rate:.2f} |\n"
                    f"| Support | monthly | "
                    f"${support_rate:.2f} | "
                    f"${support_rate:.2f} |\n"
                    f"| Support (adjustment) | monthly | "
                    f"${support_rate:.2f} | "
                    f"${support_rate:.2f} |\n"
                )
            # V003: uncontracted "consulting" line item in January
            elif vendor["id"] == "V003" and month == "January":
                consulting_fee = round(random.uniform(3000, 8000), 2)
                invoice_content += (
                    f"| Compute | {compute_usage} vCPU-hrs | "
                    f"${compute_rate:.3f} | "
                    f"${compute_usage * compute_rate:.2f} |\n"
                    f"| Storage | {storage_usage} GB-mo | "
                    f"${storage_rate:.3f} | "
                    f"${storage_usage * storage_rate:.2f} |\n"
                    f"| Bandwidth | {bw_usage} GB | "
                    f"${bw_rate:.3f} | "
                    f"${bw_usage * bw_rate:.2f} |\n"
                    f"| Support | monthly | "
                    f"${support_rate:.2f} | "
                    f"${support_rate:.2f} |\n"
                    f"| Consulting services | 1 | "
                    f"${consulting_fee:.2f} | "
                    f"${consulting_fee:.2f} |\n"
                )
            else:
                invoice_content += (
                    f"| Compute | {compute_usage} vCPU-hrs | "
                    f"${compute_rate:.3f} | "
                    f"${compute_usage * compute_rate:.2f} |\n"
                    f"| Storage | {storage_usage} GB-mo | "
                    f"${storage_rate:.3f} | "
                    f"${storage_usage * storage_rate:.2f} |\n"
                    f"| Bandwidth | {bw_usage} GB | "
                    f"${bw_rate:.3f} | "
                    f"${bw_usage * bw_rate:.2f} |\n"
                    f"| Support | monthly | "
                    f"${support_rate:.2f} | "
                    f"${support_rate:.2f} |\n"
                )

            documents.append({
                "id": (f"DOC-INV-{vendor['id']}-"
                       f"2026-{month_num:02d}"),
                "title": (f"Invoice - {vendor['name']} - "
                          f"{month} 2026"),
                "sensitivity": "high",
                "required_department": "finance",
                "content": invoice_content.strip(),
            })

    return documents


def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    random.seed(42)

    # HR data
    hr_docs = [generate_job_description()]
    tiers = (["strong"] * 20 + ["moderate"] * 30 +
             ["weak"] * 30 + ["poor"] * 20)
    random.shuffle(tiers)
    for i, tier in enumerate(tiers, 1):
        hr_docs.append(generate_resume(f"DOC-R{i:03d}", tier))

    with open(os.path.join(OUTPUT_DIR, "hr-documents.json"), "w") as f:
        json.dump(hr_docs, f, indent=2)
    print(f"Generated {len(hr_docs)} HR documents")

    # Security data
    sec_docs = generate_vuln_report()
    with open(os.path.join(OUTPUT_DIR, "security-documents.json"),
              "w") as f:
        json.dump(sec_docs, f, indent=2)
    print(f"Generated {len(sec_docs)} security documents")

    # Finance data
    fin_docs = generate_finance_data()
    with open(os.path.join(OUTPUT_DIR, "finance-documents.json"),
              "w") as f:
        json.dump(fin_docs, f, indent=2)
    print(f"Generated {len(fin_docs)} finance documents")

    print(f"\nAll data written to {OUTPUT_DIR}/")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Run the generator**

```bash
python3 scripts/generate-demo-data.py
```

Expected output:

```text
Generated 101 HR documents
Generated 2 security documents
Generated 20 finance documents

All data written to demo/batch/data/
```

- [ ] **Step 3: Verify output**

```bash
python3 -c "import json; d=json.load(open('demo/batch/data/hr-documents.json')); print(f'{len(d)} docs, first: {d[0][\"id\"]} {d[0][\"title\"]}')"
python3 -c "import json; d=json.load(open('demo/batch/data/security-documents.json')); print(f'{len(d)} docs')"
python3 -c "import json; d=json.load(open('demo/batch/data/finance-documents.json')); print(f'{len(d)} docs')"
```

- [ ] **Step 4: Commit**

```bash
git add scripts/generate-demo-data.py demo/batch/data/
git commit -s -m "feat: add synthetic data generator for demo scenarios"
```

---

## Task 9: Document-service seed script

Shell script that loads generated data into document-service
instances.

**Files:**

- Create: `scripts/seed-demo-data.sh`

- [ ] **Step 1: Create the seed script**

Write `scripts/seed-demo-data.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="$SCRIPT_DIR/../demo/batch/data"

usage() {
    echo "Usage: $0 --scenario <hr|security|finance|all> --url <document-service-url>"
    echo ""
    echo "Load demo data into document-service."
    echo ""
    echo "Options:"
    echo "  --scenario    Which scenario data to load (hr, security, finance, all)"
    echo "  --url         Document-service base URL (e.g., http://localhost:8084)"
    exit 1
}

SCENARIO=""
BASE_URL=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --scenario) SCENARIO="$2"; shift 2 ;;
        --url)      BASE_URL="$2"; shift 2 ;;
        *)          usage ;;
    esac
done

[[ -z "$SCENARIO" || -z "$BASE_URL" ]] && usage

load_documents() {
    local file="$1"
    local count=0
    local total

    total=$(python3 -c "import json; print(len(json.load(open('$file'))))")
    echo "Loading $total documents from $(basename "$file")..."

    python3 -c "
import json, sys, urllib.request

docs = json.load(open('$file'))
url = '${BASE_URL}/documents'
success = 0
for doc in docs:
    body = json.dumps(doc).encode()
    req = urllib.request.Request(url, data=body,
        headers={'Content-Type': 'application/json'},
        method='POST')
    try:
        urllib.request.urlopen(req)
        success += 1
    except Exception as e:
        print(f'  WARN: {doc[\"id\"]}: {e}', file=sys.stderr)
print(f'  Loaded {success}/{len(docs)} documents')
"
}

case "$SCENARIO" in
    hr)
        load_documents "$DATA_DIR/hr-documents.json"
        ;;
    security)
        load_documents "$DATA_DIR/security-documents.json"
        ;;
    finance)
        load_documents "$DATA_DIR/finance-documents.json"
        ;;
    all)
        load_documents "$DATA_DIR/hr-documents.json"
        load_documents "$DATA_DIR/security-documents.json"
        load_documents "$DATA_DIR/finance-documents.json"
        ;;
    *)
        echo "ERROR: Unknown scenario: $SCENARIO"
        usage
        ;;
esac

echo "Done."
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x scripts/seed-demo-data.sh
```

- [ ] **Step 3: Commit**

```bash
git add scripts/seed-demo-data.sh
git commit -s -m "feat: add document-service seed script for demo data"
```

---

## Task 10: Demo runner script and README

End-to-end script that deploys everything and runs all three acts
in sequence, plus documentation.

**Files:**

- Create: `scripts/demo-run.sh`
- Create: `demo/batch/README.md`

- [ ] **Step 1: Create the demo runner script**

Write `scripts/demo-run.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEMO_DIR="$SCRIPT_DIR/../demo/batch"
WORKER_COUNT=${WORKER_COUNT:-10}

echo "=== DocsClaw Batch Processing Demo ==="
echo ""

# --- Act 0: Setup ---
echo "--- Act 0: Namespace & infrastructure setup ---"

oc apply -f "$DEMO_DIR/namespace-setup.yaml"

# Create LLM secret in each namespace
for ns in demo-hr demo-security demo-finance; do
    if ! oc get secret llm-secret -n "$ns" >/dev/null 2>&1; then
        echo "Creating LLM secret in $ns..."
        oc create secret generic llm-secret \
            --from-literal=LLM_API_KEY="$LLM_API_KEY" \
            -n "$ns"
    fi
done

echo ""
echo "--- Deploying document-service ---"
# Deploy document-service in each namespace
# (assumes document-service manifests are available)
echo "NOTE: Deploy document-service in each namespace manually"
echo "      or via your existing deployment scripts."
echo ""

# --- Act 1: HR Resume Screening ---
echo "--- Act 1: HR Resume Screening ---"
echo ""

# Generate worker manifests from template
echo "Generating $WORKER_COUNT worker manifests..."
for i in $(seq 1 "$WORKER_COUNT"); do
    sed "s/WORKER_INDEX/$i/g" \
        "$DEMO_DIR/hr/deployment.yaml" | \
        oc apply -f -
done

echo "Waiting for workers to be ready..."
for i in $(seq 1 "$WORKER_COUNT"); do
    oc rollout status "deployment/hr-worker-$i" -n demo-hr \
        --timeout=120s
done

echo ""
echo "Pod status:"
oc get pods -n demo-hr
echo ""
echo "Resource usage:"
oc top pods -n demo-hr 2>/dev/null || echo "(metrics not yet available)"
echo ""

# Seed HR data
echo "Seeding HR documents..."
HR_DOC_SVC=$(oc get svc document-service -n demo-hr \
    -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
if [[ -n "$HR_DOC_SVC" ]]; then
    "$SCRIPT_DIR/seed-demo-data.sh" \
        --scenario hr \
        --url "http://$HR_DOC_SVC:8080"
fi

# Build agent list
AGENTS=""
for i in $(seq 1 "$WORKER_COUNT"); do
    [[ -n "$AGENTS" ]] && AGENTS="$AGENTS,"
    AGENTS="${AGENTS}http://hr-worker-${i}.demo-hr.svc:8080"
done

# Build document list
DOC_LIST=$(python3 -c "print(','.join(f'DOC-R{i:03d}' for i in range(1, 101)))")

echo ""
echo "Starting batch processing..."
echo "  Agents: $WORKER_COUNT"
echo "  Documents: 100 resumes"
echo ""

docsclaw batch \
    --agents "$AGENTS" \
    --documents "$DOC_LIST" \
    --context-doc DOC-JD001 \
    --prompt "Fetch the job description (context document). Then fetch and evaluate each resume. Score each candidate 1-10 against the job requirements. Return a JSON array with document_id, candidate_name, score, strengths, weaknesses, and recommendation for each resume." \
    --output /tmp/hr-results.csv

echo ""
echo "Top 10 candidates:"
head -11 /tmp/hr-results.csv | column -t -s,
echo ""

# --- Act 2: Security Vulnerability Triage ---
echo "--- Act 2: Security Vulnerability Triage ---"
echo ""

oc apply -f "$DEMO_DIR/security/deployment.yaml"
oc rollout status deployment/security-analyst -n demo-security \
    --timeout=120s

echo "Pod status:"
oc get pods -n demo-security
echo ""

# --- Act 3: Finance Invoice Anomaly ---
echo "--- Act 3: Finance Invoice Anomaly Detection ---"
echo ""

oc apply -f "$DEMO_DIR/finance/deployment.yaml"
oc rollout status deployment/finance-analyst -n demo-finance \
    --timeout=120s

echo "Pod status:"
oc get pods -n demo-finance
echo ""

# --- Finale ---
echo "=== Finale: The Numbers ==="
echo ""
echo "All pods across demo namespaces:"
for ns in demo-hr demo-security demo-finance; do
    echo ""
    echo "--- $ns ---"
    oc top pods -n "$ns" 2>/dev/null || echo "(metrics pending)"
done

echo ""
echo "Demo complete."
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x scripts/demo-run.sh
```

- [ ] **Step 3: Create the demo README**

Write `demo/batch/README.md`:

```markdown
# Batch processing demo

Conference demo showcasing DocsClaw's lightweight agent runtime
through three business scenarios running on OpenShift.

## What this shows

- **HR resume screening:** 10 parallel agents score 100 resumes
  against a job description via A2A fan-out
- **Security vulnerability triage:** Single agent cross-references
  findings against asset inventory and SLA tiers
- **Finance invoice anomaly detection:** Single agent compares
  invoices against contracts to flag discrepancies

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

# Act 3: Finance (single agent)
oc apply -f demo/batch/finance/deployment.yaml
a2a send http://finance-analyst.demo-finance.svc:8080 "..."
```

## Cleanup

```bash
oc delete namespace demo-hr demo-security demo-finance
```

## Architecture

See `docs/superpowers/specs/2026-05-09-batch-demo-design.md`
for the full design spec.
```

- [ ] **Step 4: Commit**

```bash
git add scripts/demo-run.sh demo/batch/README.md
git commit -s -m "feat: add demo runner script and documentation"
```

---

## Task 11: Build verification

Full build, test, and lint pass to verify everything works
together.

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

```bash
make test
```

Expected: PASS

- [ ] **Step 2: Run linter**

```bash
make lint
```

Expected: PASS

- [ ] **Step 3: Build binary**

```bash
make build
```

Expected: Binary at `bin/docsclaw`

- [ ] **Step 4: Verify batch subcommand is registered**

```bash
./bin/docsclaw batch --help
```

Expected: Help text showing `--agents`, `--documents`,
`--context-doc`, `--prompt`, `--output` flags.

- [ ] **Step 5: Verify data generation**

```bash
python3 scripts/generate-demo-data.py
ls -la demo/batch/data/
```

Expected: Three JSON files with HR, security, and finance data.

- [ ] **Step 6: Build Docker images (if Docker available)**

```bash
docker build -f demo/batch/Dockerfile.hr -t docsclaw-hr:test .
docker images docsclaw-hr:test --format "{{.Size}}"
```

Expected: Image size under 30 MiB.
