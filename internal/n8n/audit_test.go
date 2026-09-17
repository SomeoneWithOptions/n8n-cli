package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// auditBody is the shape the instance returns: one key per report, no envelope.
const auditBody = `{
  "Nodes Risk Report": {
    "risk": "nodes",
    "sections": [
      {
        "title": "Community nodes",
        "description": "This node is sourced from the community.",
        "recommendation": "Consider reviewing the source code in any community nodes.",
        "location": [
          {"kind": "community", "nodeType": "n8n-nodes-test.test", "packageUrl": "https://www.npmjs.com/package/n8n-nodes-test"}
        ]
      }
    ]
  },
  "Credentials Risk Report": {
    "risk": "credentials",
    "sections": [
      {
        "title": "Credentials not used in any workflow",
        "description": "These credentials are not used in any workflow.",
        "recommendation": "Consider deleting these credentials.",
        "location": [{"kind": "credential", "id": "1", "name": "My Test Account"}]
      },
      {
        "title": "Credentials not used in recently executed workflows",
        "description": "These credentials are used in workflows that have not executed recently.",
        "recommendation": "Consider deleting these credentials.",
        "location": [
          {"kind": "node", "workflowId": "9", "workflowName": "My Workflow", "nodeId": "51eb5852", "nodeName": "MySQL", "nodeType": "n8n-nodes-base.mySql"}
        ]
      }
    ]
  }
}`

// auditServer records the requests it receives, including their bodies, and
// answers every one of them with status and body.
type auditServer struct {
	*httptest.Server
	requests []*http.Request
	bodies   []string
}

func newAuditServer(t *testing.T, status int, body string) *auditServer {
	t.Helper()
	s := &auditServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		s.requests = append(s.requests, r.Clone(r.Context()))
		s.bodies = append(s.bodies, string(raw))

		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *auditServer) last(t *testing.T) (*http.Request, string) {
	t.Helper()
	if len(s.requests) == 0 {
		t.Fatal("no request reached the instance")
	}
	return s.requests[len(s.requests)-1], s.bodies[len(s.bodies)-1]
}

func (s *auditServer) client(t *testing.T) *Client {
	t.Helper()
	auth, err := NewAPIKeyAuth("secret-key")
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}
	client, err := New(s.URL, WithAuth(auth))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func TestGenerateAudit(t *testing.T) {
	server := newAuditServer(t, http.StatusOK, auditBody)

	audit, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{})
	if err != nil {
		t.Fatalf("GenerateAudit: %v", err)
	}

	got, body := server.last(t)
	if got.Method != http.MethodPost {
		t.Errorf("method = %q, want %q", got.Method, http.MethodPost)
	}
	if want := BasePath + AuditPath; got.URL.Path != want {
		t.Errorf("path = %q, want %q", got.URL.Path, want)
	}
	if got.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty: this operation takes no query parameters", got.URL.RawQuery)
	}
	if key := got.Header.Get(HeaderAPIKey); key != "secret-key" {
		t.Errorf("%s = %q, want the API key", HeaderAPIKey, key)
	}
	// The request body is optional in the specification; the zero options send
	// none rather than an empty object.
	if body != "" {
		t.Errorf("body = %q, want no body for the zero options", body)
	}
	if ct := got.Header.Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q, want none when there is no body", ct)
	}

	want := map[string]RiskReport{
		"Nodes Risk Report": {
			Risk: "nodes",
			Sections: []RiskSection{{
				Title:          "Community nodes",
				Description:    "This node is sourced from the community.",
				Recommendation: "Consider reviewing the source code in any community nodes.",
				Location: []RiskLocation{
					{Kind: "community", NodeType: "n8n-nodes-test.test", PackageURL: "https://www.npmjs.com/package/n8n-nodes-test"},
				},
			}},
		},
		"Credentials Risk Report": {
			Risk: "credentials",
			Sections: []RiskSection{
				{
					Title:          "Credentials not used in any workflow",
					Description:    "These credentials are not used in any workflow.",
					Recommendation: "Consider deleting these credentials.",
					Location:       []RiskLocation{{Kind: "credential", ID: "1", Name: "My Test Account"}},
				},
				{
					Title:          "Credentials not used in recently executed workflows",
					Description:    "These credentials are used in workflows that have not executed recently.",
					Recommendation: "Consider deleting these credentials.",
					Location: []RiskLocation{
						{Kind: "node", WorkflowID: "9", WorkflowName: "My Workflow", NodeID: "51eb5852", NodeName: "MySQL", NodeType: "n8n-nodes-base.mySql"},
					},
				},
			},
		},
	}
	if diff := cmp.Diff(want, audit.Reports); diff != "" {
		t.Errorf("GenerateAudit() reports mismatch (-want +got):\n%s", diff)
	}
	if audit.Findings() != 3 {
		t.Errorf("Findings() = %d, want 3", audit.Findings())
	}
}

