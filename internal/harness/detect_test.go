package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeCodeDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, home string) Env
		want  bool
		path  func(home string) string
	}{
		{
			name: "empty temp HOME",
			setup: func(_ *testing.T, home string) Env {
				return testEnv(home)
			},
		},
		{
			name: "only .claude.json exists",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				writeFile(t, filepath.Join(home, ".claude.json"), "{}", 0o600)
				return testEnv(home)
			},
			want: true,
		},
		{
			name: "only .claude dir exists",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				mkdir(t, filepath.Join(home, ".claude"))
				return testEnv(home)
			},
			want: true,
		},
		{
			name: "CLAUDE_CONFIG_DIR set to existing dir",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				configDir := filepath.Join(home, "claude-config")
				mkdir(t, configDir)
				env := testEnv(home)
				env.ClaudeConfigDir = configDir
				return env
			},
			want: true,
			path: func(home string) string {
				return filepath.Join(home, "claude-config", ".claude.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := tt.setup(t, home)
			result := detectByID(t, env, "claude-code")
			if result.Detected != tt.want {
				t.Fatalf("Claude Code detected = %v, want %v", result.Detected, tt.want)
			}
			if tt.path != nil && result.ConfigPath != tt.path(home) {
				t.Fatalf("Claude Code config path = %q, want %q", result.ConfigPath, tt.path(home))
			}
		})
	}
}

func TestDetectAllClients(t *testing.T) {
	t.Parallel()

	for _, marker := range fileWriterMarkers() {
		marker := marker
		t.Run(marker.client.ID, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			marker.create(t, env)

			results, err := Detect(env)
			if err != nil {
				t.Fatalf("Detect returned error: %v", err)
			}

			for _, result := range results {
				if result.PromptTier {
					continue
				}
				want := result.ID == marker.client.ID
				if result.Detected != want {
					t.Fatalf("%s detected = %v, want %v", result.ID, result.Detected, want)
				}
			}
		})
	}
}

func TestDetectEmptyHome(t *testing.T) {
	t.Parallel()

	results, err := Detect(testEnv(t.TempDir()))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	fileWriterDetected := 0
	for _, result := range results {
		if result.PromptTier || !result.Detected {
			continue
		}
		fileWriterDetected++
	}
	if fileWriterDetected != 0 {
		t.Fatalf("detected %d file-writer clients in empty home, want 0", fileWriterDetected)
	}
}

func TestDetectionDoesNotReadContents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode os.FileMode
		body string
	}{
		{name: "unreadable config", mode: 0, body: "{}"},
		{name: "invalid json", mode: 0o600, body: "not json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			path := cursorConfigPath(env)
			writeFile(t, path, tt.body, tt.mode)
			if tt.mode == 0 {
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			}

			result := detectByID(t, env, "cursor")
			if !result.Detected {
				t.Fatalf("Cursor was not detected")
			}
		})
	}
}

func TestConfigPathPerGOOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		goos   string
		wantVS string
	}{
		{name: "darwin", goos: "darwin", wantVS: filepath.Join("/home/test", "Library", "Application Support", "Code", "User", "mcp.json")},
		{name: "windows", goos: "windows", wantVS: filepath.Join("/appdata", "Code", "User", "mcp.json")},
		{name: "linux", goos: "linux", wantVS: filepath.Join("/xdg", "Code", "User", "mcp.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: tt.goos}
			got, ok := ConfigPath("vscode", env)
			if !ok {
				t.Fatalf("VS Code client path not found")
			}
			if got != tt.wantVS {
				t.Fatalf("VS Code path = %q, want %q", got, tt.wantVS)
			}
			if !strings.HasSuffix(filepath.ToSlash(got), "Code/User/mcp.json") {
				t.Fatalf("VS Code path %q does not end with Code/User/mcp.json", got)
			}
		})
	}
}

func detectByID(t *testing.T, env Env, id string) DetectionResult {
	t.Helper()
	results, err := Detect(env)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	for _, result := range results {
		if result.ID == id {
			return result
		}
	}
	t.Fatalf("result %q not found", id)
	return DetectionResult{}
}

func testEnv(home string) Env {
	return Env{
		Home:          home,
		XDGConfigHome: filepath.Join(home, ".config"),
		AppData:       filepath.Join(home, "AppData", "Roaming"),
		GOOS:          "darwin",
	}
}

type clientMarker struct {
	client Client
	create func(t *testing.T, env Env)
}

func fileWriterMarkers() []clientMarker {
	markers := make([]clientMarker, 0, len(FileWriterClients()))
	for _, client := range FileWriterClients() {
		client := client
		markers = append(markers, clientMarker{
			client: client,
			create: func(t *testing.T, env Env) {
				t.Helper()
				mkdir(t, markerPath(client, env))
			},
		})
	}
	return markers
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, body string, mode os.FileMode) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
