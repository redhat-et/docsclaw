package cmd

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
