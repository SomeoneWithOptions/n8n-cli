package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

const credentialResponse = `{
  "id":"cred/one?x=1",
  "name":"GitHub production",
  "type":"githubApi",
  "isManaged":false,
  "isGlobal":false,
  "isResolvable":true,
  "resolvableAllowFallback":false,
  "resolverId":null,
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "data":{"accessToken":"server-secret-that-must-be-dropped"}
}`

func mustCredentialData(t *testing.T, raw string) CredentialData {
	t.Helper()
	data, err := NewCredentialData([]byte(raw))
	if err != nil {
		t.Fatalf("NewCredentialData: %v", err)
	}
	return data
}

func TestCredentialDataRedactsEveryFormattingSurface(t *testing.T) {
	const secret = "credential-secret-never-print"
	data := mustCredentialData(t, `{"token":"`+secret+`"}`)
	request := CreateCredentialRequest{Name: "name", Type: "type", Data: data}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var logged strings.Builder
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	logger.Info("data", "value", data)
	for name, got := range map[string]string{
		"String": data.String(), "GoString": fmt.Sprintf("%#v", data),
		"format": fmt.Sprintf("%v", data), "JSON": string(encoded), "log": logged.String(),
	} {
		if strings.Contains(got, secret) {
			t.Errorf("%s leaked secret: %s", name, got)
		}
		if !strings.Contains(got, Redacted) {
			t.Errorf("%s = %q, want %q", name, got, Redacted)
		}
	}
}

func TestCredentialDataValidation(t *testing.T) {
	for _, raw := range []string{"", "null", `[]`, `"secret"`, `{broken`} {
		if _, err := NewCredentialData([]byte(raw)); err == nil {
			t.Errorf("NewCredentialData(%q) succeeded, want error", raw)
		}
	}
	if _, err := NewCredentialData([]byte(`{}`)); err != nil {
		t.Errorf("empty object rejected: %v", err)
	}
}

