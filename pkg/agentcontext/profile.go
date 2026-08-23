package agentcontext

// Profile defines the set and order of Markdown files that are loaded
// from an agent workspace directory and appended to the system prompt.
type Profile struct {
	Name  string
	Files []string
}

// Well-known profile names.
const (
	ProfileDocsclaw = "docsclaw"
	ProfileOpenClaw = "openclaw"
	ProfileHermes   = "hermes"
)

// DefaultProfiles is the built-in set of workspace context profiles.
// "openclaw" and "hermes" include MEMORY.md because both frameworks use
// a workspace Markdown file for long-term memory.
var DefaultProfiles = map[string]Profile{
	ProfileDocsclaw: {
		Name:  ProfileDocsclaw,
		Files: []string{"AGENTS.md", "SOUL.md", "USER.md", "IDENTITY.md", "TOOLS.md"},
	},
	ProfileOpenClaw: {
		Name:  ProfileOpenClaw,
		Files: []string{"AGENTS.md", "SOUL.md", "USER.md", "MEMORY.md", "IDENTITY.md", "TOOLS.md"},
	},
	ProfileHermes: {
		Name:  ProfileHermes,
		Files: []string{"AGENTS.md", "SOUL.md", "USER.md", "MEMORY.md"},
	},
}

// ResolveProfile returns the named profile and true if it is a built-in
// profile. An empty or unknown name returns the docsclaw profile and false.
func ResolveProfile(name string) (Profile, bool) {
	if name == "" {
		return DefaultProfiles[ProfileDocsclaw], false
	}
	if p, ok := DefaultProfiles[name]; ok {
		return p, true
	}
	return DefaultProfiles[ProfileDocsclaw], false
}
