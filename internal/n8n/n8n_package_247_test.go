package n8n

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// legacyImportServer answers like an n8n 2.46 instance: the 2.47 field names
// fail body validation with the first schema issue, the older names succeed.
func legacyImportServer(t *testing.T, rejection string) *communityPackageServer {
	t.Helper()
	var server *communityPackageServer
	server = newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		// The recording wrapper consumed the body before this handler runs.
		if strings.Contains(server.bodies[len(server.bodies)-1], `name="idPolicy"`) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(rejection))
			return
		}
		_, _ = w.Write([]byte(packageImportResultJSON))
	})
	return server
}

func TestImportPackageFallsBackToLegacyFieldNames(t *testing.T) {
	const missingRequired = `{"code":"invalid_type","expected":"'new-version' | 'fail' | 'skip'","received":"undefined","path":["workflowConflictPolicy"],"message":"Required"}`
	server := legacyImportServer(t, missingRequired)

	// The archive starts mid-reader: the retry must rewind to where it began,
	// not to the start of the file.
	archive := bytes.NewReader([]byte("prefix" + packageArchiveBytes))
	if _, err := archive.Seek(int64(len("prefix")), io.SeekStart); err != nil {
		t.Fatal(err)
	}
	_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		ConflictPolicy:   ImportConflictSkip,
		IDPolicy:         ImportIDSource,
		PublishingPolicy: ImportPublishAll,
	}, archive, "p.n8np", IdempotencyKey("retry-key"))
	if err != nil {
		t.Fatalf("ImportPackage: %v", err)
	}
	if len(server.requests) != 2 {
		t.Fatalf("requests = %d, want the 2.47 attempt and one legacy retry", len(server.requests))
	}
	first, _, _ := parseMultipartImport(t, server.requests[0], []byte(server.bodies[0]))
	if first["conflictPolicy"] != ImportConflictSkip || first["idPolicy"] != ImportIDSource || first["publishingPolicy"] != ImportPublishAll {
		t.Errorf("first fields = %v, want the 2.47 names", first)
	}
	second, fileBytes, _ := parseMultipartImport(t, server.requests[1], []byte(server.bodies[1]))
	for name, want := range map[string]string{
		"workflowConflictPolicy":   ImportConflictSkip,
		"workflowIdPolicy":         ImportIDSource,
		"workflowPublishingPolicy": ImportPublishAll,
	} {
		if second[name] != want {
			t.Errorf("retry %s = %q, want %q", name, second[name], want)
		}
	}
	for _, name := range []string{"conflictPolicy", "idPolicy", "publishingPolicy"} {
		if _, ok := second[name]; ok {
			t.Errorf("retry still sends %s; the 2.46 schema rejects it", name)
		}
	}
	if fileBytes != packageArchiveBytes {
		t.Errorf("retry archive = %q, want the full archive again", fileBytes)
	}
	for i, req := range server.requests {
		if got := req.Header.Get(IdempotencyKeyHeader); got != "retry-key" {
			t.Errorf("request %d %s = %q, want the same key", i, IdempotencyKeyHeader, got)
		}
	}
}

func TestImportPackageSelectionFallsBackOnUnrecognizedKeys(t *testing.T) {
	const unrecognized = `{"code":"unrecognized_keys","keys":["conflictPolicy","idPolicy"],"path":[],"message":"Unrecognized key(s) in object: 'conflictPolicy', 'idPolicy'"}`
	server := legacyImportServer(t, unrecognized)

	_, err := server.client(t).ImportPackageSelection(context.Background(), ImportPackageSelectionRequest{
		ProjectID:      "p1",
		WorkflowIDs:    []string{"wf-1"},
		ConflictPolicy: ImportConflictFail,
		IDPolicy:       ImportIDSource,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if err != nil {
		t.Fatalf("ImportPackageSelection: %v", err)
	}
	if len(server.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(server.requests))
	}
	second, _, _ := parseMultipartImport(t, server.requests[1], []byte(server.bodies[1]))
	if second["workflowConflictPolicy"] != ImportConflictFail || second["workflowIdPolicy"] != ImportIDSource || second["selectedProjectId"] != "p1" {
		t.Errorf("retry fields = %v, want legacy names and the selection", second)
	}
}

func TestImportPackageLegacyFallbackNeedsRewindableArchive(t *testing.T) {
	const missingRequired = `{"code":"invalid_type","path":["workflowConflictPolicy"],"message":"Required"}`
	server := legacyImportServer(t, missingRequired)

	// io.MultiReader hides Seek, like a pipe that cannot be read twice.
	archive := io.MultiReader(strings.NewReader(packageArchiveBytes))
	_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		ConflictPolicy: ImportConflictSkip,
		IDPolicy:       ImportIDSource,
	}, archive, "p.n8np")
	if !errors.Is(err, ErrImportArchiveNotRewindable) {
		t.Fatalf("error = %v, want ErrImportArchiveNotRewindable", err)
	}
	if !IsStatus(err, http.StatusBadRequest) {
		t.Errorf("error = %v, want the 400 kept in the chain", err)
	}
	if len(server.requests) != 1 {
		t.Errorf("requests = %d, want no retry", len(server.requests))
	}
}

