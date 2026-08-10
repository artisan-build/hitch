package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/artisan-build/hitch/internal/harness"
)

const codexTokenPrefix = "HITCH_TOKEN_"

var genericNames = map[string]bool{"api": true, "www": true, "app": true, "server": true}

type Adapter struct {
	ClientID         string
	ConfigKey        string
	ConfigFormat     string
	TokenPersisted   bool
	BuildRemoteEntry func(url string, headers map[string]string, envVar string) map[string]any
	BuildStdioEntry  func(command string, args []string, env map[string]string) map[string]any
}

type Target struct {
	Client harness.DetectionResult
	Path   string
}

type ScanStatus string

const (
	ScanHasEntry   ScanStatus = "has_entry"
	ScanNoEntry    ScanStatus = "no_entry"
	ScanUnreadable ScanStatus = "unreadable"
)

type ScanResult struct {
	Client          harness.DetectionResult
	Path            string
	Status          ScanStatus
	HoldsCredential bool
	Detail          string
}

type UninstallOptions struct {
	Name        string
	Clients     []string
	Yes         bool
	NonTTY      bool
	PickTargets func([]ScanResult, []ScanResult) ([]ScanResult, error)
	Env         harness.Env
}

type UninstallResult struct {
	Name       string
	Removed    []ScanResult
	NotPresent []ScanResult
	Unreadable []ScanResult
}

type Options struct {
	URL         string
	Name        string
	Headers     map[string]string
	Command     string
	Args        []string
	StdioEnv    map[string]string
	Clients     []string
	Yes         bool
	DryRun      bool
	Forget      bool
	NonTTY      bool
	ConfirmName func(string) (bool, error)
	PickTargets func([]Target, map[string]bool) ([]Target, error)
	Env         harness.Env
	Stdout      io.Writer
}

type Result struct {
	Name        string
	URL         string
	Written     []string
	WrittenInfo []WriteInfo
	WouldWrite  []string
	Failures    []string
	CodexEnvVar string
	Manual      []string
}

type WriteInfo struct {
	ClientName string
	Path       string
}

func Adapters() []Adapter {
	return []Adapter{
		{ClientID: "claude-code", ConfigKey: "mcpServers", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"type": "http", "url": url, "headers": headers}
		}, BuildStdioEntry: stdioCommandArgsEnvEntry},
		{ClientID: "cursor", ConfigKey: "mcpServers", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"url": url, "headers": headers}
		}, BuildStdioEntry: func(command string, args []string, env map[string]string) map[string]any {
			entry := stdioCommandArgsEnvEntry(command, args, env)
			entry["type"] = "stdio"
			return entry
		}},
		{ClientID: "windsurf", ConfigKey: "mcpServers", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"serverUrl": url, "headers": headers}
		}, BuildStdioEntry: stdioCommandArgsEnvEntry},
		{ClientID: "zed", ConfigKey: "context_servers", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"url": url, "headers": headers}
		}, BuildStdioEntry: stdioCommandArgsEnvEntry},
		{ClientID: "vscode", ConfigKey: "servers", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"type": "http", "url": url, "headers": headers}
		}, BuildStdioEntry: func(command string, args []string, env map[string]string) map[string]any {
			entry := stdioCommandArgsEnvEntry(command, args, env)
			entry["type"] = "stdio"
			return entry
		}},
		{ClientID: "gemini-cli", ConfigKey: "mcpServers", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"httpUrl": url, "headers": headers}
		}, BuildStdioEntry: stdioCommandArgsEnvEntry},
		{ClientID: "opencode", ConfigKey: "mcp", ConfigFormat: "json", TokenPersisted: true, BuildRemoteEntry: func(url string, headers map[string]string, _ string) map[string]any {
			return map[string]any{"type": "remote", "url": url, "headers": headers}
		}, BuildStdioEntry: func(command string, args []string, env map[string]string) map[string]any {
			entry := map[string]any{"type": "local", "command": append([]string{command}, args...)}
			if len(env) > 0 {
				entry["environment"] = env
			}
			return entry
		}},
	}
}

func stdioCommandArgsEnvEntry(command string, args []string, env map[string]string) map[string]any {
	entry := map[string]any{"command": command}
	if len(args) > 0 {
		entry["args"] = args
	}
	if len(env) > 0 {
		entry["env"] = env
	}
	return entry
}

func AdapterByClientID(id string) (Adapter, bool) {
	for _, adapter := range Adapters() {
		if adapter.ClientID == id {
			return adapter, true
		}
	}
	return Adapter{}, false
}

