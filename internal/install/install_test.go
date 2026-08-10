package install

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
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
	if err == nil || !strings.Contains(err.Error(), "duplicate key") || !strings.Contains(err.Error(), "top-level object") {
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
	foreign := `{"zzz":{"nested":{"v":true}},"mcpServers":{"keep":{"url":"https://keep.test/mcp"},"later":{"url":"https://later.test/mcp"}},"aaa":1}`
	writeFile(t, path, foreign, 0o600)
	beforeTopOrder := jsonObjectKeyOrder(t, []byte(foreign))
	beforeServersOrder := jsonObjectValueKeyOrder(t, []byte(foreign), "mcpServers")
	if _, err := InstallRemote(baseOptions(testEnv(home), "cursor")); err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	installed := readFile(t, path)
	if installed == foreign || !strings.Contains(installed, "renamed") {
		t.Fatalf("install did not create mixed foreign document:\n%s", installed)
	}
	installedTopOrder := jsonObjectKeyOrder(t, []byte(installed))
	if strings.Join(installedTopOrder, ",") != strings.Join(beforeTopOrder, ",") {
		t.Fatalf("install changed top-level order: before %#v after %#v\n%s", beforeTopOrder, installedTopOrder, installed)
	}
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v removed = %#v", err, res.Removed)
	}
	after := readFile(t, path)
	afterTopOrder := jsonObjectKeyOrder(t, []byte(after))
	if strings.Join(afterTopOrder, ",") != strings.Join(beforeTopOrder, ",") {
		t.Fatalf("uninstall changed top-level order: before %#v after %#v\n%s", beforeTopOrder, afterTopOrder, after)
	}
	afterServersOrder := jsonObjectValueKeyOrder(t, []byte(after), "mcpServers")
	if strings.Join(afterServersOrder, ",") != strings.Join(beforeServersOrder, ",") {
		t.Fatalf("uninstall changed server order: before %#v after %#v\n%s", beforeServersOrder, afterServersOrder, after)
	}
	data := readJSON(t, path)
	servers := data["mcpServers"].(map[string]any)
	if servers["renamed"] != nil || servers["keep"] == nil || servers["later"] == nil || data["aaa"] != float64(1) || data["zzz"] == nil {
		t.Fatalf("after uninstall data = %#v, want keep, later, aaa, and zzz preserved with renamed gone", data)
	}
}

