package cmd

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/artisan-build/hitch/internal/harness"
)

const installURL = "https://app.example.test/mcp"

// startClaimRecorder counts every hit on any path so tests can assert exact
// request counts — including zero — rather than inferring from side effects.
func startClaimRecorder(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if handler != nil {
			handler(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

func claimJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// treeBytes maps every file under root to its exact contents, so a "nothing
// changed" assertion covers bytes, deletions, and stray temp files alike.
func treeBytes(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out[rel] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
	return out
}

func runMain(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Main(args, &stdout, &stderr, func() (harness.Env, error) { return testEnv(home), nil })
	return code, stdout.String(), stderr.String()
}

func TestInstallClaimSuccessInstallsClaimedTokenExactlyOnce(t *testing.T) {
	const token = "tok_SENTINEL_claim_success"
	server, hits := startClaimRecorder(t, claimJSON(http.StatusOK, `{"version":1,"token":"`+token+`","name":"ballast"}`))
	home := t.TempDir()

	code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if n := atomic.LoadInt32(hits); n != 1 {
		t.Fatalf("claim requests = %d, want exactly 1", n)
	}
	servers := cursorServers(t, home)
	entry, ok := servers["ballast"].(map[string]any)
	if !ok {
		t.Fatalf("cursor servers = %#v, want a %q entry named by the claim response", servers, "ballast")
	}
	if entry["url"] != installURL {
		t.Fatalf("installed url = %q, want %q", entry["url"], installURL)
	}
	if got := entry["headers"].(map[string]any)["Authorization"]; got != "Bearer "+token {
		t.Fatalf("Authorization = %q, want the claimed token", got)
	}
	if strings.Contains(stdout+stderr, token) {
		t.Fatalf("claimed token leaked to output; stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, `Configured Cursor "ballast" → `+installURL) {
		t.Fatalf("stdout missing success line: %q", stdout)
	}
}

func TestInstallClaimFailureLeavesSelectedConfigsByteIdentical(t *testing.T) {
	for _, tt := range []struct {
		name     string
		body     string
		status   int
		wantCode int
		wantErr  string
	}{
		{name: "code_expired is operational failure", status: http.StatusGone, body: `{"version":1,"error":"code_expired","message":"Ask for a new one-liner."}`, wantCode: 1, wantErr: "Ask for a new one-liner."},
		{name: "invalid_code is misuse", status: http.StatusBadRequest, body: `{"version":1,"error":"invalid_code"}`, wantCode: 2, wantErr: "typo"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, hits := startClaimRecorder(t, claimJSON(tt.status, tt.body))
			home := t.TempDir()
			writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{\n  \"mcpServers\": {\n    \"existing\": {\n      \"url\": \"https://old.example.test/mcp\",\n      \"headers\": {}\n    }\n  }\n}\n", 0o600)
			writeFile(t, filepath.Join(home, ".config", "zed", "settings.json"), "{\n  \"context_servers\": {}\n}\n", 0o600)
			writeFile(t, filepath.Join(home, ".claude.json"), "{\n  \"mcpServers\": {}\n}\n", 0o600)
			before := treeBytes(t, home)

			code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor", "--client", "zed", "--client", "claude-code", "--name", "ballast")
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout, stderr)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Fatalf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
			if n := atomic.LoadInt32(hits); n != 1 {
				t.Fatalf("claim requests = %d, want 1 (the exchange must actually have been attempted)", n)
			}
			after := treeBytes(t, home)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("configs changed after a failed claim:\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestInstallClaimDryRunMakesZeroClaimRequests(t *testing.T) {
	const token = "tok_SENTINEL_dry_run"
	success := claimJSON(http.StatusOK, `{"version":1,"token":"`+token+`","name":"ballast"}`)

	t.Run("dry run hits the claim URL zero times", func(t *testing.T) {
		server, hits := startClaimRecorder(t, success)
		home := t.TempDir()
		code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor", "--dry-run")
		if code != 0 {
			t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Fatalf("claim requests = %d, want ZERO", n)
		}
		for _, want := range []string{
			"Dry run: no request was made to the claim URL; the claim code was not spent and is still valid.",
			"Dry run: the final server name is not known yet",
			"<token from claim, not yet requested>",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout missing %q:\n%s", want, stdout)
			}
		}
		if strings.Contains(stdout+stderr, token) {
			t.Fatalf("dry run output contains the server's token; stdout=%q stderr=%q", stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
			t.Fatalf("dry run wrote config, stat err = %v", err)
		}
	})

	t.Run("positive control: the same command without --dry-run claims once", func(t *testing.T) {
		server, hits := startClaimRecorder(t, success)
		home := t.TempDir()
		code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor")
		if code != 0 {
			t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
		}
		if n := atomic.LoadInt32(hits); n != 1 {
			t.Fatalf("claim requests = %d, want 1", n)
		}
	})

	t.Run("an explicit --name suppresses the unknown-name note", func(t *testing.T) {
		server, hits := startClaimRecorder(t, success)
		home := t.TempDir()
		code, stdout, _ := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor", "--dry-run", "--name", "pinned")
		if code != 0 {
			t.Fatalf("exit code = %d; stdout=%q", code, stdout)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Fatalf("claim requests = %d, want ZERO", n)
		}
		if !strings.Contains(stdout, "was not spent and is still valid") {
			t.Fatalf("stdout missing the code-still-valid note: %q", stdout)
		}
		if strings.Contains(stdout, "final server name is not known") {
			t.Fatalf("stdout has the unknown-name note despite --name: %q", stdout)
		}
	})
}

func TestInstallClaimOddServerResponsesNeverEchoToken(t *testing.T) {
	const sentinel = "tok_SENTINEL_odd_body"
	for _, tt := range []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     string
		wantAlt     bool
	}{
		{name: "html 200", status: http.StatusOK, contentType: "text/html", body: "<html>" + sentinel + "</html>", wantErr: "does not speak the hitch claim contract", wantAlt: true},
		{name: "truncated json", status: http.StatusOK, contentType: "application/json", body: `{"token":"` + sentinel + `"`, wantErr: "does not speak the hitch claim contract", wantAlt: true},
		{name: "token with 503", status: http.StatusServiceUnavailable, contentType: "application/json", body: `{"version":1,"token":"` + sentinel + `"}`, wantErr: "does not speak the hitch claim contract", wantAlt: true},
		{name: "json without token", status: http.StatusOK, contentType: "application/json", body: `{"version":1,"hint":"` + sentinel + `"}`, wantErr: `JSON without a "token" field`, wantAlt: true},
		{name: "newer contract version", status: http.StatusOK, contentType: "application/json", body: `{"version":2,"token":"` + sentinel + `"}`, wantErr: "newer than this hitch understands"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := startClaimRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			home := t.TempDir()
			code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor")
			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if strings.Contains(stdout+stderr, sentinel) {
				t.Fatalf("output leaked the response body; stdout=%q stderr=%q", stdout, stderr)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Fatalf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
			if tt.wantAlt && !strings.Contains(stderr, "hitch install "+installURL+" --token-stdin") {
				t.Fatalf("stderr missing the history-safe alternative: %q", stderr)
			}
			if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("config was written, stat err = %v", err)
			}
		})
	}
}

func TestInstallClaimBranchesOnErrorEnumNotHTTPStatus(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   int
		body     string
		wantCode int
		wantErr  string
	}{
		{name: "enum error inside HTTP 200", status: http.StatusOK, body: `{"version":1,"error":"code_expired","message":"Ask the operator for a new setup line."}`, wantCode: 1, wantErr: "Ask the operator for a new setup line."},
		{name: "invalid_code under HTTP 409 is still misuse", status: http.StatusConflict, body: `{"version":1,"error":"invalid_code"}`, wantCode: 2, wantErr: "claim failed"},
		{name: "invented enum value is generic failure not crash", status: http.StatusOK, body: `{"version":1,"error":"quantum_flux_error"}`, wantCode: 1, wantErr: "unrecognized error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := startClaimRecorder(t, claimJSON(tt.status, tt.body))
			home := t.TempDir()
			code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor")
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout, stderr)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Fatalf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
			if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("config was written, stat err = %v", err)
			}
		})
	}
}

