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

const (
	cliOtelSecret   = "collector-cli-secret"
	cliOtelSettings = `{
  "enabled":true,
  "exporterProtocol":"grpc",
  "exporterEndpoint":"https://collector.example.com:4317",
  "exporterTracingPath":"/v1/traces",
  "exporterServiceName":"n8n-production",
  "exporterHeaders":"authorization=Bearer collector-cli-secret",
  "tracesSampleRate":0.25,
  "startupConnectivityTimeoutMs":2500,
  "includeNodeSpans":false,
  "injectOutbound":true,
  "productionExecutionsOnly":true
}`
	cliOtelDefaultInput = `{
  "enabled":false,
  "exporterEndpoint":"http://collector.example.com:4318",
  "exporterTracingPath":"/v1/traces",
  "exporterServiceName":"n8n",
  "exporterHeaders":"",
  "tracesSampleRate":0,
  "startupConnectivityTimeoutMs":0,
  "includeNodeSpans":false,
  "injectOutbound":false,
  "productionExecutionsOnly":false
}`
	cliOtelTraceInput = `{
  "exporterEndpoint":"http://collector.example.com:4318",
  "exporterTracingPath":"/v1/traces",
  "exporterServiceName":"n8n-test",
  "exporterHeaders":"authorization=Bearer collector-cli-secret",
  "startupConnectivityTimeoutMs":2000
}`
)

func otelFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = cliOtelSettings
	return f
}

func writeOtelInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "otel.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestOtelGetTextRedactsHeadersAndShowsEverySetting(t *testing.T) {
	f := otelFixture(t)
	got := f.run("otel", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"OpenTelemetry settings:", f.server.URL, "Enabled:", "yes", "Exporter protocol:", "grpc",
		"Exporter endpoint:", "https://collector.example.com:4317", "Exporter tracing path:", "/v1/traces",
		"Exporter service name:", "n8n-production", "Exporter headers:", "configured (value hidden)",
		"Trace sample rate:", "0.25", "Startup connectivity timeout:", "2500 ms",
		"Include node spans:", "no", "Inject outbound trace context:", "yes", "Production executions only:", "yes",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, cliOtelSecret) {
		t.Error("text output leaked exporter headers")
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.OtelSettingsPath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
	if got := req.Header.Get(n8n.HeaderAPIKey); got != testAPIKey {
		t.Errorf("%s = %q, want API key", n8n.HeaderAPIKey, got)
	}
}

