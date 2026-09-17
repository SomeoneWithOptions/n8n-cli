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
	cliCredentialSecret = "credential-payload-secret-never-print"
	cliCredential       = `{
  "id":"credential-id",
  "name":"GitHub production",
  "type":"githubApi",
  "isManaged":false,
  "isGlobal":false,
  "isResolvable":true,
  "resolvableAllowFallback":false,
  "resolverId":null,
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "data":{"accessToken":"server-secret-never-print"}
}`
)

func credentialFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeCredentialInput(t *testing.T, f *fixture, name, body string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.dir), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write credential input: %v", err)
	}
	return path
}

func assertNoCredentialSecret(t *testing.T, got result) {
	t.Helper()
	for _, output := range []string{got.stdout, got.stderr} {
		for _, secret := range []string{cliCredentialSecret, "server-secret-never-print"} {
			if strings.Contains(output, secret) {
				t.Errorf("output leaked %q:\nstdout: %s\nstderr: %s", secret, got.stdout, got.stderr)
			}
		}
	}
}

func TestCredentialListTextAndJSON(t *testing.T) {
	page := `{"data":[` + cliCredential + `],"nextCursor":"next page"}`
	f := credentialFixture(t, page)
	got := f.run("credential", "list", "--limit", "25", "--cursor", "start")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Credentials:", "credential-id", "GitHub production", "githubApi", "Next cursor:", "next page"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	assertNoCredentialSecret(t, got)
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.CredentialsPath || req.URL.Query().Get("limit") != "25" || req.URL.Query().Get("cursor") != "start" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
	}

	got = f.run("credential", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var decoded n8n.Page[n8n.Credential]
	if err := json.Unmarshal([]byte(got.stdout), &decoded); err != nil || len(decoded.Data) != 1 {
		t.Errorf("stdout = %q, want credential page (error: %v)", got.stdout, err)
	}
	assertNoCredentialSecret(t, got)
}

