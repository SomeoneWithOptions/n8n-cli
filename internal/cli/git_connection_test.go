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

const cliGitConnectionBody = `{
  "id":"conn-1",
  "name":"prod",
  "repositoryUrl":"git@example.com:org/repo.git",
  "branchName":"main",
  "connectionType":"ssh",
  "publicKey":"ssh-ed25519 AAAAEBBY",
  "keyGeneratorType":"ed25519",
  "baseCommit":"abc123",
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const cliGitPushBody = `{
  "connectionId":"conn-1",
  "counts":{"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0,"tags":3},
  "commitSha":"deadbeef"
}`

const cliGitPullBody = `{
  "connectionId":"conn-1",
  "counts":{
    "projects":{"created":1,"updated":0,"skipped":0,"deleted":0},
    "folders":{"created":1,"skipped":0,"removed":0},
    "workflows":{"created":1,"updated":0,"skipped":0,"archived":0,"deleted":0,"publishing":{"published":1,"unpublished":0,"unchanged":0,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0},
    "dataTables":{"matched":0,"created":0},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":1,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "commitSha":"deadbeef"
}`

func gitConnectionFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeGitConnectionInput(t *testing.T, f *fixture, name, body string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.dir), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write git-connection input: %v", err)
	}
	return path
}

func TestGitConnectionListTextAndJSON(t *testing.T) {
	f := gitConnectionFixture(t, `{"data":[`+cliGitConnectionBody+`],"nextCursor":"next"}`)

	got := f.run("git-connection", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Connections:", "1", "conn-1", "prod", "git@example.com:org/repo.git", "main", "ssh", "abc123"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.GitConnectionsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("git-connection", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.GitConnection]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 1 || page.NextCursor != "next" {
		t.Errorf("page = %+v, want one connection with a cursor", page)
	}
}

func TestGitConnectionListEmptyAndPagination(t *testing.T) {
	f := gitConnectionFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("git-connection", "list")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "No Git connections") {
		t.Errorf("result = %+v, want the empty hint", got)
	}

	f.bodyFunc = func(i int) string {
		if i == 0 {
			return `{"data":[` + cliGitConnectionBody + `],"nextCursor":"next"}`
		}
		return `{"data":[],"nextCursor":null}`
	}
	got = f.run("git-connection", "list", "--all", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("all exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.GitConnection]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 {
		t.Errorf("all = %q (error %v), want one collected connection", got.stdout, err)
	}

	before := f.requestCount()
	got = f.run("git-connection", "list", "--limit", "251")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("large limit result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestGitConnectionCreateFromInput(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)

	path := writeGitConnectionInput(t, f, "connection.json", `{"name":"prod","repositoryUrl":"git@example.com:org/repo.git","connectionType":"ssh","branchName":"main"}`)
	got := f.run("git-connection", "create", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.GitConnectionsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.CreateGitConnectionRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent.Name != "prod" {
		t.Errorf("body = %q (error %v)", f.lastBody(), err)
	}
	for _, want := range []string{"Created:", "conn-1", "prod"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "secret") || strings.Contains(f.lastBody(), "hunter2") {
		t.Errorf("unexpected secret material in output")
	}

	f.stdin = `{"name":"prod","repositoryUrl":"git@example.com:org/repo.git","connectionType":"ssh"}`
	got = f.run("git-connection", "create", "--input", "-", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}
	var created n8n.GitConnection
	if err := json.Unmarshal([]byte(got.stdout), &created); err != nil || created.ID != "conn-1" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestGitConnectionCreateRejectsBadInputBeforeTransport(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)
	before := f.requestCount()

	path := writeGitConnectionInput(t, f, "connection-bad.json", `{"name":"prod","unknown":true}`)
	cases := [][]string{
		{"git-connection", "create"},
		{"git-connection", "create", "--input", path},
		{"git-connection", "create", "--input", path, "--output", "yaml"},
	}
	f.stdin = `{"name":""}`
	cases = append(cases, []string{"git-connection", "create", "--input", "-"})
	for _, args := range cases {
		got := f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v exit = %d, want %d (stderr: %s)", args, got.code, ExitError, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestGitConnectionGetTextAndJSON(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)

	got := f.run("git-connection", "get", "conn-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"conn-1", "prod", "main", "ssh"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stdout, "AAAA") {
		t.Errorf("stdout missing the public-key prefix:\n%s", got.stdout)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+"/git-connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("git-connection", "get", "conn-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var connection n8n.GitConnection
	if err := json.Unmarshal([]byte(got.stdout), &connection); err != nil || connection.ID != "conn-1" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestGitConnectionGetEscapesID(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)

	got := f.run("git-connection", "get", "a/b c")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastRequest().URL.EscapedPath(), "a%2Fb%20c") {
		t.Errorf("path = %q, want the ID escaped", f.lastRequest().URL.EscapedPath())
	}
}

func TestGitConnectionUpdatePartial(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)

	path := writeGitConnectionInput(t, f, "connection-update.json", `{"branchName":"release"}`)
	got := f.run("git-connection", "update", "conn-1", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/git-connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent["branchName"] != "release" {
		t.Errorf("body = %q (error %v)", f.lastBody(), err)
	}
	if _, hasRepo := sent["repositoryUrl"]; hasRepo {
		t.Errorf("body = %q, want only the supplied fields", f.lastBody())
	}

	before := f.requestCount()
	empty := writeGitConnectionInput(t, f, "connection-empty.json", `{}`)
	got = f.run("git-connection", "update", "conn-1", "--input", empty)
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("empty result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestGitConnectionDeleteConfirmation(t *testing.T) {
	f := gitConnectionFixture(t, ``)
	f.status = http.StatusNoContent
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("git-connection", "delete", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "checkout") {
		t.Errorf("prompt = %q, want the checkout consequence", got.stderr)
	}

	f.interactive = false
	got = f.run("git-connection", "delete", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("git-connection", "delete", "conn-1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/git-connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestGitConnectionCloneBranch(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)

	got := f.run("git-connection", "clone", "conn-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/git-connections/conn-1/clone" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if f.lastBody() != "" {
		t.Errorf("body = %q, want no body for the configured branch", f.lastBody())
	}

	got = f.run("git-connection", "clone", "conn-1", "--branch", "release", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("branch exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent["branchName"] != "release" {
		t.Errorf("branch = %q (error %v)", f.lastBody(), err)
	}

	before := f.requestCount()
	got = f.run("git-connection", "clone", "conn-1", "--branch", "  ")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("blank branch result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestGitConnectionDisconnectConfirmation(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("git-connection", "disconnect", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.interactive = false
	got = f.run("git-connection", "disconnect", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("git-connection", "disconnect", "conn-1", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/git-connections/conn-1/disconnect" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestGitConnectionProjectListAddRemove(t *testing.T) {
	f := gitConnectionFixture(t, `{"projectIds":["p1","p2"]}`)

	got := f.run("git-connection", "project", "list", "conn-1")
	if got.code != ExitSuccess {
		t.Fatalf("list exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"p1", "p2"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	f.body = `{"projectId":"p1","gitConnectionId":"conn-1"}`
	got = f.run("git-connection", "project", "add", "conn-1", "p1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("add exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/git-connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	f.status = http.StatusNoContent
	f.body = ``
	before := f.requestCount()
	f.stdin = "no\n"
	got = f.run("git-connection", "project", "remove", "conn-1", "p1")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("git-connection", "project", "remove", "conn-1", "p1", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("remove exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/git-connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestGitConnectionPushConfirmationAndBody(t *testing.T) {
	f := gitConnectionFixture(t, cliGitPushBody)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("git-connection", "push", "conn-1", "--commit-message", "sync")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "remote") {
		t.Errorf("prompt = %q, want the remote-side effect", got.stderr)
	}

	f.interactive = false
	got = f.run("git-connection", "push", "conn-1", "--commit-message", "sync")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("git-connection", "push", "conn-1", "--commit-message", "sync", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/git-connections/conn-1/push" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.PushGitConnectionRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent.CommitMessage != "sync" || sent.Force {
		t.Errorf("request = %q (error %v)", f.lastBody(), err)
	}
	var result n8n.PushGitConnectionResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.CommitSHA != "deadbeef" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	before = f.requestCount()
	got = f.run("git-connection", "push", "conn-1", "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("missing commit result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestGitConnectionPushForceText(t *testing.T) {
	f := gitConnectionFixture(t, cliGitPushBody)

	got := f.run("git-connection", "push", "conn-1", "--commit-message", "sync", "--force", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent n8n.PushGitConnectionRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || !sent.Force {
		t.Errorf("force = %q (error %v), want true", f.lastBody(), err)
	}
	if !strings.Contains(got.stdout, "deadbeef") {
		t.Errorf("stdout missing the commit SHA:\n%s", got.stdout)
	}
}

func TestGitConnectionPullConfirmationAndOutput(t *testing.T) {
	f := gitConnectionFixture(t, cliGitPullBody)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("git-connection", "pull", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "overwrite") {
		t.Errorf("prompt = %q, want the overwrite warning", got.stderr)
	}

	f.interactive = false
	got = f.run("git-connection", "pull", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("git-connection", "pull", "conn-1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/git-connections/conn-1/pull" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var result n8n.PullGitConnectionResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.CommitSHA != "deadbeef" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestGitConnectionDenialsAreReported(t *testing.T) {
	f := gitConnectionFixture(t, `{"data":[]}`)

	f.status = http.StatusUnauthorized
	got := f.run("git-connection", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "n8n auth login") {
		t.Errorf("401 result = %+v, want the credential hint", got)
	}

	f.status = http.StatusForbidden
	got = f.run("git-connection", "push", "conn-1", "--commit-message", "sync", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "gitConnection:push") || !strings.Contains(got.stderr, "n8n discover --resource gitconnections") {
		t.Errorf("403 result = %+v, want scope and discover hint", got)
	}

	f.status = http.StatusNotFound
	got = f.run("git-connection", "pull", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "does not serve git connections") || !strings.Contains(got.stderr, "n8n discover --resource gitconnections") {
		t.Errorf("404 result = %+v, want the availability hint", got)
	}

	f.status = http.StatusServiceUnavailable
	got = f.run("git-connection", "get", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "does not serve git connections") {
		t.Errorf("503 result = %+v, want the availability hint", got)
	}

	f.status = http.StatusConflict
	f.body = `{"message":"exists"}`
	path := writeGitConnectionInput(t, f, "connection-conflict.json", `{"name":"prod","repositoryUrl":"git@example.com:r.git","connectionType":"ssh"}`)
	got = f.run("git-connection", "create", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "409") {
		t.Errorf("409 result = %+v, want the conflict guidance", got)
	}

	f.status = http.StatusBadRequest
	got = f.run("git-connection", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "400") {
		t.Errorf("400 result = %+v, want the bad-request guidance", got)
	}

	f.status = http.StatusConflict
	f.body = `{"message":"conflicted"}`
	got = f.run("git-connection", "pull", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "409") {
		t.Errorf("pull 409 result = %+v, want the conflict guidance", got)
	}

	f.status = http.StatusUnprocessableEntity
	f.body = `{"message":"unresolvable"}`
	got = f.run("git-connection", "pull", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "422") {
		t.Errorf("pull 422 result = %+v, want the unprocessable guidance", got)
	}
}

func TestGitConnectionSecretsNeverReachOutput(t *testing.T) {
	f := gitConnectionFixture(t, cliGitConnectionBody)

	path := writeGitConnectionInput(t, f, "connection-secret.json", `{"name":"prod","repositoryUrl":"git@example.com:r.git","connectionType":"ssh","username":"git","password":"hunter2-super-secret"}`)
	got := f.run("git-connection", "create", "--input", path, "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, "hunter2") || strings.Contains(got.stderr, "hunter2") {
		t.Errorf("secret leaked into output:\n--- stdout ---\n%s\n--- stderr ---\n%s", got.stdout, got.stderr)
	}
	if !strings.Contains(f.lastBody(), "hunter2") {
		t.Errorf("request body lost the password: %q", f.lastBody())
	}
}
