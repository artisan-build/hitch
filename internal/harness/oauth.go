package harness

import "strings"

type OAuthSupport string

const (
	OAuthSupported OAuthSupport = "supported"
	OAuthUnknown   OAuthSupport = "unknown"
)

type OAuthCapability struct {
	ClientID          string
	NativeSupport     OAuthSupport
	HitchCanConfigure bool
	SourceURL         string
}

const (
	ClaudeDesktopOAuthSetupSource = "https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp"
	CodexOAuthSetupSource         = "https://developers.openai.com/codex/extend/mcp"
	JetBrainsMCPSetupSource       = "https://www.jetbrains.com/help/ai-assistant/mcp.html"
)

type OAuthLifecycleGuidance struct {
	ClientID      string
	LogoutCommand string
	SourceURL     string
}

// OAuthLifecycleGuidanceByClientID is the centralized table for commands hitch
// may print after uninstall. An empty command is intentional: official client
// documentation did not establish a token-removal command, so callers must use
// server-side account/token management instead of inventing one.
func OAuthLifecycleGuidanceByClientID(id string, serverName string) OAuthLifecycleGuidance {
	rows := map[string]OAuthLifecycleGuidance{
		"claude-code": {ClientID: "claude-code", LogoutCommand: "claude mcp logout " + strings.TrimSpace(serverName), SourceURL: "https://code.claude.com/docs/en/mcp#authenticate-with-remote-mcp-servers"},
		"cursor":      {ClientID: "cursor", SourceURL: "https://cursor.com/docs/mcp"},
		"windsurf":    {ClientID: "windsurf", SourceURL: "https://docs.windsurf.com/windsurf/cascade/mcp"},
		"zed":         {ClientID: "zed", SourceURL: "https://zed.dev/docs/ai/mcp"},
		"gemini-cli":  {ClientID: "gemini-cli", SourceURL: "https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md"},
		"opencode":    {ClientID: "opencode", LogoutCommand: "opencode mcp logout " + strings.TrimSpace(serverName), SourceURL: "https://opencode.ai/docs/mcp-servers/#authenticating"},
	}
	if row, ok := rows[id]; ok {
		return row
	}
	return OAuthLifecycleGuidance{ClientID: id}
}

func OAuthCapabilities() []OAuthCapability {
	return []OAuthCapability{
		{ClientID: "claude-code", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://code.claude.com/docs/en/mcp#authenticate-with-remote-mcp-servers"},
		{ClientID: "cursor", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://cursor.com/docs/mcp"},
		{ClientID: "codex", NativeSupport: OAuthSupported, HitchCanConfigure: false, SourceURL: "https://developers.openai.com/codex/mcp"},
		{ClientID: "windsurf", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://docs.windsurf.com/windsurf/cascade/mcp"},
		{ClientID: "zed", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://zed.dev/docs/ai/mcp"},
		{ClientID: "vscode", NativeSupport: OAuthUnknown, HitchCanConfigure: false, SourceURL: "https://code.visualstudio.com/docs/agents/reference/mcp-configuration"},
		{ClientID: "gemini-cli", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md"},
		{ClientID: "opencode", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://opencode.ai/docs/mcp-servers/"},
		{ClientID: "claude-desktop", NativeSupport: OAuthSupported, HitchCanConfigure: false, SourceURL: ClaudeDesktopOAuthSetupSource},
		{ClientID: "jetbrains", NativeSupport: OAuthUnknown, HitchCanConfigure: false, SourceURL: JetBrainsMCPSetupSource},
	}
}

func OAuthCapabilityByClientID(id string) (OAuthCapability, bool) {
	for _, capability := range OAuthCapabilities() {
		if capability.ClientID == id {
			return capability, true
		}
	}
	return OAuthCapability{ClientID: id, NativeSupport: OAuthUnknown}, false
}

func CanConfigureOAuth(id string) bool {
	capability, ok := OAuthCapabilityByClientID(id)
	return ok && capability.NativeSupport == OAuthSupported && capability.HitchCanConfigure
}
