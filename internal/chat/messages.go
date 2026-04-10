package chat

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role string // "user" or "agent"
	Text string
}

// responseMsg carries the agent's response text back to the model.
type responseMsg struct {
	text string
}

// errMsg carries an error back to the model.
type errMsg struct {
	err error
}
