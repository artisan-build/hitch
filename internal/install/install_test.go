package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/artisan-build/hitch/internal/harness"
)

const (
	testURL   = "https://mcp.example.test/mcp"
	testToken = "tok_SENTINEL_<secret>&value"
)

type shapeCase struct {
	id       string
	key      string
	path     string
	expected map[string]any
}

func TestInstallFreshFileExactEntryShapesAndMode(t *testing.T) {
	t.Parallel()

	for _, tt := range shapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := expectedPath(home, tt.id)

			res, err := InstallRemote(baseOptions(env, tt.id))
			if err != nil {
				t.Fatalf("InstallRemote returned error: %v", err)
			}
			if len(res.Written) != 1 || res.Written[0] != path {
				t.Fatalf("written paths = %#v, want %q", res.Written, path)
			}
			assertMode0600(t, path)
			if tt.id == "codex" {
				data := readTOML(t, path)
				server := data["mcp_servers"].(map[string]any)["example"].(map[string]any)
				if server["url"] != testURL || server["bearer_token_env_var"] != "HITCH_TOKEN_EXAMPLE" {
					t.Fatalf("codex entry = %#v", server)
				}
				if strings.Contains(readFile(t, path), testToken) {
					t.Fatalf("codex persisted token")
				}
				return
			}
			data := readJSON(t, path)
			got := data[tt.key].(map[string]any)["example"].(map[string]any)
			assertJSONEqual(t, got, tt.expected)
		})
	}
}

func TestExistingConfigsPreserveUnrelatedFormatOtherServersAndAreIdempotent(t *testing.T) {
	t.Parallel()

	for _, tt := range shapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := expectedPath(home, tt.id)
			if tt.id == "codex" {
				writeFile(t, path, "# keep me\n[profile]\nname = \"default\"\n\n[mcp_servers.other]\nurl = \"https://other/mcp\"\n", 0o600)
			} else {
				writeFile(t, path, "{\n  \"zeta\": true,\n  \""+tt.key+"\": {\n    \"other\": {\"url\": \"https://other/mcp\"}\n  },\n  \"alpha\": false\n}\n", 0o600)
			}

			_, err := InstallRemote(baseOptions(env, tt.id))
			if err != nil {
				t.Fatalf("InstallRemote returned error: %v", err)
			}
			first := readFile(t, path)
			_, err = InstallRemote(baseOptions(env, tt.id))
			if err != nil {
				t.Fatalf("second InstallRemote returned error: %v", err)
			}
			second := readFile(t, path)
			if first != second {
				t.Fatalf("second write was not byte-identical\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			if tt.id == "codex" {
				if !strings.Contains(second, "# keep me") || !strings.Contains(second, "[profile]") || !strings.Contains(second, "[mcp_servers.other]") {
					t.Fatalf("codex unrelated TOML not preserved:\n%s", second)
				}
				return
			}
			if !strings.Contains(second, "\"zeta\": true") || !strings.Contains(second, "\"alpha\": false") || !strings.Contains(second, "\"other\"") {
				t.Fatalf("json unrelated content not preserved:\n%s", second)
			}
		})
	}
}

func TestMalformedConfigsAreRefusedAndUnchanged(t *testing.T) {
	t.Parallel()

	for _, tt := range shapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := expectedPath(home, tt.id)
			bad := "{not valid json"
			if tt.id == "codex" {
				bad = "[broken\n"
			}
			writeFile(t, path, bad, 0o600)

			_, err := InstallRemote(baseOptions(env, tt.id))
			if err == nil {
				t.Fatalf("InstallRemote returned nil error")
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not name %q", err.Error(), path)
			}
			if got := readFile(t, path); got != bad {
				t.Fatalf("malformed config changed to %q", got)
			}
		})
	}
}

func TestCodexRefusesInlineMCPServersTable(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "codex")
	raw := "mcp_servers = { other = { url = \"https://other/mcp\" } }\n"
	writeFile(t, path, raw, 0o600)
	_, err := InstallRemote(baseOptions(testEnv(home), "codex"))
	if err == nil || !strings.Contains(err.Error(), "inline table") {
		t.Fatalf("error = %v, want inline table", err)
	}
	if got := readFile(t, path); got != raw {
		t.Fatalf("inline TOML changed to %q", got)
	}
}

func TestSymlinkedConfigUpdatesTargetAndKeepsLink(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	link := expectedPath(home, "claude-code")
	target := filepath.Join(home, "target.json")
	writeFile(t, target, "{\"mcpServers\": {}}\n", 0o600)
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := InstallRemote(baseOptions(env, "claude-code"))
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config symlink was replaced")
	}
	data := readJSON(t, target)
	if data["mcpServers"].(map[string]any)["example"] == nil {
		t.Fatalf("target was not updated")
	}
}

