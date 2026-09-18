package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// skipUnlessNodeTypePolicies skips when the instance does not serve this group.
// The routes need n8n 2.40.0 or later and the licensed
// type-availability-policies module; older or unlicensed instances answer 404
// or 503, which is an instance property and not a failure of the CLI.
func skipUnlessNodeTypePolicies(t *testing.T, err error) {
	t.Helper()
	switch {
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		t.Skip("instance does not serve node type policies: needs n8n 2.40.0 or later with the type-availability-policies module")
	case n8n.IsForbidden(err):
		t.Skip("integration credential lacks nodeTypePolicy:manage")
	}
}

// TestGetInstanceNodeTypePolicy verifies the read-only instance operation
// through both client and CLI. Neither PUT is exercised: a replacement is
// instance-wide or project-wide, deletes every rule it omits, and can stop
// workflows that use a blocked node type.
func TestGetInstanceNodeTypePolicy(t *testing.T) {
	instance := integration.Require(t)
	policy, err := instance.Client(t).GetInstanceNodeTypePolicy(context.Background())
	skipUnlessNodeTypePolicies(t, err)
	if err != nil {
		t.Fatalf("GetInstanceNodeTypePolicy: %v", err)
	}
	assertNodeTypePolicyShape(t, policy)

	out := runNodePolicyCLI(t, instance, "node-policy", "instance", "get", "--output", "json")
	var fromCLI n8n.NodeTypePolicy
	if err := json.Unmarshal([]byte(out), &fromCLI); err != nil {
		t.Fatalf("stdout is not a node type policy: %v\n%s", err, out)
	}
	if fromCLI.DefaultAction != policy.DefaultAction || fromCLI.Version != policy.Version || len(fromCLI.Rules) != len(policy.Rules) {
		t.Errorf("CLI policy differs from client: CLI=%+v client=%+v", fromCLI, policy)
	}
}

// TestGetProjectNodeTypePolicy reads one project's own policy. It needs a
// project ID because an unlicensed instance cannot list projects.
func TestGetProjectNodeTypePolicy(t *testing.T) {
	instance := integration.Require(t)
	projectID := integration.RequireProjectID(t)
	policy, err := instance.Client(t).GetProjectNodeTypePolicy(context.Background(), projectID)
	skipUnlessNodeTypePolicies(t, err)
	if err != nil {
		t.Fatalf("GetProjectNodeTypePolicy: %v", err)
	}
	assertNodeTypePolicyShape(t, policy)
	for _, rule := range policy.Rules {
		if rule.Action == n8n.NodeTypePolicyDelegate {
			t.Errorf("rule %s is delegate, which is an instance-scope action", rule.ID)
		}
	}

	out := runNodePolicyCLI(t, instance, "node-policy", "project", "get", projectID, "--output", "json")
	var fromCLI n8n.NodeTypePolicy
	if err := json.Unmarshal([]byte(out), &fromCLI); err != nil {
		t.Fatalf("stdout is not a node type policy: %v\n%s", err, out)
	}
	if fromCLI.DefaultAction != policy.DefaultAction || fromCLI.Version != policy.Version {
		t.Errorf("CLI policy differs from client: CLI=%+v client=%+v", fromCLI, policy)
	}
}

// assertNodeTypePolicyShape checks the invariants the contract promises for any
// scope, configured or not.
func assertNodeTypePolicyShape(t *testing.T, policy *n8n.NodeTypePolicy) {
	t.Helper()
	switch policy.DefaultAction {
	case n8n.NodeTypePolicyAllow, n8n.NodeTypePolicyDeny, n8n.NodeTypePolicyDelegate:
	default:
		t.Errorf("defaultAction = %q", policy.DefaultAction)
	}
	if policy.Version < 0 {
		t.Errorf("version = %d, want 0 or greater", policy.Version)
	}
	if !policy.Configured() && (len(policy.Rules) != 0 || policy.Version != 0 || policy.DefaultAction != n8n.NodeTypePolicyAllow) {
		t.Errorf("unconfigured scope = %+v, want no rules, allow, version 0", policy)
	}
	seen := map[string]bool{}
	for _, rule := range policy.Rules {
		if rule.ID == "" || seen[rule.ID] {
			t.Errorf("rule id %q is empty or duplicated", rule.ID)
		}
		seen[rule.ID] = true
		switch rule.Selector.Kind {
		case n8n.NodeTypePolicySelectorName, n8n.NodeTypePolicySelectorPackage:
		default:
			t.Errorf("rule %s selector kind = %q", rule.ID, rule.Selector.Kind)
		}
	}
}

func runNodePolicyCLI(t *testing.T, instance integration.Instance, args ...string) string {
	t.Helper()
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), args, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("%s exit code = %d (stderr: %s)", strings.Join(args, " "), code, errOut.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
	return out.String()
}
