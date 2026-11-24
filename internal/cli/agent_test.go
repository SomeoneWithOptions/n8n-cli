package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliAgentSummary = `{"id":"ag-1","name":"Support","projectId":"p1","availableInMCP":true,"isArchived":false,"versionId":"v2","activeVersionId":"v1","createdAt":"2026-11-01T00:00:00.000Z","updatedAt":"2026-11-02T00:00:00.000Z"}`

const cliAgentDetail = `{"id":"ag-1","name":"Support","projectId":"p1","project":{"id":"p1","name":"Ops","type":"team"},"availableInMCP":false,"isArchived":false,"versionId":"v2","activeVersionId":"v1","createdAt":"2026-11-01T00:00:00.000Z","updatedAt":"2026-11-02T00:00:00.000Z","config":{"name":"Support","model":"anthropic/claude-sonnet-4-5","instructions":"Help."},"configHash":"h1","tools":{"lookup":{"code":"return 1","descriptor":{"name":"lookup"}}},"activeVersion":{"versionId":"v1","integrations":[{"type":"slack","credentialId":"c1"}]}}`

func agentFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestAgentList(t *testing.T) {
	f := agentFixture(t, `{"data":[`+cliAgentSummary+`],"nextCursor":"next"}`)
	got := f.run("agent", "list", "--project-id", "p1", "--limit", "10")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"ID", "PUBLISHED", "ag-1", "Support", "p1", "yes", "Next cursor:", "next"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.URL.Path != n8n.BasePath+n8n.AgentsPath || req.URL.Query().Get("projectId") != "p1" || req.URL.Query().Get("limit") != "10" {
		t.Errorf("request = %s", req.URL)
	}
	got = f.run("agent", "list", "--output", "json")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, `"nextCursor": "next"`) {
		t.Errorf("json = %+v", got)
	}
	got = f.run("agent", "list", "--quiet")
	if got.code != ExitSuccess || strings.TrimSpace(got.stdout) != "ag-1" {
		t.Errorf("quiet = %+v", got)
	}
	before := f.requestCount()
	if got = f.run("agent", "list", "--limit", "251"); got.code == ExitSuccess || f.requestCount() != before {
		t.Errorf("limit 251 = %+v", got)
	}
	f.body = `{"data":[],"nextCursor":null}`
	if got = f.run("agent", "list"); got.code != ExitSuccess || !strings.Contains(got.stderr, "No Agents") {
		t.Errorf("empty = %+v", got)
	}
}

func TestAgentGet(t *testing.T) {
	f := agentFixture(t, cliAgentDetail)
	got := f.run("agent", "get", "ag-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Agent:", "ag-1", "p1 (Ops, team)", "Published version:", "v1", "Config hash:", "h1", "Tools:", "lookup", "Channel:", "slack (credential c1)"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if path := f.lastRequest().URL.Path; path != n8n.BasePath+"/agents/ag-1" {
		t.Errorf("path = %s", path)
	}
	got = f.run("agent", "get", "ag-1", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliAgentDetail) {
		t.Errorf("json = %+v", got)
	}
}

func TestAgentCreate(t *testing.T) {
	f := agentFixture(t, "")
	f.respond(http.StatusCreated, cliAgentDetail)
	got := f.run("agent", "create", "--name", "Support", "--project-id", "p1", "--available-in-mcp")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Created:") {
		t.Fatalf("create = %+v", got)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.AgentsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body := f.lastBody(); body != `{"name":"Support","projectId":"p1","availableInMCP":true}` {
		t.Errorf("body = %s", body)
	}

	// A get response seeds a copy: read-only fields are dropped and reported,
	// flags override the document.
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(path, []byte(cliAgentDetail), 0o600); err != nil {
		t.Fatal(err)
	}
	got = f.run("agent", "create", "--input", path, "--name", "Support (copy)")
	if got.code != ExitSuccess {
		t.Fatalf("copy = %+v", got)
	}
	want := `{"name":"Support (copy)","projectId":"p1","config":{"name":"Support","model":"anthropic/claude-sonnet-4-5","instructions":"Help."},"tools":{"lookup":{"code":"return 1","descriptor":{"name":"lookup"}}},"availableInMCP":false}`
	if !sameJSON(t, f.lastBody(), want) {
		t.Errorf("copy body = %s", f.lastBody())
	}
	for _, field := range []string{"id", "project", "activeVersion", "configHash", "createdAt"} {
		if !strings.Contains(got.stderr, "Dropped read-only field "+field+".") {
			t.Errorf("stderr missing dropped %s:\n%s", field, got.stderr)
		}
	}
	if strings.Contains(got.stdout, "Dropped") {
		t.Errorf("diagnostics leaked to stdout: %s", got.stdout)
	}

	before := f.requestCount()
	for _, args := range [][]string{
		{"agent", "create"},
		{"agent", "create", "--name", " "},
		{"agent", "create", "--name", "a", "--project-id", ""},
		{"agent", "create", "--input", "-", "--name", "a"},
		{"agent", "create", "--name", "a", "--idempotency-key", "bad key"},
	} {
		if got := f.run(args...); got.code == ExitSuccess || f.requestCount() != before {
			t.Errorf("%v accepted: %+v", args, got)
		}
	}
	f.stdin = `{"name":"a","unknown":1}`
	if got := f.run("agent", "create", "--input", "-", "--yes"); got.code == ExitSuccess || !strings.Contains(got.stderr, "writable fields") || f.requestCount() != before {
		t.Errorf("unknown field = %+v", got)
	}
	f.stdin = `{"name":"a"}`
	if got := f.run("agent", "create", "--input", "-", "--yes"); got.code != ExitSuccess || f.lastBody() != `{"name":"a"}` {
		t.Errorf("stdin = %+v body %s", got, f.lastBody())
	}
}

func TestAgentDelete(t *testing.T) {
	f := agentFixture(t, cliAgentSummary)
	f.interactive = false
	before := f.requestCount()
	if got := f.run("agent", "delete", "ag-1"); got.code == ExitSuccess || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
		t.Fatalf("without --yes = %+v", got)
	}
	got := f.run("agent", "delete", "ag-1", "--yes")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Deleted:") {
		t.Fatalf("delete = %+v", got)
	}
	if req := f.lastRequest(); req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/agents/ag-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestAgentErrors(t *testing.T) {
	f := agentFixture(t, "")
	f.respond(http.StatusNotFound, `{"message":"Not Found"}`)
	if got := f.run("agent", "list"); got.code == ExitSuccess || !strings.Contains(got.stderr, "n8n 2.49 or later") {
		t.Errorf("route 404 = %+v", got)
	}
	f.respond(http.StatusNotFound, `{"message":"Agent not found","code":"NOT_FOUND"}`)
	if got := f.run("agent", "get", "ag-9"); got.code == ExitSuccess || !strings.Contains(got.stderr, "n8n agent list") {
		t.Errorf("agent 404 = %+v", got)
	}
	f.respond(http.StatusForbidden, `{"message":"Forbidden"}`)
	if got := f.run("agent", "delete", "ag-1", "--yes"); got.code == ExitSuccess || !strings.Contains(got.stderr, "agent:delete") {
		t.Errorf("403 = %+v", got)
	}
	f.respond(http.StatusBadRequest, `{"message":"Invalid config"}`)
	if got := f.run("agent", "create", "--name", "a"); got.code == ExitSuccess || !strings.Contains(got.stderr, "nothing was created") {
		t.Errorf("400 = %+v", got)
	}
}
