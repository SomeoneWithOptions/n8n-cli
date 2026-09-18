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

const cliLogStreamInput = `{"type":"webhook","url":"https://destination-secret@example.com/path?token=destination-secret","label":"Audit","enabled":false,"subscribedEvents":["n8n.audit"],"headerParameters":{"parameters":[{"name":"Authorization","value":"destination-secret"}]},"options":{"futureAuth":"destination-secret"},"futureSecret":"destination-secret"}`
const cliLogStreamResponse = `{"id":"dest-1","type":"webhook","url":"https://destination-secret@example.com","label":"Audit","enabled":false,"subscribedEvents":["n8n.audit"],"headerParameters":{"parameters":[{"name":"Authorization","value":"destination-secret"}]},"options":{"futureAuth":"destination-secret"},"futureSecret":"destination-secret"}`

func logStreamFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = cliLogStreamResponse
	return f
}
func logStreamInputFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "destination.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLogStreamCLIAllOperations(t *testing.T) {
	for _, output := range []string{"text", "json"} {
		for _, tt := range []struct {
			name                         string
			args                         []string
			method, path, response, want string
		}{
			{"events", []string{"event-type", "list"}, "GET", n8n.LogStreamEventTypesPath, `{"data":["n8n.workflow.started"]}`, "n8n.workflow.started"},
			{"list", []string{"destination", "list"}, "GET", n8n.LogStreamDestinationsPath, `{"data":[` + cliLogStreamResponse + `]}`, "dest-1"},
			{"get", []string{"destination", "get", "id /?#"}, "GET", n8n.LogStreamDestinationsPath + "/id%20%2F%3F%23", cliLogStreamResponse, "dest-1"},
			{"create", []string{"destination", "create", "--input", "-"}, "POST", n8n.LogStreamDestinationsPath, cliLogStreamResponse, "dest-1"},
			{"update", []string{"destination", "update", "dest-1", "--input", "-", "--yes"}, "PUT", n8n.LogStreamDestinationsPath + "/dest-1", cliLogStreamResponse, "dest-1"},
			{"test", []string{"destination", "test", "dest-1"}, "POST", n8n.LogStreamDestinationsPath + "/dest-1/test", `{"success":true}`, ""},
			{"delete", []string{"destination", "delete", "dest-1", "--yes"}, "DELETE", n8n.LogStreamDestinationsPath + "/dest-1", cliLogStreamResponse, "dest-1"},
		} {
			t.Run(output+"/"+tt.name, func(t *testing.T) {
				f := logStreamFixture(t)
				f.body = tt.response
				f.stdin = cliLogStreamInput
				args := append([]string{"log-stream"}, tt.args...)
				args = append(args, "--output", output)
				got := f.run(args...)
				if got.code != ExitSuccess || got.stderr != "" || !strings.Contains(got.stdout, tt.want) {
					t.Fatalf("result=%+v", got)
				}
				if strings.Contains(got.stdout+got.stderr, "destination-secret") {
					t.Error("secret leaked")
				}
				if output == "json" && !json.Valid([]byte(got.stdout)) {
					t.Error("invalid JSON output")
				}
				req := f.lastRequest()
				if req.Method != tt.method || req.URL.EscapedPath() != n8n.BasePath+tt.path || req.URL.RawQuery != "" || req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
					t.Errorf("wrong request: %s %s", req.Method, req.URL)
				}
				if tt.name == "create" || tt.name == "update" {
					if !strings.Contains(f.lastBody(), "destination-secret") || !strings.Contains(f.lastBody(), `"enabled":false`) {
						t.Error("wire secret or false field lost")
					}
				} else if f.lastBody() != "" {
					t.Error("unexpected request body")
				}
			})
		}
	}
}

