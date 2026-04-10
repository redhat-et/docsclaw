# CLI Chat Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `docsclaw chat` subcommand with an interactive Bubble
Tea TUI that connects to any A2A agent, fetches its Agent Card, and
provides a chat interface with Markdown-rendered responses.

**Architecture:** Cobra subcommand in `internal/cmd/chat.go` delegates
to a Bubble Tea model in `internal/chat/`. The model manages input,
chat history viewport, and async agent communication via goroutines.
Agent Card is fetched on startup via HTTP GET to
`/.well-known/agent-card.json`.

**Tech Stack:** Bubble Tea, Bubbles (viewport, textinput, spinner),
Lip Gloss, Glamour, existing `internal/bridge.A2AClient`

---

## File structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/cmd/chat.go` | Create | Cobra subcommand, flag parsing, Agent Card fetch, launch TUI |
| `internal/chat/model.go` | Create | Bubble Tea model: state, Init, Update, View |
| `internal/chat/messages.go` | Create | Bubble Tea command/message types |
| `internal/chat/styles.go` | Create | Lip Gloss style definitions |
| `cmd/docsclaw/main.go` | Modify | Blank import `internal/cmd` (already done, verify only) |
| `go.mod` | Modify | Add Charm dependencies |

---

### Task 1: Add Charm dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add Bubble Tea and related packages**

```bash
go get charm.land/bubbletea/v2@latest
go get charm.land/bubbles/v2@latest
go get charm.land/lipgloss/v2@latest
go get charm.land/glamour/v2@latest
```

- [ ] **Step 2: Tidy modules**

```bash
go mod tidy
```

- [ ] **Step 3: Verify build still works**

```bash
make build
```

Expected: successful build, no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -s -m "deps: add Charm libraries for TUI chat client (#11)"
```

---

### Task 2: Create Lip Gloss styles

**Files:**
- Create: `internal/chat/styles.go`

- [ ] **Step 1: Write the styles file**

```go
package chat

import lipgloss "charm.land/lipgloss/v2"

var (
	// Header bar: agent name and description.
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	// User message label.
	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	// Agent message label.
	agentLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	// Error messages shown inline.
	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("9"))

	// Status bar at the bottom.
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Input prompt indicator.
	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)
)
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/chat/
```

Expected: success (package compiles standalone).

- [ ] **Step 3: Commit**

```bash
git add internal/chat/styles.go
git commit -s -m "feat(chat): add Lip Gloss style definitions (#11)"
```

---

### Task 3: Create Bubble Tea message types

**Files:**
- Create: `internal/chat/messages.go`

- [ ] **Step 1: Write the message types file**

```go
package chat

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	Role string // "user" or "agent"
	Text string // raw text (Markdown for agent responses)
}

// responseMsg is sent when the agent responds.
type responseMsg struct {
	text string
}

// errMsg is sent when a request fails.
type errMsg struct {
	err error
}

func (e errMsg) Error() string { return e.err.Error() }
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/chat/
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/chat/messages.go
git commit -s -m "feat(chat): add Bubble Tea message types (#11)"
```

---

### Task 4: Create the Bubble Tea model

**Files:**
- Create: `internal/chat/model.go`

- [ ] **Step 1: Write the model file**

This is the main TUI logic. The model manages the chat history
viewport, text input, spinner, and async agent communication.

```go
package chat

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/redhat-et/docsclaw/internal/bridge"
)

// Model is the Bubble Tea model for the chat TUI.
type Model struct {
	agentURL  string
	agentName string
	userName  string
	client    *bridge.A2AClient

	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	messages []ChatMessage
	waiting  bool
	width    int
	height   int
	ready    bool
}

