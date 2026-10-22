package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestExportPackageSendsCurrentFieldNames(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, `{"agents":1,"workflows":1}`, ""))

	var out bytes.Buffer
	_, err := server.client(t).ExportPackage(context.Background(), ExportPackageRequest{
		AgentIDs:         []string{"ag1"},
		WorkflowIDs:      []string{"wf1"},
		DependencyPolicy: ExportDependencyIncludeInPackage,
		VersionPolicy:    ExportVersionPreferPublished,
	}, &out)
	if err != nil {
		t.Fatalf("ExportPackage: %v", err)
	}
	_, raw := server.last(t)
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	want := map[string]any{
		"agentIds":         []any{"ag1"},
		"workflowIds":      []any{"wf1"},
		"dependencyPolicy": "include-in-package",
		"versionPolicy":    "prefer-published",
	}
	if diff := cmp.Diff(want, sent); diff != "" {
		t.Errorf("export body mismatch (-want +got):\n%s", diff)
	}
}

func TestExportPackageAcceptsEveryPolicyValue(t *testing.T) {
	server := newCommunityPackageServer(t, exportGzipHandler(t, "", ""))
	client := server.client(t)
	var out bytes.Buffer
	for _, policy := range exportDependencyPolicies {
		if _, err := client.ExportPackage(context.Background(), ExportPackageRequest{ProjectIDs: []string{"p"}, DependencyPolicy: policy}, &out); err != nil {
			t.Errorf("dependency policy %q: %v", policy, err)
		}
	}
	for _, policy := range exportVersionPolicies {
		if _, err := client.ExportPackage(context.Background(), ExportPackageRequest{ProjectIDs: []string{"p"}, VersionPolicy: policy}, &out); err != nil {
			t.Errorf("version policy %q: %v", policy, err)
		}
	}
	many := make([]string, maxExportIDs)
	for i := range many {
		many[i] = "p"
	}
	if _, err := client.ExportPackage(context.Background(), ExportPackageRequest{ProjectIDs: many}, &out); err != nil {
		t.Errorf("300 project IDs: %v, want accepted", err)
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
		{AgentIDs: []string{"ag"}, ProjectIDs: []string{"p"}},
		{AgentIDs: []string{" "}},
		{ProjectIDs: []string{"p", ""}},
		{WorkflowIDs: []string{"a"}, DependencyPolicy: "sometimes"},
		{WorkflowIDs: []string{"a"}, VersionPolicy: "sometimes"},
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
	for name, request := range map[string]ExportPackageRequest{
		"agentIds":    {AgentIDs: many},
		"workflowIds": {WorkflowIDs: many},
		"folderIds":   {FolderIDs: many},
		"projectIds":  {ProjectIDs: many},
	} {
		if _, err := client.ExportPackage(context.Background(), request, &out); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("301 %s: error = %v, want a validation error naming the field", name, err)
		}
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

func TestImportPackageAcceptsNewModes(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))
	client := server.client(t)

	for _, tc := range []struct {
		credential, schema string
	}{
		{ImportCredentialMissingCreateWithValues, ImportDataTableSchemaOverwrite},
		{ImportCredentialMissingCreateStub, ImportDataTableSchemaOverwriteNonDestructive},
	} {
		_, err := client.ImportPackage(context.Background(), ImportPackageRequest{
			WorkflowConflictPolicy:  ImportWorkflowConflictSkip,
			WorkflowIDPolicy:        ImportWorkflowIDSource,
			CredentialMissingMode:   tc.credential,
			DataTableSchemaConflict: tc.schema,
		}, strings.NewReader(packageArchiveBytes), "p.n8np")
		if err != nil {
			t.Fatalf("ImportPackage(%s, %s): %v", tc.credential, tc.schema, err)
		}
		req, raw := server.last(t)
		fields, _, _ := parseMultipartImport(t, req, []byte(raw))
		if fields["credentialMissingMode"] != tc.credential || fields["dataTableSchemaConflictPolicy"] != tc.schema {
			t.Errorf("fields = %v, want credential %q and schema %q", fields, tc.credential, tc.schema)
		}
	}
}

