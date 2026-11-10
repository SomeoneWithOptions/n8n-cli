package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliPackageImportAgentsResult = `{
  "package":{"sourceN8nVersion":"2.47.3","sourceId":"pkg-4"},
  "agents":[
    {"sourceAgentId":"ag-src","localId":"ag-local","name":"Helper","projectId":"p1","activeVersionId":null,"status":"created",
     "publishing":{"state":"blocked","blockedReason":"channel-setup-required"},
     "channels":[{"type":"slack","status":"setup-required","reason":"new-channel","setupUrl":"https://n8n.example/agents/ag-local/channels"}]}
  ],
  "counts":{"agents":{"created":1,"updated":0,"skipped":0}},
  "workflows":[
    {"sourceWorkflowId":"wf-src","localId":"wf-local","name":"Example","projectId":"p1","parentFolderId":null,"activeVersionId":null,"isArchived":false,"publishing":{"state":"failed","error":"boom"},"status":"created"}
  ],
  "removedWorkflows":[],"removedFolders":[],"folders":[],"projects":[],
  "bindings":{"agents":{"ag-src":"ag-local"},"workflows":{"wf-src":"wf-local"},"credentials":{}},
  "credentials":{"matched":[],"stubbed":[],"seeded":[],"incomplete":[]},
  "dataTables":{"matched":0,"created":0,"updated":0},
  "variables":{"matched":[],"missing":[],"created":[],"stubbed":[],"updated":[]},
  "tags":{"matched":[],"created":[],"renamed":[],"reconciled":[],"skipped":[]}
}`

func TestPackageImportDeprecatedPolicyAliases(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)

	got := f.run("package", "import", "--file", archive,
		"--workflow-conflict-policy", "skip", "--workflow-publishing-policy", "publish-all", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	fields, _, _ := parseCLIImportMultipart(t, f)
	if fields["conflictPolicy"] != "skip" || fields["publishingPolicy"] != "publish-all" || fields["idPolicy"] != "source" {
		t.Errorf("fields = %v, want the aliases sent under the 2.47 names", fields)
	}
	if !strings.Contains(got.stderr, "deprecated") || !strings.Contains(got.stderr, "--conflict-policy") {
		t.Errorf("stderr = %q, want a deprecation notice naming the new flag", got.stderr)
	}
	if !json.Valid([]byte(got.stdout)) {
		t.Errorf("stdout = %q, want only the JSON result", got.stdout)
	}

	got = f.run("package", "import-selection", "--file", archive, "--project-id", "p1", "--workflow-id", "a",
		"--workflow-id-policy", "source", "--workflow-conflict-policy", "fail", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("selection exit = %d, stderr: %s", got.code, got.stderr)
	}
	fields, _, _ = parseCLIImportMultipart(t, f)
	if fields["conflictPolicy"] != "fail" || fields["idPolicy"] != "source" {
		t.Errorf("selection fields = %v, want the 2.47 names", fields)
	}

	before := f.requestCount()
	got = f.run("package", "import", "--file", archive, "--conflict-policy", "skip", "--workflow-conflict-policy", "fail", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "disagree") || f.requestCount() != before {
		t.Errorf("disagreeing aliases = %+v, requests %d want %d", got, f.requestCount(), before)
	}
	got = f.run("package", "import", "--file", archive, "--conflict-policy", "skip", "--workflow-conflict-policy", "skip", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Errorf("agreeing aliases exit = %d, stderr: %s", got.code, got.stderr)
	}
}

func TestPackageImportResendsLegacyFieldsToOlderInstance(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)
	f.route = func(_ *http.Request, page int) (int, string) {
		if page == 0 {
			return http.StatusBadRequest, `{"code":"invalid_type","path":["workflowConflictPolicy"],"message":"Required"}`
		}
		return http.StatusOK, cliPackageImportResult
	}
	got := f.run("package", "import", "--file", archive, "--conflict-policy", "new-version", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if f.pages != 2 {
		t.Fatalf("requests = %d, want the 2.47 attempt and one legacy retry", f.pages)
	}
	fields, fileBytes, _ := parseCLIImportMultipart(t, f)
	if fields["workflowConflictPolicy"] != "new-version" || fields["workflowIdPolicy"] != "source" || fileBytes != cliPackageArchive {
		t.Errorf("retry fields = %v (archive %q), want legacy names and the full archive", fields, fileBytes)
	}
}