// NewModel creates a new chat model.
func NewModel(agentURL, agentName, userName string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		agentURL:  agentURL,
		agentName: agentName,
		userName:  userName,
		client:    bridge.NewA2AClient(&http.Client{}, slog.Default()),
		input:     ti,
		spinner:   sp,
		messages:  []ChatMessage{},
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.waiting || strings.TrimSpace(m.input.Value()) == "" {
				return m, nil
			}
			text := m.input.Value()
			m.messages = append(m.messages, ChatMessage{Role: "user", Text: text})
			m.input.Reset()
			m.waiting = true
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
			return m, tea.Batch(m.spinner.Tick, m.sendMessage(text))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3 // header + separator
		inputHeight := 3  // input + status
		vpHeight := m.height - headerHeight - inputHeight
		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}
		m.input.Width = m.width - 4
		m.viewport.SetContent(m.renderHistory())

	case responseMsg:
		m.messages = append(m.messages, ChatMessage{Role: "agent", Text: msg.text})
		m.waiting = false
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, nil

	case errMsg:
		m.messages = append(m.messages, ChatMessage{
			Role: "agent",
			Text: "Error: " + msg.err.Error(),
		})
		m.waiting = false
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, nil

	case spinner.TickMsg:
		if m.waiting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Update sub-components.
	if !m.waiting {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header.
	header := headerStyle.Width(m.width).Render(
		fmt.Sprintf(" DocsClaw Chat · %s", m.agentName),
	)

	// Chat area.
	chatArea := m.viewport.View()

	// Input area.
	var inputArea string
	if m.waiting {
		inputArea = fmt.Sprintf("  %s Thinking...", m.spinner.View())
	} else {
		inputArea = promptStyle.Render("> ") + m.input.View()
	}

	// Status bar.
	status := statusStyle.Width(m.width).Render(
		fmt.Sprintf("  %s  %s",
			m.agentURL,
			"Ctrl+C quit",
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		chatArea,
		inputArea,
		status,
	)
}

// sendMessage sends a message to the agent asynchronously.
func (m Model) sendMessage(text string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.client.Invoke(context.Background(), &bridge.InvokeRequest{
			AgentURL:    m.agentURL,
			MessageText: text,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return responseMsg{text: result.Text}
	}
}

// renderHistory renders all chat messages into a single string.
func (m Model) renderHistory() string {
	if len(m.messages) == 0 {
		return statusStyle.Render("  Send a message to start the conversation.")
	}

	var sb strings.Builder
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(m.width-4),
	)

	for _, msg := range m.messages {
		sb.WriteString("\n")
		if msg.Role == "user" {
			label := userLabelStyle.Render(m.userName + ":")
			sb.WriteString(fmt.Sprintf("  %s %s\n", label, msg.Text))
		} else {
			label := agentLabelStyle.Render(m.agentName + ":")
			sb.WriteString(fmt.Sprintf("  %s\n", label))
			// Render agent responses as Markdown.
			rendered := msg.Text
			if renderer != nil {
				if r, err := renderer.Render(msg.Text); err == nil {
					rendered = r
				}
			}
			sb.WriteString(rendered)
		}
	}

	if m.waiting {
		sb.WriteString("\n")
		label := agentLabelStyle.Render(m.agentName + ":")
		sb.WriteString(fmt.Sprintf("  %s\n", label))
		sb.WriteString(fmt.Sprintf("  %s Thinking...\n", m.spinner.View()))
	}

	return sb.String()
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/chat/
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/chat/model.go
git commit -s -m "feat(chat): add Bubble Tea model with viewport and async send (#11)"
```

---

### Task 5: Create the Cobra subcommand

**Files:**
- Create: `internal/cmd/chat.go`

- [ ] **Step 1: Write the chat command**

This registers the `chat` subcommand, parses flags, fetches the
Agent Card, and launches the Bubble Tea program.

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/internal/chat"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive chat with a DocsClaw agent",
	Long:  "Connect to an A2A agent and start an interactive terminal chat session.",
	RunE:  runChat,
}

func init() {
	chatCmd.Flags().String("agent-url", "", "A2A agent endpoint URL (required)")
	chatCmd.Flags().String("name", "", "Override display name for the agent")
	_ = chatCmd.MarkFlagRequired("agent-url")
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) error {
	agentURL, _ := cmd.Flags().GetString("agent-url")
	nameOverride, _ := cmd.Flags().GetString("name")

	// Normalize URL: strip trailing slash.
	agentURL = strings.TrimRight(agentURL, "/")

	// Fetch Agent Card.
	agentName := "Agent"
	card, err := fetchAgentCard(agentURL)
	if err != nil {
		fmt.Printf("Warning: could not fetch Agent Card: %v\n", err)
		fmt.Println("Proceeding without agent metadata.")
	} else {
		agentName = card.Name
		fmt.Printf("Connected to: %s\n", card.Name)
		if card.Description != "" {
			fmt.Printf("  %s\n", card.Description)
		}
		if len(card.Skills) > 0 {
			fmt.Printf("  Skills:")
			for _, s := range card.Skills {
				fmt.Printf(" [%s]", s.Name)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// Apply name override.
	if nameOverride != "" {
		agentName = nameOverride
	}

	// Launch TUI.
	model := chat.NewModel(agentURL, agentName, "You")
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// fetchAgentCard fetches the Agent Card from the well-known endpoint.
func fetchAgentCard(agentURL string) (*a2a.AgentCard, error) {
	cardURL := agentURL + "/.well-known/agent-card.json"
	resp, err := http.Get(cardURL)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", cardURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", cardURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var card a2a.AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("parsing Agent Card: %w", err)
	}

	return &card, nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: successful build.

- [ ] **Step 3: Verify the subcommand is registered**

```bash
./bin/docsclaw --help
```

Expected: `chat` appears in the list of available commands.

```bash
./bin/docsclaw chat --help
```

Expected: shows `--agent-url` (required) and `--name` flags.

- [ ] **Step 4: Commit**

```bash
git add internal/cmd/chat.go
git commit -s -m "feat(chat): add Cobra subcommand for interactive A2A chat (#11)"
```

---

### Task 6: Integration test — end-to-end chat session

**Files:**
- No new files, manual test

- [ ] **Step 1: Start a local DocsClaw agent**

In a separate terminal:

```bash
export LLM_API_KEY=<your-key>
./bin/docsclaw serve \
  --config-dir testdata/standalone \
  --llm-provider anthropic \
  --listen-plain-http
```

Wait for `Server listening on :8000`.

- [ ] **Step 2: Run the chat client**

```bash
./bin/docsclaw chat --agent-url http://localhost:8000
```

Expected: the TUI launches, header shows the agent name from the
Agent Card, and the input prompt is active.

- [ ] **Step 3: Send a test message**

Type `Hello, what can you do?` and press Enter.

Expected: spinner shows "Thinking...", then agent response appears
rendered as Markdown.

- [ ] **Step 4: Test the name override**

Quit with Ctrl+C, then relaunch:

```bash
./bin/docsclaw chat --agent-url http://localhost:8000 --name "Rex"
```

Expected: agent messages are labeled "Rex:" instead of the Agent
Card name.

- [ ] **Step 5: Test error handling**

```bash
./bin/docsclaw chat --agent-url http://localhost:9999
```

Expected: warning about failed Agent Card fetch, then chat launches
with fallback name "Agent". Sending a message shows an inline error.

- [ ] **Step 6: Run linter and tests**

```bash
make lint
make test
```

Expected: no new lint errors, all tests pass.

- [ ] **Step 7: Final commit (if any fixes needed)**

```bash
git add -A
git commit -s -m "feat(chat): finalize interactive CLI chat client (#11)"
```
