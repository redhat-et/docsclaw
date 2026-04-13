package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	ociops "github.com/redhat-et/docsclaw/internal/oci"
)

var skillInspectCmd = &cobra.Command{
	Use:   "inspect <ref>",
	Short: "Show SkillCard metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]

		ctx := context.Background()

		sc, err := ociops.Inspect(ctx, ref, ociops.InspectOptions{})
		if err != nil {
			return fmt.Errorf("failed to inspect skill: %w", err)
		}

		fmt.Printf("Name:        %s\n", sc.Metadata.Name)
		fmt.Printf("Namespace:   %s\n", sc.Metadata.Namespace)
		fmt.Printf("Version:     %s\n", sc.Metadata.Version)
		fmt.Printf("Description: %s\n", sc.Metadata.Description)
		fmt.Printf("Author:      %s\n", sc.Metadata.Author)

		if sc.Metadata.License != "" {
			fmt.Printf("License:     %s\n", sc.Metadata.License)
		}
		if len(sc.Spec.Tools.Required) > 0 {
			fmt.Printf("Tools:       %v\n", sc.Spec.Tools.Required)
		}
		if sc.Spec.Resources.EstimatedMemory != "" {
			fmt.Printf("Memory:      %s\n", sc.Spec.Resources.EstimatedMemory)
		}
		if sc.Spec.Resources.EstimatedCPU != "" {
			fmt.Printf("CPU:         %s\n", sc.Spec.Resources.EstimatedCPU)
		}
		if sc.Metadata.Ref != "" {
			fmt.Printf("OCI Ref:     %s\n", sc.Metadata.Ref)
		}

		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillInspectCmd)
}