func Scan(env harness.Env, name string, clientIDs []string) ([]ScanResult, error) {
	if name != "" {
		name = sanitizeName(name)
	}
	selected := map[string]bool{}
	for _, id := range clientIDs {
		selected[id] = true
	}
	results, err := harness.Detect(env)
	if err != nil {
		return nil, err
	}
	out := []ScanResult{}
	for _, detected := range results {
		adapter, ok := AdapterByClientID(detected.ID)
		if !ok {
			continue
		}
		if len(selected) > 0 && !selected[detected.ID] {
			continue
		}
		out = append(out, scanOne(detected, adapter, name))
	}
	if len(selected) > 0 && len(out) != len(selected) {
		for id := range selected {
			if _, ok := AdapterByClientID(id); !ok {
				return nil, fmt.Errorf("unknown file-writer client %q", id)
			}
		}
	}
	return out, nil
}

func scanOne(client harness.DetectionResult, adapter Adapter, name string) ScanResult {
	result := ScanResult{Client: client, Path: client.ConfigPath, Status: ScanNoEntry}
	raw, existed, err := readExisting(client.ConfigPath)
	if err != nil {
		result.Status = ScanUnreadable
		result.Detail = err.Error()
		return result
	}
	if !existed || len(bytes.TrimSpace(raw)) == 0 {
		return result
	}
	if err := rejectDuplicateTopLevelKeys(raw, client.ConfigPath); err != nil {
		result.Status = ScanUnreadable
		result.Detail = err.Error()
		return result
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		result.Status = ScanUnreadable
		result.Detail = jsonConfigError(client.ConfigPath, adapter.ClientID == "zed", err)
		return result
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		result.Status = ScanUnreadable
		result.Detail = fmt.Sprintf("existing config %s is not a JSON object", client.ConfigPath)
		return result
	}
	serversRaw, ok := object[adapter.ConfigKey]
	if !ok {
		return result
	}
	servers, ok := serversRaw.(map[string]any)
	if !ok {
		result.Status = ScanUnreadable
		result.Detail = fmt.Sprintf("existing config %s has non-object %q", client.ConfigPath, adapter.ConfigKey)
		return result
	}
	if name == "" {
		if len(servers) == 0 {
			return result
		}
		result.Status = ScanHasEntry
		result.HoldsCredential = holdsCredential(servers)
		return result
	}
	entry, ok := servers[name]
	if !ok {
		return result
	}
	result.Status = ScanHasEntry
	result.HoldsCredential = holdsCredential(entry)
	return result
}

func Uninstall(opts UninstallOptions) (UninstallResult, error) {
	name := sanitizeName(opts.Name)
	if name == "" {
		return UninstallResult{}, fmt.Errorf("uninstall requires a non-empty server name")
	}
	res := UninstallResult{Name: name}
	scans, err := Scan(opts.Env, name, opts.Clients)
	if err != nil {
		return res, err
	}
	present := []ScanResult{}
	for _, scan := range scans {
		switch scan.Status {
		case ScanHasEntry:
			present = append(present, scan)
		case ScanUnreadable:
			res.Unreadable = append(res.Unreadable, scan)
		default:
			res.NotPresent = append(res.NotPresent, scan)
		}
	}
	selected := present
	if len(opts.Clients) == 0 && !opts.Yes {
		if opts.NonTTY {
			return res, fmt.Errorf("non-TTY uninstall requires either -y/--yes or -c/--client")
		}
		if opts.PickTargets == nil {
			return res, fmt.Errorf("interactive picker is unavailable")
		}
		selected, err = opts.PickTargets(present, res.Unreadable)
		if err != nil {
			return res, err
		}
	}
	for _, target := range selected {
		adapter, ok := AdapterByClientID(target.Client.ID)
		if !ok {
			return res, fmt.Errorf("unknown file-writer client %q", target.Client.ID)
		}
		changed, err := RemoveEntry(target.Path, adapter, name)
		if err != nil {
			target.Status = ScanUnreadable
			target.Detail = err.Error()
			res.Unreadable = append(res.Unreadable, target)
			continue
		}
		if changed {
			res.Removed = append(res.Removed, target)
		} else {
			res.NotPresent = append(res.NotPresent, target)
		}
	}
	if len(res.Unreadable) > 0 {
		return res, fmt.Errorf("could not verify %d config file(s)", len(res.Unreadable))
	}
	return res, nil
}

