package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WeaviateClient struct {
	baseURL    string
	collection string
	textField  string
	httpClient *http.Client
}

func NewWeaviateClient(baseURL, collection, textField string) (*WeaviateClient, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("weaviate: base URL is required")
	}
	if collection == "" {
		return nil, fmt.Errorf("weaviate: collection is required")
	}
	return &WeaviateClient{
		baseURL:    baseURL,
		collection: collection,
		textField:  textField,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (w *WeaviateClient) Search(ctx context.Context, query string, limit int) ([]Chunk, error) {
	gql := fmt.Sprintf(
		`{ Get { %s(nearText: {concepts: [%s]}, limit: %d) { %s _additional { id distance } } } }`,
		w.collection,
		quoteGraphQL(query),
		limit,
		w.textField,
	)

	body, err := json.Marshal(map[string]string{"query": gql})
	if err != nil {
		return nil, fmt.Errorf("weaviate: marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.baseURL+"/v1/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("weaviate: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weaviate: request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("weaviate: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weaviate: HTTP %d: %s", resp.StatusCode, respBody)
	}

	var result graphQLResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("weaviate: decode response: %w", err)
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("weaviate graphql: %s", strings.Join(msgs, "; "))
	}

	return w.parseChunks(result.Data)
}

func (w *WeaviateClient) parseChunks(data map[string]any) ([]Chunk, error) {
	getMap, ok := data["Get"].(map[string]any)
	if !ok {
		return nil, nil
	}
	items, ok := getMap[w.collection].([]any)
	if !ok {
		return nil, nil
	}

	chunks := make([]Chunk, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, _ := obj[w.textField].(string)
		additional, _ := obj["_additional"].(map[string]any)
		id, _ := additional["id"].(string)
		distance, _ := additional["distance"].(float64)

		chunks = append(chunks, Chunk{
			ID:    id,
			Text:  text,
			Score: 1 - distance,
			Metadata: map[string]any{
				"distance": distance,
			},
		})
	}
	return chunks, nil
}

type graphQLResponse struct {
	Data   map[string]any `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type graphQLError struct {
	Message string `json:"message"`
}

func quoteGraphQL(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
