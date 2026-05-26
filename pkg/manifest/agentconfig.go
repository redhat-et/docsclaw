package manifest

import (
	"fmt"
	"strings"
)

func BuildAgentConfigYAML(m *AgentManifest) string {
	var b strings.Builder

	if m.Spec.Runtime.SkillsDir != "" {
		fmt.Fprintf(&b, "skills_dir: %s\n", m.Spec.Runtime.SkillsDir)
	}

	b.WriteString("tools:\n")
	if len(m.Spec.Runtime.Tools.Allowed) > 0 {
		b.WriteString("  allowed:\n")
		for _, tool := range m.Spec.Runtime.Tools.Allowed {
			fmt.Fprintf(&b, "    - %s\n", tool)
		}
	}

	if m.Spec.Runtime.Tools.Exec.Timeout > 0 || m.Spec.Runtime.Tools.Exec.MaxOutput > 0 {
		b.WriteString("  exec:\n")
		if m.Spec.Runtime.Tools.Exec.Timeout > 0 {
			fmt.Fprintf(&b, "    timeout: %d\n", m.Spec.Runtime.Tools.Exec.Timeout)
		}
		if m.Spec.Runtime.Tools.Exec.MaxOutput > 0 {
			fmt.Fprintf(&b, "    maxOutput: %d\n", m.Spec.Runtime.Tools.Exec.MaxOutput)
		}
	}

	if len(m.Spec.Runtime.Tools.WebFetch.AllowedHosts) > 0 {
		b.WriteString("  webFetch:\n")
		b.WriteString("    allowedHosts:\n")
		for _, host := range m.Spec.Runtime.Tools.WebFetch.AllowedHosts {
			fmt.Fprintf(&b, "      - %s\n", host)
		}
	}

	if m.Spec.Runtime.Loop.MaxIterations > 0 {
		b.WriteString("loop:\n")
		fmt.Fprintf(&b, "  maxIterations: %d\n", m.Spec.Runtime.Loop.MaxIterations)
	}

	return b.String()
}

func HasRuntimeConfig(m *AgentManifest) bool {
	r := m.Spec.Runtime
	return len(r.Tools.Allowed) > 0 ||
		r.Tools.Exec.Timeout > 0 ||
		r.Tools.Exec.MaxOutput > 0 ||
		len(r.Tools.WebFetch.AllowedHosts) > 0 ||
		r.Loop.MaxIterations > 0 ||
		r.SkillsDir != ""
}