func TestListCredentials(t *testing.T) {
	body := `{"data":[` + credentialResponse + `],"nextCursor":"next one"}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))

	page, err := server.client(t).ListCredentials(context.Background(), ListOptions{Limit: 25, Cursor: "start one"})
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	req, requestBody := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+CredentialsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+CredentialsPath)
	}
	if req.URL.Query().Get("limit") != "25" || req.URL.Query().Get("cursor") != "start one" {
		t.Errorf("query = %q, want pagination", req.URL.RawQuery)
	}
	if requestBody != "" {
		t.Errorf("body = %q, want empty", requestBody)
	}
	if len(page.Data) != 1 || page.Data[0].Name != "GitHub production" || page.NextCursor != "next one" {
		t.Errorf("page = %+v", page)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "server-secret") || strings.Contains(string(encoded), "accessToken") {
		t.Errorf("list response leaked credential data: %s", encoded)
	}
}

func TestCreateCredentialSendsSecretButNeverReturnsOrReportsIt(t *testing.T) {
	const secret = "create-secret-never-report"
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, credentialResponse))
	request := CreateCredentialRequest{
		Name: "GitHub production", Type: "githubApi",
		Data:      mustCredentialData(t, `{"accessToken":"`+secret+`"}`),
		ProjectID: "project-one", IsResolvable: Bool(false),
	}
	created, err := server.client(t).CreateCredential(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+CredentialsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	assertJSONEqual(t, `{"name":"GitHub production","type":"githubApi","data":{"accessToken":"`+secret+`"},"projectId":"project-one","isResolvable":false}`, body)
	encoded, _ := json.Marshal(created)
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "server-secret") {
		t.Errorf("create output leaked secret: %s", encoded)
	}
}

func TestGetUpdateDeleteTestSchemaAndTransferCredential(t *testing.T) {
	const secret = "update-secret-never-report"
	tests := []struct {
		name     string
		response string
		call     func(*Client) error
		method   string
		escaped  string
		body     string
	}{
		{name: "get", response: credentialResponse, call: func(c *Client) error {
			credential, err := c.GetCredential(context.Background(), "cred/one?x=1")
			if err == nil {
				encoded, _ := json.Marshal(credential)
				got := string(encoded)
				if strings.Contains(got, "server-secret") || strings.Contains(got, `"data"`) {
					t.Errorf("get leaked secret: %s", got)
				}
			}
			return err
		}, method: http.MethodGet, escaped: "/credentials/cred%2Fone%3Fx=1"},
		{name: "update", response: credentialResponse, call: func(c *Client) error {
			data := mustCredentialData(t, `{"accessToken":"`+secret+`"}`)
			name := "renamed"
			_, err := c.UpdateCredential(context.Background(), "cred/one?x=1", UpdateCredentialRequest{Name: &name, Data: &data, IsPartialData: Bool(true)})
			return err
		}, method: http.MethodPatch, escaped: "/credentials/cred%2Fone%3Fx=1", body: `{"name":"renamed","data":{"accessToken":"` + secret + `"},"isPartialData":true}`},
		{name: "delete", response: credentialResponse, call: func(c *Client) error {
			deleted, err := c.DeleteCredential(context.Background(), "cred/one?x=1")
			if err == nil {
				encoded, _ := json.Marshal(deleted)
				if strings.Contains(string(encoded), "server-secret") {
					t.Errorf("delete leaked secret: %s", encoded)
				}
			}
			return err
		}, method: http.MethodDelete, escaped: "/credentials/cred%2Fone%3Fx=1"},
		{name: "test", response: `{"status":"OK","message":"Connection succeeded"}`, call: func(c *Client) error {
			result, err := c.TestCredential(context.Background(), "cred/one?x=1")
			if err == nil && result.Status != "OK" {
				t.Errorf("result = %+v", result)
			}
			return err
		}, method: http.MethodPost, escaped: "/credentials/cred%2Fone%3Fx=1/test"},
		{name: "schema", response: `{"type":"object","properties":{"token":{"type":"string"}}}`, call: func(c *Client) error {
			schema, err := c.CredentialSchema(context.Background(), "custom/type?x=1")
			if err == nil && !strings.Contains(string(schema), "properties") {
				t.Errorf("schema = %s", schema)
			}
			return err
		}, method: http.MethodGet, escaped: "/credentials/schema/custom%2Ftype%3Fx=1"},
		{name: "transfer", response: "", call: func(c *Client) error {
			return c.TransferCredential(context.Background(), "cred/one?x=1", "project/two?x=1")
		}, method: http.MethodPut, escaped: "/credentials/cred%2Fone%3Fx=1/transfer", body: `{"destinationProjectId":"project/two?x=1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tt.response))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.escaped {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.escaped)
			}
			if tt.body == "" {
				if body != "" {
					t.Errorf("body = %q, want empty", body)
				}
			} else {
				assertJSONEqual(t, tt.body, body)
			}
		})
	}
}

func TestCredentialMutationErrorRedaction(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "update"}[update], func(t *testing.T) {
			const secret = "echoed-credential-secret"
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"bad secret `+secret+`"}`))
			data := mustCredentialData(t, `{"token":"`+secret+`"}`)
			var err error
			if update {
				_, err = server.client(t).UpdateCredential(context.Background(), "id", UpdateCredentialRequest{Data: &data})
			} else {
				_, err = server.client(t).CreateCredential(context.Background(), CreateCredentialRequest{Name: "name", Type: "type", Data: data})
			}
			if err == nil || !IsStatus(err, http.StatusBadRequest) {
				t.Fatalf("error = %v, want 400", err)
			}
			if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "details redacted") {
				t.Errorf("error = %v, want redaction", err)
			}
		})
	}
}

func TestCredentialValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, credentialResponse))
	client := server.client(t)
	calls := []func() error{
		func() error {
			_, err := client.ListCredentials(context.Background(), ListOptions{Limit: -1})
			return err
		},
		func() error {
			_, err := client.CreateCredential(context.Background(), CreateCredentialRequest{})
			return err
		},
		func() error { _, err := client.GetCredential(context.Background(), " "); return err },
		func() error {
			_, err := client.UpdateCredential(context.Background(), "id ", UpdateCredentialRequest{})
			return err
		},
		func() error { _, err := client.DeleteCredential(context.Background(), ""); return err },
		func() error { _, err := client.TestCredential(context.Background(), " id"); return err },
		func() error { _, err := client.CredentialSchema(context.Background(), ""); return err },
		func() error { return client.TransferCredential(context.Background(), "id", " ") },
	}
	for i, call := range calls {
		if err := call(); err == nil {
			t.Errorf("call %d succeeded, want validation error", i)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d invalid requests reached server", len(server.requests))
	}
}
