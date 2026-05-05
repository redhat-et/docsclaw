package chat

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	glamour "charm.land/glamour/v2"

	"github.com/redhat-et/docsclaw/internal/bridge"
)

// Skill describes a single agent capability for display.
type Skill struct {
	Name        string
	Description string
}

// Model is the Bubble Tea model for the interactive chat client.
type Model struct {
	agentURL         string
	agentName        string
	agentDescription string
	userName         string
	skills           []Skill

	client   *bridge.A2AClient
	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	messages       []ChatMessage
	sessionID      string // stable ID for server-side session continuity
	sessionConfirmed bool // true after first successful response (server has session)
	waiting        bool
	err            error

	width  int
	height int
	ready  bool

	renderer *glamour.TermRenderer
}

// NewModel creates a new chat model connected to the given agent.
func NewModel(agentURL, agentName, agentDescription, userName string, skills []Skill) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Prompt = inputPromptStyle.Render("> ")
	ti.CharLimit = 4096
	ti.Focus()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)

	return Model{
		agentURL:         agentURL,
		agentName:        agentName,
		agentDescription: agentDescription,
		userName:         userName,
		skills:           skills,
		sessionID:        generateSessionID(),
		client:    bridge.NewA2AClient(
			&http.Client{Timeout: 120 * time.Second},
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
		input:     ti,
		spinner:   sp,
		renderer:  r,
	}
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Init returns the initial command for the model.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update processes messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 1
		statusHeight := 1
		inputHeight := 1
		verticalMargin := headerHeight + statusHeight + inputHeight + 1

		if !m.ready {
			m.viewport = viewport.New(
				viewport.WithWidth(m.width),
				viewport.WithHeight(m.height-verticalMargin),
			)
			m.viewport.SoftWrap = true
			m.ready = true
		} else {
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(m.height - verticalMargin)
		}
		m.input.SetWidth(m.width - 4)

		// Re-create the glamour renderer with updated width.
		if r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(m.width-4),
		); err == nil {
			m.renderer = r
		}

		m.updateViewport()

	case tea.KeyPressMsg:
		switch {
		case msg.Code == 'c' && msg.Mod == tea.ModCtrl:
			return m, tea.Quit
		case msg.Code == tea.KeyEnter:
			if m.waiting {
				return m, nil
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			m.messages = append(m.messages, ChatMessage{Role: "user", Text: text})
			m.waiting = true
			m.updateViewport()
			return m, tea.Batch(m.sendMessage(text), m.spinner.Tick)
		}

	case responseMsg:
		m.waiting = false
		m.sessionConfirmed = true
		m.messages = append(m.messages, ChatMessage{Role: "agent", Text: msg.text})
		m.updateViewport()
		return m, textinput.Blink

	case errMsg:
		m.waiting = false
		m.err = msg.err
		m.messages = append(m.messages, ChatMessage{
			Role: "agent",
			Text: fmt.Sprintf("Error: %v", msg.err),
		})
		m.updateViewport()
		return m, textinput.Blink

	case spinner.TickMsg:
		if m.waiting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
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

// View renders the UI.
func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("Initializing...")
	}

	headerText := fmt.Sprintf(" %s Chat", m.agentName)
	if m.agentDescription != "" {
		headerText += " — " + m.agentDescription
	}
	header := headerStyle.Width(m.width).Render(headerText)

	var inputLine string
	if m.waiting {
		inputLine = agentLabelStyle.Render(m.spinner.View()+" Thinking...")
	} else {
		inputLine = m.input.View()
	}

	status := statusBarStyle.Render("Press Enter to send, Ctrl+C to quit")

	content := fmt.Sprintf("%s\n%s\n%s\n%s",
		header,
		m.viewport.View(),
		inputLine,
		status,
	)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// updateViewport rebuilds the chat content and sets it in the viewport.
func (m *Model) updateViewport() {
	var sb strings.Builder

	if len(m.messages) == 0 {
		if len(m.skills) > 0 {
			sb.WriteString(agentLabelStyle.Render("Skills:"))
			sb.WriteString("\n")
			for _, s := range m.skills {
				line := fmt.Sprintf("  • %s", s.Name)
				if s.Description != "" {
					line += " — " + s.Description
				}
				sb.WriteString(line + "\n")
			}
			sb.WriteString("\n")
		}
		sb.WriteString(statusBarStyle.Render("  Send a message to start the conversation."))
		sb.WriteString("\n")
	}

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			label := userLabelStyle.Render(m.userName + ":")
			sb.WriteString(label + " " + msg.Text + "\n\n")
		case "agent":
			label := agentLabelStyle.Render(m.agentName + ":")
			// Try rendering as markdown.
			rendered := msg.Text
			if m.renderer != nil {
				if r, err := m.renderer.Render(msg.Text); err == nil {
					rendered = strings.TrimSpace(r)
				}
			}
			// Color errors differently.
			if strings.HasPrefix(msg.Text, "Error:") {
				rendered = errorStyle.Render(msg.Text)
			}
			sb.WriteString(label + "\n" + rendered + "\n\n")
		}
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

// sendMessage dispatches a message to the agent in a goroutine.
// It prepends conversation history so the agent has context for
// follow-up questions.
func (m *Model) sendMessage(text string) tea.Cmd {
	client := m.client
	agentURL := m.agentURL
	sessionID := m.sessionID
	messageText := m.buildMessageWithHistory(text)

	return func() tea.Msg {
		result, err := client.Invoke(context.Background(), &bridge.InvokeRequest{
			AgentURL:    agentURL,
			MessageText: messageText,
			SessionID:   sessionID,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return responseMsg{text: result.Text}
	}
}

// buildMessageWithHistory formats conversation history into the message
// text so the agent sees prior turns. On the first message, returns the
// raw text with no formatting.
func (m *Model) buildMessageWithHistory(text string) string {
	// After the first successful response, the server has our session —
	// stop prepending history client-side to avoid doubling context.
	if m.sessionConfirmed {
		return text
	}

	// Collect prior turns, excluding error messages and the current
	// user message (which is already appended to m.messages).
	var history []ChatMessage
	for _, msg := range m.messages[:len(m.messages)-1] {
		if msg.Role == "agent" && strings.HasPrefix(msg.Text, "Error:") {
			continue
		}
		history = append(history, msg)
	}

	if len(history) == 0 {
		return text
	}

	var sb strings.Builder
	sb.WriteString("[Conversation history]\n")
	for _, msg := range history {
		label := m.userName
		if msg.Role == "agent" {
			label = m.agentName
		}
		sb.WriteString(label + ": " + msg.Text + "\n\n")
	}
	sb.WriteString("[Current message]\n")
	sb.WriteString(m.userName + ": " + text)

	return sb.String()
}
