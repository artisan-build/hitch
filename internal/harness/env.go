package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Env struct {
	Home            string
	XDGConfigHome   string
	AppData         string
	ClaudeConfigDir string
	GOOS            string
}

func CurrentEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return Env{}, fmt.Errorf("could not resolve user home from HOME or USERPROFILE")
	}
	if !filepath.IsAbs(home) {
		return Env{}, fmt.Errorf("resolved HOME or USERPROFILE is not absolute: %q", home)
	}
	return Env{
		Home:            home,
		XDGConfigHome:   os.Getenv("XDG_CONFIG_HOME"),
		AppData:         os.Getenv("APPDATA"),
		ClaudeConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
		GOOS:            runtime.GOOS,
	}, nil
}

func (env Env) xdgConfigHome() string {
	if env.XDGConfigHome != "" {
		return env.XDGConfigHome
	}
	return join(env.Home, ".config")
}
