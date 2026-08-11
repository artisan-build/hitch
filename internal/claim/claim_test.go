package claim

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const unitBodySentinel = "tok_SENTINEL_unit_body"

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func swapTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	old := transport
	t.Cleanup(func() { transport = old })
	transport = rt
}

func TestValidateURL(t *testing.T) {
	for _, tt := range []struct {
		name    string
		url     string
		want    string
		wantErr string
	}{
		{name: "https ok", url: "https://claims.example.test/claim", want: "https://claims.example.test/claim"},
		{name: "https with port ok", url: "https://claims.example.test:8443/claim", want: "https://claims.example.test:8443/claim"},
		{name: "http localhost ok", url: "http://localhost:8080/claim", want: "http://localhost:8080/claim"},
		{name: "http 127 ok", url: "http://127.0.0.1:9999/claim", want: "http://127.0.0.1:9999/claim"},
		{name: "http ipv6 loopback ok", url: "http://[::1]:9999/claim", want: "http://[::1]:9999/claim"},
		{name: "trims whitespace", url: "  https://claims.example.test/claim  ", want: "https://claims.example.test/claim"},
		{name: "http public refused", url: "http://claims.example.test/claim", wantErr: "must use https"},
		{name: "ftp refused", url: "ftp://claims.example.test/claim", wantErr: "must use https"},
		{name: "scheme-less refused", url: "claims.example.test/claim", wantErr: "absolute https URL"},
		{name: "empty refused", url: "", wantErr: "absolute https URL"},
		{name: "missing host refused", url: "https:///claim", wantErr: "absolute https URL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateURL(tt.url)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidateURL(%q) err = %v, want containing %q", tt.url, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateURL(%q) err = %v", tt.url, err)
			}
			if got != tt.want {
				t.Fatalf("ValidateURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExchangeRefusesPlaintextURLWithoutAnyRoundTrip(t *testing.T) {
	var roundTrips int32
	swapTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&roundTrips, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"version":1,"token":"tok-from-transport"}`)),
		}, nil
	}))

	t.Run("plaintext public URL never reaches the transport", func(t *testing.T) {
		_, err := Exchange("http://claims.example.test/claim", "A1B2-C3D4", "test")
		if err == nil || !strings.Contains(err.Error(), "must use https") {
			t.Fatalf("Exchange err = %v, want https refusal", err)
		}
		if n := atomic.LoadInt32(&roundTrips); n != 0 {
			t.Fatalf("round trips = %d, want 0", n)
		}
	})

	t.Run("positive control: a valid URL does reach the transport", func(t *testing.T) {
		resp, err := Exchange("http://127.0.0.1:1/claim", "A1B2-C3D4", "test")
		if err != nil {
			t.Fatalf("Exchange err = %v", err)
		}
		if resp.Token != "tok-from-transport" {
			t.Fatalf("token = %q, want tok-from-transport", resp.Token)
		}
		if n := atomic.LoadInt32(&roundTrips); n != 1 {
			t.Fatalf("round trips = %d, want 1", n)
		}
	})
}

func TestExchangeSendsTheContractRequest(t *testing.T) {
	type seen struct {
		method      string
		contentType string
		accept      string
		userAgent   string
		body        map[string]any
	}
	var got seen
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		got = seen{method: r.Method, contentType: r.Header.Get("Content-Type"), accept: r.Header.Get("Accept"), userAgent: r.Header.Get("User-Agent"), body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":1,"token":"tok-success","name":"ballast","expires_at":"2031-01-02T03:04:05Z"}`)
	}))
	t.Cleanup(server.Close)

	resp, err := Exchange(server.URL+"/claim", "A1B2-C3D4", "1.2.3")
	if err != nil {
		t.Fatalf("Exchange err = %v", err)
	}
	if got.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", got.method)
	}
	if got.contentType != "application/json" || got.accept != "application/json" {
		t.Fatalf("content-type = %q accept = %q, want application/json for both", got.contentType, got.accept)
	}
	if got.userAgent != "hitch/1.2.3" {
		t.Fatalf("user-agent = %q, want hitch/1.2.3", got.userAgent)
	}
	if got.body["claim_code"] != "A1B2-C3D4" {
		t.Fatalf("claim_code = %v, want A1B2-C3D4", got.body["claim_code"])
	}
	if got.body["version"] != float64(1) {
		t.Fatalf("version = %v, want 1", got.body["version"])
	}
	if resp.Token != "tok-success" || resp.Name != "ballast" {
		t.Fatalf("response = %+v, want token tok-success and name ballast", resp)
	}
	want := time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)
	if !resp.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", resp.ExpiresAt, want)
	}
}

