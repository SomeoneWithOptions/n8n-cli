package n8n

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
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

	// Reason is the machine-readable cause some endpoints add, e.g.
	// review_pending on a 409 from workflow publish.
	Reason string
	// VersionID names the draft a workflow update saved before its
	// republication was refused (403).
	VersionID string
	// ReviewRequestID names the open workflow review that blocked a
	// publication (409).
	ReviewRequestID string

	// Fields names the request fields a validation failure (400) points at:
	// the first element of the issue path, then any unrecognized keys. n8n
	// answers a rejected body with its first schema issue, e.g.
	// {"code":"unrecognized_keys","keys":["idPolicy"],"path":[],...}.
	Fields []string

	// RateLimit is set on a 429 from the headers the server sent with it.
	RateLimit *RateLimit
}

// RateLimit is what a 429 says about the rate-limit window. A field the
// server did not send stays at its zero value and its Has flag false.
type RateLimit struct {
	// RetryAfter is how long to wait before retrying (Retry-After).
	RetryAfter    time.Duration
	HasRetryAfter bool
	// Limit and Remaining are X-RateLimit-Limit and X-RateLimit-Remaining.
	Limit        int
	HasLimit     bool
	Remaining    int
	HasRemaining bool
	// Reset is when the window resets (X-RateLimit-Reset, Unix seconds).
	Reset time.Time
}

// RetryAfterOf returns the wait a 429 asked for and whether it named one.
func RetryAfterOf(err error) (time.Duration, bool) {
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.RateLimit != nil && apiErr.RateLimit.HasRetryAfter {
		return apiErr.RateLimit.RetryAfter, true
	}
	return 0, false
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
	if e.Reason != "" && e.Reason != e.Code {
		fmt.Fprintf(&b, " [%s]", e.Reason)
	}
	switch {
	case e.Message != "":
		b.WriteString(": ")
		b.WriteString(e.Message)
	case e.Body != "":
		b.WriteString(": ")
		b.WriteString(singleLine(e.Body))
	}
	if len(e.Fields) > 0 {
		fmt.Fprintf(&b, " (field %s)", strings.Join(quoteAll(e.Fields), ", "))
	}
	if e.Hint != "" {
		b.WriteString(" (")
		b.WriteString(e.Hint)
		b.WriteString(")")
	}
	if e.RateLimit != nil {
		if text := e.RateLimit.describe(); text != "" {
			b.WriteString(" (")
			b.WriteString(text)
			b.WriteString(")")
		}
	}
	if e.RequestID != "" {
		b.WriteString(" (request id ")
		b.WriteString(e.RequestID)
		b.WriteString(")")
	}
	return b.String()
}

// describe renders the rate-limit headers as one clause, e.g. "retry after
// 30s; limit 100, 0 remaining; window resets at 2026-11-10T10:00:00Z".
func (r *RateLimit) describe() string {
	var parts []string
	if r.HasRetryAfter {
		parts = append(parts, "retry after "+r.RetryAfter.String())
	}
	switch {
	case r.HasLimit && r.HasRemaining:
		parts = append(parts, fmt.Sprintf("limit %d, %d remaining", r.Limit, r.Remaining))
	case r.HasLimit:
		parts = append(parts, fmt.Sprintf("limit %d", r.Limit))
	case r.HasRemaining:
		parts = append(parts, fmt.Sprintf("%d remaining", r.Remaining))
	}
	if !r.Reset.IsZero() {
		parts = append(parts, "window resets at "+r.Reset.UTC().Format(time.RFC3339))
	}
	return strings.Join(parts, "; ")
}

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	return quoted
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

// licenseFeaturePattern matches the message n8n sends when a feature is missing
// from the instance license, e.g. "Your license does not allow for
// feat:variables. To enable feat:variables, ...".
var licenseFeaturePattern = regexp.MustCompile(`Your license does not allow for (feat:[A-Za-z0-9:_-]+)`)

