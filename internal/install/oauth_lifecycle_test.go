package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artisan-build/hitch/internal/harness"
	"github.com/artisan-build/hitch/internal/oauthregistry"
)

func oauthOptions(home string, clients ...string) Options {
	opts := baseOptions(testEnv(home), clients...)
	opts.OAuth = true
	opts.Headers = map[string]string{}
	return opts
}

func TestOAuthInstallRegistryDrivesOnlyExactSafeScanClassification(t *testing.T) {
	home := t.TempDir()
	opts := oauthOptions(home, "cursor")
	opts.URL = "https://USERINFO_SENTINEL@example.test/mcp?QUERY_SENTINEL=yes#FRAGMENT_SENTINEL"
	if _, err := InstallRemote(opts); err != nil {
		t.Fatal(err)
	}
	records, err := oauthregistry.Load(opts.Env)
	if err != nil || len(records) != 1 {
		t.Fatalf("registry = %#v, %v", records, err)
	}
	if records[0].ServerURL != "https://example.test/mcp" {
		t.Fatalf("stored URL = %q", records[0].ServerURL)
	}
	registryRaw, _ := os.ReadFile(filepath.Join(home, ".config", "hitch", "oauth-installations.json"))
	for _, secret := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "FRAGMENT_SENTINEL"} {
		if strings.Contains(string(registryRaw), secret) {
			t.Fatalf("registry leaked %s: %s", secret, registryRaw)
		}
	}

	results, err := Scan(opts.Env, "renamed", []string{"cursor"})
	if err != nil || len(results) != 1 || results[0].Auth != AuthOAuth || results[0].HoldsCredential {
		t.Fatalf("OAuth scan = %#v, %v", results, err)
	}

	path := expectedPath(home, "cursor")
	writeFile(t, path, `{"mcpServers":{"renamed":{"url":"https://other.example.test/mcp","headers":{}}}}`, 0o600)
	results, err = Scan(opts.Env, "renamed", []string{"cursor"})
	if err != nil || results[0].Auth == AuthOAuth {
		t.Fatalf("stale scan = %#v, %v", results, err)
	}

	writeFile(t, path, `{"mcpServers":{"renamed":{"url":"https://example.test/mcp","headers":{"Authorization":"Bearer SECRET_SENTINEL"}}}}`, 0o600)
	results, err = Scan(opts.Env, "renamed", []string{"cursor"})
	if err != nil || results[0].Auth != AuthCredential || !results[0].HoldsCredential {
		t.Fatalf("bearer scan = %#v, %v", results, err)
	}
}

func TestOAuthRegistryRecordsOnlySuccessfulWritesAndNeverDryRun(t *testing.T) {
	home := t.TempDir()
	zedPath := expectedPath(home, "zed")
	writeFile(t, zedPath, `{malformed`, 0o600)
	opts := oauthOptions(home, "cursor", "zed")
	res, err := InstallRemote(opts)
	if !errors.Is(err, ErrPartialFailure) || len(res.Written) != 1 {
		t.Fatalf("InstallRemote = %#v, %v", res, err)
	}
	records, loadErr := oauthregistry.Load(opts.Env)
	if loadErr != nil || len(records) != 1 || records[0].ClientID != "cursor" {
		t.Fatalf("partial registry = %#v, %v", records, loadErr)
	}

	dryHome := t.TempDir()
	dry := oauthOptions(dryHome, "cursor")
	dry.DryRun = true
	if _, err := InstallRemote(dry); err != nil {
		t.Fatal(err)
	}
	path, _ := oauthregistry.Path(dry.Env)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote registry: %v", err)
	}
}