func redirectChainServer(t *testing.T, redirects int) (*httptest.Server, *int32) {
	t.Helper()
	var claims int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/hop/", func(w http.ResponseWriter, r *http.Request) {
		var n int
		if _, err := fmt.Sscanf(r.URL.Path, "/hop/%d", &n); err != nil {
			http.NotFound(w, r)
			return
		}
		if n < redirects {
			http.Redirect(w, r, fmt.Sprintf("%s/hop/%d", server.URL, n+1), http.StatusTemporaryRedirect)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["claim_code"] != "A1B2-C3D4" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"version":1,"error":"invalid_code","message":"body lost in redirects"}`)
			return
		}
		atomic.AddInt32(&claims, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":1,"token":"tok-after-redirects"}`)
	})
	return server, &claims
}

func TestExchangeRedirectRules(t *testing.T) {
	t.Run("five same-origin redirects are followed with the body intact", func(t *testing.T) {
		server, claims := redirectChainServer(t, 5)
		resp, err := Exchange(server.URL+"/hop/0", "A1B2-C3D4", "test")
		if err != nil {
			t.Fatalf("Exchange err = %v", err)
		}
		if resp.Token != "tok-after-redirects" {
			t.Fatalf("token = %q, want tok-after-redirects", resp.Token)
		}
		if n := atomic.LoadInt32(claims); n != 1 {
			t.Fatalf("claims served = %d, want 1", n)
		}
	})

	t.Run("a sixth redirect is refused", func(t *testing.T) {
		server, claims := redirectChainServer(t, 6)
		_, err := Exchange(server.URL+"/hop/0", "A1B2-C3D4", "test")
		if err == nil || !strings.Contains(err.Error(), "redirected more than 5 times") {
			t.Fatalf("Exchange err = %v, want redirect cap refusal", err)
		}
		if n := atomic.LoadInt32(claims); n != 0 {
			t.Fatalf("claims served = %d, want 0", n)
		}
	})

	t.Run("a cross-origin redirect is refused before the other host is contacted", func(t *testing.T) {
		var otherHits int32
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&otherHits, 1)
		}))
		t.Cleanup(other.Close)
		redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL+"/claim", http.StatusTemporaryRedirect)
		}))
		t.Cleanup(redirecting.Close)
		_, err := Exchange(redirecting.URL+"/claim", "A1B2-C3D4", "test")
		if err == nil || !strings.Contains(err.Error(), "cross-origin") {
			t.Fatalf("Exchange err = %v, want cross-origin refusal", err)
		}
		if n := atomic.LoadInt32(&otherHits); n != 0 {
			t.Fatalf("other host hits = %d, want 0", n)
		}
	})
}

