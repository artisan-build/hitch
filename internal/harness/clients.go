package harness

import (
	"fmt"
	"path/filepath"
)

type Client struct {
	ID         string
	Name       string
	PromptTier bool
	ConfigPath func(Env) (string, error)
	MarkerPath func(Env) (string, error)
}

type DetectionResult struct {
	ID         string
	Name       string
	ConfigPath string
	Detected   bool
	PromptTier bool
}

func FileWriterClientByID(id string) (Client, bool) {
	for _, client := range FileWriterClients() {
		if client.ID == id {
			return client, true
		}
	}
	return Client{}, false
}

func FileWriterClients() []Client {
	return []Client{
		{ID: "claude-code", Name: "Claude Code", ConfigPath: claudeCodeConfigPath, MarkerPath: claudeCodeMarkerPath},
		{ID: "cursor", Name: "Cursor", ConfigPath: cursorConfigPath},
		{ID: "codex", Name: "Codex", ConfigPath: codexConfigPath, MarkerPath: codexMarkerPath},
		{ID: "windsurf", Name: "Windsurf", ConfigPath: windsurfConfigPath},
		{ID: "zed", Name: "Zed", ConfigPath: zedConfigPath},
		{ID: "vscode", Name: "VS Code", ConfigPath: vsCodeConfigPath},
		{ID: "gemini-cli", Name: "Gemini CLI", ConfigPath: geminiCLIConfigPath},
		{ID: "opencode", Name: "opencode", ConfigPath: opencodeConfigPath, MarkerPath: opencodeMarkerPath},
	}
}

func PromptTierClients() []Client {
	return []Client{
		{ID: "claude-desktop", Name: "Claude Desktop", PromptTier: true, MarkerPath: claudeDesktopMarkerPath},
		{ID: "jetbrains", Name: "JetBrains", PromptTier: true, MarkerPath: jetBrainsMarkerPath},
	}
}

func AllClients() []Client {
	clients := FileWriterClients()
	clients = append(clients, PromptTierClients()...)
	return clients
}

func ConfigPath(clientID string, env Env) (string, bool, error) {
	for _, client := range AllClients() {
		if client.ID != clientID {
			continue
		}
		if client.ConfigPath == nil {
			return "", true, nil
		}
		path, err := client.ConfigPath(env)
		return path, true, err
	}
	return "", false, nil
}

func ProjectConfigPath(clientID string, env Env) (string, bool, error) {
	root := env.WorkDir
	if root == "" || !filepath.IsAbs(root) {
		return "", true, fmt.Errorf("project path must be absolute; check current working directory")
	}
	paths := map[string][]string{
		"claude-code": {".mcp.json"},
		"cursor":      {".cursor", "mcp.json"},
		"zed":         {".zed", "settings.json"},
		"vscode":      {".vscode", "mcp.json"},
		"gemini-cli":  {".gemini", "settings.json"},
		"opencode":    {"opencode.json"},
	}
	parts, ok := paths[clientID]
	if !ok {
		return "", false, nil
	}
	return filepath.Join(append([]string{root}, parts...)...), true, nil
}

func claudeCodeConfigPath(env Env) (string, error) {
	if env.ClaudeConfigDir != "" {
		return joinAbs("CLAUDE_CONFIG_DIR", env.ClaudeConfigDir, ".claude.json")
	}
	return joinAbs("HOME or USERPROFILE", env.Home, ".claude.json")
}

func claudeCodeMarkerPath(env Env) (string, error) {
	if env.ClaudeConfigDir != "" {
		return requireAbs("CLAUDE_CONFIG_DIR", env.ClaudeConfigDir)
	}
	return joinAbs("HOME or USERPROFILE", env.Home, ".claude")
}

func cursorConfigPath(env Env) (string, error) {
	return joinAbs("HOME or USERPROFILE", env.Home, ".cursor", "mcp.json")
}

func codexConfigPath(env Env) (string, error) {
	if env.CodexHome != "" {
		return joinAbs("CODEX_HOME", env.CodexHome, "config.toml")
	}
	return joinAbs("HOME or USERPROFILE", env.Home, ".codex", "config.toml")

}

