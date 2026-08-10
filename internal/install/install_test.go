package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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

func TestInstallStdioFreshFileExactEntryShapesAndMode(t *testing.T) {
	t.Parallel()

	for _, tt := range stdioShapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := expectedPath(home, tt.id)

			res, err := InstallStdio(stdioOptions(env, tt.id))
			if err != nil {
				t.Fatalf("InstallStdio returned error: %v", err)
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

func TestInstallStdioExistingConfigsPreserveOtherServersAndAreIdempotent(t *testing.T) {
	t.Parallel()

	for _, tt := range stdioShapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := expectedPath(home, tt.id)
			writeFile(t, path, "{\n  \"zeta\": true,\n  \""+tt.key+"\": {\n    \"other\": {\"command\": \"old\"}\n  },\n  \"alpha\": false\n}\n", 0o600)

			_, err := InstallStdio(stdioOptions(env, tt.id))
			if err != nil {
				t.Fatalf("InstallStdio returned error: %v", err)
			}
			first := readFile(t, path)
			_, err = InstallStdio(stdioOptions(env, tt.id))
			if err != nil {
				t.Fatalf("second InstallStdio returned error: %v", err)
			}
			second := readFile(t, path)
			if first != second {
				t.Fatalf("second write was not byte-identical\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			if !strings.Contains(second, "\"zeta\": true") || !strings.Contains(second, "\"alpha\": false") || !strings.Contains(second, "\"other\"") {
				t.Fatalf("json unrelated content not preserved:\n%s", second)
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

func TestInstallStdioMalformedConfigsAreRefusedAndUnchanged(t *testing.T) {
	t.Parallel()

	for _, tt := range stdioShapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := expectedPath(home, tt.id)
			bad := "{not valid json"
			writeFile(t, path, bad, 0o600)

			_, err := InstallStdio(stdioOptions(testEnv(home), tt.id))
			if err == nil {
				t.Fatalf("InstallStdio returned nil error")
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

func TestInstallStdioEmptySanitizedNameErrorsAndWritesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	opts := stdioOptions(testEnv(home), "cursor")
	opts.Name = "!!!"
	_, err := InstallStdio(opts)
	if err == nil || !strings.Contains(err.Error(), "non-empty server name") {
		t.Fatalf("InstallStdio error = %v, want non-empty server name", err)
	}
	if _, statErr := os.Stat(expectedPath(home, "cursor")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid stdio name wrote config, stat err = %v", statErr)
	}
}

func TestInstallStdioEmptyArgsAreOmittedForEveryClient(t *testing.T) {
	t.Parallel()

	for _, tt := range stdioShapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			opts := stdioOptions(testEnv(home), tt.id)
			opts.Args = nil
			_, err := InstallStdio(opts)
			if err != nil {
				t.Fatalf("InstallStdio returned error: %v", err)
			}
			server := readJSON(t, expectedPath(home, tt.id))[tt.key].(map[string]any)["renamed"].(map[string]any)
			if tt.id == "opencode" {
				command := server["command"].([]any)
				if len(command) != 1 || command[0] != "npx" {
					t.Fatalf("opencode command = %#v, want command only", command)
				}
				return
			}
			if _, ok := server["args"]; ok {
				t.Fatalf("empty args key was written for %s stdio entry: %#v", tt.id, server)
			}
			if server["command"] != "npx" || server["env"] == nil {
				t.Fatalf("stdio entry missing command or env positive controls: %#v", server)
			}
		})
	}
}

func TestInstallStdioDryRunMasksEnvValuesAndWritesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var out strings.Builder
	opts := stdioOptions(testEnv(home), "cursor")
	opts.StdioEnv["API_KEY"] = "STDIO_ENV_SENTINEL_SECRET"
	opts.DryRun = true
	opts.Stdout = &out
	_, err := InstallStdio(opts)
	if err != nil {
		t.Fatalf("InstallStdio returned error: %v", err)
	}
	if strings.Contains(out.String(), "STDIO_ENV_SENTINEL_SECRET") || !strings.Contains(out.String(), "\"API_KEY\": \"***\"") {
		t.Fatalf("dry-run output leaked or failed to mask env: %q", out.String())
	}
	if _, err := os.Stat(expectedPath(home, "cursor")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config, stat err = %v", err)
	}
}

func TestInstallStdioAndRemoteReplaceSameServerEntry(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	if _, err := InstallRemote(baseOptions(testEnv(home), "cursor")); err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if _, err := InstallStdio(stdioOptions(testEnv(home), "cursor")); err != nil {
		t.Fatalf("InstallStdio returned error: %v", err)
	}
	server := readJSON(t, path)["mcpServers"].(map[string]any)["renamed"].(map[string]any)
	if server["url"] != nil || server["command"] != "npx" {
		t.Fatalf("stdio did not replace remote entry: %#v", server)
	}
	if _, err := InstallRemote(baseOptions(testEnv(home), "cursor")); err != nil {
		t.Fatalf("second InstallRemote returned error: %v", err)
	}
	server = readJSON(t, path)["mcpServers"].(map[string]any)["renamed"].(map[string]any)
	if server["command"] != nil || server["url"] != testURL {
		t.Fatalf("remote did not replace stdio entry: %#v", server)
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

func TestAtomicWriteFreshFileTempModeAndFinalMode(t *testing.T) {
	home := t.TempDir()
	path := expectedPath(home, "cursor")
	observed := false
	writeAtomicBeforeRename = func(tmpName string) error {
		info, err := os.Stat(tmpName)
		if err != nil {
			return err
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("temp mode = %o, want 600", info.Mode().Perm())
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("fresh target existed before rename, stat err = %v", err)
		}
		observed = true
		return nil
	}
	t.Cleanup(func() { writeAtomicBeforeRename = nil })

	_, err := InstallRemote(baseOptions(testEnv(home), "cursor"))
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if !observed {
		t.Fatalf("temp file was not observed before rename")
	}
	assertMode0600(t, path)
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

func TestEmptyInferredNameErrorsAndWritesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	opts := baseOptions(testEnv(home), "cursor")
	opts.URL = "https://!!!/mcp"
	opts.Name = ""
	_, err := InstallRemote(opts)
	if err == nil || !strings.Contains(err.Error(), "could not infer a usable server name") {
		t.Fatalf("error = %v, want unusable inferred name", err)
	}
	if _, statErr := os.Stat(expectedPath(home, "cursor")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid inferred name wrote config, stat err = %v", statErr)
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

func TestDryRunRefusesMalformedConfigAndLeavesItUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	bad := "{not-json"
	writeFile(t, path, bad, 0o600)
	var out strings.Builder
	opts := baseOptions(testEnv(home), "cursor")
	opts.DryRun = true
	opts.Stdout = &out
	res, err := InstallRemote(opts)
	if err == nil || len(res.Failures) != 1 {
		t.Fatalf("InstallRemote err = %v failures = %#v, want refusal", err, res.Failures)
	}
	if strings.Contains(out.String(), "Would write") {
		t.Fatalf("dry-run promised write for malformed config: %q", out.String())
	}
	if got := readFile(t, path); got != bad {
		t.Fatalf("malformed config changed to %q", got)
	}
}

func TestDryRunHealthyConfigWouldWriteAndLeavesItUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	before := "{\n  \"mcpServers\": {}\n}\n"
	writeFile(t, path, before, 0o600)
	var out strings.Builder
	opts := baseOptions(testEnv(home), "cursor")
	opts.DryRun = true
	opts.Stdout = &out
	res, err := InstallRemote(opts)
	if err != nil || len(res.WouldWrite) != 1 {
		t.Fatalf("InstallRemote err = %v wouldWrite = %#v, want success", err, res.WouldWrite)
	}
	if !strings.Contains(out.String(), "Would write Cursor") {
		t.Fatalf("dry-run missing would-write detail: %q", out.String())
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("dry-run changed config to %q", got)
	}
}

func TestInteractiveDryRunLeavesWholeHomeUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	makeMarker(t, home, "cursor")
	before := snapshotTree(t, home)
	var out strings.Builder
	opts := baseOptions(env)
	opts.Clients = nil
	opts.Yes = false
	opts.NonTTY = false
	opts.DryRun = true
	opts.Stdout = &out
	opts.PickTargets = func(targets []Target, preferred map[string]bool) ([]Target, error) { return targets, nil }
	res, err := InstallRemote(opts)
	if err != nil || len(res.WouldWrite) != 1 {
		t.Fatalf("InstallRemote err = %v wouldWrite = %#v", err, res.WouldWrite)
	}
	after := snapshotTree(t, home)
	if before != after {
		t.Fatalf("interactive dry-run changed HOME\nbefore:\n%s\nafter:\n%s", before, after)
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

func TestCorruptPreferencesAreIgnoredForInteractiveInstall(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	makeMarker(t, home, "cursor")
	writeFile(t, filepath.Join(home, ".config", "hitch", "preferences.json"), "{broken", 0o600)
	opts := baseOptions(env)
	opts.Clients = nil
	opts.Yes = false
	opts.NonTTY = false
	opts.PickTargets = func(targets []Target, preferred map[string]bool) ([]Target, error) {
		if preferred != nil {
			t.Fatalf("preferred = %#v, want nil after corrupt preferences", preferred)
		}
		return targets, nil
	}
	_, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
}

func TestInteractiveSavePreferencesFailureIsReturned(t *testing.T) {
	home := t.TempDir()
	env := testEnv(home)
	makeMarker(t, home, "cursor")
	writeAtomicBeforeRename = func(tmpName string) error {
		if strings.Contains(tmpName, "preferences.json") {
			return os.ErrPermission
		}
		return nil
	}
	t.Cleanup(func() { writeAtomicBeforeRename = nil })
	opts := baseOptions(env)
	opts.Clients = nil
	opts.Yes = false
	opts.NonTTY = false
	opts.PickTargets = func(targets []Target, preferred map[string]bool) ([]Target, error) { return targets, nil }
	res, err := InstallRemote(opts)
	if err == nil {
		t.Fatalf("InstallRemote returned nil error, want SavePreferences failure")
	}
	if len(res.Written) != 1 {
		t.Fatalf("written = %#v, want cursor write before preference failure", res.Written)
	}
}

func TestDuplicateTopLevelConfigKeyRefusedAndUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	raw := "{\n  \"mcpServers\": {},\n  \"mcpServers\": {}\n}\n"
	writeFile(t, path, raw, 0o600)
	_, err := InstallRemote(baseOptions(testEnv(home), "cursor"))
	if err == nil || !strings.Contains(err.Error(), "duplicate top-level key") {
		t.Fatalf("InstallRemote error = %v, want duplicate key", err)
	}
	if got := readFile(t, path); got != raw {
		t.Fatalf("duplicate-key config changed to %q", got)
	}
}

func TestDanglingSymlinkConfigIsRefused(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	missingTarget := filepath.Join(home, "missing.json")
	if err := os.Symlink(missingTarget, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := InstallRemote(baseOptions(testEnv(home), "cursor"))
	if err == nil || !strings.Contains(err.Error(), "dangling symlink") {
		t.Fatalf("InstallRemote error = %v, want dangling symlink", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dangling symlink was replaced")
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

func TestUninstallRemovesEntryForEveryClientAndPreservesOtherContent(t *testing.T) {
	t.Parallel()

	for _, tt := range shapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := expectedPath(home, tt.id)
			before := "{\n  \"zeta\": true,\n  \"" + tt.key + "\": {\n    \"alpha\": {\"url\": \"https://alpha/mcp\"},\n    \"renamed\": {\"headers\": {\"Authorization\": \"Bearer REMOVE_SENTINEL_SECRET\"}, \"url\": \"https://remove/mcp\"},\n    \"omega\": {\"url\": \"https://omega/mcp\"}\n  },\n  \"alpha\": false\n}\n"
			writeFile(t, path, before, 0o600)

			res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{tt.id}, Yes: true, NonTTY: true, Env: env})
			if err != nil {
				t.Fatalf("Uninstall returned error: %v", err)
			}
			if len(res.Removed) != 1 || res.Removed[0].Path != path || !res.Removed[0].HoldsCredential {
				t.Fatalf("removed = %#v, want credential-bearing removal from %s", res.Removed, path)
			}
			after := readFile(t, path)
			if after == before {
				t.Fatalf("uninstall made no byte change")
			}
			if strings.Contains(after, "renamed") || strings.Contains(after, "REMOVE_SENTINEL_SECRET") {
				t.Fatalf("removed entry or token survived:\n%s", after)
			}
			data := readJSON(t, path)
			if data["zeta"] != true || data["alpha"] != false {
				t.Fatalf("top-level unrelated keys not preserved: %#v", data)
			}
			servers := data[tt.key].(map[string]any)
			if servers["renamed"] != nil || servers["alpha"] == nil || servers["omega"] == nil {
				t.Fatalf("server map after removal = %#v, want alpha and omega only", servers)
			}
		})
	}
}

func TestUninstallAfterInstallOnCompactForeignConfigLeavesValidJSON(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	foreign := `{"mcpServers":{"keep":{"url":"https://keep.test/mcp"}},"otherKey":1}`
	writeFile(t, path, foreign, 0o600)
	if _, err := InstallRemote(baseOptions(testEnv(home), "cursor")); err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	installed := readFile(t, path)
	if installed == foreign || !strings.Contains(installed, "renamed") {
		t.Fatalf("install did not create mixed foreign document:\n%s", installed)
	}
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v removed = %#v", err, res.Removed)
	}
	data := readJSON(t, path)
	servers := data["mcpServers"].(map[string]any)
	if servers["renamed"] != nil || servers["keep"] == nil || data["otherKey"] != float64(1) {
		t.Fatalf("after uninstall data = %#v, want keep and otherKey preserved with renamed gone", data)
	}
}

func TestUninstallLastServerLeavesValidEmptyServerMap(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	writeFile(t, path, "{\n  \"mcpServers\": {\n    \"renamed\": {\"headers\": {\"Authorization\": \"Bearer REMOVE_LAST_SECRET\"}, \"url\": \"https://remove/mcp\"}\n  },\n  \"keep\": true\n}\n", 0o600)
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v removed = %#v", err, res.Removed)
	}
	data := readJSON(t, path)
	servers := data["mcpServers"].(map[string]any)
	if len(servers) != 0 || data["keep"] != true {
		t.Fatalf("after uninstall data = %#v, want empty mcpServers and keep=true", data)
	}
}

func TestUninstallMissingServerIsCleanNoOpAndNoWrite(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	before := "{\n  \"mcpServers\": {\n    \"other\": {\"url\": \"https://other/mcp\"}\n  }\n}\n"
	writeFile(t, path, before, 0o600)
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if len(res.Removed) != 0 || len(res.NotPresent) != 1 {
		t.Fatalf("result = %#v, want one not-present no-op", res)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("missing server changed config:\n%s", got)
	}
}

func TestScanAndUninstallTreatMalformedConfigAsUnreadableAndUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	before := "{not-json REMOVE_PARSE_SECRET"
	writeFile(t, path, before, 0o600)
	scans, err := Scan(testEnv(home), "renamed", []string{"cursor"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanUnreadable || !strings.Contains(scans[0].Detail, path) {
		t.Fatalf("scan = %#v, want unreadable naming path", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err == nil || len(res.Unreadable) != 1 {
		t.Fatalf("Uninstall err = %v result = %#v, want unreadable error", err, res)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("malformed config changed to %q", got)
	}
}

func TestScanAndUninstallTreatPermissionDeniedAsUnreadableAndUnchanged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read mode 000 files")
	}
	home := t.TempDir()
	path := expectedPath(home, "cursor")
	before := "{\n  \"mcpServers\": {\n    \"renamed\": {\"headers\": {\"Authorization\": \"Bearer PERMISSION_SECRET\"}, \"url\": \"https://remove/mcp\"}\n  }\n}\n"
	writeFile(t, path, before, 0o600)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	scans, err := Scan(testEnv(home), "renamed", []string{"cursor"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanUnreadable {
		t.Fatalf("scan = %#v, want unreadable permission outcome", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
		t.Fatalf("Uninstall err = %v result = %#v, want unreadable only", err, res)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("restore mode: %v", err)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("unreadable config changed to %q", got)
	}
}

func TestScanDetectsCredentialWithoutPrintingIt(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	writeFile(t, path, "{\"mcpServers\": {\"renamed\": {\"headers\": {\"Authorization\": \"Bearer SCAN_SECRET\"}, \"url\": \"https://remove/mcp\"}}}\n", 0o600)
	scans, err := Scan(testEnv(home), "renamed", []string{"cursor"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanHasEntry || !scans[0].HoldsCredential {
		t.Fatalf("scan = %#v, want has-entry with credential", scans)
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

func stdioOptions(env harness.Env, clientIDs ...string) Options {
	return Options{
		Name:     "renamed",
		Command:  "npx",
		Args:     []string{"-y", "@example/mcp", "arg,with,commas", ""},
		StdioEnv: map[string]string{"API_KEY": "stdio-secret-value"},
		Clients:  clientIDs,
		Yes:      true,
		NonTTY:   true,
		Env:      env,
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	entries := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			entries = append(entries, rel+"/")
			return nil
		}
		body := ""
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			body = string(raw)
		}
		entries = append(entries, rel+" "+info.Mode().String()+" "+body)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	sort.Strings(entries)
	return strings.Join(entries, "\n")
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

func stdioShapeCases(home string) []shapeCase {
	commandArgsEnv := map[string]any{"command": "npx", "args": []any{"-y", "@example/mcp", "arg,with,commas", ""}, "env": map[string]any{"API_KEY": "stdio-secret-value"}}
	commandArgsEnvWithType := map[string]any{"type": "stdio", "command": "npx", "args": []any{"-y", "@example/mcp", "arg,with,commas", ""}, "env": map[string]any{"API_KEY": "stdio-secret-value"}}
	return []shapeCase{
		{id: "claude-code", key: "mcpServers", path: filepath.Join(home, ".claude.json"), expected: commandArgsEnv},
		{id: "cursor", key: "mcpServers", path: filepath.Join(home, ".cursor", "mcp.json"), expected: commandArgsEnvWithType},
		{id: "windsurf", key: "mcpServers", path: filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), expected: commandArgsEnv},
		{id: "zed", key: "context_servers", path: filepath.Join(home, ".config", "zed", "settings.json"), expected: commandArgsEnv},
		{id: "vscode", key: "servers", path: filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json"), expected: commandArgsEnvWithType},
		{id: "gemini-cli", key: "mcpServers", path: filepath.Join(home, ".gemini", "settings.json"), expected: commandArgsEnv},
		{id: "opencode", key: "mcp", path: filepath.Join(home, ".config", "opencode", "opencode.json"), expected: map[string]any{"type": "local", "command": []any{"npx", "-y", "@example/mcp", "arg,with,commas", ""}, "environment": map[string]any{"API_KEY": "stdio-secret-value"}}},
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
