package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			data := readJSON(t, path)
			got := data[tt.key].(map[string]any)["renamed"].(map[string]any)
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
			writeFile(t, path, "{\n  \"zeta\": true,\n  \""+tt.key+"\": {\n    \"other\": {\"url\": \"https://other/mcp\"}\n  },\n  \"alpha\": false\n}\n", 0o600)

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
			if !strings.Contains(second, "\"zeta\": true") || !strings.Contains(second, "\"alpha\": false") || !strings.Contains(second, "\"other\"") {
				t.Fatalf("json unrelated content not preserved:\n%s", second)
			}
			if strings.Contains(second, "\n  ,") || !strings.Contains(second, "},\n    \"renamed\"") || !strings.Contains(second, "\n  },\n  \"alpha\"") {
				t.Fatalf("json insertion formatting is not hand-readable:\n%s", second)
			}
		})
	}
}

func TestJSONInstallTargetsOnlyTopLevelConfigKey(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "claude-code")
	var projects strings.Builder
	for i := 0; i < 32; i++ {
		if i > 0 {
			projects.WriteString(",\n")
		}
		projects.WriteString("    \"/project-")
		projects.WriteString(string(rune('a' + i%26)))
		projects.WriteString("\": {\n      \"mcpServers\": {\n        \"nested\": {\"url\": \"https://nested/mcp\"}\n      }\n    }")
	}
	writeFile(t, path, "{\n  \"alpha\": true,\n  \"projects\": {\n"+projects.String()+"\n  },\n  \"omega\": true,\n  \"mcpServers\": {\n    \"existing\": {\"url\": \"https://top-level/mcp\"}\n  }\n}\n", 0o600)

	_, err := InstallRemote(baseOptions(testEnv(home), "claude-code"))
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	data := readJSON(t, path)
	topServers := data["mcpServers"].(map[string]any)
	if topServers["renamed"] == nil {
		t.Fatalf("top-level mcpServers was not updated: %#v", topServers)
	}
	projectMap := data["projects"].(map[string]any)
	for name, rawProject := range projectMap {
		servers := rawProject.(map[string]any)["mcpServers"].(map[string]any)
		if servers["renamed"] != nil {
			t.Fatalf("nested project %s was modified: %#v", name, servers)
		}
	}
}

func TestJSONUpdateTargetsOnlyTopLevelServerName(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	writeFile(t, path, "{\n  \"mcpServers\": {\n    \"other\": {\n      \"renamed\": {\"url\": \"https://nested-should-stay/mcp\"}\n    },\n    \"renamed\": {\"url\": \"https://old-top-level/mcp\"}\n  }\n}\n", 0o600)
	_, err := InstallRemote(baseOptions(testEnv(home), "cursor"))
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	data := readJSON(t, path)
	servers := data["mcpServers"].(map[string]any)
	nested := servers["other"].(map[string]any)["renamed"].(map[string]any)
	if nested["url"] != "https://nested-should-stay/mcp" {
		t.Fatalf("nested server name was modified: %#v", nested)
	}
	top := servers["renamed"].(map[string]any)
	if top["url"] != testURL {
		t.Fatalf("top-level server not updated: %#v", top)
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

func TestZedMalformedJSONMentionsJSONCComments(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "zed")
	writeFile(t, path, "{not valid json", 0o600)
	_, err := InstallRemote(baseOptions(testEnv(home), "zed"))
	if err == nil || !strings.Contains(err.Error(), "JSONC comments") {
		t.Fatalf("Zed malformed error = %v, want JSONC comments hint", err)
	}
}

func TestAtomicWriteImplementationUsesExclusiveTempAndRename(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("install.go")
	if err != nil {
		t.Fatalf("read install.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{"os.O_EXCL", "os.Rename", "0o600"} {
		if !strings.Contains(source, want) {
			t.Fatalf("writeAtomic source missing %s", want)
		}
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
	if data["mcpServers"].(map[string]any)["renamed"] == nil {
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

func TestEmptySanitizedNameErrorsAndWritesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	opts := baseOptions(testEnv(home), "cursor")
	opts.Name = "!!!"
	_, err := InstallRemote(opts)
	if err == nil || !strings.Contains(err.Error(), "invalid after sanitizing") {
		t.Fatalf("error = %v, want invalid sanitized name", err)
	}
	if _, statErr := os.Stat(expectedPath(home, "cursor")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid name wrote config, stat err = %v", statErr)
	}
}

func TestDeclinedAmbiguousNameErrorsAndWritesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	opts := baseOptions(testEnv(home))
	opts.URL = "https://api.example.test/mcp"
	opts.Name = ""
	opts.Clients = nil
	opts.Yes = false
	opts.NonTTY = false
	opts.ConfirmName = func(string) (bool, error) { return false, nil }
	opts.PickTargets = func(targets []Target, preferred map[string]bool) ([]Target, error) {
		t.Fatalf("PickTargets should not be called after name decline")
		return nil, nil
	}
	_, err := InstallRemote(opts)
	if err == nil || !strings.Contains(err.Error(), "was not confirmed") {
		t.Fatalf("error = %v, want declined confirmation", err)
	}
	for _, tt := range shapeCases(home) {
		if _, statErr := os.Stat(tt.path); !os.IsNotExist(statErr) {
			t.Fatalf("declined name wrote %s, stat err = %v", tt.path, statErr)
		}
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
	if strings.Contains(out.String(), testToken) || !strings.Contains(out.String(), "\"Authorization\": \"***\"") {
		t.Fatalf("dry-run output leaked or failed to mask token: %q", out.String())
	}
	if _, err := os.Stat(expectedPath(home, "cursor")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config, stat err = %v", err)
	}
}

func TestDryRunMasksCustomHeaderValues(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var out strings.Builder
	opts := baseOptions(testEnv(home), "cursor")
	opts.Headers["X-Api-Key"] = "SUPERSECRET_HEADER_VALUE"
	opts.DryRun = true
	opts.Stdout = &out
	_, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if strings.Contains(out.String(), "SUPERSECRET_HEADER_VALUE") || !strings.Contains(out.String(), "\"X-Api-Key\": \"***\"") {
		t.Fatalf("dry-run custom header leaked or was not masked: %q", out.String())
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

func TestPreferencesRejectRelativeXDGConfigHome(t *testing.T) {
	t.Parallel()

	env := testEnv(t.TempDir())
	env.XDGConfigHome = "relative-xdg"
	_, err := LoadPreferences(env)
	if err == nil || !strings.Contains(err.Error(), "XDG_CONFIG_HOME") {
		t.Fatalf("LoadPreferences error = %v, want XDG_CONFIG_HOME", err)
	}
}

func TestInteractiveInstallPropagatesRelativePreferencePathError(t *testing.T) {
	t.Parallel()

	env := testEnv(t.TempDir())
	env.XDGConfigHome = "relative-xdg"
	_, err := InstallRemote(Options{
		URL:     testURL,
		Name:    "renamed",
		Headers: map[string]string{"Authorization": "Bearer " + testToken},
		Yes:     false,
		NonTTY:  false,
		Env:     env,
		PickTargets: func(targets []Target, preferred map[string]bool) ([]Target, error) {
			return targets, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "XDG_CONFIG_HOME") {
		t.Fatalf("InstallRemote error = %v, want XDG_CONFIG_HOME", err)
	}
}

func baseOptions(env harness.Env, clientIDs ...string) Options {
	return Options{
		URL:     testURL,
		Name:    "renamed",
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
