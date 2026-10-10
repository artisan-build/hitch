package harness

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
		{ClientID: "claude-desktop", NativeSupport: OAuthSupported, HitchCanConfigure: false, SourceURL: "https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp"},
		{ClientID: "jetbrains", NativeSupport: OAuthUnknown, HitchCanConfigure: false, SourceURL: "https://www.jetbrains.com/help/ai-assistant/mcp.html"},
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
