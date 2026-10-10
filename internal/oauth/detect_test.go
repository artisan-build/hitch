package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestProbeClassifiesOnlyUnauthorizedBearerResourceMetadata(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header string
		want   bool
	}{
		{name: "OAuth signal", status: http.StatusUnauthorized, header: `Bearer realm="mcp", resource_metadata="https://auth.example.test/.well-known/oauth-protected-resource/mcp"`, want: true},
		{name: "bare 401", status: http.StatusUnauthorized},
		{name: "200 with signal", status: http.StatusOK, header: `Bearer resource_metadata="https://auth.example.test/metadata"`},
		{name: "wrong challenge", status: http.StatusUnauthorized, header: `Basic resource_metadata="https://auth.example.test/metadata"`},
		{name: "invalid metadata URI", status: http.StatusUnauthorized, header: `Bearer resource_metadata="not-a-uri"`},
		{name: "similar parameter", status: http.StatusUnauthorized, header: `Bearer not_resource_metadata="https://auth.example.test/metadata"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != "" {
					t.Errorf("Authorization = %q, want empty", got)
				}
				if got := r.Header.Get("Accept"); got != mcpAccept {
					t.Errorf("Accept = %q, want %q", got, mcpAccept)
				}
				if got := r.Header.Get("Content-Type"); got != "" {
					t.Errorf("Content-Type = %q, want empty", got)
				}
				if tt.header != "" {
					w.Header().Set("WWW-Authenticate", tt.header)
				}
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			if got := Probe(context.Background(), server.Client(), server.URL); got != tt.want {
				t.Fatalf("Probe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProbeDoesNotFollowRedirects(t *testing.T) {
	destinationRequests := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationRequests++
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://auth.example.test/metadata"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer destination.Close()

	sourceRequests := 0
	sourceMethods := []string{}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceRequests++
		sourceMethods = append(sourceMethods, r.Method)
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()

	if Probe(context.Background(), source.Client(), source.URL) {
		t.Fatal("Probe() = true after redirect, want false")
	}
	if sourceRequests != 1 || len(sourceMethods) != 1 || sourceMethods[0] != http.MethodPost {
		t.Fatalf("source requests = %d methods = %#v, want one POST", sourceRequests, sourceMethods)
	}
	if destinationRequests != 0 {
		t.Fatalf("destination requests = %d, want 0", destinationRequests)
	}
}

func TestProbeStripsURLUserinfoBeforeSendingRequest(t *testing.T) {
	var authorization string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://auth.example.test/metadata"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	probeURL, err := url.Parse(server.URL + "/mcp")
	if err != nil {
		t.Fatalf("parse TLS server URL: %v", err)
	}
	probeURL.User = url.UserPassword("u", "p")

	if !Probe(context.Background(), server.Client(), probeURL.String()) {
		t.Fatal("Probe() = false, want OAuth classification after stripping URL userinfo")
	}
	if authorization != "" {
		t.Fatalf("probe sent Authorization header derived from URL userinfo: %q", authorization)
	}
}

func TestProbeTimeoutAndConnectionErrorAreNotOAuth(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if Probe(ctx, server.Client(), server.URL) {
			t.Fatal("Probe() = true, want false")
		}
	})

	t.Run("connection error", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		url := server.URL
		server.Close()
		if Probe(context.Background(), http.DefaultClient, url) {
			t.Fatal("Probe() = true, want false")
		}
	})
}

func TestBearerResourceMetadataHandlesMultipleChallenges(t *testing.T) {
	for _, header := range []string{
		`Basic realm="legacy", Bearer realm="mcp", resource_metadata="https://auth.example.test/metadata"`,
		`Bearer realm="comma,inside", resource_metadata="https://auth.example.test/metadata", Basic realm="legacy"`,
	} {
		if !bearerResourceMetadata(header) {
			t.Fatalf("bearerResourceMetadata(%q) = false, want true", header)
		}
	}
}
