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

const cliSourceControlFile = `{
  "file":"workflows/abc.json",
  "id":"abc",
  "name":"Example workflow",
  "type":"workflow",
  "status":"modified",
  "location":"local",
  "conflict":false,
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const cliSourceControlConflictFile = `{
  "file":"workflows/conflict.json",
  "id":"conflict",
  "name":"Conflicted workflow",
  "type":"workflow",
  "status":"conflicted",
  "location":"remote",
  "conflict":true,
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

func sourceControlFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeSourceControlInput(t *testing.T, f *fixture, name, body string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.dir), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write source-control input: %v", err)
	}
	return path
}

func TestSourceControlStatusTextAndJSON(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[`+cliSourceControlFile+`,`+cliSourceControlConflictFile+`]}`)

	got := f.run("source-control", "status", "--direction", "push")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Pending push changes:", "2", "Conflicts:", "1", "FILE", "workflows/abc.json", "Conflicted workflow", "conflicted", "true"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stderr, "--force") {
		t.Errorf("stderr missing the conflict hint:\n%s", got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.SourceControlStatusPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if req.URL.Query().Get("direction") != "push" {
		t.Errorf("direction = %q, want push", req.URL.Query().Get("direction"))
	}

	got = f.run("source-control", "status", "--direction", "pull", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var status n8n.SourceControlStatus
	if err := json.Unmarshal([]byte(got.stdout), &status); err != nil {
		t.Fatalf("stdout is not a status: %v\n%s", err, got.stdout)
	}
	if len(status.Data) != 2 || !status.Data[1].Conflict {
		t.Errorf("status = %+v, want both files with one conflict", status.Data)
	}
	if req.URL.Query().Get("direction") == "" {
		t.Errorf("second request lost its direction")
	}
}

func TestSourceControlStatusEmptyAdvisesNothingWaiting(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[]}`)
	got := f.run("source-control", "status", "--direction", "pull")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "Nothing is waiting") {
		t.Errorf("result = %+v, want the empty hint on stderr", got)
	}
}

func TestSourceControlStatusRejectsBadFlagsBeforeTransport(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[]}`)
	before := f.requestCount()

	for _, args := range [][]string{
		{"source-control", "status"},
		{"source-control", "status", "--direction", "both"},
		{"source-control", "status", "--direction", "push", "--output", "yaml"},
	} {
		got := f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v exit = %d, want %d", args, got.code, ExitError)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestSourceControlPushFlagsBodyAndConfirmation(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[`+cliSourceControlFile+`]}`)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("source-control", "push", "--commit-message", "sync", "--file", "abc:workflow")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "remote") {
		t.Errorf("prompt = %q, want the remote-side effect", got.stderr)
	}

	f.interactive = false
	got = f.run("source-control", "push", "--commit-message", "sync", "--file", "abc:workflow")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("source-control", "push", "--commit-message", "sync", "--file", "abc:workflow", "--file", "cred-1:credential", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.SourceControlPushPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.PushSourceControlRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not a push: %v\n%s", err, f.lastBody())
	}
	if sent.CommitMessage != "sync" || len(sent.FileNames) != 2 || sent.FileNames[1].Type != n8n.SourceControlledFileCredential || sent.Force {
		t.Errorf("request = %+v, want both files without force", sent)
	}
	var result n8n.PushSourceControlResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || len(result.Data) != 1 {
		t.Errorf("stdout = %q, want the pushed files (error: %v)", got.stdout, err)
	}
}

func TestSourceControlPushForceAndInput(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[]}`)

	got := f.run("source-control", "push", "--commit-message", "sync", "--file", "abc:workflow", "--force", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent n8n.PushSourceControlRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || !sent.Force {
		t.Errorf("force = %q (error %v), want true", f.lastBody(), err)
	}
	if !strings.Contains(got.stderr, "Force") && !strings.Contains(got.stdout, "Pushed") {
		t.Errorf("output missing the force acknowledgement:\n%s\n%s", got.stdout, got.stderr)
	}

	path := writeSourceControlInput(t, f, "push.json", `{"commitMessage":"from file","fileNames":[{"id":"abc","type":"workflow"}]}`)
	got = f.run("source-control", "push", "--input", path, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("input exit = %d, stderr: %s", got.code, got.stderr)
	}
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent.CommitMessage != "from file" {
		t.Errorf("input body = %q (error %v), want the file document", f.lastBody(), err)
	}

	f.stdin = `{"commitMessage":"from stdin","fileNames":[{"id":"abc","type":"workflow"}]}`
	got = f.run("source-control", "push", "--input", "-", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}

	f.interactive = true
	f.stdin = `{"commitMessage":"x","fileNames":[{"id":"abc","type":"workflow"}]}`
	before := f.requestCount()
	got = f.run("source-control", "push", "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
		t.Errorf("stdin without --yes result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestSourceControlPushRejectsBadSelectionBeforeTransport(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[]}`)
	before := f.requestCount()

	path := writeSourceControlInput(t, f, "push-bad.json", `{"commitMessage":"x","fileNames":[{"id":"abc","type":"workflow"}],"unknown":true}`)
	cases := [][]string{
		{"source-control", "push", "--yes"},
		{"source-control", "push", "--commit-message", "x", "--yes"},
		{"source-control", "push", "--file", "abc:workflow", "--yes"},
		{"source-control", "push", "--commit-message", "x", "--file", "abc", "--yes"},
		{"source-control", "push", "--commit-message", "x", "--file", "abc:unknown", "--yes"},
		{"source-control", "push", "--commit-message", "x", "--file", "abc:workflow", "--input", path, "--yes"},
		{"source-control", "push", "--input", path, "--yes"},
		{"source-control", "push", "--commit-message", "x", "--file", "abc:workflow", "--yes", "--output", "yaml"},
	}
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

