// Package claim exchanges a single-use claim code for a durable bearer token
// over the hitch claim contract (docs/claim-contract.md). The exchange runs
// before any config file is touched, and no code path here may place the
// response body — which contains the token — into an error message.
package claim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/artisan-build/hitch/internal/install"
)

// ContractVersion is the claim contract version this client speaks.
const ContractVersion = 1

const (
	maxResponseBody = 1 << 20
	maxRedirects    = 5
	requestTimeout  = 30 * time.Second
)

// transport is swapped in tests so an exchange that must not happen can be
// proven to make zero round trips.
var transport http.RoundTripper = http.DefaultTransport

// Response is a successful claim exchange. ExpiresAt is advisory and zero
// when the server omitted it or sent something unparseable.
type Response struct {
	Token     string
	Name      string
	ExpiresAt time.Time
}

// EnumError is a claim failure the server reported through the contract's
// `error` enum. Message is the server's human prose, printed verbatim.
type EnumError struct {
	Code    string
	Message string
}

func (e *EnumError) Error() string {
	message := e.Message
	if message == "" {
		message = defaultEnumMessage(e.Code)
	}
	return "claim failed: " + message
}

// Misuse reports whether the failure is the user's typo rather than an
// operational failure; callers map it to exit 2.
func (e *EnumError) Misuse() bool { return e.Code == "invalid_code" }

func defaultEnumMessage(code string) string {
	switch code {
	case "invalid_code":
		return "the claim code is malformed; check it for a typo"
	case "code_not_found":
		return "the server has no such claim code"
	case "code_already_claimed":
		return "this code was already used to set up a working connection; ask for a new one-liner"
	case "code_expired":
		return "the claim code has expired; ask for a new one-liner"
	case "unsupported_version":
		return "the server does not support this claim contract version"
	case "server_error":
		return "the server reported a transient error; rerun the same command"
	}
	return fmt.Sprintf("the server reported an unrecognized error (%q)", code)
}

// NotClaimEndpointError is the graceful-degradation failure: the URL answered,
// but not with the claim contract. Got names what arrived without ever
// containing the response body.
type NotClaimEndpointError struct {
	ClaimURL string
	Got      string
}

func (e *NotClaimEndpointError) Error() string {
	return fmt.Sprintf("%s does not speak the hitch claim contract (expected JSON with a \"token\" field; got %s)", e.ClaimURL, e.Got)
}

// NewerContractError means the server answered with a contract version this
// hitch does not speak.
type NewerContractError struct {
	ClaimURL string
	Version  int
}

func (e *NewerContractError) Error() string {
	return fmt.Sprintf("%s speaks claim contract version %d, newer than this hitch understands (version %d); update hitch or ask the operator for a token", e.ClaimURL, e.Version, ContractVersion)
}

// ValidateURL enforces the transport rule before any request exists: https
// always, plaintext http only for loopback hosts. It is called (and must
// succeed) before Exchange builds a request.
func ValidateURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("claim URL must be an absolute https URL with a non-empty host")
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLoopback(parsed.Hostname()) {
			return "", fmt.Errorf("claim URL must use https; the response carries a durable token (plain http is allowed only for localhost)")
		}
	default:
		return "", fmt.Errorf("claim URL must use https; the response carries a durable token (plain http is allowed only for localhost)")
	}
	return parsed.String(), nil
}

func isLoopback(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Exchange POSTs the claim code to claimURL and returns the token the server
// minted for it. One attempt, no automatic retry: a lost response leaves the
// code live (make-before-break), so the user simply reruns the one-liner.
func Exchange(claimURL string, code string, hitchVersion string) (Response, error) {
	validated, err := ValidateURL(claimURL)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(map[string]any{"claim_code": code, "version": ContractVersion})
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequest(http.MethodPost, validated, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("could not build claim request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hitch/"+hitchVersion)
	client := &http.Client{
		Timeout:       requestTimeout,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	display := redactedURL(validated)
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("claim request to %s failed: %w", display, unwrapURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return Response{}, fmt.Errorf("claim response from %s could not be read: %w", display, err)
	}
	return parseResponse(display, resp.StatusCode, resp.Header.Get("Content-Type"), raw)
}

// redactedURL masks any userinfo password so a claim URL carrying credentials
// never prints them in an error message.
func redactedURL(raw string) string {
	if parsed, err := url.Parse(raw); err == nil {
		return parsed.Redacted()
	}
	return raw
}

// unwrapURLError drops the *url.Error layer so redirect refusals read as one
// sentence instead of `Post "...": ...`; the inner error never holds a body.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	// 301/302/303 turn the POST into a bodiless GET, so the claim code would
	// never reach the destination. Only 307/308 preserve method and body.
	if resp := req.Response; resp != nil && resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusPermanentRedirect {
		return fmt.Errorf("redirected with HTTP %d, which discards the POST body carrying the claim code; only 307/308 redirects are followed", resp.StatusCode)
	}
	origin := via[0].URL
	if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
		return fmt.Errorf("redirected cross-origin to another host; refusing to send the claim code there")
	}
	if len(via) > maxRedirects {
		return fmt.Errorf("redirected more than %d times", maxRedirects)
	}
	return nil
}

// parseResponse walks the detection ladder. The enum is the contract: any JSON
// object carrying `error` is branched on regardless of HTTP status, an unknown
// enum value is a generic failure, and everything else falls through to the
// not-a-claim-endpoint message — which describes the body without quoting it.
// Fields are read leniently, one at a time, so a wrong type on one field
// cannot discard the enum carried by another.
func parseResponse(claimURL string, status int, contentType string, raw []byte) (Response, error) {
	var generic map[string]any
	if json.Unmarshal(raw, &generic) != nil {
		return Response{}, &NotClaimEndpointError{ClaimURL: claimURL, Got: describeRawBody(status, contentType)}
	}
	stringField := func(key string) string {
		if s, ok := generic[key].(string); ok {
			return s
		}
		return ""
	}
	if code := stringField("error"); code != "" {
		return Response{}, &EnumError{Code: code, Message: stringField("message")}
	}
	if version, ok := generic["version"].(float64); ok && int(version) > ContractVersion {
		return Response{}, &NewerContractError{ClaimURL: claimURL, Version: int(version)}
	}
	token := stringField("token")
	if usableToken(token) && status >= 200 && status < 300 {
		out := Response{Token: token, Name: stringField("name")}
		if expiresAt := stringField("expires_at"); expiresAt != "" {
			if ts, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil {
				out.ExpiresAt = ts
			}
		}
		return out, nil
	}
	return Response{}, &NotClaimEndpointError{ClaimURL: claimURL, Got: describeJSONBody(status, token)}
}

// usableToken applies the shared token validation — the very function the
// positional, stdin, env, and prompt sources use — so the claim path cannot
// accept a value any other source would refuse.
func usableToken(token string) bool {
	return install.ValidateTokenValue(token) == nil
}

func describeRawBody(status int, contentType string) string {
	if mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0]); mediaType != "" {
		return fmt.Sprintf("%s (HTTP %d)", mediaType, status)
	}
	return fmt.Sprintf("an unparseable body (HTTP %d)", status)
}

func describeJSONBody(status int, token string) string {
	if token == "" {
		return fmt.Sprintf("JSON without a \"token\" field (HTTP %d)", status)
	}
	if !usableToken(token) {
		return fmt.Sprintf("JSON with an unusable \"token\" value (HTTP %d)", status)
	}
	return fmt.Sprintf("HTTP %d", status)
}