func TestInstallThenUninstallLeavesForeignConfigByteIdentical(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		fixture []byte
	}{
		{
			name:    "compact single line",
			fixture: []byte("{\"zzz\":{\"nested\":{\"v\":true}},\"mcpServers\":{\"keep\":{\"url\":\"https://keep.test/mcp\"},\"later\":{\"url\":\"https://later.test/mcp\"}},\"aaa\":1}\n"),
		},
		{
			name:    "custom indentation",
			fixture: []byte("{\n  \"zzz\"       : {\n    \"nested\" : {\"v\": true}\n  },\n  \"mcpServers\": {\n    \"keep\" : {\n        \"url\" : \"https://keep.test/mcp\"\n    },\n    \"later\": {\"url\": \"https://later.test/mcp\"}\n  },\n  \"aaa\"       : 1\n}\n"),
		},
		{
			name:    "no trailing newline",
			fixture: []byte(`{"zzz":{"nested":{"v":true}},"mcpServers":{"keep":{"url":"https://keep.test/mcp"},"later":{"url":"https://later.test/mcp"}},"aaa":1}`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := expectedPath(home, "cursor")
			writeFile(t, path, string(tt.fixture), 0o600)
			before := append([]byte(nil), tt.fixture...)

			if _, err := InstallRemote(baseOptions(testEnv(home), "cursor")); err != nil {
				t.Fatalf("InstallRemote returned error: %v", err)
			}
			res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err != nil || len(res.Removed) != 1 {
				t.Fatalf("Uninstall err = %v removed = %#v", err, res.Removed)
			}
			after := []byte(readFile(t, path))
			if !bytes.Equal(before, after) {
				t.Fatalf("install-then-uninstall changed foreign config bytes\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func TestUninstallForeignEntryMatchesLiteralExpectedBytes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		before string
		after  string
	}{
		{
			name:   "non-last member",
			before: "{\n  \"mcpServers\": {\n    \"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}},\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"other\": true\n}\n",
			after:  "{\n  \"mcpServers\": {\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"other\": true\n}\n",
		},
		{
			name:   "last member",
			before: "{\n  \"mcpServers\": {\n    \"keep\": {\"url\": \"https://keep.test/mcp\"},\n    \"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}}\n  },\n  \"other\": true\n}\n",
			after:  "{\n  \"mcpServers\": {\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"other\": true\n}\n",
		},
		{
			name:   "only member",
			before: "{\n  \"mcpServers\": {\n    \"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}}\n  },\n  \"other\": true\n}\n",
			after:  "{\n  \"mcpServers\": {\n  },\n  \"other\": true\n}\n",
		},
		{
			name:   "first member",
			before: "{\n  \"mcpServers\": {\n    \"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}},\n    \"middle\": {\"url\": \"https://middle.test/mcp\"},\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"other\": true\n}\n",
			after:  "{\n  \"mcpServers\": {\n    \"middle\": {\"url\": \"https://middle.test/mcp\"},\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"other\": true\n}\n",
		},
		{
			name:   "tab indentation",
			before: "{\n\t\"mcpServers\": {\n\t\t\"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}},\n\t\t\"keep\": {\"url\": \"https://keep.test/mcp\"}\n\t},\n\t\"other\": true\n}\n",
			after:  "{\n\t\"mcpServers\": {\n\t\t\"keep\": {\"url\": \"https://keep.test/mcp\"}\n\t},\n\t\"other\": true\n}\n",
		},
		{
			name:   "crlf",
			before: "{\r\n  \"mcpServers\": {\r\n    \"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}},\r\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\r\n  },\r\n  \"other\": true\r\n}\r\n",
			after:  "{\r\n  \"mcpServers\": {\r\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\r\n  },\r\n  \"other\": true\r\n}\r\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := expectedPath(home, "cursor")
			writeFile(t, path, tt.before, 0o600)
			res, err := Uninstall(UninstallOptions{Name: "x", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err != nil || len(res.Removed) != 1 {
				t.Fatalf("Uninstall err = %v removed = %#v", err, res.Removed)
			}
			if got := readFile(t, path); got != tt.after {
				t.Fatalf("foreign removal bytes mismatch\nwant:\n%s\ngot:\n%s", tt.after, got)
			}
		})
	}
}

func TestUninstallForeignEntryPreservesExpectedBytesForEveryClient(t *testing.T) {
	t.Parallel()

	for _, tt := range shapeCases(t.TempDir()) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := expectedPath(home, tt.id)
			before := "{\n  \"z\": true,\n  \"" + tt.key + "\": {\n    \"x\": {\"url\": \"https://x.test/mcp\", \"headers\": {\"Authorization\": \"Bearer FOREIGN\"}},\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"a\": true\n}\n"
			after := "{\n  \"z\": true,\n  \"" + tt.key + "\": {\n    \"keep\": {\"url\": \"https://keep.test/mcp\"}\n  },\n  \"a\": true\n}\n"
			writeFile(t, path, before, 0o600)
			res, err := Uninstall(UninstallOptions{Name: "x", Clients: []string{tt.id}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err != nil || len(res.Removed) != 1 {
				t.Fatalf("Uninstall err = %v removed = %#v", err, res.Removed)
			}
			if got := readFile(t, path); got != after {
				t.Fatalf("%s foreign removal bytes mismatch\nwant:\n%s\ngot:\n%s", tt.id, after, got)
			}
		})
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

func TestDuplicateServerMapKeyIsUnreadableAndUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := expectedPath(home, "cursor")
	before := "{\"mcpServers\":{\"x\":{\"headers\":{\"Authorization\":\"Bearer DEAD\"}},\"keep\":{\"url\":\"https://keep.test/mcp\"},\"x\":{\"headers\":{\"Authorization\":\"Bearer LIVE\"}}}}"
	writeFile(t, path, before, 0o600)
	scans, err := Scan(testEnv(home), "x", []string{"cursor"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanUnreadable || !strings.Contains(scans[0].Detail, "duplicate key") {
		t.Fatalf("scan = %#v, want unreadable duplicate-key outcome", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "x", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
		t.Fatalf("Uninstall err = %v result = %#v, want unreadable duplicate-key refusal", err, res)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("duplicate-key config changed to %q", got)
	}
}

func TestCodexRemovalNoEntryNoOpIsByteIdentical(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	before := "# user comment\nlocal = 1979-05-27T07:32:00\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\nbearer_token_env_var = \"OTHER_TOKEN\"\n"
	writeFile(t, path, before, 0o600)
	content, changed, err := buildCodexRemoval(path, "missing-sentinel")
	if err != nil {
		t.Fatalf("buildCodexRemoval returned error: %v", err)
	}
	if changed || content != nil {
		t.Fatalf("no-op changed = %v content = %q, want unchanged nil content", changed, string(content))
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("no-op Codex removal changed bytes\nbefore:\n%q\nafter:\n%q", before, got)
	}
}

func TestCodexScanAndUninstallEveryServerSpelling(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		before     string
		after      string
	}{
		{
			name:       "table bare key",
			serverName: "sentinel",
			before:     "title = \"keep\"\n\n[mcp_servers.sentinel]\nurl = \"https://sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_SENTINEL\"\n\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\n",
			after:      "title = \"keep\"\n\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\n",
		},
		{
			name:       "dotted inline key",
			serverName: "sentinel",
			before:     "title = \"keep\"\nmcp_servers.sentinel = { url = \"https://sentinel.test/mcp\", bearer_token_env_var = \"HITCH_TOKEN_SENTINEL\" }\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\n",
			after:      "title = \"keep\"\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\n",
		},
		{
			name:       "fully quoted table keys",
			serverName: "sentinel",
			before:     "title = \"keep\"\n[\"mcp_servers\".\"sentinel\"]\nurl = \"https://sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_SENTINEL\"\n[other]\nvalue = true\n",
			after:      "title = \"keep\"\n[other]\nvalue = true\n",
		},
		{
			name:       "partially quoted table key",
			serverName: "sentinel",
			before:     "title = \"keep\"\n[mcp_servers.\"sentinel\"]\nurl = \"https://sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_SENTINEL\"\n[other]\nvalue = true\n",
			after:      "title = \"keep\"\n[other]\nvalue = true\n",
		},
		{
			name:       "dash requires quoted key",
			serverName: "codex-sentinel",
			before:     "title = \"keep\"\n[mcp_servers.\"codex-sentinel\"]\nurl = \"https://sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_CODEX_SENTINEL\"\n[other]\nvalue = true\n",
			after:      "title = \"keep\"\n[other]\nvalue = true\n",
		},
		{
			name:       "dot requires quoted key",
			serverName: "codex.sentinel",
			before:     "title = \"keep\"\n[mcp_servers.\"codex.sentinel\"]\nurl = \"https://sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_CODEX_SENTINEL\"\n[other]\nvalue = true\n",
			after:      "title = \"keep\"\n[other]\nvalue = true\n",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tt.before, 0o600)

			scans, err := Scan(testEnv(home), tt.serverName, []string{"codex"})
			if err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			if len(scans) != 1 || scans[0].Status != ScanHasEntry || scans[0].HoldsCredential {
				t.Fatalf("Codex scan = %#v, want has-entry without persisted credential", scans)
			}

			res, err := Uninstall(UninstallOptions{Name: tt.serverName, Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err != nil || len(res.Removed) != 1 || res.Removed[0].Client.ID != "codex" {
				t.Fatalf("Uninstall err = %v result = %#v, want Codex removed", err, res)
			}
			if got := readFile(t, path); got != tt.after {
				t.Fatalf("Codex uninstall bytes mismatch\nwant:\n%q\ngot:\n%q", tt.after, got)
			}
		})
	}
}

func TestCodexUnrecognizedMCPServerShapesFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serverName string
		body       string
	}{
		{name: "array table server", serverName: "x", body: "[[mcp_servers.x]]\nurl = \"https://x.test/mcp\"\nbearer_token = \"ARRAY_TABLE_SECRET\"\n"},
		{name: "top level inline mcp_servers", serverName: "x", body: "mcp_servers = { x = { url = \"https://x.test/mcp\", bearer_token = \"INLINE_TOP_SECRET\" } }\n"},
		{name: "dotted credential without parent", serverName: "x", body: "mcp_servers.x.bearer_token = \"DOTTED_SECRET\"\n"},
		{name: "orphan env subtable", serverName: "x", body: "[mcp_servers.x.env]\nAPI_TOKEN = \"ORPHAN_ENV_SECRET\"\n"},
		{name: "array of env tables", serverName: "x", body: "[[mcp_servers.x.env]]\nAPI_TOKEN = \"ARRAY_ENV_SECRET\"\n"},
		{name: "deep dotted inline without parent", serverName: "x", body: "mcp_servers.x.env = { API_TOKEN = \"DEEP_INLINE_SECRET\" }\n"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tt.body, 0o600)

			scans, err := Scan(testEnv(home), tt.serverName, []string{"codex"})
			if err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			if len(scans) != 1 || scans[0].Status != ScanUnreadable {
				t.Fatalf("Codex scan = %#v, want fail-closed unreadable", scans)
			}
			res, err := Uninstall(UninstallOptions{Name: tt.serverName, Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
				t.Fatalf("Uninstall err = %v result = %#v, want unreadable refusal", err, res)
			}
			if got := readFile(t, path); got != tt.body {
				t.Fatalf("fail-closed shape changed bytes\nwant:\n%q\ngot:\n%q", tt.body, got)
			}
		})
	}
}

func TestCodexArrayTableCannotCorruptFollowingArrayTable(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	before := "[[mcp_servers.zzs]]\nurl = \"https://zzs.test/mcp\"\nbearer_token = \"ARRAY_TABLE_SECRET\"\n\n[[shortcuts]]\nname = \"keep\"\n"
	writeFile(t, path, before, 0o600)
	res, err := Uninstall(UninstallOptions{Name: "zzs", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
		t.Fatalf("Uninstall err = %v result = %#v, want fail-closed unreadable refusal", err, res)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("array table corruption guard changed bytes\nwant:\n%q\ngot:\n%q", before, got)
	}
}

// TestCodexRemovalFollowingNeighbourSurvivesByteIdentical holds the one
// property behind every splice defect seen so far: nothing of ours is left
// behind and nothing of the neighbour's is disturbed — whatever the neighbour
// is. Each row is before = head + gap + entry + eaten + neighbour and must
// produce exactly head + neighbour: the separator gap, the entry (with any
// comment block it owns), and any explicitly eaten run disappear, while not
// one byte of the neighbour or the head changes. Rows with a refuseReason
// expect the other honest outcome: an unreadable refusal carrying exactly
// that reason, with the file byte-identical — never a "removal" that leaves a
// descendant of ours behind, and never a refusal that only happens to occur
// for some unrelated cause.
func TestCodexRemovalFollowingNeighbourSurvivesByteIdentical(t *testing.T) {
	t.Parallel()

	const head = "title = \"keep\"\n"
	const gap = "\n"
	const entry = "[mcp_servers.zzs]\nurl = \"https://zzs.invalid/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n"
	const scatterReason = "scatters mcp_servers entry \"zzs\" across the file; cannot verify or remove"
	tests := []struct {
		name         string
		head         string
		gap          string
		entry        string
		eaten        string
		neighbour    string
		startOfFile  bool
		refuseReason string
	}{
		{
			name:      "comment block",
			neighbour: "# neighbour comment one\n# neighbour comment two\n[tail]\nvalue = 1\n",
		},
		{
			name:      "array of tables",
			neighbour: "[[shortcuts]]\nname = \"build\"\n\n[[shortcuts]]\nname = \"test\"\n",
		},
		{
			name:      "bare bracket line",
			neighbour: "[tail]\n",
		},
		{
			name:      "blank line run",
			neighbour: "\n\n[tail]\nvalue = 1\n",
		},
		{
			name:      "crlf document",
			head:      "title = \"keep\"\r\n",
			gap:       "\r\n",
			entry:     "[mcp_servers.zzs]\r\nurl = \"https://zzs.invalid/mcp\"\r\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\r\n",
			neighbour: "[[shortcuts]]\r\nname = \"build\"\r\n",
		},
		{
			name:      "indented table",
			neighbour: "  [tail]\n  value = 1\n",
		},
		{
			name:      "another mcp_servers entry",
			neighbour: "[mcp_servers.other]\nurl = \"https://other.invalid/mcp\"\n",
		},
		{
			name:      "nothing at EOF",
			neighbour: "",
		},
		{
			name:      "comment owned by our entry, comment owned by neighbour",
			entry:     "# comment owned by zzs\n[mcp_servers.zzs]\nurl = \"https://zzs.invalid/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n",
			neighbour: "# comment owned by tail\n[tail]\nvalue = 1\n",
		},
		{
			name:      "floating comment above the gap is not owned",
			head:      "title = \"keep\"\n\n# floating comment\n",
			neighbour: "[tail]\nvalue = 1\n",
		},
		{
			name:      "trailing comment on the header line",
			entry:     "[mcp_servers.zzs] # instance note\nurl = \"https://zzs.invalid/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n",
			neighbour: "[tail]\nvalue = 1\n",
		},
		{
			name:      "trailing whitespace after the header",
			entry:     "[mcp_servers.zzs] \t\nurl = \"https://zzs.invalid/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n",
			neighbour: "[tail]\nvalue = 1\n",
		},
		{
			name:      "crlf document with owned comment block",
			head:      "title = \"keep\"\r\n",
			gap:       "\r\n",
			entry:     "# comment owned by zzs\r\n[mcp_servers.zzs]\r\nurl = \"https://zzs.invalid/mcp\"\r\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\r\n",
			neighbour: "[[shortcuts]]\r\nname = \"build\"\r\n",
		},
		{
			name:        "entry at start of file eats its following separator",
			startOfFile: true,
			eaten:       "\n\n",
			neighbour:   "[tail]\nvalue = 1\n",
		},
		{
			name:         "descendant preceding the root is refused",
			entry:        "[mcp_servers.zzs.env]\nAPI_TOKEN = \"ZZLEFTOVERSECRETZZ\"\n\n[mcp_servers.zzs]\nurl = \"https://zzs.invalid/mcp\"\n",
			neighbour:    "[tail]\nvalue = 1\n",
			refuseReason: scatterReason,
		},
		{
			name:         "descendant after an unrelated table is refused",
			entry:        "[mcp_servers.zzs]\nurl = \"https://zzs.invalid/mcp\"\n",
			neighbour:    "[tail]\nvalue = 1\n\n[mcp_servers.zzs.env]\nAPI_TOKEN = \"ZZLEFTOVERSECRETZZ\"\n",
			refuseReason: scatterReason,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.head == "" {
				tt.head = head
			}
			if tt.gap == "" {
				tt.gap = gap
			}
			if tt.entry == "" {
				tt.entry = entry
			}
			if tt.startOfFile {
				tt.head = ""
				tt.gap = ""
			}
			before := tt.head + tt.gap + tt.entry + tt.eaten + tt.neighbour
			want := tt.head + tt.neighbour
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, before, 0o600)
			res, err := Uninstall(UninstallOptions{Name: "zzs", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if tt.refuseReason != "" {
				if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
					t.Fatalf("Uninstall err = %v result = %#v, want unreadable refusal", err, res)
				}
				if detail := res.Unreadable[0].Detail; !strings.Contains(detail, tt.refuseReason) {
					t.Fatalf("refusal happened for the wrong reason\nwant reason: %q\ngot detail:  %q", tt.refuseReason, detail)
				}
				if got := readFile(t, path); got != before {
					t.Fatalf("refusal changed bytes\nwant:\n%q\ngot:\n%q", before, got)
				}
				return
			}
			if err != nil || len(res.Removed) != 1 {
				t.Fatalf("Uninstall err = %v result = %#v, want removal", err, res)
			}
			got := readFile(t, path)
			if got != want {
				t.Fatalf("neighbour did not survive byte-identically\nwant:\n%q\ngot:\n%q", want, got)
			}
		})
	}
}

func TestCodexDuplicateServerRootsAreUnreadableAndUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	before := "[mcp_servers.x]\nurl = \"https://first.test/mcp\"\n\n[mcp_servers.\"x\"]\nurl = \"https://second.test/mcp\"\nbearer_token = \"DUPLICATE_SECRET\"\n"
	writeFile(t, path, before, 0o600)
	scans, err := Scan(testEnv(home), "x", []string{"codex"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanUnreadable {
		t.Fatalf("Codex duplicate scan = %#v, want unreadable", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "x", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
		t.Fatalf("Uninstall err = %v result = %#v, want duplicate refusal", err, res)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("duplicate roots changed bytes to %q", got)
	}
}

func TestCodexScanWithoutNameIsHeld(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, path, "[mcp_servers.x]\nurl = \"https://x.test/mcp\"\n", 0o600)
	scans, err := Scan(testEnv(home), "", []string{"codex"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanHasEntry {
		t.Fatalf("Codex no-name scan = %#v, want has-entry", scans)
	}
}

func TestCodexUnparseableIsUnreadableAndUnchanged(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	before := "[mcp_servers.sentinel\nurl = \"https://sentinel.test/mcp\"\nbearer_token = \"CODEX_PARSE_SECRET\"\n"
	writeFile(t, path, before, 0o600)
	scans, err := Scan(testEnv(home), "sentinel", []string{"codex"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanUnreadable {
		t.Fatalf("Codex scan = %#v, want unreadable", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "sentinel", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 0 {
		t.Fatalf("Uninstall err = %v result = %#v, want unreadable refusal", err, res)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("unreadable Codex config changed to %q", got)
	}
}

func TestCodexRemovalLeavesParentTableAndRemovesSubtables(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	before := "[mcp_servers]\n\n[mcp_servers.sentinel]\nurl = \"https://sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_SENTINEL\"\n\n[mcp_servers.sentinel.env]\nKEEP = \"remove with sentinel\"\n\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\n"
	after := "[mcp_servers]\n\n[mcp_servers.other]\nurl = \"https://other.test/mcp\"\n"
	writeFile(t, path, before, 0o600)
	res, err := Uninstall(UninstallOptions{Name: "sentinel", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v result = %#v, want removal", err, res)
	}
	if got := readFile(t, path); got != after {
		t.Fatalf("Codex parent/subtable bytes mismatch\nwant:\n%q\ngot:\n%q", after, got)
	}
}

func TestCodexEOFRemovalBoundaryLayoutsAreByteExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		before string
		after  string
	}{
		{
			name:   "leading blank at EOF",
			before: "[a]\nx = 1\n\n[mcp_servers.zzs]\nurl = \"https://zzs.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n",
			after:  "[a]\nx = 1\n",
		},
		{
			name:   "no leading blank at EOF",
			before: "[a]\nx = 1\n[mcp_servers.zzs]\nurl = \"https://zzs.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n",
			after:  "[a]\nx = 1\n",
		},
		{
			name:   "middle with following table",
			before: "[a]\nx = 1\n\n[mcp_servers.zzs]\nurl = \"https://zzs.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_ZZS\"\n\n[b]\ny = 2\n",
			after:  "[a]\nx = 1\n\n[b]\ny = 2\n",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tt.before, 0o600)
			res, err := Uninstall(UninstallOptions{Name: "zzs", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err != nil || len(res.Removed) != 1 {
				t.Fatalf("Uninstall err = %v result = %#v, want removal", err, res)
			}
			if got := readFile(t, path); got != tt.after {
				t.Fatalf("EOF boundary bytes mismatch\nwant:\n%q\ngot:\n%q", tt.after, got)
			}
		})
	}
}

func TestCodexScrubbedFixtureRemovalPreservesNodeReplEnvAndDatetimes(t *testing.T) {
	fixturePath := filepath.Join("testdata", "codex-realistic-scrubbed.toml")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read scrubbed Codex fixture: %v", err)
	}
	assertScrubbedCodexFixture(t, fixture)
	if !bytes.Contains(fixture, []byte("[mcp_servers.node_repl.env]")) || !bytes.Contains(fixture, []byte("computer-use")) {
		t.Fatalf("scrubbed config fixture missing expected node_repl.env or computer-use positive controls")
	}
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	sentinel := "[mcp_servers.\"hitch-pr7-sentinel\"]\nurl = \"https://hitch-pr7-sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_HITCH_PR7_SENTINEL\"\n"
	before := string(fixture) + sentinel
	writeFile(t, path, before, 0o600)

	scans, err := Scan(testEnv(home), "hitch-pr7-sentinel", []string{"codex"})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Status != ScanHasEntry || scans[0].HoldsCredential {
		t.Fatalf("real-copy Codex scan = %#v, want sentinel env-var entry", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "hitch-pr7-sentinel", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v result = %#v, want sentinel removed", err, res)
	}
	if got := readFile(t, path); got != string(fixture) {
		t.Fatalf("scrubbed-fixture removal did not restore original bytes")
	}
	if !strings.Contains(readFile(t, path), "[mcp_servers.node_repl.env]") || !strings.Contains(readFile(t, path), "computer-use") {
		t.Fatalf("scrubbed-fixture removal damaged node_repl.env or computer-use")
	}

	nodeCopyHome := t.TempDir()
	nodeCopyPath := filepath.Join(nodeCopyHome, ".codex", "config.toml")
	writeFile(t, nodeCopyPath, string(fixture), 0o600)
	nodeRes, err := Uninstall(UninstallOptions{Name: "node_repl", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(nodeCopyHome)})
	if err != nil || len(nodeRes.Removed) != 1 {
		t.Fatalf("node_repl uninstall err = %v result = %#v, want removal", err, nodeRes)
	}
	if strings.Contains(readFile(t, nodeCopyPath), "[mcp_servers.node_repl.env]") {
		t.Fatalf("node_repl.env sub-table survived node_repl removal")
	}
}

func TestCodexScrubbedFixtureCommentOwnershipIsByteExact(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "codex-realistic-scrubbed.toml"))
	if err != nil {
		t.Fatalf("read scrubbed Codex fixture: %v", err)
	}
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, path, string(fixture), 0o600)
	res, err := Uninstall(UninstallOptions{Name: "node_repl", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v result = %#v, want node_repl removed", err, res)
	}
	want := "# Scrubbed Codex config fixture.\n# Values are placeholders; structure mirrors a real-world config shape.\nprofile = \"placeholder-profile\"\napproval_policy = \"on-request\"\n# Keep both offsets: they catch any future reserialization that normalizes datetimes.\ncreated_at = 2026-01-02T03:04:05+12:45\nupdated_at = 2026-02-03T04:05:06-03:30\n\n[projects.\"/placeholder/project-alpha\"]\ntrust_level = \"trusted\"\n\n[projects.\"/placeholder/project-beta\"]\ntrust_level = \"untrusted\"\n\n# A dashed server name must stay quoted and survive unrelated removals.\n[mcp_servers.\"computer-use\"]\nurl = \"https://placeholder-computer-use.invalid/mcp\"\nbearer_token_env_var = \"PLACEHOLDER_COMPUTER_USE_TOKEN\"\n\n# This comment intentionally sits immediately above the following table header.\n[mcp_servers.placeholder_keep]\nurl = \"https://placeholder-keep.invalid/mcp\"\nbearer_token_env_var = \"PLACEHOLDER_KEEP_TOKEN\"\n"
	if got := readFile(t, path); got != want {
		t.Fatalf("comment ownership bytes mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestCodexCredentialDetectionMatchesCredentialContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "literal bearer token", body: "[mcp_servers.x]\nurl = \"https://x.test/mcp\"\nbearer_token = \"LITERAL_TOKEN_SECRET\"\n"},
		{name: "env subtable", body: "[mcp_servers.x]\nurl = \"https://x.test/mcp\"\n\n[mcp_servers.x.env]\nAPI_TOKEN = \"ENV_TOKEN_SECRET\"\n"},
		{name: "http headers inline", body: "[mcp_servers.x]\nurl = \"https://x.test/mcp\"\nhttp_headers = { Authorization = \"Bearer HEADER_TOKEN_SECRET\" }\n"},
		{name: "inline env container", body: "mcp_servers.x = { url = \"https://x.test/mcp\", env = { API_TOKEN = \"INLINE_ENV_SECRET\" } }\n"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tt.body, 0o600)
			scans, err := Scan(testEnv(home), "x", []string{"codex"})
			if err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			if len(scans) != 1 || scans[0].Status != ScanHasEntry || !scans[0].HoldsCredential {
				t.Fatalf("Codex scan = %#v, want credential held", scans)
			}
		})
	}
}

func TestCodexScanWithoutNameReportsCredentialHeldByAnyServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		holdsCredential bool
	}{
		{
			name:            "credential in second of two",
			body:            "[mcp_servers.first]\nurl = \"https://first.invalid/mcp\"\n\n[mcp_servers.second]\nurl = \"https://second.invalid/mcp\"\nbearer_token = \"SECOND_POSITION_SECRET\"\n",
			holdsCredential: true,
		},
		{
			name:            "credential in last of three",
			body:            "[mcp_servers.first]\nurl = \"https://first.invalid/mcp\"\n\n[mcp_servers.second]\nurl = \"https://second.invalid/mcp\"\n\n[mcp_servers.third]\nurl = \"https://third.invalid/mcp\"\n\n[mcp_servers.third.env]\nAPI_TOKEN = \"LAST_POSITION_SECRET\"\n",
			holdsCredential: true,
		},
		{
			name:            "no server holds a credential",
			body:            "[mcp_servers.first]\nurl = \"https://first.invalid/mcp\"\n\n[mcp_servers.second]\nurl = \"https://second.invalid/mcp\"\n",
			holdsCredential: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tt.body, 0o600)
			scans, err := Scan(testEnv(home), "", []string{"codex"})
			if err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			if len(scans) != 1 || scans[0].Status != ScanHasEntry || scans[0].HoldsCredential != tt.holdsCredential {
				t.Fatalf("unnamed Codex scan = %#v, want has-entry with HoldsCredential = %v", scans, tt.holdsCredential)
			}
		})
	}
}

func TestCodexServerNamePrefixDoesNotOverreach(t *testing.T) {
	t.Parallel()

	before := "[mcp_servers.node]\nurl = \"https://node.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_NODE\"\n\n[mcp_servers.node_repl]\nurl = \"https://node-repl.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_NODE_REPL\"\n\n[mcp_servers.node_repl.env]\nAPI_TOKEN = \"NODE_REPL_ENV\"\n"
	afterNode := "[mcp_servers.node_repl]\nurl = \"https://node-repl.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_NODE_REPL\"\n\n[mcp_servers.node_repl.env]\nAPI_TOKEN = \"NODE_REPL_ENV\"\n"
	afterNodeRepl := "[mcp_servers.node]\nurl = \"https://node.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_NODE\"\n"
	for _, tt := range []struct {
		name   string
		remove string
		after  string
	}{
		{name: "remove node", remove: "node", after: afterNode},
		{name: "remove node_repl", remove: "node_repl", after: afterNodeRepl},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, before, 0o600)
			res, err := Uninstall(UninstallOptions{Name: tt.remove, Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
			if err != nil || len(res.Removed) != 1 {
				t.Fatalf("Uninstall err = %v result = %#v, want removal", err, res)
			}
			if got := readFile(t, path); got != tt.after {
				t.Fatalf("prefix removal bytes mismatch\nwant:\n%q\ngot:\n%q", tt.after, got)
			}
		})
	}
}