// LicenseFeature reports the license feature an [APIError] says the instance
// lacks, e.g. "feat:variables". A 403 with this message is a license denial, not
// a scope one: the credential may well hold every scope the call needs.
func LicenseFeature(err error) (string, bool) {
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		return "", false
	}
	match := licenseFeaturePattern.FindStringSubmatch(apiErr.Message)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// errorBody covers the shapes the API uses for failures. Every field is raw
// because message and code arrive as string, number, array or nested object
// depending on the handler that produced them.
type errorBody struct {
	Message     json.RawMessage `json:"message"`
	Error       json.RawMessage `json:"error"`
	Code        json.RawMessage `json:"code"`
	Hint        json.RawMessage `json:"hint"`
	Description json.RawMessage `json:"description"`

	Reason                  json.RawMessage `json:"reason"`
	VersionID               json.RawMessage `json:"versionId"`
	WorkflowReviewRequestID json.RawMessage `json:"workflowReviewRequestId"`

	// Path and Keys come from a schema issue, the shape n8n answers a
	// rejected request body with.
	Path json.RawMessage `json:"path"`
	Keys json.RawMessage `json:"keys"`
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
	if resp.StatusCode == http.StatusTooManyRequests {
		apiErr.RateLimit = parseRateLimit(resp.Header, time.Now())
	}

	var decoded errorBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		apiErr.Body = truncateText(strings.TrimSpace(string(body)), 512)
		return apiErr
	}

	apiErr.Message = firstText(decoded.Message, decoded.Error, decoded.Description)
	apiErr.Code = firstText(decoded.Code)
	apiErr.Hint = firstText(decoded.Hint)
	apiErr.Reason = firstText(decoded.Reason)
	apiErr.VersionID = firstText(decoded.VersionID)
	apiErr.ReviewRequestID = firstText(decoded.WorkflowReviewRequestID)
	if resp.StatusCode == http.StatusBadRequest {
		apiErr.Fields = issueFields(decoded.Path, decoded.Keys)
	}
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

// issueFields names the request fields a schema issue points at: the first
// path element when it is a field name, then the unrecognized keys. Anything
// that is not a list of strings is ignored.
func issueFields(path, keys json.RawMessage) []string {
	var fields []string
	var elements []json.RawMessage
	if len(path) > 0 && json.Unmarshal(path, &elements) == nil && len(elements) > 0 {
		var first string
		if json.Unmarshal(elements[0], &first) == nil && first != "" {
			fields = append(fields, first)
		}
	}
	var names []string
	if len(keys) > 0 && json.Unmarshal(keys, &names) == nil {
		for _, name := range names {
			if name != "" && !slices.Contains(fields, name) {
				fields = append(fields, name)
			}
		}
	}
	return fields
}

// parseRateLimit reads the 429 headers. Retry-After is seconds or an HTTP
// date; malformed values are dropped rather than guessed at. It returns nil
// when no header was usable.
func parseRateLimit(h http.Header, now time.Time) *RateLimit {
	var r RateLimit
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds >= 0 {
			r.RetryAfter, r.HasRetryAfter = time.Duration(seconds)*time.Second, true
		} else if at, err := http.ParseTime(v); err == nil {
			r.RetryAfter, r.HasRetryAfter = max(at.Sub(now).Round(time.Second), 0), true
		}
	}
	if v, err := strconv.Atoi(strings.TrimSpace(h.Get("X-RateLimit-Limit"))); err == nil && v >= 0 {
		r.Limit, r.HasLimit = v, true
	}
	if v, err := strconv.Atoi(strings.TrimSpace(h.Get("X-RateLimit-Remaining"))); err == nil && v >= 0 {
		r.Remaining, r.HasRemaining = v, true
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(h.Get("X-RateLimit-Reset")), 10, 64); err == nil && v > 0 {
		r.Reset = time.Unix(v, 0).UTC()
	}
	if !r.HasRetryAfter && !r.HasLimit && !r.HasRemaining && r.Reset.IsZero() {
		return nil
	}
	return &r
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
