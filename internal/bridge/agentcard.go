package bridge

import (
	"github.com/a2aproject/a2a-go/v2/a2a"
)

// AgentCardParams holds the parameters for building an A2A AgentCard.
type AgentCardParams struct {
	Name        string
	Description string
	Version     string
	URL         string
	Skills      []a2a.AgentSkill
}

// BuildAgentCard creates an a2a.AgentCard from the given parameters.
func BuildAgentCard(p AgentCardParams) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:        p.Name,
		Description: p.Description,
		Version:     p.Version,
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(p.URL, a2a.TransportProtocolJSONRPC),
		},
		Skills:       p.Skills,
		Capabilities: a2a.AgentCapabilities{},
		DefaultInputModes: []string{
			"application/json",
		},
		DefaultOutputModes: []string{
			"text/plain",
		},
	}
}
