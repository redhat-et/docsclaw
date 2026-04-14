package cmd

import "github.com/spf13/cobra"

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage OCI-distributed skills",
	Long:  "Package, push, pull, verify, and inspect OCI-distributed skills.",
}

func init() {
	rootCmd.AddCommand(skillCmd)
}
