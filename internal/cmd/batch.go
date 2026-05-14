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

	if matches := jsonBlockRe.FindStringSubmatch(text); len(matches) > 1 {
		jsonStr = matches[1]
	}

	jsonStr = strings.TrimSpace(jsonStr)

	var results []candidateResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
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
				return nil
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
		defer func() {
			if cerr := f.Close(); cerr != nil {
				slog.Warn("failed to close output file", "error", cerr)
			}
		}()
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