func TestNameInference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url       string
		want      string
		ambiguous bool
	}{
		{url: "https://ballast.now/mcp", want: "ballast"},
		{url: "https://mcp.context7.com/mcp", want: "context7"},
		{url: "https://api.example.com/mcp", want: "api", ambiguous: true},
		{url: "https://www.example.com/mcp", want: "www", ambiguous: true},
	}
	for _, tt := range tests {
		got, ambiguous, err := InferName(tt.url)
		if err != nil {
			t.Fatalf("InferName(%q): %v", tt.url, err)
		}
		if got != tt.want || ambiguous != tt.ambiguous {
			t.Fatalf("InferName(%q) = %q, %v; want %q, %v", tt.url, got, ambiguous, tt.want, tt.ambiguous)
		}
	}
	_, err := ResolveName("https://api.example.com/mcp", "", true, nil)
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("ambiguous yes error = %v, want --name", err)
	}
}

func TestDryRunWritesNothingAndMasksToken(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	var out strings.Builder
	opts := baseOptions(env, "cursor")
	opts.DryRun = true
	opts.Stdout = &out
	_, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if strings.Contains(out.String(), testToken) || !strings.Contains(out.String(), "Bearer ***") {
		t.Fatalf("dry-run output leaked or failed to mask token: %q", out.String())
	}
	if _, err := os.Stat(expectedPath(home, "cursor")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config, stat err = %v", err)
	}
}

func TestPreferencesContainOnlySelectionAndMode0600(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	makeMarker(t, home, "cursor")
	makeMarker(t, home, "gemini-cli")
	opts := baseOptions(env)
	opts.Clients = nil
	opts.Yes = false
	opts.NonTTY = false
	opts.PickTargets = func(targets []Target, _ map[string]bool) ([]Target, error) { return targets[:1], nil }
	_, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	prefs := filepath.Join(home, ".config", "hitch", "preferences.json")
	assertMode0600(t, prefs)
	raw := readFile(t, prefs)
	if strings.Contains(raw, testToken) || strings.Contains(raw, testURL) {
		t.Fatalf("preferences contain token or URL: %s", raw)
	}
	if !strings.Contains(raw, "cursor") {
		t.Fatalf("preferences missing selection: %s", raw)
	}
}

func baseOptions(env harness.Env, clientIDs ...string) Options {
	return Options{
		URL:     testURL,
		Name:    "example",
		Headers: map[string]string{"Authorization": "Bearer " + testToken},
		Clients: clientIDs,
		Yes:     true,
		NonTTY:  true,
		Env:     env,
	}
}

func shapeCases(home string) []shapeCase {
	return []shapeCase{
		{id: "claude-code", key: "mcpServers", path: filepath.Join(home, ".claude.json"), expected: map[string]any{"type": "http", "url": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
		{id: "cursor", key: "mcpServers", path: filepath.Join(home, ".cursor", "mcp.json"), expected: map[string]any{"url": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
		{id: "codex", key: "mcp_servers", path: filepath.Join(home, ".codex", "config.toml")},
		{id: "windsurf", key: "mcpServers", path: filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), expected: map[string]any{"serverUrl": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
		{id: "zed", key: "context_servers", path: filepath.Join(home, ".config", "zed", "settings.json"), expected: map[string]any{"url": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
		{id: "vscode", key: "servers", path: filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json"), expected: map[string]any{"type": "http", "url": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
		{id: "gemini-cli", key: "mcpServers", path: filepath.Join(home, ".gemini", "settings.json"), expected: map[string]any{"httpUrl": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
		{id: "opencode", key: "mcp", path: filepath.Join(home, ".config", "opencode", "opencode.json"), expected: map[string]any{"type": "remote", "url": testURL, "headers": map[string]any{"Authorization": "Bearer " + testToken}}},
	}
}

func expectedPath(home string, id string) string {
	for _, tt := range shapeCases(home) {
		if tt.id == id {
			return tt.path
		}
	}
	panic(id)
}

func makeMarker(t *testing.T, home string, id string) {
	t.Helper()
	markers := map[string]string{
		"cursor":     filepath.Join(home, ".cursor"),
		"gemini-cli": filepath.Join(home, ".gemini"),
	}
	if err := os.MkdirAll(markers[id], 0o700); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
}

func testEnv(home string) harness.Env {
	return harness.Env{Home: home, XDGConfigHome: filepath.Join(home, ".config"), AppData: filepath.Join(home, "AppData", "Roaming"), GOOS: "darwin"}
}

func writeFile(t *testing.T, path string, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(raw)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &data); err != nil {
		t.Fatalf("json parse %q: %v", path, err)
	}
	return data
}

func readTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	data := map[string]any{}
	if _, err := toml.Decode(readFile(t, path), &data); err != nil {
		t.Fatalf("toml parse %q: %v", path, err)
	}
	return data
}

func assertMode0600(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode for %q = %o, want 600", path, info.Mode().Perm())
	}
}

func assertJSONEqual(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	gotBytes, _ := json.Marshal(got)
	wantBytes, _ := json.Marshal(want)
	if string(gotBytes) != string(wantBytes) {
		t.Fatalf("entry = %s, want %s", gotBytes, wantBytes)
	}
}
