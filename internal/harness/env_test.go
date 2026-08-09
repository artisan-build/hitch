package harness

import (
	"errors"
	"testing"
)

func TestCurrentEnvHomeResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		home    string
		homeErr error
		wantErr string
	}{
		{
			name:    "both vars unset",
			homeErr: errors.New("missing home"),
			wantErr: "could not resolve user home from HOME or USERPROFILE",
		},
		{
			name:    "relative value",
			home:    "relative-home",
			wantErr: "resolved HOME or USERPROFILE is not absolute: \"relative-home\"",
		},
		{
			name: "valid absolute value",
			home: "/home/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env, err := currentEnv(func() (string, error) {
				return tt.home, tt.homeErr
			}, func(key string) string {
				values := map[string]string{
					"XDG_CONFIG_HOME":   "/xdg",
					"APPDATA":           "/appdata",
					"CLAUDE_CONFIG_DIR": "/claude-config",
					"CODEX_HOME":        "/codex-home",
				}
				return values[key]
			}, "linux")

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("currentEnv returned nil error, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("currentEnv error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("currentEnv returned error: %v", err)
			}
			if env.Home != "/home/test" || env.XDGConfigHome != "/xdg" || env.AppData != "/appdata" || env.ClaudeConfigDir != "/claude-config" || env.CodexHome != "/codex-home" || env.GOOS != "linux" {
				t.Fatalf("currentEnv = %#v", env)
			}
		})
	}
}
