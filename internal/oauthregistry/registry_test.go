package oauthregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artisan-build/hitch/internal/harness"
)

func TestRegistryFreshModeIdempotenceAndScopeSeparation(t *testing.T) {
	home := t.TempDir()
	env := harness.Env{Home: home, XDGConfigHome: filepath.Join(home, "xdg")}
	global := Record{Scope: ScopeGlobal, ClientID: "cursor", ConfigPath: filepath.Join(home, ".cursor", "mcp.json"), ServerName: "example", ServerURL: "https://example.test/mcp"}
	project := global
	project.Scope = ScopeProject
	project.ConfigPath = filepath.Join(home, "project", ".cursor", "mcp.json")

	if err := Upsert(env, []Record{global, project}); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	path, _ := Path(env)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("registry mode = %o, want 600", info.Mode().Perm())
	}
	before, _ := os.ReadFile(path)
	if err := Upsert(env, []Record{project, global}); err != nil {
		t.Fatalf("idempotent Upsert returned error: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("idempotent update changed registry:\nbefore=%s\nafter=%s", before, after)
	}
	records, err := Load(env)
	if err != nil || len(records) != 2 || !Matches(records, global) || !Matches(records, project) {
		t.Fatalf("Load = %#v, %v", records, err)
	}
}

func TestRegistryMalformedAndUnreadableAreNeverOverwritten(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, path string) []byte
	}{
		{name: "malformed", setup: func(t *testing.T, path string) []byte {
			t.Helper()
			raw := []byte(`{"version":1,"installations":[`)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			return raw
		}},
		{name: "unreadable shape", setup: func(t *testing.T, path string) []byte {
			t.Helper()
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return nil
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			env := harness.Env{Home: home}
			path, _ := Path(env)
			before := tt.setup(t, path)
			record := Record{Scope: ScopeGlobal, ClientID: "cursor", ConfigPath: filepath.Join(home, "mcp.json"), ServerName: "example", ServerURL: "https://example.test/mcp"}
			if err := Upsert(env, []Record{record}); err == nil {
				t.Fatal("Upsert returned nil error")
			}
			if before != nil {
				after, err := os.ReadFile(path)
				if err != nil || string(after) != string(before) {
					t.Fatalf("registry changed: %q, %v", after, err)
				}
			} else if info, err := os.Stat(path); err != nil || !info.IsDir() {
				t.Fatalf("unreadable registry shape changed: %v, %v", info, err)
			}
		})
	}
}

func TestSanitizeURLDropsAllPotentialTokenMaterial(t *testing.T) {
	got, err := SanitizeURL("https://USERINFO_SENTINEL@example.test/mcp?ordinary=QUERY_SENTINEL&token=TOKEN_SENTINEL#FRAGMENT_SENTINEL")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.test/mcp" {
		t.Fatalf("SanitizeURL = %q", got)
	}
	for _, secret := range []string{"USERINFO_SENTINEL", "QUERY_SENTINEL", "TOKEN_SENTINEL", "FRAGMENT_SENTINEL"} {
		if strings.Contains(got, secret) {
			t.Fatalf("SanitizeURL leaked %s", secret)
		}
	}
}

func TestRemoveIdentitiesIgnoresURLButPreservesOtherScopes(t *testing.T) {
	home := t.TempDir()
	env := harness.Env{Home: home}
	global := Record{Scope: ScopeGlobal, ClientID: "cursor", ConfigPath: filepath.Join(home, "mcp.json"), ServerName: "example", ServerURL: "https://old.example.test/mcp"}
	project := global
	project.Scope = ScopeProject
	if err := Upsert(env, []Record{global, project}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveIdentities(env, []Identity{{Scope: ScopeGlobal, ClientID: global.ClientID, ConfigPath: global.ConfigPath, ServerName: global.ServerName}}); err != nil {
		t.Fatal(err)
	}
	records, err := Load(env)
	if err != nil || len(records) != 1 || records[0] != project {
		t.Fatalf("records = %#v, %v; want project record only", records, err)
	}
}
