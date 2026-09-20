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

const cliLDAPConfiguration = `{
  "loginEnabled":true,
  "loginLabel":"Company LDAP",
  "connectionUrl":"ldap.example.com",
  "allowUnauthorizedCerts":false,
  "connectionSecurity":"startTls",
  "connectionPort":389,
  "baseDn":"dc=example,dc=com",
  "bindingAdminDn":"cn=admin,dc=example,dc=com",
  "bindingAdminPassword":"` + n8n.CredentialBlankingValue + `",
  "firstNameAttribute":"givenName",
  "lastNameAttribute":"sn",
  "emailAttribute":"mail",
  "loginIdAttribute":"mail",
  "ldapIdAttribute":"uid",
  "userFilter":"(objectClass=inetOrgPerson)",
  "synchronizationEnabled":true,
  "synchronizationInterval":60,
  "searchPageSize":1000,
  "searchTimeout":30,
  "enforceEmailUniqueness":true
}`

const cliLDAPDisabledInput = `{
  "loginEnabled":false,
  "loginLabel":"LDAP",
  "connectionUrl":"ldap.example.com",
  "allowUnauthorizedCerts":false,
  "connectionSecurity":"none",
  "connectionPort":0,
  "baseDn":"",
  "bindingAdminDn":"",
  "bindingAdminPassword":"` + n8n.CredentialBlankingValue + `",
  "firstNameAttribute":"givenName",
  "lastNameAttribute":"sn",
  "emailAttribute":"mail",
  "loginIdAttribute":"mail",
  "ldapIdAttribute":"uid",
  "userFilter":"",
  "synchronizationEnabled":false,
  "synchronizationInterval":0,
  "searchPageSize":0,
  "searchTimeout":0,
  "enforceEmailUniqueness":false
}`

const cliLDAPHistory = `{
  "id":42,
  "runMode":"dry",
  "status":"success",
  "startedAt":"2026-09-17T10:00:00Z",
  "endedAt":"2026-09-17T10:00:02Z",
  "scanned":12,
  "created":2,
  "updated":3,
  "disabled":1,
  "error":""
}`

func ldapFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeLDAPInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ldap.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLDAPGetTextHidesPasswordAndShowsEverySetting(t *testing.T) {
	f := ldapFixture(t, cliLDAPConfiguration)
	got := f.run("ldap", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"LDAP configuration:", f.server.URL, "Login enabled:", "yes", "Company LDAP",
		"ldap.example.com", "Connection port:", "389", "startTls", "Allow unauthorized certificates:", "no",
		"dc=example,dc=com", "cn=admin,dc=example,dc=com", "Binding admin password:", "configured (value hidden)",
		"givenName", "sn", "mail", "uid", "(objectClass=inetOrgPerson)",
		"Synchronization enabled:", "60 minutes", "1000", "30 seconds", "Enforce email uniqueness:",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, n8n.CredentialBlankingValue) {
		t.Error("text output printed password placeholder")
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.LDAPSettingsPath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
}

func TestLDAPGetJSONSupportsSafeRoundTrip(t *testing.T) {
	f := ldapFixture(t, cliLDAPConfiguration)
	got := f.run("ldap", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var configuration n8n.LDAPConfiguration
	if err := json.Unmarshal([]byte(got.stdout), &configuration); err != nil {
		t.Fatalf("stdout is not LDAP configuration JSON: %v\n%s", err, got.stdout)
	}
	if configuration.BindingAdminPassword != n8n.CredentialBlankingValue || configuration.ConnectionSecurity != n8n.LDAPSecurityStartTLS {
		t.Errorf("configuration = %+v", configuration)
	}
	if strings.Contains(got.stdout, "plaintext-bind-secret") {
		t.Error("JSON output leaked plaintext")
	}
}

func TestLDAPGetSanitizesNonconformingPlaintextResponse(t *testing.T) {
	const secret = "plaintext-bind-secret"
	f := ldapFixture(t, strings.Replace(cliLDAPConfiguration, n8n.CredentialBlankingValue, secret, 1))
	got := f.run("ldap", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, secret) || !strings.Contains(got.stdout, n8n.CredentialBlankingValue) {
		t.Errorf("unsafe stdout: %s", got.stdout)
	}
}

func TestLDAPSetDisabledRequiresHighRiskConfirmation(t *testing.T) {
	f := ldapFixture(t, cliLDAPConfiguration)
	path := writeLDAPInput(t, cliLDAPDisabledInput)
	before := f.requestCount()

	f.stdin = "no\n"
	got := f.run("ldap", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Fatalf("declined result = %+v", got)
	}
	for _, want := range []string{"HIGH RISK", "loginEnabled=false", "deletes all stored LDAP identities", "disables synchronization"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("prompt missing %q: %s", want, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Error("declined replacement reached instance")
	}

	f.interactive = false
	got = f.run("ldap", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v", got)
	}

	got = f.run("ldap", "set", "--input", path, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.LDAPSettingsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if len(sent) != 20 || sent["loginEnabled"] != false || sent["synchronizationEnabled"] != false || sent["connectionPort"] != float64(0) || sent["bindingAdminPassword"] != n8n.CredentialBlankingValue {
		t.Errorf("full replacement not preserved: %#v", sent)
	}
}

func TestLDAPSetEnabledConfirmsImmediateReplacement(t *testing.T) {
	f := ldapFixture(t, cliLDAPConfiguration)
	path := writeLDAPInput(t, cliLDAPConfiguration)
	f.stdin = "yes\n"
	got := f.run("ldap", "set", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "change LDAP login and synchronization immediately") || strings.Contains(got.stderr, "HIGH RISK") {
		t.Errorf("prompt = %q", got.stderr)
	}
}

func TestLDAPSetFromStdinRequiresYesAndPreservesSentinel(t *testing.T) {
	f := ldapFixture(t, cliLDAPConfiguration)
	f.stdin = cliLDAPDisabledInput
	before := f.requestCount()
	got := f.run("ldap", "set", "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "consumes stdin") || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
		t.Errorf("result = %+v", got)
	}

	f.stdin = cliLDAPDisabledInput
	got = f.run("ldap", "set", "--input", "-", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastBody(), n8n.CredentialBlankingValue) {
		t.Error("replacement dropped password placeholder")
	}
}

func TestLDAPSetValidationAndSecretSafeErrors(t *testing.T) {
	const secret = "must-never-appear"
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "input required", args: []string{"ldap", "set"}, want: "--input is required"},
		{name: "unknown output", args: []string{"ldap", "set", "--input", "missing", "--output", "yaml"}, want: "unknown output format"},
		{name: "malformed", input: `{"bindingAdminPassword":"` + secret, args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "details redacted"},
		{name: "unknown field", input: strings.TrimSuffix(cliLDAPDisabledInput, "}") + `,"secretField":"` + secret + `"}`, args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "unknown field"},
		{name: "trailing document", input: cliLDAPDisabledInput + `{}`, args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "details redacted"},
		{name: "missing required", input: `{"loginEnabled":false}`, args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "loginLabel"},
		{name: "invalid security", input: strings.Replace(cliLDAPDisabledInput, `"connectionSecurity":"none"`, `"connectionSecurity":"ssl"`, 1), args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "none, tls, or startTls"},
		{name: "fractional integer", input: strings.Replace(cliLDAPDisabledInput, `"connectionPort":0`, `"connectionPort":1.5`, 1), args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "cannot unmarshal number"},
		{name: "oversized", input: strings.Repeat("x", maxLDAPDocument+1), args: []string{"ldap", "set", "--input", "-", "--yes"}, want: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ldapFixture(t, cliLDAPConfiguration)
			f.stdin = tt.input
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if strings.Contains(got.stderr, secret) || strings.Contains(got.stdout, secret) {
				t.Error("validation output leaked secret input")
			}
			if f.requestCount() != before {
				t.Error("invalid replacement reached instance")
			}
		})
	}
}

func TestLDAPSyncHistoryTextJSONPaginationAndAll(t *testing.T) {
	f := ldapFixture(t, `{"data":[`+cliLDAPHistory+`],"nextCursor":"next page"}`)
	got := f.run("ldap", "sync", "history", "--limit", "25", "--cursor", "start")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"LDAP synchronizations:", "42", "dry", "success", "12", "2", "3", "1", "Next cursor:", "next page"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if query := f.lastRequest().URL.Query(); query.Get("limit") != "25" || query.Get("cursor") != "start" {
		t.Errorf("query = %q", f.lastRequest().URL.RawQuery)
	}

	pages := []string{
		`{"data":[` + cliLDAPHistory + `],"nextCursor":"page-two"}`,
		`{"data":[` + strings.Replace(cliLDAPHistory, `"id":42`, `"id":43`, 1) + `],"nextCursor":null}`,
	}
	f.bodyFunc = func(n int) string { return pages[n%len(pages)] }
	f.pages = 0
	got = f.run("ldap", "sync", "history", "--all", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("all exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.LDAPSyncHistory]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 || page.Data[1].ID != 43 || page.NextCursor != "" {
		t.Errorf("stdout = %q (error: %v), page=%+v", got.stdout, err, page)
	}
	if got := f.lastRequest().URL.Query().Get("cursor"); got != "page-two" {
		t.Errorf("last cursor = %q", got)
	}
}

func TestLDAPSyncHistoryEmptyAndValidation(t *testing.T) {
	f := ldapFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("ldap", "sync", "history")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "sync run --type dry") {
		t.Errorf("result = %+v", got)
	}
	before := f.requestCount()
	for _, args := range [][]string{
		{"ldap", "sync", "history", "--limit", "-1"},
		{"ldap", "sync", "history", "--limit", "251"},
		{"ldap", "sync", "history", "--output", "yaml"},
	} {
		got = f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v result = %+v", args, got)
		}
	}
	if f.requestCount() != before {
		t.Error("invalid history invocation reached instance")
	}
}

