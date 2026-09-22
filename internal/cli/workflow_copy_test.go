package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// copyFixtures are two machines sharing one config directory and credential
// store, the way a user with a staging and a production context has them.
func copyFixtures(t *testing.T) (*fixture, *fixture) {
	t.Helper()
	from, to := newFixture(t), newFixture(t)
	from.login("--context", "staging")
	to.dir, to.keyring = from.dir, from.keyring
	to.prompt.secret = "production-key"
	to.login("--context", "prod")
	from.body = copyWorkflow("AAA", "Invoice sync")
	to.body = copyWorkflow("BBB", "Invoice sync")
	from.clearRequests()
	to.clearRequests()
	return from, to
}

func copyWorkflow(id, name string) string {
	body := strings.Replace(cliWorkflow, `"id":"wf-1"`, `"id":"`+id+`"`, 1)
	return strings.Replace(body, `"name":"Invoice sync"`, `"name":"`+name+`"`, 1)
}

func copyArgs(extra ...string) []string {
	return append([]string{"workflow", "copy", "--from-context", "staging", "--to-context", "prod", "--yes"}, extra...)
}

// stageSource answers a source instance that resolves --name and then serves
// the definition.
func stageSource(f *fixture, list, workflow string) {
	f.route = func(r *http.Request, _ int) (int, string) {
		if strings.HasSuffix(r.URL.Path, "/workflows") {
			return http.StatusOK, list
		}
		return http.StatusOK, workflow
	}
}

// stageTarget answers a target instance: list is the name-lookup page, existing
// the target read (empty means the workflow is gone), written the write and
// publish response.
func stageTarget(f *fixture, list, existing, written string) {
	f.route = func(r *http.Request, _ int) (int, string) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflows"):
			return http.StatusOK, list
		case r.Method == http.MethodGet:
			if existing == "" {
				return http.StatusNotFound, `{"message":"not found"}`
			}
			return http.StatusOK, existing
		default:
			return http.StatusOK, written
		}
	}
}

// clearRequests forgets the login traffic, so a test can assert on exactly the
// requests its own run produced.
func (f *fixture) clearRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests, f.bodies, f.pages = nil, nil, 0
}

// writeRequest returns the first mutating request the instance received and
// its body.
func (f *fixture) writeRequest() (*http.Request, string) {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.requests {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			return r, f.bodies[i]
		}
	}
	f.t.Fatal("no write request reached the instance")
	return nil, ""
}

// request returns one recorded request by index.
func (f *fixture) request(i int) *http.Request {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.requests) {
		f.t.Fatalf("request %d never reached the instance", i)
	}
	return f.requests[i]
}

func (f *fixture) mutated() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.Method != http.MethodGet {
			return true
		}
	}
	return false
}

func TestWorkflowCopyValidatesBeforeTransport(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no contexts", []string{"workflow", "copy", "--from-id", "AAA"}, "--from-context and --to-context are required"},
		{"no source", copyArgs(), "source workflow ID is required"},
		{"name with from-id", copyArgs("--name", "Invoice sync", "--from-id", "AAA"), "if any flags in the group"},
		{"blank name", copyArgs("--name", " "), "must be nonempty"},
		{"project without name", copyArgs("--from-id", "AAA", "--from-project-id", "DEV"), "--from-project-id requires --name"},
		{"create and update only", copyArgs("--from-id", "AAA", "--create-only", "--update-only"), "if any flags in the group"},
		{"bad output", copyArgs("--from-id", "AAA", "--output", "yaml"), "output"},
		{"blank to-name", copyArgs("--from-id", "AAA", "--to-name", " "), "--to-name must not be blank"},
		{"padded id", copyArgs("--from-id", " AAA"), "whitespace"},
		{"padded to-id", copyArgs("--from-id", "AAA", "--to-id", "BBB "), "target workflow ID must not"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := copyFixtures(t)
			got := from.run(tc.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
				t.Fatalf("result: %+v, want %q", got, tc.want)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want nothing on a rejected run", got.stdout)
			}
			if from.requestCount() != 0 || to.requestCount() != 0 {
				t.Error("a rejected run reached an instance")
			}
		})
	}
}

func TestWorkflowCopyRefusesOneInstance(t *testing.T) {
	f := newFixture(t)
	f.login("--context", "staging")
	f.login("--context", "prod")
	f.clearRequests()

	got := f.run(copyArgs("--from-id", "AAA", "--to-id", "BBB")...)
	if got.code != ExitError || !strings.Contains(got.stderr, "same instance") || !strings.Contains(got.stderr, f.server.URL) {
		t.Fatalf("result: %+v", got)
	}
	if !strings.Contains(got.stderr, "n8n workflow create --input -") {
		t.Errorf("stderr must name the single-instance alternative: %s", got.stderr)
	}
	if f.requestCount() != 0 {
		t.Error("the same-instance guard ran after transport")
	}
}