func codexMarkerPath(env Env) (string, error) {
	if env.CodexHome != "" {
		return requireAbs("CODEX_HOME", env.CodexHome)
	}
	return joinAbs("HOME or USERPROFILE", env.Home, ".codex")
}

func windsurfConfigPath(env Env) (string, error) {
	return joinAbs("HOME or USERPROFILE", env.Home, ".codeium", "windsurf", "mcp_config.json")
}

func zedConfigPath(env Env) (string, error) {
	switch env.GOOS {
	case "darwin":
		return joinAbs("HOME or USERPROFILE", env.Home, ".config", "zed", "settings.json")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
			return joinAbs("HOME or USERPROFILE", base, "Zed", "settings.json")
		}
		return joinAbs("APPDATA", base, "Zed", "settings.json")
	default:
		return joinAbs(env.xdgConfigVar(), env.xdgConfigHome(), "zed", "settings.json")
	}
}

func vsCodeConfigPath(env Env) (string, error) {
	if env.VSCodePortable != "" {
		return joinAbs("VSCODE_PORTABLE", env.VSCodePortable, "user-data", "User", "mcp.json")
	}
	if env.VSCodeAppData != "" {
		return joinAbs("VSCODE_APPDATA", env.VSCodeAppData, "Code", "User", "mcp.json")
	}
	switch env.GOOS {
	case "darwin":
		return joinAbs("HOME or USERPROFILE", env.Home, "Library", "Application Support", "Code", "User", "mcp.json")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
			return joinAbs("HOME or USERPROFILE", base, "Code", "User", "mcp.json")
		}
		return joinAbs("APPDATA", base, "Code", "User", "mcp.json")
	default:
		return joinAbs(env.xdgConfigVar(), env.xdgConfigHome(), "Code", "User", "mcp.json")
	}
}

func geminiCLIConfigPath(env Env) (string, error) {
	return joinAbs("HOME or USERPROFILE", env.Home, ".gemini", "settings.json")
}

func opencodeConfigPath(env Env) (string, error) {
	if env.OpencodeConfigDir != "" {
		return joinAbs("OPENCODE_CONFIG_DIR", env.OpencodeConfigDir, "opencode.json")
	}
	return joinAbs(env.xdgConfigVar(), env.xdgConfigHome(), "opencode", "opencode.json")
}

func opencodeMarkerPath(env Env) (string, error) {
	if env.OpencodeConfigDir != "" {
		return requireAbs("OPENCODE_CONFIG_DIR", env.OpencodeConfigDir)
	}
	return joinAbs(env.xdgConfigVar(), env.xdgConfigHome(), "opencode")
}

func claudeDesktopMarkerPath(env Env) (string, error) {
	switch env.GOOS {
	case "darwin":
		return joinAbs("HOME or USERPROFILE", env.Home, "Library", "Application Support", "Claude")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
			return joinAbs("HOME or USERPROFILE", base, "Claude")
		}
		return joinAbs("APPDATA", base, "Claude")
	default:
		return joinAbs(env.xdgConfigVar(), env.xdgConfigHome(), "Claude")
	}
}

func jetBrainsMarkerPath(env Env) (string, error) {
	switch env.GOOS {
	case "darwin":
		return joinAbs("HOME or USERPROFILE", env.Home, "Library", "Application Support", "JetBrains")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
			return joinAbs("HOME or USERPROFILE", base, "JetBrains")
		}
		return joinAbs("APPDATA", base, "JetBrains")
	default:
		return joinAbs(env.xdgConfigVar(), env.xdgConfigHome(), "JetBrains")
	}
}

func markerPath(client Client, env Env) (string, error) {
	if client.MarkerPath != nil {
		return client.MarkerPath(env)
	}
	if client.ConfigPath == nil {
		return "", fmt.Errorf("client %q has no config path or marker path", client.ID)
	}
	configPath, err := client.ConfigPath(env)
	if err != nil {
		return "", err
	}
	return requireAbs("config path", filepath.Dir(configPath))
}

func join(elem ...string) string {
	return filepath.Join(elem...)
}

func joinAbs(source string, elem ...string) (string, error) {
	return requireAbs(source, filepath.Join(elem...))
}

func requireAbs(source string, path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("config path must be absolute; check %s", source)
	}
	return path, nil
}