func TestOtelGetJSONIsStableAndIncludesEditDocument(t *testing.T) {
	f := otelFixture(t)
	got := f.run("otel", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	const want = `{
  "enabled": true,
  "exporterProtocol": "grpc",
  "exporterEndpoint": "https://collector.example.com:4317",
  "exporterTracingPath": "/v1/traces",
  "exporterServiceName": "n8n-production",
  "exporterHeaders": "authorization=Bearer collector-cli-secret",
  "tracesSampleRate": 0.25,
  "startupConnectivityTimeoutMs": 2500,
  "includeNodeSpans": false,
  "injectOutbound": true,
  "productionExecutionsOnly": true
}
`
	if got.stdout != want {
		t.Errorf("stdout =\n%s\nwant stable JSON =\n%s", got.stdout, want)
	}
}

func TestOtelSetRequiresReplacementConfirmationAndWarnsAboutRuntime(t *testing.T) {
	f := otelFixture(t)
	path := writeOtelInput(t, cliOtelDefaultInput)
	before := f.requestCount()

	f.stdin = "n\n"
	got := f.run("otel", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Fatalf("declined result = %+v", got)
	}
	for _, want := range []string{"Fully replace OpenTelemetry settings", "running instance immediately", "change or stop exported traces"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("confirmation missing %q: %s", want, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Error("declined replacement reached instance")
	}

	f.stdin = "yes\n"
	got = f.run("otel", "set", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "Applied immediately to running instance:") {
		t.Errorf("stdout lacks runtime application result:\n%s", got.stdout)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.OtelSettingsPath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
}

func TestOtelSetFromStdinPreservesFalseAndUsesDefaultProtocol(t *testing.T) {
	f := otelFixture(t)
	f.stdin = cliOtelDefaultInput
	got := f.run("otel", "set", "--input", "-", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var settings n8n.OtelSettings
	if err := json.Unmarshal([]byte(got.stdout), &settings); err != nil {
		t.Fatalf("stdout is not settings JSON: %v\n%s", err, got.stdout)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if _, ok := sent["exporterProtocol"]; ok {
		t.Errorf("omitted exporterProtocol was sent: %#v", sent)
	}
	for _, field := range []string{"enabled", "includeNodeSpans", "injectOutbound", "productionExecutionsOnly"} {
		if value, ok := sent[field].(bool); !ok || value {
			t.Errorf("%s = %#v, want false", field, sent[field])
		}
	}
	if sent["tracesSampleRate"] != float64(0) || sent["startupConnectivityTimeoutMs"] != float64(0) {
		t.Errorf("zero values not preserved: %#v", sent)
	}
}

func TestOtelSetStdinRequiresExplicitConfirmation(t *testing.T) {
	f := otelFixture(t)
	f.stdin = cliOtelDefaultInput
	before := f.requestCount()
	got := f.run("otel", "set", "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "consumes stdin") || !strings.Contains(got.stderr, "--yes") {
		t.Errorf("result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("unconfirmed stdin replacement reached instance")
	}
}

func TestOtelTestTraceReportsAcceptedAndRejectedResults(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response string
		args     []string
		want     []string
	}{
		{name: "accepted text", response: `{"success":true}`, want: []string{"Test span accepted:", "yes", "Instance:"}},
		{name: "rejected text", response: `{"success":false,"error":"401 Unauthorized; echoed Bearer collector-cli-secret"}`, want: []string{"Test span accepted:", "no", "Error:", "401 Unauthorized", "[redacted]"}},
		{name: "rejected json", response: `{"success":false,"error":"timeout"}`, args: []string{"--output", "json"}, want: []string{`"success": false`, `"error": "timeout"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := otelFixture(t)
			f.body = tt.response
			f.stdin = cliOtelTraceInput
			args := append([]string{"otel", "test-trace", "--input", "-"}, tt.args...)
			got := f.run(args...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("stdout missing %q:\n%s", want, got.stdout)
				}
			}
			if strings.Contains(got.stdout, cliOtelSecret) || strings.Contains(got.stderr, cliOtelSecret) {
				t.Error("test output leaked exporter headers")
			}
			req := f.lastRequest()
			if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+n8n.OtelTestTracePath {
				t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
			}
			var sent map[string]any
			if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if _, ok := sent["exporterProtocol"]; ok {
				t.Error("test request did not preserve omitted default protocol")
			}
		})
	}
}

func TestOtelSetValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "input required", args: []string{"otel", "set"}, want: "--input is required"},
		{name: "unknown output", args: []string{"otel", "set", "--input", "missing", "--output", "yaml"}, want: "unknown output format"},
		{name: "malformed", input: `{`, args: []string{"otel", "set", "--input", "-", "--yes"}, want: "decode OpenTelemetry settings JSON"},
		{name: "unknown field", input: strings.TrimSuffix(cliOtelDefaultInput, "}") + `,"surprise":true}`, args: []string{"otel", "set", "--input", "-", "--yes"}, want: "unknown field"},
		{name: "trailing document", input: cliOtelDefaultInput + `{}`, args: []string{"otel", "set", "--input", "-", "--yes"}, want: "multiple documents"},
		{name: "missing required", input: `{"enabled":false}`, args: []string{"otel", "set", "--input", "-", "--yes"}, want: "exporterEndpoint"},
		{name: "invalid protocol", input: strings.Replace(cliOtelDefaultInput, `"enabled":false,`, `"enabled":false,"exporterProtocol":"json",`, 1), args: []string{"otel", "set", "--input", "-", "--yes"}, want: "http/protobuf or grpc"},
		{name: "invalid endpoint", input: strings.Replace(cliOtelDefaultInput, "http://collector.example.com:4318", "ftp://collector.example.com", 1), args: []string{"otel", "set", "--input", "-", "--yes"}, want: "http:// or https://"},
		{name: "sample out of range", input: strings.Replace(cliOtelDefaultInput, `"tracesSampleRate":0`, `"tracesSampleRate":2`, 1), args: []string{"otel", "set", "--input", "-", "--yes"}, want: "between 0 and 1"},
		{name: "oversized", input: strings.Repeat("x", maxOtelDocument+1), args: []string{"otel", "set", "--input", "-", "--yes"}, want: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := otelFixture(t)
			f.stdin = tt.input
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid settings request reached instance")
			}
		})
	}
}

func TestOtelTestTraceValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "input required", args: []string{"otel", "test-trace"}, want: "--input is required"},
		{name: "unknown output", args: []string{"otel", "test-trace", "--input", "-", "--output", "yaml"}, want: "unknown output format"},
		{name: "unknown field", input: strings.TrimSuffix(cliOtelTraceInput, "}") + `,"enabled":true}`, args: []string{"otel", "test-trace", "--input", "-"}, want: "unknown field"},
		{name: "missing headers", input: `{"exporterEndpoint":"http://localhost:4318","exporterTracingPath":"/v1/traces","exporterServiceName":"n8n","startupConnectivityTimeoutMs":1}`, args: []string{"otel", "test-trace", "--input", "-"}, want: "exporterHeaders"},
		{name: "negative timeout", input: strings.Replace(cliOtelTraceInput, "2000", "-1", 1), args: []string{"otel", "test-trace", "--input", "-"}, want: "at least 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := otelFixture(t)
			f.stdin = tt.input
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid test-trace request reached instance")
			}
		})
	}
}

func TestOtelAPIErrorsExplainScopeReplacementAndEnvironmentOwnership(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		input  string
		want   []string
	}{
		{name: "get scope failure", status: http.StatusForbidden, args: []string{"otel", "get"}, want: []string{"otel:manage", "discover --resource otel"}},
		{name: "set scope failure", status: http.StatusForbidden, args: []string{"otel", "set", "--input", "-", "--yes"}, input: cliOtelDefaultInput, want: []string{"otel:manage", "discover --resource otel"}},
		{name: "test scope failure", status: http.StatusForbidden, args: []string{"otel", "test-trace", "--input", "-"}, input: cliOtelTraceInput, want: []string{"otel:manage", "discover --resource otel"}},
		{name: "invalid replacement", status: http.StatusBadRequest, args: []string{"otel", "set", "--input", "-", "--yes"}, input: cliOtelDefaultInput, want: []string{"full OpenTelemetry replacement", "every field", "http/protobuf", "redacted"}},
		{name: "invalid test", status: http.StatusBadRequest, args: []string{"otel", "test-trace", "--input", "-"}, input: cliOtelTraceInput, want: []string{"test connection details", "endpoint", "redacted"}},
		{name: "environment conflict", status: http.StatusConflict, args: []string{"otel", "set", "--input", "-", "--yes"}, input: cliOtelDefaultInput, want: []string{"409", "managed by environment variables", "no changes were made", "otel get", "restart n8n"}},
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"otel", "get"}, want: []string{"rejected the credential", "auth login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := otelFixture(t)
			f.status = tt.status
			f.stdin = tt.input
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
