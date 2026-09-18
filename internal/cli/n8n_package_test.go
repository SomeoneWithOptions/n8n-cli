package cli

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliPackageArchive = "fake-n8np-archive-bytes"

const cliPackageImportResult = `{
  "package":{"sourceN8nVersion":"1.0.0","sourceId":"pkg-1","exportedAt":"2026-09-17T00:00:00.000Z"},
  "workflows":[
    {"sourceWorkflowId":"wf-src","localId":"wf-local","name":"Example","projectId":"p1","parentFolderId":null,"activeVersionId":null,"isArchived":false,"publishing":{"state":"unpublished"},"status":"created"},
    {"sourceWorkflowId":"wf-old","localId":"wf-old","name":"Kept","projectId":"p1","parentFolderId":null,"activeVersionId":null,"isArchived":false,"publishing":{"state":"unchanged"},"status":"skipped"}
  ],
  "removedWorkflows":[],
  "removedFolders":[],
  "folders":[],
  "projects":[],
  "bindings":{"workflows":{"wf-src":"wf-local"},"credentials":{}},
  "credentials":{"matched":[],"stubbed":["cred-1"]},
  "dataTables":{"matched":0,"created":0},
  "variables":{"matched":[],"missing":["MISSING_VAR"],"created":[],"stubbed":[],"updated":[]},
  "tags":{"matched":[],"created":[],"renamed":[],"reconciled":[],"skipped":["Dropped"]}
}`

func packageFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writePackageArchive(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test-package.n8np")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write package archive: %v", err)
	}
	return path
}

func writePackageInput(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write package input: %v", err)
	}
	return path
}

func parseCLIImportMultipart(t *testing.T, f *fixture) (map[string]string, string, string) {
	t.Helper()
	req := f.lastRequest()
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("Content-Type = %q, want multipart/form-data", req.Header.Get("Content-Type"))
	}
	mr := multipart.NewReader(strings.NewReader(f.lastBody()), params["boundary"])
	fields := map[string]string{}
	var fileBytes, filename string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if part.FormName() == "package" {
			fileBytes = string(data)
			filename = part.FileName()
		} else {
			fields[part.FormName()] = string(data)
		}
	}
	return fields, fileBytes, filename
}

func TestPackageExportWritesFileAndSummary(t *testing.T) {
	f := packageFixture(t, cliPackageArchive)
	out := filepath.Join(t.TempDir(), "workflows.n8np")

	got := f.run("package", "export", "--workflow-id", "abc", "--out", out)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.N8nPackageExportPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.ExportPackageRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not an export: %v\n%s", err, f.lastBody())
	}
	if len(sent.WorkflowIDs) != 1 || sent.WorkflowIDs[0] != "abc" {
		t.Errorf("request = %+v, want workflow abc", sent)
	}
	if sent.IncludeVariableValues != nil || sent.IncludeTags != nil {
		t.Errorf("request = %+v, want omitted bools when flags are unset", sent)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if string(data) != cliPackageArchive {
		t.Errorf("archive = %q, want the streamed bytes", data)
	}
	for _, want := range []string{"Exported:", "Bytes:", out} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
}

