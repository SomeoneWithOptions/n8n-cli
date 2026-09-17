package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// auditPayload is what the instance returns: one key per report, no envelope.
const auditPayload = `{
  "Nodes Risk Report": {
    "risk": "nodes",
    "sections": [
      {
        "title": "Community nodes",
        "description": "This node is sourced from the community and is not vetted by n8n.",
        "recommendation": "Consider reviewing the source code in any community nodes\ninstalled in this instance.",
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
        "location": [{"kind": "credential", "id": "17", "name": "My Test Account"}]
      }
    ]
  }
}`

// auditFixture is a logged-in machine whose instance answers audit calls.
func auditFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestAuditGenerateText(t *testing.T) {
	f := auditFixture(t, auditPayload)

	got := f.run("audit", "generate")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{
		f.server.URL,
		"Context:",
		DefaultContextName,
		"Reports:",
		"2",
		"Findings:",
		"RISK",
		"REPORT",
		"FINDINGS",
		"LOCATIONS",
		"nodes",
		"Nodes Risk Report",
		"credentials",
		"Credentials Risk Report",
		"Community nodes",
		"Recommendation: Consider reviewing the source code in any community nodes installed in this instance.",
		"community n8n-nodes-test.test",
		"credential My Test Account",
		"id 17",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stderr, "--output json") {
		t.Errorf("stderr = %q, want a pointer to the full report", got.stderr)
	}

	req := f.lastRequest()
	if req.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", req.Method)
	}
	if want := n8n.BasePath + n8n.AuditPath; req.URL.Path != want {
		t.Errorf("path = %q, want %q", req.URL.Path, want)
	}
	if body := f.lastBody(); body != "" {
		t.Errorf("body = %q, want none for an unfiltered audit", body)
	}
	if req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
		t.Error("the saved credential was not sent")
	}
}

func TestAuditGenerateJSON(t *testing.T) {
	f := auditFixture(t, auditPayload)

	got := f.run("audit", "generate", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want JSON output to be quiet", got.stderr)
	}

	var want, gotJSON any
	if err := json.Unmarshal([]byte(auditPayload), &want); err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(got.stdout), &gotJSON); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, got.stdout)
	}
	// JSON output is the instance's own document: the report keys are the API's
	// and nothing the CLI does not model is dropped.
	if diff := diffJSON(want, gotJSON); diff != "" {
		t.Errorf("stdout is not the instance report: %s\n--- stdout ---\n%s", diff, got.stdout)
	}
}

// diffJSON reports the first difference between two decoded JSON documents.
func diffJSON(want, got any) string {
	wantEncoded, _ := json.Marshal(want)
	gotEncoded, _ := json.Marshal(got)
	if string(wantEncoded) == string(gotEncoded) {
		return ""
	}
	return "want " + string(wantEncoded) + ", got " + string(gotEncoded)
}