func TestCodexLiveFixtureOptIn(t *testing.T) {
	if os.Getenv("HITCH_LIVE_CODEX_FIXTURE") != "1" {
		t.Skip("live Codex fixture not requested; set HITCH_LIVE_CODEX_FIXTURE=1")
	}
	livePath := filepath.Join(os.Getenv("HOME"), ".codex", "config.toml")
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live Codex fixture: %v", err)
	}
	if !bytes.Contains(live, []byte("[mcp_servers.node_repl.env]")) || !bytes.Contains(live, []byte("computer-use")) {
		t.Fatalf("live config fixture missing expected node_repl.env or computer-use positive controls")
	}
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	sentinel := "[mcp_servers.\"hitch-pr7-sentinel\"]\nurl = \"https://hitch-pr7-sentinel.test/mcp\"\nbearer_token_env_var = \"HITCH_TOKEN_HITCH_PR7_SENTINEL\"\n"
	writeFile(t, path, string(live)+sentinel, 0o600)
	res, err := Uninstall(UninstallOptions{Name: "hitch-pr7-sentinel", Clients: []string{"codex"}, Yes: true, NonTTY: true, Env: testEnv(home)})
	if err != nil || len(res.Removed) != 1 {
		t.Fatalf("Uninstall err = %v result = %#v, want sentinel removed", err, res)
	}
	if got := readFile(t, path); got != string(live) {
		t.Fatalf("live fixture removal did not restore original bytes")
	}
}

