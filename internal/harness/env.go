package harness

import (
	"os"
	"runtime"
)

type Env struct {
	Home            string
	XDGConfigHome   string
	AppData         string
	ClaudeConfigDir string
	GOOS            string
}

func CurrentEnv() Env {
	home, _ := os.UserHomeDir()
	return Env{
		Home:            home,
		XDGConfigHome:   os.Getenv("XDG_CONFIG_HOME"),
		AppData:         os.Getenv("APPDATA"),
		ClaudeConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
		GOOS:            runtime.GOOS,
	}
}

func (env Env) xdgConfigHome() string {
	if env.XDGConfigHome != "" {
		return env.XDGConfigHome
	}
	return join(env.Home, ".config")
}