func TestSourceControlPullConfirmationAutoPublishAndBody(t *testing.T) {
	f := sourceControlFixture(t, `[`+cliSourceControlFile+`]`)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("source-control", "pull")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "rewrite local") {
		t.Errorf("prompt = %q, want the local-side effect", got.stderr)
	}

	f.interactive = false
	got = f.run("source-control", "pull")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.interactive = true
	f.stdin = "no\n"
	got = f.run("source-control", "pull", "--force")
	if got.code != ExitError || !strings.Contains(got.stderr, "discarded") || f.requestCount() != before {
		t.Errorf("force-declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("source-control", "pull", "--auto-publish", "published", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.SourceControlPullPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.PullSourceControlRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not a pull: %v\n%s", err, f.lastBody())
	}
	if sent.AutoPublish != n8n.SourceControlAutoPublishPublished || sent.Force {
		t.Errorf("request = %+v, want published without force", sent)
	}
	var files []n8n.SourceControlledFile
	if err := json.Unmarshal([]byte(got.stdout), &files); err != nil || len(files) != 1 {
		t.Errorf("stdout = %q, want the pulled files (error: %v)", got.stdout, err)
	}
}

func TestSourceControlPullForceTextAndValidation(t *testing.T) {
	f := sourceControlFixture(t, `[`+cliSourceControlFile+`,`+cliSourceControlConflictFile+`]`)
	got := f.run("source-control", "pull", "--force", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent n8n.PullSourceControlRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || !sent.Force {
		t.Errorf("force = %q (error %v), want true", f.lastBody(), err)
	}
	for _, want := range []string{"Pulled:", "2", "Conflicted workflow"} {
		if !strings.Contains(got.stdout, want) && !strings.Contains(got.stderr, want) {
			t.Errorf("output missing %q:\n--- stdout ---\n%s\n--- stderr ---\n%s", want, got.stdout, got.stderr)
		}
	}

	before := f.requestCount()
	got = f.run("source-control", "pull", "--auto-publish", "sometimes", "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("bad auto-publish result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestSourceControlConflictsPointAtForce(t *testing.T) {
	f := sourceControlFixture(t, "")

	f.status = http.StatusConflict
	f.body = `{"message":"unresolved conflicts"}`
	got := f.run("source-control", "push", "--commit-message", "sync", "--file", "abc:workflow", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "409") || !strings.Contains(got.stderr, "--force") {
		t.Errorf("push conflict result = %+v, want 409 with --force", got)
	}

	f.body = `[` + cliSourceControlConflictFile + `]`
	got = f.run("source-control", "pull", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "409") || !strings.Contains(got.stderr, "--force") || !strings.Contains(got.stderr, "discard") {
		t.Errorf("pull conflict result = %+v, want 409 with --force", got)
	}
}

func TestSourceControlDenialsAreReported(t *testing.T) {
	f := sourceControlFixture(t, `{"data":[]}`)

	f.status = http.StatusUnauthorized
	got := f.run("source-control", "status", "--direction", "push")
	if got.code != ExitError || !strings.Contains(got.stderr, "n8n auth login") {
		t.Errorf("401 result = %+v, want the credential hint", got)
	}

	f.status = http.StatusForbidden
	got = f.run("source-control", "push", "--commit-message", "sync", "--file", "abc:workflow", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "sourceControl:push") || !strings.Contains(got.stderr, "n8n discover --resource sourcecontrol") {
		t.Errorf("403 result = %+v, want scope and discover hint", got)
	}

	f.status = http.StatusNotFound
	got = f.run("source-control", "pull", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "Source Control") || !strings.Contains(got.stderr, "n8n discover --resource sourcecontrol") {
		t.Errorf("404 result = %+v, want the availability hint", got)
	}

	f.status = http.StatusBadRequest
	got = f.run("source-control", "status", "--direction", "push")
	if got.code != ExitError || !strings.Contains(got.stderr, "400") {
		t.Errorf("400 result = %+v, want the bad-request guidance", got)
	}
}
