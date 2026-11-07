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
	"github.com/redhat-et/docsclaw/internal/session"
	"github.com/redhat-et/docsclaw/internal/telemetry"
	"github.com/redhat-et/docsclaw/pkg/inboundauth"
	"github.com/redhat-et/docsclaw/pkg/llm"
)

// MessageProcessor processes a conversation with an LLM.
// Every A2A message takes this path: the executor holds no credentials and
// forwards no caller token.
type MessageProcessor func(ctx context.Context, messages []llm.Message) (string, error)

// AgentExecutor implements a2asrv.AgentExecutor by bridging A2A messages
// to an in-process message processor (optionally an agentic tool loop).
type AgentExecutor struct {
	Log            *logger.Logger
	ProcessMessage MessageProcessor     // required
	Sessions       session.SessionStore // optional: server-side conversation state
	SystemPrompt   string               // system prompt for new sessions
}

// Execute handles an incoming A2A message by processing it with the
// configured message processor and yielding the result as an artifact.
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

		// Extract the session ID and trace context from ServiceParams. The
		// caller's Authorization value is deliberately not read here: inbound
		// authentication owns it, and it must never reach the LLM or session.
		var sessionID string
		if sp := execCtx.ServiceParams; sp != nil {
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

		if e.ProcessMessage == nil {
			e.Log.Error("Agent not configured: ProcessMessage is nil")
			yield(e.failedEvent(execCtx, "agent not configured"), nil)
			return
		}

		userText := extractTextContent(execCtx.Message)
		e.Log.Info("A2A request received", "text_length", len(userText))

		// Identity comes only from verified claims. The middleware stripped
		// the raw token before this handler ran.
		if claims, ok := inboundauth.ClaimsFrom(ctx); ok {
			e.Log.Info("A2A request authenticated",
				"sub", claims.Subject,
				"azp", claims.AZP,
				"scope", strings.Join(claims.Scopes, " "),
				"aud", strings.Join(claims.Audience, " "))
		}

		if len(sessionID) > 64 {
			sessionID = sessionID[:64]
		}
		sessionID = sessionKey(ctx, sessionID)

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
			e.Log.Info("Processing message via agentic loop",
				"session_id", sessionID,
				"message_count", len(messages))
		} else {
			e.Log.Info("Processing message via agentic loop")
			messages = []llm.Message{
				{Role: "system", Content: e.SystemPrompt},
				{Role: "user", Content: userText},
			}
		}

		result, err := e.ProcessMessage(ctx, messages)
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

		// Write the result as an artifact
		if !yield(a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(result)), nil) {
			return
		}

		// Mark completed
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

// Cancel handles task cancellation.
// sessionKey isolates conversations per verified caller. Without claims it
// falls back to the caller-provided ID; an empty ID means stateless.
func sessionKey(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	if claims, ok := inboundauth.ClaimsFrom(ctx); ok {
		return claims.Subject + "|" + claims.AZP + "|" + sessionID
	}
	return sessionID
}

func (e *AgentExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func (e *AgentExecutor) failedEvent(execCtx *a2asrv.ExecutorContext, reason string) a2a.Event {
	msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(reason))
	return a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed, msg)
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