func InferName(rawURL string) (string, bool, error) {
	withoutScheme := rawURL
	if i := strings.Index(withoutScheme, "://"); i >= 0 {
		withoutScheme = withoutScheme[i+3:]
	}
	host := withoutScheme
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i+1:], "]") {
		host = host[:i]
	}
	host = strings.Trim(strings.ToLower(host), "[] .")
	if host == "" {
		return "", false, fmt.Errorf("invalid URL %q: missing host", rawURL)
	}
	host = strings.TrimPrefix(host, "mcp.")
	label := strings.Split(host, ".")[0]
	if label == "" {
		return "", false, fmt.Errorf("could not infer a server name from %q", rawURL)
	}
	return sanitizeName(label), genericNames[label], nil
}

func ResolveName(rawURL string, explicit string, yes bool, confirm func(string) (bool, error)) (string, error) {
	if explicit != "" {
		name := sanitizeName(explicit)
		if name == "" {
			return "", fmt.Errorf("server name %q is invalid after sanitizing; provide a name with letters or numbers", explicit)
		}
		return name, nil
	}
	name, ambiguous, err := InferName(rawURL)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("could not infer a usable server name from %q", rawURL)
	}
	if !ambiguous {
		return name, nil
	}
	if yes {
		return "", fmt.Errorf("inferred server name %q is ambiguous; rerun with --name", name)
	}
	if confirm == nil {
		return "", fmt.Errorf("inferred server name %q is ambiguous; rerun with --name", name)
	}
	ok, err := confirm(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("server name %q was not confirmed; rerun with --name", name)
	}
	return name, nil
}

func InstallRemote(opts Options) (Result, error) {
	normalizedURL, err := ValidateRemoteInstall(opts.URL, opts.Headers)
	if err != nil {
		return Result{}, err
	}
	opts.URL = normalizedURL
	if opts.Name == "" {
		name, ambiguous, err := InferName(opts.URL)
		if err != nil {
			return Result{}, err
		}
		if ambiguous && (opts.NonTTY || opts.Yes || len(opts.Clients) > 0) {
			return Result{}, fmt.Errorf("inferred server name %q is ambiguous; rerun with --name", name)
		}
	}
	name, err := ResolveName(opts.URL, opts.Name, opts.Yes, opts.ConfirmName)
	if err != nil {
		return Result{}, err
	}
	if opts.Forget {
		if err := ForgetPreferences(opts.Env); err != nil {
			return Result{}, err
		}
	}
	targets, interactive, err := resolveTargets(opts)
	if err != nil {
		return Result{}, err
	}
	res := Result{Name: name, URL: opts.URL, CodexEnvVar: CodexTokenEnvVar(name)}
	if len(opts.Clients) == 0 {
		if codexDetected(opts.Env) {
			res.Manual = append(res.Manual, codexManualInstructions(name, opts.URL, res.CodexEnvVar))
		}
	}
	if len(targets) == 0 {
		return res, fmt.Errorf("no detected file-writer harnesses selected")
	}
	for _, target := range targets {
		adapter, ok := AdapterByClientID(target.Client.ID)
		if !ok {
			if target.Client.ID == "codex" {
				res.Manual = append(res.Manual, codexManualInstructions(name, opts.URL, res.CodexEnvVar))
				res.Failures = append(res.Failures, "Codex: hitch cannot configure Codex automatically yet")
				continue
			}
			res.Failures = append(res.Failures, fmt.Sprintf("%s: unsupported client", target.Client.Name))
			continue
		}
		entry := adapter.BuildRemoteEntry(opts.URL, opts.Headers, res.CodexEnvVar)
		if opts.DryRun {
			if err := ValidateEntry(target.Path, adapter, name, entry); err != nil {
				res.Failures = append(res.Failures, fmt.Sprintf("%s: %v", target.Client.Name, err))
				continue
			}
			res.WouldWrite = append(res.WouldWrite, target.Path)
			if opts.Stdout != nil {
				_, _ = fmt.Fprintf(opts.Stdout, "Would write %s to %s:\n%s\n", target.Client.Name, target.Path, MaskedJSON(entry))
			}
			continue
		}
		if err := WriteEntry(target.Path, adapter, name, entry); err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("%s: %v", target.Client.Name, err))
			continue
		}
		res.Written = append(res.Written, target.Path)
		res.WrittenInfo = append(res.WrittenInfo, WriteInfo{ClientName: target.Client.Name, Path: target.Path})
	}
	if interactive && len(res.Written) > 0 && len(res.Failures) == 0 && !opts.DryRun {
		ids := make([]string, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.Client.ID)
		}
		if err := SavePreferences(opts.Env, ids); err != nil {
			return res, err
		}
	}
	if len(res.Failures) > 0 {
		return res, fmt.Errorf("some harnesses were not configured: %s", strings.Join(res.Failures, "; "))
	}
	return res, nil
}

