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

func claudeCodeConfigPath(env Env) (string, error) {
	if env.ClaudeConfigDir != "" {
		return joinAbs(env.ClaudeConfigDir, ".claude.json")
	}
	return joinAbs(env.Home, ".claude.json")
}

func claudeCodeMarkerPath(env Env) (string, error) {
	if env.ClaudeConfigDir != "" {
		return requireAbs(env.ClaudeConfigDir)
	}
	return joinAbs(env.Home, ".claude")
}

func cursorConfigPath(env Env) (string, error) {
	return joinAbs(env.Home, ".cursor", "mcp.json")
}

func codexConfigPath(env Env) (string, error) {
	return joinAbs(env.Home, ".codex", "config.toml")
}

func windsurfConfigPath(env Env) (string, error) {
	return joinAbs(env.Home, ".codeium", "windsurf", "mcp_config.json")
}

func zedConfigPath(env Env) (string, error) {
	return joinAbs(env.xdgConfigHome(), "zed", "settings.json")
}

func vsCodeConfigPath(env Env) (string, error) {
	switch env.GOOS {
	case "darwin":
		return joinAbs(env.Home, "Library", "Application Support", "Code", "User", "mcp.json")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
		}
		return joinAbs(base, "Code", "User", "mcp.json")
	default:
		return joinAbs(env.xdgConfigHome(), "Code", "User", "mcp.json")
	}
}

func geminiCLIConfigPath(env Env) (string, error) {
	return joinAbs(env.Home, ".gemini", "settings.json")
}

func opencodeConfigPath(env Env) (string, error) {
	return joinAbs(env.xdgConfigHome(), "opencode", "opencode.json")
}

func claudeDesktopMarkerPath(env Env) (string, error) {
	switch env.GOOS {
	case "darwin":
		return joinAbs(env.Home, "Library", "Application Support", "Claude")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
		}
		return joinAbs(base, "Claude")
	default:
		return joinAbs(env.xdgConfigHome(), "Claude")
	}
}

func jetBrainsMarkerPath(env Env) (string, error) {
	switch env.GOOS {
	case "darwin":
		return joinAbs(env.Home, "Library", "Application Support", "JetBrains")
	case "windows":
		base := env.AppData
		if base == "" {
			base = join(env.Home, "AppData", "Roaming")
		}
		return joinAbs(base, "JetBrains")
	default:
		return joinAbs(env.xdgConfigHome(), "JetBrains")
	}
}

func markerPath(client Client, env Env) (string, error) {
	if client.MarkerPath != nil {
		return client.MarkerPath(env)
	}
	configPath, err := client.ConfigPath(env)
	if err != nil {
		return "", err
	}
	return requireAbs(filepath.Dir(configPath))
}

func join(elem ...string) string {
	return filepath.Join(elem...)
}

func joinAbs(elem ...string) (string, error) {
	return requireAbs(filepath.Join(elem...))
}

func requireAbs(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("config path must be absolute; check HOME or USERPROFILE")
	}
	return path, nil
}
