package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/redhat-et/docsclaw/internal/config"
)

// usageError marks a command-line usage mistake so Execute can map it to
// exit code 2.
type usageError struct {
	err error
}

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

var (
	cfgFile string
	v       *viper.Viper
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "docsclaw",
	Short:   "DocsClaw Agent Runtime",
	Long:    "Universal AI agent runtime with ConfigMap-driven personality.",
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor maps errors to process exit codes: 2 for usage mistakes,
// 1 for everything else.
func exitCodeFor(err error) int {
	var usage *usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default is ./config.yaml)")

	v = config.InitViper("docsclaw")
	config.BindFlags(rootCmd, v)
}

func initConfig() {
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	}
}
