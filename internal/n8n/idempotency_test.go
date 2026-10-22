package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

const (
	idempotencyInProgressBody = `{"message":"A request with this Idempotency-Key is already in progress."}`
	idempotencyMismatchBody   = `{"message":"This Idempotency-Key was already used with a different request."}`
)

func TestValidateIdempotencyKey(t *testing.T) {
	for _, key := range []string{
		"a",
		"deploy-2026-10-22",
		"!~",
		"0b5d4a0e-8f1c-4c2e-9a77-3f7d2f1a9b10",
		strings.Repeat("k", MaxIdempotencyKeyLength),
	} {
		if err := ValidateIdempotencyKey(key); err != nil {
			t.Errorf("ValidateIdempotencyKey(%q) = %v, want nil", key, err)
		}
	}
	for name, key := range map[string]string{
		"empty":     "",
		"too long":  strings.Repeat("k", MaxIdempotencyKeyLength+1),
		"space":     "two words",
		"leading":   " key",
		"tab":       "key\t",
		"newline":   "key\n",
		"delete":    "key\x7f",
		"non-ascii": "clé",
	} {
		if err := ValidateIdempotencyKey(key); err == nil {
			t.Errorf("%s: ValidateIdempotencyKey(%q) = nil, want error", name, key)
		}
	}
}

func TestNewIdempotencyKeyIsRandomUUIDv4(t *testing.T) {
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	first, second := NewIdempotencyKey(), NewIdempotencyKey()
	for _, key := range []string{first, second} {
		if !uuidV4.MatchString(key) {
			t.Errorf("NewIdempotencyKey() = %q, want a version 4 UUID", key)
		}
		if err := ValidateIdempotencyKey(key); err != nil {
			t.Errorf("ValidateIdempotencyKey(%q) = %v", key, err)
		}
	}
	if first == second {
		t.Errorf("two keys are both %q, want distinct keys", first)
	}
}