func InstallStdio(opts Options) (Result, error) {
	name := sanitizeName(opts.Name)
	if name == "" {
		return Result{}, fmt.Errorf("stdio install requires a non-empty server name")
	}
	if strings.TrimSpace(opts.Command) == "" {
		return Result{}, fmt.Errorf("stdio install requires --command")
	}
	if opts.Forget {
		if err := ForgetPreferences(opts.Env); err != nil {
			return Result{}, err
		}
	}
	targets, interactive, err := resolveTargets(opts)
	if err != nil {
		return Result{}, err
	}
	res := Result{Name: name, URL: opts.Command}
	if len(opts.Clients) == 0 && codexDetected(opts.Env) {
		res.Manual = append(res.Manual, codexStdioManualInstructions(name, opts.Command, opts.Args, opts.StdioEnv))
	}
	if len(targets) == 0 {
		return res, fmt.Errorf("no detected file-writer harnesses selected")
	}
	for _, target := range targets {
		adapter, ok := AdapterByClientID(target.Client.ID)
		if !ok {
			if target.Client.ID == "codex" {
				res.Manual = append(res.Manual, codexStdioManualInstructions(name, opts.Command, opts.Args, opts.StdioEnv))
				res.Failures = append(res.Failures, "Codex: hitch cannot configure Codex automatically yet")
				continue
			}
			res.Failures = append(res.Failures, fmt.Sprintf("%s: unsupported client", target.Client.Name))
			continue
		}
		entry := adapter.BuildStdioEntry(opts.Command, opts.Args, opts.StdioEnv)
		if opts.DryRun {
			if err := ValidateEntry(target.Path, adapter, name, entry); err != nil {
				res.Failures = append(res.Failures, fmt.Sprintf("%s: %v", target.Client.Name, err))
				continue
			}
			res.WouldWrite = append(res.WouldWrite, target.Path)
			if opts.Stdout != nil {
				_, _ = fmt.Fprintf(opts.Stdout, "Would write %s to %s:\n%s\n", target.Client.Name, target.Path, MaskedJSON(entry))
			}
			continue
		}
		if err := WriteEntry(target.Path, adapter, name, entry); err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("%s: %v", target.Client.Name, err))
			continue
		}
		res.Written = append(res.Written, target.Path)
		res.WrittenInfo = append(res.WrittenInfo, WriteInfo{ClientName: target.Client.Name, Path: target.Path})
	}
	if interactive && len(res.Written) > 0 && len(res.Failures) == 0 && !opts.DryRun {
		ids := make([]string, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.Client.ID)
		}
		if err := SavePreferences(opts.Env, ids); err != nil {
			return res, err
		}
	}
	if len(res.Failures) > 0 {
		return res, fmt.Errorf("some harnesses were not configured: %s", strings.Join(res.Failures, "; "))
	}
	return res, nil
}

func ValidateRemoteInstall(rawURL string, headers map[string]string) (string, error) {
	normalizedURL, parsed, err := normalizeRemoteURL(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("install URL must be absolute with http or https scheme and non-empty host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("install URL must use http or https scheme")
	}
	if parsed.Scheme == "http" && len(headers) > 0 && !isLocalhost(parsed.Hostname()) {
		return "", fmt.Errorf("refusing to send credentials over insecure http URL; use https or localhost")
	}
	return normalizedURL, nil
}

func NormalizeRemoteURL(rawURL string) (string, error) {
	normalizedURL, _, err := normalizeRemoteURL(rawURL)
	return normalizedURL, err
}

func normalizeRemoteURL(rawURL string) (string, *url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", nil, err
	}
	return parsed.String(), parsed, nil
}

func isLocalhost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func codexManualInstructions(name string, url string, envVar string) string {
	return fmt.Sprintf("hitch cannot configure Codex automatically yet. Add this to Codex config manually:\n[mcp_servers.%s]\nurl = %q\nbearer_token_env_var = %q\n\nBefore starting Codex, run:\nexport %s=YOUR_TOKEN", name, url, envVar, envVar)
}

func codexStdioManualInstructions(name string, command string, args []string, env map[string]string) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "hitch cannot configure Codex automatically yet. Add this to Codex config manually:\n[mcp_servers.%s]\ncommand = %q\n", name, command)
	if len(args) > 0 {
		encoded, _ := json.Marshal(args)
		_, _ = fmt.Fprintf(&b, "args = %s\n", encoded)
	}
	for key := range env {
		_, _ = fmt.Fprintf(&b, "# Set %s in the Codex server environment.\n", key)
	}
	return b.String()
}