func assertScrubbedCodexFixture(t *testing.T, fixture []byte) {
	t.Helper()
	text := string(fixture)
	for _, required := range []string{
		"# Keep both offsets: they catch any future reserialization that normalizes datetimes.",
		"created_at = 2026-01-02T03:04:05+12:45",
		"updated_at = 2026-02-03T04:05:06-03:30",
		"[mcp_servers.node_repl.env]",
		"[mcp_servers.\"computer-use\"]",
		"# This comment intentionally sits immediately above the following table header.\n[mcp_servers.placeholder_keep]",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("scrubbed fixture missing required structure %q", required)
		}
	}
	for _, forbidden := range []string{
		"/Users/",
		"/home/",
		"/private/",
		"ballast",
		"solo",
		"artisan-agency",
		"node_repl_token",
		"github.com",
		"localhost",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scrubbed fixture contains forbidden real-looking value %q", forbidden)
		}
	}
}

func TestLookupNameThatSanitizesDifferentlyIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := Scan(testEnv(t.TempDir()), "ZSentinel", []string{"cursor"}); err == nil || !strings.Contains(err.Error(), "exact stored key") {
		t.Fatalf("Scan error = %v, want exact stored key refusal", err)
	}
	if _, err := Uninstall(UninstallOptions{Name: "ZSentinel", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: testEnv(t.TempDir())}); err == nil || !strings.Contains(err.Error(), "exact stored key") {
		t.Fatalf("Uninstall error = %v, want exact stored key refusal", err)
	}
}