func TestLogStreamStableRedactedJSON(t *testing.T) {
	f := logStreamFixture(t)
	got := f.run("log-stream", "destination", "get", "dest-1", "--output", "json")
	const want = `{
  "id": "dest-1",
  "type": "webhook",
  "label": "Audit",
  "enabled": false,
  "subscribedEvents": [
    "n8n.audit"
  ],
  "url": "[REDACTED]",
  "headerParameters": "[REDACTED]",
  "options": "[REDACTED]"
}
`
	if got.code != ExitSuccess || got.stdout != want {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestLogStreamConfirmation(t *testing.T) {
	for _, action := range []string{"update", "delete"} {
		t.Run(action, func(t *testing.T) {
			f := logStreamFixture(t)
			args := []string{"log-stream", "destination", action, "dest-1"}
			if action == "update" {
				args = append(args, "--input", logStreamInputFile(t, cliLogStreamInput))
			}
			before := f.requestCount()
			f.stdin = "n\n"
			got := f.run(args...)
			if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before || got.stdout != "" {
				t.Fatalf("refusal=%+v", got)
			}
			warning := "stop future delivery"
			if action == "update" {
				warning = "Changes apply immediately"
			}
			if !strings.Contains(got.stderr, warning) {
				t.Error("missing consequence warning")
			}
			f.interactive = false
			got = f.run(args...)
			if got.code != ExitError || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
				t.Fatalf("noninteractive=%+v", got)
			}
			f.interactive = true
			f.stdin = "yes\n"
			got = f.run(args...)
			if got.code != ExitSuccess || f.requestCount() != before+1 {
				t.Fatalf("confirmed=%+v", got)
			}
		})
	}
}

func TestLogStreamInputAndArgsRejectedBeforeTransport(t *testing.T) {
	for _, tt := range []struct {
		name  string
		args  []string
		input string
		want  string
	}{
		{"missing id", []string{"destination", "get"}, "", "arg"},
		{"blank id", []string{"destination", "get", " "}, "", "ID is required"},
		{"extra arg", []string{"event-type", "list", "extra"}, "", "unknown command"},
		{"missing input", []string{"destination", "create"}, "", "--input is required"},
		{"missing file", []string{"destination", "create", "--input", "/nonexistent/destination.json"}, "", "open destination input"},
		{"invalid output", []string{"destination", "list", "--output", "yaml"}, "", "output"},
		{"stdin confirmation", []string{"destination", "update", "id", "--input", "-"}, cliLogStreamInput, "--yes"},
		{"malformed", []string{"destination", "create", "--input", "-"}, `{"destination-secret":`, "valid JSON"},
		{"multiple docs", []string{"destination", "create", "--input", "-"}, cliLogStreamInput + ` {}`, "exactly one"},
		{"null", []string{"destination", "create", "--input", "-"}, `null`, "JSON object"},
		{"array", []string{"destination", "create", "--input", "-"}, `[]`, "JSON object"},
		{"missing variant", []string{"destination", "create", "--input", "-"}, `{}`, "type must be"},
		{"bad type", []string{"destination", "create", "--input", "-"}, `{"type":"webhook","url":123}`, "invalid JSON type"},
		{"redacted", []string{"destination", "update", "id", "--input", "-", "--yes"}, `{"type":"sentry","dsn":"[REDACTED]"}`, "restore original secrets"},
		{"oversized", []string{"destination", "create", "--input", "-"}, strings.Repeat(" ", maxLogStreamDocument+1), "exceeds"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := logStreamFixture(t)
			f.stdin = tt.input
			before := f.requestCount()
			got := f.run(append([]string{"log-stream"}, tt.args...)...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) || got.stdout != "" || f.requestCount() != before {
				t.Fatalf("result=%+v requests=%d", got, f.requestCount()-before)
			}
			if strings.Contains(got.stderr, "destination-secret") {
				t.Error("input secret leaked")
			}
		})
	}
}

func TestLogStreamErrors(t *testing.T) {
	for _, action := range []string{"list", "get", "create", "update", "test", "delete"} {
		for _, status := range []int{400, 401, 403, 404, 409, 503} {
			t.Run(action+http.StatusText(status), func(t *testing.T) {
				f := logStreamFixture(t)
				f.status = status
				f.body = `{"message":"destination-secret","code":"destination-secret"}`
				f.stdin = cliLogStreamInput
				args := []string{"log-stream", "destination", action}
				if action != "list" && action != "create" {
					args = append(args, "id")
				}
				if action == "create" || action == "update" {
					args = append(args, "--input", "-")
				}
				if action == "delete" || action == "update" {
					args = append(args, "--yes")
				}
				got := f.run(args...)
				if got.code != ExitError || got.stdout != "" || strings.Contains(got.stderr, "destination-secret") {
					t.Fatalf("result=%+v", got)
				}
				wants := []string{}
				if status == 403 {
					scope := action
					if scope == "get" {
						scope = "read"
					}
					wants = []string{"Log Streaming license", "eventBusDestination:" + scope, "n8n discover"}
				}
				if status == 409 {
					wants = []string{"environment variables", "nothing changed", "restart n8n"}
				}
				for _, want := range wants {
					if !strings.Contains(got.stderr, want) {
						t.Errorf("missing %q: %s", want, got.stderr)
					}
				}
			})
		}
	}
}

func TestLogStreamEmptyAndFailedDelivery(t *testing.T) {
	for _, tt := range []struct {
		args           []string
		response, want string
	}{
		{[]string{"event-type", "list"}, `{"data":[]}`, "No streamable"},
		{[]string{"destination", "list"}, `{"data":[]}`, "No log streaming"},
		{[]string{"destination", "list", "--output", "json"}, `{"data":null}`, "\"data\": []"},
		{[]string{"destination", "test", "id"}, `{"success":false,"error":"destination-secret"}`, "Test message delivered: no"},
		{[]string{"destination", "test", "id", "--output", "json"}, `{"success":false}`, "\"success\": false"},
	} {
		f := logStreamFixture(t)
		f.body = tt.response
		got := f.run(append([]string{"log-stream"}, tt.args...)...)
		if got.code != ExitSuccess || got.stderr != "" || !strings.Contains(got.stdout, tt.want) || strings.Contains(got.stdout, "destination-secret") {
			t.Fatalf("result=%+v", got)
		}
	}
}
