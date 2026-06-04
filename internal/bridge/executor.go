package bridge

import (
	"context"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/redhat-et/docsclaw/internal/logger"
	"github.com/redhat-et/docsclaw/internal/telemetry"
	"github.com/redhat-et/docsclaw/internal/session"
	"github.com/redhat-et/docsclaw/pkg/llm"
)

// DocumentFetcher fetches a document by ID from the document-service.
// The bearerToken parameter is optional; when non-empty, it is forwarded
// as an Authorization header for JWT-based access control.
// Delegation context (X-Delegation-User/Agent headers) is injected
// automatically by DelegationTransport from the request context.
type DocumentFetcher func(ctx context.Context, documentID, bearerToken string) (map[string]any, error)

// LLMProcessor processes a document with an LLM and returns the result text.
type LLMProcessor func(ctx context.Context, title, content string) (string, error)

// MessageProcessor processes a conversation with an LLM.
// Used in phase 2 (tool-use) mode for standalone operation.
type MessageProcessor func(ctx context.Context, messages []llm.Message) (string, error)

// AgentExecutor implements a2asrv.AgentExecutor by bridging A2A messages
// to document fetch and LLM processing.
type AgentExecutor struct {
	Log            *logger.Logger
	FetchDocument  DocumentFetcher
	ProcessLLM     LLMProcessor
	ProcessMessage MessageProcessor   // optional: handles free-form messages
	Sessions       session.SessionStore // optional: server-side conversation state
	SystemPrompt   string             // system prompt for new sessions
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

		// Extract bearer token, delegation context, and session ID from ServiceParams.
		var bearerToken string
		var userSPIFFEID, agentSPIFFEID string
		var sessionID string
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
			if vals, found := sp.Get("x-session-id"); found && len(vals) > 0 {
				sessionID = vals[0]
			}

			// Extract W3C trace context from incoming A2A request
			// so this agent's spans join the caller's distributed trace.
			carrier := propagation.MapCarrier{}
			for _, key := range []string{"traceparent", "tracestate", "baggage"} {
				if vals, found := sp.Get(key); found && len(vals) > 0 {
					carrier.Set(key, vals[0])
				}
			}
			ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
		}

		// Create a span for the A2A execution, parented to the extracted trace.
		ctx, execSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "a2a.execute",
			trace.WithAttributes(
				attribute.String("a2a.task_id", string(execCtx.TaskID)),
				telemetry.AttrSessionID.String(sessionID),
			))
		defer execSpan.End()

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
			// Free-form message mode: no document ID, use session-based
			// conversation or fall back to single-turn.
			userText := extractTextContent(execCtx.Message)
			e.Log.Info("A2A free-form request received",
				"text_length", len(userText))

			if len(sessionID) > 64 {
				sessionID = sessionID[:64]
			}

			var messages []llm.Message
			if e.Sessions != nil && sessionID != "" {
				if _, err := e.Sessions.GetOrCreate(sessionID, e.SystemPrompt); err != nil {
					e.Log.Error("Session creation failed", "error", err)
					yield(e.failedEvent(execCtx, "Session error: "+err.Error()), nil)
					return
				}
				var err error
				messages, err = e.Sessions.AppendAndSnapshot(sessionID,
					llm.Message{Role: "user", Content: userText})
				if err != nil {
					e.Log.Error("Session append failed", "error", err)
					yield(e.failedEvent(execCtx, "Session error: "+err.Error()), nil)
					return
				}
				e.Log.Info("Processing free-form message via agentic loop",
					"session_id", sessionID,
					"message_count", len(messages))
			} else {
				e.Log.Info("Processing free-form message via agentic loop")
				messages = []llm.Message{
					{Role: "system", Content: e.SystemPrompt},
					{Role: "user", Content: userText},
				}
			}

			var err error
			result, err = e.ProcessMessage(ctx, messages)
			if err != nil {
				e.Log.Error("Message processing failed", "error", err)
				yield(e.failedEvent(execCtx, "Processing failed: "+err.Error()), nil)
				return
			}

			if e.Sessions != nil && sessionID != "" {
				if err := e.Sessions.Append(sessionID,
					llm.Message{Role: "assistant", Content: result}); err != nil {
					e.Log.Error("Failed to save assistant response to session",
						"session_id", sessionID, "error", err)
				}
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