func resolveTargets(opts Options) ([]Target, bool, error) {
	results, err := harness.Detect(opts.Env)
	if err != nil {
		return nil, false, err
	}
	byID := map[string]harness.DetectionResult{}
	detected := []Target{}
	for _, result := range results {
		if result.PromptTier {
			continue
		}
		byID[result.ID] = result
		if result.Detected {
			if result.ID == "codex" {
				continue
			}
			detected = append(detected, Target{Client: result, Path: result.ConfigPath})
		}
	}
	if len(opts.Clients) > 0 {
		targets := make([]Target, 0, len(opts.Clients))
		for _, id := range opts.Clients {
			result, ok := byID[id]
			if !ok {
				return nil, false, fmt.Errorf("unknown file-writer client %q", id)
			}
			targets = append(targets, Target{Client: result, Path: result.ConfigPath})
		}
		return targets, false, nil
	}
	if opts.Yes {
		return detected, false, nil
	}
	if opts.NonTTY {
		return nil, false, fmt.Errorf("non-TTY install requires either -y/--yes or -c/--client")
	}
	preferred, err := LoadPreferences(opts.Env)
	if err != nil {
		return nil, false, err
	}
	if opts.PickTargets == nil {
		return nil, false, fmt.Errorf("interactive picker is unavailable")
	}
	chosen, err := opts.PickTargets(detected, preferred)
	return chosen, true, err
}

func codexDetected(env harness.Env) bool {
	results, err := harness.Detect(env)
	if err != nil {
		return false
	}
	for _, result := range results {
		if result.ID == "codex" && result.Detected {
			return true
		}
	}
	return false
}

func CodexTokenEnvVar(name string) string {
	upper := strings.ToUpper(sanitizeName(name))
	upper = regexp.MustCompile(`[^A-Z0-9]+`).ReplaceAllString(upper, "_")
	upper = strings.Trim(upper, "_")
	if upper == "" {
		upper = "MCP"
	}
	return codexTokenPrefix + upper
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-_")
	return name
}

func WriteEntry(path string, adapter Adapter, name string, entry map[string]any) error {
	content, err := buildJSONEntry(path, adapter.ConfigKey, adapter.ClientID == "zed", name, entry)
	if err != nil {
		return err
	}
	return writeAtomic(path, content)
}

func ValidateEntry(path string, adapter Adapter, name string, entry map[string]any) error {
	_, err := buildJSONEntry(path, adapter.ConfigKey, adapter.ClientID == "zed", name, entry)
	return err
}

func buildJSONEntry(path string, key string, zedHint bool, name string, entry map[string]any) ([]byte, error) {
	raw, existed, err := readExisting(path)
	if err != nil {
		return nil, err
	}
	if !existed || len(bytes.TrimSpace(raw)) == 0 {
		return jsonConfigWithEntry(key, name, entry)
	}
	if err := rejectDuplicateTopLevelKeys(raw, path); err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.New(jsonConfigError(path, zedHint, err))
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("existing config %s is not a JSON object", path)
	}
	if servers, ok := object[key]; ok {
		if _, ok := servers.(map[string]any); !ok {
			return nil, fmt.Errorf("existing config %s has non-object %q", path, key)
		}
	}
	updated, err := setJSONObjectEntry(raw, key, name, entry)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func jsonConfigError(path string, zedHint bool, err error) string {
	hint := ""
	if zedHint {
		hint = " Zed settings may contain JSONC comments; remove comments before retrying."
	}
	return fmt.Sprintf("existing config %s is not valid JSON (%v); fix or remove it, then retry.%s", path, err, hint)
}

func rejectDuplicateTopLevelKeys(raw []byte, path string) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil
	}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil
		}
		if seen[key] {
			return fmt.Errorf("existing config %s has duplicate top-level key %q", path, key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil
		}
	}
	return nil
}

