package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const policyDocumentJSON = `{"id":"p1","kind":"future-kind","rules":[],"version":2,"updatedBy":"u1","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`
const policyDocumentResultJSON = `{"policy":` + policyDocumentJSON + `,"warnings":[{"kind":"notLoaded","ruleId":"r1"}]}`
const policyAttachmentsJSON = `{"attachments":[{"policyId":"p1","rules":[],"priority":0,"isFloor":false}],"version":2}`

func policyTestAPIs(c *Client) map[string]PolicyClient {
	return map[string]PolicyClient{"node-type-policies": c.NodeTypePolicies(), "credential-type-policies": c.CredentialTypePolicies()}
}
func policyBool(v bool) *bool { return &v }

type policyOperation struct {
	name, method, path, body, response string
	status                             int
	call                               func(context.Context, PolicyClient) error
}

func policyOperations() []policyOperation {
	return []policyOperation{
		{"list", "GET", "/policies", "", `{"data":[` + policyDocumentJSON + `],"nextCursor":"next"}`, 200, func(ctx context.Context, p PolicyClient) error {
			r, e := p.ListDocuments(ctx, ListOptions{Limit: 1, Cursor: "a+/ b"})
			if e == nil && (len(r.Data) != 1 || r.Data[0].Kind != "future-kind" || r.NextCursor != "next") {
				return errors.New("list decode lost fields")
			}
			return e
		}},
		{"get", "GET", "/policies/a%2Fb%20c", "", policyDocumentJSON, 200, func(ctx context.Context, p PolicyClient) error {
			r, e := p.GetDocument(ctx, "a/b c")
			if e == nil && (r.ID != "p1" || r.Version != 2 || r.UpdatedBy != "u1") {
				return errors.New("document decode lost fields")
			}
			return e
		}},
		{"create", "POST", "/policies", `{"rules":[]}`, policyDocumentResultJSON, 201, func(ctx context.Context, p PolicyClient) error {
			r, e := p.CreateDocument(ctx, CreatePolicyDocumentRequest{})
			if e == nil && r.Policy.ID != "" && (r.Policy.ID != "p1" || len(r.Warnings) != 1) {
				return errors.New("write decode lost fields")
			}
			return e
		}},
		{"update", "PUT", "/policies/a%2Fb%20c", `{"rules":[],"version":2}`, policyDocumentResultJSON, 200, func(ctx context.Context, p PolicyClient) error {
			_, e := p.UpdateDocument(ctx, "a/b c", UpdatePolicyDocumentRequest{Version: version(2)})
			return e
		}},
		{"delete", "DELETE", "/policies/a%2Fb%20c", "", "", 204, func(ctx context.Context, p PolicyClient) error { return p.DeleteDocument(ctx, "a/b c") }},
		{"attachments get", "GET", "/scopes/a%2Fb%20c/attachments", "", policyAttachmentsJSON, 200, func(ctx context.Context, p PolicyClient) error {
			r, e := p.GetAttachments(ctx, "a/b c")
			if e == nil && (len(r.Attachments) != 1 || r.Version != 2 || r.Attachments[0].IsFloor) {
				return errors.New("attachment decode lost fields")
			}
			return e
		}},
		{"attachments replace", "PUT", "/scopes/a%2Fb%20c/attachments", `{"attachments":[{"policyId":"p1","priority":0,"isFloor":false}],"version":2}`, policyAttachmentsJSON, 200, func(ctx context.Context, p PolicyClient) error {
			_, e := p.ReplaceAttachments(ctx, "a/b c", ReplacePolicyAttachmentsRequest{Attachments: []PolicyAttachmentInput{{PolicyID: "p1", Priority: version(0), IsFloor: policyBool(false)}}, Version: version(2)})
			return e
		}},
		{"instance get", "GET", "/instance", "", unconfiguredNodeTypePolicyResponse, 200, func(ctx context.Context, p PolicyClient) error {
			r, e := p.GetInstance(ctx)
			if e == nil && (r.Configured() || r.Version != 0) {
				return errors.New("scope decode lost fields")
			}
			return e
		}},
		{"project get", "GET", "/projects/a%2Fb%20c", "", unconfiguredNodeTypePolicyResponse, 200, func(ctx context.Context, p PolicyClient) error { _, e := p.GetProject(ctx, "a/b c"); return e }},
		{"instance replace", "PUT", "/instance", `{"rules":[],"defaultAction":"allow","version":0}`, nodeTypePolicyWriteResponse, 200, func(ctx context.Context, p PolicyClient) error {
			_, e := p.ReplaceInstance(ctx, ReplaceNodeTypePolicyRequest{DefaultAction: action(NodeTypePolicyAllow), Version: version(0)})
			return e
		}},
		{"project replace", "PUT", "/projects/a%2Fb%20c", `{"rules":[],"defaultAction":"allow","version":0}`, nodeTypePolicyWriteResponse, 200, func(ctx context.Context, p PolicyClient) error {
			_, e := p.ReplaceProject(ctx, "a/b c", ReplaceNodeTypePolicyRequest{DefaultAction: action(NodeTypePolicyAllow), Version: version(0)})
			return e
		}},
	}
}
func TestPolicyTransport(t *testing.T) {
	for _, group := range []string{"node-type-policies", "credential-type-policies"} {
		for _, op := range policyOperations() {
			t.Run(group+"/"+op.name, func(t *testing.T) {
				server := newCommunityPackageServer(t, packageJSONHandler(op.status, op.response))
				p := policyTestAPIs(server.client(t))[group]
				if err := op.call(context.Background(), p); err != nil {
					t.Fatal(err)
				}
				req, body := server.last(t)
				if req.Method != op.method || req.URL.EscapedPath() != BasePath+"/"+group+op.path {
					t.Errorf("request: %s %s", req.Method, req.URL.EscapedPath())
				}
				if req.Header.Get(HeaderAPIKey) != "community-secret" {
					t.Error("missing auth")
				}
				if op.body == "" {
					if body != "" {
						t.Errorf("unexpected body %q", body)
					}
				} else {
					assertJSONEqual(t, op.body, body)
					if req.Header.Get("Content-Type") != contentTypeJSON {
						t.Error("missing JSON content type")
					}
				}
				if op.name == "list" {
					if req.URL.Query().Get("cursor") != "a+/ b" || req.URL.Query().Get("limit") != "1" || strings.Contains(req.URL.RawQuery, " ") {
						t.Errorf("bad query %s", req.URL.RawQuery)
					}
				} else if req.URL.RawQuery != "" {
					t.Errorf("unexpected query %s", req.URL.RawQuery)
				}
			})
		}
	}
}
func TestPolicyFailuresAndCancellation(t *testing.T) {
	for _, group := range []string{"node-type-policies", "credential-type-policies"} {
		for _, op := range policyOperations() {
			t.Run(group+"/"+op.name, func(t *testing.T) {
				for _, status := range []int{400, 401, 403, 404, 409, 415, 422, 503} {
					server := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"refused"}`))
					p := policyTestAPIs(server.client(t))[group]
					if err := op.call(context.Background(), p); StatusCodeOf(err) != status {
						t.Errorf("status %d: %v", status, err)
					}
				}
				server := newCommunityPackageServer(t, packageJSONHandler(op.status, `{`))
				p := policyTestAPIs(server.client(t))[group]
				if op.method != "DELETE" {
					if err := op.call(context.Background(), p); err == nil {
						t.Error("accepted malformed JSON")
					}
				}
				before := len(server.requests)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if err := op.call(ctx, p); !errors.Is(err, context.Canceled) {
					t.Errorf("cancellation: %v", err)
				}
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()
				if err := op.call(ctx, p); !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("deadline: %v", err)
				}
				if len(server.requests) != before {
					t.Error("canceled request reached server")
				}
			})
		}
	}
	// Successful creation may have an empty body; deletion always does.
	server := newCommunityPackageServer(t, packageJSONHandler(201, ""))
	if _, err := server.client(t).NodeTypePolicies().CreateDocument(context.Background(), CreatePolicyDocumentRequest{}); err != nil {
		t.Fatal(err)
	}
}
func TestPolicyLocalValidation(t *testing.T) {
	rule := NodeTypePolicyRule{ID: "r", Action: NodeTypePolicyAllow, Selector: NodeTypePolicySelector{Kind: NodeTypePolicySelectorName, Value: "x"}}
	cases := []struct {
		name string
		call func(PolicyClient) error
	}{
		{"empty policy", func(p PolicyClient) error { _, e := p.GetDocument(context.Background(), ""); return e }},
		{"empty scope", func(p PolicyClient) error { _, e := p.GetAttachments(context.Background(), " "); return e }},
		{"empty project", func(p PolicyClient) error { _, e := p.GetProject(context.Background(), ""); return e }},
		{"missing version", func(p PolicyClient) error {
			_, e := p.UpdateDocument(context.Background(), "p", UpdatePolicyDocumentRequest{})
			return e
		}},
		{"negative version", func(p PolicyClient) error {
			_, e := p.UpdateDocument(context.Background(), "p", UpdatePolicyDocumentRequest{Version: version(-1)})
			return e
		}},
		{"limit", func(p PolicyClient) error {
			_, e := p.ListDocuments(context.Background(), ListOptions{Limit: 251})
			return e
		}},
		{"negative limit", func(p PolicyClient) error {
			_, e := p.ListDocuments(context.Background(), ListOptions{Limit: -1})
			return e
		}},
		{"duplicate rule", func(p PolicyClient) error {
			_, e := p.CreateDocument(context.Background(), CreatePolicyDocumentRequest{Rules: []NodeTypePolicyRule{rule, rule}})
			return e
		}},
		{"invalid rule action", func(p PolicyClient) error {
			r := rule
			r.Action = "bad"
			return p.ValidateRules([]NodeTypePolicyRule{r}, NodeTypePolicyScopeInstance)
		}},
		{"empty rule id", func(p PolicyClient) error {
			r := rule
			r.ID = ""
			return p.ValidateRules([]NodeTypePolicyRule{r}, NodeTypePolicyScopeInstance)
		}},
		{"empty selector value", func(p PolicyClient) error {
			r := rule
			r.Selector.Value = ""
			return p.ValidateRules([]NodeTypePolicyRule{r}, NodeTypePolicyScopeInstance)
		}},
		{"bad selector", func(p PolicyClient) error {
			r := rule
			r.Selector.Kind = "unknown"
			return p.ValidateRules([]NodeTypePolicyRule{r}, NodeTypePolicyScopeInstance)
		}},
		{"project delegate", func(p PolicyClient) error {
			r := rule
			r.Action = NodeTypePolicyDelegate
			_, e := p.ReplaceProject(context.Background(), "p", ReplaceNodeTypePolicyRequest{Rules: []NodeTypePolicyRule{r}, Version: version(0), DefaultAction: action(NodeTypePolicyAllow)})
			return e
		}},
	}
	for _, group := range []string{"node-type-policies", "credential-type-policies"} {
		for _, tt := range cases {
			t.Run(group+"/"+tt.name, func(t *testing.T) {
				s := newCommunityPackageServer(t, packageJSONHandler(200, `{}`))
				if err := tt.call(policyTestAPIs(s.client(t))[group]); err == nil {
					t.Error("expected local error")
				}
				if len(s.requests) != 0 {
					t.Error("validation reached transport")
				}
			})
		}
	}
	rule.Selector.Kind = NodeTypePolicySelectorExtends
	s := newCommunityPackageServer(t, packageJSONHandler(201, policyDocumentResultJSON))
	if _, err := s.client(t).NodeTypePolicies().CreateDocument(context.Background(), CreatePolicyDocumentRequest{Rules: []NodeTypePolicyRule{rule}}); err == nil {
		t.Error("node policy accepted extends")
	}
	if _, err := s.client(t).CredentialTypePolicies().CreateDocument(context.Background(), CreatePolicyDocumentRequest{Rules: []NodeTypePolicyRule{rule}}); err != nil {
		t.Fatal(err)
	}
}
func TestPolicyAttachmentValidation(t *testing.T) {
	a := PolicyAttachmentInput{PolicyID: "p", Priority: version(0), IsFloor: policyBool(false)}
	for _, tt := range []struct {
		name string
		r    ReplacePolicyAttachmentsRequest
	}{
		{"no version", ReplacePolicyAttachmentsRequest{}},
		{"negative version", ReplacePolicyAttachmentsRequest{Version: version(-1)}},
		{"empty ID", ReplacePolicyAttachmentsRequest{Version: version(1), Attachments: []PolicyAttachmentInput{{Priority: version(0), IsFloor: policyBool(false)}}}},
		{"missing priority", ReplacePolicyAttachmentsRequest{Version: version(1), Attachments: []PolicyAttachmentInput{{PolicyID: "p", IsFloor: policyBool(false)}}}},
		{"missing floor", ReplacePolicyAttachmentsRequest{Version: version(1), Attachments: []PolicyAttachmentInput{{PolicyID: "p", Priority: version(0)}}}},
		{"duplicate ID", ReplacePolicyAttachmentsRequest{Version: version(1), Attachments: []PolicyAttachmentInput{a, {PolicyID: "p", Priority: version(1), IsFloor: policyBool(false)}}}},
		{"duplicate pair", ReplacePolicyAttachmentsRequest{Version: version(1), Attachments: []PolicyAttachmentInput{a, {PolicyID: "other", Priority: version(0), IsFloor: policyBool(false)}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newCommunityPackageServer(t, packageJSONHandler(200, policyAttachmentsJSON))
			if _, err := s.client(t).NodeTypePolicies().ReplaceAttachments(context.Background(), "s", tt.r); err == nil {
				t.Error("expected local error")
			}
			if len(s.requests) != 0 {
				t.Error("request sent")
			}
		})
	}
	r := ReplacePolicyAttachmentsRequest{Version: version(1), Attachments: []PolicyAttachmentInput{a, {PolicyID: "other", Priority: version(0), IsFloor: policyBool(true)}}}
	if err := r.Validate(); err != nil {
		t.Errorf("same priority in separate floor groups: %v", err)
	}
}
func TestPolicyDocumentPagination(t *testing.T) {
	for _, group := range []string{"node-type-policies", "credential-type-policies"} {
		t.Run(group, func(t *testing.T) {
			calls := 0
			s := newCommunityPackageServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				calls++
				if r.URL.Query().Get("limit") != "1" {
					t.Error("lost page limit")
				}
				cursor := r.URL.Query().Get("cursor")
				next := ""
				if calls == 1 {
					if cursor != "start +" {
						t.Error("lost starting cursor")
					}
					next = "next +"
				} else if cursor != "next +" {
					t.Error("lost cursor")
				}
				_ = json.NewEncoder(w).Encode(Page[PolicyDocument]{Data: []PolicyDocument{{ID: "p"}}, NextCursor: next})
			})
			result, err := Collect(context.Background(), policyTestAPIs(s.client(t))[group].ListDocuments, ListOptions{Limit: 1, Cursor: "start +"}, 0)
			if err != nil || len(result) != 2 || calls != 2 {
				t.Fatalf("result %v calls %d err %v", result, calls, err)
			}
		})
	}
	s := newCommunityPackageServer(t, packageJSONHandler(200, `{"data":[`+policyDocumentJSON+`],"nextCursor":"bad"}`))
	p, err := s.client(t).NodeTypePolicies().ListDocuments(context.Background(), ListOptions{})
	if err != nil || p.NextCursor != "" {
		t.Errorf("short page should stop: %+v %v", p, err)
	}
}
