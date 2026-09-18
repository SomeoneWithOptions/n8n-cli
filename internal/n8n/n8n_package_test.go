package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const packageArchiveBytes = "\x1f\x8b\x08\x00fake-gzip-package"

const packageImportResultJSON = `{
  "package":{"sourceN8nVersion":"1.0.0","sourceId":"pkg-1","exportedAt":"2026-09-17T00:00:00.000Z"},
  "workflows":[{"sourceWorkflowId":"wf-src","localId":"wf-local","name":"Example","projectId":"p1","parentFolderId":null,"activeVersionId":null,"isArchived":false,"publishing":{"state":"unpublished"},"status":"created"}],
  "removedWorkflows":[],
  "removedFolders":[],
  "folders":[],
  "projects":[],
  "bindings":{"workflows":{"wf-src":"wf-local"},"credentials":{}},
  "credentials":{"matched":[],"stubbed":["cred-1"]},
  "dataTables":{"matched":0,"created":0},
  "variables":{"matched":[],"missing":[],"created":[],"stubbed":[],"updated":[]},
  "tags":{"matched":[],"created":[],"renamed":[],"reconciled":[],"skipped":[]},
  "extraFieldAddedLater":"ignored"
}`

func exportGzipHandler(t *testing.T, counts, disposition string) func(http.ResponseWriter, *http.Request) {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		if counts != "" {
			w.Header().Set(ExportCountsHeader, counts)
		}
		if disposition != "" {
			w.Header().Set("Content-Disposition", disposition)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(packageArchiveBytes))
	}
}

func TestExportPackageStreamsArchive(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, `{"workflows":1,"folders":0,"credentials":0,"dataTables":0,"variables":0}`, `attachment; filename="workflows.n8np"`))

	var out bytes.Buffer
	result, err := server.client(t).ExportPackage(context.Background(), ExportPackageRequest{
		WorkflowIDs: []string{"abc"},
	}, &out)
	if err != nil {
		t.Fatalf("ExportPackage: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+N8nPackageExportPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+N8nPackageExportPath)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if accept := req.Header.Get("Accept"); accept != "application/gzip" {
		t.Errorf("Accept = %q, want application/gzip", accept)
	}
	if ctype := req.Header.Get("Content-Type"); ctype != contentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", ctype, contentTypeJSON)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if ids, ok := sent["workflowIds"].([]any); !ok || len(ids) != 1 || ids[0] != "abc" {
		t.Errorf("workflowIds = %v, want [abc]", sent["workflowIds"])
	}
	if _, omitted := sent["includeVariableValues"]; omitted {
		t.Errorf("includeVariableValues = present, want omitted when unset")
	}
	if out.String() != packageArchiveBytes {
		t.Errorf("archive = %q, want the streamed gzip bytes", out.String())
	}
	if result.Bytes != int64(len(packageArchiveBytes)) {
		t.Errorf("bytes = %d, want %d", result.Bytes, len(packageArchiveBytes))
	}
	if result.Counts["workflows"] != 1 {
		t.Errorf("counts = %v, want workflows 1", result.Counts)
	}
	if result.Filename != "workflows.n8np" {
		t.Errorf("filename = %q, want workflows.n8np", result.Filename)
	}
}

func TestExportPackageOptionalBools(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, "", ""))

	var out bytes.Buffer
	_, err := server.client(t).ExportPackage(context.Background(), ExportPackageRequest{
		ProjectIDs:            []string{"p1"},
		IncludeVariableValues: Bool(false),
		IncludeTags:           Bool(true),
		IncludeArchived:       Bool(true),
	}, &out)
	if err != nil {
		t.Fatalf("ExportPackage: %v", err)
	}
	_, raw := server.last(t)
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["includeVariableValues"] != false || sent["includeTags"] != true || sent["includeArchivedWorkflows"] != true {
		t.Errorf("bools = %v, want explicit false/true/true", sent)
	}
}