func jsonConfigWithEntry(key string, name string, entry map[string]any) ([]byte, error) {
	root := map[string]any{key: map[string]any{name: entry}}
	return marshalJSON(root)
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func setJSONObjectEntry(raw []byte, key string, name string, entry map[string]any) ([]byte, error) {
	keyRange, found, err := findTopLevelObjectValue(raw, key)
	if err != nil {
		return nil, err
	}
	entryBytes, err := marshalJSON(map[string]any{name: entry})
	if err != nil {
		return nil, err
	}
	entryObject := bytes.TrimSpace(entryBytes)
	if !found {
		return insertTopLevelKey(raw, key, entryObject)
	}
	serverRange, serverFound, err := findTopLevelObjectValueInRange(raw, name, keyRange)
	if err != nil {
		return nil, err
	}
	serverBytes, err := marshalJSON(entry)
	if err != nil {
		return nil, err
	}
	serverObject := bytes.TrimSpace(serverBytes)
	if serverFound {
		return replaceRange(raw, serverRange.start, serverRange.end, indentMultiline(serverObject, valueContinuationIndent(raw, serverRange.start))), nil
	}
	return insertIntoObject(raw, keyRange.start, keyRange.end, name, serverObject)
}

func RemoveEntry(path string, adapter Adapter, name string) (bool, error) {
	content, changed, err := buildJSONRemoval(path, adapter.ConfigKey, adapter.ClientID == "zed", name)
	if err != nil || !changed {
		return changed, err
	}
	return true, writeAtomic(path, content)
}

func buildJSONRemoval(path string, key string, zedHint bool, name string) ([]byte, bool, error) {
	raw, existed, err := readExisting(path)
	if err != nil || !existed || len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, err
	}
	if err := rejectDuplicateTopLevelKeys(raw, path); err != nil {
		return nil, false, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false, errors.New(jsonConfigError(path, zedHint, err))
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("existing config %s is not a JSON object", path)
	}
	if servers, ok := object[key]; ok {
		if _, ok := servers.(map[string]any); !ok {
			return nil, false, fmt.Errorf("existing config %s has non-object %q", path, key)
		}
	}
	keyRange, found, err := findTopLevelObjectValue(raw, key)
	if err != nil || !found {
		return nil, false, err
	}
	serverRange, serverFound, err := findTopLevelObjectMemberInRange(raw, name, keyRange)
	if err != nil || !serverFound {
		return nil, false, err
	}
	updated, err := removeObjectMember(raw, keyRange, serverRange)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

type byteRange struct{ start, end int }

func findTopLevelObjectValue(raw []byte, key string) (byteRange, bool, error) {
	return findTopLevelObjectValueInRange(raw, key, byteRange{start: 0, end: len(raw)})
}

func findTopLevelObjectValueInRange(raw []byte, key string, bounds byteRange) (byteRange, bool, error) {
	if bounds.start < 0 || bounds.end > len(raw) || bounds.start >= bounds.end {
		return byteRange{}, false, errors.New("invalid JSON object range")
	}
	base := bounds.start
	section := raw[bounds.start:bounds.end]
	span, found, err := findTopLevelObjectValueInSection(section, key)
	if err != nil || !found {
		return byteRange{}, found, err
	}
	return byteRange{start: base + span.start, end: base + span.end}, true, nil
}

func holdsCredential(v any) bool {
	switch typed := v.(type) {
	case map[string]any:
		for key, value := range typed {
			if credentialContainerKey(key) && mapHasStringValue(value) {
				return true
			}
			if holdsCredential(value) {
				return true
			}
		}
	case []any:
		for _, value := range typed {
			if holdsCredential(value) {
				return true
			}
		}
	}
	return false
}

func credentialContainerKey(key string) bool {
	return strings.EqualFold(key, "headers") || strings.EqualFold(key, "env") || strings.EqualFold(key, "environment")
}

func mapHasStringValue(v any) bool {
	switch typed := v.(type) {
	case map[string]any:
		for _, value := range typed {
			if s, ok := value.(string); ok && s != "" {
				return true
			}
		}
	case map[string]string:
		for _, value := range typed {
			if value != "" {
				return true
			}
		}
	}
	return false
}

func findTopLevelObjectMemberInRange(raw []byte, key string, bounds byteRange) (byteRange, bool, error) {
	if bounds.start < 0 || bounds.end > len(raw) || bounds.start >= bounds.end {
		return byteRange{}, false, errors.New("invalid JSON object range")
	}
	base := bounds.start
	section := raw[bounds.start:bounds.end]
	span, found, err := findTopLevelObjectMemberInSection(section, key)
	if err != nil || !found {
		return byteRange{}, found, err
	}
	return byteRange{start: base + span.start, end: base + span.end}, true, nil
}

func findTopLevelObjectMemberInSection(raw []byte, key string) (byteRange, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return byteRange{}, false, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return byteRange{}, false, errors.New("existing config is not a JSON object")
	}
	for dec.More() {
		beforeKey := int(dec.InputOffset())
		keyTok, err := dec.Token()
		if err != nil {
			return byteRange{}, false, err
		}
		keyString, ok := keyTok.(string)
		if !ok {
			return byteRange{}, false, errors.New("JSON object key is not a string")
		}
		keyStart, err := nextJSONStringStart(raw, beforeKey)
		if err != nil {
			return byteRange{}, false, err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return byteRange{}, false, err
		}
		valueEnd := int(dec.InputOffset())
		if keyString == key {
			return byteRange{start: keyStart, end: valueEnd}, true, nil
		}
	}
	if _, err := dec.Token(); err != nil {
		return byteRange{}, false, err
	}
	return byteRange{}, false, nil
}