func TestGenerateAuditRequestBody(t *testing.T) {
	tests := []struct {
		name string
		opts AuditOptions
		want any
	}{
		{name: "no options", opts: AuditOptions{}, want: nil},
		{
			name: "days only",
			opts: AuditOptions{DaysAbandonedWorkflow: 30},
			want: map[string]any{"additionalOptions": map[string]any{"daysAbandonedWorkflow": float64(30)}},
		},
		{
			name: "one category",
			opts: AuditOptions{Categories: []string{AuditCategoryCredentials}},
			want: map[string]any{"additionalOptions": map[string]any{"categories": []any{"credentials"}}},
		},
		{
			name: "every category and days",
			opts: AuditOptions{DaysAbandonedWorkflow: 7, Categories: AuditCategories},
			want: map[string]any{"additionalOptions": map[string]any{
				"daysAbandonedWorkflow": float64(7),
				"categories":            []any{"credentials", "database", "nodes", "filesystem", "instance"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAuditServer(t, http.StatusOK, auditBody)

			if _, err := server.client(t).GenerateAudit(context.Background(), tt.opts); err != nil {
				t.Fatalf("GenerateAudit: %v", err)
			}

			req, body := server.last(t)
			if tt.want == nil {
				if body != "" {
					t.Fatalf("body = %q, want none", body)
				}
				return
			}
			if ct := req.Header.Get("Content-Type"); ct != contentTypeJSON {
				t.Errorf("Content-Type = %q, want %q", ct, contentTypeJSON)
			}
			var decoded any
			if err := json.Unmarshal([]byte(body), &decoded); err != nil {
				t.Fatalf("request body is not JSON: %v\n%s", err, body)
			}
			if diff := cmp.Diff(tt.want, decoded); diff != "" {
				t.Errorf("request body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestGenerateAuditEveryCategory covers each documented category on its own, so
// a category the API renames cannot hide behind the combined case.
func TestGenerateAuditEveryCategory(t *testing.T) {
	for _, category := range AuditCategories {
		t.Run(category, func(t *testing.T) {
			server := newAuditServer(t, http.StatusOK, auditBody)

			if _, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{Categories: []string{category}}); err != nil {
				t.Fatalf("GenerateAudit(%s): %v", category, err)
			}

			_, body := server.last(t)
			var decoded auditRequest
			if err := json.Unmarshal([]byte(body), &decoded); err != nil {
				t.Fatalf("request body is not JSON: %v\n%s", err, body)
			}
			if diff := cmp.Diff([]string{category}, decoded.AdditionalOptions.Categories); diff != "" {
				t.Errorf("categories mismatch (-want +got):\n%s", diff)
			}
			if decoded.AdditionalOptions.DaysAbandonedWorkflow != 0 {
				t.Errorf("daysAbandonedWorkflow = %d, want it omitted", decoded.AdditionalOptions.DaysAbandonedWorkflow)
			}
		})
	}
}

// TestGenerateAuditEmptyReport covers the instance's real answer when nothing
// is found: an empty JSON array, not an empty object.
func TestGenerateAuditEmptyReport(t *testing.T) {
	for _, body := range []string{`[]`, `{}`, `null`, ``} {
		t.Run("body "+body, func(t *testing.T) {
			server := newAuditServer(t, http.StatusOK, body)

			audit, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{})
			if err != nil {
				t.Fatalf("GenerateAudit: %v", err)
			}
			if len(audit.Reports) != 0 {
				t.Errorf("Reports = %v, want none", audit.Reports)
			}
			if audit.Findings() != 0 {
				t.Errorf("Findings() = %d, want 0", audit.Findings())
			}
			if names := audit.ReportNames(); len(names) != 0 {
				t.Errorf("ReportNames() = %v, want none", names)
			}
		})
	}
}

// TestGenerateAuditRejectsPopulatedArray guards the tolerance above: an empty
// array means "nothing found", but a populated one is a shape we do not model
// and must not be silently dropped.
func TestGenerateAuditRejectsPopulatedArray(t *testing.T) {
	server := newAuditServer(t, http.StatusOK, `[{"risk":"nodes"}]`)

	_, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{})
	if err == nil {
		t.Fatal("GenerateAudit() = nil error, want a decode failure")
	}
	if !strings.Contains(err.Error(), "array") {
		t.Errorf("error = %v, want it to name the unexpected array", err)
	}
}

func TestGenerateAuditPreservesRawDocument(t *testing.T) {
	// A field the models do not cover must survive to --output json.
	const body = `{"Instance Risk Report":{"risk":"instance","sections":[{"title":"Outdated instance","nextVersions":[{"name":"1.2.3"}]}]}}`
	server := newAuditServer(t, http.StatusOK, body)

	audit, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{})
	if err != nil {
		t.Fatalf("GenerateAudit: %v", err)
	}

	encoded, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal re-encoded audit: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("re-encoded audit mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerateAuditErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`, check: IsUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"missing scope"}`, check: IsForbidden},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
			body:   `{"message":"boom"}`,
			check:  func(err error) bool { return IsStatus(err, http.StatusInternalServerError) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAuditServer(t, tt.status, tt.body)

			audit, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{})
			if err == nil {
				t.Fatalf("GenerateAudit() = %+v, want an error", audit)
			}
			if !tt.check(err) {
				t.Errorf("error = %v, want status %d", err, tt.status)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want an *APIError", err)
			}
			if apiErr.Method != http.MethodPost || apiErr.Path != AuditPath {
				t.Errorf("error method and path = %s %s, want POST %s", apiErr.Method, apiErr.Path, AuditPath)
			}
		})
	}
}

func TestAuditOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    AuditOptions
		wantErr string
	}{
		{name: "zero", opts: AuditOptions{}},
		{name: "every category", opts: AuditOptions{Categories: AuditCategories, DaysAbandonedWorkflow: 1}},
		{name: "unknown category", opts: AuditOptions{Categories: []string{"workflows"}}, wantErr: `unknown audit category "workflows"`},
		{name: "wrong case", opts: AuditOptions{Categories: []string{"Credentials"}}, wantErr: "unknown audit category"},
		{name: "negative days", opts: AuditOptions{DaysAbandonedWorkflow: -1}, wantErr: "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestGenerateAuditValidatesBeforeSending keeps a bad category from costing a
// round trip.
func TestGenerateAuditValidatesBeforeSending(t *testing.T) {
	server := newAuditServer(t, http.StatusOK, auditBody)

	if _, err := server.client(t).GenerateAudit(context.Background(), AuditOptions{Categories: []string{"nope"}}); err == nil {
		t.Fatal("GenerateAudit() = nil error, want a validation failure")
	}
	if len(server.requests) != 0 {
		t.Errorf("%d requests reached the instance, want none", len(server.requests))
	}
}

func TestAuditReportLookup(t *testing.T) {
	var audit Audit
	if err := json.Unmarshal([]byte(auditBody), &audit); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	want := []string{"Credentials Risk Report", "Nodes Risk Report"}
	if diff := cmp.Diff(want, audit.ReportNames()); diff != "" {
		t.Errorf("ReportNames() mismatch (-want +got):\n%s", diff)
	}

	// By the report's own risk field.
	report, ok := audit.Report("nodes")
	if !ok {
		t.Fatalf("Report(%q) = not found, want the nodes report", "nodes")
	}
	if len(report.Sections) != 1 || report.Locations() != 1 {
		t.Errorf("nodes report = %+v, want one section with one location", report)
	}

	// By the report name, for an instance whose risk field disagrees.
	renamed := Audit{Reports: map[string]RiskReport{
		"Instance Risk Report": {Risk: "execution", Sections: []RiskSection{{Title: "Unprotected webhooks"}}},
	}}
	if _, ok := renamed.Report("instance"); !ok {
		t.Error(`Report("instance") = not found, want a match on the report name`)
	}
	if _, ok := renamed.Report("execution"); !ok {
		t.Error(`Report("execution") = not found, want a match on the risk field`)
	}
	if _, ok := audit.Report("filesystem"); ok {
		t.Error(`Report("filesystem") = found, want a miss for a report the audit did not return`)
	}
}
