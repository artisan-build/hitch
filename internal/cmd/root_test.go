package cmd

import (
	"bytes"
	"errors"
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

	root := NewRootCommand(func() (harness.Env, error) { return harness.Env{}, nil })
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
	root := NewRootCommand(func() (harness.Env, error) { return harness.Env{}, nil })
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
	for _, hidden := range []string{"completion", "help        Help"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("help output includes hidden command %q: %q", hidden, got)
		}
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

	root := NewRootCommand(func() (harness.Env, error) { return env, nil })
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list command returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Cursor", filepath.Join(home, ".cursor", "mcp.json"), "detected", "Claude Desktop", "prompt-tier - use 'hitch prompt'"} {
		if !strings.Contains(got, want) {
			t.Fatalf("list output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, filepath.Join(home, "Library", "Application Support", "JetBrains")) {
		t.Fatalf("list output printed prompt-tier marker path: %q", got)
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

func TestUnknownCommandReturnsError(t *testing.T) {
	root := NewRootCommand(func() (harness.Env, error) { return harness.Env{}, nil })
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := ExecuteForTest(root, "bogus"); err == nil {
		t.Fatalf("unknown command returned nil error")
	}
}

func TestMainUnknownCommandExitsNonZeroWithStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"bogus"}, &stdout, &stderr, func() (harness.Env, error) { return harness.Env{}, nil })
	if code == 0 {
		t.Fatalf("Main exit code = 0, want non-zero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Fatalf("stderr is empty")
	}
}

func TestListMissingHomeErrorsLoudly(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main([]string{"list"}, &stdout, &stderr, func() (harness.Env, error) {
		return harness.Env{}, errors.New("could not resolve user home from HOME or USERPROFILE")
	})
	if code == 0 {
		t.Fatalf("Main exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "HOME") || !strings.Contains(stderr.String(), "USERPROFILE") {
		t.Fatalf("stderr = %q, want HOME and USERPROFILE", stderr.String())
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
