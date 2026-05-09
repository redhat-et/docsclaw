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