func TestInteractiveUninstallPickerReceivesUnreadableAndHonorsSelection(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cursorPath := expectedPath(home, "cursor")
	geminiPath := expectedPath(home, "gemini-cli")
	zedPath := expectedPath(home, "zed")
	writeFile(t, cursorPath, "{\"mcpServers\":{\"x\":{\"url\":\"https://x.test/mcp\",\"headers\":{\"Authorization\":\"Bearer CURSOR\"}}}}\n", 0o600)
	writeFile(t, geminiPath, "{\"mcpServers\":{\"x\":{\"httpUrl\":\"https://x.test/mcp\",\"headers\":{\"Authorization\":\"Bearer GEMINI\"}}}}\n", 0o600)
	writeFile(t, zedPath, "{not-json PICKER_UNREADABLE_SECRET", 0o600)
	pickerCalled := false
	res, err := Uninstall(UninstallOptions{
		Name:   "x",
		Yes:    false,
		NonTTY: false,
		Env:    testEnv(home),
		PickTargets: func(targets []ScanResult, unreadable []ScanResult) ([]ScanResult, error) {
			pickerCalled = true
			if len(targets) != 2 {
				t.Fatalf("selectable targets = %#v, want cursor and gemini only", targets)
			}
			for _, target := range targets {
				if target.Client.ID == "zed" {
					t.Fatalf("unreadable target was selectable: %#v", targets)
				}
			}
			if len(unreadable) != 1 || unreadable[0].Client.ID != "zed" {
				t.Fatalf("unreadable = %#v, want zed", unreadable)
			}
			for _, target := range targets {
				if target.Client.ID == "cursor" {
					return []ScanResult{target}, nil
				}
			}
			t.Fatalf("cursor target missing: %#v", targets)
			return nil, nil
		},
	})
	if !pickerCalled {
		t.Fatalf("picker was not called")
	}
	if err == nil || len(res.Unreadable) != 1 || len(res.Removed) != 1 || len(res.Kept) != 1 {
		t.Fatalf("Uninstall err = %v result = %#v, want one removed, one kept, one unreadable", err, res)
	}
	if strings.Contains(readFile(t, cursorPath), "\"x\"") {
		t.Fatalf("selected cursor entry was not removed")
	}
	if !strings.Contains(readFile(t, geminiPath), "Bearer GEMINI") {
		t.Fatalf("deselected gemini entry was removed")
	}
	if !strings.Contains(readFile(t, zedPath), "PICKER_UNREADABLE_SECRET") {
		t.Fatalf("unreadable zed config changed")
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

func TestProjectInstallRemoteUsesProjectPaths(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	for _, tt := range projectShapeCases(project) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			opts := baseOptions(projectEnv(project), tt.id)
			opts.Project = true
			res, err := InstallRemote(opts)
			if err != nil {
				t.Fatalf("InstallRemote returned error: %v", err)
			}
			if len(res.Written) != 1 || res.Written[0] != tt.path {
				t.Fatalf("written paths = %#v, want %q", res.Written, tt.path)
			}
			if _, err := os.Stat(expectedPath(opts.Env.Home, tt.id)); !os.IsNotExist(err) {
				t.Fatalf("project install wrote global path for %s, stat err = %v", tt.id, err)
			}
		})
	}
}