// TestIdempotencyKeyHeader sends every call that accepts the option once with
// a key and once without: the header appears only when set, and the body is
// the same either way.
func TestIdempotencyKeyHeader(t *testing.T) {
	message := "sync"
	source := selectionSource()
	filter := RowFilter{Filters: []RowCondition{{ColumnName: "email", Condition: "eq", Value: json.RawMessage(`"a@example.com"`)}}}
	applied := `{"status":"applied","counts":{},"warnings":[]}`
	cases := []struct {
		name, path, response string
		call                 func(context.Context, *Client, ...CallOption) error
	}{
		{"execution stop many", "/executions/stop", `{"stopped":1}`, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.StopManyExecutions(ctx, StopManyExecutionsRequest{Status: []string{"running"}, WorkflowID: "wf-1"}, opts...)
			return err
		}},
		{"execution stop", "/executions/1000/stop", `{"status":"canceled"}`, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.StopExecution(ctx, "1000", opts...)
			return err
		}},
		{"execution retry", "/executions/1000/retry", executionResponse, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.RetryExecution(ctx, "1000", RetryExecutionOptions{LoadWorkflow: true}, opts...)
			return err
		}},
		{"workflow create", "/workflows", `{"id":"wf-1","name":"Invoice sync"}`, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.CreateWorkflow(ctx, writeDocument(), opts...)
			return err
		}},
		{"row insert", "/data-tables/dt-1/rows", `{"success":true,"insertedRows":1}`, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.InsertDataTableRows(ctx, "dt-1", InsertRowsRequest{Data: []DataTableRow{{"age": json.RawMessage(`30`)}}}, opts...)
			return err
		}},
		{"row upsert", "/data-tables/dt-1/rows/upsert", `true`, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.UpsertDataTableRow(ctx, "dt-1", UpsertRowRequest{Filter: filter, Data: DataTableRow{"age": json.RawMessage(`31`)}}, opts...)
			return err
		}},
		{"promote", "/promotions/connections/conn-1/promote", promotionPromoteResultBody, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.PromotePackage(ctx, "conn-1", PromotePromotionRequest{CommitMessage: "sync"}, opts...)
			return err
		}},
		{"apply", "/promotions/connections/conn-1/apply", applied, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.ApplyPackage(ctx, "conn-1", opts...)
			return err
		}},
		{"apply continue", "/promotions/connections/conn-1/apply/continue", applied, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.ContinueApplyPackage(ctx, "conn-1", ContinueApplyPromotionRequest{ExpectedSource: source}, opts...)
			return err
		}},
		{"project apply", "/promotions/projects/proj-1/apply", applied, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.ApplyProjectSelection(ctx, "proj-1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}}, opts...)
			return err
		}},
		{"project apply continue", "/promotions/projects/proj-1/apply/continue", applied, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.ContinueApplyProjectSelection(ctx, "proj-1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}, ExpectedSource: source}, opts...)
			return err
		}},
		{"project promote", "/promotions/projects/proj-1/promote", promotionPromoteResultBody, func(ctx context.Context, c *Client, opts ...CallOption) error {
			_, err := c.PromoteProjectSelection(ctx, "proj-1", PromoteProjectSelectionRequest{WorkflowIDs: []string{"wf"}, CommitMessage: &message}, opts...)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tc.response))
			client := server.client(t)

			if err := tc.call(context.Background(), client); err != nil {
				t.Fatalf("without key: %v", err)
			}
			req, plainBody := server.last(t)
			if values, sent := req.Header[IdempotencyKeyHeader]; sent {
				t.Errorf("without key: %s = %q, want no header", IdempotencyKeyHeader, values)
			}

			if err := tc.call(context.Background(), client, IdempotencyKey("retry-key-1")); err != nil {
				t.Fatalf("with key: %v", err)
			}
			req, keyedBody := server.last(t)
			if req.Method != http.MethodPost || req.URL.EscapedPath() != BasePath+tc.path {
				t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.EscapedPath(), BasePath+tc.path)
			}
			if got := req.Header.Get(IdempotencyKeyHeader); got != "retry-key-1" {
				t.Errorf("%s = %q, want retry-key-1", IdempotencyKeyHeader, got)
			}
			if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
				t.Errorf("%s = %q, want the API key next to the idempotency key", HeaderAPIKey, got)
			}
			if keyedBody != plainBody {
				t.Errorf("body with key = %q, without = %q; the key must not change the body", keyedBody, plainBody)
			}
		})
	}
}

func TestIdempotencyKeyValidatedBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	for name, call := range map[string]func() error{
		"Do": func() error {
			_, err := client.Do(context.Background(), Request{Method: http.MethodPost, Path: "/tags", IdempotencyKey: "has space"}, nil)
			return err
		},
		"method": func() error {
			_, err := client.StopExecution(context.Background(), "1000", IdempotencyKey(strings.Repeat("k", MaxIdempotencyKeyLength+1)))
			return err
		},
		"package import": func() error {
			_, err := client.ImportPackage(context.Background(), ImportPackageRequest{
				WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource,
			}, strings.NewReader(packageArchiveBytes), "workflows.n8np", IdempotencyKey("clé"))
			return err
		},
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "idempotency key") {
			t.Errorf("%s: err = %v, want an idempotency key validation error", name, err)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d requests reached the instance, want none", len(server.requests))
	}
}

func TestIdempotencyRefusals(t *testing.T) {
	cases := []struct {
		name                 string
		status               int
		body                 string
		inProgress, mismatch bool
	}{
		{"in progress", http.StatusConflict, idempotencyInProgressBody, true, false},
		{"mismatch", http.StatusUnprocessableEntity, idempotencyMismatchBody, false, true},
		{"resource conflict", http.StatusConflict, `{"message":"Execution is not in a stoppable state"}`, false, false},
		{"validation failure", http.StatusUnprocessableEntity, `{"message":"request/body/name must be string"}`, false, false},
		{"key message on another status", http.StatusBadRequest, `{"message":"Idempotency-Key must be 1 to 128 visible ASCII characters."}`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
			_, err := server.client(t).StopExecution(context.Background(), "1000", IdempotencyKey("key-1"))
			if err == nil {
				t.Fatal("err = nil, want an API error")
			}
			if got := IsIdempotencyInProgress(err); got != tc.inProgress {
				t.Errorf("IsIdempotencyInProgress = %t, want %t", got, tc.inProgress)
			}
			if got := IsIdempotencyMismatch(err); got != tc.mismatch {
				t.Errorf("IsIdempotencyMismatch = %t, want %t", got, tc.mismatch)
			}
		})
	}
	if IsIdempotencyInProgress(errors.New("409 A request with this Idempotency-Key is already in progress.")) {
		t.Error("IsIdempotencyInProgress matched a plain error, want APIError only")
	}
}

