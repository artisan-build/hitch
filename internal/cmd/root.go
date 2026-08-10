package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/artisan-build/hitch/internal/harness"
	"github.com/artisan-build/hitch/internal/install"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func NewRootCommand(envFn func() (harness.Env, error)) *cobra.Command {
	root := &cobra.Command{
		Use:           "hitch",
		Short:         "Install MCP servers into coding agents safely",
		Long:          "hitch installs MCP servers into detected coding agents while keeping credentials out of output and preserving user config safety.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(&cobra.Command{Hidden: true})

	root.AddCommand(newVersionCommand())
	root.AddCommand(newListCommand(envFn))
	root.AddCommand(newInstallCommand(envFn))

	return root
}

type exitError struct {
	err  error
	code int
}

func (e exitError) Error() string { return e.err.Error() }

type silentExitError struct {
	err  error
	code int
}

func (e silentExitError) Error() string { return e.err.Error() }

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

func newInstallCommand(envFn func() (harness.Env, error)) *cobra.Command {
	var clients []string
	var yes bool
	var dryRun bool
	var tokenStdin bool
	var tokenEnv string
	var headers []string
	var name string
	var forget bool

	cmd := &cobra.Command{
		Use:   "install <url> [token]",
		Short: "Install a remote HTTP MCP server into selected harnesses",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return exitError{err: fmt.Errorf("install requires <url> and optional [token]"), code: 2}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := envFn()
			if err != nil {
				return err
			}
			normalizedURL, err := install.NormalizeRemoteURL(args[0])
			if err != nil {
				return err
			}
			resolvedToken, err := resolveToken(cmd, args, tokenStdin, tokenEnv)
			if err != nil {
				return err
			}
			parsedHeaders, err := parseHeaders(headers)
			if err != nil {
				return exitError{err: err, code: 2}
			}
			if resolvedToken != "" {
				if _, ok := parsedHeaders["Authorization"]; ok {
					return exitError{err: fmt.Errorf("authorization header cannot be combined with bearer token input"), code: 2}
				}
				parsedHeaders["Authorization"] = "Bearer " + resolvedToken
			}
			if normalizedURL, err = install.ValidateRemoteInstall(normalizedURL, parsedHeaders); err != nil {
				return err
			}
			canonicalClients, err := normalizeClientIDs(clients)
			if err != nil {
				return exitError{err: err, code: 2}
			}
			result, err := install.InstallRemote(install.Options{
				URL:     normalizedURL,
				Name:    name,
				Headers: parsedHeaders,
				Clients: canonicalClients,
				Yes:     yes,
				DryRun:  dryRun,
				Forget:  forget,
				NonTTY:  !isTerminal(cmd.InOrStdin()),
				ConfirmName: func(inferred string) (bool, error) {
					var ok bool
					form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(fmt.Sprintf("Use inferred server name %q?", inferred)).Value(&ok)))
					return ok, form.Run()
				},
				PickTargets: func(targets []install.Target, preferred map[string]bool) ([]install.Target, error) {
					return pickTargets(normalizedURL, targets, preferred)
				},
				Env:    env,
				Stdout: cmd.OutOrStdout(),
			})
			if summaryErr := printInstallSummary(cmd.OutOrStdout(), result, dryRun); summaryErr != nil {
				return summaryErr
			}
			if err != nil {
				if len(result.Failures) > 0 && (len(result.Written) > 0 || len(result.WouldWrite) > 0) {
					return nil
				}
				if len(result.Failures) == 0 {
					return err
				}
				return silentExitError{err: err, code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&clients, "client", "c", nil, "target an explicit harness (repeatable)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "accept every detected harness without prompting")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned writes without changing files")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the bearer token from stdin")
	cmd.Flags().StringVar(&tokenEnv, "token-env", "", "read the bearer token from an environment variable")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "additional HTTP header as 'K: V' (repeatable)")
	cmd.Flags().StringVar(&name, "name", "", "override the inferred server name")
	cmd.Flags().BoolVar(&forget, "forget", false, "clear the remembered harness preference before installing")
	return cmd
}

func resolveToken(cmd *cobra.Command, args []string, tokenStdin bool, tokenEnv string) (string, error) {
	if len(args) == 2 {
		if strings.TrimSpace(args[1]) == "" {
			return "", fmt.Errorf("token argument is empty")
		}
		return args[1], nil
	}
	if tokenStdin {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("read token from stdin: %w", err)
		}
		value := strings.TrimSpace(string(raw))
		if value == "" {
			return "", fmt.Errorf("token read from stdin is empty")
		}
		return value, nil
	}
	if tokenEnv != "" {
		value, ok := os.LookupEnv(tokenEnv)
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("environment variable %s is unset or empty", tokenEnv)
		}
		return value, nil
	}
	if isTerminal(cmd.InOrStdin()) {
		if file, ok := cmd.InOrStdin().(*os.File); ok {
			_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Bearer token (optional, input hidden): ")
			raw, err := term.ReadPassword(int(file.Fd()))
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(raw)), nil
		}
	}
	return "", nil
}