func nextJSONStringStart(raw []byte, offset int) (int, error) {
	for i := offset; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\n', '\r', '\t', ',', '{':
			continue
		case '"':
			return i, nil
		default:
			return 0, errors.New("could not locate JSON object key start")
		}
	}
	return 0, errors.New("could not locate JSON object key start")
}

func removeObjectMember(raw []byte, objectRange byteRange, memberRange byteRange) ([]byte, error) {
	if objectRange.start < 0 || objectRange.end > len(raw) || memberRange.start < objectRange.start || memberRange.end > objectRange.end {
		return nil, errors.New("invalid JSON removal range")
	}
	before := memberRange.start
	for before > objectRange.start+1 && isSpace(raw[before-1]) {
		before--
	}
	if before > objectRange.start+1 && raw[before-1] == ',' {
		return replaceRange(raw, before-1, memberRange.end, nil), nil
	}
	after := memberRange.end
	for after < objectRange.end-1 && isSpace(raw[after]) {
		after++
	}
	if after < objectRange.end-1 && raw[after] == ',' {
		return replaceRange(raw, memberRange.start, after+1, nil), nil
	}
	return replaceRange(raw, memberRange.start, memberRange.end, nil), nil
}

func findTopLevelObjectValueInSection(raw []byte, key string) (byteRange, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return byteRange{}, false, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return byteRange{}, false, errors.New("existing config is not a JSON object")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return byteRange{}, false, err
		}
		keyString, ok := keyTok.(string)
		if !ok {
			return byteRange{}, false, errors.New("top-level JSON object key is not a string")
		}
		keyEnd := int(dec.InputOffset())
		valueStart, err := jsonValueStart(raw, keyEnd)
		if err != nil {
			return byteRange{}, false, err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return byteRange{}, false, err
		}
		valueEnd := int(dec.InputOffset())
		if keyString != key {
			continue
		}
		if len(value) == 0 || value[0] != '{' {
			return byteRange{}, false, fmt.Errorf("value for %q is not an object", key)
		}
		return byteRange{start: valueStart, end: valueEnd}, true, nil
	}
	if _, err := dec.Token(); err != nil {
		return byteRange{}, false, err
	}
	return byteRange{}, false, nil
}

func jsonValueStart(raw []byte, keyEnd int) (int, error) {
	i := keyEnd
	for i < len(raw) && isSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != ':' {
		return 0, errors.New("expected ':' after JSON object key")
	}
	i++
	for i < len(raw) && isSpace(raw[i]) {
		i++
	}
	if i >= len(raw) {
		return 0, errors.New("expected JSON value after object key")
	}
	return i, nil
}

func matchingBrace(raw []byte, open int) (int, error) {
	depth := 0
	inString := false
	escaped := false
	for i := open; i < len(raw); i++ {
		c := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, errors.New("could not find matching JSON object brace")
}

func insertTopLevelKey(raw []byte, key string, value []byte) ([]byte, error) {
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		return nil, errors.New("existing config is not a JSON object")
	}
	end, err := matchingBrace(raw, start)
	if err != nil {
		return nil, err
	}
	return insertIntoObject(raw, start, end+1, key, value)
}

func insertIntoObject(raw []byte, start int, end int, key string, value []byte) ([]byte, error) {
	inner := bytes.TrimSpace(raw[start+1 : end-1])
	var insertion []byte
	keyBytes, _ := json.Marshal(key)
	memberIndent := objectMemberIndent(raw, start, end)
	formattedValue := indentMultiline(value, memberIndent)
	if len(inner) == 0 {
		insertion = append([]byte("\n"+memberIndent), keyBytes...)
		insertion = append(insertion, []byte(": ")...)
		insertion = append(insertion, formattedValue...)
		insertion = append(insertion, []byte("\n"+parentIndent(raw, start))...)
		return replaceRange(raw, start+1, end-1, insertion), nil
	}
	insertAt := end - 1
	for insertAt > start && isSpace(raw[insertAt-1]) {
		insertAt--
	}
	insertion = append([]byte(",\n"+memberIndent), keyBytes...)
	insertion = append(insertion, []byte(": ")...)
	insertion = append(insertion, formattedValue...)
	return replaceRange(raw, insertAt, insertAt, insertion), nil
}

