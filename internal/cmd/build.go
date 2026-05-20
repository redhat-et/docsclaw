package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/pkg/catalog"
	"github.com/redhat-et/docsclaw/pkg/manifest"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build agent container image from manifest",
	Long: `Build agent container image from manifest.

Reads an agent manifest, validates tools against the catalog, checks skill
compatibility, and generates Containerfile + K8s manifests.

Examples:
  docsclaw build --manifest agent-manifest.yaml --output ./build
  docsclaw build --manifest agent-manifest.yaml --dry-run
  docsclaw build --manifest agent-manifest.yaml --only containerfile > Containerfile
`,
	RunE: runBuild,
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().String("manifest", "", "Path to agent-manifest.yaml (required)")
	buildCmd.Flags().String("output", "", "Directory to write generated files (default: print to stdout)")
	buildCmd.Flags().String("only", "", "Generate only 'containerfile' or 'k8s'")
	buildCmd.Flags().Bool("dry-run", false, "Print compatibility report and risk score only")
	buildCmd.Flags().String("catalog", "", "Path to custom tool catalog (default: embedded catalog)")
	buildCmd.Flags().Int("max-risk", 0, "Max allowed risk score (0 = no limit)")

	_ = buildCmd.MarkFlagRequired("manifest")
}

func runBuild(cmd *cobra.Command, args []string) error {
	manifestPath, _ := cmd.Flags().GetString("manifest")
	outputDir, _ := cmd.Flags().GetString("output")
	onlyMode, _ := cmd.Flags().GetString("only")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	catalogPath, _ := cmd.Flags().GetString("catalog")
	maxRisk, _ := cmd.Flags().GetInt("max-risk")

	// Validate --only flag
	if onlyMode != "" && onlyMode != "containerfile" && onlyMode != "k8s" {
		return fmt.Errorf("--only must be 'containerfile' or 'k8s'")
	}

	// 1. Parse manifest
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	// 2. Load catalog
	var cat *catalog.ToolCatalog
	if catalogPath != "" {
		cat, err = catalog.LoadFromFile(catalogPath)
		if err != nil {
			return fmt.Errorf("load catalog: %w", err)
		}
	} else {
		cat, err = catalog.LoadDefault()
		if err != nil {
			return fmt.Errorf("load default catalog: %w", err)
		}
	}

	// 3. Validate tools
	if err := cat.Validate(m.Spec.Tools); err != nil {
		return err
	}

	// 4. Merge core tools and compute risk score + tier
	allTools := mergeToolsWithCore(m.Spec.Tools, cat)
	sort.Strings(allTools)
	tier := cat.HighestTier(allTools)
	riskScore := cat.MaxRiskScore(allTools)

	// 5. Print compatibility report
	printReport(os.Stderr, m, allTools, tier, riskScore)

	// 6. Check max-risk constraint
	if maxRisk > 0 && riskScore > maxRisk {
		return fmt.Errorf("risk score %d exceeds max allowed %d", riskScore, maxRisk)
	}

	// 7. If dry-run, exit after report
	if dryRun {
		return nil
	}

	// 8. Generate files based on --only flag
	if onlyMode == "containerfile" || onlyMode == "" {
		containerfile, err := manifest.GenerateContainerfile(m, cat)
		if err != nil {
			return fmt.Errorf("generate containerfile: %w", err)
		}

		toolsJSON, err := manifest.GenerateToolsJSON(m, cat)
		if err != nil {
			return fmt.Errorf("generate tools.json: %w", err)
		}

		if outputDir != "" {
			if err := writeContainerfileOutputs(outputDir, containerfile, toolsJSON, m); err != nil {
				return err
			}
		} else if onlyMode == "containerfile" {
			// Print Containerfile to stdout only if --only containerfile
			fmt.Print(containerfile)
		}
	}

	if onlyMode == "k8s" || onlyMode == "" {
		k8sOutput, err := manifest.GenerateK8s(m, nil)
		if err != nil {
			return fmt.Errorf("generate k8s manifests: %w", err)
		}

		if outputDir != "" {
			if err := writeK8sOutputs(outputDir, k8sOutput); err != nil {
				return err
			}
		}
	}

	// If outputDir specified and no --only, write everything
	if outputDir != "" && onlyMode == "" {
		fmt.Fprintf(os.Stderr, "\n✓ Build artifacts written to %s\n", outputDir)
	}

	return nil
}

