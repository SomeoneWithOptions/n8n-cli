package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestGenerateAudit exercises POST /audit against the live instance. It is a
// POST but not a mutation: the report is derived from workflows, credentials
// and settings that already exist, and nothing is created or changed, so it is
// safe on the production instance these tests target.
func TestGenerateAudit(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	audit, err := client.GenerateAudit(context.Background(), n8n.AuditOptions{})
	if err != nil {
		t.Fatalf("GenerateAudit: %v", err)
	}
	if len(audit.Raw) == 0 {
		t.Fatal("the instance sent an empty body")
	}
	// A clean instance legitimately reports nothing, so the shape is what is
	// under test, not the finding count.
	for _, name := range audit.ReportNames() {
		report := audit.Reports[name]
		if report.Risk == "" {
			t.Errorf("report %q has no risk field", name)
		}
		if len(report.Sections) == 0 {
			t.Errorf("report %q is present but lists no sections", name)
		}
		for _, section := range report.Sections {
			if section.Title == "" {
				t.Errorf("report %q has a section with no title: %+v", name, section)
			}
		}
	}
	t.Logf("audit reported %d findings across %v", audit.Findings(), audit.ReportNames())
}

// TestGenerateAuditCategories runs every documented category on its own, which
// is what the phase exit gate asks for: a category the API stops accepting
// fails here rather than silently returning an unfiltered report.
func TestGenerateAuditCategories(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	for _, category := range n8n.AuditCategories {
		t.Run(category, func(t *testing.T) {
			audit, err := client.GenerateAudit(context.Background(), n8n.AuditOptions{Categories: []string{category}})
			if err != nil {
				t.Fatalf("GenerateAudit(%s): %v", category, err)
			}
			for _, name := range audit.ReportNames() {
				if _, ok := audit.Report(category); !ok {
					t.Errorf("audit filtered to %q returned %q, which does not match the filter", category, name)
				}
			}
		})
	}
}

// TestGenerateAuditRejectsUnknownCategory pins the server-side enum the client
// validates against: if the API gains or renames a category, this stops
// failing and the client's list is out of date.
func TestGenerateAuditRejectsUnknownCategory(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	// Bypass client-side validation by sending the request the API's way.
	var out n8n.Audit
	_, err := client.Do(context.Background(), n8n.Request{
		Method: "POST",
		Path:   n8n.AuditPath,
		Body:   map[string]any{"additionalOptions": map[string]any{"categories": []string{"workflows"}}},
	}, &out)
	if err == nil {
		t.Fatal("the instance accepted an undocumented audit category")
	}
	if !n8n.IsStatus(err, 400) {
		t.Fatalf("GenerateAudit with a bad category = %v, want 400", err)
	}
}

func TestGenerateAuditDaysAbandoned(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	audit, err := client.GenerateAudit(context.Background(), n8n.AuditOptions{
		DaysAbandonedWorkflow: 1,
		Categories:            []string{n8n.AuditCategoryCredentials, n8n.AuditCategoryInstance},
	})
	if err != nil {
		t.Fatalf("GenerateAudit(daysAbandonedWorkflow=1): %v", err)
	}
	if audit.Raw == nil {
		t.Error("the instance sent an empty body")
	}
}

// TestAuditCommand runs the CLI itself against the instance, with the
// credential supplied through the environment and a throwaway config
// directory, so nothing on the developer's machine is read or written.
func TestAuditCommand(t *testing.T) {
	instance := integration.Require(t)

	env := map[string]string{
		config.EnvURL:    instance.URL,
		config.EnvAPIKey: instance.APIKey.Reveal(),
	}
	var out, errOut strings.Builder
	interactive := false
	code := cli.Run(context.Background(), []string{"audit", "generate", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("n8n audit generate exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}

	var report any
	if err := json.Unmarshal([]byte(out.String()), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("the credential leaked into stdout")
	}
}
