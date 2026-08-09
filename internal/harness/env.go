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
	CodexHome       string
	GOOS            string
}

func CurrentEnv() (Env, error) {
	return currentEnv(os.UserHomeDir, os.Getenv, runtime.GOOS)
}

func currentEnv(homeFn func() (string, error), getenv func(string) string, goos string) (Env, error) {
	home, err := homeFn()
	if err != nil || home == "" {
		return Env{}, fmt.Errorf("could not resolve user home from HOME or USERPROFILE")
	}
	if !filepath.IsAbs(home) {
		return Env{}, fmt.Errorf("resolved HOME or USERPROFILE is not absolute: %q", home)
	}
	return Env{
		Home:            home,
		XDGConfigHome:   getenv("XDG_CONFIG_HOME"),
		AppData:         getenv("APPDATA"),
		ClaudeConfigDir: getenv("CLAUDE_CONFIG_DIR"),
		CodexHome:       getenv("CODEX_HOME"),
		GOOS:            goos,
	}, nil
}

func (env Env) xdgConfigHome() string {
	if env.XDGConfigHome != "" {
		return env.XDGConfigHome
	}
	return join(env.Home, ".config")
}

func (env Env) xdgConfigVar() string {
	if env.XDGConfigHome != "" {
		return "XDG_CONFIG_HOME"
	}
	return "HOME or USERPROFILE"
}