func TestImportPackageDoesNotFallBackOnOtherBadRequests(t *testing.T) {
	for name, body := range map[string]string{
		"2.47 refuses idPolicy new": `{"code":"invalid_enum_value","options":["source"],"path":["idPolicy"],"message":"Invalid enum value. Expected 'source', received 'new'"}`,
		"bad bindings":              `{"code":"custom","path":["bindings"],"message":"bindings must be a JSON object"}`,
		"plain message":             `{"message":"package is malformed"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, body))
			_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
				ConflictPolicy: ImportConflictSkip,
				IDPolicy:       ImportIDNew,
			}, strings.NewReader(packageArchiveBytes), "p.n8np")
			if !IsStatus(err, http.StatusBadRequest) || errors.Is(err, ErrImportArchiveNotRewindable) {
				t.Fatalf("error = %v, want the plain 400", err)
			}
			if len(server.requests) != 1 {
				t.Errorf("requests = %d, want 1", len(server.requests))
			}
		})
	}
}

func TestImportPackageResultDecodesAgents(t *testing.T) {
	body := `{
  "package":{"sourceN8nVersion":"2.47.3","sourceId":"pkg-4"},
  "agents":[{"sourceAgentId":"ag-1","localId":"ag-1","name":"Helper","projectId":"p1","activeVersionId":null,"status":"created",
    "publishing":{"state":"blocked","blockedReason":"channel-setup-required"},
    "channels":[{"type":"slack","status":"setup-required","reason":"new-channel","setupUrl":"https://n8n.example/agents/ag-1/channels"},{"type":"chat","status":"reused"}]}],
  "counts":{"agents":{"created":1,"updated":0,"skipped":0}},
  "workflows":[],"removedWorkflows":[],"removedFolders":[],"folders":[],"projects":[],
  "bindings":{"agents":{"ag-1":"ag-1"},"workflows":{},"credentials":{}},
  "credentials":{"matched":[],"stubbed":[],"seeded":[],"incomplete":[]},
  "dataTables":{"matched":0,"created":0,"updated":0},
  "variables":{"matched":[],"missing":[],"created":[],"stubbed":[],"updated":[]},
  "tags":{"matched":[],"created":[],"renamed":[],"reconciled":[],"skipped":[]}
}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	result, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
		ConflictPolicy: ImportConflictSkip, IDPolicy: ImportIDSource,
	}, strings.NewReader(packageArchiveBytes), "p.n8np")
	if err != nil {
		t.Fatalf("ImportPackage: %v", err)
	}
	if len(result.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(result.Agents))
	}
	agent := result.Agents[0]
	if agent.Status != "created" || agent.ActiveVersionID != nil || agent.Publishing.BlockedReason != ImportPublishReasonChannelSetupRequired {
		t.Errorf("agent = %+v, want created, no active version, blocked on channel setup", agent)
	}
	if len(agent.Channels) != 2 || agent.Channels[0].Status != ImportChannelSetupRequired || agent.Channels[0].SetupURL == "" {
		t.Errorf("channels = %+v, want a setup-required channel with its URL", agent.Channels)
	}
	if result.Counts == nil || result.Counts.Agents.Created != 1 {
		t.Errorf("counts = %+v, want one created agent", result.Counts)
	}
	if result.Bindings.Agents["ag-1"] != "ag-1" {
		t.Errorf("bindings.agents = %v, want ag-1", result.Bindings.Agents)
	}
	if result.Package.ExportedAt != "" {
		t.Errorf("exportedAt = %q, want empty on 2.47", result.Package.ExportedAt)
	}
}

