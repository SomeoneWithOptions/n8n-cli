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

const cliSAMLConfiguration = `{
  "entityID":"https://n8n.example.com/rest/sso/saml/metadata",
  "returnUrl":"https://n8n.example.com/rest/sso/saml/acs",
  "mapping":{"email":"email","firstName":"first","lastName":"last","userPrincipalName":"upn","n8nInstanceRole":"role","n8nProjectRoles":["project:role"]},
  "metadata":"` + n8n.SAMLRedactedValue + `",
  "metadataUrl":"https://idp.example.com/metadata",
  "ignoreSSL":false,
  "loginBinding":"redirect",
  "loginEnabled":true,
  "loginLabel":"Company SSO",
  "authnRequestsSigned":true,
  "wantAssertionsSigned":true,
  "wantMessageSigned":false,
  "signingPrivateKey":"` + n8n.SAMLRedactedValue + `",
  "signingCertificate":"` + n8n.SAMLRedactedValue + `",
  "acsBinding":"post",
  "signatureConfig":{"prefix":"ds","location":{"reference":"/samlp:Response/saml:Issuer","action":"after"}},
  "relayState":"https://n8n.example.com/home"
}`

const cliSAMLDisabledInput = `{
  "entityID":"https://n8n.example.com/rest/sso/saml/metadata",
  "returnUrl":"https://n8n.example.com/rest/sso/saml/acs",
  "mapping":{"email":"email","firstName":"","lastName":"","userPrincipalName":"","n8nInstanceRole":"","n8nProjectRoles":[]},
  "metadata":"` + n8n.SAMLRedactedValue + `",
  "metadataUrl":"",
  "ignoreSSL":false,
  "loginBinding":"redirect",
  "loginEnabled":false,
  "loginLabel":"",
  "authnRequestsSigned":false,
  "wantAssertionsSigned":false,
  "wantMessageSigned":false,
  "signingPrivateKey":"` + n8n.SAMLRedactedValue + `",
  "signingCertificate":"` + n8n.SAMLRedactedValue + `",
  "acsBinding":"post",
  "signatureConfig":{"prefix":"ds","location":{"reference":"/samlp:Response/saml:Issuer","action":"after"}},
  "relayState":""
}`

func samlFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeSAMLInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "saml.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestSAMLGetTextHidesSecretsAndShowsEverySetting(t *testing.T) {
	f := samlFixture(t, cliSAMLConfiguration)
	got := f.run("saml", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"SAML configuration:", f.server.URL, "Entity ID:", "rest/sso/saml/metadata", "ACS return URL:",
		"Mapping email:", "Mapping first name:", "Mapping last name:", "Mapping user principal name:",
		"Mapping instance role:", "Mapping project roles:", "project:role", "Identity provider metadata:",
		"configured (value hidden)", "Metadata URL:", "idp.example.com", "Ignore metadata URL SSL errors:", "no",
		"Login binding:", "redirect", "Login enabled:", "yes", "Login label:", "Company SSO",
		"Authentication requests signed:", "Signed assertions required:", "Signed messages required:",
		"Signing private key:", "Signing certificate:", "ACS binding:", "post", "Signature prefix:",
		"Signature location reference:", "Signature location action:", "after", "Relay state:",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, n8n.SAMLRedactedValue) {
		t.Error("text output printed redaction placeholder")
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.SAMLSettingsPath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
}

