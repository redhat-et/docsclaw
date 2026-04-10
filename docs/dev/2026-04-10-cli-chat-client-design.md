# Interactive CLI chat client with Bubble Tea

**Issue:** #11
**Date:** 2026-04-10
**Status:** Approved

## Goal

Add a `docsclaw chat` subcommand that provides an interactive terminal
chat interface to any DocsClaw A2A agent, built with Bubble Tea and
the Charm ecosystem.

## Command

```bash
docsclaw chat --agent-url http://localhost:8080
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--agent-url` | (required) | A2A agent endpoint URL |
| `--name` | Agent Card name | Override display name for the agent |

## Connection flow

1. Parse flags
2. Fetch `/.well-known/agent-card.json` from the agent URL
3. Display agent name, description, and available skills in header
4. Enter interactive chat loop

## UI layout

```text
+-------------------------------------------------+
| DocsClaw Chat · connected to:                   |
| Research Agent — Summarizes documents            |
+-------------------------------------------------+
|                                                 |
| You: What can you help me with?                 |
|                                                 |
| Research Agent: I can summarize documents,      |
| review code, and fetch web pages. Send me a     |
| document ID or ask a question.                  |
|                                                 |
| You: Summarize DOC-42                           |
|                                                 |
| · Thinking...                                   |
|                                                 |
+-------------------------------------------------+
| > _                                  Ctrl+C quit |
+-------------------------------------------------+
```

The agent label uses the Agent Card `name` field by default,
overridden with `--name`.

## Dependencies

| Package | Purpose |
|---------|---------|
| `bubbletea` | TUI framework, event loop |
| `bubbles` | Viewport, text input, spinner |
| `lipgloss` | Header, message labels, status bar styling |
| `glamour` | Markdown rendering of agent responses |

## Code structure

| File | Responsibility |
|------|----------------|
| `internal/cmd/chat.go` | Cobra subcommand, flag parsing, Agent Card fetch |
| `internal/chat/model.go` | Bubble Tea model: state, Update, View |
| `internal/chat/messages.go` | Bubble Tea message types (response, error) |
| `internal/chat/styles.go` | Lip Gloss style definitions |

## Key behaviors

**Input:** Single-line text input at the bottom, send on Enter.

**Sending:** Uses existing `A2AClient.Invoke()` with `MessageText`
(gateway mode). Runs in a goroutine via Bubble Tea `Cmd`, sends a
message on completion.

**Waiting:** Spinner component while the request is in flight. Input
disabled during wait.

**Rendering:** Agent responses rendered through Glamour for Markdown
(code blocks, lists, bold). User messages displayed as plain text.

**Scrolling:** Viewport component for chat history, scrollable with
arrow keys and mouse wheel.

**Quit:** Ctrl+C.

**Errors:** Connection failures and timeouts displayed inline as
styled error messages.

## Conversation model

Each message is independent on the server side (single-turn). The
client stores the full conversation in a `[]ChatMessage` slice for
display purposes. This slice is ready for multi-turn when the server
supports conversation context.

## Not in scope (v1)

- Multi-turn server-side context (depends on #5)
- Streaming/SSE (current API is blocking request/response)
- Authentication (bearer token, SPIFFE identity)
- Conversation save/load
- File attachments
- In-chat slash commands (e.g., `/name`, `/clear`, `/agents`)

## Future extensions

- **Streaming:** When the server implements `tasks/sendSubscribe`
  (SSE), swap the blocking `Invoke()` call for a streaming handler.
  The UI separation (goroutine + Bubble Tea message) makes this a
  localized change.
- **Slash commands:** Add in-chat commands like `/name`, `/clear`,
  `/agents` for runtime configuration.
- **Multi-turn:** Send conversation history with each request once
  the server or memory layer (#5) supports it.
