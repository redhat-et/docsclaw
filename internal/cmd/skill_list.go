package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/redhat-et/docsclaw/pkg/skills"
)

var skillListCmd = &cobra.Command{
	Use:   "list [skills-dir]",
	Short: "List locally available skills",
	Long:  "List skills in a directory. Defaults to ~/.docsclaw/skills/.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := ""
		if len(args) > 0 {
			dir = args[0]
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}
			dir = filepath.Join(home, ".docsclaw", "skills")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No skills found.")
				return nil
			}
			return fmt.Errorf("read directory: %w", err)
		}

		found := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillDir := filepath.Join(dir, entry.Name())

			// Try skill.yaml first, fall back to SKILL.md presence
			cardPath := filepath.Join(skillDir, "skill.yaml")
			if sy, err := skills.ParseSkillYAML(cardPath); err == nil {
				name := sy.Metadata.Name
				if name == "" {
					name = entry.Name()
				}
				fmt.Printf("%-25s %-10s %s\n", name, sy.Metadata.Version, sy.Metadata.Description)
				found++
				continue
			}

			mdPath := filepath.Join(skillDir, "SKILL.md")
			if meta, err := skills.ParseFrontmatter(mdPath); err == nil {
				name := meta.Name
				if name == "" {
					name = entry.Name()
				}
				fmt.Printf("%-25s %-10s %s\n", name, "-", meta.Description)
				found++
			} else if _, statErr := os.Stat(mdPath); statErr == nil {
				fmt.Printf("%-25s %-10s %s\n", entry.Name(), "-", "(failed to parse SKILL.md)")
				found++
			}
		}

		if found == 0 {
			fmt.Println("No skills found.")
		}

		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillListCmd)
}