func TestSAMLGetJSONSupportsSafeRoundTrip(t *testing.T) {
	f := samlFixture(t, cliSAMLConfiguration)
	got := f.run("saml", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var configuration n8n.SAMLConfiguration
	if err := json.Unmarshal([]byte(got.stdout), &configuration); err != nil {
		t.Fatalf("stdout is not SAML configuration JSON: %v\n%s", err, got.stdout)
	}
	if configuration.Metadata != n8n.SAMLRedactedValue || configuration.SigningPrivateKey != n8n.SAMLRedactedValue || configuration.SigningCertificate != n8n.SAMLRedactedValue || !configuration.LoginEnabled {
		t.Errorf("configuration = %+v", configuration)
	}
}

func TestSAMLGetSanitizesNonconformingPlaintextResponse(t *testing.T) {
	const secret = "plaintext-private-material"
	f := samlFixture(t, strings.ReplaceAll(cliSAMLConfiguration, n8n.SAMLRedactedValue, secret))
	got := f.run("saml", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, secret) || strings.Count(got.stdout, n8n.SAMLRedactedValue) != 3 {
		t.Errorf("unsafe stdout: %s", got.stdout)
	}
}

func TestSAMLSetRequiresConfirmationAndPreservesFullReplacement(t *testing.T) {
	f := samlFixture(t, cliSAMLConfiguration)
	path := writeSAMLInput(t, cliSAMLDisabledInput)
	before := f.requestCount()

	f.stdin = "no\n"
	got := f.run("saml", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || !strings.Contains(got.stderr, "login flow immediately") {
		t.Fatalf("declined result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("declined replacement reached instance")
	}

	f.interactive = false
	got = f.run("saml", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v", got)
	}

	got = f.run("saml", "set", "--input", path, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.SAMLSettingsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if len(sent) != 15 || sent["loginEnabled"] != false || sent["metadataUrl"] != "" || sent["relayState"] != "" || sent["signingPrivateKey"] != n8n.SAMLRedactedValue {
		t.Errorf("full replacement not preserved: %#v", sent)
	}
	if _, ok := sent["entityID"]; ok {
		t.Error("read-only entityID was sent")
	}
	if _, ok := sent["returnUrl"]; ok {
		t.Error("read-only returnUrl was sent")
	}
	mapping := sent["mapping"].(map[string]any)
	if roles, ok := mapping["n8nProjectRoles"].([]any); !ok || len(roles) != 0 {
		t.Errorf("empty project roles not preserved: %#v", mapping["n8nProjectRoles"])
	}
}

func TestSAMLSetFromStdinRequiresYesAndPreservesSentinels(t *testing.T) {
	f := samlFixture(t, cliSAMLConfiguration)
	f.stdin = cliSAMLDisabledInput
	before := f.requestCount()
	got := f.run("saml", "set", "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "consumes stdin") || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
		t.Errorf("result = %+v", got)
	}

	f.stdin = cliSAMLDisabledInput
	got = f.run("saml", "set", "--input", "-", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Count(f.lastBody(), n8n.SAMLRedactedValue) != 3 {
		t.Errorf("replacement dropped redaction placeholders: %s", f.lastBody())
	}
}

func TestSAMLSetCanReplaceSecretsWithoutRenderingThem(t *testing.T) {
	const secret = "new-private-material"
	input := strings.ReplaceAll(cliSAMLDisabledInput, n8n.SAMLRedactedValue, secret)
	response := strings.ReplaceAll(cliSAMLConfiguration, n8n.SAMLRedactedValue, secret)
	f := samlFixture(t, response)
	f.stdin = input
	got := f.run("saml", "set", "--input", "-", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Count(f.lastBody(), secret) != 3 {
		t.Errorf("request did not submit all replacement values: %s", f.lastBody())
	}
	if strings.Contains(got.stdout, secret) || strings.Contains(got.stderr, secret) || strings.Count(got.stdout, n8n.SAMLRedactedValue) != 3 {
		t.Errorf("secret rendered in result: %+v", got)
	}
}

func TestSAMLSetValidationAndSecretSafeErrors(t *testing.T) {
	const secret = "must-never-appear"
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{name: "input required", args: []string{"saml", "set"}, want: "--input is required"},
		{name: "unknown output", args: []string{"saml", "set", "--input", "missing", "--output", "yaml"}, want: "unknown output format"},
		{name: "malformed", input: `{"signingPrivateKey":"` + secret, args: []string{"saml", "set", "--input", "-", "--yes"}, want: "details redacted"},
		{name: "unknown field", input: strings.TrimSuffix(cliSAMLDisabledInput, "}") + `,"extra":"` + secret + `"}`, args: []string{"saml", "set", "--input", "-", "--yes"}, want: "unknown field"},
		{name: "trailing document", input: cliSAMLDisabledInput + `{}`, args: []string{"saml", "set", "--input", "-", "--yes"}, want: "details redacted"},
		{name: "missing required", input: `{"metadata":""}`, args: []string{"saml", "set", "--input", "-", "--yes"}, want: "mapping"},
		{name: "missing mapping field", input: strings.Replace(cliSAMLDisabledInput, `"email":"email",`, "", 1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "mapping.email"},
		{name: "null project roles", input: strings.Replace(cliSAMLDisabledInput, `"n8nProjectRoles":[]`, `"n8nProjectRoles":null`, 1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "mapping.n8nProjectRoles"},
		{name: "invalid login binding", input: strings.Replace(cliSAMLDisabledInput, `"loginBinding":"redirect"`, `"loginBinding":"soap"`, 1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "loginBinding"},
		{name: "invalid ACS binding", input: strings.Replace(cliSAMLDisabledInput, `"acsBinding":"post"`, `"acsBinding":"soap"`, 1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "acsBinding"},
		{name: "missing signature field", input: strings.Replace(cliSAMLDisabledInput, `"prefix":"ds",`, "", 1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "signatureConfig.prefix"},
		{name: "invalid signature action", input: strings.Replace(cliSAMLDisabledInput, `"action":"after"`, `"action":"inside"`, 1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "prepend"},
		{name: "oversized", input: strings.Repeat("x", maxSAMLDocument+1), args: []string{"saml", "set", "--input", "-", "--yes"}, want: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := samlFixture(t, cliSAMLConfiguration)
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

func TestSAMLAPIErrorsExplainLicenseConflictAndAvailability(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		input  string
		want   []string
	}{
		{name: "forbidden", status: http.StatusForbidden, args: []string{"saml", "get"}, want: []string{"saml:manage", "SAML license", "discover --resource settingsssosaml"}},
		{name: "invalid replacement", status: http.StatusBadRequest, args: []string{"saml", "set", "--input", "-", "--yes"}, input: cliSAMLDisabledInput, want: []string{"full SAML replacement", "15 writable fields", "response details redacted"}},
		{name: "environment managed", status: http.StatusConflict, args: []string{"saml", "set", "--input", "-", "--yes"}, input: cliSAMLDisabledInput, want: []string{"managed by environment variables", "no changes were made", "saml get"}},
		{name: "not found", status: http.StatusNotFound, args: []string{"saml", "get"}, want: []string{"SAML endpoint", "discover --resource settingsssosaml"}},
		{name: "unavailable", status: http.StatusServiceUnavailable, args: []string{"saml", "get"}, want: []string{"SAML service", "discover --resource settingsssosaml"}},
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"saml", "get"}, want: []string{"rejected the credential", "auth login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := samlFixture(t, cliSAMLConfiguration)
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
