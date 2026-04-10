package chat

import (
	lipgloss "charm.land/lipgloss/v2"
)

var (
	// headerStyle renders the top bar with the agent name.
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	// userLabelStyle renders the "You:" label.
	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#5B9BD5"))

	// agentLabelStyle renders the agent name label.
	agentLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#6BCB77"))

	// errorStyle renders error messages.
	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B"))

	// statusBarStyle renders the bottom status line.
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	// inputPromptStyle renders the input prompt indicator.
	inputPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#5B9BD5"))
)