func TestCredentialCreateFromFileRedactsSecrets(t *testing.T) {
	f := credentialFixture(t, cliCredential)
	input := `{"name":"GitHub production","type":"githubApi","data":{"accessToken":"` + cliCredentialSecret + `"},"projectId":"project-one"}`
	path := writeCredentialInput(t, f, "credential-create.json", input)
	got := f.run("credential", "create", "--file", path, "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertNoCredentialSecret(t, got)
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.CredentialsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	assertCLIJSON(t, input, f.lastBody())
	var credential n8n.Credential
	if err := json.Unmarshal([]byte(got.stdout), &credential); err != nil || credential.ID != "credential-id" {
		t.Errorf("stdout = %q, want metadata JSON (error: %v)", got.stdout, err)
	}
}

func TestCredentialUpdateFromPipedStdin(t *testing.T) {
	f := credentialFixture(t, cliCredential)
	f.interactive = false
	f.stdin = `{"data":{"accessToken":"` + cliCredentialSecret + `"},"isPartialData":true}`
	id := "credential/id?x=1"
	got := f.run("credential", "update", id, "--stdin")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertNoCredentialSecret(t, got)
	req := f.lastRequest()
	if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/credentials/credential%2Fid%3Fx=1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	assertCLIJSON(t, f.stdin, f.lastBody())
	if !strings.Contains(got.stdout, "Secret data:") || !strings.Contains(got.stdout, "omitted") {
		t.Errorf("stdout = %q, want explicit secret omission", got.stdout)
	}
}

func TestCredentialGetDropsUnexpectedData(t *testing.T) {
	f := credentialFixture(t, cliCredential)
	got := f.run("credential", "get", "credential-id", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertNoCredentialSecret(t, got)
	if strings.Contains(got.stdout, `"data"`) {
		t.Errorf("stdout contains data field: %s", got.stdout)
	}
	if req := f.lastRequest(); req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+"/credentials/credential-id" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestCredentialDeleteRequiresConfirmationAndDropsData(t *testing.T) {
	f := credentialFixture(t, cliCredential)
	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("credential", "delete", "credential-id")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("decline = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("credential", "delete", "credential-id", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertNoCredentialSecret(t, got)
	if req := f.lastRequest(); req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
}

func TestCredentialTestAndSchema(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		f := credentialFixture(t, `{"status":"OK","message":"Connection succeeded"}`)
		got := f.run("credential", "test", "credential-id")
		if got.code != ExitSuccess || !strings.Contains(got.stdout, "Status:") || !strings.Contains(got.stdout, "OK") {
			t.Errorf("result = %+v", got)
		}
		if req := f.lastRequest(); req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/credentials/credential-id/test" || f.lastBody() != "" {
			t.Errorf("request = %s %s body %q", req.Method, req.URL.Path, f.lastBody())
		}
	})
	t.Run("schema", func(t *testing.T) {
		f := credentialFixture(t, `{"type":"object","properties":{"apiKey":{"type":"string"}},"required":["apiKey"]}`)
		got := f.run("credential", "schema", "githubApi")
		if got.code != ExitSuccess || !strings.Contains(got.stdout, `"properties"`) || !json.Valid([]byte(got.stdout)) {
			t.Errorf("result = %+v", got)
		}
		if req := f.lastRequest(); req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+"/credentials/schema/githubApi" {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
	})
}

func TestCredentialTransferRequiresConfirmation(t *testing.T) {
	f := credentialFixture(t, "")
	f.status = http.StatusNoContent
	f.interactive = false
	before := f.requestCount()
	got := f.run("credential", "transfer", "credential-id", "--destination-project", "project-id")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("unconfirmed result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("credential", "transfer", "credential-id", "--destination-project", "project-id", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertCLIJSON(t, `{"id":"credential-id","destinationProjectId":"project-id","transferred":true}`, got.stdout)
	assertCLIJSON(t, `{"destinationProjectId":"project-id"}`, f.lastBody())
	if req := f.lastRequest(); req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/credentials/credential-id/transfer" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestCredentialInputSafetyAndValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		stdin       string
		args        []string
		want        string
	}{
		{name: "missing input", interactive: true, args: []string{"credential", "create"}, want: "exactly one"},
		{name: "both inputs", interactive: false, stdin: `{}`, args: []string{"credential", "create", "--stdin", "--file", "unused"}, want: "exactly one"},
		{name: "interactive stdin", interactive: true, stdin: `{"name":"x"}`, args: []string{"credential", "create", "--stdin"}, want: "may echo"},
		{name: "unknown top-level field", interactive: false, stdin: `{"name":"x","type":"x","data":{},"secret":"` + cliCredentialSecret + `"}`, args: []string{"credential", "create", "--stdin"}, want: "unknown field"},
		{name: "invalid data scalar", interactive: false, stdin: `{"name":"x","type":"x","data":"` + cliCredentialSecret + `"}`, args: []string{"credential", "create", "--stdin"}, want: "JSON object"},
		{name: "negative limit", interactive: true, args: []string{"credential", "list", "--limit", "-1"}, want: "negative"},
		{name: "missing transfer destination", interactive: true, args: []string{"credential", "transfer", "id", "--yes"}, want: "destination-project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := credentialFixture(t, cliCredential)
			f.interactive = tt.interactive
			f.stdin = tt.stdin
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid invocation reached instance")
			}
			assertNoCredentialSecret(t, got)
		})
	}
}

func TestCredentialOwnerAdminAndScopeErrors(t *testing.T) {
	for _, tt := range []struct {
		status int
		want   []string
	}{
		{status: http.StatusUnauthorized, want: []string{"401", "auth login"}},
		{status: http.StatusForbidden, want: []string{"403", "credential:list", "owner or admin", "discover --resource credential"}},
	} {
		f := credentialFixture(t, `{"data":[],"nextCursor":null}`)
		f.status = tt.status
		got := f.run("credential", "list")
		if got.code != ExitError || got.stdout != "" {
			t.Errorf("result = %+v", got)
		}
		for _, want := range tt.want {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("stderr = %q, want %q", got.stderr, want)
			}
		}
	}
}