func TestProjectInstallStdioUsesProjectPaths(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	for _, tt := range projectShapeCases(project) {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			opts := stdioOptions(projectEnv(project), tt.id)
			opts.Project = true
			res, err := InstallStdio(opts)
			if err != nil {
				t.Fatalf("InstallStdio returned error: %v", err)
			}
			if len(res.Written) != 1 || res.Written[0] != tt.path {
				t.Fatalf("written paths = %#v, want %q", res.Written, tt.path)
			}
		})
	}
}

func TestProjectScanAndUninstallUseProjectPaths(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	path := filepath.Join(project, ".cursor", "mcp.json")
	writeFile(t, path, "{\"mcpServers\":{\"renamed\":{\"headers\":{\"Authorization\":\"Bearer PROJECT_SCAN_SECRET\"},\"url\":\"https://x.test/mcp\"}}}\n", 0o600)
	globalPath := expectedPath(projectEnv(project).Home, "cursor")
	writeFile(t, globalPath, "{\"mcpServers\":{\"renamed\":{\"headers\":{\"Authorization\":\"Bearer GLOBAL_SHOULD_STAY\"},\"url\":\"https://x.test/mcp\"}}}\n", 0o600)

	scans, err := ScanScoped(projectEnv(project), "renamed", []string{"cursor"}, true)
	if err != nil {
		t.Fatalf("ScanScoped returned error: %v", err)
	}
	if len(scans) != 1 || scans[0].Path != path || scans[0].Status != ScanHasEntry || !scans[0].HoldsCredential {
		t.Fatalf("project scan = %#v, want project cursor credential", scans)
	}
	res, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Project: true, Env: projectEnv(project)})
	if err != nil || len(res.Removed) != 1 || res.Removed[0].Path != path {
		t.Fatalf("Uninstall err = %v result = %#v, want project removal", err, res)
	}
	if strings.Contains(readFile(t, path), "renamed") {
		t.Fatalf("project entry survived uninstall")
	}
	if !strings.Contains(readFile(t, globalPath), "GLOBAL_SHOULD_STAY") {
		t.Fatalf("global config was changed by project uninstall")
	}
}

func TestProjectInstallUsesGitTopLevelFromSubdirectory(t *testing.T) {
	project := initGitRepo(t)
	subdir := filepath.Join(project, "sub", "deeper")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	env := projectEnv(project)
	env.WorkDir = subdir
	opts := baseOptions(env, "cursor")
	opts.Project = true
	res, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	want := filepath.Join(project, ".cursor", "mcp.json")
	if len(res.Written) != 1 || res.Written[0] != want {
		t.Fatalf("written = %#v, want git top-level path %s", res.Written, want)
	}
	if _, err := os.Stat(filepath.Join(subdir, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("project install wrote cwd subdir path, stat err = %v", err)
	}
}

func TestProjectYesRespectsDetectedClients(t *testing.T) {
	project := initGitRepo(t)
	env := projectEnv(project)
	makeMarker(t, env.Home, "cursor")
	opts := baseOptions(env)
	opts.Project = true
	opts.Yes = true
	res, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	want := filepath.Join(project, ".cursor", "mcp.json")
	if len(res.Written) != 1 || res.Written[0] != want {
		t.Fatalf("written = %#v, want only detected cursor project path", res.Written)
	}
	for _, path := range []string{filepath.Join(project, ".mcp.json"), filepath.Join(project, ".zed", "settings.json"), filepath.Join(project, ".vscode", "mcp.json"), filepath.Join(project, ".gemini", "settings.json"), filepath.Join(project, "opencode.json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("undetected client project path was written: %s", path)
		}
	}
}

func TestProjectInteractiveInstallDoesNotSaveGlobalPreferences(t *testing.T) {
	project := initGitRepo(t)
	env := projectEnv(project)
	makeMarker(t, env.Home, "cursor")
	opts := baseOptions(env)
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = false
	opts.PickTargets = func(targets []Target, preferred map[string]bool) ([]Target, error) {
		if preferred != nil {
			t.Fatalf("project picker preferred = %#v, want nil", preferred)
		}
		return targets, nil
	}
	opts.ConfirmProjectWrite = func(string) (bool, error) { return true, nil }
	if _, err := InstallRemote(opts); err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "hitch", "preferences.json")); !os.IsNotExist(err) {
		t.Fatalf("project interactive install saved global preferences, stat err = %v", err)
	}
}

func TestProjectCodexContract(t *testing.T) {
	project := initGitRepo(t)
	env := projectEnv(project)
	writeFile(t, filepath.Join(env.Home, ".codex", "config.toml"), "[mcp_servers.renamed]\nurl = \"https://x.test/mcp\"\nbearer_token = \"CODEX_GLOBAL_SECRET\"\n", 0o600)
	projectScans, err := ScanScoped(env, "renamed", nil, true)
	if err != nil {
		t.Fatalf("project ScanScoped returned error: %v", err)
	}
	for _, scan := range projectScans {
		if scan.Client.ID == "codex" || strings.Contains(scan.Path, ".codex") {
			t.Fatalf("project scan included Codex: %#v", projectScans)
		}
	}
	_, err = InstallRemote(Options{URL: testURL, Name: "renamed", Headers: map[string]string{"Authorization": "Bearer " + testToken}, Clients: []string{"codex"}, Project: true, Yes: true, NonTTY: true, Env: env})
	if err == nil || !strings.Contains(err.Error(), "no project config path") {
		t.Fatalf("project Codex install error = %v, want no project config path", err)
	}
	globalScans, err := Scan(env, "renamed", []string{"codex"})
	if err != nil {
		t.Fatalf("global Scan returned error: %v", err)
	}
	if len(globalScans) != 1 || globalScans[0].Client.ID != "codex" || globalScans[0].Status != ScanHasEntry {
		t.Fatalf("global Codex scan = %#v, want scannable global Codex", globalScans)
	}
}

func TestProjectClientWithoutProjectPathGetsSpecificError(t *testing.T) {
	project := initGitRepo(t)
	_, err := InstallRemote(Options{URL: testURL, Name: "renamed", Headers: map[string]string{"Authorization": "Bearer " + testToken}, Clients: []string{"windsurf"}, Project: true, Yes: true, NonTTY: true, Env: projectEnv(project)})
	if err == nil || !strings.Contains(err.Error(), "no project config path") || strings.Contains(err.Error(), "unknown") {
		t.Fatalf("Windsurf project error = %v, want known client with no project config path", err)
	}
}

func TestGlobalNonTTYInstallRefusalIsDistinctFromProjectGitignoreRefusal(t *testing.T) {
	env := testEnv(t.TempDir())
	makeMarker(t, env.Home, "cursor")
	_, err := InstallRemote(Options{URL: testURL, Name: "renamed", Headers: map[string]string{"Authorization": "Bearer " + testToken}, Yes: false, NonTTY: true, Env: env})
	if err == nil || !strings.Contains(err.Error(), "requires either -y/--yes or -c/--client") || strings.Contains(err.Error(), "gitignored") {
		t.Fatalf("global non-TTY error = %v, want general picker refusal", err)
	}
}

