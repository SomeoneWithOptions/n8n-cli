package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliEnvironmentID = "0b8f7a2e-4c1d-4e5f-9a6b-7c8d9e0f1a2b"

const cliEnvironmentPath = n8n.BasePath + n8n.PromotionEnvironmentsPath + "/" + cliEnvironmentID

// cliPromotionEnvironmentBody is a full 'environment get' response, including
// every read-only field a round trip must drop.
const cliPromotionEnvironmentBody = `{
  "id":"` + cliEnvironmentID + `",
  "name":"prod",
  "scope":"instance",
  "remote":{"schemaVersion":1,"remoteUrl":"https://gitlab.example.com/org/repo.git","repositoryPath":"org/repo"},
  "provider":{"id":"prov-1","name":"gitlab","type":"gitlab","authType":"token","createdAt":"2026-10-01T00:00:00.000Z","updatedAt":"2026-10-01T00:00:00.000Z"},
  "createdAt":"2026-10-01T00:00:00.000Z",
  "updatedAt":"2026-10-01T00:00:00.000Z",
  "direction":"apply",
  "settings":{"schemaVersion":1,"branchName":"main"},
  "checkout":{"hasCheckout":true,"matchesEnvironment":false}
}`

const cliPromotionEnvironmentCheckoutBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "direction":"apply",
  "branchName":"main",
  "hasCheckout":true
}`

func TestPromotionEnvironmentListTextAndJSON(t *testing.T) {
	f := promotionFixture(t, `{"data":[`+cliPromotionEnvironmentBody+`],"nextCursor":"next"}`)

	got := f.run("promotion", "environment", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Environments:", cliEnvironmentID, "prod", "instance", "apply", "main", "https://gitlab.example.com/org/repo.git", "gitlab", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.PromotionEnvironmentsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("promotion", "environment", "list", "--scope", "instance", "--direction", "apply", "--provider-id", "prov-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("filtered exit = %d, stderr: %s", got.code, got.stderr)
	}
	query := f.lastRequest().URL.Query()
	if query.Get("scope") != "instance" || query.Get("direction") != "apply" || query.Get("providerId") != "prov-1" {
		t.Errorf("query = %q, want scope, direction and providerId", f.lastRequest().URL.RawQuery)
	}
	var page n8n.Page[n8n.PromotionEnvironment]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].Settings.BranchName != "main" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	before := f.requestCount()
	for _, args := range [][]string{
		{"promotion", "environment", "list", "--direction", "sideways"},
		{"promotion", "environment", "list", "--direction", " "},
		{"promotion", "environment", "list", "--scope", "global"},
		{"promotion", "environment", "list", "--limit", "251"},
	} {
		if got := f.run(args...); got.code != ExitError {
			t.Errorf("%v exit = %d, want %d", args, got.code, ExitError)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestPromotionEnvironmentListAllKeepsFilters(t *testing.T) {
	f := promotionFixture(t, ``)
	f.bodyFunc = func(i int) string {
		if i == 0 {
			return `{"data":[` + cliPromotionEnvironmentBody + `],"nextCursor":"next"}`
		}
		return `{"data":[],"nextCursor":null}`
	}
	got := f.run("promotion", "environment", "list", "--all", "--direction", "apply", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if query := f.lastRequest().URL.Query(); query.Get("cursor") != "next" || query.Get("direction") != "apply" {
		t.Errorf("second page query = %q, want the cursor and the filter", f.lastRequest().URL.RawQuery)
	}

	f.bodyFunc = nil
	f.body = `{"data":[],"nextCursor":null}`
	got = f.run("promotion", "environment", "list")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "No promotion environments") {
		t.Errorf("empty result = %+v, want the empty hint", got)
	}
}

func TestPromotionEnvironmentCreateDropsReadOnlyFields(t *testing.T) {
	f := promotionFixture(t, cliPromotionEnvironmentBody)

	// A 'get --output json' document plus providerId round-trips.
	doc := strings.Replace(cliPromotionEnvironmentBody, `"name":"prod",`, `"name":"prod","providerId":"prov-1",`, 1)
	path := writePromotionInput(t, f, "environment.json", doc)
	got := f.run("promotion", "environment", "create", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("create exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.PromotionEnvironmentsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("body = %q (error %v)", f.lastBody(), err)
	}
	for _, field := range []string{"id", "provider", "checkout", "createdAt", "updatedAt"} {
		if _, ok := sent[field]; ok {
			t.Errorf("read-only field %q was sent: %s", field, f.lastBody())
		}
	}
	if strings.Contains(f.lastBody(), "repositoryPath") {
		t.Errorf("remote.repositoryPath was sent: %s", f.lastBody())
	}
	for _, field := range []string{"name", "scope", "providerId", "remote", "direction", "settings"} {
		if _, ok := sent[field]; !ok {
			t.Errorf("writable field %q was not sent: %s", field, f.lastBody())
		}
	}
	if !strings.Contains(got.stderr, "Not sent (read-only): id, provider, checkout, createdAt, updatedAt, remote.repositoryPath") {
		t.Errorf("stderr = %q, want the dropped fields named", got.stderr)
	}
	if strings.Contains(got.stdout, "Not sent") {
		t.Errorf("diagnostic leaked into stdout:\n%s", got.stdout)
	}
	for _, want := range []string{"Created:", cliEnvironmentID, "Repository path:", "org/repo", "connected, does not match"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	// Create needs no confirmation, so stdin input works without --yes.
	f.stdin = `{"name":"push","scope":"projects","providerId":"prov-1","remote":{"schemaVersion":1,"remoteUrl":"git@example.com:o/r.git"},"direction":"promote","settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":false}}`
	got = f.run("promotion", "environment", "create", "--input", "-", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastBody(), `"createBranchOnPromotion":false`) {
		t.Errorf("body = %s, want the explicit false", f.lastBody())
	}
	var created n8n.PromotionEnvironment
	if err := json.Unmarshal([]byte(got.stdout), &created); err != nil || created.ID != cliEnvironmentID {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionEnvironmentCreateRejectsBadInputBeforeTransport(t *testing.T) {
	f := promotionFixture(t, cliPromotionEnvironmentBody)
	before := f.requestCount()

	valid := `{"name":"prod","scope":"instance","providerId":"prov-1","remote":{"schemaVersion":1,"remoteUrl":"git@example.com:o/r.git"},"direction":"apply","settings":{"schemaVersion":1,"branchName":"main"}}`
	bad := map[string]string{
		"unknown field":     strings.Replace(valid, `"name":"prod"`, `"name":"prod","color":"red"`, 1),
		"unknown nested":    strings.Replace(valid, `"branchName":"main"`, `"branchName":"main","depth":1`, 1),
		"mismatched":        strings.Replace(valid, `"direction":"apply"`, `"direction":"promote"`, 1),
		"missing provider":  strings.Replace(valid, `"providerId":"prov-1",`, ``, 1),
		"two documents":     valid + valid,
		"array":             `[]`,
		"bad remote schema": strings.Replace(valid, `"remote":{"schemaVersion":1`, `"remote":{"schemaVersion":2`, 1),
	}
	for name, body := range bad {
		path := writePromotionInput(t, f, "bad-environment.json", body)
		if got := f.run("promotion", "environment", "create", "--input", path); got.code != ExitError {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)", name, got.code, ExitError, got.stderr)
		}
	}
	if got := f.run("promotion", "environment", "create"); got.code != ExitError || !strings.Contains(got.stderr, "--input is required") {
		t.Errorf("missing input = %+v", got)
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestPromotionEnvironmentGetTextAndJSON(t *testing.T) {
	f := promotionFixture(t, cliPromotionEnvironmentBody)

	got := f.run("promotion", "environment", "get", cliEnvironmentID)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Environment:", "Direction:", "apply", "Branch:", "main", "Checkout:", "connect again"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != cliEnvironmentPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("promotion", "environment", "get", cliEnvironmentID, "--output", "json")
	var environment n8n.PromotionEnvironment
	if err := json.Unmarshal([]byte(got.stdout), &environment); err != nil || environment.Checkout == nil || environment.Remote.RepositoryPath != "org/repo" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	before := f.requestCount()
	for _, id := range []string{"env-1", " ", "a/b"} {
		got = f.run("promotion", "environment", "get", id)
		if got.code != ExitError || !strings.Contains(got.stderr, "n8n promotion environment list") {
			t.Errorf("id %q result = %+v, want the list hint", id, got)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestPromotionEnvironmentUpdateRoundTripAndConfirmation(t *testing.T) {
	f := promotionFixture(t, cliPromotionEnvironmentBody)
	path := writePromotionInput(t, f, "environment-update.json", cliPromotionEnvironmentBody)
	before := f.requestCount()

	f.stdin = "no\n"
	got := f.run("promotion", "environment", "update", cliEnvironmentID, "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "changes name, remote, settings") {
		t.Errorf("prompt = %q, want the replaced fields", got.stderr)
	}

	f.interactive = false
	got = f.run("promotion", "environment", "update", cliEnvironmentID, "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.stdin = `{"name":"staging"}`
	got = f.run("promotion", "environment", "update", cliEnvironmentID, "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "--input - consumes stdin") || f.requestCount() != before {
		t.Errorf("stdin without --yes = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	// The settings shape is checked against the document's direction before
	// direction is dropped.
	mismatch := writePromotionInput(t, f, "environment-mismatch.json", strings.Replace(cliPromotionEnvironmentBody,
		`"settings":{"schemaVersion":1,"branchName":"main"}`, `"settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":true}`, 1))
	got = f.run("promotion", "environment", "update", cliEnvironmentID, "--input", mismatch, "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "direction is apply") || f.requestCount() != before {
		t.Errorf("mismatch result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	empty := writePromotionInput(t, f, "environment-empty.json", `{"id":"x","scope":"instance"}`)
	got = f.run("promotion", "environment", "update", cliEnvironmentID, "--input", empty, "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "update is empty") || f.requestCount() != before {
		t.Errorf("empty result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "environment", "update", cliEnvironmentID, "--input", path, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.Path != cliEnvironmentPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("body = %q (error %v)", f.lastBody(), err)
	}
	if len(sent) != 3 || sent["name"] == nil || sent["remote"] == nil || sent["settings"] == nil {
		t.Errorf("body = %s, want only name, remote and settings", f.lastBody())
	}
	if string(sent["remote"]) != `{"schemaVersion":1,"remoteUrl":"https://gitlab.example.com/org/repo.git"}` {
		t.Errorf("remote = %s, want it without repositoryPath", sent["remote"])
	}
	if !strings.Contains(got.stderr, "Not sent (read-only): id, provider, checkout, createdAt, updatedAt, scope, direction, remote.repositoryPath") {
		t.Errorf("stderr = %q, want every dropped field named", got.stderr)
	}
	var updated n8n.PromotionEnvironment
	if err := json.Unmarshal([]byte(got.stdout), &updated); err != nil || updated.ID != cliEnvironmentID {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionEnvironmentDeleteConfirmation(t *testing.T) {
	f := promotionFixture(t, ``)
	f.status = http.StatusNoContent
	before := f.requestCount()

	f.stdin = "no\n"
	got := f.run("promotion", "environment", "delete", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "project links and local checkout") {
		t.Errorf("prompt = %q, want the consequence", got.stderr)
	}

	f.interactive = false
	got = f.run("promotion", "environment", "delete", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "environment", "delete", "not-a-uuid", "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("bad ID result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "environment", "delete", cliEnvironmentID, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != cliEnvironmentPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var ack map[string]string
	if err := json.Unmarshal([]byte(got.stdout), &ack); err != nil || ack["action"] != "deleted" || ack["environmentId"] != cliEnvironmentID || ack["connectionId"] != "" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionEnvironmentConnectDisconnect(t *testing.T) {
	f := promotionFixture(t, cliPromotionEnvironmentCheckoutBody)
	f.interactive = false

	// Connect is safe to repeat and needs no --yes even without a terminal.
	got := f.run("promotion", "environment", "connect", cliEnvironmentID)
	if got.code != ExitSuccess {
		t.Fatalf("connect exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != cliEnvironmentPath+"/connect" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	for _, want := range []string{"Connected:", cliEnvironmentID, "apply", "main", "true"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	before := f.requestCount()
	got = f.run("promotion", "environment", "disconnect", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	f.interactive = true
	f.stdin = "no\n"
	got = f.run("promotion", "environment", "disconnect", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.body = strings.Replace(cliPromotionEnvironmentCheckoutBody, `"hasCheckout":true`, `"hasCheckout":false`, 1)
	got = f.run("promotion", "environment", "disconnect", cliEnvironmentID, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("disconnect exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != cliEnvironmentPath+"/disconnect" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var checkout n8n.PromotionEnvironmentCheckout
	if err := json.Unmarshal([]byte(got.stdout), &checkout); err != nil || checkout.HasCheckout {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionEnvironmentProjectListAddRemove(t *testing.T) {
	f := promotionFixture(t, `{"projectIds":["p1","p2"]}`)

	got := f.run("promotion", "environment", "project", "list", cliEnvironmentID)
	if got.code != ExitSuccess {
		t.Fatalf("list exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Environment:", "p1", "p2"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if req := f.lastRequest(); req.Method != http.MethodGet || req.URL.Path != cliEnvironmentPath+"/projects" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	f.body = `{"projectId":"a/b c","environmentId":"` + cliEnvironmentID + `"}`
	got = f.run("promotion", "environment", "project", "add", cliEnvironmentID, "a/b c", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("add exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.EscapedPath() != cliEnvironmentPath+"/projects/a%2Fb%20c" {
		t.Errorf("request = %s %s, want the project ID escaped", req.Method, req.URL.EscapedPath())
	}
	var link n8n.PromotionEnvironmentProjectLink
	if err := json.Unmarshal([]byte(got.stdout), &link); err != nil || link.EnvironmentID != cliEnvironmentID {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	f.status = http.StatusNoContent
	f.body = ``
	before := f.requestCount()
	f.stdin = "no\n"
	got = f.run("promotion", "environment", "project", "remove", cliEnvironmentID, "p1")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	f.interactive = false
	got = f.run("promotion", "environment", "project", "remove", cliEnvironmentID, "p1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "environment", "project", "remove", cliEnvironmentID, " ", "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("blank project result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "environment", "project", "remove", cliEnvironmentID, "p1", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("remove exit = %d, stderr: %s", got.code, got.stderr)
	}
	if req := f.lastRequest(); req.Method != http.MethodDelete || req.URL.Path != cliEnvironmentPath+"/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	for _, want := range []string{"project removed", "Environment:", "Project:", "p1"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
}

func TestPromotionEnvironmentErrorsNameTheFix(t *testing.T) {
	f := promotionFixture(t, `{}`)

	// A bare 404 may mean an instance older than n8n 2.46.
	f.route = func(*http.Request, int) (int, string) { return http.StatusNotFound, `{"message":"not found"}` }
	got := f.run("promotion", "environment", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "older than n8n 2.46") || !strings.Contains(got.stderr, "'n8n promotion connection'") {
		t.Errorf("collection 404 = %+v, want the connection hint", got)
	}
	got = f.run("promotion", "environment", "get", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "'n8n promotion connection'") || !strings.Contains(got.stderr, "n8n promotion environment list") {
		t.Errorf("item 404 = %+v, want the list and connection hints", got)
	}
	// And the reverse for connections on n8n 2.46 and later.
	got = f.run("promotion", "connection", "get", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "'n8n promotion environment'") || !strings.Contains(got.stderr, "does not serve promotions") {
		t.Errorf("connection 404 = %+v, want the environment hint", got)
	}
	got = f.run("promotion", "connection", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "'n8n promotion environment'") || !strings.Contains(got.stderr, "does not serve promotions (404)") {
		t.Errorf("connection list 404 = %+v, want the environment hint", got)
	}

	// A 404 with its own message is a missing environment, not a missing route.
	f.route = func(*http.Request, int) (int, string) {
		return http.StatusNotFound, `{"message":"Promotion environment not found"}`
	}
	got = f.run("promotion", "environment", "get", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "has no promotion environment") || strings.Contains(got.stderr, "older than") {
		t.Errorf("missing environment = %+v, want the ID hint only", got)
	}

	// 503 stays the availability diagnostic without a generation hint.
	f.route = nil
	f.status = http.StatusServiceUnavailable
	got = f.run("promotion", "environment", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "does not serve promotions (503)") || strings.Contains(got.stderr, "older than") {
		t.Errorf("503 = %+v", got)
	}

	f.status = http.StatusForbidden
	got = f.run("promotion", "environment", "connect", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "gitConnection:clone") {
		t.Errorf("connect 403 = %+v, want the missing scope", got)
	}
	got = f.run("promotion", "environment", "project", "add", cliEnvironmentID, "p1")
	if got.code != ExitError || !strings.Contains(got.stderr, "gitConnection:manageProjects") {
		t.Errorf("project add 403 = %+v, want the missing scope", got)
	}

	f.status = http.StatusConflict
	f.body = `{"message":"exists"}`
	got = f.run("promotion", "environment", "project", "add", cliEnvironmentID, "p1")
	if got.code != ExitError || !strings.Contains(got.stderr, "already has an environment for this direction") {
		t.Errorf("project add 409 = %+v", got)
	}
	path := writePromotionInput(t, f, "env-409.json", `{"name":"x"}`)
	got = f.run("promotion", "environment", "update", cliEnvironmentID, "--input", path, "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "provider changed during the update") || !strings.Contains(got.stderr, "environment get") {
		t.Errorf("update 409 = %+v", got)
	}

	f.status = http.StatusUnprocessableEntity
	f.body = `{"message":"branch missing"}`
	got = f.run("promotion", "environment", "connect", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "branch exists") {
		t.Errorf("connect 422 = %+v", got)
	}
}
