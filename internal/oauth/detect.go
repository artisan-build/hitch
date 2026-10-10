package oauth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const probeTimeout = 2 * time.Second

// Detect probes an MCP endpoint without credentials. Probe failures are
// deliberately indistinguishable from a non-OAuth response so installation can
// fall back to the existing bearer-token path.
func Detect(rawURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	return Probe(ctx, http.DefaultClient, rawURL)
}

// Probe reports OAuth only for a 401 Bearer challenge carrying a valid
// resource_metadata URI. It never adds headers to the request.
func Probe(ctx context.Context, client *http.Client, rawURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	for _, header := range resp.Header.Values("WWW-Authenticate") {
		if bearerResourceMetadata(header) {
			return true
		}
	}
	return false
}

func bearerResourceMetadata(header string) bool {
	for i := 0; i < len(header); {
		i = skipSeparators(header, i)
		scheme, next := readToken(header, i)
		if scheme == "" {
			return false
		}
		i = next
		if i >= len(header) || (header[i] != ' ' && header[i] != '\t') {
			return false
		}
		bearer := strings.EqualFold(scheme, "Bearer")

		for i < len(header) {
			i = skipWhitespace(header, i)
			if i >= len(header) {
				break
			}
			if header[i] == ',' {
				i = skipWhitespace(header, i+1)
			}
			nameStart := i
			name, afterName := readToken(header, i)
			if name == "" {
				return false
			}
			i = skipWhitespace(header, afterName)
			if i >= len(header) || header[i] != '=' {
				i = nameStart
				break
			}
			i = skipWhitespace(header, i+1)
			value, afterValue, ok := readAuthValue(header, i)
			if !ok {
				return false
			}
			if bearer && strings.EqualFold(name, "resource_metadata") && validMetadataURI(value) {
				return true
			}
			i = skipWhitespace(header, afterValue)
			if i < len(header) && header[i] != ',' {
				return false
			}
		}
	}
	return false
}

func validMetadataURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && parsed.Hostname() != "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

func skipSeparators(value string, i int) int {
	for i < len(value) && (value[i] == ',' || value[i] == ' ' || value[i] == '\t') {
		i++
	}
	return i
}

func skipWhitespace(value string, i int) int {
	for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
		i++
	}
	return i
}

func readToken(value string, i int) (string, int) {
	start := i
	for i < len(value) && isTokenChar(value[i]) {
		i++
	}
	return value[start:i], i
}

func readAuthValue(value string, i int) (string, int, bool) {
	if i >= len(value) {
		return "", i, false
	}
	if value[i] != '"' {
		token, next := readToken(value, i)
		return token, next, token != ""
	}

	var decoded strings.Builder
	for i++; i < len(value); i++ {
		switch value[i] {
		case '"':
			return decoded.String(), i + 1, true
		case '\\':
			i++
			if i >= len(value) {
				return "", i, false
			}
			decoded.WriteByte(value[i])
		default:
			decoded.WriteByte(value[i])
		}
	}
	return "", i, false
}

func isTokenChar(char byte) bool {
	if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char))
}
