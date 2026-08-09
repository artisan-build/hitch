package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/artisan-build/hitch/internal/harness"
	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "hitch",
		Short:         "Install MCP servers into coding agents safely",
		Long:          "hitch installs MCP servers into detected coding agents while keeping credentials out of output and preserving user config safety.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(newVersionCommand())
	root.AddCommand(newListCommand(harness.CurrentEnv))

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "version: %s\ncommit: %s\ndate: %s\n", Version, Commit, Date)
			return err
		},
	}
}

func newListCommand(envFn func() harness.Env) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List detected coding-agent harnesses",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.OutOrStdout(), envFn())
		},
	}
}

func runList(out io.Writer, env harness.Env) error {
	results, err := harness.Detect(env)
	if err != nil {
		return err
	}

	for _, result := range results {
		status := "not detected"
		if result.Detected {
			status = "detected"
		}
		if result.PromptTier {
			status += " (prompt-tier)"
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", result.Name, result.ConfigPath, status); err != nil {
			return err
		}
	}

	return nil
}

func ExecuteForTest(root *cobra.Command, args ...string) error {
	root.SetArgs(args)
	root.SetIn(os.Stdin)
	return root.Execute()
}
