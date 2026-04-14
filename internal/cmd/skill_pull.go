package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	ociops "github.com/redhat-et/docsclaw/internal/oci"
	"github.com/redhat-et/docsclaw/internal/verify"
)

var (
	pullVerify    bool
	pullKey       string
	pullOutput    string
	pullTLSVerify bool
)

var skillPullCmd = &cobra.Command{
	Use:   "pull <ref>",
	Short: "Pull skill from registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]

		ctx := context.Background()

		// Verify signature if requested
		if pullVerify {
			mode := verify.ModeEnforce
			if pullKey == "" {
				fmt.Fprintln(os.Stderr, "Warning: --verify without --key skips signature verification")
				mode = verify.ModeWarn
			}

			policy := verify.Policy{
				Mode:      mode,
				PublicKey: pullKey,
			}

			if err := verify.Verify(ctx, ref, policy); err != nil {
				return fmt.Errorf("verification failed: %w", err)
			}
		}

		// Determine output directory
		output := pullOutput
		if output == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			output = filepath.Join(home, ".docsclaw", "skills")
		}

		// Ensure output directory exists.
		if err := os.MkdirAll(output, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		// Pull the skill
		tlsVerify := pullTLSVerify
		opts := ociops.PullOptions{TLSVerify: &tlsVerify}
		if err := ociops.Pull(ctx, ref, output, opts); err != nil {
			return fmt.Errorf("failed to pull skill: %w", err)
		}

		fmt.Printf("Successfully pulled %s to %s\n", ref, output)

		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillPullCmd)
	skillPullCmd.Flags().BoolVar(&pullVerify, "verify", false, "Verify signature before pulling")
	skillPullCmd.Flags().StringVar(&pullKey, "key", "", "Public key for signature verification")
	skillPullCmd.Flags().StringVarP(&pullOutput, "output", "o", "", "Output directory (default: ~/.docsclaw/skills/)")
	skillPullCmd.Flags().BoolVar(&pullTLSVerify, "tls-verify", true, "Require HTTPS and verify certificates")
}
