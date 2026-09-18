package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
