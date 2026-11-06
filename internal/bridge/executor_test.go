package bridge

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/redhat-et/docsclaw/internal/logger"
	"github.com/redhat-et/docsclaw/pkg/llm"
)

func newTestExecutor(process MessageProcessor) *AgentExecutor {
	return &AgentExecutor{
		Log:            logger.NewWithWriter(logger.ComponentAgent, io.Discard, false),
		ProcessMessage: process,
		SystemPrompt:   "test system prompt",
	}
}

func runExecute(t *testing.T, e *AgentExecutor, msg *a2a.Message, sp *a2asrv.ServiceParams) []a2a.Event {
	t.Helper()
	execCtx := &a2asrv.ExecutorContext{
		Message:       msg,
		TaskID:        a2a.TaskID("task-1"),
		ContextID:     "ctx-1",
		ServiceParams: sp,
	}

	var events []a2a.Event
	for ev, err := range e.Execute(context.Background(), execCtx) {
		if err != nil {
			t.Fatalf("Execute yielded error: %v", err)
		}
		events = append(events, ev)
	}
	return events
}

func finalStatus(t *testing.T, events []a2a.Event) *a2a.TaskStatusUpdateEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if ev, ok := events[i].(*a2a.TaskStatusUpdateEvent); ok {
			return ev
		}
	}
	t.Fatal("no status update event in sequence")
	return nil
}

// The caller's Authorization service param must never reach the LLM. In v2 the
// token is verified and stripped by the inbound-auth middleware; the executor
// has no business reading it.
func TestExecute_IgnoresAuthorizationServiceParam(t *testing.T) {
	var captured []llm.Message
	e := newTestExecutor(func(_ context.Context, messages []llm.Message) (string, error) {
		captured = append(captured, messages...)
		return "answer", nil
	})

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello"))
	sp := a2asrv.NewServiceParams(map[string][]string{
		"authorization": {"Bearer secret-token"},
	})

	events := runExecute(t, e, msg, sp)

	for _, m := range captured {
		if strings.Contains(m.Content, "secret-token") {
			t.Fatalf("token leaked into LLM message: role=%q content=%q", m.Role, m.Content)
		}
	}
	if got := finalStatus(t, events).Status.State; got != a2a.TaskStateCompleted {
		t.Fatalf("final state = %q, want %q", got, a2a.TaskStateCompleted)
	}
}

// A message that looks like a document request is an ordinary message now.
// Document mode forwarded the caller's raw bearer token to the document
// service, which v2 forbids.
func TestExecute_DocumentIDMessageGoesToProcessMessage(t *testing.T) {
	var captured []llm.Message
	calls := 0
	e := newTestExecutor(func(_ context.Context, messages []llm.Message) (string, error) {
		calls++
		captured = append(captured, messages...)
		return "summary", nil
	})

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("DOC-001"))

	events := runExecute(t, e, msg, nil)

	if calls != 1 {
		t.Fatalf("ProcessMessage calls = %d, want 1", calls)
	}
	var userTexts []string
	for _, m := range captured {
		if m.Role == "user" {
			userTexts = append(userTexts, m.Content)
		}
	}
	if len(userTexts) != 1 || userTexts[0] != "DOC-001" {
		t.Fatalf("user messages = %q, want exactly [DOC-001]", userTexts)
	}
	if got := finalStatus(t, events).Status.State; got != a2a.TaskStateCompleted {
		t.Fatalf("final state = %q, want %q", got, a2a.TaskStateCompleted)
	}
}

func TestExecute_NilProcessMessageFails(t *testing.T) {
	e := newTestExecutor(nil)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello"))
	events := runExecute(t, e, msg, nil)

	status := finalStatus(t, events)
	if status.Status.State != a2a.TaskStateFailed {
		t.Fatalf("final state = %q, want %q", status.Status.State, a2a.TaskStateFailed)
	}
	if status.Status.Message == nil {
		t.Fatal("failed status has no message")
	}
	reason := extractTextContent(status.Status.Message)
	if !strings.Contains(reason, "agent not configured") {
		t.Fatalf("failure reason = %q, want it to contain %q", reason, "agent not configured")
	}
}
