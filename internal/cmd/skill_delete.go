package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var skillDeleteCmd = &cobra.Command{
	Use:   "delete <skill-name>",
	Short: "Delete a locally cached skill",
	Long:  "Remove a skill from the local cache directory.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// Validate name to prevent path traversal
		if name != filepath.Base(name) || name == "." || name == ".." {
			return fmt.Errorf("invalid skill name: %q", name)
		}

		dir, _ := cmd.Flags().GetString("dir")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}
			dir = filepath.Join(home, ".docsclaw", "skills")
		}

		skillPath := filepath.Join(dir, name)
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			return fmt.Errorf("skill %q not found in %s", name, dir)
		}

		if err := os.RemoveAll(skillPath); err != nil {
			return fmt.Errorf("failed to delete skill: %w", err)
		}

		fmt.Printf("Deleted skill %q from %s\n", name, dir)
		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillDeleteCmd)
	skillDeleteCmd.Flags().String("dir", "", "Skills directory (default: ~/.docsclaw/skills/)")
}
