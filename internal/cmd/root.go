package cmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/artisan-build/hitch/internal/claim"
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
	root.AddCommand(newScanCommand(envFn))
	root.AddCommand(newUninstallCommand(envFn))
	root.AddCommand(newPromptCommand())

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
			version, commit, date := buildVersionInfo()
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "version: %s\ncommit: %s\ndate: %s\n", version, commit, date)
			return err
		},
	}
}

func buildVersionInfo() (string, string, string) {
	version := Version
	commit := Commit
	date := Date
	if info, ok := debug.ReadBuildInfo(); ok {
		version, commit, date = versionInfoFromBuildInfo(version, commit, date, info)
	}
	return version, commit, date
}

func versionInfoFromBuildInfo(version string, commit string, date string, info *debug.BuildInfo) (string, string, string) {
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if commit == "none" && setting.Value != "" {
				commit = setting.Value
			}
		case "vcs.time":
			if date == "unknown" && setting.Value != "" {
				date = setting.Value
			}
		}
	}
	return version, commit, date
}

func newInstallCommand(envFn func() (harness.Env, error)) *cobra.Command {
	var clients []string
	var yes bool
	var dryRun bool
	var tokenStdin bool
	var tokenEnv string
	var project bool
	var headers []string
	var name string
	var forget bool
	var command string
	var argsCSV string
	var envVars []string
	var claimCode string
	var claimURL string

	cmd := &cobra.Command{
		Use:   "install <url|name> [token]",
		Short: "Install a remote HTTP or stdio MCP server into selected harnesses",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("command") {
				if len(args) != 1 {
					return exitError{err: fmt.Errorf("stdio install requires <name> with --command"), code: 2}
				}
				return nil
			}
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
			canonicalClients, err := normalizeClientIDs(clients)
			if err != nil {
				return exitError{err: err, code: 2}
			}
			stdioMode := cmd.Flags().Changed("command")
			if stdioMode {
				if strings.TrimSpace(command) == "" {
					return exitError{err: fmt.Errorf("stdio install requires non-empty --command"), code: 2}
				}
				if strings.Contains(args[0], "://") {
					return exitError{err: fmt.Errorf("stdio install requires a server name, not a URL; remove --command for remote installs"), code: 2}
				}
				if cmd.Flags().Changed("name") {
					return exitError{err: fmt.Errorf("stdio install uses positional <name>; --name is only valid for remote installs"), code: 2}
				}
				if len(headers) > 0 || tokenStdin || tokenEnv != "" {
					return exitError{err: fmt.Errorf("stdio install cannot use --header, --token-stdin, or --token-env"), code: 2}
				}
				if claimCode != "" || claimURL != "" {
					return exitError{err: fmt.Errorf("stdio install cannot use --claim or --claim-url"), code: 2}
				}
				parsedArgs, err := parseArgsCSV(argsCSV)
				if err != nil {
					return exitError{err: err, code: 2}
				}
				parsedEnv, err := parseEnvVars(envVars)
				if err != nil {
					return exitError{err: err, code: 2}
				}
				result, err := install.InstallStdio(install.Options{
					Name:     args[0],
					Command:  command,
					Args:     parsedArgs,
					StdioEnv: parsedEnv,
					Clients:  canonicalClients,
					Yes:      yes,
					Project:  project,
					DryRun:   dryRun,
					Forget:   forget,
					NonTTY:   !isTerminal(cmd.InOrStdin()),
					PickTargets: func(targets []install.Target, preferred map[string]bool) ([]install.Target, error) {
						return pickTargets(args[0], targets, preferred)
					},
					ConfirmProjectWrite: func(path string) (bool, error) {
						return confirmProjectCredentialWrite(path)
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
			}
			if cmd.Flags().Changed("args") || len(envVars) > 0 {
				return exitError{err: fmt.Errorf("remote install cannot use --args or --env; use --command for stdio installs"), code: 2}
			}
			claimMode := claimCode != "" || claimURL != ""
			if claimMode {
				if err := validateClaimFlagUse(claimCode, claimURL, args, tokenStdin, tokenEnv); err != nil {
					return err
				}
			}
			normalizedURL, err := install.NormalizeRemoteURL(args[0])
			if err != nil {
				return err
			}
			parsedHeaders, err := parseHeaders(headers)
			if err != nil {
				return exitError{err: err, code: 2}
			}
			var resolvedToken string
			if claimMode {
				if _, ok := parsedHeaders["Authorization"]; ok {
					return exitError{err: fmt.Errorf("authorization header cannot be combined with --claim; the claim exchange supplies the bearer token"), code: 2}
				}
			} else {
				if resolvedToken, err = resolveToken(cmd, args, tokenStdin, tokenEnv); err != nil {
					return err
				}
			}
			if resolvedToken != "" {
				if _, ok := parsedHeaders["Authorization"]; ok {
					return exitError{err: fmt.Errorf("authorization header cannot be combined with bearer token input"), code: 2}
				}
				parsedHeaders["Authorization"] = "Bearer " + resolvedToken
			}
			var validatedClaimURL string
			if claimMode {
				if validatedClaimURL, err = claim.ValidateURL(claimURL); err != nil {
					return err
				}
				// The placeholder marks the credential intent so the insecure-http
				// install-URL gate fires before the exchange, exactly as it would
				// for a supplied token.
				parsedHeaders["Authorization"] = install.ClaimPendingAuthorization
			}
			if normalizedURL, err = install.ValidateRemoteInstall(normalizedURL, parsedHeaders); err != nil {
				return err
			}
			var nameFromServer bool
			var namePending bool
			if claimMode {
				if dryRun {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run: no request was made to the claim URL; the claim code was not spent and is still valid."); err != nil {
						return err
					}
					if name == "" {
						if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run: the final server name is not known yet; it will come from --name, the claim response, or URL inference."); err != nil {
							return err
						}
						// Internal validation key only; NamePending keeps it out
						// of every output path.
						name = "claim-name-pending"
						namePending = true
					}
				} else {
					// Every precondition that can be evaluated without the token is
					// evaluated before the exchange: a run that is going to be
					// refused must never spend the claim code.
					if err := install.PreflightTargets(install.Options{
						Clients: canonicalClients,
						Yes:     yes,
						Project: project,
						NonTTY:  !isTerminal(cmd.InOrStdin()),
						Env:     env,
					}); err != nil {
						return err
					}
					version, _, _ := buildVersionInfo()
					resp, exchangeErr := claim.Exchange(validatedClaimURL, claimCode, version)
					if exchangeErr != nil {
						return claimFailure(exchangeErr, normalizedURL, name)
					}
					parsedHeaders["Authorization"] = "Bearer " + resp.Token
					if name == "" {
						if suggested := install.SanitizeName(resp.Name); suggested != "" {
							if len(suggested) > 64 {
								return fmt.Errorf("the claim response suggested a server name %d characters long; rerun with --name to choose a name", len(suggested))
							}
							name = suggested
							nameFromServer = true
						}
					}
					if err := printClaimExpiry(cmd.OutOrStdout(), resp.ExpiresAt); err != nil {
						return err
					}
				}
			}
			result, err := install.InstallRemote(install.Options{
				URL:            normalizedURL,
				Name:           name,
				NameFromServer: nameFromServer,
				NamePending:    namePending,
				Headers:        parsedHeaders,
				Clients:        canonicalClients,
				Yes:            yes,
				Project:        project,
				DryRun:         dryRun,
				Forget:         forget,
				NonTTY:         !isTerminal(cmd.InOrStdin()),
				ConfirmName: func(inferred string) (bool, error) {
					var ok bool
					form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(fmt.Sprintf("Use inferred server name %q?", inferred)).Value(&ok)))
					return ok, form.Run()
				},
				PickTargets: func(targets []install.Target, preferred map[string]bool) ([]install.Target, error) {
					return pickTargets(normalizedURL, targets, preferred)
				},
				ConfirmProjectWrite: func(path string) (bool, error) {
					return confirmProjectCredentialWrite(path)
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
	cmd.Flags().BoolVarP(&project, "project", "p", false, "write project-local MCP config paths")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the bearer token from stdin")
	cmd.Flags().StringVar(&tokenEnv, "token-env", "", "read the bearer token from an environment variable")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "additional HTTP header as 'K: V' (repeatable)")
	cmd.Flags().StringVar(&name, "name", "", "override the inferred server name")
	cmd.Flags().BoolVar(&forget, "forget", false, "clear the remembered harness preference before installing")
	cmd.Flags().StringVar(&command, "command", "", "stdio command to run")
	cmd.Flags().StringVar(&argsCSV, "args", "", "comma-separated stdio command arguments")
	cmd.Flags().StringArrayVar(&envVars, "env", nil, "stdio environment variable as K=V (repeatable)")
	cmd.Flags().StringVar(&claimCode, "claim", "", "single-use claim code to exchange for a bearer token before installing")
	cmd.Flags().StringVar(&claimURL, "claim-url", "", "claim endpoint that redeems --claim (https required)")
	return cmd
}

func validateClaimFlagUse(claimCode string, claimURL string, args []string, tokenStdin bool, tokenEnv string) error {
	if claimCode == "" {
		return exitError{err: fmt.Errorf("--claim-url requires --claim"), code: 2}
	}
	if claimURL == "" {
		return exitError{err: fmt.Errorf("--claim requires --claim-url; hitch never derives a claim endpoint from the server URL"), code: 2}
	}
	if len(args) == 2 {
		return exitError{err: fmt.Errorf("--claim cannot be combined with a positional token; the claim exchange supplies the token"), code: 2}
	}
	if tokenStdin {
		return exitError{err: fmt.Errorf("--claim cannot be combined with --token-stdin; the claim exchange supplies the token"), code: 2}
	}
	if tokenEnv != "" {
		return exitError{err: fmt.Errorf("--claim cannot be combined with --token-env; the claim exchange supplies the token"), code: 2}
	}
	return nil
}

// claimFailure maps exchange errors onto hitch's exit taxonomy: a malformed
// code is the user's typo (exit 2), everything else is operational (exit 1).
// The not-a-claim-endpoint path additionally offers the history-safe install
// alternative, since a token handed over out-of-band still works.
func claimFailure(err error, serverURL string, name string) error {
	var enumErr *claim.EnumError
	if errors.As(err, &enumErr) && enumErr.Misuse() {
		return exitError{err: enumErr, code: 2}
	}
	var notClaim *claim.NotClaimEndpointError
	if errors.As(err, &notClaim) {
		alternative := fmt.Sprintf("hitch install %s --token-stdin", serverURL)
		if name != "" {
			alternative += " --name " + name
		}
		return fmt.Errorf("%s\n\nAsk the server operator for a token, then install it without putting it in your shell history:\n  %s", notClaim.Error(), alternative)
	}
	return err
}

func printClaimExpiry(out io.Writer, expiresAt time.Time) error {
	if expiresAt.IsZero() {
		return nil
	}
	if expiresAt.Before(time.Now()) {
		_, err := fmt.Fprintf(out, "WARNING: the server reports this token already expired at %s; installing anyway — ask the operator for a fresh setup line.\n", expiresAt.Format(time.RFC3339))
		return err
	}
	_, err := fmt.Fprintf(out, "Token expires at %s.\n", expiresAt.Format(time.RFC3339))
	return err
}

func parseArgsCSV(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(value))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	args, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("invalid --args CSV: %w", err)
	}
	if _, err := reader.Read(); err != io.EOF {
		return nil, fmt.Errorf("invalid --args CSV: multiple records are not supported")
	}
	for _, arg := range args {
		if arg == "" {
			return nil, fmt.Errorf("invalid --args CSV: empty arguments are not supported")
		}
	}
	return args, nil
}

