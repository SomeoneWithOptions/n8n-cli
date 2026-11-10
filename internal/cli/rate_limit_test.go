package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliRateLimitSettings = `{
  "enabled":true,
  "globalReadLimit":600,
  "globalReadWindowSeconds":60,
  "globalWriteLimit":120,
  "globalWriteWindowSeconds":60,
  "endpointReadLimit":0,
  "endpointReadWindowSeconds":60,
  "endpointWriteLimit":30,
  "endpointWriteWindowSeconds":10,
  "envManagedFields":["globalReadLimit"]
}`

const cliRateLimitInput = `{"enabled":true,"globalReadLimit":600,"globalReadWindowSeconds":60,"globalWriteLimit":120,"globalWriteWindowSeconds":60,"endpointReadLimit":0,"endpointReadWindowSeconds":60,"endpointWriteLimit":30,"endpointWriteWindowSeconds":10}`

func rateLimitFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = cliRateLimitSettings
	return f
}

func writeRateLimitInput(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rate-limits.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRateLimitGetText(t *testing.T) {
	f := rateLimitFixture(t)
	got := f.run("rate-limit", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"Rate-limit settings:", f.server.URL, "Enabled:", "yes", "Env-managed fields:", "globalReadLimit",
		"SCOPE", "global", "read", "600", "60s", "endpoint", "write", "30", "10s", "none",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.RateLimitSettingsPath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
}

func TestRateLimitGetJSONRoundTripsIntoSet(t *testing.T) {
	f := rateLimitFixture(t)
	got := f.run("rate-limit", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var settings n8n.RateLimitSettings
	if err := json.Unmarshal([]byte(got.stdout), &settings); err != nil {
		t.Fatalf("stdout is not settings JSON: %v\n%s", err, got.stdout)
	}
	f.stdin = got.stdout
	set := f.run("rate-limit", "set", "--input", "-", "--yes", "--endpoint-write-limit", "45")
	if set.code != ExitSuccess {
		t.Fatalf("set exit = %d, stderr: %s", set.code, set.stderr)
	}
	if !strings.Contains(set.stderr, "Dropped read-only field envManagedFields.") {
		t.Errorf("stderr does not report the dropped field: %s", set.stderr)
	}
	if !strings.Contains(set.stdout, "Updated rate-limit settings:") {
		t.Errorf("stdout = %s", set.stdout)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.RateLimitSettingsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, ok := sent["envManagedFields"]; ok {
		t.Error("read-only envManagedFields was sent")
	}
	if len(sent) != 9 || sent["endpointWriteLimit"] != float64(45) || sent["globalReadLimit"] != float64(600) {
		t.Errorf("body = %s", f.lastBody())
	}
}

func TestRateLimitSetFromFlagsOnly(t *testing.T) {
	f := rateLimitFixture(t)
	got := f.run("rate-limit", "set", "--yes", "--output", "json", "--enabled=false",
		"--global-read-limit", "0", "--global-read-window-seconds", "60",
		"--global-write-limit", "0", "--global-write-window-seconds", "60",
		"--endpoint-read-limit", "0", "--endpoint-read-window-seconds", "60",
		"--endpoint-write-limit", "0", "--endpoint-write-window-seconds", "60")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if sent["enabled"] != false || len(sent) != 9 {
		t.Errorf("body = %s", f.lastBody())
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
}

func TestRateLimitSetRequiresConfirmation(t *testing.T) {
	f := rateLimitFixture(t)
	path := writeRateLimitInput(t, cliRateLimitInput)
	before := f.requestCount()

	f.stdin = "n\n"
	got := f.run("rate-limit", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Fatalf("declined result = %+v", got)
	}
	if !strings.Contains(got.stderr, "including the one this CLI uses") {
		t.Errorf("confirmation lacks the warning: %s", got.stderr)
	}
	if f.requestCount() != before {
		t.Error("declined replacement reached instance")
	}

	f.stdin = "yes\n"
	got = f.run("rate-limit", "set", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	if f.lastRequest().Method != http.MethodPut {
		t.Errorf("method = %s, want PUT", f.lastRequest().Method)
	}
}

func TestRateLimitSetValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "nothing given", args: []string{"rate-limit", "set", "--yes"}, want: "enabled is required and no --input was given: pass --enabled"},
		{name: "stdin without yes", input: cliRateLimitInput, args: []string{"rate-limit", "set", "--input", "-"}, want: "consumes stdin"},
		{name: "unknown output", input: cliRateLimitInput, args: []string{"rate-limit", "set", "--input", "-", "--yes", "--output", "yaml"}, want: "unknown output format"},
		{name: "malformed", input: `{`, args: []string{"rate-limit", "set", "--input", "-", "--yes"}, want: "decode rate-limit settings JSON"},
		{name: "unknown field", input: `{"enabled":true,"surprise":1}`, args: []string{"rate-limit", "set", "--input", "-", "--yes"}, want: "unknown field"},
		{name: "trailing document", input: cliRateLimitInput + ` {}`, args: []string{"rate-limit", "set", "--input", "-", "--yes"}, want: "multiple documents"},
		{name: "input omits field", input: `{"enabled":true,"globalReadLimit":1}`, args: []string{"rate-limit", "set", "--input", "-", "--yes"}, want: "globalReadWindowSeconds is required and --input omits it: pass --global-read-window-seconds"},
		{name: "negative limit flag", input: cliRateLimitInput, args: []string{"rate-limit", "set", "--input", "-", "--yes", "--global-write-limit", "-1"}, want: "globalWriteLimit must be at least 0"},
		{name: "zero window flag", input: cliRateLimitInput, args: []string{"rate-limit", "set", "--input", "-", "--yes", "--endpoint-read-window-seconds", "0"}, want: "endpointReadWindowSeconds must be at least 1"},
		{name: "enabled without limits", input: `{"enabled":true,"globalReadLimit":0,"globalReadWindowSeconds":60,"globalWriteLimit":0,"globalWriteWindowSeconds":60,"endpointReadLimit":0,"endpointReadWindowSeconds":60,"endpointWriteLimit":0,"endpointWriteWindowSeconds":60}`, args: []string{"rate-limit", "set", "--input", "-", "--yes"}, want: "every limit is 0"},
		{name: "oversized", input: strings.Repeat("x", maxRateLimitDocument+1), args: []string{"rate-limit", "set", "--input", "-", "--yes"}, want: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := rateLimitFixture(t)
			f.stdin = tt.input
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid rate-limit request reached instance")
			}
		})
	}
}

func TestRateLimitAPIErrorsStateTheFix(t *testing.T) {
	setArgs := []string{"rate-limit", "set", "--input", "-", "--yes"}
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "get forbidden", status: http.StatusForbidden, args: []string{"rate-limit", "get"}, want: []string{"securitySettings:manage", "discover --resource settingsratelimits"}},
		{name: "set forbidden", status: http.StatusForbidden, args: setArgs, want: []string{"securitySettings:manage"}},
		{name: "env managed conflict", status: http.StatusConflict, args: setArgs, want: []string{"409", "environment variables", "nothing changed", "rate-limit get"}},
		{name: "invalid replacement", status: http.StatusBadRequest, args: setArgs, want: []string{"full rate-limit replacement", "at least one limit above 0"}},
		{name: "older instance", status: http.StatusNotFound, args: []string{"rate-limit", "get"}, want: []string{"n8n 2.47 or later", "discover --resource settingsratelimits"}},
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"rate-limit", "get"}, want: []string{"rejected the credential", "auth login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := rateLimitFixture(t)
			f.status = tt.status
			f.stdin = cliRateLimitInput
			got := f.run(tt.args...)
			if got.code != ExitError {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q: %s", want, got.stderr)
				}
			}
		})
	}
}