func TestWorkflowCopyCreatesMissingTarget(t *testing.T) {
	from, to := copyFixtures(t)
	stageSource(from, `{"data":[{"id":"AAA","name":"Invoice sync"}]}`, from.body)
	stageTarget(to, `{"data":[]}`, "", to.body)

	got := from.run(copyArgs("--name", "Invoice sync", "--to-project-id", "PROD", "--to-parent-folder-id", "FLD")...)
	if got.code != ExitSuccess {
		t.Fatalf("result: %+v", got)
	}
	req, body := to.writeRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/workflows" {
		t.Fatalf("wrote to %s %s, want POST the collection", req.Method, req.URL.Path)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"id", "versionId", "active", "staticData", "pinData", "tags", "createdAt"} {
		if _, ok := doc[absent]; ok {
			t.Errorf("create body carries %q: %s", absent, body)
		}
	}
	for field, want := range map[string]string{"name": `"Invoice sync"`, "projectId": `"PROD"`, "parentFolderId": `"FLD"`} {
		if string(doc[field]) != want {
			t.Errorf("create body %s = %s, want %s", field, doc[field], want)
		}
	}
	if q := from.lastRequest().URL.Query().Get("excludePinnedData"); q != "true" {
		t.Errorf("source read excludePinnedData = %q, want true", q)
	}
	for _, want := range []string{"Action:", "created", "Invoice sync", "BBB", "staging", "prod"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("text output missing %q:\n%s", want, got.stdout)
		}
	}
	for _, want := range []string{"Not sent", "id", "createdAt", "tags", "Pinned sample data was not copied", "Runtime static data was not copied", "Tags were not copied", "Credentials do not transfer", "n8n workflow diff"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, got.stderr)
		}
	}
}

func TestWorkflowCopyReplacesExistingTargetAsDraft(t *testing.T) {
	from, to := copyFixtures(t)
	written := strings.Replace(to.body, `"versionId":"ver-1"`, `"versionId":"ver-2"`, 1)
	stageTarget(to, `{"data":[]}`, to.body, written)

	got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB", "--output", "json")...)
	if got.code != ExitSuccess {
		t.Fatalf("result: %+v", got)
	}
	req, body := to.writeRequest()
	if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/workflows/BBB" {
		t.Fatalf("wrote to %s %s, want PUT the target", req.Method, req.URL.Path)
	}
	if q := req.URL.Query().Get("publishIfActive"); q != "false" {
		t.Errorf("publishIfActive = %q, want false on every update", q)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"id", "versionId", "active", "staticData", "pinData", "projectId"} {
		if _, ok := sent[absent]; ok {
			t.Errorf("update body carries %q: %s", absent, body)
		}
	}
	var result workflowCopyResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 1 || result.Action != "updated" {
		t.Fatalf("unexpected contract: %s", got.stdout)
	}
	if result.From.Context != "staging" || result.From.WorkflowID != "AAA" || result.From.URL != from.server.URL+n8n.BasePath {
		t.Errorf("wrong source endpoint: %+v", result.From)
	}
	if result.To.Context != "prod" || result.To.WorkflowID != "BBB" || result.To.VersionID != "ver-2" || result.To.ActiveVersionID != "ver-1" {
		t.Errorf("wrong target endpoint: %+v", result.To)
	}
	if result.Publish.Requested || result.Publish.Performed {
		t.Errorf("copy published without --publish: %+v", result.Publish)
	}
	if result.Nodes != 1 || len(result.DroppedFields) == 0 || len(result.Warnings) != 2 {
		t.Errorf("unexpected result detail: %+v", result)
	}
	// The target stayed published on its old version: the copy is a draft.
	text := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB")...)
	if !strings.Contains(text.stdout, "live version ver-1 unchanged") {
		t.Errorf("text output must say the live version is unchanged:\n%s", text.stdout)
	}
}

func TestWorkflowCopyRefusesUnknownTargetID(t *testing.T) {
	from, to := copyFixtures(t)
	stageTarget(to, `{"data":[]}`, "", to.body)

	got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB")...)
	if got.code != ExitError || !strings.Contains(got.stderr, "does not exist") || !strings.Contains(got.stderr, "omit --to-id") {
		t.Fatalf("result: %+v", got)
	}
	if to.mutated() {
		t.Error("a missing --to-id created a workflow")
	}
}

