package cli

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	cliIdempotencyInProgress = `{"message":"A request with this Idempotency-Key is already in progress."}`
	cliIdempotencyMismatch   = `{"message":"This Idempotency-Key was already used with a different request."}`
)

// idempotentCommand is one command that takes --idempotency-key, with the
// arguments and response a successful non-interactive run needs.
type idempotentCommand struct {
	name     string
	args     func(t *testing.T) []string
	path     string
	response string
}

func idempotentCommands() []idempotentCommand {
	fixed := func(args ...string) func(*testing.T) []string {
		return func(*testing.T) []string { return args }
	}
	expected := []string{"--expected-config-id", "cfg", "--expected-branch", "main", "--expected-commit", cliSelectionSHA}
	applied := `{"status":"applied","counts":{},"warnings":[]}`
	return []idempotentCommand{
		{"execution stop", fixed("execution", "stop", "1000", "--yes"), "/executions/1000/stop", `{"status":"canceled"}`},
		{"execution stop-many", fixed("execution", "stop-many", "--status", "running", "--workflow-id", "wf-1", "--yes"), "/executions/stop", `{"stopped":1}`},
		{"execution retry", fixed("execution", "retry", "1000"), "/executions/1000/retry", cliExecution},
		{"workflow create", fixed("workflow", "create", "--name", "Scratch"), "/workflows", `{"id":"wf-1","name":"Scratch","nodes":[],"connections":{}}`},
		{"row insert", fixed("data-table", "row", "insert", "dt-1", "--set", "age=30"), "/data-tables/dt-1/rows", `{"success":true,"insertedRows":1}`},
		{"row upsert", fixed("data-table", "row", "upsert", "dt-1", "--where", "email=a@example.com", "--set", "age=31"), "/data-tables/dt-1/rows/upsert", `true`},
		{"promotion promote", fixed("promotion", "promote", "conn-1", "--commit-message", "sync", "--yes"), "/promotions/connections/conn-1/promote", cliPromotionPromoteBody},
		{"promotion apply", fixed("promotion", "apply", "conn-1", "--yes"), "/promotions/connections/conn-1/apply", applied},
		{"promotion apply-continue", fixed(append([]string{"promotion", "apply-continue", "conn-1", "--yes"}, expected...)...), "/promotions/connections/conn-1/apply/continue", applied},
		{"project apply", fixed("promotion", "project", "apply", "proj-1", "--workflow-id", "wf", "--yes"), "/promotions/projects/proj-1/apply", applied},
		{"project apply-continue", fixed(append([]string{"promotion", "project", "apply-continue", "proj-1", "--workflow-id", "wf", "--yes"}, expected...)...), "/promotions/projects/proj-1/apply/continue", applied},
		{"project promote", fixed("promotion", "project", "promote", "proj-1", "--workflow-id", "wf", "--yes"), "/promotions/projects/proj-1/promote", cliPromotionPromoteBody},
		{"package import", func(t *testing.T) []string {
			return []string{"package", "import", "--file", writePackageArchive(t, cliPackageArchive), "--workflow-conflict-policy", "fail", "--yes"}
		}, n8n.N8nPackageImportPath, cliPackageImportResult},
		{"package import-selection", func(t *testing.T) []string {
			return []string{"package", "import-selection", "--file", writePackageArchive(t, cliPackageArchive), "--project-id", "p1", "--workflow-id", "wf-1", "--yes"}
		}, n8n.N8nPackageImportSelectionPath, cliPackageImportResult},
	}
}

func TestIdempotencyKeyHeaderOnlyWhenSet(t *testing.T) {
	for _, tc := range idempotentCommands() {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.login()
			f.interactive = false
			f.body = tc.response
			args := tc.args(t)

			got := f.run(args...)
			if got.code != ExitSuccess {
				t.Fatalf("without key: exit = %d, stderr: %s", got.code, got.stderr)
			}
			if values, sent := f.lastRequest().Header[n8n.IdempotencyKeyHeader]; sent {
				t.Errorf("without key: %s = %q, want no header", n8n.IdempotencyKeyHeader, values)
			}
			plainBody := f.lastBody()

			got = f.run(append(args, "--idempotency-key", "cli-key-1")...)
			if got.code != ExitSuccess {
				t.Fatalf("with key: exit = %d, stderr: %s", got.code, got.stderr)
			}
			req := f.lastRequest()
			if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+tc.path {
				t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.EscapedPath(), n8n.BasePath+tc.path)
			}
			if key := req.Header.Get(n8n.IdempotencyKeyHeader); key != "cli-key-1" {
				t.Errorf("%s = %q, want cli-key-1", n8n.IdempotencyKeyHeader, key)
			}
			if strings.Contains(got.stderr, "Idempotency key:") {
				t.Errorf("stderr = %q, want no announcement for a literal key", got.stderr)
			}
			if !strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/") && f.lastBody() != plainBody {
				t.Errorf("body with key = %q, without = %q; the key must not change the body", f.lastBody(), plainBody)
			}
		})
	}
}

