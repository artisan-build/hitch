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

	for _, marker := range expectedFileWriterClients() {
		marker := marker
		t.Run(marker.id, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := testEnv(home)
			mkdir(t, marker.markerPath(t, env))

			results, err := Detect(env)
			if err != nil {
				t.Fatalf("Detect returned error: %v", err)
			}

			for _, result := range results {
				if result.PromptTier {
					continue
				}
				want := result.ID == marker.id
				if result.Detected != want {
					t.Fatalf("%s detected = %v, want %v", result.ID, result.Detected, want)
				}
				if result.ID == marker.id && result.ConfigPath != marker.configPath(t, env) {
					t.Fatalf("%s config path = %q, want %q", result.ID, result.ConfigPath, marker.configPath(t, env))
				}
			}
		})
	}
}

func TestCodexHomeDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, home string) Env
		detected bool
		path     func(home string) string
	}{
		{
			name: "unset CODEX_HOME uses default",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				mkdir(t, filepath.Join(home, ".codex"))
				return testEnv(home)
			},
			detected: true,
			path: func(home string) string {
				return filepath.Join(home, ".codex", "config.toml")
			},
		},
		{
			name: "CODEX_HOME set and exists",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				env := testEnv(home)
				env.CodexHome = filepath.Join(home, "codex-home")
				mkdir(t, env.CodexHome)
				return env
			},
			detected: true,
			path: func(home string) string {
				return filepath.Join(home, "codex-home", "config.toml")
			},
		},
		{
			name: "CODEX_HOME set and missing",
			setup: func(_ *testing.T, home string) Env {
				env := testEnv(home)
				env.CodexHome = filepath.Join(home, "missing-codex-home")
				return env
			},
			path: func(home string) string {
				return filepath.Join(home, "missing-codex-home", "config.toml")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := tt.setup(t, home)
			result := detectByID(t, env, "codex")
			if result.Detected != tt.detected {
				t.Fatalf("Codex detected = %v, want %v", result.Detected, tt.detected)
			}
			if result.ConfigPath != tt.path(home) {
				t.Fatalf("Codex config path = %q, want %q", result.ConfigPath, tt.path(home))
			}
		})
	}
}

func TestOpencodeConfigDirDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, home string) Env
		detected bool
		path     func(home string) string
	}{
		{
			name: "unset OPENCODE_CONFIG_DIR uses XDG default",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				env := testEnv(home)
				mkdir(t, filepath.Join(env.XDGConfigHome, "opencode"))
				return env
			},
			detected: true,
			path: func(home string) string {
				return filepath.Join(home, ".config", "opencode", "opencode.json")
			},
		},
		{
			name: "OPENCODE_CONFIG_DIR set and exists",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				env := testEnv(home)
				env.OpencodeConfigDir = filepath.Join(home, "opencode-config")
				mkdir(t, env.OpencodeConfigDir)
				return env
			},
			detected: true,
			path: func(home string) string {
				return filepath.Join(home, "opencode-config", "opencode.json")
			},
		},
		{
			name: "OPENCODE_CONFIG_DIR set and missing",
			setup: func(_ *testing.T, home string) Env {
				env := testEnv(home)
				env.OpencodeConfigDir = filepath.Join(home, "missing-opencode-config")
				return env
			},
			path: func(home string) string {
				return filepath.Join(home, "missing-opencode-config", "opencode.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := tt.setup(t, home)
			result := detectByID(t, env, "opencode")
			if result.Detected != tt.detected {
				t.Fatalf("opencode detected = %v, want %v", result.Detected, tt.detected)
			}
			if result.ConfigPath != tt.path(home) {
				t.Fatalf("opencode config path = %q, want %q", result.ConfigPath, tt.path(home))
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
			path := filepath.Join(home, ".claude.json")
			writeFile(t, path, tt.body, tt.mode)
			if tt.mode == 0 {
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			}

			if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
				t.Fatalf("fixture must not create Claude Code marker dir; stat err = %v", err)
			}

			result := detectByID(t, env, "claude-code")
			if !result.Detected {
				t.Fatalf("Claude Code was not detected")
			}
		})
	}
}