func TestExchangeRedirectKinds(t *testing.T) {
	for _, tt := range []struct {
		status  int
		allowed bool
	}{
		{status: http.StatusMovedPermanently},
		{status: http.StatusFound},
		{status: http.StatusSeeOther},
		{status: http.StatusTemporaryRedirect, allowed: true},
		{status: http.StatusPermanentRedirect, allowed: true},
	} {
		t.Run(fmt.Sprintf("HTTP %d", tt.status), func(t *testing.T) {
			var destHits int32
			var destMethod string
			var destBody []byte
			mux := http.NewServeMux()
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", server.URL+"/dest")
				w.WriteHeader(tt.status)
			})
			mux.HandleFunc("/dest", func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&destHits, 1)
				destMethod = r.Method
				destBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"version":1,"token":"tok-redirect"}`)
			})
			resp, err := Exchange(server.URL+"/start", "A1B2-C3D4", "test")
			if tt.allowed {
				if err != nil {
					t.Fatalf("Exchange err = %v, want the %d redirect followed", err, tt.status)
				}
				if resp.Token != "tok-redirect" {
					t.Fatalf("token = %q, want tok-redirect", resp.Token)
				}
				if destMethod != http.MethodPost || !strings.Contains(string(destBody), "A1B2-C3D4") {
					t.Fatalf("destination saw %s with body %q, want a POST carrying the claim code", destMethod, destBody)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "only 307/308 redirects are followed") {
				t.Fatalf("Exchange err = %v, want the body-discarding-redirect refusal", err)
			}
			if n := atomic.LoadInt32(&destHits); n != 0 {
				t.Fatalf("destination hits = %d, want 0 — the code must never be delivered bodiless", n)
			}
		})
	}
}

func TestExchangeRedactsClaimURLUserinfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>nope</html>")
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	_, err = Exchange("http://alice:hunter2@"+parsed.Host+"/claim", "A1B2-C3D4", "test")
	if err == nil {
		t.Fatalf("Exchange err = nil, want the not-a-claim-endpoint failure")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error text leaked the claim URL password: %v", err)
	}
	if !strings.Contains(err.Error(), "xxxxx") {
		t.Fatalf("error text should carry the redacted userinfo marker: %v", err)
	}
}

func TestExchangeResponseLadder(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		contentType string
		body        string
		check       func(t *testing.T, resp Response, err error)
	}{
		{
			name: "known enum with matching status", status: http.StatusGone, contentType: "application/json",
			body: `{"version":1,"error":"code_expired","message":"Code expired on Tuesday."}`,
			check: func(t *testing.T, _ Response, err error) {
				var enumErr *EnumError
				if !errors.As(err, &enumErr) || enumErr.Code != "code_expired" {
					t.Fatalf("err = %v, want EnumError code_expired", err)
				}
				if enumErr.Misuse() {
					t.Fatalf("code_expired reported as misuse")
				}
				if !strings.Contains(err.Error(), "Code expired on Tuesday.") {
					t.Fatalf("err = %v, want the server message verbatim", err)
				}
			},
		},
		{
			name: "enum wins over a 200 status", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"error":"code_expired","message":"Enum beats status."}`,
			check: func(t *testing.T, _ Response, err error) {
				var enumErr *EnumError
				if !errors.As(err, &enumErr) || enumErr.Code != "code_expired" {
					t.Fatalf("err = %v, want EnumError code_expired despite HTTP 200", err)
				}
			},
		},
		{
			name: "invalid_code is misuse whatever the status", status: http.StatusConflict, contentType: "application/json",
			body: `{"version":1,"error":"invalid_code"}`,
			check: func(t *testing.T, _ Response, err error) {
				var enumErr *EnumError
				if !errors.As(err, &enumErr) || !enumErr.Misuse() {
					t.Fatalf("err = %v, want misuse EnumError despite HTTP 409", err)
				}
				if !strings.Contains(err.Error(), "typo") {
					t.Fatalf("err = %v, want the invalid_code default message", err)
				}
			},
		},
		{
			name: "unknown enum value is a generic failure not a crash", status: http.StatusBadRequest, contentType: "application/json",
			body: `{"version":1,"error":"flux_capacitor_drained"}`,
			check: func(t *testing.T, _ Response, err error) {
				var enumErr *EnumError
				if !errors.As(err, &enumErr) || enumErr.Code != "flux_capacitor_drained" {
					t.Fatalf("err = %v, want EnumError with the unknown code", err)
				}
				if enumErr.Misuse() {
					t.Fatalf("unknown enum reported as misuse")
				}
				if !strings.Contains(err.Error(), "unrecognized error") {
					t.Fatalf("err = %v, want the generic unrecognized-error message", err)
				}
			},
		},
		{
			name: "html body is not a claim endpoint and is never quoted", status: http.StatusOK, contentType: "text/html",
			body: "<html>" + unitBodySentinel + "</html>",
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) || !strings.Contains(notClaim.Got, "text/html") {
					t.Fatalf("err = %v, want NotClaimEndpointError naming text/html", err)
				}
			},
		},
		{
			name: "truncated json is not a claim endpoint and is never quoted", status: http.StatusOK, contentType: "application/json",
			body: `{"token":"` + unitBodySentinel + `"`,
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) {
					t.Fatalf("err = %v, want NotClaimEndpointError", err)
				}
			},
		},
		{
			name: "json without a token field", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"hint":"` + unitBodySentinel + `"}`,
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) || !strings.Contains(notClaim.Got, `JSON without a "token" field`) {
					t.Fatalf("err = %v, want NotClaimEndpointError naming the missing token field", err)
				}
			},
		},
		{
			name: "empty token string counts as missing", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"token":""}`,
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) {
					t.Fatalf("err = %v, want NotClaimEndpointError", err)
				}
			},
		},
		{
			name: "token on a non-2xx status is refused without quoting it", status: http.StatusInternalServerError, contentType: "application/json",
			body: `{"version":1,"token":"` + unitBodySentinel + `"}`,
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) || !strings.Contains(notClaim.Got, "HTTP 500") {
					t.Fatalf("err = %v, want NotClaimEndpointError naming HTTP 500", err)
				}
			},
		},
		{
			name: "newer contract version is a distinct message without the token", status: http.StatusOK, contentType: "application/json",
			body: `{"version":2,"token":"` + unitBodySentinel + `"}`,
			check: func(t *testing.T, _ Response, err error) {
				var newer *NewerContractError
				if !errors.As(err, &newer) || newer.Version != 2 {
					t.Fatalf("err = %v, want NewerContractError version 2", err)
				}
			},
		},
		{
			name: "404 html is not a claim endpoint", status: http.StatusNotFound, contentType: "text/html",
			body: "<h1>Not Found</h1>",
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) {
					t.Fatalf("err = %v, want NotClaimEndpointError", err)
				}
			},
		},
		{
			name: "401 with an empty body is not an auth error", status: http.StatusUnauthorized, contentType: "",
			body: "",
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) {
					t.Fatalf("err = %v, want NotClaimEndpointError, never an auth-shaped error", err)
				}
				if !strings.Contains(err.Error(), "does not speak the hitch claim contract") {
					t.Fatalf("err = %v, want the claim-contract framing", err)
				}
			},
		},
		{
			name: "a wrong type on version does not hide the error enum", status: http.StatusGone, contentType: "application/json",
			body: `{"version":"1","error":"code_expired","message":"Expired."}`,
			check: func(t *testing.T, _ Response, err error) {
				var enumErr *EnumError
				if !errors.As(err, &enumErr) || enumErr.Code != "code_expired" {
					t.Fatalf("err = %v, want EnumError code_expired despite the string-typed version", err)
				}
			},
		},
		{
			name: "a wrong type on version does not sink a usable token", status: http.StatusOK, contentType: "application/json",
			body: `{"version":"1","token":"tok-1"}`,
			check: func(t *testing.T, resp Response, err error) {
				if err != nil || resp.Token != "tok-1" {
					t.Fatalf("resp = %+v err = %v, want success with tok-1", resp, err)
				}
			},
		},
		{
			name: "a blank token is unusable", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"token":"   "}`,
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) || !strings.Contains(notClaim.Got, "unusable") {
					t.Fatalf("err = %v, want NotClaimEndpointError naming an unusable token", err)
				}
			},
		},
		{
			name: "control characters in the token are refused without echoing them", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"token":"good\r\nX-Injected: 1"}`,
			check: func(t *testing.T, _ Response, err error) {
				var notClaim *NotClaimEndpointError
				if !errors.As(err, &notClaim) || !strings.Contains(notClaim.Got, "unusable") {
					t.Fatalf("err = %v, want NotClaimEndpointError naming an unusable token", err)
				}
				if strings.Contains(err.Error(), "X-Injected") {
					t.Fatalf("error text leaked the injected header shape: %v", err)
				}
			},
		},
		{
			name: "success ignores unknown fields including server_url", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"token":"tok-1","name":"ballast","server_url":"https://evil.example.test/mcp","future_field":123}`,
			check: func(t *testing.T, resp Response, err error) {
				if err != nil {
					t.Fatalf("err = %v, want success", err)
				}
				if resp.Token != "tok-1" || resp.Name != "ballast" {
					t.Fatalf("resp = %+v, want token tok-1 and name ballast", resp)
				}
			},
		},
		{
			name: "null expires_at is treated as absent", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"token":"tok-1","expires_at":null}`,
			check: func(t *testing.T, resp Response, err error) {
				if err != nil || !resp.ExpiresAt.IsZero() {
					t.Fatalf("resp = %+v err = %v, want success with zero expiry", resp, err)
				}
			},
		},
		{
			name: "malformed expires_at is ignored", status: http.StatusOK, contentType: "application/json",
			body: `{"version":1,"token":"tok-1","expires_at":"soon"}`,
			check: func(t *testing.T, resp Response, err error) {
				if err != nil || !resp.ExpiresAt.IsZero() {
					t.Fatalf("resp = %+v err = %v, want success with zero expiry", resp, err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)
			resp, err := Exchange(server.URL+"/claim", "A1B2-C3D4", "test")
			if err != nil && strings.Contains(err.Error(), unitBodySentinel) {
				t.Fatalf("error text leaked the response body: %v", err)
			}
			tt.check(t, resp, err)
		})
	}
}
