package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artisan-build/hitch/internal/harness"
	installpkg "github.com/artisan-build/hitch/internal/install"
	"github.com/charmbracelet/huh"
)

func TestInstallNonTTYWithoutYesOrClientExitsNonZeroAndWritesNothing(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, ".cursor"))
	root := NewRootCommand(func() (harness.Env, error) { return testEnv(home), nil })
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_non_tty"})
	if err := root.Execute(); err == nil {
		t.Fatalf("Execute returned nil, want error")
	} else if !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "--client") {
		t.Fatalf("error = %q, want --yes and --client", err.Error())
	}
	if strings.Contains(stdout.String()+stderr.String(), "tok_SENTINEL_non_tty") {
		t.Fatalf("token leaked to output")
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("config was written, stat err = %v", err)
	}
}

func TestInstallTokenSourcesAndPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		stdin     string
		envName   string
		envValue  string
		wantToken string
	}{
		{name: "argv wins", args: []string{"install", "https://mcp.example.test/mcp", "argv-token", "--client", "cursor", "--token-env", "HITCH_TEST_TOKEN"}, envName: "HITCH_TEST_TOKEN", envValue: "env-token", wantToken: "argv-token"},
		{name: "argv wins over stdin", args: []string{"install", "https://mcp.example.test/mcp", "argv-token", "--client", "cursor", "--token-stdin"}, stdin: "stdin-token\n", wantToken: "argv-token"},
		{name: "argv wins over env", args: []string{"install", "https://mcp.example.test/mcp", "argv-token", "--client", "cursor", "--token-env", "HITCH_TEST_TOKEN"}, envName: "HITCH_TEST_TOKEN", envValue: "env-token", wantToken: "argv-token"},
		{name: "argv wins over stdin and env", args: []string{"install", "https://mcp.example.test/mcp", "argv-token", "--client", "cursor", "--token-stdin", "--token-env", "HITCH_TEST_TOKEN"}, stdin: "stdin-token\n", envName: "HITCH_TEST_TOKEN", envValue: "env-token", wantToken: "argv-token"},
		{name: "stdin wins over env", args: []string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-stdin", "--token-env", "HITCH_TEST_TOKEN"}, stdin: "stdin-token\n", envName: "HITCH_TEST_TOKEN", envValue: "env-token", wantToken: "stdin-token"},
		{name: "env used", args: []string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-env", "HITCH_TEST_TOKEN"}, envName: "HITCH_TEST_TOKEN", envValue: "env-token", wantToken: "env-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.envName != "" {
				t.Setenv(tt.envName, tt.envValue)
			}
			root := NewRootCommand(func() (harness.Env, error) { return testEnv(home), nil })
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetIn(strings.NewReader(tt.stdin))
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v; stderr=%q", err, stderr.String())
			}
			got := cursorAuthorization(t, home)
			if got != "Bearer "+tt.wantToken {
				t.Fatalf("Authorization = %q, want Bearer %s", got, tt.wantToken)
			}
		})
	}
}

func TestInstallTokenSourceErrorsWriteNothing(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   io.Reader
		args []string
		want string
	}{
		{name: "empty positional", in: strings.NewReader(""), args: []string{"install", "https://mcp.example.test/mcp", "", "--client", "cursor"}, want: "token argument is empty"},
		{name: "unset env", in: strings.NewReader(""), args: []string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-env", "HITCH_TEST_MISSING_TOKEN"}, want: "HITCH_TEST_MISSING_TOKEN"},
		{name: "empty env", in: strings.NewReader(""), args: []string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-env", "HITCH_TEST_EMPTY_TOKEN"}, want: "HITCH_TEST_EMPTY_TOKEN"},
		{name: "empty stdin", in: strings.NewReader("\n"), args: []string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-stdin"}, want: "token read from stdin is empty"},
		{name: "stdin read error", in: errReader{}, args: []string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-stdin"}, want: "read token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.name == "empty env" {
				t.Setenv("HITCH_TEST_EMPTY_TOKEN", "")
			}
			root := NewRootCommand(func() (harness.Env, error) { return testEnv(home), nil })
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetIn(tt.in)
			root.SetArgs(tt.args)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute error = %v, want %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("config was written, stat err = %v", err)
			}
		})
	}
}