func parseEnvVars(values []string) (map[string]string, error) {
	out := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --env; use K=V")
		}
		out[key] = val
	}
	return out, nil
}

func resolveToken(cmd *cobra.Command, args []string, tokenStdin bool, tokenEnv string) (string, error) {
	if len(args) == 2 {
		if err := install.ValidateTokenValue(args[1]); err != nil {
			return "", fmt.Errorf("token argument %v", err)
		}
		return args[1], nil
	}
	if tokenStdin {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("read token from stdin: %w", err)
		}
		value := strings.TrimSpace(string(raw))
		if err := install.ValidateTokenValue(value); err != nil {
			return "", fmt.Errorf("token read from stdin %v", err)
		}
		return value, nil
	}
	if tokenEnv != "" {
		value, ok := os.LookupEnv(tokenEnv)
		if !ok {
			return "", fmt.Errorf("environment variable %s is unset or empty", tokenEnv)
		}
		if err := install.ValidateTokenValue(value); err != nil {
			return "", fmt.Errorf("environment variable %s %v", tokenEnv, err)
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
			value := strings.TrimSpace(string(raw))
			if value == "" {
				return "", nil
			}
			if err := install.ValidateTokenValue(value); err != nil {
				return "", fmt.Errorf("token %v", err)
			}
			return value, nil
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
	if err := runInstallPicker(url, options, &selected); err != nil {
		return nil, err
	}
	return targetsBySelectedIDs(targets, selected), nil
}

var runInstallPicker = func(url string, options []huh.Option[string], selected *[]string) error {
	form := huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Title(fmt.Sprintf("Install %q into which harnesses?", url)).Options(options...).Value(selected)))
	return form.Run()
}

