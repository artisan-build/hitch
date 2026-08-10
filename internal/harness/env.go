package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Env struct {
	Home              string
	XDGConfigHome     string
	AppData           string
	ClaudeConfigDir   string
	CodexHome         string
	OpencodeConfigDir string
	VSCodePortable    string
	VSCodeAppData     string
	WorkDir           string
	GOOS              string
}

func CurrentEnv() (Env, error) {
	env, err := currentEnv(os.UserHomeDir, os.Getenv, runtime.GOOS)
	if err != nil {
		return Env{}, err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return Env{}, fmt.Errorf("could not resolve current working directory: %w", err)
	}
	if !filepath.IsAbs(workDir) {
		return Env{}, fmt.Errorf("resolved current working directory is not absolute: %q", workDir)
	}
	env.WorkDir = workDir
	return env, nil
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
		Home:              home,
		XDGConfigHome:     getenv("XDG_CONFIG_HOME"),
		AppData:           getenv("APPDATA"),
		ClaudeConfigDir:   getenv("CLAUDE_CONFIG_DIR"),
		CodexHome:         getenv("CODEX_HOME"),
		OpencodeConfigDir: getenv("OPENCODE_CONFIG_DIR"),
		VSCodePortable:    getenv("VSCODE_PORTABLE"),
		VSCodeAppData:     getenv("VSCODE_APPDATA"),
		GOOS:              goos,
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