func TestConfigPathLiteralTable(t *testing.T) {
	t.Parallel()

	home := "/home/test"
	env := Env{Home: home, XDGConfigHome: filepath.Join(home, ".config"), AppData: filepath.Join(home, "AppData", "Roaming"), GOOS: "darwin"}
	for _, client := range expectedFileWriterClients() {
		client := client
		t.Run(client.id, func(t *testing.T) {
			t.Parallel()
			got, ok, err := ConfigPath(client.id, env)
			if err != nil {
				t.Fatalf("ConfigPath returned error: %v", err)
			}
			if !ok {
				t.Fatalf("ConfigPath did not find %s", client.id)
			}
			want := client.configPath(t, env)
			if got != want {
				t.Fatalf("ConfigPath(%s) = %q, want %q", client.id, got, want)
			}
			if !filepath.IsAbs(got) {
				t.Fatalf("ConfigPath(%s) = %q, want absolute", client.id, got)
			}
			if client.shapeSuffix != "" && !strings.HasSuffix(filepath.ToSlash(got), client.shapeSuffix) {
				t.Fatalf("ConfigPath(%s) = %q, want suffix %q", client.id, got, client.shapeSuffix)
			}
		})
	}
}

func TestProjectConfigPathLiteralTable(t *testing.T) {
	t.Parallel()

	project := filepath.Join(string(filepath.Separator), "work", "project")
	env := Env{Home: "/home/test", WorkDir: project, XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: "darwin"}
	tests := map[string]string{
		"claude-code": filepath.Join(project, ".mcp.json"),
		"cursor":      filepath.Join(project, ".cursor", "mcp.json"),
		"codex":       filepath.Join(project, ".codex", "config.toml"),
		"zed":         filepath.Join(project, ".zed", "settings.json"),
		"vscode":      filepath.Join(project, ".vscode", "mcp.json"),
		"gemini-cli":  filepath.Join(project, ".gemini", "settings.json"),
		"opencode":    filepath.Join(project, "opencode.json"),
	}
	for id, want := range tests {
		id, want := id, want
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			got, ok, err := ProjectConfigPath(id, env)
			if err != nil {
				t.Fatalf("ProjectConfigPath returned error: %v", err)
			}
			if !ok {
				t.Fatalf("ProjectConfigPath did not find %s", id)
			}
			if got != want {
				t.Fatalf("ProjectConfigPath(%s) = %q, want %q", id, got, want)
			}
		})
	}
	if got, ok, err := ProjectConfigPath("windsurf", env); err != nil || ok || got != "" {
		t.Fatalf("ProjectConfigPath(windsurf) = %q, %v, %v; want unsupported", got, ok, err)
	}
}

func TestConfigPathPerGOOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		wantVS  string
		wantZed string
	}{
		{name: "darwin", goos: "darwin", wantVS: filepath.Join("/home/test", "Library", "Application Support", "Code", "User", "mcp.json"), wantZed: filepath.Join("/home/test", ".config", "zed", "settings.json")},
		{name: "windows", goos: "windows", wantVS: filepath.Join("/appdata", "Code", "User", "mcp.json"), wantZed: filepath.Join("/appdata", "Zed", "settings.json")},
		{name: "linux", goos: "linux", wantVS: filepath.Join("/xdg", "Code", "User", "mcp.json"), wantZed: filepath.Join("/xdg", "zed", "settings.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: tt.goos}
			got, ok, err := ConfigPath("vscode", env)
			if err != nil {
				t.Fatalf("ConfigPath returned error: %v", err)
			}
			if !ok {
				t.Fatalf("VS Code client path not found")
			}
			if got != tt.wantVS {
				t.Fatalf("VS Code path = %q, want %q", got, tt.wantVS)
			}
			if !strings.HasSuffix(filepath.ToSlash(got), "Code/User/mcp.json") {
				t.Fatalf("VS Code path %q does not end with Code/User/mcp.json", got)
			}

			gotZed, ok, err := ConfigPath("zed", env)
			if err != nil {
				t.Fatalf("Zed ConfigPath returned error: %v", err)
			}
			if !ok {
				t.Fatalf("Zed client path not found")
			}
			if gotZed != tt.wantZed {
				t.Fatalf("Zed path = %q, want %q", gotZed, tt.wantZed)
			}
		})
	}
}

func TestVSCodeUserDataOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  Env
		want string
	}{
		{
			name: "VSCODE_PORTABLE wins over VSCODE_APPDATA",
			env:  Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", VSCodePortable: "/portable", VSCodeAppData: "/vscode-appdata", GOOS: "linux"},
			want: filepath.Join("/portable", "user-data", "User", "mcp.json"),
		},
		{
			name: "VSCODE_APPDATA before platform default",
			env:  Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", VSCodeAppData: "/vscode-appdata", GOOS: "linux"},
			want: filepath.Join("/vscode-appdata", "Code", "User", "mcp.json"),
		},
		{
			name: "platform default after overrides",
			env:  Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: "linux"},
			want: filepath.Join("/xdg", "Code", "User", "mcp.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := ConfigPath("vscode", tt.env)
			if err != nil {
				t.Fatalf("ConfigPath returned error: %v", err)
			}
			if !ok {
				t.Fatalf("VS Code client path not found")
			}
			if got != tt.want {
				t.Fatalf("VS Code path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVSCodeUserDataOverrideDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, home string) Env
		wantPath func(home string) string
	}{
		{
			name: "VSCODE_PORTABLE marker wins over VSCODE_APPDATA",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				env := testEnv(home)
				env.GOOS = "linux"
				env.VSCodePortable = filepath.Join(home, "portable")
				env.VSCodeAppData = filepath.Join(home, "vscode-appdata")
				mkdir(t, filepath.Join(env.VSCodePortable, "user-data", "User"))
				return env
			},
			wantPath: func(home string) string { return filepath.Join(home, "portable", "user-data", "User", "mcp.json") },
		},
		{
			name: "VSCODE_APPDATA marker before platform default",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				env := testEnv(home)
				env.GOOS = "linux"
				env.VSCodeAppData = filepath.Join(home, "vscode-appdata")
				mkdir(t, filepath.Join(env.VSCodeAppData, "Code", "User"))
				return env
			},
			wantPath: func(home string) string { return filepath.Join(home, "vscode-appdata", "Code", "User", "mcp.json") },
		},
		{
			name: "override missing is not detected by platform default marker",
			setup: func(t *testing.T, home string) Env {
				t.Helper()
				env := testEnv(home)
				env.GOOS = "linux"
				env.VSCodeAppData = filepath.Join(home, "missing-vscode-appdata")
				mkdir(t, filepath.Join(env.XDGConfigHome, "Code", "User"))
				return env
			},
			wantPath: func(home string) string {
				return filepath.Join(home, "missing-vscode-appdata", "Code", "User", "mcp.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := tt.setup(t, home)
			result := detectByID(t, env, "vscode")
			wantDetected := !strings.Contains(tt.name, "missing")
			if result.Detected != wantDetected {
				t.Fatalf("VS Code detected = %v, want %v", result.Detected, wantDetected)
			}
			if result.ConfigPath != tt.wantPath(home) {
				t.Fatalf("VS Code config path = %q, want %q", result.ConfigPath, tt.wantPath(home))
			}
		})
	}
}

func TestZedDarwinIgnoresXDGConfigHome(t *testing.T) {
	t.Parallel()

	env := Env{Home: "/home/test", XDGConfigHome: "/elsewhere", AppData: "/appdata", GOOS: "darwin"}
	got, ok, err := ConfigPath("zed", env)
	if err != nil {
		t.Fatalf("ConfigPath returned error: %v", err)
	}
	if !ok {
		t.Fatalf("Zed client path not found")
	}
	want := filepath.Join("/home/test", ".config", "zed", "settings.json")
	if got != want {
		t.Fatalf("Zed darwin path = %q, want %q", got, want)
	}
}

func TestZedAndOpencodeDarwinXDGSourceDifference(t *testing.T) {
	t.Parallel()

	// Zed source: zed-industries/zed crates/paths/src/paths.rs config_dir()
	// hardcodes ~/.config/zed on macOS. opencode source: sst/opencode
	// packages/core/src/global.ts uses xdg-basedir and honors XDG_CONFIG_HOME.
	env := Env{Home: "/home/test", XDGConfigHome: "/elsewhere", AppData: "/appdata", GOOS: "darwin"}
	zed, ok, err := ConfigPath("zed", env)
	if err != nil || !ok {
		t.Fatalf("Zed ConfigPath = %q, %v, %v", zed, ok, err)
	}
	opencode, ok, err := ConfigPath("opencode", env)
	if err != nil || !ok {
		t.Fatalf("opencode ConfigPath = %q, %v, %v", opencode, ok, err)
	}
	if zed != filepath.Join("/home/test", ".config", "zed", "settings.json") {
		t.Fatalf("Zed darwin path = %q", zed)
	}
	if opencode != filepath.Join("/elsewhere", "opencode", "opencode.json") {
		t.Fatalf("opencode darwin path = %q", opencode)
	}
}

func TestConfigPathRejectsEmptyHome(t *testing.T) {
	t.Parallel()

	_, _, err := ConfigPath("cursor", Env{GOOS: "darwin"})
	if err == nil {
		t.Fatalf("ConfigPath with empty home returned nil error")
	}
	if !strings.Contains(err.Error(), "HOME") || !strings.Contains(err.Error(), "USERPROFILE") {
		t.Fatalf("ConfigPath error = %q, want HOME and USERPROFILE", err.Error())
	}
}

func TestConfigPathRejectsNonAbsoluteOverrideVariables(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  Env
		id   string
		want string
	}{
		{name: "XDG_CONFIG_HOME", env: Env{Home: "/home/test", XDGConfigHome: "relative-xdg", GOOS: "linux"}, id: "opencode", want: "XDG_CONFIG_HOME"},
		{name: "APPDATA", env: Env{Home: "/home/test", AppData: "relative-appdata", GOOS: "windows"}, id: "vscode", want: "APPDATA"},
		{name: "CLAUDE_CONFIG_DIR", env: Env{Home: "/home/test", ClaudeConfigDir: "relative-claude", GOOS: "darwin"}, id: "claude-code", want: "CLAUDE_CONFIG_DIR"},
		{name: "CODEX_HOME", env: Env{Home: "/home/test", CodexHome: "relative-codex", GOOS: "darwin"}, id: "codex", want: "CODEX_HOME"},
		{name: "OPENCODE_CONFIG_DIR", env: Env{Home: "/home/test", OpencodeConfigDir: "relative-opencode", GOOS: "darwin"}, id: "opencode", want: "OPENCODE_CONFIG_DIR"},
		{name: "VSCODE_PORTABLE", env: Env{Home: "/home/test", VSCodePortable: "relative-portable", GOOS: "darwin"}, id: "vscode", want: "VSCODE_PORTABLE"},
		{name: "VSCODE_APPDATA", env: Env{Home: "/home/test", VSCodeAppData: "relative-vscode-appdata", GOOS: "darwin"}, id: "vscode", want: "VSCODE_APPDATA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := ConfigPath(tt.id, tt.env)
			if err == nil {
				t.Fatalf("ConfigPath returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ConfigPath error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestAllResolvedPathsAreAbsolute(t *testing.T) {
	t.Parallel()

	env := testEnv(t.TempDir())
	for _, client := range AllClients() {
		client := client
		t.Run(client.ID, func(t *testing.T) {
			t.Parallel()
			if client.ConfigPath != nil {
				path, err := client.ConfigPath(env)
				if err != nil {
					t.Fatalf("ConfigPath returned error: %v", err)
				}
				if !filepath.IsAbs(path) {
					t.Fatalf("config path = %q, want absolute", path)
				}
			}
			marker, err := markerPath(client, env)
			if err != nil {
				t.Fatalf("markerPath returned error: %v", err)
			}
			if !filepath.IsAbs(marker) {
				t.Fatalf("marker path = %q, want absolute", marker)
			}
		})
	}
}

func TestMarkerPathErrorsWithoutConfigOrMarker(t *testing.T) {
	t.Parallel()

	_, err := markerPath(Client{ID: "broken"}, testEnv(t.TempDir()))
	if err == nil {
		t.Fatalf("markerPath returned nil error")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Fatalf("markerPath error = %q, want client ID", err.Error())
	}
}

func TestPromptTierDetectionAndLabelData(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	mkdir(t, filepath.Join(home, "Library", "Application Support", "Claude"))
	mkdir(t, filepath.Join(home, "Library", "Application Support", "JetBrains"))

	for _, id := range []string{"claude-desktop", "jetbrains"} {
		result := detectByID(t, env, id)
		if !result.PromptTier {
			t.Fatalf("%s PromptTier = false, want true", id)
		}
		if !result.Detected {
			t.Fatalf("%s Detected = false, want true", id)
		}
		if result.ConfigPath != "" {
			t.Fatalf("%s ConfigPath = %q, want empty", id, result.ConfigPath)
		}
	}
}

func TestPromptTierFieldIsUsed(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	env := testEnv(home)
	mkdir(t, filepath.Join(home, "Library", "Application Support", "Claude"))
	clients := PromptTierClients()
	clients[0].PromptTier = false
	results, err := detect(env, nil, clients[:1])
	if err != nil {
		t.Fatalf("detect returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if results[0].PromptTier {
		t.Fatalf("PromptTier = true, want false after flipped client field")
	}
}

func TestPromptTierMarkerPathPerGOOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		env               Env
		wantClaudeDesktop string
		wantJetBrains     string
	}{
		{
			name:              "darwin",
			env:               Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: "darwin"},
			wantClaudeDesktop: filepath.Join("/home/test", "Library", "Application Support", "Claude"),
			wantJetBrains:     filepath.Join("/home/test", "Library", "Application Support", "JetBrains"),
		},
		{
			name:              "windows",
			env:               Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: "windows"},
			wantClaudeDesktop: filepath.Join("/appdata", "Claude"),
			wantJetBrains:     filepath.Join("/appdata", "JetBrains"),
		},
		{
			name:              "linux",
			env:               Env{Home: "/home/test", XDGConfigHome: "/xdg", AppData: "/appdata", GOOS: "linux"},
			wantClaudeDesktop: filepath.Join("/xdg", "Claude"),
			wantJetBrains:     filepath.Join("/xdg", "JetBrains"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claudeDesktop, err := claudeDesktopMarkerPath(tt.env)
			if err != nil {
				t.Fatalf("claudeDesktopMarkerPath returned error: %v", err)
			}
			if claudeDesktop != tt.wantClaudeDesktop {
				t.Fatalf("Claude Desktop marker = %q, want %q", claudeDesktop, tt.wantClaudeDesktop)
			}
			jetBrains, err := jetBrainsMarkerPath(tt.env)
			if err != nil {
				t.Fatalf("jetBrainsMarkerPath returned error: %v", err)
			}
			if jetBrains != tt.wantJetBrains {
				t.Fatalf("JetBrains marker = %q, want %q", jetBrains, tt.wantJetBrains)
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

type expectedClient struct {
	id          string
	shapeSuffix string
	configPath  func(t *testing.T, env Env) string
	markerPath  func(t *testing.T, env Env) string
}

func expectedFileWriterClients() []expectedClient {
	return []expectedClient{
		{id: "claude-code", shapeSuffix: ".claude.json", configPath: literalPath(".claude.json"), markerPath: literalPath(".claude")},
		{id: "cursor", shapeSuffix: ".cursor/mcp.json", configPath: literalPath(".cursor", "mcp.json"), markerPath: literalPath(".cursor")},
		{id: "codex", shapeSuffix: ".codex/config.toml", configPath: literalPath(".codex", "config.toml"), markerPath: literalPath(".codex")},
		{id: "windsurf", shapeSuffix: ".codeium/windsurf/mcp_config.json", configPath: literalPath(".codeium", "windsurf", "mcp_config.json"), markerPath: literalPath(".codeium", "windsurf")},
		{id: "zed", shapeSuffix: ".config/zed/settings.json", configPath: literalPath(".config", "zed", "settings.json"), markerPath: literalPath(".config", "zed")},
		{id: "vscode", shapeSuffix: "Code/User/mcp.json", configPath: literalPath("Library", "Application Support", "Code", "User", "mcp.json"), markerPath: literalPath("Library", "Application Support", "Code", "User")},
		{id: "gemini-cli", shapeSuffix: ".gemini/settings.json", configPath: literalPath(".gemini", "settings.json"), markerPath: literalPath(".gemini")},
		{id: "opencode", shapeSuffix: ".config/opencode/opencode.json", configPath: xdgLiteralPath("opencode", "opencode.json"), markerPath: xdgLiteralPath("opencode")},
	}
}

func literalPath(elem ...string) func(t *testing.T, env Env) string {
	return func(t *testing.T, env Env) string {
		t.Helper()
		parts := append([]string{env.Home}, elem...)
		return filepath.Join(parts...)
	}
}

func xdgLiteralPath(elem ...string) func(t *testing.T, env Env) string {
	return func(t *testing.T, env Env) string {
		t.Helper()
		parts := append([]string{env.XDGConfigHome}, elem...)
		return filepath.Join(parts...)
	}
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