func TestImportBlockingIssueDescribe247(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []string
	}{
		{"policy-check-failed", `{"type":"policy-check-failed","point":"credentialSave"}`,
			[]string{"policy-check-failed: the credentialSave policy check could not run"}},
		{"data-table column-id-conflict", `{"type":"data-table-unresolved","kind":"column-id-conflict","sourceId":"dt-1","name":"Orders","column":{"id":"col-9","name":"total"}}`,
			[]string{`data-table-unresolved: kind="column-id-conflict" name="Orders" sourceId="dt-1"`, `(column "total", id "col-9")`}},
		{"credential-unresolved reason", `{"type":"credential-unresolved","sourceId":"c-1","reason":"access-required"}`,
			[]string{`reason="access-required"`}},
		{"project-conflict kind", `{"type":"project-conflict","kind":"create-forbidden","sourceProjectId":"p1"}`,
			[]string{`kind="create-forbidden"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var issue ImportBlockingIssue
			if err := issue.UnmarshalJSON([]byte(tc.raw)); err != nil {
				t.Fatal(err)
			}
			got := issue.Describe()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("Describe() = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestAPIErrorBadRequestFields(t *testing.T) {
	err := decodeError(t, http.StatusBadRequest, nil,
		`{"code":"unrecognized_keys","keys":["conflictPolicy","idPolicy"],"path":[],"message":"Unrecognized key(s) in object: 'conflictPolicy', 'idPolicy'"}`)
	if len(err.Fields) != 2 || err.Fields[0] != "conflictPolicy" || err.Fields[1] != "idPolicy" {
		t.Errorf("Fields = %v, want the unrecognized keys", err.Fields)
	}
	if !strings.Contains(err.Error(), `(field "conflictPolicy", "idPolicy")`) {
		t.Errorf("Error() = %q, want the fields named", err.Error())
	}
	err = decodeError(t, http.StatusBadRequest, nil, `{"code":"invalid_type","path":["workflowConflictPolicy",0],"message":"Required"}`)
	if len(err.Fields) != 1 || err.Fields[0] != "workflowConflictPolicy" {
		t.Errorf("Fields = %v, want the path head", err.Fields)
	}
	if got := decodeError(t, http.StatusNotFound, nil, `{"message":"x","path":["a"]}`).Fields; got != nil {
		t.Errorf("404 Fields = %v, want none", got)
	}
}

func TestAPIErrorRateLimit(t *testing.T) {
	header := http.Header{}
	header.Set("Retry-After", "30")
	header.Set("X-RateLimit-Limit", "100")
	header.Set("X-RateLimit-Remaining", "0")
	header.Set("X-RateLimit-Reset", "1793000000")
	err := decodeError(t, http.StatusTooManyRequests, header, `{"message":"Too many requests"}`)
	if !IsRateLimited(err) || err.RateLimit == nil {
		t.Fatalf("error = %#v, want a rate limit", err)
	}
	if d, ok := RetryAfterOf(err); !ok || d != 30*time.Second {
		t.Errorf("RetryAfterOf = %v, %v, want 30s", d, ok)
	}
	want := "retry after 30s; limit 100, 0 remaining; window resets at " + time.Unix(1793000000, 0).UTC().Format(time.RFC3339)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}

	date := http.Header{}
	date.Set("Retry-After", time.Now().Add(90*time.Second).UTC().Format(http.TimeFormat))
	if d, ok := RetryAfterOf(decodeError(t, http.StatusTooManyRequests, date, "")); !ok || d < 80*time.Second || d > 91*time.Second {
		t.Errorf("HTTP-date Retry-After = %v, %v, want about 90s", d, ok)
	}

	junk := http.Header{}
	junk.Set("Retry-After", "soon")
	junk.Set("X-RateLimit-Limit", "-3")
	if got := decodeError(t, http.StatusTooManyRequests, junk, "").RateLimit; got != nil {
		t.Errorf("RateLimit = %+v, want nil for malformed headers", got)
	}
	if _, ok := RetryAfterOf(errors.New("plain")); ok {
		t.Error("RetryAfterOf(plain error) = ok, want false")
	}
	if got := decodeError(t, http.StatusServiceUnavailable, header, "").RateLimit; got != nil {
		t.Errorf("503 RateLimit = %+v, want headers read only on 429", got)
	}
}
