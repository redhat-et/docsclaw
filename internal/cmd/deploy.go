package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/pkg/manifest"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Generate K8s manifests with secrets resolved",
	Long: `Generate K8s manifests with secrets resolved from flags or environment.

Reads an agent manifest, resolves secrets from --secret flags or environment
variables, and outputs K8s manifests ready for deployment.

Examples:
  docsclaw deploy --manifest agent-manifest.yaml --secret API_KEY=xyz | oc apply -f -
  docsclaw deploy --manifest agent-manifest.yaml --output ./deploy
  export API_KEY=xyz && docsclaw deploy --manifest agent-manifest.yaml
`,
	RunE: runDeploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.Flags().String("manifest", "", "Path to agent-manifest.yaml (required)")
	deployCmd.Flags().StringSlice("secret", nil, "Secret values as NAME=value pairs")
	deployCmd.Flags().String("output", "", "Directory to write generated files (optional, default: stdout)")

	_ = deployCmd.MarkFlagRequired("manifest")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	manifestPath, _ := cmd.Flags().GetString("manifest")
	secretFlags, _ := cmd.Flags().GetStringSlice("secret")
	outputDir, _ := cmd.Flags().GetString("output")

	// 1. Parse manifest
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	// 2. Resolve secrets
	resolvedSecrets, err := resolveSecrets(m.Spec.Secrets, secretFlags)
	if err != nil {
		return err
	}

	// 3. Generate K8s manifests
	k8sOutput, err := manifest.GenerateK8s(m, resolvedSecrets)
	if err != nil {
		return fmt.Errorf("generate k8s manifests: %w", err)
	}

	// 4. Output to directory or stdout
	if outputDir != "" {
		if err := writeK8sOutputs(outputDir, k8sOutput); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "✓ K8s manifests written to %s/k8s\n", outputDir)
	} else {
		printK8sManifests(k8sOutput)
	}

	return nil
}

func resolveSecrets(decls []manifest.SecretDecl, flagSecrets []string) (map[string]string, error) {
	// Parse --secret flags into map
	overrides := make(map[string]string)
	for _, s := range flagSecrets {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --secret format: %q (expected NAME=value)", s)
		}
		overrides[parts[0]] = parts[1]
	}

	// Resolve secrets: flag overrides, then env vars, then fail if required
	resolved := make(map[string]string)
	for _, decl := range decls {
		if v, ok := overrides[decl.Name]; ok {
			resolved[decl.Name] = v
		} else if v := os.Getenv(decl.Name); v != "" {
			resolved[decl.Name] = v
		} else if decl.Required {
			return nil, fmt.Errorf("required secret %q not set (use --secret %s=value or export %s)", decl.Name, decl.Name, decl.Name)
		}
	}
	return resolved, nil
}

func printK8sManifests(k8s *manifest.K8sOutput) {
	var manifests []string

	// Order: ServiceAccount, ConfigMap, Secret (if present), Service, Deployment
	if k8s.ServiceAccount != "" {
		manifests = append(manifests, k8s.ServiceAccount)
	}
	if k8s.ConfigMap != "" {
		manifests = append(manifests, k8s.ConfigMap)
	}
	if k8s.Secret != "" {
		manifests = append(manifests, k8s.Secret)
	}
	if k8s.Service != "" {
		manifests = append(manifests, k8s.Service)
	}
	if k8s.Deployment != "" {
		manifests = append(manifests, k8s.Deployment)
	}

	fmt.Print(strings.Join(manifests, "---\n"))
}