func TestPackageImportTextShowsAgentsAndSetupHints(t *testing.T) {
	f := packageFixture(t, cliPackageImportAgentsResult)
	archive := writePackageArchive(t, cliPackageArchive)

	got := f.run("package", "import", "--file", archive, "--conflict-policy", "skip", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Agents:", "1 (1 created, 0 updated, 0 skipped)", "SOURCE AGENT", "ag-local", "Helper", "blocked"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	for _, want := range []string{`Agent "Helper" (ag-local): slack channel needs setup (new-channel): open https://n8n.example/agents/ag-local/channels`, "2 Agent(s) or workflow(s) were imported but not published"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, got.stderr)
		}
	}
	if strings.Contains(got.stdout, "https://") {
		t.Errorf("stdout = %q, want hints on stderr only", got.stdout)
	}
}

func TestPackageImportScopeAndVersionErrors(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)

	f.route = func(*http.Request, int) (int, string) { return http.StatusForbidden, `{"message":"Forbidden"}` }
	got := f.run("package", "import", "--file", archive, "--conflict-policy", "skip", "--yes")
	for _, want := range []string{"either one", "workflow:import", "agent:import", "agent:read"} {
		if got.code != ExitError || !strings.Contains(got.stderr, want) {
			t.Errorf("403 stderr missing %q: %+v", want, got)
		}
	}

	f.route = func(*http.Request, int) (int, string) {
		return http.StatusBadRequest, `{"code":"invalid_enum_value","options":["source"],"path":["idPolicy"],"message":"Invalid enum value. Expected 'source', received 'new'"}`
	}
	got = f.run("package", "import", "--file", archive, "--conflict-policy", "skip", "--id-policy", "new", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "accept only --id-policy source") {
		t.Errorf("idPolicy 400 = %+v, want the fix", got)
	}
}

func TestPackageAPIErrorNonRewindableArchive(t *testing.T) {
	apiErr := &n8n.APIError{StatusCode: http.StatusBadRequest, Fields: []string{"workflowConflictPolicy"}}
	err := packageAPIError(fmt.Errorf("n8n: POST /n8n-packages/import: %w: %w", n8n.ErrImportArchiveNotRewindable, apiErr),
		config.Resolution{URL: "https://n8n.example.com"}, "import", allOf("workflow:import"))
	for _, want := range []string{"predates n8n 2.47", "--file PATH instead of --file -", "nothing was written"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	}
}

func TestReportRateLimitHint(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.route = func(*http.Request, int) (int, string) {
		return http.StatusTooManyRequests, `{"message":"Too many requests"}`
	}
	got := f.run("package", "import", "--file", writePackageArchive(t, cliPackageArchive), "--conflict-policy", "skip", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "rate-limited this API key (429)") || !strings.Contains(got.stderr, "'n8n rate-limit get'") {
		t.Errorf("429 result = %+v, want the rate-limit fix", got)
	}

	var errOut bytes.Buffer
	err := fmt.Errorf("list workflows: %w", &n8n.APIError{StatusCode: http.StatusTooManyRequests,
		RateLimit: &n8n.RateLimit{RetryAfter: 12 * time.Second, HasRetryAfter: true}})
	if code := report(&errOut, err); code != ExitError || !strings.Contains(errOut.String(), "wait 12s and retry") {
		t.Errorf("report = %d, %q, want the wait named", code, errOut.String())
	}
	errOut.Reset()
	report(&errOut, fmt.Errorf("boom"))
	if strings.Contains(errOut.String(), "rate-limit") {
		t.Errorf("report(non-429) = %q, want no rate-limit hint", errOut.String())
	}
}
