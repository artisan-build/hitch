package harness

import "testing"

func TestOAuthCapabilitiesCoverEveryHarnessWithFrozenSources(t *testing.T) {
	want := map[string]OAuthCapability{
		"claude-code":    {ClientID: "claude-code", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://code.claude.com/docs/en/mcp#authenticate-with-remote-mcp-servers"},
		"cursor":         {ClientID: "cursor", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://cursor.com/docs/mcp"},
		"codex":          {ClientID: "codex", NativeSupport: OAuthSupported, SourceURL: "https://developers.openai.com/codex/mcp"},
		"windsurf":       {ClientID: "windsurf", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://docs.windsurf.com/windsurf/cascade/mcp"},
		"zed":            {ClientID: "zed", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://zed.dev/docs/ai/mcp"},
		"vscode":         {ClientID: "vscode", NativeSupport: OAuthUnknown, SourceURL: "https://code.visualstudio.com/docs/agents/reference/mcp-configuration"},
		"gemini-cli":     {ClientID: "gemini-cli", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md"},
		"opencode":       {ClientID: "opencode", NativeSupport: OAuthSupported, HitchCanConfigure: true, SourceURL: "https://opencode.ai/docs/mcp-servers/"},
		"claude-desktop": {ClientID: "claude-desktop", NativeSupport: OAuthSupported, SourceURL: "https://support.anthropic.com/en/articles/11503834-building-custom-integrations-via-remote-mcp-servers"},
		"jetbrains":      {ClientID: "jetbrains", NativeSupport: OAuthUnknown, SourceURL: "https://www.jetbrains.com/help/ai-assistant/mcp.html"},
	}

	got := map[string]OAuthCapability{}
	for _, capability := range OAuthCapabilities() {
		if capability.SourceURL == "" {
			t.Fatalf("capability %q has no source URL", capability.ClientID)
		}
		if _, duplicate := got[capability.ClientID]; duplicate {
			t.Fatalf("duplicate capability row for %q", capability.ClientID)
		}
		got[capability.ClientID] = capability
	}
	for _, client := range AllClients() {
		capability, ok := got[client.ID]
		if !ok {
			t.Fatalf("client %q has no OAuth capability row", client.ID)
		}
		if capability != want[client.ID] {
			t.Fatalf("capability %q = %#v, want %#v", client.ID, capability, want[client.ID])
		}
		delete(got, client.ID)
	}
	if len(got) != 0 {
		t.Fatalf("capability table has rows for unknown clients: %#v", got)
	}
}

func TestUnknownOAuthCapabilityIsNotWritable(t *testing.T) {
	capability, ok := OAuthCapabilityByClientID("future-client")
	if ok || capability.NativeSupport != OAuthUnknown || capability.HitchCanConfigure || CanConfigureOAuth("future-client") {
		t.Fatalf("unknown capability = %#v, found = %v", capability, ok)
	}
}