func TestExportPackageValidation(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, "", ""))
	client := server.client(t)
	var out bytes.Buffer

	cases := []ExportPackageRequest{
		{},
		{WorkflowIDs: []string{""}},
		{WorkflowIDs: []string{"  padded "}},
		{WorkflowIDs: []string{"a"}, ProjectIDs: []string{"p"}},
		{FolderIDs: []string{"f"}, ProjectIDs: []string{"p"}},
		{WorkflowIDs: []string{"a"}, MissingDependencyPolicy: "sometimes"},
		{WorkflowIDs: []string{"a"}, WorkflowVersionPolicy: "sometimes"},
		{WorkflowIDs: []string{"a"}, CredentialExportPolicy: "sometimes"},
	}
	for i, request := range cases {
		if _, err := client.ExportPackage(context.Background(), request, &out); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	many := make([]string, maxExportIDs+1)
	for i := range many {
		many[i] = "w"
	}
	if _, err := client.ExportPackage(context.Background(), ExportPackageRequest{WorkflowIDs: many}, &out); err == nil {
		t.Error("301 workflow IDs: want a validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
	if _, err := client.ExportPackage(context.Background(), ExportPackageRequest{WorkflowIDs: []string{"a"}}, nil); err == nil {
		t.Error("nil writer: want an error")
	}
}

func TestExportPackageErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"bad request", http.StatusBadRequest, `{"message":"1 workflow(s) not found"}`, func(err error) bool { return IsStatus(err, http.StatusBadRequest) }},
		{"unauthorized", http.StatusUnauthorized, `{"message":"unauthorized"}`, IsUnauthorized},
		{"forbidden", http.StatusForbidden, `{"message":"denied"}`, IsForbidden},
		{"not found", http.StatusNotFound, `{"message":"not found"}`, IsNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
			var out bytes.Buffer
			_, err := server.client(t).ExportPackage(context.Background(), ExportPackageRequest{WorkflowIDs: []string{"a"}}, &out)
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the %d behavior", err, tc.status)
			}
		})
	}
}

func TestExportPackageBadCountsHeader(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, "not-json", ""))
	var out bytes.Buffer
	_, err := server.client(t).ExportPackage(context.Background(), ExportPackageRequest{WorkflowIDs: []string{"a"}}, &out)
	if err == nil || !strings.Contains(err.Error(), ExportCountsHeader) {
		t.Fatalf("error = %v, want the counts header failure", err)
	}
}

func TestExportPackageNilContext(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, "", ""))
	var out bytes.Buffer
	// A nil context is the behavior under test, so it is bound to a variable
	// rather than written as a literal that the analysers reject.
	var ctx context.Context
	if _, err := server.client(t).ExportPackage(ctx, ExportPackageRequest{WorkflowIDs: []string{"a"}}, &out); err == nil {
		t.Error("nil context: want an error")
	}
}

func multipartReader(contentType string, raw []byte) (*multipart.Reader, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if mediaType != "multipart/form-data" {
		return nil, io.ErrUnexpectedEOF
	}
	return multipart.NewReader(bytes.NewReader(raw), params["boundary"]), nil
}

func parseMultipartImport(t *testing.T, req *http.Request, raw []byte) (map[string]string, string, string) {
	t.Helper()
	contentType := req.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", contentType)
	}
	server := req
	_ = server
	// Re-parse from the captured raw body: the handler already consumed it.
	mr, err := multipartReader(contentType, raw)
	if err != nil {
		t.Fatalf("multipart reader: %v", err)
	}
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
		name := part.FormName()
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part %s: %v", name, err)
		}
		if name == "package" {
			fileBytes = string(data)
			filename = part.FileName()
		} else {
			fields[name] = string(data)
		}
	}
	return fields, fileBytes, filename
}

func TestImportPackageStreamsMultipart(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))

	archive := strings.NewReader(packageArchiveBytes)
	result, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		WorkflowConflictPolicy: ImportWorkflowConflictFail,
		WorkflowIDPolicy:       ImportWorkflowIDSource,
		ProjectID:              "p1",
		TagConflictPolicy:      ImportTagConflictRename,
	}, archive, "workflows.n8np")
	if err != nil {
		t.Fatalf("ImportPackage: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+N8nPackageImportPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+N8nPackageImportPath)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	fields, fileBytes, filename := parseMultipartImport(t, req, []byte(raw))
	if fields["workflowConflictPolicy"] != ImportWorkflowConflictFail {
		t.Errorf("workflowConflictPolicy = %q, want fail", fields["workflowConflictPolicy"])
	}
	if fields["workflowIdPolicy"] != ImportWorkflowIDSource {
		t.Errorf("workflowIdPolicy = %q, want source", fields["workflowIdPolicy"])
	}
	if fields["projectId"] != "p1" || fields["tagConflictPolicy"] != ImportTagConflictRename {
		t.Errorf("fields = %v, want project and tag policies", fields)
	}
	if _, omitted := fields["folderConflictPolicy"]; omitted {
		t.Errorf("folderConflictPolicy = present, want omitted when unset")
	}
	if fileBytes != packageArchiveBytes {
		t.Errorf("package bytes = %q, want the streamed archive", fileBytes)
	}
	if filename != "workflows.n8np" {
		t.Errorf("filename = %q, want workflows.n8np", filename)
	}

	want := ImportedWorkflow{
		SourceWorkflowID: "wf-src",
		LocalID:          "wf-local",
		Name:             "Example",
		ProjectID:        "p1",
		Publishing:       ImportPublishing{State: "unpublished"},
		Status:           "created",
	}
	if diff := cmp.Diff(want, result.Workflows[0]); diff != "" {
		t.Errorf("workflow mismatch (-want +got):\n%s", diff)
	}
	if result.Package.SourceID != "pkg-1" || result.Credentials.Stubbed[0] != "cred-1" {
		t.Errorf("result = %+v, want the package id and stubbed credential", result)
	}
	if result.Bindings.Workflows["wf-src"] != "wf-local" {
		t.Errorf("bindings = %+v, want wf-src to wf-local", result.Bindings)
	}
}

func TestImportPackageDefaultsFilename(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))

	_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		WorkflowConflictPolicy: ImportWorkflowConflictSkip,
		WorkflowIDPolicy:       ImportWorkflowIDSource,
	}, strings.NewReader(packageArchiveBytes), "")
	if err != nil {
		t.Fatalf("ImportPackage: %v", err)
	}
	req, raw := server.last(t)
	_, _, filename := parseMultipartImport(t, req, []byte(raw))
	if filename != DefaultImportPackageFilename {
		t.Errorf("filename = %q, want %q", filename, DefaultImportPackageFilename)
	}
}

func TestImportPackageValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))
	client := server.client(t)
	archive := func() io.Reader { return strings.NewReader(packageArchiveBytes) }

	cases := []ImportPackageRequest{
		{},
		{WorkflowConflictPolicy: "sometimes", WorkflowIDPolicy: ImportWorkflowIDSource},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: "sometimes"},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource, CredentialMatchingMode: "sometimes"},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource, Bindings: "[1,2]"},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource, Bindings: "not-json"},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource, ProjectID: " padded "},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource, OverwriteDeletionPolicy: "sometimes"},
		{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource, VariableParentPolicy: "sometimes"},
	}
	for i, request := range cases {
		if _, err := client.ImportPackage(context.Background(), request, archive(), "p.n8np"); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	if _, err := client.ImportPackage(context.Background(), ImportPackageRequest{
		WorkflowConflictPolicy: ImportWorkflowConflictFail,
		WorkflowIDPolicy:       ImportWorkflowIDSource,
	}, nil, "p.n8np"); err == nil {
		t.Error("nil archive: want an error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestImportPackageBlockedPreservesIssues(t *testing.T) {
	body := `{"message":"import blocked","issues":[{"type":"workflow-conflict","sourceWorkflowId":"a","existingWorkflowId":"b","name":"Example"},{"type":"tag-conflict","kind":"fail-policy","sourceTagId":"t","name":"Tag","customFutureField":1}]}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, body))

	_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		WorkflowConflictPolicy: ImportWorkflowConflictFail,
		WorkflowIDPolicy:       ImportWorkflowIDSource,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if err == nil || !IsImportConflict(err) || IsImportUnprocessable(err) {
		t.Fatalf("error = %v, want a 409 blocked error", err)
	}
	blocked, ok := AsImportBlocked(err)
	if !ok {
		t.Fatalf("AsImportBlocked(%v) = false, want true", err)
	}
	if blocked.Message != "import blocked" || len(blocked.Issues) != 2 {
		t.Fatalf("blocked = %+v, want the message and 2 issues", blocked)
	}
	if blocked.Issues[0].Type != "workflow-conflict" || blocked.Issues[1].Type != "tag-conflict" {
		t.Errorf("issue types = %q, %q", blocked.Issues[0].Type, blocked.Issues[1].Type)
	}
	if !strings.Contains(string(blocked.Issues[1].Raw), "customFutureField") {
		t.Errorf("raw issue = %s, want the unknown field preserved", blocked.Issues[1].Raw)
	}
	if !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "2 issue(s)") {
		t.Errorf("error = %q, want status and issue summary", err.Error())
	}
}

func TestImportPackageUnprocessable(t *testing.T) {
	body := `{"message":"missing credentials","issues":[{"type":"credential-missing","credentialId":"c1"}]}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusUnprocessableEntity, body))

	_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		WorkflowConflictPolicy: ImportWorkflowConflictSkip,
		WorkflowIDPolicy:       ImportWorkflowIDNew,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if err == nil || !IsImportUnprocessable(err) || IsImportConflict(err) {
		t.Fatalf("error = %v, want a 422 blocked error", err)
	}
}

func TestImportPackageOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"bad request", http.StatusBadRequest, `{"message":"bad bindings"}`, func(err error) bool { return IsStatus(err, http.StatusBadRequest) }},
		{"unauthorized", http.StatusUnauthorized, `{"message":"unauthorized"}`, IsUnauthorized},
		{"forbidden", http.StatusForbidden, `{"message":"denied"}`, IsForbidden},
		{"not found", http.StatusNotFound, `{"message":"not found"}`, IsNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
			_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
				WorkflowConflictPolicy: ImportWorkflowConflictSkip,
				WorkflowIDPolicy:       ImportWorkflowIDSource,
			}, strings.NewReader(packageArchiveBytes), "p.n8np")
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the %d behavior", err, tc.status)
			}
			if _, blocked := AsImportBlocked(err); blocked {
				t.Errorf("error = %v, want a plain API error, not a blocked error", err)
			}
		})
	}
}