func TestInstallCLIHidesTokenOnSuccessAndDryRun(t *testing.T) {
	for _, args := range [][]string{
		{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_cli", "--client", "cursor"},
		{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_cli", "--client", "cursor", "--dry-run"},
	} {
		home := t.TempDir()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Main(args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
		if code != 0 {
			t.Fatalf("Main(%v) code = %d, stderr = %q", args, code, stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "tok_SENTINEL_cli") {
			t.Fatalf("token leaked for args %v\nstdout:%s\nstderr:%s", args, stdout.String(), stderr.String())
		}
	}
}

func TestInstallStdioCLIParsesArgsEnvAndMasksEnvOnDryRun(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		home := t.TempDir()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		args := []string{"install", "local-server", "--command", "npx", "--args", "-y,\"@scope/pkg,with-comma\"", "--env", "API_KEY=STDIO_CLI_SENTINEL_SECRET", "--client", "cursor"}
		if dryRun {
			args = append(args, "--dry-run")
		}
		code := Main(args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
		if code != 0 {
			t.Fatalf("dryRun=%v exit code = %d, stderr = %q", dryRun, code, stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "STDIO_CLI_SENTINEL_SECRET") {
			t.Fatalf("stdio env leaked; stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		path := filepath.Join(home, ".cursor", "mcp.json")
		if dryRun {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("dry-run wrote config, stat err = %v", err)
			}
			if !strings.Contains(stdout.String(), "\"API_KEY\": \"***\"") {
				t.Fatalf("dry-run did not mask env value: %q", stdout.String())
			}
			continue
		}
		server := onlyCursorServer(t, home)
		if server["type"] != "stdio" || server["command"] != "npx" {
			t.Fatalf("stdio server basics = %#v", server)
		}
		gotArgs := server["args"].([]any)
		wantArgs := []string{"-y", "@scope/pkg,with-comma"}
		if len(gotArgs) != len(wantArgs) {
			t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
		}
		for i, want := range wantArgs {
			if gotArgs[i] != want {
				t.Fatalf("arg %d = %q, want %q", i, gotArgs[i], want)
			}
		}
		if server["env"].(map[string]any)["API_KEY"] != "STDIO_CLI_SENTINEL_SECRET" {
			t.Fatalf("env not written correctly: %#v", server["env"])
		}
	}
}

func TestInstallModeSelectionRejectsEmptyCommandAndIgnoredFlags(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		secret   string
		wantCode int
		wantErr  string
	}{
		{
			name:     "empty command stays stdio and errors",
			args:     []string{"install", "stripe", "--command", "", "--env", "API_KEY=MODE_SENTINEL_SECRET", "--yes", "--client", "cursor"},
			secret:   "MODE_SENTINEL_SECRET",
			wantCode: 2,
			wantErr:  "non-empty --command",
		},
		{
			name:     "remote rejects args",
			args:     []string{"install", "https://mcp.example.test/mcp", "tok", "--args", "-y,@example/mcp", "--client", "cursor"},
			wantCode: 2,
			wantErr:  "remote install cannot use --args or --env",
		},
		{
			name:     "remote rejects env",
			args:     []string{"install", "https://mcp.example.test/mcp", "tok", "--env", "API_KEY=MODE_SENTINEL_SECRET", "--client", "cursor"},
			secret:   "MODE_SENTINEL_SECRET",
			wantCode: 2,
			wantErr:  "remote install cannot use --args or --env",
		},
		{
			name:     "stdio rejects name flag",
			args:     []string{"install", "stdio-name", "--command", "npx", "--name", "ignored", "--client", "cursor"},
			wantCode: 2,
			wantErr:  "--name is only valid for remote installs",
		},
		{
			name:     "stdio rejects URL positional",
			args:     []string{"install", "https://mcp.example.test/mcp", "--command", "npx", "--client", "cursor"},
			wantCode: 2,
			wantErr:  "server name, not a URL",
		},
		{
			name:     "stdio rejects empty args entry",
			args:     []string{"install", "stdio-name", "--command", "npx", "--args", "-y,", "--client", "cursor"},
			wantCode: 2,
			wantErr:  "empty arguments are not supported",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Main(tt.args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String()+stderr.String(), tt.wantErr) {
				t.Fatalf("output missing %q; stdout=%q stderr=%q", tt.wantErr, stdout.String(), stderr.String())
			}
			if tt.secret != "" && strings.Contains(stdout.String()+stderr.String(), tt.secret) {
				t.Fatalf("mode selection leaked sentinel; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("config was written, stat err = %v", err)
			}
		})
	}
}

func TestInstallStdioCLIEnvParsingCoversMalformedAndPositiveCases(t *testing.T) {
	for _, tt := range []struct {
		name        string
		envFlag     string
		wantCode    int
		wantDetail  string
		wantKey     string
		wantValue   string
		writeBroken bool
	}{
		{name: "bare no equals", envFlag: "STDIO_ENV_SENTINEL_BARE", wantCode: 2, wantDetail: "invalid --env; use K=V"},
		{name: "leading equals empty key", envFlag: "=STDIO_ENV_SENTINEL_EMPTY_KEY", wantCode: 2, wantDetail: "invalid --env; use K=V"},
		{name: "whitespace empty key", envFlag: "   =STDIO_ENV_SENTINEL_WHITESPACE_KEY", wantCode: 2, wantDetail: "invalid --env; use K=V"},
		{name: "whitespace no equals", envFlag: "  STDIO_ENV_SENTINEL_SPACE_NO_EQUALS  ", wantCode: 2, wantDetail: "invalid --env; use K=V"},
		{name: "empty value positive", envFlag: "API_KEY=", wantCode: 0, wantDetail: "Configured ", wantKey: "API_KEY", wantValue: ""},
		{name: "multiple equals positive", envFlag: "API_KEY=a=STDIO_ENV_SENTINEL_MULTI", wantCode: 0, wantDetail: "Configured ", wantKey: "API_KEY", wantValue: "a=STDIO_ENV_SENTINEL_MULTI"},
		{name: "trim key positive", envFlag: " API_KEY =STDIO_ENV_SENTINEL_TRIMMED", wantCode: 0, wantDetail: "Configured ", wantKey: "API_KEY", wantValue: "STDIO_ENV_SENTINEL_TRIMMED"},
		{name: "malformed config does not echo env", envFlag: "API_KEY=STDIO_ENV_SENTINEL_CONFIG", wantCode: 1, wantDetail: "Not configured: Cursor:", writeBroken: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.writeBroken {
				writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{not-json", 0o600)
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Main([]string{"install", "local-server", "--command", "npx", "--env", tt.envFlag, "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "STDIO_ENV_SENTINEL") {
				t.Fatalf("stdio error leaked env value; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String()+stderr.String(), tt.wantDetail) {
				t.Fatalf("output missing %q; stdout=%q stderr=%q", tt.wantDetail, stdout.String(), stderr.String())
			}
			if tt.wantCode == 0 {
				server := onlyCursorServer(t, home)
				if got := server["env"].(map[string]any)[tt.wantKey]; got != tt.wantValue {
					t.Fatalf("env %q = %q, want %q", tt.wantKey, got, tt.wantValue)
				}
			} else if !tt.writeBroken {
				if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
					t.Fatalf("config was written, stat err = %v", err)
				}
			}
		})
	}
}

func TestInstallStdioExplicitCodexPrintsManualInstructionsAndFails(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "local-server", "--command", "npx", "--args", "-y,@example/mcp", "--env", "API_KEY=STDIO_CODEX_SENTINEL_SECRET", "--client", "codex"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("codex config was written, stat err = %v", err)
	}
	for _, want := range []string{"hitch cannot configure Codex automatically yet", "[mcp_servers.local-server]", "command = \"npx\"", "args = [\"-y\",\"@example/mcp\"]", "Set API_KEY"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), "STDIO_CODEX_SENTINEL_SECRET") {
		t.Fatalf("Codex stdio manual output leaked env value; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestInstallHeaderParsingDoesNotEchoSecret(t *testing.T) {
	for _, tt := range []struct {
		name         string
		header       string
		secret       string
		wantCode     int
		wantDetail   string
		wantNoDetail string
		wantHeader   string
		wantValue    string
	}{
		{name: "bare value no colon no whitespace", header: "SENTINEL_BARE_NO_DELIMITER", secret: "SENTINEL_BARE_NO_DELIMITER", wantCode: 2, wantDetail: "invalid --header; use 'K: V'", wantNoDetail: "for key"},
		{name: "leading colon", header: ":SENTINEL_LEADING_COLON", secret: "SENTINEL_LEADING_COLON", wantCode: 2, wantDetail: "invalid --header; use 'K: V'", wantNoDetail: "for key"},
		{name: "leading whitespace then colon", header: "  :SENTINEL_WHITESPACE_COLON", secret: "SENTINEL_WHITESPACE_COLON", wantCode: 2, wantDetail: "invalid --header; use 'K: V'", wantNoDetail: "for key"},
		{name: "space separated no colon", header: "X-Api-Key SENTINEL_SPACE_NO_COLON", secret: "SENTINEL_SPACE_NO_COLON", wantCode: 2, wantDetail: "invalid --header for key \"X-Api-Key\"; use 'K: V'"},
		{name: "empty key", header: ": SENTINEL_EMPTY_KEY", secret: "SENTINEL_EMPTY_KEY", wantCode: 2, wantDetail: "invalid --header; use 'K: V'", wantNoDetail: "for key"},
		{name: "empty value", header: "X-Api-Key:", secret: "SENTINEL_EMPTY_VALUE", wantCode: 0, wantDetail: "Configured ", wantHeader: "X-Api-Key", wantValue: ""},
		{name: "multiple colons", header: "X-Api-Key:a:SENTINEL_MULTI", secret: "SENTINEL_MULTI", wantCode: 0, wantDetail: "Configured ", wantHeader: "X-Api-Key", wantValue: "a:SENTINEL_MULTI"},
		{name: "valid control", header: "X-Trace: ok", secret: "SENTINEL_VALID_VALUE", wantCode: 0, wantDetail: "Configured ", wantHeader: "X-Trace", wantValue: "ok"},
		{name: "colon only", header: ":", secret: "SENTINEL_COLON_ONLY", wantCode: 2, wantDetail: "invalid --header; use 'K: V'", wantNoDetail: "for key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--header", tt.header}, &stdout, &stderr, func() (harness.Env, error) {
				return testEnv(home), nil
			})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), tt.secret) || strings.Contains(stderr.String(), tt.secret) {
				t.Fatalf("header handling leaked sentinel; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if tt.wantDetail != "" && !strings.Contains(stdout.String()+stderr.String(), tt.wantDetail) {
				t.Fatalf("header handling missing detail %q; stdout=%q stderr=%q", tt.wantDetail, stdout.String(), stderr.String())
			}
			if tt.wantNoDetail != "" && strings.Contains(stdout.String()+stderr.String(), tt.wantNoDetail) {
				t.Fatalf("header handling included forbidden detail %q; stdout=%q stderr=%q", tt.wantNoDetail, stdout.String(), stderr.String())
			}
			if tt.wantCode == 0 {
				headers := cursorHeaders(t, home)
				if got := headers[tt.wantHeader]; got != tt.wantValue {
					t.Fatalf("header %q = %q, want %q", tt.wantHeader, got, tt.wantValue)
				}
			}
		})
	}
}

func TestInstallRejectsAuthorizationHeaderWithToken(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--header", "Authorization: other"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("config was written, stat err = %v", err)
	}
}

func TestInstallURLCredentialGate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		url         string
		wantCode    int
		wantWritten bool
		wantURL     string
	}{
		{name: "scheme-less normalizes to https", url: "ballast.now/mcp", wantCode: 0, wantWritten: true, wantURL: "https://ballast.now/mcp"},
		{name: "https succeeds", url: "https://ballast.now/mcp", wantCode: 0, wantWritten: true, wantURL: "https://ballast.now/mcp"},
		{name: "localhost http succeeds", url: "http://localhost:3000/mcp", wantCode: 0, wantWritten: true, wantURL: "http://localhost:3000/mcp"},
		{name: "127 http succeeds", url: "http://127.0.0.1:3000/mcp", wantCode: 0, wantWritten: true, wantURL: "http://127.0.0.1:3000/mcp"},
		{name: "ipv6 localhost http succeeds", url: "http://[::1]:3000/mcp", wantCode: 0, wantWritten: true, wantURL: "http://[::1]:3000/mcp"},
		{name: "public http refuses", url: "http://insecure.test/mcp", wantCode: 1},
		{name: "missing host refuses", url: "https:///mcp", wantCode: 1},
		{name: "wrong scheme refuses", url: "ftp://ballast.now/mcp", wantCode: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Main([]string{"install", tt.url, "SUPERSECRET_URL_TOKEN", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) {
				return testEnv(home), nil
			})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "SUPERSECRET_URL_TOKEN") {
				t.Fatalf("token leaked; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if tt.wantWritten {
				if got := onlyCursorServer(t, home)["url"]; got != tt.wantURL {
					t.Fatalf("url = %q, want %q", got, tt.wantURL)
				}
				if tt.url == "ballast.now/mcp" {
					if !strings.Contains(stdout.String(), "Configured Cursor \"ballast\" → https://ballast.now/mcp (") {
						t.Fatalf("stdout missing normalized URL success line: %q", stdout.String())
					}
					if strings.Contains(stdout.String(), "→ ballast.now/mcp") {
						t.Fatalf("stdout echoed raw scheme-less URL: %q", stdout.String())
					}
				}
			} else if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("config was written, stat err = %v", err)
			}
		})
	}
}

