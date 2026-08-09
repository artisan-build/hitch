package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artisan-build/hitch/internal/harness"
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