func confirmProjectCredentialWrite(path string) (bool, error) {
	return runProjectCredentialConfirm(path)
}

var runProjectCredentialConfirm = func(path string) (bool, error) {
	var ok bool
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(fmt.Sprintf("Project config %s is not gitignored and may store credentials. Write anyway?", path)).Value(&ok)))
	return ok, form.Run()
}

type uninstallPickerOption struct {
	ID    string
	Label string
}

func uninstallPickerModel(targets []install.ScanResult, unreadable []install.ScanResult) ([]uninstallPickerOption, []string) {
	warnings := make([]string, 0, len(unreadable))
	for _, result := range unreadable {
		warnings = append(warnings, fmt.Sprintf("[!] %s\t%s\t(unreadable - cannot verify)", result.Client.Name, result.Path))
	}
	model := make([]uninstallPickerOption, 0, len(targets))
	for _, target := range targets {
		model = append(model, uninstallPickerOption{ID: target.Client.ID, Label: fmt.Sprintf("%s\t%s\t(%s)", target.Client.Name, target.Path, credentialLabel(target.HoldsCredential))})
	}
	return model, warnings
}

func pickUninstallTargets(out io.Writer, name string, targets []install.ScanResult, unreadable []install.ScanResult) ([]install.ScanResult, error) {
	model, warnings := uninstallPickerModel(targets, unreadable)
	for _, warning := range warnings {
		_, _ = fmt.Fprintln(out, warning)
	}
	selected := make([]string, 0, len(targets))
	options := make([]huh.Option[string], 0, len(model))
	for _, option := range model {
		selected = append(selected, option.ID)
		options = append(options, huh.NewOption(option.Label, option.ID))
	}
	if err := runUninstallPicker(name, options, &selected); err != nil {
		return nil, err
	}
	return selectUninstallTargets(targets, unreadable, selected), nil
}