func TestAuditGenerateRequestBody(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no filters", args: nil, want: ""},
		{
			name: "one category",
			args: []string{"--category", "credentials"},
			want: `{"additionalOptions":{"categories":["credentials"]}}`,
		},
		{
			name: "repeated category flag",
			args: []string{"--category", "credentials", "--category", "nodes"},
			want: `{"additionalOptions":{"categories":["credentials","nodes"]}}`,
		},
		{
			name: "comma separated categories",
			args: []string{"--category", "database,filesystem"},
			want: `{"additionalOptions":{"categories":["database","filesystem"]}}`,
		},
		{
			name: "days abandoned",
			args: []string{"--days-abandoned", "30"},
			want: `{"additionalOptions":{"daysAbandonedWorkflow":30}}`,
		},
		{
			name: "every documented category and days",
			args: []string{"--category", "credentials,database,nodes,filesystem,instance", "--days-abandoned", "7"},
			want: `{"additionalOptions":{"daysAbandonedWorkflow":7,"categories":["credentials","database","nodes","filesystem","instance"]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := auditFixture(t, auditPayload)

			got := f.run(append([]string{"audit", "generate"}, tt.args...)...)
			if got.code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
			}
			if body := f.lastBody(); body != tt.want {
				t.Errorf("body = %q, want %q", body, tt.want)
			}
		})
	}
}

// TestAuditGenerateEmptyReport covers the instance's real answer for a clean
// audit: an empty JSON array. Nothing found is a result, not a failure.
func TestAuditGenerateEmptyReport(t *testing.T) {
	for _, body := range []string{`[]`, `{}`} {
		t.Run("body "+body, func(t *testing.T) {
			f := auditFixture(t, body)

			got := f.run("audit", "generate", "--category", "database")
			if got.code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
			}
			if !strings.Contains(got.stderr, "database") {
				t.Errorf("stderr = %q, want it to repeat the category that found nothing", got.stderr)
			}
			if strings.Contains(got.stdout, "RISK") {
				t.Errorf("stdout = %q, want no report table", got.stdout)
			}
			if !strings.Contains(got.stdout, "Findings:") {
				t.Errorf("stdout = %q, want the summary to still report zero findings", got.stdout)
			}
		})
	}
}

func TestAuditGenerateEmptyReportUnfiltered(t *testing.T) {
	f := auditFixture(t, `[]`)

	got := f.run("audit", "generate")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stderr, "No risks reported") {
		t.Errorf("stderr = %q, want it to say the audit found nothing", got.stderr)
	}
}

func TestAuditGenerateEchoesFiltersInSummary(t *testing.T) {
	f := auditFixture(t, auditPayload)

	got := f.run("audit", "generate", "--category", "nodes", "--days-abandoned", "45")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{"Categories:", "nodes", "Abandoned after:", "45 days"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
}

func TestAuditGenerateRejectsBadFlags(t *testing.T) {
	tests := map[string][]string{
		"unknown category": {"audit", "generate", "--category", "workflows"},
		"category case":    {"audit", "generate", "--category", "Credentials"},
		"negative days":    {"audit", "generate", "--days-abandoned", "-1"},
		"unknown output":   {"audit", "generate", "--output", "yaml"},
		"positional arg":   {"audit", "generate", "credentials"},
		"unknown flag":     {"audit", "generate", "--categories", "nodes"},
		"missing action":   {"audit", "scan"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			f := auditFixture(t, auditPayload)
			before := f.requestCount()

			got := f.run(args...)
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d (stdout: %s)", got.code, ExitError, got.stdout)
			}
			if got.stderr == "" {
				t.Error("stderr is empty, want an error message")
			}
			if f.requestCount() != before {
				t.Error("an invalid invocation still reached the instance")
			}
		})
	}
}

// TestAuditGenerateUnknownCategoryNamesTheOptions keeps the fix in the error,
// because agents act on stderr text.
func TestAuditGenerateUnknownCategoryNamesTheOptions(t *testing.T) {
	f := auditFixture(t, auditPayload)

	got := f.run("audit", "generate", "--category", "workflows")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	for _, want := range append([]string{`"workflows"`}, n8n.AuditCategories...) {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
		}
	}
}

func TestAuditGenerateAPIErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr []string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantErr: []string{"401", "auth login"}},
		{name: "forbidden", status: http.StatusForbidden, wantErr: []string{"403", "scope", "n8n discover --resource audit"}},
		{name: "server error", status: http.StatusInternalServerError, wantErr: []string{"500"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := auditFixture(t, auditPayload)
			f.status = tt.status

			got := f.run("audit", "generate")
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d", got.code, ExitError)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty on failure", got.stdout)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
				}
			}
		})
	}
}

func TestAuditGenerateRequiresCredential(t *testing.T) {
	f := newFixture(t)
	f.env[config.EnvURL] = f.server.URL

	got := f.run("audit", "generate")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	if !strings.Contains(got.stderr, "auth login") {
		t.Errorf("stderr = %q, want it to point at 'n8n auth login'", got.stderr)
	}
	if f.requestCount() != 0 {
		t.Error("a request was sent without a credential")
	}
}

func TestAuditGenerateUsesURLOverride(t *testing.T) {
	f := auditFixture(t, auditPayload)

	got := f.run("audit", "generate", "--url", f.server.URL)
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stdout, f.server.URL) {
		t.Errorf("stdout = %q, want the overridden instance", got.stdout)
	}
}

func TestDescribeLocation(t *testing.T) {
	tests := []struct {
		name     string
		location n8n.RiskLocation
		want     string
	}{
		{
			name:     "credential",
			location: n8n.RiskLocation{Kind: "credential", ID: "1", Name: "My Test Account"},
			want:     "credential My Test Account id 1",
		},
		{
			name: "node in a workflow",
			location: n8n.RiskLocation{
				Kind: "node", WorkflowID: "9", WorkflowName: "My Workflow",
				NodeID: "51eb", NodeName: "MySQL", NodeType: "n8n-nodes-base.mySql",
			},
			want: "node MySQL in workflow My Workflow (n8n-nodes-base.mySql) id 9",
		},
		{
			name:     "community package",
			location: n8n.RiskLocation{Kind: "community", NodeType: "n8n-nodes-test.test", PackageURL: "https://npm.example/test"},
			want:     "community n8n-nodes-test.test https://npm.example/test",
		},
		{name: "kind only", location: n8n.RiskLocation{Kind: "instance"}, want: "instance"},
		{name: "empty", location: n8n.RiskLocation{}, want: "(unidentified location)"},
		{name: "no kind", location: n8n.RiskLocation{Name: "Orphan"}, want: "Orphan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeLocation(tt.location); got != tt.want {
				t.Errorf("describeLocation() = %q, want %q", got, tt.want)
			}
		})
	}
}
