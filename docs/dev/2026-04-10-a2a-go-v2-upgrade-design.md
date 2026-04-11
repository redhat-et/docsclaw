# Upgrade a2a-go to v2

**Issue:** #14
**Date:** 2026-04-10
**Status:** Approved

## Goal

Migrate from `github.com/a2aproject/a2a-go` v0.3.x to
`github.com/a2aproject/a2a-go/v2` v2.2.0 so that DocsClaw serves
A2A v1.0 Agent Cards and is compatible with the `a2a` CLI tool.

## Key API changes

| Area | v1 | v2 |
|------|-----|-----|
| Import path | `github.com/a2aproject/a2a-go/...` | `github.com/a2aproject/a2a-go/v2/...` |
| Agent Card | `URL` + `ProtocolVersion` fields | `SupportedInterfaces []*AgentInterface` |
| Protocol version | `"0.3.0"` string | `a2a.Version` constant (`"1.0"`) |
| Parts | `a2a.TextPart{Text: "..."}` concrete type | `a2a.NewTextPart("...")` returns `*Part` |
| NewMessage | accepts `Part` interface values | accepts `*Part` pointers |
| Send params | `a2a.MessageSendParams` | `a2a.SendMessageRequest` |
| Executor | `Execute(ctx, reqCtx, queue) error` | `Execute(ctx, execCtx) iter.Seq2[Event, error]` |
| Executor context | `a2asrv.RequestContext` | `a2asrv.ExecutorContext` |
| Event writing | `queue.Write(ctx, event)` | `yield(event, nil)` |
| Client endpoints | `[]a2a.AgentInterface` | `[]*a2a.AgentInterface` |

## Files to modify

| File | Scope |
|------|-------|
| `go.mod` | Replace dependency |
| `internal/bridge/agentcard.go` | New card format with `SupportedInterfaces` |
| `internal/bridge/client.go` | Imports, `SendMessageRequest`, `NewTextPart`, pointer endpoints |
| `internal/bridge/executor.go` | Rewrite: `iter.Seq2` yield pattern replaces queue |
| `internal/bridge/message.go` | Part access via `.Text()` method |
| `internal/bridge/signedcard.go` | Import update |
| `internal/cmd/serve.go` | Imports, handler setup |
| `internal/cmd/chat.go` | Import update |
| `internal/cmd/serve_test.go` | Test assertions for new card format |

## Approach

Mechanical import path swap and type adaptation. The executor
rewrite is the only non-trivial change — converting from
queue-based event writing to Go 1.23 iterator-based yielding.
