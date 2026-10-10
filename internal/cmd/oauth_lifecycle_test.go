package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

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
	code := Main([]string{"prompt", "https://USERINFO_SENTINEL@example.test/mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL", "--oauth"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{
		"Claude Desktop", "Customize > Connectors", harness.ClaudeDesktopOAuthSetupSource,
		"JetBrains", "does not establish a native OAuth login step", harness.JetBrainsMCPSetupSource,
		"Codex", "codex mcp add example --url https://example.test/mcp", "codex mcp login example", harness.CodexOAuthSetupSource,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("OAuth prompt missing %q:\n%s", want, stdout.String())
		}
	}
	for _, forbidden := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL", "Bearer", "bearer", "YOUR_TOKEN", "export "} {
		if strings.Contains(stdout.String(), forbidden) || strings.Contains(stderr.String(), forbidden) {
			t.Fatalf("OAuth prompt contains %q: stdout=%q stderr=%q", forbidden, stdout.String(), stderr.String())
		}
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
	if code := Main([]string{"prompt", "mcp.example.test/mcp"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil }); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	url := "https://mcp.example.test/mcp"
	name := "example"
	want := fmt.Sprintf("Claude Desktop: add a stdio MCP entry that runs mcp-remote for %s; hitch does not write it because remote HTTP requires a local proxy and Node runtime.\n\nJetBrains: add this MCP server through the JetBrains MCP UI for %s; hitch does not write it because the dialog has no Authorization-headers field.\n\nCodex: hitch does not install Codex automatically yet, but hitch scan and uninstall can verify and remove this manual entry later. Add this to Codex config manually:\n[%s]\nurl = %q\nbearer_token_env_var = \"%s\"\n\nBefore starting Codex, run:\nexport %s=YOUR_TOKEN\n", url, url, install.CodexServerTableHeader(name), url, install.CodexTokenEnvVar(name), install.CodexTokenEnvVar(name))
	if stdout.String() != want {
		t.Fatalf("inconclusive prompt changed:\ngot:  %q\nwant: %q", stdout.String(), want)
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
		if strings.Contains(scanOut.String()+scanErr.String()+uninstallOut.String()+uninstallErr.String(), secret) {
			t.Fatalf("lifecycle output leaked %s", secret)
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
