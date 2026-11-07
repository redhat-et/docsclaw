package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/redhat-et/docsclaw/pkg/llm"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// askRuntimeOptions lets tests inject a provider and capture logs.
var askRuntimeOptions runtimeOptions

func newAskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask [flags] PROMPT",
		Short: "Run one prompt through the agent and print the answer",
		Long: "Run one prompt through the same agent setup as serve, print the\n" +
			"final answer to stdout, and exit non-zero on failure.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runAsk,
	}
	addAgentFlags(cmd)
	cmd.Flags().Bool("trace", false,
		"Print each tool call and its result summary to stderr")
	return cmd
}

func init() {
	rootCmd.AddCommand(newAskCmd())
}

func runAsk(cmd *cobra.Command, args []string) error {
	bindAgentFlags(cmd)

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := askRuntimeOptions
	if opts.LogWriter == nil {
		opts.LogWriter = cmd.ErrOrStderr()
	}
	rt, err := buildRuntime(ctx, cfg, opts)
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close() }()

	if rt.Provider == nil {
		return errors.New("LLM API key not configured")
	}

	var hook tools.Hook
	trace, err := cmd.Flags().GetBool("trace")
	if err != nil {
		return err
	}
	if trace {
		hook = &traceHook{w: cmd.ErrOrStderr()}
	}

	messages := []llm.Message{{Role: "user", Content: args[0]}}
	if rt.Registry != nil {
		messages = append([]llm.Message{
			{Role: "system", Content: rt.SystemPrompt},
		}, messages...)
	}

	answer, err := rt.Run(ctx, messages, hook)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), answer)
	return nil
}
