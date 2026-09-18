package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const unconfiguredNodeTypePolicyResponse = `{
  "scopeId":null,
  "rules":[],
  "defaultAction":"allow",
  "version":0
}`

const instanceNodeTypePolicyResponse = `{
  "scopeId":"instance-1",
  "rules":[
    {"id":"deny-http","action":"deny","selector":{"kind":"name","value":"n8n-nodes-base.httpRequest"}},
    {"id":"delegate-community","action":"delegate","selector":{"kind":"package","value":"n8n-nodes-weather"}}
  ],
  "defaultAction":"allow",
  "version":7
}`

const nodeTypePolicyWriteResponse = `{
  "scopeId":"instance-1",
  "rules":[{"id":"deny-http","action":"deny","selector":{"kind":"name","value":"n8n-nodes-base.httpRequest"}}],
  "defaultAction":"deny",
  "version":8,
  "warnings":[{"ruleId":"deny-http-again","shadowedByRuleId":"deny-http"}]
}`

func action(a NodeTypePolicyAction) *NodeTypePolicyAction { return &a }

func version(v int) *int { return &v }

func TestGetInstanceNodeTypePolicy(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, instanceNodeTypePolicyResponse))
	policy, err := server.client(t).GetInstanceNodeTypePolicy(context.Background())
	if err != nil {
		t.Fatalf("GetInstanceNodeTypePolicy: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+NodeTypePolicyInstancePath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.EscapedPath(), BasePath+NodeTypePolicyInstancePath)
	}
	if req.URL.RawQuery != "" || body != "" {
		t.Errorf("query, body = %q, %q; want empty", req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !policy.Configured() || *policy.ScopeID != "instance-1" || policy.DefaultAction != NodeTypePolicyAllow || policy.Version != 7 {
		t.Errorf("policy = %+v", policy)
	}
	if len(policy.Rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(policy.Rules))
	}
	if policy.Rules[0].ID != "deny-http" || policy.Rules[0].Action != NodeTypePolicyDeny ||
		policy.Rules[0].Selector.Kind != NodeTypePolicySelectorName || policy.Rules[0].Selector.Value != "n8n-nodes-base.httpRequest" {
		t.Errorf("rules[0] = %+v", policy.Rules[0])
	}
	if policy.Rules[1].Action != NodeTypePolicyDelegate || policy.Rules[1].Selector.Kind != NodeTypePolicySelectorPackage {
		t.Errorf("rules[1] = %+v", policy.Rules[1])
	}
}

func TestGetInstanceNodeTypePolicyUnconfiguredScope(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, unconfiguredNodeTypePolicyResponse))
	policy, err := server.client(t).GetInstanceNodeTypePolicy(context.Background())
	if err != nil {
		t.Fatalf("GetInstanceNodeTypePolicy: %v", err)
	}
	if policy.Configured() {
		t.Errorf("scopeId = %v, want nil for a scope that was never configured", *policy.ScopeID)
	}
	if len(policy.Rules) != 0 || policy.DefaultAction != NodeTypePolicyAllow || policy.Version != 0 {
		t.Errorf("policy = %+v, want no rules, allow, version 0", policy)
	}
}

func TestGetProjectNodeTypePolicyEscapesProjectID(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, unconfiguredNodeTypePolicyResponse))
	if _, err := server.client(t).GetProjectNodeTypePolicy(context.Background(), "a/b c"); err != nil {
		t.Fatalf("GetProjectNodeTypePolicy: %v", err)
	}
	req, _ := server.last(t)
	want := BasePath + "/node-type-policies/projects/a%2Fb%20c"
	if req.URL.EscapedPath() != want {
		t.Errorf("escaped path = %q, want %q", req.URL.EscapedPath(), want)
	}
}

func TestGetProjectNodeTypePolicyRequiresProjectID(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, unconfiguredNodeTypePolicyResponse))
	if _, err := server.client(t).GetProjectNodeTypePolicy(context.Background(), ""); err == nil {
		t.Fatal("empty project ID reached the instance")
	}
	if len(server.requests) != 0 {
		t.Errorf("%d request(s) reached the instance, want none", len(server.requests))
	}
}

