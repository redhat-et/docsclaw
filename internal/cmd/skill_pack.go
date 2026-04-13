package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	ociops "github.com/redhat-et/docsclaw/internal/oci"
	"oras.land/oras-go/v2/content/oci"
)

var (
	packAsImage bool
	packOutput  string
)

var skillPackCmd = &cobra.Command{
	Use:   "pack <skill-dir>",
	Short: "Package a skill directory into local OCI layout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		skillDir := args[0]

		ctx := context.Background()

		// Use default output if not specified
		output := packOutput
		if output == "" {
			output = skillDir + "/oci-layout"
		}

		// Create local OCI store
		store, err := oci.New(output)
		if err != nil {
			return fmt.Errorf("failed to create OCI store: %w", err)
		}

		// Pack the skill
		desc, err := ociops.Pack(ctx, skillDir, store, ociops.PackOptions{AsImage: packAsImage})
		if err != nil {
			return fmt.Errorf("failed to pack skill: %w", err)
		}

		fmt.Printf("Packed skill to %s\n", output)
		fmt.Printf("Digest: %s\n", desc.Digest)
		fmt.Printf("Size: %d bytes\n", desc.Size)

		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillPackCmd)
	skillPackCmd.Flags().BoolVar(&packAsImage, "as-image", false, "Pack as OCI image instead of artifact")
	skillPackCmd.Flags().StringVarP(&packOutput, "output", "o", "", "Output directory for OCI layout (default: <skill-dir>/oci-layout)")
}
