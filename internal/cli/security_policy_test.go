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

const cliSecurityPolicy = `{
  "personalSpacePublishing":true,
  "personalSpaceSharing":false,
  "publishedPersonalWorkflowsCount":3,
  "sharedPersonalWorkflowsCount":5,
  "sharedPersonalCredentialsCount":2,
  "redactionEnforcement":{"floor":"production"}
}`

func securityPolicyFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = cliSecurityPolicy
	return f
}

func TestSecurityPolicyGetTextShowsPolicyAndReadOnlyUsage(t *testing.T) {
	f := securityPolicyFixture(t)
	got := f.run("security-policy", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"Security policy:", f.server.URL, "Personal-space publishing:", "yes",
		"Personal-space sharing:", "no", "Redaction enforcement floor:", "production",
		"Published personal workflows:", "3", "Shared personal workflows:", "5",
		"Shared personal credentials:", "2",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.SecurityPolicyPath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
	if got := req.Header.Get(n8n.HeaderAPIKey); got != testAPIKey {
		t.Errorf("%s = %q, want API key", n8n.HeaderAPIKey, got)
	}
}

func TestSecurityPolicyGetJSONIsStable(t *testing.T) {
	f := securityPolicyFixture(t)
	got := f.run("security-policy", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	const want = `{
  "personalSpacePublishing": true,
  "personalSpaceSharing": false,
  "publishedPersonalWorkflowsCount": 3,
  "sharedPersonalWorkflowsCount": 5,
  "sharedPersonalCredentialsCount": 2,
  "redactionEnforcement": {
    "floor": "production"
  }
}
`
	if got.stdout != want {
		t.Errorf("stdout =\n%s\nwant stable JSON =\n%s", got.stdout, want)
	}
}

func TestSecurityPolicySetFromGetDocument(t *testing.T) {
	f := securityPolicyFixture(t)
	path := filepath.Join(t.TempDir(), "security-policy.json")
	input := `{
		"personalSpacePublishing":false,
		"personalSpaceSharing":true,
		"publishedPersonalWorkflowsCount":3,
		"sharedPersonalWorkflowsCount":5,
		"sharedPersonalCredentialsCount":2,
		"redactionEnforcement":{"floor":"all"}
	}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := f.run("security-policy", "set", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Updated security policy:", "Personal-space publishing:", "Redaction enforcement floor:", "Published personal workflows:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.SecurityPolicyPath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
	var sent n8n.UpdateSecurityPolicyRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not policy JSON: %v\n%s", err, f.lastBody())
	}
	if sent.PersonalSpacePublishing == nil || *sent.PersonalSpacePublishing || sent.PersonalSpaceSharing == nil || !*sent.PersonalSpaceSharing {
		t.Errorf("writable booleans = publishing %#v, sharing %#v", sent.PersonalSpacePublishing, sent.PersonalSpaceSharing)
	}
	if sent.RedactionEnforcement.Floor != n8n.SecurityRedactionAll {
		t.Errorf("floor = %q, want all", sent.RedactionEnforcement.Floor)
	}
	if sent.PublishedPersonalWorkflowsCount == nil || *sent.PublishedPersonalWorkflowsCount != 3 ||
		sent.SharedPersonalWorkflowsCount == nil || *sent.SharedPersonalWorkflowsCount != 5 ||
		sent.SharedPersonalCredentialsCount == nil || *sent.SharedPersonalCredentialsCount != 2 {
		t.Errorf("GET usage counts were not accepted and preserved: %+v", sent)
	}
}

func TestSecurityPolicySetReadsStdinAndWritesJSON(t *testing.T) {
	f := securityPolicyFixture(t)
	f.stdin = `{"personalSpacePublishing":false,"personalSpaceSharing":false,"redactionEnforcement":{"floor":"off"}}`
	got := f.run("security-policy", "set", "--input", "-", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var policy n8n.SecurityPolicy
	if err := json.Unmarshal([]byte(got.stdout), &policy); err != nil {
		t.Fatalf("stdout is not policy JSON: %v\n%s", err, got.stdout)
	}
	if policy.RedactionEnforcement.Floor != n8n.SecurityRedactionProduction {
		t.Errorf("response policy = %+v", policy)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, ok := sent["publishedPersonalWorkflowsCount"]; ok {
		t.Error("omitted read-only count was added to request")
	}
}

func TestSecurityPolicySetValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "input required", args: []string{"security-policy", "set"}, want: "--input is required"},
		{name: "unknown output", args: []string{"security-policy", "set", "--input", "-", "--output", "yaml"}, want: "unknown output format"},
		{name: "malformed", input: `{`, args: []string{"security-policy", "set", "--input", "-"}, want: "decode security policy JSON"},
		{name: "unknown field", input: `{"personalSpacePublishing":true,"personalSpaceSharing":true,"redactionEnforcement":{"floor":"off"},"surprise":true}`, args: []string{"security-policy", "set", "--input", "-"}, want: "unknown field"},
		{name: "trailing document", input: `{"personalSpacePublishing":true,"personalSpaceSharing":true,"redactionEnforcement":{"floor":"off"}} {}`, args: []string{"security-policy", "set", "--input", "-"}, want: "multiple documents"},
		{name: "missing publishing", input: `{"personalSpaceSharing":true,"redactionEnforcement":{"floor":"off"}}`, args: []string{"security-policy", "set", "--input", "-"}, want: "personalSpacePublishing"},
		{name: "missing sharing", input: `{"personalSpacePublishing":true,"redactionEnforcement":{"floor":"off"}}`, args: []string{"security-policy", "set", "--input", "-"}, want: "personalSpaceSharing"},
		{name: "invalid floor", input: `{"personalSpacePublishing":true,"personalSpaceSharing":true,"redactionEnforcement":{"floor":"sometimes"}}`, args: []string{"security-policy", "set", "--input", "-"}, want: "off, production, or all"},
		{name: "oversized", input: strings.Repeat("x", maxSecurityPolicyDocument+1), args: []string{"security-policy", "set", "--input", "-"}, want: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := securityPolicyFixture(t)
			f.stdin = tt.input
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid security policy request reached instance")
			}
		})
	}
}

func TestSecurityPolicyGetValidationBeforeTransport(t *testing.T) {
	f := securityPolicyFixture(t)
	before := f.requestCount()
	got := f.run("security-policy", "get", "--output", "yaml")
	if got.code != ExitError || !strings.Contains(got.stderr, "unknown output format") {
		t.Errorf("result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("invalid get request reached instance")
	}
}

func TestSecurityPolicyAPIErrorsExplainLicenseAndEnvironmentOwnership(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		input  string
		want   []string
	}{
		{
			name: "get license or scope failure", status: http.StatusForbidden,
			args: []string{"security-policy", "get"},
			want: []string{"securitySettings:manage", "Personal Space Policy", "discover --resource security-policy"},
		},
		{
			name: "set license or scope failure", status: http.StatusForbidden,
			args: []string{"security-policy", "set", "--input", "-"}, input: `{"personalSpacePublishing":true,"personalSpaceSharing":true,"redactionEnforcement":{"floor":"production"}}`,
			want: []string{"securitySettings:manage", "Personal Space Policy", "discover --resource security-policy"},
		},
		{
			name: "environment managed conflict", status: http.StatusConflict,
			args: []string{"security-policy", "set", "--input", "-"}, input: `{"personalSpacePublishing":true,"personalSpaceSharing":true,"redactionEnforcement":{"floor":"production"}}`,
			want: []string{"409", "managed by environment variables", "no changes were made", "security-policy get"},
		},
		{
			name: "invalid replacement", status: http.StatusBadRequest,
			args: []string{"security-policy", "set", "--input", "-"}, input: `{"personalSpacePublishing":true,"personalSpaceSharing":true,"redactionEnforcement":{"floor":"production"}}`,
			want: []string{"full security policy replacement", "personalSpacePublishing", "redactionEnforcement.floor"},
		},
		{
			name: "unauthorized", status: http.StatusUnauthorized,
			args: []string{"security-policy", "get"},
			want: []string{"rejected the credential", "auth login"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := securityPolicyFixture(t)
			f.status = tt.status
			f.stdin = tt.input
			got := f.run(tt.args...)
			if got.code != ExitError {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q: %s", want, got.stderr)
				}
			}
		})
	}
}
