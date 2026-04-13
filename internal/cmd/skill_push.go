package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	ociops "github.com/redhat-et/docsclaw/internal/oci"
)

var (
	pushAsImage   bool
	pushTLSVerify bool
)

var skillPushCmd = &cobra.Command{
	Use:   "push <skill-dir> <ref>",
	Short: "Pack and push skill to registry",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		skillDir := args[0]
		ref := args[1]

		ctx := context.Background()

		tlsVerify := pushTLSVerify
		opts := ociops.PushOptions{
			AsImage:   pushAsImage,
			TLSVerify: &tlsVerify,
		}

		if err := ociops.Push(ctx, skillDir, ref, opts); err != nil {
			return fmt.Errorf("failed to push skill: %w", err)
		}

		fmt.Printf("Successfully pushed %s to %s\n", skillDir, ref)

		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillPushCmd)
	skillPushCmd.Flags().BoolVar(&pushAsImage, "as-image", false, "Push as OCI image instead of artifact")
	skillPushCmd.Flags().BoolVar(&pushTLSVerify, "tls-verify", true, "Require HTTPS and verify certificates")
}