var runUninstallPicker = func(name string, options []huh.Option[string], selected *[]string) error {
	form := huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Title(fmt.Sprintf("Remove %q from which harnesses?", name)).Options(options...).Value(selected)))
	return form.Run()
}

func selectUninstallTargets(targets []install.ScanResult, _ []install.ScanResult, chosenIDs []string) []install.ScanResult {
	chosen := map[string]bool{}
	for _, id := range chosenIDs {
		chosen[id] = true
	}
	selected := make([]install.ScanResult, 0, len(chosenIDs))
	for _, target := range targets {
		if chosen[target.Client.ID] {
			selected = append(selected, target)
		}
	}
	return selected
}

func credentialLabel(holds bool) string {
	if holds {
		return "holds a credential"
	}
	return "no credential"
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
		for _, written := range result.WrittenInfo {
			if _, err := fmt.Fprintf(out, "Configured %s %q → %s (%s)\n", written.ClientName, result.Name, result.URL, written.Path); err != nil {
				return err
			}
		}
		if len(result.WrittenInfo) == 0 {
			for _, path := range result.Written {
				if _, err := fmt.Fprintf(out, "Configured %q → %s (%s)\n", result.Name, result.URL, path); err != nil {
					return err
				}
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

func newScanCommand(envFn func() (harness.Env, error)) *cobra.Command {
	var project bool
	cmd := &cobra.Command{
		Use:   "scan [name]",
		Short: "Scan client configs for a server entry",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return exitError{err: fmt.Errorf("scan accepts at most one server name"), code: 2}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := envFn()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			results, err := install.ScanScoped(env, name, nil, project)
			if err != nil {
				return err
			}
			return printScanResults(cmd.OutOrStdout(), results)
		},
	}
	cmd.Flags().BoolVarP(&project, "project", "p", false, "scan project-local MCP config paths")
	return cmd
}

func printScanResults(out io.Writer, results []install.ScanResult) error {
	for _, result := range results {
		status := "no entry"
		switch result.Status {
		case install.ScanHasEntry:
			status = "has entry (" + credentialLabel(result.HoldsCredential) + ")"
		case install.ScanUnreadable:
			status = "UNREADABLE - cannot verify"
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", result.Client.Name, result.Path, status); err != nil {
			return err
		}
	}
	return nil
}

func newUninstallCommand(envFn func() (harness.Env, error)) *cobra.Command {
	var clients []string
	var yes bool
	var project bool
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an MCP server from selected harnesses",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitError{err: fmt.Errorf("uninstall requires <name>"), code: 2}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := envFn()
			if err != nil {
				return err
			}
			canonicalClients, err := normalizeClientIDs(clients)
			if err != nil {
				return exitError{err: err, code: 2}
			}
			result, err := install.Uninstall(install.UninstallOptions{
				Name:    args[0],
				Clients: canonicalClients,
				Yes:     yes,
				Project: project,
				NonTTY:  !isTerminal(cmd.InOrStdin()),
				PickTargets: func(targets []install.ScanResult, unreadable []install.ScanResult) ([]install.ScanResult, error) {
					return pickUninstallTargets(cmd.OutOrStdout(), args[0], targets, unreadable)
				},
				Env: env,
			})
			if err != nil {
				if result.Name == "" {
					return err
				}
				if strings.Contains(err.Error(), "non-TTY uninstall") {
					return exitError{err: err, code: 2}
				}
				if summaryErr := printUninstallSummary(cmd.OutOrStdout(), result); summaryErr != nil {
					return summaryErr
				}
				return silentExitError{err: err, code: 1}
			}
			if summaryErr := printUninstallSummary(cmd.OutOrStdout(), result); summaryErr != nil {
				return summaryErr
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&clients, "client", "c", nil, "target an explicit harness (repeatable)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove from every config where the server is present without prompting")
	cmd.Flags().BoolVarP(&project, "project", "p", false, "remove from project-local MCP config paths")
	return cmd
}

func printUninstallSummary(out io.Writer, result install.UninstallResult) error {
	for _, removed := range result.Removed {
		if _, err := fmt.Fprintf(out, "Removed %s %q (%s, %s)\n", removed.Client.Name, result.Name, removed.Path, credentialLabel(removed.HoldsCredential)); err != nil {
			return err
		}
	}
	for _, kept := range result.Kept {
		if _, err := fmt.Fprintf(out, "Kept %s %q (%s, %s)\n", kept.Client.Name, result.Name, kept.Path, credentialLabel(kept.HoldsCredential)); err != nil {
			return err
		}
	}
	if len(result.Removed) == 0 && len(result.Kept) == 0 {
		if _, err := fmt.Fprintf(out, "No matching %q entries removed\n", result.Name); err != nil {
			return err
		}
	}
	for _, unreadable := range result.Unreadable {
		if _, err := fmt.Fprintf(out, "UNREADABLE - cannot verify: %s (%s)\n", unreadable.Client.Name, unreadable.Path); err != nil {
			return err
		}
	}
	return nil
}

func newPromptCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt <url>",
		Short: "Print manual setup prompts for clients hitch does not write",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitError{err: fmt.Errorf("prompt requires <url>"), code: 2}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := install.NormalizeRemoteURL(args[0])
			if err != nil {
				return err
			}
			name, _, err := install.InferName(url)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Claude Desktop: add a stdio MCP entry that runs mcp-remote for %s; hitch does not write it because remote HTTP requires a local proxy and Node runtime.\n\nJetBrains: add this MCP server through the JetBrains MCP UI for %s; hitch does not write it because the dialog has no Authorization-headers field.\n\nCodex: hitch does not install Codex automatically yet, but hitch scan and uninstall can verify and remove this manual entry later. Add this to Codex config manually:\n[%s]\nurl = %q\nbearer_token_env_var = \"%s\"\n\nBefore starting Codex, run:\nexport %s=YOUR_TOKEN\n", url, url, install.CodexServerTableHeader(name), url, install.CodexTokenEnvVar(name), install.CodexTokenEnvVar(name))
			return err
		},
	}
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
