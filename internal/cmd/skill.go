package cmd

import "github.com/spf13/cobra"

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage locally available skills",
	Long:  "List and delete locally available skills.",
}

func init() {
	rootCmd.AddCommand(skillCmd)
}
