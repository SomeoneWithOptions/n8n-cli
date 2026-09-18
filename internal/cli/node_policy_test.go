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

const cliNodePolicy = `{
  "scopeId":"instance-1",
  "rules":[
    {"id":"deny-http","action":"deny","selector":{"kind":"name","value":"n8n-nodes-base.httpRequest"}},
    {"id":"delegate-weather","action":"delegate","selector":{"kind":"package","value":"n8n-nodes-weather"}}
  ],
  "defaultAction":"allow",
  "version":7
}`

const cliNodePolicyUnconfigured = `{"scopeId":null,"rules":[],"defaultAction":"allow","version":0}`

const cliNodePolicyWriteResult = `{
  "scopeId":"instance-1",
  "rules":[{"id":"deny-http","action":"deny","selector":{"kind":"name","value":"n8n-nodes-base.httpRequest"}}],
  "defaultAction":"deny",
  "version":8,
  "warnings":[{"ruleId":"deny-http-copy","shadowedByRuleId":"deny-http"}]
}`

const cliNodePolicyInput = `{
  "scopeId":"instance-1",
  "rules":[{"id":"deny-http","action":"deny","selector":{"kind":"name","value":"n8n-nodes-base.httpRequest"}}],
  "defaultAction":"deny",
  "version":7,
  "warnings":[]
}`

func nodePolicyFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeNodePolicyInput(t *testing.T, document string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "node-policy.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestNodePolicyInstanceGetTextShowsRulesInEvaluationOrder(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicy)
	got := f.run("node-policy", "instance", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"Instance node type policy:", f.server.URL, "Scope document:", "instance-1",
		"Default action:", "allow", "Version:", "7", "Rules:", "2",
		"ORDER", "deny-http", "name=n8n-nodes-base.httpRequest",
		"delegate", "package=n8n-nodes-weather",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Index(got.stdout, "deny-http") > strings.Index(got.stdout, "delegate-weather") {
		t.Errorf("rules are not printed in evaluation order:\n%s", got.stdout)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+n8n.NodeTypePolicyInstancePath || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
	if got := req.Header.Get(n8n.HeaderAPIKey); got != testAPIKey {
		t.Errorf("%s = %q, want API key", n8n.HeaderAPIKey, got)
	}
}

func TestNodePolicyGetReportsUnconfiguredScope(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicyUnconfigured)
	got := f.run("node-policy", "instance", "get")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"none (never configured", "Default action:", "allow", "Version:", "0"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "ORDER") {
		t.Errorf("empty rule list printed a table:\n%s", got.stdout)
	}
}

func TestNodePolicyInstanceGetJSONIsStable(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicyUnconfigured)
	got := f.run("node-policy", "instance", "get", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	const want = `{
  "scopeId": null,
  "rules": [],
  "defaultAction": "allow",
  "version": 0
}
`
	if got.stdout != want {
		t.Errorf("stdout =\n%s\nwant stable JSON =\n%s", got.stdout, want)
	}
}

func TestNodePolicyProjectGetEscapesProjectIDAndRequiresIt(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicy)
	got := f.run("node-policy", "project", "get", "a/b")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "Node type policy of project a/b:") {
		t.Errorf("stdout lacks the project heading:\n%s", got.stdout)
	}
	req := f.lastRequest()
	want := n8n.BasePath + "/node-type-policies/projects/a%2Fb"
	if req.URL.EscapedPath() != want {
		t.Errorf("escaped path = %q, want %q", req.URL.EscapedPath(), want)
	}

	before := f.requestCount()
	missing := f.run("node-policy", "project", "get")
	if missing.code != ExitError {
		t.Errorf("missing project ID exit = %d, want an error", missing.code)
	}
	if f.requestCount() != before {
		t.Error("a request without a project ID reached the instance")
	}
}

func TestNodePolicySetSendsReplacementWithoutServerOwnedFields(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicyWriteResult)
	path := writeNodePolicyInput(t, cliNodePolicyInput)
	got := f.run("node-policy", "instance", "set", "--input", path, "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+n8n.NodeTypePolicyInstancePath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, f.lastBody())
	}
	if _, ok := sent["scopeId"]; ok {
		t.Errorf("server-owned scopeId was sent back: %#v", sent)
	}
	if _, ok := sent["warnings"]; ok {
		t.Errorf("server-owned warnings were sent back: %#v", sent)
	}
	if sent["defaultAction"] != "deny" || sent["version"] != float64(7) {
		t.Errorf("body = %#v, want the edited default action and the version last read", sent)
	}
	if rules, ok := sent["rules"].([]any); !ok || len(rules) != 1 {
		t.Errorf("rules = %#v, want the one edited rule", sent["rules"])
	}
	for _, want := range []string{"Replaced instance node type policy:", "Version:", "8", "never matches", "deny-http-copy", "deny-http"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
}

