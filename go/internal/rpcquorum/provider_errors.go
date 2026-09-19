package rpcquorum

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/rpc"
)

const maxProviderDiagnosticBytes = 2 * 1024

var (
	diagnosticURL           = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	diagnosticSecret        = regexp.MustCompile(`(?i)(\b(?:authorization|proxy-authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password|passwd)\b["']?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:basic|bearer)\s+[^\s,;\}\]]+|[^\s,;\}\]]+)`)
	diagnosticAuthorization = regexp.MustCompile(`(?i)\b(?:basic|bearer)\s+[^\s,"';\}\]]+`)
	diagnosticPercentEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)
)

type providerOperationError struct {
	providerID string
	operation  string
	diagnostic string
	cause      error
}

func (e *providerOperationError) Error() string {
	if e == nil {
		return "rpc provider operation failed"
	}
	return fmt.Sprintf("%s %s failed: %s", e.providerID, e.operation, e.diagnostic)
}

func (e *providerOperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (c *Client) wrapProviderOperationError(index int, operation string, err error) error {
	if err == nil {
		return nil
	}
	c.mu.Lock()
	var rawURL string
	if index >= 0 && index < len(c.providers) {
		rawURL = c.providers[index].url
	}
	c.mu.Unlock()
	return wrapProviderOperationError(index, operation, err, rawURL)
}

func wrapProviderOperationError(index int, operation string, err error, rawURL string) error {
	if err == nil {
		return nil
	}
	return &providerOperationError{
		providerID: providerID(index), operation: operation, cause: err,
		diagnostic: providerDiagnostic(err, rawURL),
	}
}

func providerDiagnostic(err error, rawURL string) string {
	// JSON-RPC message text is intentionally displayed verbatim. Only the
	// matched RPC error is rendered, not outer wrappers or ErrorData.
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		return fmt.Sprintf("rpc error %d: %s", rpcErr.ErrorCode(), rpcErr.Error())
	}
	var wrapped *providerOperationError
	if errors.As(err, &wrapped) {
		return wrapped.diagnostic
	}
	message := err.Error()
	var httpErr rpc.HTTPError
	if errors.As(err, &httpErr) {
		// Status metadata is local, structured data, not upstream text. Keep it
		// outside redaction so short URL values cannot erase status digits.
		body := redactProviderDiagnostic(string(httpErr.Body), rawURL)
		return limitProviderDiagnostic(fmt.Sprintf("HTTP %d %s: %s", httpErr.StatusCode, http.StatusText(httpErr.StatusCode), body))
	}
	return redactProviderDiagnostic(message, rawURL)
}

func redactProviderDiagnostic(message, rawURL string) string {
	// Remove known URL components even when upstream repeats a credential
	// outside the URL, including URL-encoded and JSON-escaped forms.
	secrets := make(map[string]bool)
	add := func(value string, credential bool) {
		if value == "" {
			return
		}
		quoted := strconv.Quote(value)
		for _, form := range []string{value, url.QueryEscape(value), url.PathEscape(value), quoted[1 : len(quoted)-1]} {
			secrets[form] = secrets[form] || credential
		}
	}
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		add(rawURL, true)
		add(parsed.Host, false)
		add(parsed.Hostname(), false)
		if parsed.User != nil {
			add(parsed.User.Username(), true)
			password, _ := parsed.User.Password()
			add(password, true)
		}
		add(parsed.Path, false)
		add(parsed.EscapedPath(), false)
		for _, segment := range strings.Split(parsed.Path, "/") {
			add(segment, false)
		}
		for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
			add(segment, false)
		}
		for _, parameter := range strings.Split(parsed.RawQuery, "&") {
			key, value, _ := strings.Cut(parameter, "=")
			key, _ = url.QueryUnescape(key)
			add(value, diagnosticSecret.MatchString(key+"=x"))
		}
		for key, values := range parsed.Query() {
			for _, value := range values {
				add(value, diagnosticSecret.MatchString(key+"=x"))
			}
		}
	} else if strings.HasPrefix(rawURL, "/") {
		add(rawURL, true) // IPC socket path.
	}
	values := make([]string, 0, len(secrets))
	for value := range secrets {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) == len(values[j]) {
			return values[i] < values[j]
		}
		return len(values[i]) > len(values[j])
	})
	patterns := make([]string, 0, len(values))
	var credentialPatterns []string
	for _, value := range values {
		// Percent escapes are case-insensitive, but the credential itself is
		// not. Accept mixed-case escapes without case-folding literal letters.
		pattern := diagnosticPercentEscape.ReplaceAllStringFunc(regexp.QuoteMeta(value), func(escape string) string {
			return "(?i:" + escape + ")"
		})
		if secrets[value] {
			credentialPatterns = append(credentialPatterns, pattern)
		} else {
			patterns = append(patterns, pattern)
		}
	}
	// Filter whole URLs first so component replacements do not split them.
	message = diagnosticURL.ReplaceAllString(message, "[REDACTED]")
	message = diagnosticSecret.ReplaceAllString(message, "${1}[REDACTED]")
	message = diagnosticAuthorization.ReplaceAllString(message, "[REDACTED]")
	if len(credentialPatterns) > 0 {
		message = regexp.MustCompile(strings.Join(credentialPatterns, "|")).ReplaceAllString(message, "[REDACTED]")
	}
	if len(patterns) > 0 {
		matches := regexp.MustCompile(strings.Join(patterns, "|")).FindAllStringIndex(message, -1)
		var redacted strings.Builder
		last := 0
		for _, match := range matches {
			start, end := match[0], match[1]
			// Endpoint components not explicitly identified as credentials
			// must not replace fragments of unrelated numbers or words.
			if !diagnosticTokenBoundary(message, start, end) {
				continue
			}
			redacted.WriteString(message[last:start])
			redacted.WriteString("[REDACTED]")
			last = end
		}
		redacted.WriteString(message[last:])
		message = redacted.String()
	}
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, message)
	message = strings.TrimSpace(message)
	return limitProviderDiagnostic(message)
}

func diagnosticTokenBoundary(message string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(message[:start])
		if unicode.IsLetter(before) || unicode.IsDigit(before) || before == '_' {
			return false
		}
	}
	if end < len(message) {
		after, _ := utf8.DecodeRuneInString(message[end:])
		if unicode.IsLetter(after) || unicode.IsDigit(after) || after == '_' {
			return false
		}
	}
	return true
}

func limitProviderDiagnostic(message string) string {
	if len(message) > maxProviderDiagnosticBytes {
		const suffix = " [truncated]"
		end := maxProviderDiagnosticBytes - len(suffix)
		for !utf8.RuneStart(message[end]) {
			end--
		}
		message = message[:end] + suffix
	}
	return message
}

// providerFailureDetail retains the logical read context and the already
// formatted provider error, without exposing raw error text at aggregation.
func providerFailureDetail(index int, context string, err error) string {
	detail := fmt.Sprintf("%s %s", providerID(index), context)
	if err != nil {
		detail += ": " + err.Error()
	}
	return detail
}