func TestReplaceInstanceNodeTypePolicy(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, nodeTypePolicyWriteResponse))
	result, err := server.client(t).ReplaceInstanceNodeTypePolicy(context.Background(), ReplaceNodeTypePolicyRequest{
		Rules: []NodeTypePolicyRule{{
			ID:       "deny-http",
			Action:   NodeTypePolicyDeny,
			Selector: NodeTypePolicySelector{Kind: NodeTypePolicySelectorName, Value: "n8n-nodes-base.httpRequest"},
		}},
		DefaultAction: action(NodeTypePolicyDelegate),
		Version:       version(7),
	})
	if err != nil {
		t.Fatalf("ReplaceInstanceNodeTypePolicy: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+NodeTypePolicyInstancePath {
		t.Errorf("request = %s %s, want PUT %s", req.Method, req.URL.EscapedPath(), BasePath+NodeTypePolicyInstancePath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	if sent["defaultAction"] != "delegate" || sent["version"] != float64(7) {
		t.Errorf("body = %#v, want defaultAction delegate and version 7", sent)
	}
	rules, ok := sent["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("rules = %#v, want one rule", sent["rules"])
	}
	rule := rules[0].(map[string]any)
	selector := rule["selector"].(map[string]any)
	if rule["id"] != "deny-http" || rule["action"] != "deny" || selector["kind"] != "name" || selector["value"] != "n8n-nodes-base.httpRequest" {
		t.Errorf("rules[0] = %#v", rule)
	}
	if result.Version != 8 || result.DefaultAction != NodeTypePolicyDeny {
		t.Errorf("result = %+v, want the stored version and default action", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].RuleID != "deny-http-again" || result.Warnings[0].ShadowedByRuleID != "deny-http" {
		t.Errorf("warnings = %+v", result.Warnings)
	}
}

func TestReplaceNodeTypePolicySendsEmptyRuleList(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, nodeTypePolicyWriteResponse))
	if _, err := server.client(t).ReplaceInstanceNodeTypePolicy(context.Background(), ReplaceNodeTypePolicyRequest{
		DefaultAction: action(NodeTypePolicyDeny),
		Version:       version(0),
	}); err != nil {
		t.Fatalf("ReplaceInstanceNodeTypePolicy: %v", err)
	}
	_, body := server.last(t)
	if !strings.Contains(body, `"rules":[]`) {
		t.Errorf("body = %s, want an empty rule array rather than null", body)
	}
}

func TestReplaceProjectNodeTypePolicy(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, nodeTypePolicyWriteResponse))
	if _, err := server.client(t).ReplaceProjectNodeTypePolicy(context.Background(), "proj-1", ReplaceNodeTypePolicyRequest{
		Rules: []NodeTypePolicyRule{{
			ID:       "allow-slack",
			Action:   NodeTypePolicyAllow,
			Selector: NodeTypePolicySelector{Kind: NodeTypePolicySelectorPackage, Value: "n8n-nodes-base"},
		}},
		DefaultAction: action(NodeTypePolicyDeny),
		Version:       version(3),
	}); err != nil {
		t.Fatalf("ReplaceProjectNodeTypePolicy: %v", err)
	}
	req, _ := server.last(t)
	want := BasePath + "/node-type-policies/projects/proj-1"
	if req.Method != http.MethodPut || req.URL.EscapedPath() != want {
		t.Errorf("request = %s %s, want PUT %s", req.Method, req.URL.EscapedPath(), want)
	}
}

func TestReplaceProjectNodeTypePolicyRejectsDelegate(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, nodeTypePolicyWriteResponse))
	_, err := server.client(t).ReplaceProjectNodeTypePolicy(context.Background(), "proj-1", ReplaceNodeTypePolicyRequest{
		DefaultAction: action(NodeTypePolicyDelegate),
		Version:       version(0),
	})
	if err == nil {
		t.Fatal("delegate reached the instance at project scope")
	}
	if !strings.Contains(err.Error(), "instance-scope action") {
		t.Errorf("error = %v, want it to explain that delegate is instance-scope only", err)
	}
	if len(server.requests) != 0 {
		t.Errorf("%d request(s) reached the instance, want none", len(server.requests))
	}
}