func TestInstallClaimIgnoresServerURLInResponse(t *testing.T) {
	const token = "tok_SENTINEL_server_url"
	server, _ := startClaimRecorder(t, claimJSON(http.StatusOK, `{"version":1,"token":"`+token+`","name":"ballast","server_url":"https://evil.example.test/mcp"}`))
	home := t.TempDir()
	code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor")
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if got := cursorServers(t, home)["ballast"].(map[string]any)["url"]; got != installURL {
		t.Fatalf("installed url = %q, want the URL the user passed (%q)", got, installURL)
	}
	raw := readText(t, filepath.Join(home, ".cursor", "mcp.json"))
	if strings.Contains(raw, "evil.example.test") {
		t.Fatalf("config contains the server-suggested URL: %s", raw)
	}
	if strings.Contains(stdout+stderr, "evil.example.test") {
		t.Fatalf("output mentions the server-suggested URL; stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestInstallClaimFlagConflictsAreMisuse(t *testing.T) {
	const positional = "tok_SENTINEL_conflict"
	for _, tt := range []struct {
		name     string
		args     func(claimURL string) []string
		wantErr  string
		useStdin string
	}{
		{name: "claim with positional token", args: func(u string) []string {
			return []string{"install", installURL, positional, "--claim", "C", "--claim-url", u, "--client", "cursor"}
		}, wantErr: "positional token"},
		{name: "claim with token-stdin", args: func(u string) []string {
			return []string{"install", installURL, "--claim", "C", "--claim-url", u, "--token-stdin", "--client", "cursor"}
		}, wantErr: "--token-stdin", useStdin: "stdin-token\n"},
		{name: "claim with token-env", args: func(u string) []string {
			return []string{"install", installURL, "--claim", "C", "--claim-url", u, "--token-env", "HITCH_TEST_CLAIM_TOKEN", "--client", "cursor"}
		}, wantErr: "--token-env"},
		{name: "claim without claim-url", args: func(string) []string {
			return []string{"install", installURL, "--claim", "C", "--client", "cursor"}
		}, wantErr: "--claim requires --claim-url"},
		{name: "claim-url without claim", args: func(u string) []string {
			return []string{"install", installURL, "--claim-url", u, "--client", "cursor"}
		}, wantErr: "--claim-url requires --claim"},
		{name: "claim with stdio command", args: func(u string) []string {
			return []string{"install", "local-server", "--command", "npx", "--claim", "C", "--claim-url", u, "--client", "cursor"}
		}, wantErr: "stdio install cannot use --claim"},
		{name: "claim with authorization header", args: func(u string) []string {
			return []string{"install", installURL, "--claim", "C", "--claim-url", u, "--header", "Authorization: Bearer x", "--client", "cursor"}
		}, wantErr: "authorization header cannot be combined with --claim"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, hits := startClaimRecorder(t, claimJSON(http.StatusOK, `{"version":1,"token":"tok-should-never-be-fetched"}`))
			home := t.TempDir()
			t.Setenv("HITCH_TEST_CLAIM_TOKEN", "env-token")
			root := NewRootCommand(func() (harness.Env, error) { return testEnv(home), nil })
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetIn(strings.NewReader(tt.useStdin))
			root.SetArgs(tt.args(server.URL + "/claim"))
			err := root.Execute()
			if err == nil {
				t.Fatalf("Execute returned nil, want misuse error; stdout=%q", stdout.String())
			}
			var exit exitError
			if !errors.As(err, &exit) || exit.code != 2 {
				t.Fatalf("err = %v, want exit code 2", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %q, want containing %q", err.Error(), tt.wantErr)
			}
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Fatalf("claim requests = %d, want 0 — misuse must not spend the code", n)
			}
			if strings.Contains(stdout.String()+stderr.String()+err.Error(), positional) {
				t.Fatalf("output leaked the positional token")
			}
			if _, statErr := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(statErr) {
				t.Fatalf("config was written, stat err = %v", statErr)
			}
		})
	}
}

func TestInstallClaimNamePrecedence(t *testing.T) {
	const token = "tok_SENTINEL_name"
	for _, tt := range []struct {
		name     string
		url      string
		response string
		args     []string
		wantKey  string
		wantErr  string
	}{
		{
			name: "--name beats the response name", url: installURL,
			response: `{"version":1,"token":"` + token + `","name":"fromserver"}`,
			args:     []string{"--name", "pinned"},
			wantKey:  "pinned",
		},
		{
			name: "response name is sanitized and used when --name is absent", url: installURL,
			response: `{"version":1,"token":"` + token + `","name":"My Server!"}`,
			wantKey:  "my-server",
		},
		{
			name: "inference applies when the response has no name", url: "https://ballast.example.test/mcp",
			response: `{"version":1,"token":"` + token + `"}`,
			wantKey:  "ballast",
		},
		{
			name: "ambiguous inference still refuses when nothing names the server", url: installURL,
			response: `{"version":1,"token":"` + token + `"}`,
			wantErr:  "ambiguous; rerun with --name",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := startClaimRecorder(t, claimJSON(http.StatusOK, tt.response))
			home := t.TempDir()
			args := append([]string{"install", tt.url, "--claim", "A1B2-C3D4", "--claim-url", server.URL + "/claim", "--client", "cursor"}, tt.args...)
			code, stdout, stderr := runMain(t, home, args...)
			if strings.Contains(stdout+stderr, token) {
				t.Fatalf("output leaked the token; stdout=%q stderr=%q", stdout, stderr)
			}
			if tt.wantErr != "" {
				if code != 1 || !strings.Contains(stderr, tt.wantErr) {
					t.Fatalf("code = %d stderr = %q, want exit 1 containing %q", code, stderr, tt.wantErr)
				}
				if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
					t.Fatalf("config was written, stat err = %v", err)
				}
				return
			}
			if code != 0 {
				t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
			}
			servers := cursorServers(t, home)
			entry, ok := servers[tt.wantKey].(map[string]any)
			if !ok {
				t.Fatalf("cursor servers = %#v, want key %q", servers, tt.wantKey)
			}
			if got := entry["headers"].(map[string]any)["Authorization"]; got != "Bearer "+token {
				t.Fatalf("Authorization = %q, want the claimed token", got)
			}
		})
	}
}

