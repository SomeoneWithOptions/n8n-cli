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

// dependencyTypes are the relation types the API documents.
var dependencyTypes = map[string]bool{
	n8n.DependencyAgent: true, n8n.DependencyCredential: true, n8n.DependencyDataTable: true,
	n8n.DependencyErrorWorkflow: true, n8n.DependencyErrorWorkflowParent: true,
	n8n.DependencyParentWorkflow: true, n8n.DependencySubWorkflow: true,
}

// checkDependencyPage validates one live page, skipping when the instance
// predates the dependency endpoints (a 404 for an ID that was just listed).
func checkDependencyPage(t *testing.T, what string, page n8n.DependencyPage, err error) {
	t.Helper()
	if n8n.IsNotFound(err) {
		t.Skipf("%s dependencies: instance answered 404 for a listed ID; it does not serve dependency lookups: %v", what, err)
	}
	if err != nil {
		t.Fatalf("%s dependencies: %v", what, err)
	}
	if page.Data == nil || page.InaccessibleCount < 0 {
		t.Errorf("%s dependencies: page = %+v", what, page)
	}
	for _, dep := range page.Data {
		if dep.ID == "" || !dependencyTypes[dep.Type] {
			t.Errorf("%s dependency has no ID or an undocumented type: %+v", what, dep)
		}
	}
	t.Logf("%s has %d listed and %d inaccessible dependency(ies)", what, len(page.Data), page.InaccessibleCount)
}

// TestDependencyLookups is read-only: it looks up the dependencies of the first
// existing workflow, credential and data table, skipping any kind the instance
// has none of.
func TestDependencyLookups(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()

	t.Run("workflow", func(t *testing.T) {
		list, err := client.ListWorkflows(ctx, n8n.ListWorkflowsOptions{ListOptions: n8n.ListOptions{Limit: 1}, ExcludePinnedData: true})
		if err != nil {
			t.Fatalf("ListWorkflows: %v", err)
		}
		if len(list.Data) == 0 {
			t.Skip("instance has no workflow to look up")
		}
		page, err := client.WorkflowDependencies(ctx, list.Data[0].ID, n8n.ListOptions{Limit: 10})
		checkDependencyPage(t, "workflow", page, err)
	})
	t.Run("credential", func(t *testing.T) {
		list, err := client.ListCredentials(ctx, n8n.ListOptions{Limit: 1})
		if err != nil {
			t.Skipf("ListCredentials needs an owner or admin key: %v", err)
		}
		if len(list.Data) == 0 {
			t.Skip("instance has no credential to look up")
		}
		page, err := client.CredentialDependencies(ctx, list.Data[0].ID, n8n.ListOptions{Limit: 10})
		checkDependencyPage(t, "credential", page, err)
	})
	t.Run("data table", func(t *testing.T) {
		list, err := client.ListDataTables(ctx, n8n.ListDataTablesOptions{ListOptions: n8n.ListOptions{Limit: 1}})
		if err != nil {
			t.Skipf("ListDataTables: %v", err)
		}
		if len(list.Data) == 0 {
			t.Skip("instance has no data table to look up")
		}
		page, err := client.DataTableDependencies(ctx, list.Data[0].ID, n8n.ListOptions{Limit: 10})
		checkDependencyPage(t, "data table", page, err)
	})
}

// TestWorkflowDependenciesCommand checks the JSON page through the full CLI.
func TestWorkflowDependenciesCommand(t *testing.T) {
	instance := integration.Require(t)
	list, err := instance.Client(t).ListWorkflows(context.Background(), n8n.ListWorkflowsOptions{ListOptions: n8n.ListOptions{Limit: 1}, ExcludePinnedData: true})
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(list.Data) == 0 {
		t.Skip("instance has no workflow to look up")
	}
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"workflow", "dependencies", list.Data[0].ID, "--all", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess && strings.Contains(errOut.String(), "404") {
		t.Skipf("instance does not serve dependency lookups: %s", errOut.String())
	}
	if code != cli.ExitSuccess {
		t.Fatalf("workflow dependencies exit = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page struct {
		Data              []n8n.Dependency `json:"data"`
		InaccessibleCount *int             `json:"inaccessibleCount"`
		Collection        *struct {
			Truncated bool `json:"truncated"`
		} `json:"collection"`
	}
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not dependency page JSON: %v\n%s", err, out.String())
	}
	if page.Data == nil || page.InaccessibleCount == nil || page.Collection == nil {
		t.Errorf("JSON lacks data, inaccessibleCount or collection: %s", out.String())
	}
	if strings.Contains(out.String()+errOut.String(), instance.APIKey.Reveal()) {
		t.Error("API key leaked into output")
	}
}