func TestInstallDoesNotPrintCodexEnvLineWhenCodexNotTargeted(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_cursor", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Codex uses an environment variable") {
		t.Fatalf("stdout printed false Codex note: %q", stdout.String())
	}
}

func TestInstallExplicitCodexPrintsManualInstructionsAndFails(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_codex", "--client", "codex"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if strings.Contains(stdout.String(), "Codex uses an environment variable") {
		t.Fatalf("stdout printed false Codex note: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("codex config was written, stat err = %v", err)
	}
	for _, want := range []string{
		"hitch cannot configure Codex automatically yet",
		"[mcp_servers.example]",
		"bearer_token_env_var = \"HITCH_TOKEN_EXAMPLE\"",
		"export HITCH_TOKEN_EXAMPLE=YOUR_TOKEN",
		"Not configured: Codex: hitch cannot configure Codex automatically yet",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), "tok_SENTINEL_codex") {
		t.Fatalf("Codex manual output leaked token; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestInstallWriteFailureOutputDoesNotLeakToken(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{not-json", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_failure", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if strings.Contains(stdout.String()+stderr.String(), "tok_SENTINEL_failure") {
		t.Fatalf("write failure leaked token; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestInstallDryRunRefusesMalformedConfigLikeRealRun(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "mcp.json")
	bad := "{not-json"
	writeFile(t, path, bad, 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--dry-run"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if strings.Contains(stdout.String(), "Dry run: would update") || !strings.Contains(stdout.String(), "Not configured: Cursor:") {
		t.Fatalf("dry-run output = %q, want refusal without would-update", stdout.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read malformed config: %v", err)
	}
	if string(raw) != bad {
		t.Fatalf("malformed config changed to %q", string(raw))
	}
}

func TestInstallDryRunHealthyConfigReportsWouldUpdate(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "mcp.json")
	writeFile(t, path, "{\n  \"mcpServers\": {}\n}\n", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--dry-run"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Dry run: would update "+path) {
		t.Fatalf("dry-run output = %q, want would-update path", stdout.String())
	}
}

func TestInstallNameOverrideWins(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--name", "override-name"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	servers := cursorServers(t, home)
	if servers["override-name"] == nil || servers["example"] != nil {
		t.Fatalf("servers = %#v, want override-name only", servers)
	}
}

func TestAmbiguousNameConfirmIsRequired(t *testing.T) {
	confirmed := false
	name, err := installpkg.ResolveName("https://api.example.test/mcp", "", false, func(inferred string) (bool, error) {
		confirmed = true
		return inferred == "api", nil
	})
	if err != nil || name != "api" || !confirmed {
		t.Fatalf("ResolveName = %q, %v, confirmed=%v", name, err, confirmed)
	}
	if _, err := installpkg.ResolveName("https://api.example.test/mcp", "", true, nil); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("yes ambiguous error = %v, want --name", err)
	}
}

func TestInstallAutoDetectedCodexIsManualOnly(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, ".cursor"))
	mkdirAll(t, filepath.Join(home, ".codex"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--yes"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); err != nil {
		t.Fatalf("cursor config was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("codex config was written, stat err = %v", err)
	}
	if !strings.Contains(stdout.String(), "Configured Cursor \"example\" → https://mcp.example.test/mcp ("+filepath.Join(home, ".cursor", "mcp.json")+")") || !strings.Contains(stdout.String(), "hitch cannot configure Codex automatically yet") {
		t.Fatalf("stdout missing configured cursor or Codex manual note: %q", stdout.String())
	}
}

func TestInstallAutoDetectedCodexOnlyPrintsManualInstructions(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, ".codex"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--yes"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "hitch cannot configure Codex automatically yet") || !strings.Contains(stdout.String(), "export HITCH_TOKEN_EXAMPLE=YOUR_TOKEN") {
		t.Fatalf("stdout missing Codex manual note: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no detected file-writer harnesses selected") {
		t.Fatalf("stderr missing no-writers error: %q", stderr.String())
	}
}

func TestInstallExplicitClientWithTokenStdinDoesNotPrompt(t *testing.T) {
	home := t.TempDir()
	root := NewRootCommand(func() (harness.Env, error) { return testEnv(home), nil })
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader("stdin-token\n"))
	root.SetArgs([]string{"install", "https://mcp.example.test/mcp", "--client", "cursor", "--token-stdin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "Use inferred server name") || strings.Contains(stdout.String()+stderr.String(), "Install ") {
		t.Fatalf("explicit non-TTY install prompted; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPickerSelectionUsesRememberedPreference(t *testing.T) {
	targets := []installpkg.Target{
		{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"}, Path: "/cursor.json"},
		{Client: harness.DetectionResult{ID: "gemini-cli", Name: "Gemini CLI"}, Path: "/gemini.json"},
	}
	selected := defaultSelectedTargetIDs(targets, map[string]bool{"gemini-cli": true})
	if strings.Join(selected, ",") != "gemini-cli" {
		t.Fatalf("selected = %#v, want gemini-cli", selected)
	}
	chosen := targetsBySelectedIDs(targets, selected)
	if len(chosen) != 1 || chosen[0].Client.ID != "gemini-cli" {
		t.Fatalf("chosen = %#v, want gemini-cli", chosen)
	}
}

func TestPickerSelectionDefaultsToAllAndAllowsEmpty(t *testing.T) {
	targets := []installpkg.Target{
		{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"}, Path: "/cursor.json"},
		{Client: harness.DetectionResult{ID: "gemini-cli", Name: "Gemini CLI"}, Path: "/gemini.json"},
	}
	selected := defaultSelectedTargetIDs(targets, nil)
	if strings.Join(selected, ",") != "cursor,gemini-cli" {
		t.Fatalf("selected = %#v, want both targets", selected)
	}
	if chosen := targetsBySelectedIDs(targets, nil); len(chosen) != 0 {
		t.Fatalf("chosen = %#v, want empty selection", chosen)
	}
}

func TestInstallClientSelectionBypassesPickerAndValidatesNames(t *testing.T) {
	home := t.TempDir()
	pickerCalled := false
	_, err := installpkg.InstallRemote(installpkg.Options{
		URL:     "https://mcp.example.test/mcp",
		Name:    "example",
		Headers: map[string]string{"Authorization": "Bearer tok"},
		Clients: []string{"cursor"},
		Yes:     false,
		NonTTY:  false,
		Env:     testEnv(home),
		PickTargets: func([]installpkg.Target, map[string]bool) ([]installpkg.Target, error) {
			pickerCalled = true
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if pickerCalled {
		t.Fatalf("explicit --client path called picker")
	}
	_, err = installpkg.InstallRemote(installpkg.Options{URL: "https://mcp.example.test/mcp", Name: "example", Clients: []string{"unknown"}, Env: testEnv(t.TempDir())})
	if err == nil || !strings.Contains(err.Error(), "unknown file-writer client") {
		t.Fatalf("unknown client error = %v", err)
	}
}

func TestInstallForgetClearsPreferences(t *testing.T) {
	home := t.TempDir()
	prefs := filepath.Join(home, ".config", "hitch", "preferences.json")
	writeFile(t, prefs, "{\"clients\":[\"cursor\"]}\n", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--forget"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(prefs); !os.IsNotExist(err) {
		t.Fatalf("preferences still exist after --forget, stat err = %v", err)
	}
}

func TestInstallNonInteractiveDoesNotSavePreferences(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "hitch", "preferences.json")); !os.IsNotExist(err) {
		t.Fatalf("non-interactive install saved preferences, stat err = %v", err)
	}
}

func TestInstallPartialFailureContinuesAndSummarizesWrittenFiles(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{not-json", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--client", "gemini"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	geminiPath := filepath.Join(home, ".gemini", "settings.json")
	if _, err := os.Stat(geminiPath); err != nil {
		t.Fatalf("healthy harness was not written: %v", err)
	}
	if !strings.Contains(stdout.String(), "Configured Gemini CLI \"example\" → https://mcp.example.test/mcp ("+geminiPath+")") || !strings.Contains(stdout.String(), "Not configured: Cursor:") {
		t.Fatalf("summary missing written path or failure: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestInstallExitCodeIsOneWhenNoHarnessConfigured(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{not-json", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestScanCLIReportsThreeOutcomesAndDoesNotPrintToken(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\": {\"example\": {\"headers\": {\"Authorization\": \"Bearer SCAN_CLI_SECRET\"}, \"url\": \"https://mcp.example.test/mcp\"}}}\n", 0o600)
	writeFile(t, filepath.Join(home, ".gemini", "settings.json"), "{not-json SCAN_PARSE_SECRET", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"scan", "example"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"Cursor\t" + filepath.Join(home, ".cursor", "mcp.json") + "\thas entry (holds a credential)", "Claude Code\t" + filepath.Join(home, ".claude.json") + "\tno entry", "Gemini CLI\t" + filepath.Join(home, ".gemini", "settings.json") + "\tUNREADABLE - cannot verify"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("scan output missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), "SCAN_CLI_SECRET") || strings.Contains(stdout.String()+stderr.String(), "SCAN_PARSE_SECRET") {
		t.Fatalf("scan leaked secret; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestScanAndUninstallCLIRefuseNameThatSanitizesDifferently(t *testing.T) {
	for _, args := range [][]string{{"scan", "ZSentinel"}, {"uninstall", "ZSentinel", "--client", "cursor"}} {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\":{\"ZSentinel\":{\"headers\":{\"Authorization\":\"Bearer CASE_SECRET\"}}}}", 0o600)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Main(args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
		if code == 0 {
			t.Fatalf("Main(%v) exit code = 0, want refusal", args)
		}
		if !strings.Contains(stdout.String()+stderr.String(), "exact stored key") {
			t.Fatalf("output missing exact-key refusal; stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "CASE_SECRET") {
			t.Fatalf("refusal leaked token; stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	}
}

func TestScanAndUninstallCLIReportCodexAsUnverifiable(t *testing.T) {
	home := t.TempDir()
	codexPath := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, codexPath, "[mcp_servers.example]\nurl = \"https://mcp.example.test/mcp\"\nbearer_token = \"CODEX_CLI_SECRET\"\n", 0o600)
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\":{\"example\":{\"url\":\"https://mcp.example.test/mcp\",\"headers\":{\"Authorization\":\"Bearer CURSOR_SECRET\"}}}}", 0o600)
	for _, tt := range []struct {
		args     []string
		wantCode int
		wantOut  string
	}{
		{args: []string{"scan", "example"}, wantCode: 0, wantOut: "Codex\t" + codexPath + "\tUNREADABLE - cannot verify (hitch cannot configure Codex automatically yet"},
		{args: []string{"uninstall", "example", "--yes"}, wantCode: 1, wantOut: "UNREADABLE - cannot verify: Codex (" + codexPath + ") - hitch cannot configure Codex automatically yet"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Main(tt.args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
		if code != tt.wantCode {
			t.Fatalf("Main(%v) code = %d, want %d; stdout=%q stderr=%q", tt.args, code, tt.wantCode, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), tt.wantOut) {
			t.Fatalf("stdout missing %q: %q", tt.wantOut, stdout.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "CODEX_CLI_SECRET") || strings.Contains(stdout.String()+stderr.String(), "CURSOR_SECRET") {
			t.Fatalf("Codex reporting leaked secret; stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	}
}

func TestUninstallCLIExitCodesAndSummaries(t *testing.T) {
	for _, tt := range []struct {
		name     string
		setup    func(string)
		wantCode int
		wantOut  string
	}{
		{name: "removed some", setup: func(home string) {
			writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\": {\"example\": {\"headers\": {\"Authorization\": \"Bearer UNINSTALL_CLI_SECRET\"}, \"url\": \"https://mcp.example.test/mcp\"}, \"other\": {\"url\": \"https://other/mcp\"}}}\n", 0o600)
		}, wantCode: 0, wantOut: "Removed Cursor \"example\""},
		{name: "removed none", setup: func(home string) {
			writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\": {\"other\": {\"url\": \"https://other/mcp\"}}}\n", 0o600)
		}, wantCode: 0, wantOut: "No matching \"example\" entries removed"},
		{name: "could not verify", setup: func(home string) {
			writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{not-json UNINSTALL_PARSE_SECRET", 0o600)
		}, wantCode: 1, wantOut: "UNREADABLE - cannot verify: Cursor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tt.setup(home)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Main([]string{"uninstall", "example", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Fatalf("stdout missing %q: %q", tt.wantOut, stdout.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "UNINSTALL_CLI_SECRET") || strings.Contains(stdout.String()+stderr.String(), "UNINSTALL_PARSE_SECRET") {
				t.Fatalf("uninstall leaked secret; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUninstallAbsentAndUnreadableOutcomesStayDistinct(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		wantCode   int
		wantOutput string
	}{
		{name: "absent is clean no-op", body: "{\"mcpServers\": {\"other\": {\"url\": \"https://other/mcp\"}}}\n", wantCode: 0, wantOutput: "No matching \"example\" entries removed"},
		{name: "unreadable is not clean", body: "{not-json DISTINCT_UNREADABLE_SECRET", wantCode: 1, wantOutput: "UNREADABLE - cannot verify: Cursor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), tt.body, 0o600)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Main([]string{"uninstall", "example", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Fatalf("stdout missing %q: %q", tt.wantOutput, stdout.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "DISTINCT_UNREADABLE_SECRET") {
				t.Fatalf("uninstall leaked unreadable sentinel; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUninstallNonTTYWithoutYesOrClientExitsTwoAndWritesNothing(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "mcp.json")
	before := "{\"mcpServers\": {\"example\": {\"headers\": {\"Authorization\": \"Bearer NONTTY_UNINSTALL_SECRET\"}, \"url\": \"https://mcp.example.test/mcp\"}}}\n"
	writeFile(t, path, before, 0o600)
	mkdirAll(t, filepath.Join(home, ".cursor"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"uninstall", "example"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty refusal path", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--yes") || !strings.Contains(stderr.String(), "--client") {
		t.Fatalf("stderr = %q, want --yes and --client", stderr.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := string(raw); got != before {
		t.Fatalf("non-TTY uninstall changed config to %q", got)
	}
	if strings.Contains(stdout.String()+stderr.String(), "NONTTY_UNINSTALL_SECRET") {
		t.Fatalf("non-TTY uninstall leaked secret; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestUninstallSummaryReportsKeptEntries(t *testing.T) {
	result := installpkg.UninstallResult{
		Name: "x",
		Kept: []installpkg.ScanResult{{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"}, Path: "/tmp/cursor.json", HoldsCredential: true}},
	}
	var out bytes.Buffer
	if err := printUninstallSummary(&out, result); err != nil {
		t.Fatalf("printUninstallSummary returned error: %v", err)
	}
	if strings.Contains(out.String(), "No matching") || !strings.Contains(out.String(), "Kept Cursor \"x\" (/tmp/cursor.json, holds a credential)") {
		t.Fatalf("kept summary output = %q", out.String())
	}
}

func TestUninstallPickerModelSeparatesUnreadableFromSelectable(t *testing.T) {
	targets := []installpkg.ScanResult{{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"}, Path: "/tmp/cursor.json", HoldsCredential: true}}
	unreadable := []installpkg.ScanResult{{Client: harness.DetectionResult{ID: "zed", Name: "Zed"}, Path: "/tmp/zed.json"}}
	options, warnings := uninstallPickerModel(targets, unreadable)
	if len(options) != 1 || options[0].ID != "cursor" || !strings.Contains(options[0].Label, "holds a credential") {
		t.Fatalf("options = %#v, want cursor selectable", options)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Zed") || !strings.Contains(warnings[0], "unreadable") {
		t.Fatalf("warnings = %#v, want zed unreadable warning", warnings)
	}
}

func TestPickUninstallTargetsUsesModelAndFiltersUnreadableSelections(t *testing.T) {
	targets := []installpkg.ScanResult{{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"}, Path: "/tmp/cursor.json", HoldsCredential: true}}
	unreadable := []installpkg.ScanResult{{Client: harness.DetectionResult{ID: "zed", Name: "Zed"}, Path: "/tmp/zed.json"}}
	oldRun := runUninstallPicker
	t.Cleanup(func() { runUninstallPicker = oldRun })
	runUninstallPicker = func(name string, options []huh.Option[string], selected *[]string) error {
		if name != "x" {
			t.Fatalf("picker name = %q, want x", name)
		}
		if len(options) != 1 || options[0].Value != "cursor" || !strings.Contains(options[0].Key, "holds a credential") {
			t.Fatalf("options = %#v, want only cursor option", options)
		}
		*selected = []string{"zed", "cursor"}
		return nil
	}
	var out bytes.Buffer
	selected, err := pickUninstallTargets(&out, "x", targets, unreadable)
	if err != nil {
		t.Fatalf("pickUninstallTargets returned error: %v", err)
	}
	if len(selected) != 1 || selected[0].Client.ID != "cursor" {
		t.Fatalf("selected = %#v, want cursor only", selected)
	}
	if !strings.Contains(out.String(), "Zed") || !strings.Contains(out.String(), "unreadable") {
		t.Fatalf("picker warnings output = %q, want unreadable zed warning", out.String())
	}
}

func TestSelectUninstallTargetsHonorsChoicesAndExcludesUnreadable(t *testing.T) {
	targets := []installpkg.ScanResult{
		{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"}, Path: "/tmp/cursor.json"},
		{Client: harness.DetectionResult{ID: "gemini-cli", Name: "Gemini CLI"}, Path: "/tmp/gemini.json"},
		{Client: harness.DetectionResult{ID: "opencode", Name: "opencode"}, Path: "/tmp/opencode.json"},
	}
	unreadable := []installpkg.ScanResult{
		{Client: harness.DetectionResult{ID: "zed", Name: "Zed"}, Path: "/tmp/zed.json"},
		{Client: harness.DetectionResult{ID: "cursor", Name: "Cursor Unreadable"}, Path: "/tmp/unreadable-cursor.json"},
	}
	for _, tt := range []struct {
		name      string
		chosenIDs []string
		wantIDs   string
	}{
		{name: "chosen only in target order", chosenIDs: []string{"opencode", "cursor"}, wantIDs: "cursor,opencode"},
		{name: "unchecked target omitted", chosenIDs: []string{"gemini-cli"}, wantIDs: "gemini-cli"},
		{name: "unreadable chosen id omitted", chosenIDs: []string{"zed", "cursor"}, wantIDs: "cursor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selected := selectUninstallTargets(targets, unreadable, tt.chosenIDs)
			ids := make([]string, 0, len(selected))
			for _, target := range selected {
				ids = append(ids, target.Client.ID)
				if strings.Contains(target.Path, "unreadable") || target.Client.ID == "zed" {
					t.Fatalf("selected unreadable target: %#v", target)
				}
			}
			if got := strings.Join(ids, ","); got != tt.wantIDs {
				t.Fatalf("selected ids = %q, want %q", got, tt.wantIDs)
			}
		})
	}
}

func TestPromptCLIIncludesManualClientsAndCodexNotYetImplemented(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"prompt", "mcp.example.test/mcp"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"Claude Desktop", "JetBrains", "Codex", "hitch cannot configure Codex automatically yet", "https://mcp.example.test/mcp", "bearer_token_env_var"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("prompt output missing %q:\n%s", want, stdout.String())
		}
	}
}

func cursorAuthorization(t *testing.T, home string) string {
	t.Helper()
	return cursorHeaders(t, home)["Authorization"]
}

func cursorHeaders(t *testing.T, home string) map[string]string {
	t.Helper()
	headers := onlyCursorServer(t, home)["headers"].(map[string]any)
	out := map[string]string{}
	for key, value := range headers {
		out[key] = value.(string)
	}
	return out
}

func onlyCursorServer(t *testing.T, home string) map[string]any {
	t.Helper()
	servers := cursorServers(t, home)
	if len(servers) != 1 {
		t.Fatalf("cursor servers = %#v, want exactly one", servers)
	}
	for _, raw := range servers {
		return raw.(map[string]any)
	}
	panic("unreachable")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read token failure") }

func cursorServers(t *testing.T, home string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatalf("read cursor config: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode cursor config: %v", err)
	}
	return data["mcpServers"].(map[string]any)
}