func TestReplaceNodeTypePolicyValidationBeforeTransport(t *testing.T) {
	rule := func(id string, a NodeTypePolicyAction, kind NodeTypePolicySelectorKind, value string) NodeTypePolicyRule {
		return NodeTypePolicyRule{ID: id, Action: a, Selector: NodeTypePolicySelector{Kind: kind, Value: value}}
	}
	tests := []struct {
		name    string
		request ReplaceNodeTypePolicyRequest
		want    string
	}{
		{
			name:    "missing default action",
			request: ReplaceNodeTypePolicyRequest{Version: version(0)},
			want:    "defaultAction is required",
		},
		{
			name:    "unknown default action",
			request: ReplaceNodeTypePolicyRequest{DefaultAction: action("block"), Version: version(0)},
			want:    "defaultAction:",
		},
		{
			name:    "missing version",
			request: ReplaceNodeTypePolicyRequest{DefaultAction: action(NodeTypePolicyAllow)},
			want:    "version is required",
		},
		{
			name:    "negative version",
			request: ReplaceNodeTypePolicyRequest{DefaultAction: action(NodeTypePolicyAllow), Version: version(-1)},
			want:    "version must be 0 or greater",
		},
		{
			name: "empty rule id",
			request: ReplaceNodeTypePolicyRequest{
				Rules:         []NodeTypePolicyRule{rule("", NodeTypePolicyDeny, NodeTypePolicySelectorName, "x")},
				DefaultAction: action(NodeTypePolicyAllow), Version: version(0),
			},
			want: "rules[0]: id is required",
		},
		{
			name: "duplicate rule id",
			request: ReplaceNodeTypePolicyRequest{
				Rules: []NodeTypePolicyRule{
					rule("dup", NodeTypePolicyDeny, NodeTypePolicySelectorName, "x"),
					rule("dup", NodeTypePolicyAllow, NodeTypePolicySelectorName, "y"),
				},
				DefaultAction: action(NodeTypePolicyAllow), Version: version(0),
			},
			want: "rule ids must be unique",
		},
		{
			name: "unknown selector kind",
			request: ReplaceNodeTypePolicyRequest{
				Rules:         []NodeTypePolicyRule{rule("r1", NodeTypePolicyDeny, "regex", "x")},
				DefaultAction: action(NodeTypePolicyAllow), Version: version(0),
			},
			want: "selector.kind must be",
		},
		{
			name: "empty selector value",
			request: ReplaceNodeTypePolicyRequest{
				Rules:         []NodeTypePolicyRule{rule("r1", NodeTypePolicyDeny, NodeTypePolicySelectorName, "")},
				DefaultAction: action(NodeTypePolicyAllow), Version: version(0),
			},
			want: "selector.value is required",
		},
		{
			name: "unknown rule action",
			request: ReplaceNodeTypePolicyRequest{
				Rules:         []NodeTypePolicyRule{rule("r1", "warn", NodeTypePolicySelectorName, "x")},
				DefaultAction: action(NodeTypePolicyAllow), Version: version(0),
			},
			want: "rules[0] (r1): action must be",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, nodeTypePolicyWriteResponse))
			_, err := server.client(t).ReplaceInstanceNodeTypePolicy(context.Background(), tt.request)
			if err == nil {
				t.Fatal("invalid replacement reached the instance")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d request(s) reached the instance, want none", len(server.requests))
			}
		})
	}
}

func TestReplaceNodeTypePolicyStaleVersionConflict(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `{"message":"version is stale"}`))
	_, err := server.client(t).ReplaceInstanceNodeTypePolicy(context.Background(), ReplaceNodeTypePolicyRequest{
		DefaultAction: action(NodeTypePolicyAllow),
		Version:       version(2),
	})
	if !IsConflict(err) {
		t.Fatalf("error = %v, want a 409", err)
	}
}

func TestNodeTypePolicyModuleDisabled(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusServiceUnavailable, `{"message":"module disabled"}`))
	_, err := server.client(t).GetInstanceNodeTypePolicy(context.Background())
	if !IsStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("error = %v, want a 503", err)
	}
}

func TestNodeTypePolicyEmptyResponseBody(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	policy, err := server.client(t).GetInstanceNodeTypePolicy(context.Background())
	if err != nil {
		t.Fatalf("GetInstanceNodeTypePolicy: %v", err)
	}
	if policy.Configured() || len(policy.Rules) != 0 || policy.Version != 0 {
		t.Errorf("policy = %+v, want the zero policy for an empty body", policy)
	}
}