func TestInstallClaimRequiresHTTPSClaimURL(t *testing.T) {
	for _, tt := range []struct {
		name     string
		claimURL string
		wantErr  string
	}{
		{name: "public http refused", claimURL: "http://claims.example.test/claim", wantErr: "claim URL must use https"},
		{name: "garbage refused", claimURL: "not a url", wantErr: "absolute https URL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", tt.claimURL, "--client", "cursor")
			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Fatalf("stderr = %q, want the pre-request refusal %q, not a network error", stderr, tt.wantErr)
			}
			if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("config was written, stat err = %v", err)
			}
		})
	}
}

func TestInstallClaimSurfacesExpiry(t *testing.T) {
	const token = "tok_SENTINEL_expiry"
	for _, tt := range []struct {
		name       string
		expiresAt  string
		wantOutput string
	}{
		{name: "future expiry is surfaced", expiresAt: "2999-01-02T03:04:05Z", wantOutput: "Token expires at 2999-01-02T03:04:05Z."},
		{name: "past expiry warns but still installs", expiresAt: "2001-01-02T03:04:05Z", wantOutput: "WARNING: the server reports this token already expired at 2001-01-02T03:04:05Z"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := startClaimRecorder(t, claimJSON(http.StatusOK, `{"version":1,"token":"`+token+`","name":"ballast","expires_at":"`+tt.expiresAt+`"}`))
			home := t.TempDir()
			code, stdout, stderr := runMain(t, home, "install", installURL, "--claim", "A1B2-C3D4", "--claim-url", server.URL+"/claim", "--client", "cursor")
			if code != 0 {
				t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stdout, tt.wantOutput) {
				t.Fatalf("stdout = %q, want containing %q", stdout, tt.wantOutput)
			}
			if strings.Contains(stdout+stderr, token) {
				t.Fatalf("output leaked the token; stdout=%q stderr=%q", stdout, stderr)
			}
			if got := cursorServers(t, home)["ballast"].(map[string]any)["headers"].(map[string]any)["Authorization"]; got != "Bearer "+token {
				t.Fatalf("Authorization = %q, want the claimed token installed despite the warning", got)
			}
		})
	}
}