func TestIdempotencyKeyAutoIsPrintedOnStderr(t *testing.T) {
	f := executionFixture(t, cliExecution)
	got := f.run("execution", "retry", "1000", "--idempotency-key", "auto", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	sent := f.lastRequest().Header.Get(n8n.IdempotencyKeyHeader)
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV4.MatchString(sent) {
		t.Fatalf("%s = %q, want a generated version 4 UUID", n8n.IdempotencyKeyHeader, sent)
	}
	if !strings.Contains(got.stderr, sent) || !strings.Contains(got.stderr, "--idempotency-key "+sent) {
		t.Errorf("stderr = %q, want the generated key and how to retry with it", got.stderr)
	}
	if strings.Contains(got.stdout, sent) {
		t.Errorf("stdout = %q, want the key on stderr only", got.stdout)
	}
	var execution n8n.Execution
	if err := json.Unmarshal([]byte(got.stdout), &execution); err != nil || execution.ID != "1000" {
		t.Errorf("stdout = %q, want only the execution JSON (error: %v)", got.stdout, err)
	}

	got = f.run("execution", "retry", "1000", "--idempotency-key", "auto")
	if again := f.lastRequest().Header.Get(n8n.IdempotencyKeyHeader); again == sent || got.code != ExitSuccess {
		t.Errorf("second auto key = %q (exit %d), want a fresh key", again, got.code)
	}
}

func TestIdempotencyKeyInvalidRejectedBeforeTransport(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.interactive = false
	for _, tc := range idempotentCommands() {
		for _, key := range []string{"two words", strings.Repeat("k", n8n.MaxIdempotencyKeyLength+1), "clé"} {
			before := f.requestCount()
			got := f.run(append(tc.args(t), "--idempotency-key", key)...)
			if got.code != ExitError || !strings.Contains(got.stderr, "--idempotency-key") {
				t.Errorf("%s with %q: exit = %d, stderr = %q, want a flag error", tc.name, key, got.code, got.stderr)
			}
			if f.requestCount() != before {
				t.Errorf("%s with %q: request reached the instance", tc.name, key)
			}
		}
	}
}

func TestIdempotencyRefusalMessages(t *testing.T) {
	for _, tc := range idempotentCommands() {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.login()
			f.interactive = false
			args := tc.args(t)
			for _, refusal := range []struct {
				status   int
				body     string
				want     []string
				notFound string
			}{
				{http.StatusConflict, cliIdempotencyInProgress, []string{"(409)", "still running", "retry later with the same key", `"cli-key-1"`, "already in progress"}, "new key"},
				{http.StatusUnprocessableEntity, cliIdempotencyMismatch, []string{"(422)", "different request", "new key", "--idempotency-key auto", `"cli-key-1"`}, "retry later"},
			} {
				f.route = func(*http.Request, int) (int, string) { return refusal.status, refusal.body }
				got := f.run(append(args, "--idempotency-key", "cli-key-1")...)
				if got.code != ExitError {
					t.Fatalf("%d: exit = %d, want error; stderr: %s", refusal.status, got.code, got.stderr)
				}
				for _, want := range refusal.want {
					if !strings.Contains(got.stderr, want) {
						t.Errorf("%d: stderr = %q, want %q", refusal.status, got.stderr, want)
					}
				}
				if strings.Contains(got.stderr, refusal.notFound) {
					t.Errorf("%d: stderr = %q, want no %q", refusal.status, got.stderr, refusal.notFound)
				}
			}
		})
	}
}

// TestIdempotencyKeyKeepsResourceConflicts checks that a 409 the resource
// itself sends keeps its own diagnostic when a key was set.
func TestIdempotencyKeyKeepsResourceConflicts(t *testing.T) {
	f := executionFixture(t, "")
	f.route = func(*http.Request, int) (int, string) {
		return http.StatusConflict, `{"message":"The execution cannot be retried"}`
	}
	got := f.run("execution", "retry", "1000", "--idempotency-key", "cli-key-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "cannot retry execution") || strings.Contains(got.stderr, "still running") {
		t.Errorf("result = %+v, want the retry conflict diagnostic", got)
	}
}
