package n8n

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// maxErrorBody caps how much of a non-2xx body is read. A misrouted request can
// land on a proxy that answers with megabytes of HTML.
const maxErrorBody = 64 << 10

// APIError is a non-2xx response from the n8n API.
//
// Status and RequestID survive decoding failures: a server that answers with
// HTML or an empty body still produces a usable error.
type APIError struct {
	StatusCode int    // HTTP status code
	Status     string // HTTP status line, e.g. "404 Not Found"
	Method     string // request method
	Path       string // request path, query excluded
	RequestID  string // server request or correlation ID, when sent
	Code       string // machine-readable error code, when sent
	Message    string // human-readable message, when sent
	Hint       string // remediation hint, when sent
	Body       string // truncated raw body, set when it was not decodable JSON
	Truncated  bool   // body was longer than maxErrorBody
}

func (e *APIError) Error() string {
	var b strings.Builder
	if e.Method != "" && e.Path != "" {
		fmt.Fprintf(&b, "%s %s: ", e.Method, e.Path)
	}
	b.WriteString(e.statusText())
	if e.Code != "" {
		fmt.Fprintf(&b, " [%s]", e.Code)
	}
	switch {
	case e.Message != "":
		b.WriteString(": ")
		b.WriteString(e.Message)
	case e.Body != "":
		b.WriteString(": ")
		b.WriteString(singleLine(e.Body))
	}
	if e.Hint != "" {
		b.WriteString(" (")
		b.WriteString(e.Hint)
		b.WriteString(")")
	}
	if e.RequestID != "" {
		b.WriteString(" (request id ")
		b.WriteString(e.RequestID)
		b.WriteString(")")
	}
	return b.String()
}

func (e *APIError) statusText() string {
	if e.Status != "" {
		return e.Status
	}
	if text := http.StatusText(e.StatusCode); text != "" {
		return strconv.Itoa(e.StatusCode) + " " + text
	}
	return "HTTP " + strconv.Itoa(e.StatusCode)
}

// StatusCodeOf returns the HTTP status carried by err, or 0 when err is not an
// [APIError].
func StatusCodeOf(err error) int {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.StatusCode
	}
	return 0
}

// IsStatus reports whether err is an [APIError] with the given status code.
func IsStatus(err error, code int) bool { return StatusCodeOf(err) == code }

// IsUnauthorized reports a 401: missing, malformed, expired or revoked credential.
func IsUnauthorized(err error) bool { return IsStatus(err, http.StatusUnauthorized) }

// IsForbidden reports a 403: authenticated, but the scope or license is missing.
func IsForbidden(err error) bool { return IsStatus(err, http.StatusForbidden) }

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return IsStatus(err, http.StatusNotFound) }

// IsConflict reports a 409.
func IsConflict(err error) bool { return IsStatus(err, http.StatusConflict) }

// IsRateLimited reports a 429.
func IsRateLimited(err error) bool { return IsStatus(err, http.StatusTooManyRequests) }

// errorBody covers the shapes the API uses for failures. Every field is raw
// because message and code arrive as string, number, array or nested object
// depending on the handler that produced them.
type errorBody struct {
	Message     json.RawMessage `json:"message"`
	Error       json.RawMessage `json:"error"`
	Code        json.RawMessage `json:"code"`
	Hint        json.RawMessage `json:"hint"`
	Description json.RawMessage `json:"description"`
}

// newAPIError builds an error from a response body that has already been read
// and truncated by the caller.
func newAPIError(resp *http.Response, method, path string, body []byte, truncated bool) *APIError {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Method:     method,
		Path:       path,
		RequestID:  requestID(resp.Header),
		Truncated:  truncated,
	}

	var decoded errorBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		apiErr.Body = truncateText(strings.TrimSpace(string(body)), 512)
		return apiErr
	}

	apiErr.Message = firstText(decoded.Message, decoded.Error, decoded.Description)
	apiErr.Code = firstText(decoded.Code)
	apiErr.Hint = firstText(decoded.Hint)
	if apiErr.Message == "" && apiErr.Code == "" {
		// Valid JSON in a shape we do not know: keep it rather than drop it.
		apiErr.Body = truncateText(strings.TrimSpace(string(body)), 512)
	}
	return apiErr
}

// firstText returns the first raw value that flattens to a non-empty string.
func firstText(values ...json.RawMessage) string {
	for _, v := range values {
		if text := flattenText(v); text != "" {
			return text
		}
	}
	return ""
}

// flattenText renders a JSON value as one human-readable string. Strings and
// numbers pass through, arrays join, and objects fall back to their message,
// error or description field.
func flattenText(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}

	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		parts := make([]string, 0, len(list))
		for _, item := range list {
			if text := flattenText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "; ")
	}

	var obj struct {
		Message     json.RawMessage `json:"message"`
		Error       json.RawMessage `json:"error"`
		Description json.RawMessage `json:"description"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return firstText(obj.Message, obj.Error, obj.Description)
	}
	return ""
}

// requestID reads the correlation identifiers n8n and its proxies emit.
func requestID(h http.Header) string {
	for _, name := range []string{"X-Request-Id", "X-Correlation-Id", "X-Amzn-Trace-Id"} {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func truncateText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