func TestWorkflowCopyModeGuardsStopBeforeWriting(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		list     string
		args     []string
		want     string
	}{
		{"create-only with a target", to_(), `{"data":[{"id":"BBB","name":"Invoice sync"}]}`, []string{"--name", "Invoice sync", "--create-only"}, "--create-only"},
		{"update-only without a target", "", `{"data":[]}`, []string{"--name", "Invoice sync", "--update-only"}, "--update-only"},
		{"archived target", strings.Replace(to_(), `"isArchived":false`, `"isArchived":true`, 1), `{"data":[{"id":"BBB","name":"Invoice sync"}]}`, []string{"--name", "Invoice sync"}, "is archived"},
		{"folder on an existing target", to_(), `{"data":[{"id":"BBB","name":"Invoice sync"}]}`, []string{"--name", "Invoice sync", "--to-parent-folder-id", "FLD"}, "--to-parent-folder-id applies only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := copyFixtures(t)
			stageSource(from, `{"data":[{"id":"AAA","name":"Invoice sync"}]}`, from.body)
			stageTarget(to, tc.list, tc.existing, to.body)
			got := from.run(copyArgs(tc.args...)...)
			if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
				t.Fatalf("result: %+v, want %q", got, tc.want)
			}
			if to.mutated() {
				t.Error("the guard ran after the write")
			}
		})
	}
}

// to_ is the target workflow body the guard table reuses.
func to_() string { return copyWorkflow("BBB", "Invoice sync") }

func TestWorkflowCopyRefusesArchivedSource(t *testing.T) {
	from, to := copyFixtures(t)
	from.body = strings.Replace(from.body, `"isArchived":false`, `"isArchived":true`, 1)
	stageTarget(to, `{"data":[]}`, to.body, to.body)

	got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB")...)
	if got.code != ExitError || !strings.Contains(got.stderr, "source workflow") || !strings.Contains(got.stderr, "archived") {
		t.Fatalf("result: %+v", got)
	}
	if !strings.Contains(got.stderr, "n8n workflow unarchive AAA --context staging") {
		t.Errorf("stderr must name the repair: %s", got.stderr)
	}
	if to.requestCount() != 0 {
		t.Error("an archived source still read the target")
	}
}

