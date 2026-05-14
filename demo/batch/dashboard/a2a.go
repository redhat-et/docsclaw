package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

var rpcID atomic.Int64

type A2AClient struct {
	httpClient *http.Client
}

func NewA2AClient() *A2AClient {
	return &A2AClient{httpClient: &http.Client{Timeout: 5 * time.Minute}}
}

type TaskStatus struct {
	ID       string
	State    string
	Artifact string
}

// SendMessage sends an A2A message/send request and returns the completed task.
// The call blocks until the agent finishes processing.
// Retries on transient errors (HTML responses, method not found) to handle
// Route propagation delays and pod startup races.
func (c *A2AClient) SendMessage(agentURL, text string) (TaskStatus, error) {
	const maxRetries = 5
	const retryDelay = 4 * time.Second

	var lastErr error
	for attempt := range maxRetries {
		status, err := c.sendOnce(agentURL, text)
		if err == nil {
			return status, nil
		}
		lastErr = err
		if attempt < maxRetries-1 {
			slog.Warn("A2A send retry", "agent", agentURL, "attempt", attempt+1, "error", err)
			time.Sleep(retryDelay)
		}
	}
	return TaskStatus{}, lastErr
}

func (c *A2AClient) sendOnce(agentURL, text string) (TaskStatus, error) {
	id := rpcID.Add(1)

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  "SendMessage",
		"id":      fmt.Sprintf("%d", id),
		"params": map[string]any{
			"message": map[string]any{
				"messageId": fmt.Sprintf("dash-%d", id),
				"role":      "user",
				"parts":     []map[string]any{{"kind": "text", "text": text}},
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return TaskStatus{}, err
	}

	resp, err := c.httpClient.Post(agentURL+"/a2a", "application/json", bytes.NewReader(body))
	if err != nil {
		return TaskStatus{}, fmt.Errorf("send to %s: %w", agentURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return TaskStatus{}, err
	}

	if resp.StatusCode >= 500 || (len(data) > 0 && data[0] == '<') {
		return TaskStatus{}, fmt.Errorf("agent not ready (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return TaskStatus{}, fmt.Errorf("parse response: %w", err)
	}
	if result.Error != nil {
		return TaskStatus{}, fmt.Errorf("A2A error: %s", result.Error.Message)
	}

	// Result is {"task": {"id": ..., "status": {"state": ...}, "artifacts": [...]}}
	var taskWrapper struct {
		Task json.RawMessage `json:"task"`
	}
	if err := json.Unmarshal(result.Result, &taskWrapper); err != nil {
		return TaskStatus{}, fmt.Errorf("parse task wrapper: %w", err)
	}

	taskData := taskWrapper.Task
	if taskData == nil {
		taskData = result.Result
	}

	taskID, _ := extractField(taskData, "id")
	state, _ := extractNestedField(taskData, "status", "state")
	artifact := extractArtifactText(taskData)

	return TaskStatus{
		ID:       taskID,
		State:    state,
		Artifact: artifact,
	}, nil
}

// GetTaskStatus polls the status of an A2A task.
func (c *A2AClient) GetTaskStatus(agentURL, taskID string) (TaskStatus, error) {
	id := rpcID.Add(1)

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  "GetTask",
		"id":      fmt.Sprintf("%d", id),
		"params":  map[string]any{"id": taskID},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return TaskStatus{}, err
	}

	resp, err := c.httpClient.Post(agentURL+"/a2a", "application/json", bytes.NewReader(body))
	if err != nil {
		return TaskStatus{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return TaskStatus{}, err
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return TaskStatus{}, err
	}
	if rpcResp.Error != nil {
		return TaskStatus{}, fmt.Errorf("A2A error: %s", rpcResp.Error.Message)
	}

	state, _ := extractNestedField(rpcResp.Result, "status", "state")
	artifact := extractArtifactText(rpcResp.Result)

	return TaskStatus{
		ID:       taskID,
		State:    state,
		Artifact: artifact,
	}, nil
}

func extractField(data json.RawMessage, key string) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", false
	}
	return s, true
}

func extractNestedField(data json.RawMessage, keys ...string) (string, bool) {
	current := data
	for i, key := range keys {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(current, &m); err != nil {
			return "", false
		}
		v, ok := m[key]
		if !ok {
			return "", false
		}
		if i == len(keys)-1 {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return "", false
			}
			return s, true
		}
		current = v
	}
	return "", false
}

func extractArtifactText(data json.RawMessage) string {
	var task struct {
		Artifacts []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &task); err != nil {
		return ""
	}
	for _, a := range task.Artifacts {
		for _, p := range a.Parts {
			if p.Text != "" {
				return p.Text
			}
		}
	}
	return ""
}
