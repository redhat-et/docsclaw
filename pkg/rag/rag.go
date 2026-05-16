package rag

import "context"

type Client interface {
	Search(ctx context.Context, query string, limit int) ([]Chunk, error)
}

type Chunk struct {
	ID       string
	Text     string
	Score    float64         // similarity score (1 - distance); assumes cosine distance metric
	Metadata map[string]any
}