func TestNodePolicySetRequiresConfirmation(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicyWriteResult)
	path := writeNodePolicyInput(t, cliNodePolicyInput)
	before := f.requestCount()

	f.stdin = "n\n"
	got := f.run("node-policy", "instance", "set", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Fatalf("declined result = %+v", got)
	}
	for _, want := range []string{"Replace the node type policy of the instance", "1 rule", "deny", "Rules not in the document are deleted"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("confirmation missing %q: %s", want, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Error("declined replacement reached instance")
	}

	f.stdin = "yes\n"
	got = f.run("node-policy", "instance", "set", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
}

func TestNodePolicySetStdinRequiresExplicitConfirmation(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicyWriteResult)
	f.stdin = cliNodePolicyInput
	before := f.requestCount()
	got := f.run("node-policy", "instance", "set", "--input", "-")
	if got.code != ExitError || !strings.Contains(got.stderr, "consumes stdin") || !strings.Contains(got.stderr, "--yes") {
		t.Errorf("result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("unconfirmed stdin replacement reached instance")
	}
}

func TestNodePolicyProjectSetRejectsDelegateBeforeTransport(t *testing.T) {
	f := nodePolicyFixture(t, cliNodePolicyWriteResult)
	path := writeNodePolicyInput(t, `{"rules":[],"defaultAction":"delegate","version":0}`)
	before := f.requestCount()
	got := f.run("node-policy", "project", "set", "proj-1", "--input", path, "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "instance-scope action") {
		t.Errorf("result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("a project delegate policy reached the instance")
	}
}

func TestNodePolicySetValidatesInput(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		input string
		want  string
	}{
		{name: "missing input flag", args: []string{"node-policy", "instance", "set", "--yes"}, want: "--input is required"},
		{name: "unknown field", input: `{"rules":[],"defaultAction":"allow","version":0,"extra":1}`, want: "decode node type policy JSON"},
		{name: "missing version", input: `{"rules":[],"defaultAction":"allow"}`, want: "version is required"},
		{name: "missing default action", input: `{"rules":[],"version":0}`, want: "defaultAction is required"},
		{name: "duplicate rule id", input: `{"rules":[
			{"id":"a","action":"deny","selector":{"kind":"name","value":"x"}},
			{"id":"a","action":"allow","selector":{"kind":"name","value":"y"}}
		],"defaultAction":"allow","version":0}`, want: "rule ids must be unique"},
		{name: "unknown selector kind", input: `{"rules":[
			{"id":"a","action":"deny","selector":{"kind":"regex","value":"x"}}
		],"defaultAction":"allow","version":0}`, want: "selector.kind must be"},
		{name: "two documents", input: `{"rules":[],"defaultAction":"allow","version":0} {"rules":[]}`, want: "multiple documents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := nodePolicyFixture(t, cliNodePolicyWriteResult)
			args := tt.args
			if args == nil {
				args = []string{"node-policy", "instance", "set", "--input", writeNodePolicyInput(t, tt.input), "--yes"}
			}
			before := f.requestCount()
			got := f.run(args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want an error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("an invalid replacement reached the instance")
			}
		})
	}
}

func TestNodePolicyAPIErrorsExplainAvailabilityAndConflict(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{
			name:   "not found",
			status: http.StatusNotFound,
			args:   []string{"node-policy", "instance", "get"},
			want:   []string{"does not serve node type policies (404)", "2.40.0", "type-availability-policies", "n8n discover --resource nodetypepolicy"},
		},
		{
			name:   "module disabled",
			status: http.StatusServiceUnavailable,
			args:   []string{"node-policy", "instance", "get"},
			want:   []string{"does not serve node type policies (503)", "type-availability-policies"},
		},
		{
			name:   "forbidden",
			status: http.StatusForbidden,
			args:   []string{"node-policy", "instance", "get"},
			want:   []string{"denied node type policy read (403)", "nodeTypePolicy:manage"},
		},
		{
			name:   "stale version",
			status: http.StatusConflict,
			args:   []string{"node-policy", "instance", "set", "--yes"},
			want:   []string{"(409)", "changed by someone else", "n8n node-policy instance get"},
		},
		{
			name:   "project conflict names shared documents",
			status: http.StatusConflict,
			args:   []string{"node-policy", "project", "set", "proj-1", "--yes"},
			want:   []string{"(409)", "shared with another scope", "n8n node-policy project get"},
		},
		{
			name:   "bad request",
			status: http.StatusBadRequest,
			args:   []string{"node-policy", "instance", "set", "--yes"},
			want:   []string{"(400)", "unique rule ids", "kind"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := nodePolicyFixture(t, cliNodePolicyWriteResult)
			f.status = tt.status
			args := tt.args
			if args[2] == "set" || (len(args) > 3 && args[3] == "set") {
				args = append(args, "--input", writeNodePolicyInput(t, cliNodePolicyInput))
			}
			got := f.run(args...)
			if got.code != ExitError {
				t.Fatalf("exit = %d, want an error (stdout: %s)", got.code, got.stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q:\n%s", want, got.stderr)
				}
			}
		})
	}
}
