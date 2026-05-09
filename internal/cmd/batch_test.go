package cmd

import (
	"strings"
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
