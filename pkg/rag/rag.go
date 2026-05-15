package rag

import "context"

type Client interface {
	Search(ctx context.Context, query string, limit int) ([]Chunk, error)
}

type Chunk struct {
	ID       string
	Text     string
	Score    float64
	Metadata map[string]any
}
