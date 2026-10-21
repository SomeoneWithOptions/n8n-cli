package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliPromotionAppliedBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"0123456789abcdef0123456789abcdef01234567","branchName":"main"},
  "status":"applied",
  "counts":{
    "projects":{"created":1,"updated":0,"skipped":0,"deleted":0},
    "folders":{"created":0,"skipped":0,"removed":0},
    "workflows":{"created":1,"updated":0,"skipped":0,"archived":0,"deleted":0,"publishing":{"published":1,"unpublished":0,"unchanged":0,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0},
    "dataTables":{"matched":0,"created":0,"updated":2},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":0,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "warnings":[
    {"kind":"variable","code":"variable-shadowed","name":"API_URL","scope":{"kind":"global"},
     "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-1","name":"Sync"}]}]}
  ]
}`

const cliPromotionBlockedBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"0123456789abcdef0123456789abcdef01234567","branchName":"main"},
  "status":"blocked",
  "preflight":{
    "missingProjects":[{"id":"p-9","name":"Ops"}],
    "missingBindings":[
      {"kind":"credential","sourceId":"cred-1","name":"Slack","credentialType":"slackApi",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-1","name":"Sync"},{"id":"wf-2","name":"Notify"}]}],
       "ownerProject":{"id":"p-1","name":"Sales"}}
    ],
    "accessRequirements":[
      {"kind":"credential","sourceId":"cred-2","name":"Postgres","credentialType":"postgres","code":"access-required",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-3","name":"Load"}]}]}
    ],
    "conflicts":[
      {"kind":"project","code":"project-deletion","project":{"id":"p-7","name":"Legacy"}},
      {"kind":"data-table","code":"destructive-change","id":"dt-1","name":"Orders",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-5","name":"Orders"}]}],
       "changes":[{"kind":"remove-column","column":"total","type":"number","destructive":true},
                  {"kind":"rename-table","from":"orders","to":"Orders","destructive":false}]}
    ],
    "warnings":[]
  }
}`

const cliPromotionSourceChangedBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"fedcba9876543210fedcba9876543210fedcba98","branchName":"main"},
  "status":"source-changed"
}`

func sameJSON(t *testing.T, got, want string) bool {
	t.Helper()
	var a, b bytes.Buffer
	if err := json.Compact(&a, []byte(got)); err != nil {
		t.Errorf("stdout is not JSON: %v\n%s", err, got)
		return false
	}
	if err := json.Compact(&b, []byte(want)); err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
	return a.String() == b.String()
}

func TestPromotionApplyBlocked(t *testing.T) {
	f := promotionFixture(t, cliPromotionBlockedBody)

	got := f.run("promotion", "apply", "conn-1", "--yes", "--output", "json")
	if got.code != ExitPromotionBlocked {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, ExitPromotionBlocked, got.stderr)
	}
	if !sameJSON(t, got.stdout, cliPromotionBlockedBody) {
		t.Errorf("stdout = %s, want the full blocked result", got.stdout)
	}
	for _, want := range []string{"Nothing was applied", "configId=cfg-apply", "branchName=main", "commitSha=0123456789abcdef0123456789abcdef01234567", "explicit confirmation", "blocked at preflight"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, got.stderr)
		}
	}
	if strings.Contains(got.stderr, "Run 'n8n --help'") {
		t.Errorf("stderr = %q, want no usage hint for a reported outcome", got.stderr)
	}

	got = f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitPromotionBlocked {
		t.Fatalf("text exit = %d, want %d (stderr: %s)", got.code, ExitPromotionBlocked, got.stderr)
	}
	for _, want := range []string{
		"Not applied:", "blocked",
		"MISSING PROJECTS (1)", "p-9", "Ops",
		"MISSING BINDINGS (1)", "Slack", "type slackApi", "2 workflow(s) in 1 project(s)",
		"ACCESS REQUIREMENTS (1)", "access-required", "Postgres",
		"CONFLICTS (2)", "project-deletion", "Legacy", "deletes this team project",
		"destructive-change", "Orders", "remove-column total",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "Applied:") || strings.Contains(got.stdout, "ENTITY") || strings.Contains(got.stdout, "rename-table") {
		t.Errorf("stdout = %s, want no applied counts and only destructive changes", got.stdout)
	}
}

func TestPromotionApplySourceChanged(t *testing.T) {
	f := promotionFixture(t, cliPromotionSourceChangedBody)

	got := f.run("promotion", "apply", "conn-1", "--yes", "--output", "json")
	if got.code != ExitPromotionSourceChanged {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, ExitPromotionSourceChanged, got.stderr)
	}
	if !sameJSON(t, got.stdout, cliPromotionSourceChangedBody) {
		t.Errorf("stdout = %s, want the full result", got.stdout)
	}
	if !strings.Contains(got.stderr, "branch moved") || !strings.Contains(got.stderr, "Nothing was applied") || !strings.Contains(got.stderr, "n8n promotion apply conn-1") {
		t.Errorf("stderr = %q, want the re-run hint", got.stderr)
	}

	got = f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitPromotionSourceChanged || !strings.Contains(got.stdout, "source-changed") || strings.Contains(got.stdout, "ENTITY") {
		t.Errorf("text result = %+v, want the status without counts", got)
	}
}

func TestPromotionApplyUnknownStatusIsNotSuccess(t *testing.T) {
	body := `{"connectionId":"conn-1","configId":"cfg-apply","git":{"commitSha":"abc","branchName":"main"},"status":"queued","queue":{"position":1}}`
	f := promotionFixture(t, body)

	got := f.run("promotion", "apply", "conn-1", "--yes", "--output", "json")
	if got.code != ExitError {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, ExitError, got.stderr)
	}
	if !sameJSON(t, got.stdout, body) {
		t.Errorf("stdout = %s, want the unknown result kept intact", got.stdout)
	}
	if !strings.Contains(got.stderr, `unknown status "queued"`) {
		t.Errorf("stderr = %q, want the unknown status named", got.stderr)
	}
}

func TestPromotionApplyAppliedWithWarnings(t *testing.T) {
	f := promotionFixture(t, cliPromotionAppliedBody)

	got := f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Applied:", "conn-1", "dataTables   updated      2", "WARNINGS (1)", "variable-shadowed", "API_URL", "scope global"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stderr, "1 warning(s)") {
		t.Errorf("stderr = %q, want the warning hint", got.stderr)
	}

	got = f.run("promotion", "apply", "conn-1", "--yes", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliPromotionAppliedBody) {
		t.Errorf("json result = %+v, want the verbatim applied result", got)
	}
}

func TestPromotionProviderCreateGitLabFromFlags(t *testing.T) {
	f := promotionFixture(t, `{"provider":{"id":"prov-2","name":"gitlab","type":"gitlab","authType":"token",
	  "config":{"schemaVersion":1,"baseUrl":"https://gitlab.example.com"},"createdAt":"t","updatedAt":"t"},"publicKey":null}`)

	f.stdin = "glpat-super-secret\n"
	got := f.run("promotion", "provider", "create", "--name", "gitlab", "--type", "gitlab",
		"--base-url", "https://gitlab.example.com", "--auth-type", "token", "--password-stdin")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if string(sent["type"]) != `"gitlab"` || string(sent["config"]) != `{"schemaVersion":1,"baseUrl":"https://gitlab.example.com"}` {
		t.Errorf("request = %s, want gitlab with config", f.lastBody())
	}
	if string(sent["auth"]) != `{"authType":"token","password":"glpat-super-secret"}` {
		t.Errorf("auth = %s, want the token without a username", sent["auth"])
	}
	if !strings.Contains(got.stdout, "Base URL:") || !strings.Contains(got.stdout, "https://gitlab.example.com") {
		t.Errorf("stdout = %s, want the base URL", got.stdout)
	}
	if strings.Contains(got.stdout+got.stderr, "glpat") {
		t.Errorf("token leaked into output:\n%s\n%s", got.stdout, got.stderr)
	}

	// Interactive token auth asks without echo.
	f.prompt.secret = "prompted-token"
	f.prompt.prompts = nil
	got = f.run("promotion", "provider", "create", "--name", "gitlab", "--type", "gitlab", "--auth-type", "token")
	if got.code != ExitSuccess || len(f.prompt.prompts) != 1 || !strings.Contains(f.lastBody(), "prompted-token") {
		t.Errorf("prompted result = %+v, prompts = %q, body = %s", got, f.prompt.prompts, f.lastBody())
	}

	// SSH-key auth on a git provider needs no secret at all.
	f.prompt.prompts = nil
	got = f.run("promotion", "provider", "create", "--name", "github")
	if got.code != ExitSuccess || len(f.prompt.prompts) != 0 || !strings.Contains(f.lastBody(), `"type":"git"`) || !strings.Contains(f.lastBody(), `"authType":"ssh-key"`) {
		t.Errorf("ssh-key result = %+v, body = %s", got, f.lastBody())
	}
}

func TestPromotionProviderCreateFlagErrorsBeforeTransport(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderCreateBody)
	before := f.requestCount()
	path := writePromotionInput(t, f, "provider.json", `{"name":"github","type":"git","auth":{"authType":"ssh-key"}}`)

	f.prompt.prompts = nil
	for _, tc := range []struct {
		args        []string
		interactive bool
		want        string
	}{
		{[]string{"--input", path, "--name", "x"}, true, "mutually exclusive"},
		{[]string{"--name", "x", "--type", "git", "--base-url", "https://gitlab.example.com"}, true, "gitlab providers only"},
		{[]string{"--name", "x", "--type", "gitlab", "--base-url", "gitlab.example.com"}, true, "absolute http or https URL"},
		{[]string{"--name", "x", "--auth-type", "token"}, true, "username is required"},
		{[]string{"--name", "x", "--password-stdin"}, true, "needs --auth-type token"},
		{[]string{"--name", "x", "--type", "gitlab", "--auth-type", "token"}, false, "pipe it with --password-stdin"},
		{[]string{"--type", "bitbucket", "--name", "x"}, true, "unknown provider type"},
	} {
		f.interactive = tc.interactive
		got := f.run(append([]string{"promotion", "provider", "create"}, tc.args...)...)
		if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
			t.Errorf("%v result = %+v, want %q", tc.args, got, tc.want)
		}
	}
	if f.requestCount() != before || len(f.prompt.prompts) != 0 {
		t.Errorf("requests = %d want %d, prompts = %q: want refusal before the token prompt and transport", f.requestCount(), before, f.prompt.prompts)
	}
}

func TestPromotionGitLabTokenInputNeedsNoUsername(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderCreateBody)

	path := writePromotionInput(t, f, "gitlab.json", `{"name":"gl","type":"gitlab","auth":{"authType":"token","password":"secret"},"config":{"schemaVersion":1,"baseUrl":"https://gitlab.example.com"}}`)
	if got := f.run("promotion", "provider", "create", "--input", path); got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastBody(), `"config":{"schemaVersion":1,"baseUrl":"https://gitlab.example.com"}`) {
		t.Errorf("body = %s, want the config forwarded", f.lastBody())
	}

	f.body = cliPromotionProviderBody
	update := writePromotionInput(t, f, "gitlab-update.json", `{"config":{"schemaVersion":1,"baseUrl":"https://gitlab2.example.com"},"auth":{"authType":"token","password":"new"}}`)
	if got := f.run("promotion", "provider", "update", "prov-1", "--input", update, "--yes"); got.code != ExitSuccess {
		t.Fatalf("update exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastBody(), "gitlab2.example.com") {
		t.Errorf("update body = %s, want the new base URL", f.lastBody())
	}
}

func TestPromotionConnectionTargetRepositoryPath(t *testing.T) {
	body := `{"id":"conn-2","name":"gl","scope":"instance",
	  "target":{"schemaVersion":1,"remoteUrl":"https://gitlab.example.com/org/repo.git","repositoryPath":"org/repo"},
	  "provider":{"id":"prov-2","name":"gitlab","type":"gitlab","authType":"token","createdAt":"t","updatedAt":"t"},
	  "configs":{"apply":{"id":"cfg-a","name":"Apply","createdAt":"t","updatedAt":"t","checkout":{"hasCheckout":true,"matchesConfig":false},"settings":{"schemaVersion":1,"branchName":"main"}}},
	  "createdAt":"t","updatedAt":"t"}`
	f := promotionFixture(t, body)

	got := f.run("promotion", "connection", "get", "conn-2", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, body) {
		t.Errorf("json result = %+v, want the connection with repositoryPath and checkout intact", got)
	}
	got = f.run("promotion", "connection", "get", "conn-2")
	for _, want := range []string{"Repository path:", "org/repo", "Apply checkout:", "does not match"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	before := f.requestCount()
	path := writePromotionInput(t, f, "conn-update.json", `{"target":{"schemaVersion":1,"remoteUrl":"https://gitlab.example.com/org/repo.git","repositoryPath":"org/repo"}}`)
	got = f.run("promotion", "connection", "update", "conn-2", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "repositoryPath is read-only") || f.requestCount() != before {
		t.Errorf("read-only update result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPromotionNotFoundNamesTheMissingResource(t *testing.T) {
	f := promotionFixture(t, `{}`)
	notFound := func(body string) func(*http.Request, int) (int, string) {
		return func(*http.Request, int) (int, string) { return http.StatusNotFound, body }
	}

	f.route = notFound(`{"message":"Promotion connection conn-1 not found"}`)
	got := f.run("promotion", "connection", "get", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, `has no promotion connection "conn-1" (404)`) || !strings.Contains(got.stderr, "n8n promotion connection list") {
		t.Errorf("specific 404 = %+v, want the missing connection named", got)
	}
	if strings.Contains(got.stderr, "does not serve promotions") {
		t.Errorf("specific 404 = %q, want no module diagnostic", got.stderr)
	}

	f.route = notFound(`{"message":"not found"}`)
	got = f.run("promotion", "provider", "get", "prov-1")
	if got.code != ExitError || !strings.Contains(got.stderr, `promotion provider "prov-1"`) || !strings.Contains(got.stderr, "does not serve promotions") {
		t.Errorf("generic 404 on an ID route = %+v, want both causes", got)
	}

	got = f.run("promotion", "project", "add", "conn-1", "proj-1")
	if got.code != ExitError || !strings.Contains(got.stderr, `project "proj-1"`) || !strings.Contains(got.stderr, "n8n project list") {
		t.Errorf("project add 404 = %+v, want both IDs named", got)
	}

	got = f.run("promotion", "connection", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "does not serve promotions (404)") || !strings.Contains(got.stderr, "n8n discover --resource promotions") {
		t.Errorf("collection 404 = %+v, want the module diagnostic", got)
	}
}

func TestPromotionScopeCoversDiscover(t *testing.T) {
	for _, action := range []string{"provider repositories", "provider branches"} {
		if got := promotionScope(action); got != "gitConnection:discover" {
			t.Errorf("promotionScope(%q) = %q, want gitConnection:discover", action, got)
		}
	}
}

func TestPromotionApplyStoppedErrorExitCodes(t *testing.T) {
	for status, want := range map[string]int{
		n8n.PromotionApplyStatusBlocked:       ExitPromotionBlocked,
		n8n.PromotionApplyStatusSourceChanged: ExitPromotionSourceChanged,
		"queued":                              ExitError,
	} {
		if got := (&promotionApplyStoppedError{connectionID: "c", status: status}).exitCode(); got != want {
			t.Errorf("exit code for %q = %d, want %d", status, got, want)
		}
	}
}
