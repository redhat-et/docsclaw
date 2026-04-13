package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/redhat-et/docsclaw/internal/verify"
)

var verifyKey string

var skillVerifyCmd = &cobra.Command{
	Use:   "verify <ref>",
	Short: "Verify skill signature",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]

		ctx := context.Background()

		policy := verify.Policy{
			Mode:      verify.ModeEnforce,
			PublicKey: verifyKey,
		}

		if err := verify.Verify(ctx, ref, policy); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		fmt.Printf("Successfully verified signature for %s\n", ref)

		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillVerifyCmd)
	skillVerifyCmd.Flags().StringVar(&verifyKey, "key", "", "Public key for signature verification (required)")
	if err := skillVerifyCmd.MarkFlagRequired("key"); err != nil {
		panic(err)
	}
}
