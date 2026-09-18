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

const cliOIDCConfiguration = `{
  "clientId":"n8n-client",
  "clientSecret":"` + n8n.OIDCClientSecretRedactedValue + `",
  "discoveryEndpoint":"https://accounts.example.com/.well-known/openid-configuration",
  "loginEnabled":true,
  "prompt":"select_account",
  "authenticationContextClassReference":["mfa","pwd"],
  "additionalScopes":"groups roles",
  "emailVerifiedRequired":false,
  "rpInitiatedLogoutEnabled":true
}`

const cliOIDCDisabledInput = `{
  "clientId":"n8n-client",
  "clientSecret":"` + n8n.OIDCClientSecretRedactedValue + `",
  "discoveryEndpoint":"https://accounts.example.com/.well-known/openid-configuration",
  "loginEnabled":false,
  "prompt":"none",
  "authenticationContextClassReference":[],
  "additionalScopes":"",
  "emailVerifiedRequired":false,
  "rpInitiatedLogoutEnabled":false
}`

func oidcFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeOIDCInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oidc.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestOIDCGetTextHidesSecretAndShowsEverySetting(t *testing.T) {
	f := oidcFixture(t, cliOIDCConfiguration)
	got := f.run("oidc", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"OIDC configuration:", f.server.URL, "Client ID:", "n8n-client", "Client secret:", "configured (value hidden)",
		"Discovery endpoint:", "accounts.example.com", "Login enabled:", "yes", "Prompt:", "select_account",
		"Authentication context class references:", "mfa, pwd", "Additional scopes:", "groups roles",
		"Verified email required:", "no", "RP-initiated logout enabled:", "yes",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, n8n.OIDCClientSecretRedactedValue) {
		t.Error("text output printed client-secret placeholder")
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.OIDCSettingsPath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
}

func TestOIDCGetJSONSupportsSafeRoundTrip(t *testing.T) {
	f := oidcFixture(t, cliOIDCConfiguration)
	got := f.run("oidc", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var configuration n8n.OIDCConfiguration
	if err := json.Unmarshal([]byte(got.stdout), &configuration); err != nil {
		t.Fatalf("stdout is not OIDC configuration JSON: %v\n%s", err, got.stdout)
	}
	if configuration.ClientSecret != n8n.OIDCClientSecretRedactedValue || configuration.Prompt != n8n.OIDCPromptSelectAccount || !configuration.RPInitiatedLogoutEnabled {
		t.Errorf("configuration = %+v", configuration)
	}
}

func TestOIDCGetSanitizesNonconformingPlaintextResponse(t *testing.T) {
	const secret = "plaintext-client-secret"
	f := oidcFixture(t, strings.Replace(cliOIDCConfiguration, n8n.OIDCClientSecretRedactedValue, secret, 1))
	got := f.run("oidc", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, secret) || !strings.Contains(got.stdout, n8n.OIDCClientSecretRedactedValue) {
		t.Errorf("unsafe stdout: %s", got.stdout)
	}
}

func TestOIDCSetRequiresConfirmationAndPreservesFullReplacement(t *testing.T) {
	f := oidcFixture(t, cliOIDCConfiguration)
	path := writeOIDCInput(t, cliOIDCDisabledInput)
	before := f.requestCount()

	f.stdin = "no\n"
	got := f.run("oidc", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || !strings.Contains(got.stderr, "login flow immediately") {
		t.Fatalf("declined result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("declined replacement reached instance")
	}

	f.interactive = false
	got = f.run("oidc", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v", got)
	}

	got = f.run("oidc", "set", "--input", path, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.OIDCSettingsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if len(sent) != 9 || sent["loginEnabled"] != false || sent["emailVerifiedRequired"] != false || sent["rpInitiatedLogoutEnabled"] != false || sent["additionalScopes"] != "" || sent["clientSecret"] != n8n.OIDCClientSecretRedactedValue {
		t.Errorf("full replacement not preserved: %#v", sent)
	}
	if acr, ok := sent["authenticationContextClassReference"].([]any); !ok || len(acr) != 0 {
		t.Errorf("empty ACR array not preserved: %#v", sent["authenticationContextClassReference"])
	}
}

func TestOIDCSetFromStdinRequiresYesAndPreservesSentinel(t *testing.T) {
	f := oidcFixture(t, cliOIDCConfiguration)
	f.stdin = cliOIDCDisabledInput
	before := f.requestCount()
	got := f.run("oidc", "set", "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "consumes stdin") || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
		t.Errorf("result = %+v", got)
	}

	f.stdin = cliOIDCDisabledInput
	got = f.run("oidc", "set", "--input", "-", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastBody(), n8n.OIDCClientSecretRedactedValue) {
		t.Error("replacement dropped client-secret placeholder")
	}
}

func TestOIDCSetCanReplaceSecretWithoutRenderingIt(t *testing.T) {
	const secret = "new-client-secret"
	input := strings.Replace(cliOIDCDisabledInput, n8n.OIDCClientSecretRedactedValue, secret, 1)
	response := strings.Replace(cliOIDCConfiguration, n8n.OIDCClientSecretRedactedValue, secret, 1)
	f := oidcFixture(t, response)
	f.stdin = input
	got := f.run("oidc", "set", "--input", "-", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastBody(), secret) {
		t.Error("request did not submit replacement secret")
	}
	if strings.Contains(got.stdout, secret) || strings.Contains(got.stderr, secret) || !strings.Contains(got.stdout, n8n.OIDCClientSecretRedactedValue) {
		t.Errorf("secret rendered in result: %+v", got)
	}
}

func TestOIDCSetValidationAndSecretSafeErrors(t *testing.T) {
	const secret = "must-never-appear"
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "input required", args: []string{"oidc", "set"}, want: "--input is required"},
		{name: "unknown output", args: []string{"oidc", "set", "--input", "missing", "--output", "yaml"}, want: "unknown output format"},
		{name: "malformed", input: `{"clientSecret":"` + secret, args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "details redacted"},
		{name: "unknown field", input: strings.TrimSuffix(cliOIDCDisabledInput, "}") + `,"extra":"` + secret + `"}`, args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "unknown field"},
		{name: "trailing document", input: cliOIDCDisabledInput + `{}`, args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "details redacted"},
		{name: "missing required", input: `{"clientId":"id"}`, args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "clientSecret"},
		{name: "empty client ID", input: strings.Replace(cliOIDCDisabledInput, `"clientId":"n8n-client"`, `"clientId":""`, 1), args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "clientId must not be empty"},
		{name: "empty secret", input: strings.Replace(cliOIDCDisabledInput, n8n.OIDCClientSecretRedactedValue, "", 1), args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "sentinel"},
		{name: "invalid endpoint", input: strings.Replace(cliOIDCDisabledInput, "https://accounts.example.com/.well-known/openid-configuration", "not-a-url", 1), args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "discoveryEndpoint"},
		{name: "invalid prompt", input: strings.Replace(cliOIDCDisabledInput, `"prompt":"none"`, `"prompt":"signup"`, 1), args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "select_account"},
		{name: "null array", input: strings.Replace(cliOIDCDisabledInput, `"authenticationContextClassReference":[]`, `"authenticationContextClassReference":null`, 1), args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "authenticationContextClassReference"},
		{name: "oversized", input: strings.Repeat("x", maxOIDCDocument+1), args: []string{"oidc", "set", "--input", "-", "--yes"}, want: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := oidcFixture(t, cliOIDCConfiguration)
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

func TestOIDCAPIErrorsExplainLicenseConflictAndAvailability(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		input  string
		want   []string
	}{
		{name: "forbidden", status: http.StatusForbidden, args: []string{"oidc", "get"}, want: []string{"oidc:manage", "OIDC license", "discover --resource oidc"}},
		{name: "invalid replacement", status: http.StatusBadRequest, args: []string{"oidc", "set", "--input", "-", "--yes"}, input: cliOIDCDisabledInput, want: []string{"full OIDC replacement", "all nine fields", "response details redacted"}},
		{name: "environment managed", status: http.StatusConflict, args: []string{"oidc", "set", "--input", "-", "--yes"}, input: cliOIDCDisabledInput, want: []string{"managed by environment variables", "no changes were made", "oidc get"}},
		{name: "not found", status: http.StatusNotFound, args: []string{"oidc", "get"}, want: []string{"OIDC endpoint", "discover --resource oidc"}},
		{name: "unavailable", status: http.StatusServiceUnavailable, args: []string{"oidc", "get"}, want: []string{"OIDC service", "discover --resource oidc"}},
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"oidc", "get"}, want: []string{"rejected the credential", "auth login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := oidcFixture(t, cliOIDCConfiguration)
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
