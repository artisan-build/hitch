package oauthregistry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/artisan-build/hitch/internal/harness"
)

const Version = 1

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

type Record struct {
	Scope      Scope  `json:"scope"`
	ClientID   string `json:"client_id"`
	ConfigPath string `json:"config_path"`
	ServerName string `json:"server_name"`
	ServerURL  string `json:"server_url"`
}

type Identity struct {
	Scope      Scope
	ClientID   string
	ConfigPath string
	ServerName string
}

type file struct {
	Version       int      `json:"version"`
	Installations []Record `json:"installations"`
}

func Path(env harness.Env) (string, error) {
	base := env.XDGConfigHome
	if base == "" {
		base = filepath.Join(env.Home, ".config")
	}
	if base == "" || !filepath.IsAbs(base) {
		return "", fmt.Errorf("OAuth registry path must be absolute; check XDG_CONFIG_HOME or HOME")
	}
	return filepath.Join(base, "hitch", "oauth-installations.json"), nil
}

// SanitizeURL removes URL material that must never be persisted as identity
// metadata. Queries are stripped in full because arbitrary parameter names can
// carry credentials.
func SanitizeURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("OAuth server URL must be absolute with a non-empty host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("OAuth server URL must use http or https scheme")
	}
	parsed.User = nil
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	return parsed.String(), nil
}

func Origin(raw string) (string, error) {
	sanitized, err := SanitizeURL(raw)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(sanitized)
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String(), nil
}

func Load(env harness.Env) ([]Record, error) {
	path, err := Path(env)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read OAuth installation registry %s: %w", path, err)
	}
	var registry file
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("OAuth installation registry %s is malformed; fix or remove it before retrying: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("OAuth installation registry %s is malformed; fix or remove it before retrying", path)
	}
	if registry.Version != Version {
		return nil, fmt.Errorf("OAuth installation registry %s has unsupported version %d", path, registry.Version)
	}
	seen := map[string]bool{}
	for _, record := range registry.Installations {
		if err := validateRecord(record); err != nil {
			return nil, fmt.Errorf("OAuth installation registry %s is malformed; fix or remove it before retrying: %w", path, err)
		}
		key := identityKey(record)
		if seen[key] {
			return nil, fmt.Errorf("OAuth installation registry %s is malformed; duplicate installation identity", path)
		}
		seen[key] = true
	}
	return registry.Installations, nil
}

func Upsert(env harness.Env, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	existing, err := Load(env)
	if err != nil {
		return err
	}
	byIdentity := make(map[string]Record, len(existing)+len(records))
	for _, record := range existing {
		byIdentity[identityKey(record)] = record
	}
	for _, record := range records {
		if err := validateRecord(record); err != nil {
			return err
		}
		byIdentity[identityKey(record)] = record
	}
	updated := make([]Record, 0, len(byIdentity))
	for _, record := range byIdentity {
		updated = append(updated, record)
	}
	sortRecords(updated)
	return save(env, updated)
}

// RemoveIdentities removes records regardless of their recorded URL. It is
// used after hitch successfully replaces an OAuth entry with a non-OAuth
// entry, where retaining the old identity could later misclassify a manually
// recreated URL-only entry as OAuth.
func RemoveIdentities(env harness.Env, identities []Identity) error {
	if len(identities) == 0 {
		return nil
	}
	existing, err := Load(env)
	if err != nil {
		return err
	}
	remove := make(map[string]bool, len(identities))
	for _, identity := range identities {
		if err := validateIdentity(identity); err != nil {
			return err
		}
		remove[identityKey(identity)] = true
	}
	kept := make([]Record, 0, len(existing))
	for _, record := range existing {
		if !remove[identityKey(record)] {
			kept = append(kept, record)
		}
	}
	if len(kept) == len(existing) {
		return nil
	}
	return save(env, kept)
}

func Matches(records []Record, candidate Record) bool {
	for _, record := range records {
		if record == candidate {
			return true
		}
	}
	return false
}

func validateRecord(record Record) error {
	if err := validateIdentity(identityFromRecord(record)); err != nil {
		return err
	}
	sanitized, err := SanitizeURL(record.ServerURL)
	if err != nil || sanitized != record.ServerURL {
		return fmt.Errorf("OAuth installation URL is not sanitized")
	}
	return nil
}

func validateIdentity(identity Identity) error {
	if identity.Scope != ScopeGlobal && identity.Scope != ScopeProject {
		return fmt.Errorf("invalid OAuth installation scope")
	}
	if strings.TrimSpace(identity.ClientID) == "" || strings.TrimSpace(identity.ServerName) == "" {
		return fmt.Errorf("OAuth installation identity is incomplete")
	}
	if identity.ConfigPath == "" || !filepath.IsAbs(identity.ConfigPath) {
		return fmt.Errorf("OAuth installation config path is not absolute")
	}
	return nil
}

func identityFromRecord(record Record) Identity {
	return Identity{Scope: record.Scope, ClientID: record.ClientID, ConfigPath: record.ConfigPath, ServerName: record.ServerName}
}

type installationIdentity interface {
	Record | Identity
}

func identityKey[T installationIdentity](value T) string {
	var identity Identity
	switch typed := any(value).(type) {
	case Record:
		identity = identityFromRecord(typed)
	case Identity:
		identity = typed
	}
	return strings.Join([]string{string(identity.Scope), identity.ClientID, identity.ConfigPath, identity.ServerName}, "\x00")
}

func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		return identityKey(records[i]) < identityKey(records[j])
	})
}

func save(env harness.Env, records []Record) error {
	path, err := Path(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create OAuth registry directory: %w", err)
	}
	raw, err := json.MarshalIndent(file{Version: Version, Installations: records}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OAuth installation registry: %w", err)
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".oauth-installations.tmp-")
	if err != nil {
		return fmt.Errorf("create OAuth registry temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure OAuth registry temporary file: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write OAuth installation registry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync OAuth installation registry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close OAuth installation registry: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace OAuth installation registry: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure OAuth installation registry: %w", err)
	}
	return nil
}