func TestImportPackageIdempotencyKey(t *testing.T) {
	request := ImportPackageRequest{WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource}
	selection := ImportPackageSelectionRequest{ProjectID: "p1", WorkflowIDs: []string{"wf-1"}, WorkflowIDPolicy: ImportWorkflowIDSource}
	calls := map[string]func(*Client, ...CallOption) error{
		"import": func(c *Client, opts ...CallOption) error {
			_, err := c.ImportPackage(context.Background(), request, strings.NewReader(packageArchiveBytes), "workflows.n8np", opts...)
			return err
		},
		"import selection": func(c *Client, opts ...CallOption) error {
			_, err := c.ImportPackageSelection(context.Background(), selection, strings.NewReader(packageArchiveBytes), "project.n8np", opts...)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, packageImportResultJSON))
			client := server.client(t)

			if err := call(client); err != nil {
				t.Fatalf("without key: %v", err)
			}
			req, _ := server.last(t)
			if values, sent := req.Header[IdempotencyKeyHeader]; sent {
				t.Errorf("without key: %s = %q, want no header", IdempotencyKeyHeader, values)
			}

			// A retry must send the same bytes, so the boundary may not be random.
			var bodies []string
			for range 2 {
				if err := call(client, IdempotencyKey("import-1")); err != nil {
					t.Fatalf("with key: %v", err)
				}
				req, body := server.last(t)
				if got := req.Header.Get(IdempotencyKeyHeader); got != "import-1" {
					t.Errorf("%s = %q, want import-1", IdempotencyKeyHeader, got)
				}
				fields, fileBytes, _ := parseMultipartImport(t, req, []byte(body))
				if fileBytes != packageArchiveBytes || len(fields) == 0 {
					t.Errorf("multipart = %v fields and %q, want the policies and the archive", fields, fileBytes)
				}
				bodies = append(bodies, req.Header.Get("Content-Type")+"\n"+body)
			}
			if bodies[0] != bodies[1] {
				t.Error("two requests with the same key sent different bytes, want identical retries")
			}
			if err := call(client, IdempotencyKey("import-2")); err != nil {
				t.Fatalf("with second key: %v", err)
			}
			req, body := server.last(t)
			if req.Header.Get("Content-Type")+"\n"+body == bodies[0] {
				t.Error("a different key reused the same boundary, want one derived from the key")
			}
		})
	}
}

func TestImportPackageIdempotencyRefusalIsNotImportBlocked(t *testing.T) {
	for status, body := range map[int]string{
		http.StatusConflict:            idempotencyInProgressBody,
		http.StatusUnprocessableEntity: idempotencyMismatchBody,
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(status, body))
			_, err := server.client(t).ImportPackage(context.Background(), ImportPackageRequest{
				WorkflowConflictPolicy: ImportWorkflowConflictFail, WorkflowIDPolicy: ImportWorkflowIDSource,
			}, strings.NewReader(packageArchiveBytes), "workflows.n8np", IdempotencyKey("import-1"))
			if _, blocked := AsImportBlocked(err); blocked {
				t.Errorf("err = %v, want an API error, not an import block", err)
			}
			if !IsIdempotencyInProgress(err) && !IsIdempotencyMismatch(err) {
				t.Errorf("err = %v, want an idempotency refusal", err)
			}
		})
	}
}