func mergeToolsWithCore(tools []string, cat *catalog.ToolCatalog) []string {
	seen := make(map[string]bool)
	var result []string

	// Add core tools first
	for _, name := range cat.CoreTools() {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	// Add manifest tools
	for _, name := range tools {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	return result
}

func printReport(w *os.File, m *manifest.AgentManifest, tools []string, tier string, riskScore int) {
	fmt.Fprintf(w, "\n=== Agent Build Report ===\n\n")
	fmt.Fprintf(w, "Agent:       %s\n", m.Metadata.Name)
	fmt.Fprintf(w, "Version:     %s\n", m.Metadata.Version)
	fmt.Fprintf(w, "Base:        %s\n", m.Spec.Base.Image)
	fmt.Fprintf(w, "Tier:        %s\n", tier)
	fmt.Fprintf(w, "Risk Score:  %d\n", riskScore)
	fmt.Fprintf(w, "Tools:       %d total\n\n", len(tools))

	// Group by tier
	fmt.Fprintf(w, "Tool Inventory:\n")
	for _, name := range tools {
		fmt.Fprintf(w, "  - %s\n", name)
	}
	fmt.Fprintf(w, "\n")
}

func writeContainerfileOutputs(dir, containerfile string, toolsJSON []byte, m *manifest.AgentManifest) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Write Containerfile
	containerfilePath := filepath.Join(dir, "Containerfile")
	if err := os.WriteFile(containerfilePath, []byte(containerfile), 0644); err != nil {
		return fmt.Errorf("write Containerfile: %w", err)
	}

	// Write tools.json
	toolsJSONPath := filepath.Join(dir, "tools.json")
	if err := os.WriteFile(toolsJSONPath, toolsJSON, 0644); err != nil {
		return fmt.Errorf("write tools.json: %w", err)
	}

	// Write system-prompt.txt
	systemPromptPath := filepath.Join(dir, "system-prompt.txt")
	if err := os.WriteFile(systemPromptPath, []byte(m.Spec.Prompt.Text), 0644); err != nil {
		return fmt.Errorf("write system-prompt.txt: %w", err)
	}

	// Write agent-config.yaml if runtime config exists
	if m.Spec.Runtime.Tools.Allowed != nil || m.Spec.Runtime.Loop.MaxIterations > 0 {
		agentConfig := buildAgentConfigYAML(m)
		agentConfigPath := filepath.Join(dir, "agent-config.yaml")
		if err := os.WriteFile(agentConfigPath, []byte(agentConfig), 0644); err != nil {
			return fmt.Errorf("write agent-config.yaml: %w", err)
		}
	}

	return nil
}

func writeK8sOutputs(dir string, k8s *manifest.K8sOutput) error {
	k8sDir := filepath.Join(dir, "k8s")
	if err := os.MkdirAll(k8sDir, 0755); err != nil {
		return fmt.Errorf("create k8s directory: %w", err)
	}

	files := map[string]string{
		"configmap.yaml":      k8s.ConfigMap,
		"deployment.yaml":     k8s.Deployment,
		"service.yaml":        k8s.Service,
		"serviceaccount.yaml": k8s.ServiceAccount,
	}

	if k8s.Secret != "" {
		files["secret.yaml"] = k8s.Secret
	}

	for name, content := range files {
		path := filepath.Join(k8sDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	return nil
}

func buildAgentConfigYAML(m *manifest.AgentManifest) string {
	var b strings.Builder

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
