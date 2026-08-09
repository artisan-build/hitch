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
	if err := executeForTest(root, "version"); err != nil {
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
	if err := executeForTest(root, "--help"); err != nil {
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

func TestHelpCommandMatchesHelpFlagAndStaysHidden(t *testing.T) {
	var flagOut bytes.Buffer
	var flagErr bytes.Buffer
	if code := Main([]string{"--help"}, &flagOut, &flagErr, func() (harness.Env, error) { return harness.Env{}, nil }); code != 0 {
		t.Fatalf("--help exit code = %d, stderr = %q", code, flagErr.String())
	}

	var commandOut bytes.Buffer
	var commandErr bytes.Buffer
	if code := Main([]string{"help"}, &commandOut, &commandErr, func() (harness.Env, error) { return harness.Env{}, nil }); code != 0 {
		t.Fatalf("help exit code = %d, stderr = %q", code, commandErr.String())
	}

	if commandOut.String() != flagOut.String() {
		t.Fatalf("hitch help output differs from --help\nhelp: %q\n--help: %q", commandOut.String(), flagOut.String())
	}
	if strings.Contains(flagOut.String(), "help        Help") {
		t.Fatalf("--help lists hidden help command: %q", flagOut.String())
	}
}

type fileSnapshot map[string]fileState

type fileState struct {
	Size    int64
	ModTime time.Time
	Mode    os.FileMode
}

func TestListWritesNothing(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir %q: %v", cwd, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCWD); err != nil {
			t.Fatalf("restore cwd %q: %v", oldCWD, err)
		}
	})
	env := testEnv(home)
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{}", 0o600)
	mkdirAll(t, filepath.Join(home, "Library", "Application Support", "Claude"))
	before := snapshotTree(t, home)
	beforeCWD := snapshotTree(t, cwd)

	root := NewRootCommand(func() (harness.Env, error) { return env, nil })
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list command returned error: %v", err)
	}
	got := out.String()
	wantLines := []string{
		"Claude Code\t" + filepath.Join(home, ".claude.json") + "\tnot detected",
		"Cursor\t" + filepath.Join(home, ".cursor", "mcp.json") + "\tdetected",
		"Codex\t" + filepath.Join(home, ".codex", "config.toml") + "\tnot detected",
		"Windsurf\t" + filepath.Join(home, ".codeium", "windsurf", "mcp_config.json") + "\tnot detected",
		"Zed\t" + filepath.Join(home, ".config", "zed", "settings.json") + "\tnot detected",
		"VS Code\t" + filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json") + "\tnot detected",
		"Gemini CLI\t" + filepath.Join(home, ".gemini", "settings.json") + "\tnot detected",
		"opencode\t" + filepath.Join(home, ".config", "opencode", "opencode.json") + "\tnot detected",
		"Claude Desktop\t-\tdetected (prompt-tier - hitch does not write this client's config)",
		"JetBrains\t-\tnot detected (prompt-tier - hitch does not write this client's config)",
	}
	gotLines := strings.Split(strings.TrimSpace(got), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("list output line count = %d, want %d\noutput:\n%s", len(gotLines), len(wantLines), got)
	}
	for i, want := range wantLines {
		if gotLines[i] != want {
			t.Fatalf("list line %d = %q, want %q\nfull output:\n%s", i+1, gotLines[i], want, got)
		}
	}
	if strings.Contains(got, filepath.Join(home, "Library", "Application Support", "JetBrains")) {
		t.Fatalf("list output printed prompt-tier marker path: %q", got)
	}
	after := snapshotTree(t, home)
	afterCWD := snapshotTree(t, cwd)
	assertSnapshotsEqual(t, "HOME", before, after)
	assertSnapshotsEqual(t, "cwd", beforeCWD, afterCWD)
}

func assertSnapshotsEqual(t *testing.T, name string, before fileSnapshot, after fileSnapshot) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("%s snapshot length changed: before %d after %d", name, len(before), len(after))
	}
	for path, beforeState := range before {
		afterState, ok := after[path]
		if !ok {
			t.Fatalf("%s path %q missing after list", name, path)
		}
		if afterState != beforeState {
			t.Fatalf("%s path %q changed: before %#v after %#v", name, path, beforeState, afterState)
		}
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	root := NewRootCommand(func() (harness.Env, error) { return harness.Env{}, nil })
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := executeForTest(root, "bogus"); err == nil {
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
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
}
