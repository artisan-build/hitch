package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"

	"github.com/artisan-build/hitch/internal/harness"
	"github.com/artisan-build/hitch/internal/install"
)

func TestPromptOAuthOverrideSkipsProbeAndPrintsSourcedManualPaths(t *testing.T) {
	oldProbe := detectOAuth
	detectOAuth = func(string) bool {
		t.Fatal("OAuth override called probe")
		return false
	}
	t.Cleanup(func() { detectOAuth = oldProbe })

	var stdout, stderr bytes.Buffer
	code := Main([]string{"prompt", "https://example.test/mcp", "--oauth"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{
		"Claude Desktop", "Customize > Connectors", harness.ClaudeDesktopOAuthSetupSource,
		"JetBrains", "does not establish a native OAuth login step", harness.JetBrainsMCPSetupSource,
		"Codex", "codex mcp add example --url 'https://example.test/mcp'", "codex mcp login example", harness.CodexOAuthSetupSource,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("OAuth prompt missing %q:\n%s", want, stdout.String())
		}
	}
	for _, forbidden := range []string{"Bearer", "bearer", "YOUR_TOKEN", "export "} {
		if strings.Contains(stdout.String(), forbidden) || strings.Contains(stderr.String(), forbidden) {
			t.Fatalf("OAuth prompt contains %q: stdout=%q stderr=%q", forbidden, stdout.String(), stderr.String())
		}
	}
}

func TestPromptOAuthRefusesSensitiveURLComponentsWithoutEchoingThem(t *testing.T) {
	for _, rawURL := range []string{
		"https://USERINFO_SENTINEL@example.test/mcp",
		"https://example.test/mcp?QUERY_SENTINEL=yes",
		"https://example.test/mcp#FRAGMENT_SENTINEL",
		"https://USERINFO_SENTINEL@./mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL",
	} {
		var stdout, stderr bytes.Buffer
		code := Main([]string{"prompt", rawURL, "--oauth"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
		if code == 0 {
			t.Fatalf("prompt accepted sensitive URL %q", rawURL)
		}
		if !strings.Contains(stderr.String(), "OAuth manual setup") {
			t.Fatalf("missing actionable refusal for %q: stdout=%q stderr=%q", rawURL, stdout.String(), stderr.String())
		}
		for _, secret := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL"} {
			if strings.Contains(stdout.String()+stderr.String(), secret) {
				t.Fatalf("refusal leaked %s: stdout=%q stderr=%q", secret, stdout.String(), stderr.String())
			}
		}
	}
}

func TestOAuthInstallPickerAndNameErrorsDoNotEchoSensitiveURLComponents(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldTerminal := installInputIsTerminal
	oldPicker := runInstallPicker
	installInputIsTerminal = func(io.Reader) bool { return true }
	runInstallPicker = func(url string, _ []huh.Option[string], selected *[]string) error {
		if want := "https://example.test/mcp"; url != want {
			t.Fatalf("picker URL = %q, want %q", url, want)
		}
		*selected = nil
		return nil
	}
	t.Cleanup(func() {
		installInputIsTerminal = oldTerminal
		runInstallPicker = oldPicker
	})

	rawURL := "https://USERINFO_SENTINEL@example.test/mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL"
	var stdout, stderr bytes.Buffer
	code := Main([]string{"install", rawURL, "--oauth", "--name", "example"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
	if code != 0 {
		t.Fatalf("picker install code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, secret := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL"} {
		if strings.Contains(stdout.String()+stderr.String(), secret) {
			t.Fatalf("picker output leaked %s: stdout=%q stderr=%q", secret, stdout.String(), stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	rawURL = "https://USERINFO_SENTINEL@./mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL"
	code = Main([]string{"install", rawURL, "--oauth", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
	if code == 0 {
		t.Fatalf("install accepted unusable host: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, secret := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL"} {
		if strings.Contains(stdout.String()+stderr.String(), secret) {
			t.Fatalf("name error leaked %s: stdout=%q stderr=%q", secret, stdout.String(), stderr.String())
		}
	}
}

func TestPromptOAuthAppliesCredentialTransportPolicy(t *testing.T) {
	for _, tt := range []struct {
		name     string
		url      string
		wantCode int
	}{
		{name: "public plaintext rejected", url: "http://public.example/mcp", wantCode: 1},
		{name: "localhost plaintext allowed", url: "http://localhost:8080/mcp", wantCode: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Main([]string{"prompt", tt.url, "--oauth"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
			if code != tt.wantCode {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if tt.wantCode != 0 && !strings.Contains(stderr.String(), "refusing to send credentials over insecure http URL") {
				t.Fatalf("missing plaintext refusal: %q", stderr.String())
			}
		})
	}
}

func TestPromptDetectedOAuthAndInconclusiveProbePreservesBearerOutput(t *testing.T) {
	oldProbe := detectOAuth
	t.Cleanup(func() { detectOAuth = oldProbe })

	detectOAuth = func(string) bool { return true }
	var oauthOut, oauthErr bytes.Buffer
	if code := Main([]string{"prompt", "https://example.test/mcp"}, &oauthOut, &oauthErr, func() (harness.Env, error) { return harness.Env{}, nil }); code != 0 || !strings.Contains(oauthOut.String(), "codex mcp login example") {
		t.Fatalf("detected OAuth prompt code=%d stdout=%q stderr=%q", code, oauthOut.String(), oauthErr.String())
	}

	detectOAuth = func(string) bool { return false }
	var stdout, stderr bytes.Buffer
	rawURL := "https://USERINFO_SENTINEL@mcp.example.test/mcp?tenant=acme#fragment"
	if code := Main([]string{"prompt", rawURL}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil }); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	url := rawURL
	name := "example"
	want := fmt.Sprintf("Claude Desktop: add a stdio MCP entry that runs mcp-remote for %s; hitch does not write it because remote HTTP requires a local proxy and Node runtime.\n\nJetBrains: add this MCP server through the JetBrains MCP UI for %s; hitch does not write it because the dialog has no Authorization-headers field.\n\nCodex: hitch does not install Codex automatically yet, but hitch scan and uninstall can verify and remove this manual entry later. Add this to Codex config manually:\n[%s]\nurl = %q\nbearer_token_env_var = \"%s\"\n\nBefore starting Codex, run:\nexport %s=YOUR_TOKEN\n", url, url, install.CodexServerTableHeader(name), url, install.CodexTokenEnvVar(name), install.CodexTokenEnvVar(name))
	if stdout.String() != want {
		t.Fatalf("inconclusive prompt changed:\ngot:  %q\nwant: %q", stdout.String(), want)
	}
}

func TestPromptOAuthCodexURLIsShellQuoted(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "executed")
	rawURL := "https://example.test/mcp;touch$IFS" + sentinel
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"prompt", rawURL, "--oauth"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil }); code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var codexLine string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "codex mcp add ") {
			codexLine = line
			break
		}
	}
	if codexLine == "" || !strings.Contains(codexLine, "--url '") {
		t.Fatalf("missing quoted Codex line: %q", stdout.String())
	}
	command := exec.Command("sh", "-c", "codex() { :; }\n"+codexLine)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute printed command: %v: %s", err, output)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("printed Codex line executed injected command: %v", err)
	}
	if got, want := posixSingleQuote("one'two"), `'one'"'"'two'`; got != want {
		t.Fatalf("embedded quote = %q, want %q", got, want)
	}
}

func TestScanAndUninstallOAuthLabelsAndSecretFreeGuidance(t *testing.T) {
	home := t.TempDir()
	env := testEnv(home)
	var installOut, installErr bytes.Buffer
	code := Main([]string{"install", "https://USERINFO_SENTINEL@example.test/mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL", "--oauth", "--name", "example", "--client", "cursor"}, &installOut, &installErr, func() (harness.Env, error) { return env, nil })
	if code != 0 {
		t.Fatalf("install code=%d stdout=%q stderr=%q", code, installOut.String(), installErr.String())
	}
	configRaw, err := os.ReadFile(filepath.Join(home, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, operational := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL"} {
		if !strings.Contains(string(configRaw), operational) {
			t.Fatalf("written config dropped operational URL component %s: %s", operational, configRaw)
		}
	}

	var dryOut, dryErr bytes.Buffer
	code = Main([]string{"install", "https://USERINFO_SENTINEL@example.test/mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL", "--oauth", "--name", "example", "--client", "cursor", "--dry-run"}, &dryOut, &dryErr, func() (harness.Env, error) { return env, nil })
	if code != 0 {
		t.Fatalf("dry-run code=%d stdout=%q stderr=%q", code, dryOut.String(), dryErr.String())
	}

	var scanOut, scanErr bytes.Buffer
	code = Main([]string{"scan", "example"}, &scanOut, &scanErr, func() (harness.Env, error) { return env, nil })
	if code != 0 || !strings.Contains(scanOut.String(), "has entry (OAuth (token held by harness))") {
		t.Fatalf("scan code=%d stdout=%q stderr=%q", code, scanOut.String(), scanErr.String())
	}

	var uninstallOut, uninstallErr bytes.Buffer
	code = Main([]string{"uninstall", "example", "--client", "cursor"}, &uninstallOut, &uninstallErr, func() (harness.Env, error) { return env, nil })
	if code != 0 {
		t.Fatalf("uninstall code=%d stdout=%q stderr=%q", code, uninstallOut.String(), uninstallErr.String())
	}
	for _, want := range []string{"removed the config entry but did not remove Cursor's stored OAuth token", "server's account or token management at https://example.test"} {
		if !strings.Contains(uninstallOut.String(), want) {
			t.Fatalf("uninstall missing %q: %s", want, uninstallOut.String())
		}
	}
	for _, secret := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL"} {
		if strings.Contains(installOut.String()+installErr.String()+dryOut.String()+dryErr.String()+scanOut.String()+scanErr.String()+uninstallOut.String()+uninstallErr.String(), secret) {
			t.Fatalf("lifecycle output leaked %s", secret)
		}
	}
}

func TestUninstallMainPrintsRegistryCleanupFailureAfterRemovalSummary(t *testing.T) {
	oldUninstall := uninstallServer
	uninstallServer = func(install.UninstallOptions) (install.UninstallResult, error) {
		result := install.UninstallResult{
			Name: "example",
			Removed: []install.ScanResult{{
				Client: harness.DetectionResult{ID: "cursor", Name: "Cursor"},
				Path:   "/safe/config",
				Status: install.ScanHasEntry,
				Auth:   install.AuthCredential,
			}},
		}
		return result, fmt.Errorf("config entry removed but OAuth registry cleanup failed: read OAuth installation registry /SENTINEL/registry.json: CLEANUP_SENTINEL")
	}
	t.Cleanup(func() { uninstallServer = oldUninstall })

	var stdout, stderr bytes.Buffer
	code := Main([]string{"uninstall", "example", "--client", "cursor"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
	if code == 0 {
		t.Fatalf("cleanup failure returned success: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `Removed Cursor "example"`) || strings.Contains(stdout.String(), "No matching") {
		t.Fatalf("incorrect removal summary: %q", stdout.String())
	}
	for _, want := range []string{"OAuth registry cleanup failed", "/SENTINEL/registry.json", "CLEANUP_SENTINEL"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}

func TestScanAndUninstallMainReportBlockingRegistryError(t *testing.T) {
	for _, registry := range []string{
		`{"version":1,"installations":[`,
		`{"version":99,"installations":[]}`,
	} {
		for _, args := range [][]string{{"scan", "example"}, {"uninstall", "example", "--client", "cursor"}} {
			home := t.TempDir()
			registryPath := filepath.Join(home, ".config", "hitch", "oauth-installations.json")
			writeFile(t, registryPath, registry, 0o600)
			configPath := filepath.Join(home, ".cursor", "mcp.json")
			config := `{"mcpServers":{"example":{"url":"https://example.test/mcp","headers":{"Authorization":"Bearer SECRET_SENTINEL"}}}}`
			writeFile(t, configPath, config, 0o600)

			var stdout, stderr bytes.Buffer
			code := Main(args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
			if code == 0 {
				t.Fatalf("Main(%v) accepted registry %q", args, registry)
			}
			if !strings.Contains(stderr.String(), registryPath) || (!strings.Contains(stderr.String(), "malformed") && !strings.Contains(stderr.String(), "unsupported version")) {
				t.Fatalf("Main(%v) hid registry error: stdout=%q stderr=%q", args, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), "No matching") {
				t.Fatalf("Main(%v) claimed entry was absent: %q", args, stdout.String())
			}
			if raw, err := os.ReadFile(registryPath); err != nil || string(raw) != registry {
				t.Fatalf("registry changed: %q, %v", raw, err)
			}
			if raw, err := os.ReadFile(configPath); err != nil || string(raw) != config {
				t.Fatalf("config changed: %q, %v", raw, err)
			}
		}
	}
}

func TestUninstallOAuthUsesBothVerifiedLogoutCommands(t *testing.T) {
	for _, tt := range []struct {
		id, name, command string
	}{
		{id: "claude-code", name: "Claude Code", command: "claude mcp logout example"},
		{id: "opencode", name: "opencode", command: "opencode mcp logout example"},
	} {
		var out bytes.Buffer
		result := install.UninstallResult{Name: "example", Removed: []install.ScanResult{{Client: harness.DetectionResult{ID: tt.id, Name: tt.name}, Path: "/safe/config", Status: install.ScanHasEntry, Auth: install.AuthOAuth, OAuthServerURL: "https://example.test/mcp"}}}
		if err := printUninstallSummary(&out, result); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "`"+tt.command+"`") {
			t.Fatalf("%s guidance = %q", tt.id, out.String())
		}
	}
}