func TestWorkflowCopyOverridesTargetName(t *testing.T) {
	for _, tc := range []struct{ name, existing, list string }{
		{"create", "", `{"data":[]}`},
		{"update", to_(), `{"data":[{"id":"BBB","name":"Invoice sync (prod)"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, to := copyFixtures(t)
			stageTarget(to, tc.list, tc.existing, to.body)
			got := from.run(copyArgs("--from-id", "AAA", "--to-name", "Invoice sync (prod)")...)
			if got.code != ExitSuccess {
				t.Fatalf("result: %+v", got)
			}
			_, body := to.writeRequest()
			if !strings.Contains(body, `"name":"Invoice sync (prod)"`) {
				t.Errorf("write body kept the source name: %s", body)
			}
			// The target was looked up under the overriding name.
			if tc.existing == "" {
				return
			}
			if q := to.request(0).URL.Query().Get("name"); q != "Invoice sync (prod)" {
				t.Errorf("name lookup used %q, want the override", q)
			}
		})
	}
}

func TestWorkflowCopyIncludesOptionalPayloadsOnRequest(t *testing.T) {
	from, to := copyFixtures(t)
	from.body = strings.Replace(from.body, `"staticData":null`, `"staticData":{"lastId":7}`, 1)
	from.body = strings.Replace(from.body, `"pinData":{}`, `"pinData":{"Webhook":[{"json":{"a":1}}]}`, 1)
	stageTarget(to, `{"data":[]}`, "", to.body)

	got := from.run(copyArgs("--from-id", "AAA", "--include-pinned-data", "--include-static-data")...)
	if got.code != ExitSuccess {
		t.Fatalf("result: %+v", got)
	}
	if q := from.lastRequest().URL.Query().Get("excludePinnedData"); q != "" {
		t.Errorf("excludePinnedData = %q, want it absent when pins are copied", q)
	}
	_, body := to.writeRequest()
	for _, want := range []string{`"staticData":{"lastId":7}`, `"pinData":{"Webhook"`} {
		if !strings.Contains(body, want) {
			t.Errorf("write body missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{"Pinned sample data was not copied", "Runtime static data was not copied"} {
		if strings.Contains(got.stderr, unwanted) {
			t.Errorf("stderr wrongly reports an exclusion: %s", got.stderr)
		}
	}
}

func TestWorkflowCopyConfirmsBeforeWriting(t *testing.T) {
	from, to := copyFixtures(t)
	stageTarget(to, `{"data":[]}`, to.body, to.body)
	args := []string{"workflow", "copy", "--from-context", "staging", "--to-context", "prod", "--from-id", "AAA", "--to-id", "BBB"}

	from.stdin = "n\n"
	got := from.run(args...)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Fatalf("declined: %+v", got)
	}
	if !strings.Contains(got.stderr, "Replace workflow") || !strings.Contains(got.stderr, "cannot be undone") {
		t.Errorf("prompt must name the blast radius: %s", got.stderr)
	}
	if to.mutated() {
		t.Error("a declined copy still wrote")
	}

	from.interactive = false
	from.stdin = ""
	got = from.run(args...)
	if got.code != ExitError || !strings.Contains(got.stderr, "--yes") {
		t.Fatalf("non-interactive: %+v", got)
	}
	if to.mutated() {
		t.Error("a non-interactive copy without --yes still wrote")
	}
}

func TestWorkflowCopyPublishesOnlyWhenAsked(t *testing.T) {
	draft := strings.Replace(to_(), `"active":true`, `"active":false`, 1)

	t.Run("publishes a draft", func(t *testing.T) {
		from, to := copyFixtures(t)
		stageTarget(to, `{"data":[]}`, draft, draft)
		got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB", "--publish", "--output", "json")...)
		if got.code != ExitSuccess {
			t.Fatalf("result: %+v", got)
		}
		last := to.lastRequest()
		if last.Method != http.MethodPost || last.URL.Path != n8n.BasePath+"/workflows/BBB/publish" {
			t.Fatalf("last call was %s %s, want the publish", last.Method, last.URL.Path)
		}
		var result workflowCopyResult
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
			t.Fatal(err)
		}
		if !result.Publish.Requested || !result.Publish.Performed {
			t.Errorf("publish not reported: %+v", result.Publish)
		}
	})

	t.Run("skips an already published target", func(t *testing.T) {
		from, to := copyFixtures(t)
		stageTarget(to, `{"data":[]}`, to.body, to.body)
		got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB", "--publish")...)
		if got.code != ExitSuccess {
			t.Fatalf("result: %+v", got)
		}
		if strings.HasSuffix(to.lastRequest().URL.Path, "/publish") {
			t.Error("published a target that was already published")
		}
		if !strings.Contains(got.stderr, "already published") {
			t.Errorf("stderr must explain the skip: %s", got.stderr)
		}
	})

	for _, tc := range []struct {
		name   string
		status int
		want   string
	}{
		{"forbidden", http.StatusForbidden, "workflow:activate"},
		{"conflict", http.StatusConflict, "webhook path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, to := copyFixtures(t)
			to.route = func(r *http.Request, _ int) (int, string) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/publish"):
					return tc.status, `{"message":"denied"}`
				case r.Method == http.MethodGet:
					return http.StatusOK, draft
				default:
					return http.StatusOK, draft
				}
			}
			got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB", "--publish")...)
			if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
				t.Fatalf("result: %+v, want %q", got, tc.want)
			}
			if !strings.Contains(got.stderr, "only publication failed") || !strings.Contains(got.stdout, "updated") {
				t.Errorf("a failed publish must still report the landed copy: %+v", got)
			}
		})
	}
}

func TestWorkflowCopyUsesSavedContextsOnly(t *testing.T) {
	from, to := copyFixtures(t)
	stageTarget(to, `{"data":[]}`, to.body, to.body)
	from.env = map[string]string{
		config.EnvURL: "https://wrong.invalid", config.EnvAPIKey: "wrong-key", config.EnvContext: ".invalid",
		config.EnvBearerToken: "wrong-token", config.EnvAuthCookie: "wrong-cookie",
	}
	got := from.run(copyArgs("--from-id", "AAA", "--to-id", "BBB")...)
	if got.code != ExitSuccess {
		t.Fatalf("result: %+v", got)
	}
	if key := from.lastRequest().Header.Get(n8n.HeaderAPIKey); key != testAPIKey {
		t.Errorf("source credential = %q, want the saved one", key)
	}
	if key := to.lastRequest().Header.Get(n8n.HeaderAPIKey); key != "production-key" {
		t.Errorf("target credential = %q, want the saved one", key)
	}
	if strings.Contains(got.stdout+got.stderr, "production-key") || strings.Contains(got.stdout+got.stderr, testAPIKey) {
		t.Error("credential exposed in output")
	}
}

func TestWorkflowCopyFailsClosedOnAmbiguousNames(t *testing.T) {
	from, to := copyFixtures(t)
	stageSource(from, `{"data":[{"id":"AAA","name":"Invoice sync"},{"id":"CCC","name":"Invoice sync"}]}`, from.body)

	got := from.run(copyArgs("--name", "Invoice sync")...)
	if got.code != ExitError || !strings.Contains(got.stderr, "ambiguous name") {
		t.Fatalf("result: %+v", got)
	}
	if to.requestCount() != 0 {
		t.Error("an unresolved source still reached the target")
	}
}
