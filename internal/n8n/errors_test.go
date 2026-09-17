package n8n

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// decodeError builds an APIError the way the transport does, from a status,
// headers and a raw body.
func decodeError(t *testing.T, status int, header http.Header, body string) *APIError {
	t.Helper()
	if header == nil {
		header = http.Header{}
	}
	resp := &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     header,
	}
	data, truncated, err := readLimited(strings.NewReader(body), maxErrorBody)
	if err != nil {
		t.Fatalf("readLimited: %v", err)
	}
	return newAPIError(resp, http.MethodGet, "/workflows/42", data, truncated)
}

func TestAPIErrorDecoding(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
		wantCode    string
		wantHint    string
		wantBody    string
	}{
		{
			name:        "message field",
			body:        `{"message":"workflow not found"}`,
			wantMessage: "workflow not found",
		},
		{
			name:        "error field",
			body:        `{"error":"unauthorized"}`,
			wantMessage: "unauthorized",
		},
		{
			name:        "description field",
			body:        `{"description":"license required"}`,
			wantMessage: "license required",
		},
		{
			name:        "message, code and hint",
			body:        `{"message":"forbidden","code":"NO_SCOPE","hint":"run n8n discover"}`,
			wantMessage: "forbidden",
			wantCode:    "NO_SCOPE",
			wantHint:    "run n8n discover",
		},
		{
			name:        "numeric code",
			body:        `{"message":"nope","code":1042}`,
			wantMessage: "nope",
			wantCode:    "1042",
		},
		{
			name:        "array of messages",
			body:        `{"message":["name is required","id must be a string"]}`,
			wantMessage: "name is required; id must be a string",
		},
		{
			name:        "nested error object",
			body:        `{"error":{"message":"inner failure"}}`,
			wantMessage: "inner failure",
		},
		{
			name:        "message preferred over error",
			body:        `{"message":"outer","error":"inner"}`,
			wantMessage: "outer",
		},
		{
			name:     "unknown json shape is preserved",
			body:     `{"detail":"something else"}`,
			wantBody: `{"detail":"something else"}`,
		},
		{
			name:     "html from a proxy",
			body:     "<html><body>502 Bad Gateway</body></html>",
			wantBody: "<html><body>502 Bad Gateway</body></html>",
		},
		{
			name: "empty body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeError(t, http.StatusNotFound, nil, tt.body)
			if got.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMessage)
			}
			if got.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", got.Hint, tt.wantHint)
			}
			if got.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", got.Body, tt.wantBody)
			}
			if got.StatusCode != http.StatusNotFound {
				t.Errorf("StatusCode = %d, want 404", got.StatusCode)
			}
		})
	}
}

func TestAPIErrorMessage(t *testing.T) {
	err := decodeError(t, http.StatusForbidden,
		http.Header{"X-Request-Id": {"req-123"}},
		`{"message":"missing scope","code":"NO_SCOPE","hint":"see n8n discover"}`)

	for _, want := range []string{
		"GET /workflows/42", "403 Forbidden", "[NO_SCOPE]",
		"missing scope", "see n8n discover", "request id req-123",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", err, want)
		}
	}
}

func TestAPIErrorMessageWithoutStatusText(t *testing.T) {
	err := &APIError{StatusCode: 599}
	if got := err.Error(); got != "HTTP 599" {
		t.Errorf("Error() = %q, want %q", got, "HTTP 599")
	}
}

func TestAPIErrorTruncatesOversizedBody(t *testing.T) {
	body := strings.Repeat("A", maxErrorBody*2)
	got := decodeError(t, http.StatusBadGateway, nil, body)

	if !got.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(got.Body) > 512+3 {
		t.Errorf("len(Body) = %d, want it capped at 515", len(got.Body))
	}
	if !strings.HasSuffix(got.Body, "...") {
		t.Errorf("Body = %q, want a truncation marker", got.Body)
	}
	if len(got.Error()) > 1024 {
		t.Errorf("len(Error()) = %d, want a readable message", len(got.Error()))
	}
}

func TestRequestIDHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		// net/http canonicalizes header keys when parsing a response, so the
		// canonical spelling is the only one that can reach this function.
		{"x-request-id", http.Header{"X-Request-Id": {"a"}}, "a"},
		{"correlation id", http.Header{"X-Correlation-Id": {"b"}}, "b"},
		{"trace id", http.Header{"X-Amzn-Trace-Id": {"c"}}, "c"},
		{"request id wins", http.Header{"X-Request-Id": {"a"}, "X-Correlation-Id": {"b"}}, "a"},
		{"blank ignored", http.Header{"X-Request-Id": {"  "}, "X-Correlation-Id": {"b"}}, "b"},
		{"none", http.Header{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestID(tt.header); got != tt.want {
				t.Errorf("requestID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusHelpers(t *testing.T) {
	tests := []struct {
		status int
		check  func(error) bool
		name   string
	}{
		{http.StatusUnauthorized, IsUnauthorized, "IsUnauthorized"},
		{http.StatusForbidden, IsForbidden, "IsForbidden"},
		{http.StatusNotFound, IsNotFound, "IsNotFound"},
		{http.StatusConflict, IsConflict, "IsConflict"},
		{http.StatusTooManyRequests, IsRateLimited, "IsRateLimited"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrapped, because callers add context before the CLI inspects it.
			err := fmt.Errorf("listing workflows: %w", &APIError{StatusCode: tt.status})
			if !tt.check(err) {
				t.Errorf("%s(%d) = false, want true", tt.name, tt.status)
			}
			if tt.check(&APIError{StatusCode: http.StatusTeapot}) {
				t.Errorf("%s(418) = true, want false", tt.name)
			}
			if tt.check(errors.New("plain")) {
				t.Errorf("%s(non-API error) = true, want false", tt.name)
			}
			if got := StatusCodeOf(err); got != tt.status {
				t.Errorf("StatusCodeOf() = %d, want %d", got, tt.status)
			}
		})
	}

	if got := StatusCodeOf(errors.New("plain")); got != 0 {
		t.Errorf("StatusCodeOf(non-API error) = %d, want 0", got)
	}
	if got := StatusCodeOf(nil); got != 0 {
		t.Errorf("StatusCodeOf(nil) = %d, want 0", got)
	}
}
