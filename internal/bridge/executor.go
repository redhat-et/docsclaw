package bridge

import (
	"context"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/redhat-et/docsclaw/internal/logger"
)

// DocumentFetcher fetches a document by ID from the document-service.
// The bearerToken parameter is optional; when non-empty, it is forwarded
// as an Authorization header for JWT-based access control.
// Delegation context (X-Delegation-User/Agent headers) is injected
// automatically by DelegationTransport from the request context.
type DocumentFetcher func(ctx context.Context, documentID, bearerToken string) (map[string]any, error)

// LLMProcessor processes a document with an LLM and returns the result text.
type LLMProcessor func(ctx context.Context, title, content string) (string, error)

// MessageProcessor processes a free-form user message (no document).
// Used in phase 2 (tool-use) mode for standalone operation.
type MessageProcessor func(ctx context.Context, userMessage string) (string, error)

// AgentExecutor implements a2asrv.AgentExecutor by bridging A2A messages
// to document fetch and LLM processing.
type AgentExecutor struct {
	Log            *logger.Logger
	FetchDocument  DocumentFetcher
	ProcessLLM     LLMProcessor
	ProcessMessage MessageProcessor // optional: handles free-form messages
}

// Execute handles an incoming A2A message: extracts the document ID,
// fetches the document, processes it with the LLM, and yields results.
func (e *AgentExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		// Create task if new
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}

		// Transition to working state
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		// Extract bearer token and delegation context from ServiceParams.
		var bearerToken string
		var userSPIFFEID, agentSPIFFEID string
		if sp := execCtx.ServiceParams; sp != nil {
			if vals, found := sp.Get("authorization"); found && len(vals) > 0 {
				bearerToken = strings.TrimPrefix(vals[0], "Bearer ")
			}
			if vals, found := sp.Get("x-delegation-user"); found && len(vals) > 0 {
				userSPIFFEID = vals[0]
			}
			if vals, found := sp.Get("x-delegation-agent"); found && len(vals) > 0 {
				agentSPIFFEID = vals[0]
			}
		}

		// Store delegation context so DelegationTransport can inject headers
		// on outbound HTTP requests (e.g., to document-service).
		if userSPIFFEID != "" || agentSPIFFEID != "" {
			ctx = WithDelegation(ctx, DelegationContext{
				UserSPIFFEID:  userSPIFFEID,
				AgentSPIFFEID: agentSPIFFEID,
			})
		}

		// Extract document ID from the incoming message
		documentID, docErr := ExtractDocumentID(execCtx.Message)

		var result string

		if docErr != nil && e.ProcessMessage != nil {
			// Free-form message mode: no document ID, pass raw text
			// to the agentic loop
			userText := extractTextContent(execCtx.Message)
			e.Log.Info("A2A free-form request received",
				"text_length", len(userText))

			var err error
			result, err = e.ProcessMessage(ctx, userText)
			if err != nil {
				e.Log.Error("Message processing failed", "error", err)
				yield(e.failedEvent(execCtx, "Processing failed: "+err.Error()), nil)
				return
			}
		} else if docErr != nil {
			// No document ID and no free-form handler
			e.Log.Error("Failed to extract document ID from A2A message", "error", docErr)
			yield(e.failedEvent(execCtx, "Invalid request: "+docErr.Error()), nil)
			return
		} else {
			// Document mode: fetch and process
			e.Log.Info("A2A document request received",
				"document_id", documentID,
				"has_bearer_token", bearerToken != "",
				"delegation_user", userSPIFFEID,
				"delegation_agent", agentSPIFFEID)

			doc, err := e.FetchDocument(ctx, documentID, bearerToken)
			if err != nil {
				e.Log.Error("Document fetch failed", "error", err)
				yield(e.failedEvent(execCtx, err.Error()), nil)
				return
			}
			if doc == nil || doc["content"] == nil {
				e.Log.Deny("Access denied by document service")
				yield(e.rejectedEvent(execCtx, "Access denied"), nil)
				return
			}

			e.Log.Allow("Document access granted via A2A")

			title, _ := doc["title"].(string)
			content, _ := doc["content"].(string)

			result, err = e.ProcessLLM(ctx, title, content)
			if err != nil {
				e.Log.Error("LLM processing failed", "error", err)
				yield(e.failedEvent(execCtx, "LLM processing failed: "+err.Error()), nil)
				return
			}
		}

		// Write the result as an artifact
		if !yield(a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(result)), nil) {
			return
		}

		// Mark completed
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

// Cancel handles task cancellation.
func (e *AgentExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func (e *AgentExecutor) failedEvent(execCtx *a2asrv.ExecutorContext, reason string) a2a.Event {
	msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(reason))
	return a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed, msg)
}

func (e *AgentExecutor) rejectedEvent(execCtx *a2asrv.ExecutorContext, reason string) a2a.Event {
	msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(reason))
	return a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateRejected, msg)
}

// extractTextContent extracts all text parts from an A2A message.
func extractTextContent(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, part := range msg.Parts {
		if t := part.Text(); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}