func TestLDAPSyncRunDryNeedsNoConfirmation(t *testing.T) {
	f := ldapFixture(t, cliLDAPHistory)
	f.interactive = false
	got := f.run("ldap", "sync", "run", "--type", "dry")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"LDAP synchronization:", "42", "Mode:", "dry", "Scanned:", "12", "Created:", "2", "Updated:", "3", "Disabled:", "1"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if req := f.lastRequest(); req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+n8n.LDAPSyncPath {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	assertCLIJSON(t, `{"type":"dry"}`, f.lastBody())
}

func TestLDAPSyncRunLiveRequiresConfirmation(t *testing.T) {
	f := ldapFixture(t, strings.Replace(cliLDAPHistory, `"dry"`, `"live"`, 1))
	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("ldap", "sync", "run", "--type", "live")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || !strings.Contains(got.stderr, "may disable users") || f.requestCount() != before {
		t.Errorf("declined result = %+v", got)
	}
	f.interactive = false
	got = f.run("ldap", "sync", "run", "--type", "live")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v", got)
	}
	got = f.run("ldap", "sync", "run", "--type", "live", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	var history n8n.LDAPSyncHistory
	if err := json.Unmarshal([]byte(got.stdout), &history); err != nil || history.RunMode != n8n.LDAPSyncLive {
		t.Errorf("stdout = %q (error: %v)", got.stdout, err)
	}
	assertCLIJSON(t, `{"type":"live"}`, f.lastBody())
}

func TestLDAPSyncRunValidationBeforeTransport(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{args: []string{"ldap", "sync", "run"}, want: "live or dry"},
		{args: []string{"ldap", "sync", "run", "--type", "weekly"}, want: "live or dry"},
		{args: []string{"ldap", "sync", "run", "--type", "dry", "--output", "yaml"}, want: "unknown output format"},
	} {
		f := ldapFixture(t, cliLDAPHistory)
		before := f.requestCount()
		got := f.run(tt.args...)
		if got.code != ExitError || !strings.Contains(got.stderr, tt.want) || f.requestCount() != before {
			t.Errorf("result = %+v", got)
		}
	}
}

func TestLDAPAPIErrorsExplainLicenseScopesAndAvailability(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		input  string
		want   []string
	}{
		{name: "manage forbidden", status: http.StatusForbidden, args: []string{"ldap", "get"}, want: []string{"ldap:manage", "LDAP license", "discover --resource settingsldap"}},
		{name: "sync forbidden", status: http.StatusForbidden, args: []string{"ldap", "sync", "history"}, want: []string{"ldap:sync", "LDAP license", "discover --resource settingsldap"}},
		{name: "invalid replacement", status: http.StatusBadRequest, args: []string{"ldap", "set", "--input", "-", "--yes"}, input: cliLDAPDisabledInput, want: []string{"full LDAP replacement", "all 20 fields", "response details redacted"}},
		{name: "invalid sync", status: http.StatusBadRequest, args: []string{"ldap", "sync", "run", "--type", "dry"}, want: []string{"rejected the LDAP synchronization", "configuration"}},
		{name: "not found", status: http.StatusNotFound, args: []string{"ldap", "get"}, want: []string{"LDAP endpoint", "discover --resource settingsldap"}},
		{name: "unavailable", status: http.StatusServiceUnavailable, args: []string{"ldap", "sync", "history"}, want: []string{"LDAP service", "discover --resource settingsldap"}},
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"ldap", "get"}, want: []string{"rejected the credential", "auth login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ldapFixture(t, cliLDAPConfiguration)
			f.status = tt.status
			f.stdin = tt.input
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Fatalf("result = %+v", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q: %s", want, got.stderr)
				}
			}
		})
	}
}
