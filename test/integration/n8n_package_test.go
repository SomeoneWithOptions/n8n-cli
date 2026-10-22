package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestN8nPackageExport is read-only: it exports one existing workflow to
// memory and through the CLI. Exporting reads instance content and writes
// nothing server-side.
func TestN8nPackageExport(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()

	page, err := client.ListWorkflows(ctx, n8n.ListWorkflowsOptions{ListOptions: n8n.ListOptions{Limit: 1}})
	if err != nil {
		if n8n.IsForbidden(err) || n8n.IsNotFound(err) {
			t.Skip("integration instance does not serve workflows")
		}
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(page.Data) == 0 {
		t.Skip("integration instance has no workflow to export")
	}
	id := page.Data[0].ID

	var archive bytes.Buffer
	result, err := client.ExportPackage(ctx, n8n.ExportPackageRequest{WorkflowIDs: []string{id}}, &archive)
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) {
		t.Skip("integration instance has no licensed Packages feature")
	}
	if err != nil {
		t.Fatalf("ExportPackage: %v", err)
	}
	if archive.Len() == 0 || int64(archive.Len()) != result.Bytes {
		t.Errorf("archive = %d bytes, result reports %d", archive.Len(), result.Bytes)
	}
	if magic := archive.Bytes()[:2]; magic[0] != 0x1f || magic[1] != 0x8b {
		t.Errorf("archive magic = %x, want gzip (1f 8b)", magic)
	}
	if result.Counts["workflows"] < 1 {
		t.Errorf("counts = %v, want at least one workflow", result.Counts)
	}

	out := filepath.Join(t.TempDir(), "export.n8np")
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var stdout, stderr strings.Builder
	code := cli.Run(ctx, []string{"package", "export", "--workflow-id", id, "--out", out, "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(stderr.String(), "403") || strings.Contains(stderr.String(), "404") {
			t.Skip("integration instance has no licensed Packages feature")
		}
		t.Fatalf("package export exit code = %d (stderr: %s)", code, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read exported archive: %v", err)
	}
	if len(data) == 0 || data[0] != 0x1f || data[1] != 0x8b {
		t.Errorf("exported file is %d bytes, want a non-empty gzip archive", len(data))
	}
	var summary struct {
		File   string `json:"file"`
		Result *struct {
			Bytes  int64          `json:"bytes"`
			Counts map[string]int `json:"counts"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &summary); err != nil {
		t.Fatalf("stdout is not an export summary: %v\n%s", err, stdout.String())
	}
	if summary.File != out || summary.Result == nil || summary.Result.Bytes != int64(len(data)) {
		t.Errorf("summary = %s, want the file and byte size", stdout.String())
	}
	if strings.Contains(stdout.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}

// TestN8nPackageImportValidation posts a malformed archive that the server
// must reject before writing anything, so the shared instance is unchanged.
// A successful import is never exercised here: it would create workflows on a
// production instance. The success path is covered by client and CLI tests.
func TestN8nPackageImportValidation(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	_, err := client.ImportPackage(context.Background(), n8n.ImportPackageRequest{
		WorkflowConflictPolicy: n8n.ImportWorkflowConflictSkip,
		WorkflowIDPolicy:       n8n.ImportWorkflowIDSource,
	}, strings.NewReader("not-a-gzip-package"), "invalid.n8np")
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) {
		t.Skip("integration instance has no licensed Packages feature")
	}
	if err == nil {
		t.Fatal("ImportPackage(invalid archive): want a rejection, got success")
	}
	if !n8n.IsStatus(err, http.StatusBadRequest) && !n8n.IsImportUnprocessable(err) {
		t.Errorf("error = %v, want a 400 or 422 validation rejection", err)
	}
	t.Logf("invalid archive rejected as expected: %v", err)
}

// TestN8nPackageImportSelectionValidation posts a malformed archive to the
// selection endpoint, which must reject it before writing anything.
func TestN8nPackageImportSelectionValidation(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	_, err := client.ImportPackageSelection(context.Background(), n8n.ImportPackageSelectionRequest{
		ProjectID:        integration.ResourcePrefix + "missing-project",
		WorkflowIDs:      []string{integration.ResourcePrefix + "missing-workflow"},
		WorkflowIDPolicy: n8n.ImportWorkflowIDSource,
	}, strings.NewReader("not-a-gzip-package"), "invalid.n8np")
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skip("integration instance does not serve package import-selection")
	}
	if err == nil {
		t.Fatal("ImportPackageSelection(invalid archive): want a rejection, got success")
	}
	if !n8n.IsStatus(err, http.StatusBadRequest) && !n8n.IsImportUnprocessable(err) {
		t.Errorf("error = %v, want a 400 or 422 validation rejection", err)
	}
	t.Logf("invalid archive rejected as expected: %v", err)
}

// TestN8nPackageImportSelectionRoundTrip creates a prefixed project with two
// workflows, exports it as a project package, and imports one workflow back
// while archiving the other. It only touches what it created, and cleanup is
// registered before the first assertion.
func TestN8nPackageImportSelectionRoundTrip(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	name := fmt.Sprintf("%spackage-selection-%d", integration.ResourcePrefix, stamp)

	project, err := client.CreateProject(ctx, n8n.CreateProjectRequest{Name: name})
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) {
		t.Skipf("integration instance denies project creation: %v", err)
	}
	if err != nil {
		t.Fatalf("CreateProject(%q): %v", name, err)
	}
	projectID := project.ID
	if projectID == "" {
		projects, err := n8n.Collect(ctx, client.ListProjects, n8n.ListOptions{Limit: 100}, 0)
		if err != nil {
			t.Fatalf("ListProjects after create: %v", err)
		}
		for _, candidate := range projects {
			if candidate.Name == name {
				projectID = candidate.ID
			}
		}
	}
	if projectID == "" {
		t.Fatalf("created project %q was not found in the project list", name)
	}
	t.Cleanup(func() {
		if err := client.DeleteProject(context.Background(), projectID); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteProject(%q): %v", projectID, err)
		}
	})

	createInProject := func(suffix string) string {
		t.Helper()
		workflowName := fmt.Sprintf("%s-%s", name, suffix)
		created, err := client.CreateWorkflow(ctx, workflowDefinition(workflowName, fmt.Sprintf("%s%d-%s", integration.ResourcePrefix, stamp, suffix)))
		if err != nil {
			t.Fatalf("CreateWorkflow(%q): %v", workflowName, err)
		}
		t.Cleanup(func() {
			if _, err := client.DeleteWorkflow(context.Background(), created.ID); err != nil && !n8n.IsNotFound(err) {
				t.Errorf("cleanup DeleteWorkflow(%q): %v", created.ID, err)
			}
		})
		if err := client.TransferWorkflow(ctx, created.ID, projectID); err != nil {
			t.Fatalf("TransferWorkflow(%q, %q): %v", created.ID, projectID, err)
		}
		return created.ID
	}
	kept := createInProject("kept")
	removed := createInProject("removed")

	var archive bytes.Buffer
	if _, err := client.ExportPackage(ctx, n8n.ExportPackageRequest{ProjectIDs: []string{projectID}}, &archive); err != nil {
		if n8n.IsForbidden(err) || n8n.IsNotFound(err) {
			t.Skip("integration instance has no licensed Packages feature")
		}
		t.Fatalf("ExportPackage(project %q): %v", projectID, err)
	}

	result, err := client.ImportPackageSelection(ctx, n8n.ImportPackageSelectionRequest{
		ProjectID:               projectID,
		WorkflowIDs:             []string{kept},
		DeletedWorkflowIDs:      []string{removed},
		WorkflowConflictPolicy:  n8n.ImportWorkflowConflictNewVersion,
		WorkflowIDPolicy:        n8n.ImportWorkflowIDSource,
		OverwriteDeletionPolicy: n8n.ImportDeletionArchive,
	}, bytes.NewReader(archive.Bytes()), "project.n8np")
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skipf("integration instance does not serve package import-selection: %v", err)
	}
	if err != nil {
		t.Fatalf("ImportPackageSelection: %v", err)
	}
	if len(result.Workflows) != 1 || result.Workflows[0].LocalID != kept || result.Workflows[0].ProjectID != projectID {
		t.Errorf("workflows = %+v, want only %q updated in place in project %q", result.Workflows, kept, projectID)
	}
	if len(result.RemovedWorkflows) != 1 || result.RemovedWorkflows[0].WorkflowID != removed || result.RemovedWorkflows[0].Deletion != "archived" {
		t.Errorf("removedWorkflows = %+v, want %q archived", result.RemovedWorkflows, removed)
	}

	path := filepath.Join(t.TempDir(), "project.n8np")
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var stdout, stderr strings.Builder
	code := cli.Run(ctx, []string{"package", "import-selection", "--file", path, "--project-id", projectID,
		"--workflow-id", kept, "--workflow-conflict-policy", "skip", "--yes", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("package import-selection exit code = %d (stderr: %s)", code, stderr.String())
	}
	var skipped n8n.ImportPackageResult
	if err := json.Unmarshal([]byte(stdout.String()), &skipped); err != nil {
		t.Fatalf("stdout is not an import result: %v\n%s", err, stdout.String())
	}
	if len(skipped.Workflows) != 1 || skipped.Workflows[0].Status != "skipped" {
		t.Errorf("workflows = %+v, want %q skipped under the skip policy", skipped.Workflows, kept)
	}
	if strings.Contains(stdout.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