func TestImportPackageResultKeepsNewFields(t *testing.T) {
	body := `{
  "package":{"sourceN8nVersion":"2.0.0","sourceId":"pkg-2","exportedAt":"2026-10-01T00:00:00.000Z"},
  "workflows":[],"removedWorkflows":[],"removedFolders":[],"folders":[],"projects":[],
  "bindings":{"workflows":{},"credentials":{"c-seeded":"t-seeded","c-partial":"t-partial"}},
  "credentials":{"matched":[],"stubbed":[],"seeded":["c-seeded"],"incomplete":["c-partial"],"futureBucket":["x"]},
  "dataTables":{"matched":1,"created":0,"updated":2},
  "variables":{"matched":[],"missing":[],"created":[],"stubbed":[],"updated":[]},
  "tags":{"matched":[],"created":[],"renamed":[],"reconciled":[],"skipped":[]},
  "agents":{"created":["ag-1"]}
}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))

	result, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		WorkflowConflictPolicy: ImportWorkflowConflictSkip,
		WorkflowIDPolicy:       ImportWorkflowIDSource,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if err != nil {
		t.Fatalf("ImportPackage: %v", err)
	}
	wantCredentials := ImportCredentials{Matched: []string{}, Stubbed: []string{}, Seeded: []string{"c-seeded"}, Incomplete: []string{"c-partial"}}
	if diff := cmp.Diff(wantCredentials, result.Credentials); diff != "" {
		t.Errorf("credentials mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ImportDataTables{Matched: 1, Updated: 2}, result.DataTables); diff != "" {
		t.Errorf("data tables mismatch (-want +got):\n%s", diff)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip lost data (-want +got):\n%s", diff)
	}

	built := ImportPackageResult{DataTables: ImportDataTables{Updated: 3}}
	encoded, err = json.Marshal(built)
	if err != nil || !strings.Contains(string(encoded), `"updated":3`) {
		t.Errorf("Marshal without raw = %s (error %v), want the typed fields", encoded, err)
	}
}

func TestImportBlockedErrorNamesItsPath(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusConflict, Status: "409 Conflict", Header: http.Header{}}
	blocked := newImportBlockedError(resp, "/n8n-packages/import-selection", []byte(`{"message":"overlap","issues":[{"type":"workflow-removal-conflict"}]}`))
	if blocked.Path != "/n8n-packages/import-selection" {
		t.Errorf("path = %q, want the selection endpoint", blocked.Path)
	}
	if !strings.Contains(blocked.Error(), "POST /n8n-packages/import-selection") {
		t.Errorf("error = %q, want the path in the message", blocked.Error())
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

func TestImportPackageSelectionStreamsMultipart(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))

	result, err := server.client(t).ImportPackageSelection(context.Background(), ImportPackageSelectionRequest{
		ProjectID:               "src-project",
		WorkflowIDs:             []string{"wf-1", "wf-2"},
		DeletedWorkflowIDs:      []string{"wf-3"},
		WorkflowConflictPolicy:  ImportWorkflowConflictSkip,
		WorkflowIDPolicy:        ImportWorkflowIDNew,
		OverwriteDeletionPolicy: ImportDeletionHardDelete,
	}, strings.NewReader(packageArchiveBytes), "project.n8np")
	if err != nil {
		t.Fatalf("ImportPackageSelection: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+N8nPackageImportSelectionPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+N8nPackageImportSelectionPath)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	fields, fileBytes, filename := parseMultipartImport(t, req, []byte(raw))
	want := map[string]string{
		"selectedProjectId":       "src-project",
		"selectedWorkflowIds":     `["wf-1","wf-2"]`,
		"deletedWorkflowIds":      `["wf-3"]`,
		"workflowConflictPolicy":  ImportWorkflowConflictSkip,
		"workflowIdPolicy":        ImportWorkflowIDNew,
		"overwriteDeletionPolicy": ImportDeletionHardDelete,
	}
	if diff := cmp.Diff(want, fields); diff != "" {
		t.Errorf("fields mismatch (-want +got):\n%s", diff)
	}
	if fileBytes != packageArchiveBytes || filename != "project.n8np" {
		t.Errorf("package = %q (%q), want the streamed archive and its name", fileBytes, filename)
	}
	if len(result.Workflows) != 1 || result.Workflows[0].LocalID != "wf-local" {
		t.Errorf("result = %+v, want the shared import result", result)
	}
}

func TestImportPackageSelectionOmitsUnsetFields(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))

	_, err := server.client(t).ImportPackageSelection(context.Background(), ImportPackageSelectionRequest{
		ProjectID:          "src-project",
		DeletedWorkflowIDs: []string{"wf-3"},
		WorkflowIDPolicy:   ImportWorkflowIDSource,
	}, strings.NewReader(packageArchiveBytes), "")
	if err != nil {
		t.Fatalf("ImportPackageSelection: %v", err)
	}
	req, raw := server.last(t)
	fields, _, filename := parseMultipartImport(t, req, []byte(raw))
	want := map[string]string{
		"selectedProjectId":   "src-project",
		"selectedWorkflowIds": `[]`,
		"deletedWorkflowIds":  `["wf-3"]`,
		"workflowIdPolicy":    ImportWorkflowIDSource,
	}
	if diff := cmp.Diff(want, fields); diff != "" {
		t.Errorf("delete-only fields mismatch (-want +got):\n%s", diff)
	}
	if filename != DefaultImportPackageFilename {
		t.Errorf("filename = %q, want %q", filename, DefaultImportPackageFilename)
	}

	_, err = server.client(t).ImportPackageSelection(context.Background(), ImportPackageSelectionRequest{
		ProjectID:        "src-project",
		WorkflowIDs:      []string{"wf-1"},
		WorkflowIDPolicy: ImportWorkflowIDSource,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if err != nil {
		t.Fatalf("ImportPackageSelection: %v", err)
	}
	req, raw = server.last(t)
	fields, _, _ = parseMultipartImport(t, req, []byte(raw))
	for _, name := range []string{"deletedWorkflowIds", "workflowConflictPolicy", "overwriteDeletionPolicy"} {
		if _, present := fields[name]; present {
			t.Errorf("%s = present, want omitted when unset", name)
		}
	}
}

func TestImportPackageSelectionValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))
	client := server.client(t)

	base := func(mutate func(*ImportPackageSelectionRequest)) ImportPackageSelectionRequest {
		request := ImportPackageSelectionRequest{
			ProjectID:        "p",
			WorkflowIDs:      []string{"a"},
			WorkflowIDPolicy: ImportWorkflowIDSource,
		}
		mutate(&request)
		return request
	}
	cases := map[string]ImportPackageSelectionRequest{
		"no project":           base(func(r *ImportPackageSelectionRequest) { r.ProjectID = "" }),
		"padded project":       base(func(r *ImportPackageSelectionRequest) { r.ProjectID = " p " }),
		"nothing selected":     base(func(r *ImportPackageSelectionRequest) { r.WorkflowIDs = nil }),
		"empty workflow id":    base(func(r *ImportPackageSelectionRequest) { r.WorkflowIDs = []string{""} }),
		"padded workflow id":   base(func(r *ImportPackageSelectionRequest) { r.WorkflowIDs = []string{"a "} }),
		"duplicate selected":   base(func(r *ImportPackageSelectionRequest) { r.WorkflowIDs = []string{"a", "a"} }),
		"duplicate deleted":    base(func(r *ImportPackageSelectionRequest) { r.DeletedWorkflowIDs = []string{"b", "b"} }),
		"selected and deleted": base(func(r *ImportPackageSelectionRequest) { r.DeletedWorkflowIDs = []string{"a"} }),
		"empty deleted id":     base(func(r *ImportPackageSelectionRequest) { r.DeletedWorkflowIDs = []string{" "} }),
		"bad conflict policy":  base(func(r *ImportPackageSelectionRequest) { r.WorkflowConflictPolicy = "sometimes" }),
		"missing id policy":    base(func(r *ImportPackageSelectionRequest) { r.WorkflowIDPolicy = "" }),
		"bad id policy":        base(func(r *ImportPackageSelectionRequest) { r.WorkflowIDPolicy = "sometimes" }),
		"bad deletion policy": base(func(r *ImportPackageSelectionRequest) {
			r.DeletedWorkflowIDs = []string{"b"}
			r.OverwriteDeletionPolicy = "sometimes"
		}),
		"deletion without delete": base(func(r *ImportPackageSelectionRequest) { r.OverwriteDeletionPolicy = ImportDeletionArchive }),
	}
	for name, request := range cases {
		if _, err := client.ImportPackageSelection(context.Background(), request, strings.NewReader(packageArchiveBytes), "p.n8np"); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	if _, err := client.ImportPackageSelection(context.Background(), base(func(*ImportPackageSelectionRequest) {}), nil, "p.n8np"); err == nil {
		t.Error("nil archive: want an error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestImportPackageSelectionBlocked(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusConflict, `{"message":"overlap","issues":[{"type":"workflow-removal-conflict","workflowId":"a"}]}`, IsImportConflict},
		{http.StatusUnprocessableEntity, `{"message":"denied","issues":[{"type":"workflow-removal-forbidden","workflowId":"b"}]}`, IsImportUnprocessable},
	} {
		server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
		_, err := server.client(t).ImportPackageSelection(context.Background(), ImportPackageSelectionRequest{
			ProjectID:          "p",
			WorkflowIDs:        []string{"a"},
			DeletedWorkflowIDs: []string{"b"},
			WorkflowIDPolicy:   ImportWorkflowIDNew,
		}, strings.NewReader(packageArchiveBytes), "p.n8np")
		if err == nil || !tc.check(err) {
			t.Fatalf("%d: error = %v, want a blocked error", tc.status, err)
		}
		blocked, _ := AsImportBlocked(err)
		if blocked.Path != N8nPackageImportSelectionPath || len(blocked.Issues) != 1 || !strings.Contains(string(blocked.Issues[0].Raw), "workflowId") {
			t.Errorf("%d: blocked = %+v, want the selection path and the verbatim issue", tc.status, blocked)
		}
		if !strings.Contains(err.Error(), "POST "+N8nPackageImportSelectionPath) {
			t.Errorf("%d: error = %q, want the selection path", tc.status, err.Error())
		}
	}
}

func TestImportPackageSelectionOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"workflow package", http.StatusBadRequest, `{"message":"import-selection needs a project package"}`, func(err error) bool { return IsStatus(err, http.StatusBadRequest) }},
		{"forbidden", http.StatusForbidden, `{"message":"denied"}`, IsForbidden},
		{"not found", http.StatusNotFound, `{"message":"not found"}`, IsNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
			_, err := server.client(t).ImportPackageSelection(context.Background(), ImportPackageSelectionRequest{
				ProjectID:        "p",
				WorkflowIDs:      []string{"a"},
				WorkflowIDPolicy: ImportWorkflowIDSource,
			}, strings.NewReader(packageArchiveBytes), "p.n8np")
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the %d behavior", err, tc.status)
			}
			if !strings.Contains(err.Error(), N8nPackageImportSelectionPath) {
				t.Errorf("error = %q, want the selection path", err.Error())
			}
		})
	}
}

func TestImportPackageSelectionCancellation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := server.client(t).ImportPackageSelection(ctx, ImportPackageSelectionRequest{
		ProjectID:        "p",
		WorkflowIDs:      []string{"a"},
		WorkflowIDPolicy: ImportWorkflowIDSource,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}