func objectMemberIndent(raw []byte, start int, end int) string {
	for i := start + 1; i < end-1; i++ {
		if raw[i] == '\n' || raw[i] == '\r' {
			j := i + 1
			spaces := 0
			for j+spaces < end-1 && raw[j+spaces] == ' ' {
				spaces++
			}
			if j+spaces < end-1 && raw[j+spaces] != '\n' && raw[j+spaces] != '\r' {
				return strings.Repeat(" ", spaces)
			}
		}
	}
	return parentIndent(raw, start) + "  "
}

func parentIndent(raw []byte, pos int) string {
	lineStart := pos
	for lineStart > 0 && raw[lineStart-1] != '\n' && raw[lineStart-1] != '\r' {
		lineStart--
	}
	spaces := 0
	for lineStart+spaces < len(raw) && raw[lineStart+spaces] == ' ' {
		spaces++
	}
	return strings.Repeat(" ", spaces)
}

func indentMultiline(value []byte, prefix string) []byte {
	lines := bytes.Split(value, []byte("\n"))
	for i := 1; i < len(lines); i++ {
		if len(lines[i]) > 0 {
			lines[i] = append([]byte(prefix), lines[i]...)
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

func valueContinuationIndent(raw []byte, pos int) string {
	lineStart := pos
	for lineStart > 0 && raw[lineStart-1] != '\n' && raw[lineStart-1] != '\r' {
		lineStart--
	}
	spaces := 0
	for lineStart+spaces < len(raw) && raw[lineStart+spaces] == ' ' {
		spaces++
	}
	return strings.Repeat(" ", spaces)
}

func replaceRange(raw []byte, start int, end int, replacement []byte) []byte {
	out := make([]byte, 0, len(raw)-end+start+len(replacement))
	out = append(out, raw[:start]...)
	out = append(out, replacement...)
	out = append(out, raw[end:]...)
	return out
}

func isSpace(c byte) bool { return c == ' ' || c == '\n' || c == '\r' || c == '\t' }

func readExisting(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		return raw, true, nil
	}
	if os.IsNotExist(err) {
		if info, lstatErr := os.Lstat(path); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("config path %s is a dangling symlink; fix or remove it, then retry", path)
		}
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("could not read %s: %w", path, err)
}

func writeAtomic(path string, content []byte) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf(".%s.%d.tmp", filepath.Base(target), os.Getpid())), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if writeAtomicBeforeRename != nil {
		if err := writeAtomicBeforeRename(tmpName); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	return os.Chmod(target, 0o600)
}

var writeAtomicBeforeRename func(string) error

func MaskedJSON(entry map[string]any) string {
	masked := maskValue(entry).(map[string]any)
	out, err := marshalJSON(masked)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func maskValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range typed {
			if strings.EqualFold(k, "headers") || strings.EqualFold(k, "env") || strings.EqualFold(k, "environment") {
				out[k] = maskHeaderMap(val)
			} else {
				out[k] = maskValue(val)
			}
		}
		return out
	case map[string]string:
		return maskHeaderMap(typed)
	default:
		return v
	}
}

func maskHeaderMap(v any) any {
	switch typed := v.(type) {
	case map[string]string:
		out := map[string]any{}
		for k := range typed {
			out[k] = "***"
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k := range typed {
			out[k] = "***"
		}
		return out
	default:
		return v
	}
}

func LoadPreferences(env harness.Env) (map[string]bool, error) {
	path, err := preferencesPath(env)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pref struct {
		Clients []string `json:"clients"`
	}
	if err := json.Unmarshal(raw, &pref); err != nil {
		return nil, nil
	}
	out := map[string]bool{}
	for _, id := range pref.Clients {
		out[id] = true
	}
	return out, nil
}

func SavePreferences(env harness.Env, clients []string) error {
	sorted := append([]string{}, clients...)
	sort.Strings(sorted)
	content, err := marshalJSON(struct {
		Clients []string `json:"clients"`
	}{Clients: sorted})
	if err != nil {
		return err
	}
	path, err := preferencesPath(env)
	if err != nil {
		return err
	}
	return writeAtomic(path, content)
}

func ForgetPreferences(env harness.Env) error {
	path, pathErr := preferencesPath(env)
	if pathErr != nil {
		return pathErr
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func preferencesPath(env harness.Env) (string, error) {
	base := env.XDGConfigHome
	source := "XDG_CONFIG_HOME"
	if base == "" {
		base = filepath.Join(env.Home, ".config")
		source = "HOME or USERPROFILE"
	}
	path := filepath.Join(base, "hitch", "preferences.json")
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("config path must be absolute; check %s", source)
	}
	return path, nil
}
