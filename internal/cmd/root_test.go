package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artisan-build/hitch/internal/harness"
)

func TestVersionCommand(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	Version, Commit, Date = "v9.9.9", "abc123", "2026-08-09"
	t.Cleanup(func() { Version, Commit, Date = oldVersion, oldCommit, oldDate })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	if err := ExecuteForTest(root, "version"); err != nil {
		t.Fatalf("version command returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"v9.9.9", "abc123", "2026-08-09"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output %q does not contain %q", got, want)
		}
	}
}

func TestHelpListsOnlyPR1Commands(t *testing.T) {
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	if err := ExecuteForTest(root, "--help"); err != nil {
		t.Fatalf("help command returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"list", "version"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "completion") {
		t.Fatalf("help output includes Cobra completion command: %q", got)
	}
}

type fileSnapshot map[string]fileState

type fileState struct {
	Size    int64
	ModTime time.Time
	Mode    os.FileMode
}

func TestListWritesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{}", 0o600)
	before := snapshotTree(t, home)

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"list"})
	root.Commands()[1] = newListCommand(func() harness.Env { return env })
	if err := root.Execute(); err != nil {
		t.Fatalf("list command returned error: %v", err)
	}
	after := snapshotTree(t, home)
	if len(before) != len(after) {
		t.Fatalf("snapshot length changed: before %d after %d", len(before), len(after))
	}
	for path, beforeState := range before {
		afterState, ok := after[path]
		if !ok {
			t.Fatalf("path %q missing after list", path)
		}
		if afterState != beforeState {
			t.Fatalf("path %q changed: before %#v after %#v", path, beforeState, afterState)
		}
	}
}

func snapshotTree(t *testing.T, root string) fileSnapshot {
	t.Helper()
	snapshot := fileSnapshot{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[rel] = fileState{Size: info.Size(), ModTime: info.ModTime(), Mode: info.Mode()}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot tree: %v", err)
	}
	return snapshot
}

func testEnv(home string) harness.Env {
	return harness.Env{
		Home:          home,
		XDGConfigHome: filepath.Join(home, ".config"),
		AppData:       filepath.Join(home, "AppData", "Roaming"),
		GOOS:          "darwin",
	}
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