func TestGitIgnoredProjectCredentialWriteDoesNotRequireConfirmation(t *testing.T) {
	project := initGitRepo(t)
	writeFile(t, filepath.Join(project, ".gitignore"), ".cursor/mcp.json\n", 0o600)
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	opts.ConfirmProjectWrite = func(string) (bool, error) {
		t.Fatalf("ignored path should not request confirmation")
		return false, nil
	}
	res, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if len(res.Written) != 1 || res.Written[0] != filepath.Join(project, ".cursor", "mcp.json") {
		t.Fatalf("written = %#v, want ignored project cursor path", res.Written)
	}
}

func TestUnignoredProjectCredentialWriteWarnsRefusesAndLeavesTargetAbsent(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	oldIgnored := projectPathIgnored
	projectPathIgnored = func(harness.Env, string) (bool, error) { return false, nil }
	t.Cleanup(func() { projectPathIgnored = oldIgnored })
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	opts.ConfirmProjectWrite = func(string) (bool, error) {
		t.Fatalf("non-TTY guard must not prompt")
		return false, nil
	}
	opts.Stdout = &out
	res, err := InstallRemote(opts)
	if err == nil || len(res.Failures) != 1 {
		t.Fatalf("InstallRemote err = %v failures = %#v, want refusal", err, res.Failures)
	}
	if !strings.Contains(err.Error(), "write credentials anyway") {
		t.Fatalf("InstallRemote error = %v, want gitignore-specific non-TTY refusal", err)
	}
	if !strings.Contains(out.String(), "WARNING") || !strings.Contains(out.String(), path) || strings.Contains(out.String(), testToken) {
		t.Fatalf("warning output = %q, want path-only warning", out.String())
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("refused project write created target, stat err = %v", statErr)
	}
}

func TestConfirmedProjectCredentialWriteSucceedsWithModeAndContent(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = false
	opts.Stdout = &out
	opts.ConfirmProjectWrite = func(got string) (bool, error) {
		if got != path {
			t.Fatalf("confirm path = %q, want %q", got, path)
		}
		return true, nil
	}
	res, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if len(res.Written) != 1 || res.Written[0] != path {
		t.Fatalf("written = %#v, want %s", res.Written, path)
	}
	assertMode0600(t, path)
	if got := cursorServerAt(t, path)["headers"].(map[string]any)["Authorization"]; got != "Bearer "+testToken {
		t.Fatalf("Authorization = %q, want token persisted in selected file", got)
	}
	if !strings.Contains(out.String(), "WARNING") || strings.Contains(out.String(), testToken) {
		t.Fatalf("confirmation warning output = %q", out.String())
	}
}

func TestProjectGuardChecksResolvedSymlinkTargetAndSummaryUsesTarget(t *testing.T) {
	project := initGitRepo(t)
	shared := filepath.Join(project, "shared", "mcp.json")
	link := filepath.Join(project, ".cursor", "mcp.json")
	writeFile(t, filepath.Join(project, ".gitignore"), ".cursor/mcp.json\n", 0o600)
	writeFile(t, shared, "{\"mcpServers\":{}}\n", 0o600)
	runGit(t, project, "add", "shared/mcp.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "shared", "mcp.json"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = true
	opts.Stdout = &out
	res, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote returned error: %v", err)
	}
	if len(res.Written) != 1 || res.Written[0] != shared {
		t.Fatalf("written = %#v, want resolved target %s", res.Written, shared)
	}
	if !strings.Contains(out.String(), "WARNING") || !strings.Contains(out.String(), shared) || strings.Contains(out.String(), link+")") || strings.Contains(out.String(), testToken) {
		t.Fatalf("output = %q, want warning/summary for resolved target without token", out.String())
	}
	if got := cursorServerAt(t, shared)["headers"].(map[string]any)["Authorization"]; got != "Bearer "+testToken {
		t.Fatalf("shared target Authorization = %q", got)
	}
}

func TestProjectGuardIgnoresGITIndexFileOverride(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	before := "{\"mcpServers\":{}}\n"
	writeFile(t, filepath.Join(project, ".gitignore"), ".cursor/mcp.json\n", 0o600)
	writeFile(t, path, before, 0o600)
	runGit(t, project, "add", "-f", ".cursor/mcp.json")
	t.Setenv("GIT_INDEX_FILE", filepath.Join(project, "missing-index"))
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	opts.Stdout = &out
	_, err := InstallRemote(opts)
	if err == nil || !strings.Contains(err.Error(), "write credentials anyway") {
		t.Fatalf("InstallRemote error = %v, want gitignore refusal", err)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("tracked target changed under GIT_INDEX_FILE override to %q", got)
	}
}

