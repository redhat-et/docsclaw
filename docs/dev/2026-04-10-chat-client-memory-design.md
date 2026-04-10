# Client-side conversation memory for CLI chat

**Issue:** #11
**Date:** 2026-04-10
**Status:** Approved

## Goal

Give the chat client conversational context so follow-up questions
work as users expect. The agent should see prior turns when
processing each new message.

## Approach

Client-side history injection. Before sending each message, the chat
client formats the full conversation history into the message text.
The bridge and A2A protocol layer remain untouched.

This is a stopgap until server-side memory (#5) is implemented. It
will be removed or made optional once the server tracks context
natively. The bridge is also slated for an a2a-go v2 upgrade (#14),
so no bridge changes should be made now.

## Message format

First message (no history) is sent as-is. Subsequent messages
prepend conversation history:

```text
[Conversation history]
User: What tools do you have?
Agent: I can summarize documents and fetch web pages.

[Current message]
User: Summarize DOC-42
```

- Each turn rendered as `User: <text>` / `Agent: <text>`
- Blank line between turns for readability
- `[Conversation history]` and `[Current message]` delimiters
  make the structure explicit for the LLM
- Error messages from failed requests are excluded from history

## Token management

Send the full history every time. The conversation is bounded by
the chat session (no persistence across restarts), so payloads
stay manageable in practice. A sliding window or token budget can
be added later if needed.

## Scope

| What | Changes? |
|------|----------|
| `internal/chat/model.go` (`sendMessage`) | Yes — build context string from `m.messages` |
| `internal/bridge/` | No |
| `InvokeRequest` | No |
| `updateViewport()` | No |
| Display / UI | No |

## Not in scope

- Server-side memory / context threading (#5)
- Persistent conversation history across sessions
- Token budget / sliding window
- Bridge or A2A protocol changes (#14)
