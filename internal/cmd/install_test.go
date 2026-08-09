package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artisan-build/hitch/internal/harness"
	installpkg "github.com/artisan-build/hitch/internal/install"
)

func TestInstallNonTTYWithoutYesOrClientExitsNonZeroAndWritesNothing(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, ".cursor"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_non_tty"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "--yes") || !strings.Contains(stderr.String(), "--client") {
		t.Fatalf("stderr = %q, want --yes and --client", stderr.String())
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

func TestInstallHeaderParseErrorDoesNotEchoSecret(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--header", "X-Api-Key SUPERSECRET_NO_COLON"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "SUPERSECRET_NO_COLON") || strings.Contains(combined, "X-Api-Key SUPERSECRET_NO_COLON") {
		t.Fatalf("header parse error leaked secret; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "X-Api-Key") {
		t.Fatalf("header parse error should name only the key; stderr=%q", stderr.String())
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

func TestInstallDoesNotPrintCodexEnvLineWhenCodexWriteFails(t *testing.T) {
	home := t.TempDir()
	codexConfig := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, codexConfig, "[broken\n", 0o600)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok_SENTINEL_codex", "--client", "codex"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if strings.Contains(stdout.String(), "Codex uses an environment variable") {
		t.Fatalf("stdout printed false Codex note: %q", stdout.String())
	}
	if strings.Count(stdout.String()+stderr.String(), string(os.PathSeparator)+"config.toml") != 1 {
		t.Fatalf("failure detail should be printed once, stdout=%q stderr=%q", stdout.String(), stderr.String())
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

func TestInstallNameOverrideWins(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "cursor", "--name", "override-name"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
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

func TestInstallPrintsCodexEnvLineOnSuccess(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"install", "https://mcp.example.test/mcp", "tok", "--client", "codex"}, &stdout, &stderr, func() (harness.Env, error) {
		return testEnv(home), nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex uses an environment variable; export HITCH_TOKEN_EXAMPLE") {
		t.Fatalf("stdout missing Codex note: %q", stdout.String())
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
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	geminiPath := filepath.Join(home, ".gemini", "settings.json")
	if _, err := os.Stat(geminiPath); err != nil {
		t.Fatalf("healthy harness was not written: %v", err)
	}
	if !strings.Contains(stdout.String(), "Configured "+geminiPath) || !strings.Contains(stdout.String(), "Not configured: Cursor:") {
		t.Fatalf("summary missing written path or failure: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func cursorAuthorization(t *testing.T, home string) string {
	t.Helper()
	servers := cursorServers(t, home)
	entry := servers["example"].(map[string]any)
	if entry == nil {
		entry = servers["override-name"].(map[string]any)
	}
	return entry["headers"].(map[string]any)["Authorization"].(string)
}

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