func TestProjectGuardIgnoresDecoyGitDirAndWorkTree(t *testing.T) {
	project := initGitRepo(t)
	decoy := initGitRepo(t)
	writeFile(t, filepath.Join(decoy, ".git", "info", "exclude"), ".cursor/mcp.json\n", 0o600)
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	path := filepath.Join(project, ".cursor", "mcp.json")
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	opts.Stdout = &out
	_, err := InstallRemote(opts)
	if err == nil || !strings.Contains(err.Error(), "write credentials anyway") {
		t.Fatalf("InstallRemote error = %v, want real repo gitignore refusal", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("decoy git env allowed write, stat err = %v", statErr)
	}
}

func TestProjectGuardFailsClosedWhenGitCannotRun(t *testing.T) {
	project := t.TempDir()
	t.Setenv("PATH", filepath.Join(project, "empty-bin"))
	if err := os.MkdirAll(filepath.Join(project, "empty-bin"), 0o700); err != nil {
		t.Fatalf("mkdir empty bin: %v", err)
	}
	path := filepath.Join(project, ".cursor", "mcp.json")
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	opts.Stdout = &out
	_, err := InstallRemote(opts)
	if err == nil || !strings.Contains(err.Error(), "write credentials anyway") {
		t.Fatalf("InstallRemote error = %v, want fail-closed gitignore refusal", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("git failure allowed write, stat err = %v", statErr)
	}
}

func TestProjectDryRunReportsGitignoreWarningAndWritesNothing(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	var out strings.Builder
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.DryRun = true
	opts.Yes = false
	opts.NonTTY = true
	opts.Stdout = &out
	res, err := InstallRemote(opts)
	if err != nil {
		t.Fatalf("InstallRemote dry-run returned error: %v", err)
	}
	if len(res.WouldWrite) != 1 || res.WouldWrite[0] != path {
		t.Fatalf("wouldWrite = %#v, want %s", res.WouldWrite, path)
	}
	if !strings.Contains(out.String(), "WARNING") || !strings.Contains(out.String(), "Would write Cursor to "+path) || strings.Contains(out.String(), testToken) {
		t.Fatalf("dry-run output = %q", out.String())
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run wrote target, stat err = %v", statErr)
	}
}

func TestUnignoredProjectStdioCredentialWriteWarnsRefusesAndLeavesTargetAbsent(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	oldIgnored := projectPathIgnored
	projectPathIgnored = func(harness.Env, string) (bool, error) { return false, nil }
	t.Cleanup(func() { projectPathIgnored = oldIgnored })
	var out strings.Builder
	opts := stdioOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	opts.Stdout = &out
	res, err := InstallStdio(opts)
	if err == nil || len(res.Failures) != 1 {
		t.Fatalf("InstallStdio err = %v failures = %#v, want refusal", err, res.Failures)
	}
	if !strings.Contains(out.String(), "WARNING") || !strings.Contains(out.String(), path) || strings.Contains(out.String(), "stdio-secret-value") {
		t.Fatalf("warning output = %q, want path-only warning", out.String())
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("refused project stdio write created target, stat err = %v", statErr)
	}
}

func TestProjectStdioWithoutCredentialSkipsGitignoreGuard(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	oldIgnored := projectPathIgnored
	projectPathIgnored = func(harness.Env, string) (bool, error) {
		t.Fatalf("stdio without env credentials should not check gitignore")
		return false, nil
	}
	t.Cleanup(func() { projectPathIgnored = oldIgnored })
	opts := stdioOptions(projectEnv(project), "cursor")
	opts.StdioEnv = nil
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = true
	res, err := InstallStdio(opts)
	if err != nil {
		t.Fatalf("InstallStdio returned error: %v", err)
	}
	if len(res.Written) != 1 || res.Written[0] != path {
		t.Fatalf("written = %#v, want %s", res.Written, path)
	}
}

func TestDeclinedProjectCredentialWriteLeavesExistingTargetByteIdentical(t *testing.T) {
	project := initGitRepo(t)
	path := filepath.Join(project, ".cursor", "mcp.json")
	oldIgnored := projectPathIgnored
	projectPathIgnored = func(harness.Env, string) (bool, error) { return false, nil }
	t.Cleanup(func() { projectPathIgnored = oldIgnored })
	before := "{\n  \"mcpServers\": {}\n}\n"
	writeFile(t, path, before, 0o600)
	opts := baseOptions(projectEnv(project), "cursor")
	opts.Project = true
	opts.Yes = false
	opts.NonTTY = false
	opts.ConfirmProjectWrite = func(got string) (bool, error) {
		if got != path {
			t.Fatalf("confirm path = %q, want %q", got, path)
		}
		return false, nil
	}
	_, err := InstallRemote(opts)
	if err == nil {
		t.Fatalf("InstallRemote returned nil error, want declined write")
	}
	if got := readFile(t, path); !bytes.Equal([]byte(got), []byte(before)) {
		t.Fatalf("declined write changed bytes\nbefore:%q\nafter:%q", before, got)
	}
}

func TestGitCheckIgnoredCoversGitIgnoreBoundary(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, project string) string
		want  bool
	}{
		{name: "root gitignore", setup: func(t *testing.T, project string) string {
			writeFile(t, filepath.Join(project, ".gitignore"), ".cursor/mcp.json\n", 0o600)
			return filepath.Join(project, ".cursor", "mcp.json")
		}, want: true},
		{name: "nested gitignore", setup: func(t *testing.T, project string) string {
			writeFile(t, filepath.Join(project, ".cursor", ".gitignore"), "mcp.json\n", 0o600)
			return filepath.Join(project, ".cursor", "mcp.json")
		}, want: true},
		{name: "negated pattern is not ignored", setup: func(t *testing.T, project string) string {
			writeFile(t, filepath.Join(project, ".gitignore"), "*\n!.cursor/\n!.cursor/mcp.json\n", 0o600)
			return filepath.Join(project, ".cursor", "mcp.json")
		}},
		{name: "info exclude", setup: func(t *testing.T, project string) string {
			writeFile(t, filepath.Join(project, ".git", "info", "exclude"), ".vscode/mcp.json\n", 0o600)
			return filepath.Join(project, ".vscode", "mcp.json")
		}, want: true},
		{name: "global excludes file", setup: func(t *testing.T, project string) string {
			globalIgnore := filepath.Join(project, "global-ignore")
			writeFile(t, globalIgnore, "opencode.json\n", 0o600)
			runGit(t, project, "config", "core.excludesFile", globalIgnore)
			return filepath.Join(project, "opencode.json")
		}, want: true},
		{name: "not ignored", setup: func(t *testing.T, project string) string {
			return filepath.Join(project, ".gemini", "settings.json")
		}},
		{name: "tracked beats ignored", setup: func(t *testing.T, project string) string {
			path := filepath.Join(project, ".cursor", "mcp.json")
			writeFile(t, filepath.Join(project, ".gitignore"), ".cursor/mcp.json\n", 0o600)
			writeFile(t, path, "{}\n", 0o600)
			runGit(t, project, "add", "-f", ".cursor/mcp.json")
			return path
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := initGitRepo(t)
			pinGitEnv(t, project)
			path := tt.setup(t, project)
			got, err := gitCheckIgnored(projectEnv(project), path)
			if err != nil {
				t.Fatalf("gitCheckIgnored returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("gitCheckIgnored(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestGitCheckIgnoredTreatsNonGitRepositoryAsNotIgnored(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	ignored, err := gitCheckIgnored(projectEnv(project), filepath.Join(project, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatalf("gitCheckIgnored returned error: %v", err)
	}
	if ignored {
		t.Fatalf("non-git directory reported ignored")
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

func projectEnv(project string) harness.Env {
	env := testEnv(filepath.Join(project, "home"))
	env.WorkDir = project
	return env
}

func projectShapeCases(project string) []shapeCase {
	return []shapeCase{
		{id: "claude-code", key: "mcpServers", path: filepath.Join(project, ".mcp.json")},
		{id: "cursor", key: "mcpServers", path: filepath.Join(project, ".cursor", "mcp.json")},
		{id: "zed", key: "context_servers", path: filepath.Join(project, ".zed", "settings.json")},
		{id: "vscode", key: "servers", path: filepath.Join(project, ".vscode", "mcp.json")},
		{id: "gemini-cli", key: "mcpServers", path: filepath.Join(project, ".gemini", "settings.json")},
		{id: "opencode", key: "mcp", path: filepath.Join(project, "opencode.json")},
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	runGit(t, project, "init", "-q")
	root, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatalf("resolve temp git repo %q: %v", project, err)
	}
	return root
}

func runGit(t *testing.T, project string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = project
	cmd.Env = gitTestEnv(project)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitTestEnv(project string) []string {
	path := os.Getenv("PATH")
	return []string{
		"PATH=" + path,
		"HOME=" + filepath.Join(project, "git-home"),
		"GIT_CONFIG_GLOBAL=" + filepath.Join(project, "git-global-config"),
		"GIT_CONFIG_SYSTEM=" + filepath.Join(project, "git-system-config"),
	}
}

func pinGitEnv(t *testing.T, project string) {
	t.Helper()
	t.Setenv("HOME", filepath.Join(project, "git-home"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(project, "git-global-config"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(project, "git-system-config"))
	t.Setenv("GIT_INDEX_FILE", "")
	t.Setenv("GIT_DIR", "")
	t.Setenv("GIT_WORK_TREE", "")
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

func cursorServerAt(t *testing.T, path string) map[string]any {
	t.Helper()
	data := readJSON(t, path)
	servers := data["mcpServers"].(map[string]any)
	server := servers["renamed"].(map[string]any)
	return server
}

func jsonObjectValueKeyOrder(t *testing.T, raw []byte, key string) []string {
	t.Helper()
	span, found, err := findTopLevelObjectValue(raw, key)
	if err != nil {
		t.Fatalf("find value %q: %v", key, err)
	}
	if !found {
		t.Fatalf("value %q not found in %s", key, raw)
	}
	return jsonObjectKeyOrder(t, raw[span.start:span.end])
}

func jsonObjectKeyOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("read object start from %s: %v", raw, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("raw is not JSON object: %s", raw)
	}
	keys := []string{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("read object key from %s: %v", raw, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			t.Fatalf("object key is %T, want string", keyTok)
		}
		keys = append(keys, key)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("skip value for %q from %s: %v", key, raw, err)
		}
	}
	return keys
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
