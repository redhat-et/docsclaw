package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/internal/chat"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat with an A2A agent",
	Long:  "Connect to an A2A agent and start an interactive terminal chat session.",
	RunE:  runChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().String("agent-url", "", "URL of the A2A agent (required)")
	chatCmd.Flags().String("name", "", "Override the agent display name")
	_ = chatCmd.MarkFlagRequired("agent-url")
}

func runChat(cmd *cobra.Command, _ []string) error {
	agentURL, _ := cmd.Flags().GetString("agent-url")
	nameOverride, _ := cmd.Flags().GetString("name")

	agentName := "Agent"
	agentDescription := ""
	var skills []chat.Skill

	// Fetch agent card from well-known endpoint.
	card, err := fetchAgentCard(agentURL)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: could not fetch agent card: %v\n", err)
	} else {
		agentName = card.Name
		agentDescription = card.Description
		for _, s := range card.Skills {
			skills = append(skills, chat.Skill{
				Name:        s.Name,
				Description: s.Description,
			})
		}
	}

	if nameOverride != "" {
		agentName = nameOverride
	}

	m := chat.NewModel(agentURL, agentName, agentDescription, "You", skills)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

// fetchAgentCard retrieves the agent card from the well-known endpoint.
func fetchAgentCard(agentURL string) (*a2a.AgentCard, error) {
	base := strings.TrimSuffix(agentURL, "/")
	// Try to derive the base URL (strip /a2a or similar suffixes).
	for _, suffix := range []string{"/a2a", "/rpc"} {
		base = strings.TrimSuffix(base, suffix)
	}

	cardURL := base + "/.well-known/agent-card.json"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(cardURL) //nolint:noctx // short-lived CLI request
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s",
			resp.StatusCode, cardURL)
	}

	var card a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("failed to decode agent card: %w", err)
	}
	return &card, nil
}
