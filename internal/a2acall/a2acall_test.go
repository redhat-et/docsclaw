package a2acall

import (
	"context"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/redhat-et/docsclaw/internal/bridge"
	"github.com/redhat-et/docsclaw/internal/egress"
	"github.com/redhat-et/docsclaw/pkg/inboundauth"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type scriptedExecutor struct {
	state a2a.TaskState
	text  string
	delay time.Duration
	err   error
}

func (e *scriptedExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if e.delay > 0 {
			select {
			case <-time.After(e.delay):
			case <-ctx.Done():
				return
			}
		}
		if e.err != nil {
			yield(nil, e.err)
			return
		}
		if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}
		if e.text != "" {
			if !yield(a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(e.text)), nil) {
				return
			}
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, e.state, nil), nil)
	}
}

func (e *scriptedExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func newA2AServer(t *testing.T, ex a2asrv.AgentExecutor) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(ex)))
	t.Cleanup(srv.Close)
	return srv
}

func newTool(t *testing.T, peers []Peer) tools.Tool {
	t.Helper()
	return New(peers, bridge.NewA2AClient(egress.NewClient(5*time.Second), slog.Default()), slog.Default())
}

func TestA2ACall(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		srv := newA2AServer(t, &scriptedExecutor{state: a2a.TaskStateCompleted, text: "peer answer"})
		res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(context.Background(),
			map[string]any{"peer": "peer", "message": "hi"})
		if res.Error {
			t.Fatalf("unexpected error: %s", res.Output)
		}
		if res.Output != "[completed] peer answer" {
			t.Fatalf("output = %q", res.Output)
		}
	})

	for _, state := range []a2a.TaskState{a2a.TaskStateFailed, a2a.TaskStateRejected, a2a.TaskStateCanceled} {
		t.Run(string(state), func(t *testing.T) {
			srv := newA2AServer(t, &scriptedExecutor{state: state})
			res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(context.Background(),
				map[string]any{"peer": "peer", "message": "hi"})
			if !res.Error {
				t.Fatalf("expected error result, got %q", res.Output)
			}
		})
	}

	for _, state := range []a2a.TaskState{a2a.TaskStateInputRequired, a2a.TaskStateAuthRequired} {
		t.Run(string(state), func(t *testing.T) {
			srv := newA2AServer(t, &scriptedExecutor{state: state, text: "need more"})
			res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(context.Background(),
				map[string]any{"peer": "peer", "message": "hi"})
			if res.Error {
				t.Fatalf("unexpected error: %s", res.Output)
			}
			if !strings.Contains(res.Output, "peer needs more input: need more") {
				t.Fatalf("output = %q", res.Output)
			}
		})
	}

	t.Run("unknown peer makes no request", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			hits.Add(1)
		}))
		t.Cleanup(srv.Close)

		res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(context.Background(),
			map[string]any{"peer": "nope", "message": "hi"})
		if !res.Error || !strings.Contains(res.Output, `unknown peer "nope"; available: peer`) {
			t.Fatalf("result = %+v", res)
		}
		if hits.Load() != 0 {
			t.Fatalf("server got %d requests, want 0", hits.Load())
		}
	})

	t.Run("missing message", func(t *testing.T) {
		res := newTool(t, nil).Execute(context.Background(), map[string]any{"peer": "peer"})
		if !res.Error || !strings.Contains(res.Output, "message is required") {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("401 denial", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"caller_not_allowed"}`)
		}))
		t.Cleanup(srv.Close)

		res := newTool(t, []Peer{{Name: "engineering-analyst", URL: srv.URL}}).Execute(context.Background(),
			map[string]any{"peer": "engineering-analyst", "message": "hi"})
		if !res.Error {
			t.Fatalf("expected error, got %q", res.Output)
		}
		if !strings.Contains(res.Output, "peer engineering-analyst: 401 caller_not_allowed") {
			t.Fatalf("output = %q", res.Output)
		}
	})

	t.Run("403 denial", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"insufficient_scope"}`)
		}))
		t.Cleanup(srv.Close)

		res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(context.Background(),
			map[string]any{"peer": "peer", "message": "hi"})
		if !res.Error || !strings.Contains(res.Output, "403 insufficient_scope") {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := newA2AServer(t, &scriptedExecutor{state: a2a.TaskStateCompleted, text: "late", delay: time.Second})
		res := newTool(t, []Peer{{Name: "peer", URL: srv.URL, Timeout: 100 * time.Millisecond}}).
			Execute(context.Background(), map[string]any{"peer": "peer", "message": "hi"})
		if !res.Error || !strings.Contains(res.Output, "context deadline exceeded") {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("large reply is not dropped", func(t *testing.T) {
		big := strings.Repeat("x", 100*1024)
		srv := newA2AServer(t, &scriptedExecutor{state: a2a.TaskStateCompleted, text: big})
		res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(context.Background(),
			map[string]any{"peer": "peer", "message": "hi"})
		if res.Error || len(res.Output) <= len(big) {
			t.Fatalf("large reply lost: error=%v len=%d", res.Error, len(res.Output))
		}
	})
}

func TestA2ACall_SendsNoAuthorization(t *testing.T) {
	var gotAuth string
	var hits atomic.Int32
	inner := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(
		&scriptedExecutor{state: a2a.TaskStateCompleted, text: "ok"}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotAuth = r.Header.Get("Authorization")
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	ctx := inboundauth.ContextWithClaims(context.Background(), inboundauth.Claims{
		Subject: "alice",
		AZP:     "spiffe://openshell.local/openshell/sandbox/alice-assistant",
	})
	res := newTool(t, []Peer{{Name: "peer", URL: srv.URL}}).Execute(ctx,
		map[string]any{"peer": "peer", "message": "hi"})
	if res.Error {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if hits.Load() == 0 {
		t.Fatal("peer was never called")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header leaked to peer: %q", gotAuth)
	}
}