func TestOAuthMalformedRegistryIsVisibleAndUnchangedAfterConfigWrite(t *testing.T) {
	home := t.TempDir()
	opts := oauthOptions(home, "cursor")
	path, _ := oauthregistry.Path(opts.Env)
	malformed := []byte(`{"version":1,"installations":[`)
	writeFile(t, path, string(malformed), 0o600)
	res, err := InstallRemote(opts)
	if err == nil || len(res.Written) != 1 || !strings.Contains(err.Error(), "could not be persisted") {
		t.Fatalf("InstallRemote = %#v, %v", res, err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(malformed) {
		t.Fatalf("malformed registry changed: %q", got)
	}
}

func TestOAuthUninstallRemovesMatchingMetadataAndSurfacesCleanupFailure(t *testing.T) {
	home := t.TempDir()
	opts := oauthOptions(home, "cursor")
	if _, err := InstallRemote(opts); err != nil {
		t.Fatal(err)
	}
	result, err := Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: opts.Env})
	if err != nil || len(result.Removed) != 1 || result.Removed[0].Auth != AuthOAuth {
		t.Fatalf("Uninstall = %#v, %v", result, err)
	}
	records, loadErr := oauthregistry.Load(opts.Env)
	if loadErr != nil || len(records) != 0 {
		t.Fatalf("registry after uninstall = %#v, %v", records, loadErr)
	}

	if _, err := InstallRemote(opts); err != nil {
		t.Fatal(err)
	}
	oldRemove := removeOAuthRegistry
	removeOAuthRegistry = func(_ harness.Env, _ []oauthregistry.Record) error { return errors.New("CLEANUP_SENTINEL") }
	t.Cleanup(func() { removeOAuthRegistry = oldRemove })
	result, err = Uninstall(UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: opts.Env})
	if err == nil || !strings.Contains(err.Error(), "CLEANUP_SENTINEL") || len(result.Removed) != 1 {
		t.Fatalf("cleanup failure Uninstall = %#v, %v", result, err)
	}
	if _, statErr := os.Stat(expectedPath(home, "cursor")); statErr != nil {
		t.Fatalf("config itself should remain after empty removal, stat = %v", statErr)
	}
	records, loadErr = oauthregistry.Load(opts.Env)
	if loadErr != nil || len(records) != 1 {
		t.Fatalf("failed cleanup removed metadata: %#v, %v", records, loadErr)
	}
}

func TestOAuthRegistryIdentityResolvesSymlinkedConfigPath(t *testing.T) {
	home := t.TempDir()
	linkPath := expectedPath(home, "cursor")
	targetPath := filepath.Join(home, "real", "cursor.json")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, targetPath, `{}`, 0o600)
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatal(err)
	}
	opts := oauthOptions(home, "cursor")
	if _, err := InstallRemote(opts); err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	records, err := oauthregistry.Load(opts.Env)
	if err != nil || len(records) != 1 || records[0].ConfigPath != resolvedTarget {
		t.Fatalf("symlink registry = %#v, %v", records, err)
	}
	results, err := Scan(opts.Env, "renamed", []string{"cursor"})
	if err != nil || len(results) != 1 || results[0].Auth != AuthOAuth {
		t.Fatalf("symlink scan = %#v, %v", results, err)
	}
}

func TestOAuthUninstallDoesNotDropMetadataForKeptUnreadableOrAbsentTargets(t *testing.T) {
	for _, tt := range []struct {
		name    string
		prepare func(t *testing.T, path string)
		opts    func(env harness.Env) UninstallOptions
	}{
		{name: "kept", opts: func(env harness.Env) UninstallOptions {
			return UninstallOptions{Name: "renamed", Env: env, PickTargets: func(_ []ScanResult, _ []ScanResult) ([]ScanResult, error) { return nil, nil }}
		}},
		{name: "unreadable", prepare: func(t *testing.T, path string) { writeFile(t, path, `{malformed`, 0o600) }, opts: func(env harness.Env) UninstallOptions {
			return UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: env}
		}},
		{name: "not present", prepare: func(t *testing.T, path string) { writeFile(t, path, `{"mcpServers":{}}`, 0o600) }, opts: func(env harness.Env) UninstallOptions {
			return UninstallOptions{Name: "renamed", Clients: []string{"cursor"}, Yes: true, NonTTY: true, Env: env}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			installOpts := oauthOptions(home, "cursor")
			if _, err := InstallRemote(installOpts); err != nil {
				t.Fatal(err)
			}
			if tt.prepare != nil {
				tt.prepare(t, expectedPath(home, "cursor"))
			}
			_, _ = Uninstall(tt.opts(installOpts.Env))
			records, err := oauthregistry.Load(installOpts.Env)
			if err != nil || len(records) != 1 {
				t.Fatalf("metadata changed for %s target: %#v, %v", tt.name, records, err)
			}
		})
	}
}