func TestPackageExportJSONAndPolicies(t *testing.T) {
	f := packageFixture(t, cliPackageArchive)
	out := filepath.Join(t.TempDir(), "project.n8np")

	got := f.run("package", "export",
		"--project-id", "p1",
		"--missing-workflow-dependency-policy", "include-in-package",
		"--workflow-version-policy", "latest",
		"--credential-export-policy", "no-values",
		"--include-archived",
		"--out", out, "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent n8n.ExportPackageRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("request body is not an export: %v", err)
	}
	if len(sent.ProjectIDs) != 1 || sent.MissingDependencyPolicy != "include-in-package" ||
		sent.WorkflowVersionPolicy != "latest" || sent.CredentialExportPolicy != "no-values" {
		t.Errorf("request = %+v, want the project selection and policies", sent)
	}
	if sent.IncludeArchived == nil || !*sent.IncludeArchived {
		t.Errorf("request = %+v, want explicit include-archived true", sent)
	}
	var summary struct {
		File   string `json:"file"`
		Result *struct {
			Bytes int64 `json:"bytes"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &summary); err != nil || summary.File != out {
		t.Errorf("stdout = %q, want the JSON summary with the file (error: %v)", got.stdout, err)
	}
}

func TestPackageExportStdoutMovesSummaryToStderr(t *testing.T) {
	f := packageFixture(t, cliPackageArchive)

	got := f.run("package", "export", "--workflow-id", "abc", "--out", "-")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if got.stdout != cliPackageArchive {
		t.Errorf("stdout = %q, want the raw archive bytes", got.stdout)
	}
	if !strings.Contains(got.stderr, "Exported:") {
		t.Errorf("stderr = %q, want the summary when stdout carries the archive", got.stderr)
	}
}

func TestPackageExportRejectsBadSelectionBeforeTransport(t *testing.T) {
	f := packageFixture(t, cliPackageArchive)
	before := f.requestCount()
	out := filepath.Join(t.TempDir(), "x.n8np")
	input := writePackageInput(t, "selection.json", `{"workflowIds":["a"]}`)

	cases := [][]string{
		{"package", "export", "--out", out},
		{"package", "export", "--workflow-id", "a", "--project-id", "p", "--out", out},
		{"package", "export", "--workflow-id", "a", "--missing-workflow-dependency-policy", "sometimes", "--out", out},
		{"package", "export", "--workflow-id", "a", "--out", out, "--output", "yaml"},
		{"package", "export", "--workflow-id", "a", "--out", out, "--input", input},
		{"package", "export", "--workflow-id", "a"},
	}
	for _, args := range cases {
		got := f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v exit = %d, want %d (stderr: %s)", args, got.code, ExitError, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestPackageExportInputDocument(t *testing.T) {
	f := packageFixture(t, cliPackageArchive)

	out := filepath.Join(t.TempDir(), "from-file.n8np")
	input := writePackageInput(t, "selection.json", `{"folderIds":["f1"],"workflowVersionPolicy":"latest"}`)
	got := f.run("package", "export", "--input", input, "--out", out)
	if got.code != ExitSuccess {
		t.Fatalf("input exit = %d, stderr: %s", got.code, got.stderr)
	}
	var sent n8n.ExportPackageRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || len(sent.FolderIDs) != 1 {
		t.Errorf("request = %q (error %v), want the file document", f.lastBody(), err)
	}

	f.stdin = `{"workflowIds":["a"]}`
	got = f.run("package", "export", "--input", "-", "--out", filepath.Join(t.TempDir(), "from-stdin.n8np"))
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}

	bad := writePackageInput(t, "bad.json", `{"workflowIds":["a"],"unknown":true}`)
	before := f.requestCount()
	got = f.run("package", "export", "--input", bad, "--out", filepath.Join(t.TempDir(), "bad.n8np"))
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("unknown-field result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPackageImportConfirmationPolicyAndBody(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "new-version")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	for _, want := range []string{"new-version", "source", "stubs"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("prompt = %q, want %q", got.stderr, want)
		}
	}

	f.interactive = false
	got = f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "new-version")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("package", "import", "--file", archive,
		"--workflow-conflict-policy", "new-version",
		"--project-id", "p1",
		"--tag-conflict-policy", "rename",
		"--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.N8nPackageImportPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	fields, fileBytes, filename := parseCLIImportMultipart(t, f)
	if fields["workflowConflictPolicy"] != "new-version" || fields["workflowIdPolicy"] != "source" {
		t.Errorf("fields = %v, want conflict new-version and explicit id source", fields)
	}
	if fields["projectId"] != "p1" || fields["tagConflictPolicy"] != "rename" {
		t.Errorf("fields = %v, want project and tag policies", fields)
	}
	if fileBytes != cliPackageArchive || filename != "test-package.n8np" {
		t.Errorf("package = %q (%q), want the archive bytes and base name", fileBytes, filename)
	}
	var result n8n.ImportPackageResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || len(result.Workflows) != 2 {
		t.Errorf("stdout = %q, want the import result (error: %v)", got.stdout, err)
	}
}

func TestPackageImportTextWarnsAboutDroppedItems(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)

	got := f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "skip", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Imported:", "2 workflows", "1 skipped", "Credentials:", "stubbed", "1 missing", "1 skipped"} {
		if !strings.Contains(got.stdout, want) && !strings.Contains(got.stderr, want) {
			t.Errorf("output missing %q:\n--- stdout ---\n%s\n--- stderr ---\n%s", want, got.stdout, got.stderr)
		}
	}
}

func TestPackageImportStdinRequiresYes(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	f.stdin = cliPackageArchive
	before := f.requestCount()

	got := f.run("package", "import", "--file", "-", "--workflow-conflict-policy", "skip")
	if got.code != ExitError || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
		t.Errorf("stdin without --yes result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.stdin = cliPackageArchive
	got = f.run("package", "import", "--file", "-", "--workflow-conflict-policy", "skip", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}
	_, fileBytes, filename := parseCLIImportMultipart(t, f)
	if fileBytes != cliPackageArchive || filename != n8n.DefaultImportPackageFilename {
		t.Errorf("package = %q (%q), want stdin bytes under the default name", fileBytes, filename)
	}
}

func TestPackageImportHardDeleteWarnsStronger(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)
	f.stdin = "no\n"

	got := f.run("package", "import", "--file", archive,
		"--workflow-conflict-policy", "new-version",
		"--folder-conflict-policy", "overwrite",
		"--overwrite-deletion-policy", "hard-delete")
	if got.code != ExitError {
		t.Fatalf("exit = %d, want a decline (stderr: %s)", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "hard-delete") || !strings.Contains(got.stderr, "prefer archive") {
		t.Errorf("prompt = %q, want the hard-delete warning", got.stderr)
	}
}

func TestPackageImportRejectsBadPoliciesBeforeTransport(t *testing.T) {
	f := packageFixture(t, cliPackageImportResult)
	archive := writePackageArchive(t, cliPackageArchive)
	before := f.requestCount()

	cases := [][]string{
		{"package", "import", "--workflow-conflict-policy", "new-version", "--yes"},
		{"package", "import", "--file", archive, "--yes"},
		{"package", "import", "--file", archive, "--workflow-conflict-policy", "sometimes", "--yes"},
		{"package", "import", "--file", archive, "--workflow-conflict-policy", "new-version", "--workflow-id-policy", "sometimes", "--yes"},
		{"package", "import", "--file", archive, "--workflow-conflict-policy", "new-version", "--bindings", "not-json", "--yes"},
		{"package", "import", "--file", archive, "--workflow-conflict-policy", "new-version", "--variable-parent-policy", "sometimes", "--yes"},
		{"package", "import", "--file", archive, "--workflow-conflict-policy", "new-version", "--yes", "--output", "yaml"},
		{"package", "import", "--file", filepath.Join(t.TempDir(), "missing.n8np"), "--workflow-conflict-policy", "new-version", "--yes"},
	}
	for _, args := range cases {
		got := f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v exit = %d, want %d (stderr: %s)", args, got.code, ExitError, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestPackageImportBlockedPointsAtPolicies(t *testing.T) {
	f := packageFixture(t, "")

	f.status = http.StatusConflict
	f.body = `{"message":"import blocked","issues":[{"type":"workflow-conflict","sourceWorkflowId":"a"}]}`
	archive := writePackageArchive(t, cliPackageArchive)
	got := f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "fail", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "409") || !strings.Contains(got.stderr, "--workflow-conflict-policy") {
		t.Errorf("409 result = %+v, want the conflict remediation", got)
	}

	f.body = `{"message":"missing credentials","issues":[{"type":"credential-missing"}]}`
	f.status = http.StatusUnprocessableEntity
	got = f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "skip", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "422") || !strings.Contains(got.stderr, "--credential-missing-mode") {
		t.Errorf("422 result = %+v, want the missing-reference remediation", got)
	}
}

func TestPackageDenialsAreReported(t *testing.T) {
	f := packageFixture(t, cliPackageArchive)
	out := filepath.Join(t.TempDir(), "denied.n8np")

	f.status = http.StatusUnauthorized
	got := f.run("package", "export", "--workflow-id", "a", "--out", out)
	if got.code != ExitError || !strings.Contains(got.stderr, "n8n auth login") {
		t.Errorf("401 result = %+v, want the credential hint", got)
	}

	archive := writePackageArchive(t, cliPackageArchive)
	f.status = http.StatusForbidden
	got = f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "skip", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "workflow:import") || !strings.Contains(got.stderr, "n8n discover --resource n8npackage") {
		t.Errorf("403 result = %+v, want scope and discover hint", got)
	}

	f.status = http.StatusNotFound
	got = f.run("package", "export", "--workflow-id", "a", "--out", out)
	if got.code != ExitError || !strings.Contains(got.stderr, "Packages") || !strings.Contains(got.stderr, "n8n discover --resource n8npackage") {
		t.Errorf("404 result = %+v, want the availability hint", got)
	}

	f.status = http.StatusBadRequest
	got = f.run("package", "import", "--file", archive, "--workflow-conflict-policy", "skip", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "400") {
		t.Errorf("400 result = %+v, want the bad-request guidance", got)
	}
}

func TestPackageExportCleansPartialFile(t *testing.T) {
	f := packageFixture(t, "")
	f.status = http.StatusBadRequest
	out := filepath.Join(t.TempDir(), "partial.n8np")

	got := f.run("package", "export", "--workflow-id", "a", "--out", out)
	if got.code != ExitError {
		t.Fatalf("exit = %d, want %d", got.code, ExitError)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("partial file = %v, want it removed after a failed export", err)
	}
}

func TestPackageExportBetaIsVisible(t *testing.T) {
	got := run(t, "package", "export", "--help")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d", got.code)
	}
	for _, want := range []string{"Beta", "Packages", "--out"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("help missing %q:\n%s", want, got.stdout)
		}
	}
}
