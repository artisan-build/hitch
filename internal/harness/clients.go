package harness

import "path/filepath"

type Client struct {
	ID         string
	Name       string
	PromptTier bool
	ConfigPath func(Env) string
	MarkerPath func(Env) string
}

type DetectionResult struct {
	ID         string
	Name       string
	ConfigPath string
	Detected   bool
	PromptTier bool
}

func FileWriterClients() []Client {
	return []Client{
		{ID: "claude-code", Name: "Claude Code", ConfigPath: claudeCodeConfigPath, MarkerPath: claudeCodeMarkerPath},
		{ID: "cursor", Name: "Cursor", ConfigPath: cursorConfigPath},
		{ID: "codex", Name: "Codex", ConfigPath: codexConfigPath},
		{ID: "windsurf", Name: "Windsurf", ConfigPath: windsurfConfigPath},
		{ID: "zed", Name: "Zed", ConfigPath: zedConfigPath},
		{ID: "vscode", Name: "VS Code", ConfigPath: vsCodeConfigPath},
		{ID: "gemini-cli", Name: "Gemini CLI", ConfigPath: geminiCLIConfigPath},
		{ID: "opencode", Name: "opencode", ConfigPath: opencodeConfigPath},
	}
}

func PromptTierClients() []Client {
	return []Client{
		{ID: "claude-desktop", Name: "Claude Desktop", PromptTier: true, ConfigPath: claudeDesktopConfigPath},
		{ID: "jetbrains", Name: "JetBrains", PromptTier: true, ConfigPath: jetBrainsConfigPath},
	}
}

func AllClients() []Client {
	clients := FileWriterClients()
	clients = append(clients, PromptTierClients()...)
	return clients
}

func ConfigPath(clientID string, env Env) (string, bool) {
	for _, client := range AllClients() {
		if client.ID == clientID {
			return client.ConfigPath(env), true
		}
	}
	return "", false
}

func claudeCodeConfigPath(env Env) string {
	if env.ClaudeConfigDir != "" {
		return join(env.ClaudeConfigDir, ".claude.json")
	}
	return join(env.Home, ".claude.json")
}

func claudeCodeMarkerPath(env Env) string {
	if env.ClaudeConfigDir != "" {
		return env.ClaudeConfigDir
	}
	return join(env.Home, ".claude")
}

func cursorConfigPath(env Env) string {
	return join(env.Home, ".cursor", "mcp.json")
}

func codexConfigPath(env Env) string {
	return join(env.Home, ".codex", "config.toml")
}

func windsurfConfigPath(env Env) string {
	return join(env.Home, ".codeium", "windsurf", "mcp_config.json")
}

func zedConfigPath(env Env) string {
	return join(env.xdgConfigHome(), "zed", "settings.json")
}

func vsCodeConfigPath(env Env) string {
	switch env.GOOS {
	case "darwin":
		return join(env.Home, "Library", "Application Support", "Code", "User", "mcp.json")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
		}
		return join(base, "Code", "User", "mcp.json")
	default:
		return join(env.xdgConfigHome(), "Code", "User", "mcp.json")
	}
}

func geminiCLIConfigPath(env Env) string {
	return join(env.Home, ".gemini", "settings.json")
}

func opencodeConfigPath(env Env) string {
	return join(env.xdgConfigHome(), "opencode", "opencode.json")
}

func claudeDesktopConfigPath(env Env) string {
	switch env.GOOS {
	case "darwin":
		return join(env.Home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
		}
		return join(base, "Claude", "claude_desktop_config.json")
	default:
		return join(env.xdgConfigHome(), "Claude", "claude_desktop_config.json")
	}
}

func jetBrainsConfigPath(env Env) string {
	return join(env.xdgConfigHome(), "JetBrains")
}

func markerPath(client Client, env Env) string {
	if client.MarkerPath != nil {
		return client.MarkerPath(env)
	}
	return filepath.Dir(client.ConfigPath(env))
}

func join(elem ...string) string {
	return filepath.Join(elem...)
}