func parseHeaders(values []string) (map[string]string, error) {
	headers := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, ":")
		if ok && strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --header; use 'K: V'")
		}
		if !ok {
			// With no delimiter, the text could be a key or a credential value; echoing it is guessing about a string that might be secret.
			if candidate := strings.Fields(value); len(candidate) > 1 && strings.TrimSpace(candidate[0]) != "" {
				return nil, fmt.Errorf("invalid --header for key %q; use 'K: V'", candidate[0])
			}
			return nil, fmt.Errorf("invalid --header; use 'K: V'")
		}
		headers[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return headers, nil
}

func normalizeClientIDs(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	aliases := map[string]string{"claude": "claude-code", "claude-code": "claude-code", "gemini": "gemini-cli", "gemini-cli": "gemini-cli"}
	for _, value := range values {
		id := strings.ToLower(strings.TrimSpace(value))
		if alias, ok := aliases[id]; ok {
			id = alias
		}
		if _, ok := harness.FileWriterClientByID(id); !ok {
			return nil, fmt.Errorf("unknown file-writer client %q", value)
		}
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out, nil
}

func pickTargets(url string, targets []install.Target, preferred map[string]bool) ([]install.Target, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	selected := defaultSelectedTargetIDs(targets, preferred)
	options := make([]huh.Option[string], 0, len(targets))
	for _, target := range targets {
		label := fmt.Sprintf("%s\t%s", target.Client.Name, target.Path)
		options = append(options, huh.NewOption(label, target.Client.ID))
	}
	form := huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Title(fmt.Sprintf("Install %q into which harnesses?", url)).Options(options...).Value(&selected)))
	if err := form.Run(); err != nil {
		return nil, err
	}
	return targetsBySelectedIDs(targets, selected), nil
}

func defaultSelectedTargetIDs(targets []install.Target, preferred map[string]bool) []string {
	selected := []string{}
	for _, target := range targets {
		if preferred == nil || preferred[target.Client.ID] {
			selected = append(selected, target.Client.ID)
		}
	}
	return selected
}

func targetsBySelectedIDs(targets []install.Target, selected []string) []install.Target {
	chosenIDs := map[string]bool{}
	for _, id := range selected {
		chosenIDs[id] = true
	}
	chosen := make([]install.Target, 0, len(selected))
	for _, target := range targets {
		if chosenIDs[target.Client.ID] {
			chosen = append(chosen, target)
		}
	}
	return chosen
}

func printInstallSummary(out io.Writer, result install.Result, dryRun bool) error {
	if dryRun {
		for _, path := range result.WouldWrite {
			if _, err := fmt.Fprintf(out, "Dry run: would update %s\n", path); err != nil {
				return err
			}
		}
	} else {
		for _, path := range result.Written {
			if _, err := fmt.Fprintf(out, "Configured %s\n", path); err != nil {
				return err
			}
		}
	}
	for _, manual := range result.Manual {
		if _, err := fmt.Fprintf(out, "%s\n", manual); err != nil {
			return err
		}
	}
	for _, failure := range result.Failures {
		if _, err := fmt.Fprintf(out, "Not configured: %s\n", failure); err != nil {
			return err
		}
	}
	return nil
}

func isTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func newListCommand(envFn func() (harness.Env, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List detected coding-agent harnesses",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := envFn()
			if err != nil {
				return err
			}
			return runList(cmd.OutOrStdout(), env)
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
			status += " (prompt-tier - hitch does not write this client's config)"
		}

		path := result.ConfigPath
		if path == "" {
			path = "-"
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", result.Name, path, status); err != nil {
			return err
		}
	}

	return nil
}

func Main(args []string, stdout io.Writer, stderr io.Writer, envFn func() (harness.Env, error)) int {
	if len(args) == 1 && args[0] == "help" {
		args = []string{"--help"}
	}
	root := NewRootCommand(envFn)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		var silent silentExitError
		if errors.As(err, &silent) {
			return silent.code
		}
		_, _ = fmt.Fprintln(stderr, err)
		var ee exitError
		if errors.As(err, &ee) {
			return ee.code
		}
		return 1
	}
	return 0
}

func executeForTest(root *cobra.Command, args ...string) error {
	root.SetArgs(args)
	root.SetIn(os.Stdin)
	return root.Execute()
}
