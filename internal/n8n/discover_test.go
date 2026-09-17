package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const discoverBody = `{
  "data": {
    "scopes": ["workflow:read", "workflow:list"],
    "resources": {
      "workflow": {
        "operations": ["read", "list"],
        "endpoints": [
          {"method": "GET", "path": "/api/v1/workflows", "operationId": "getWorkflows"},
          {"method": "GET", "path": "/api/v1/workflows/{id}", "operationId": "getWorkflow"}
        ]
      },
      "audit": {
        "operations": ["generate"],
        "endpoints": [
          {"method": "POST", "path": "/api/v1/audit", "operationId": "generateAudit",
           "requestSchema": {"type": "object"}}
        ]
      }
    },
    "filters": {
      "include": {"description": "Include additional data", "values": ["schemas"]}
    },
    "specUrl": "/api/v1/openapi.yml"
  }
}`

// discoverServer records the single request it receives and answers with body.
func discoverServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	var got http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(r.Context())
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &got
}

func TestDiscover(t *testing.T) {
	server, got := discoverServer(t, http.StatusOK, discoverBody)

	auth, err := NewAPIKeyAuth("secret-key")
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}
	client, err := New(server.URL, WithAuth(auth))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	discovery, err := client.Discover(context.Background(), DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want %q", got.Method, http.MethodGet)
	}
	if want := BasePath + DiscoverPath; got.URL.Path != want {
		t.Errorf("path = %q, want %q", got.URL.Path, want)
	}
	if got.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty for the zero options", got.URL.RawQuery)
	}
	if key := got.Header.Get(HeaderAPIKey); key != "secret-key" {
		t.Errorf("%s = %q, want the API key", HeaderAPIKey, key)
	}
	if got.Header.Get("Authorization") != "" {
		t.Error("Authorization is set; exactly one credential must be sent")
	}
	if accept := got.Header.Get("Accept"); accept != contentTypeJSON {
		t.Errorf("Accept = %q, want %q", accept, contentTypeJSON)
	}

	want := &Discovery{
		Scopes: []string{"workflow:read", "workflow:list"},
		Resources: map[string]DiscoveredResource{
			"workflow": {
				Operations: []string{"read", "list"},
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/api/v1/workflows", OperationID: "getWorkflows"},
					{Method: "GET", Path: "/api/v1/workflows/{id}", OperationID: "getWorkflow"},
				},
			},
			"audit": {
				Operations: []string{"generate"},
				Endpoints: []DiscoveredEndpoint{
					{Method: "POST", Path: "/api/v1/audit", OperationID: "generateAudit",
						RequestSchema: json.RawMessage(`{"type": "object"}`)},
				},
			},
		},
		Filters: map[string]DiscoverFilter{
			"include": {Description: "Include additional data", Values: []string{"schemas"}},
		},
		SpecURL: "/api/v1/openapi.yml",
	}
	if diff := cmp.Diff(want, discovery); diff != "" {
		t.Errorf("Discover() mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverQueryEncoding(t *testing.T) {
	tests := []struct {
		name string
		opts DiscoverOptions
		want string
	}{
		{name: "none", opts: DiscoverOptions{}, want: ""},
		{name: "include", opts: DiscoverOptions{Include: DiscoverIncludeSchemas}, want: "include=schemas"},
		{name: "resource", opts: DiscoverOptions{Resource: "workflow"}, want: "resource=workflow"},
		{name: "operation", opts: DiscoverOptions{Operation: "read"}, want: "operation=read"},
		{
			name: "all, sorted",
			opts: DiscoverOptions{Include: DiscoverIncludeSchemas, Resource: "data table", Operation: "list"},
			want: "include=schemas&operation=list&resource=data+table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, got := discoverServer(t, http.StatusOK, discoverBody)
			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := client.Discover(context.Background(), tt.opts); err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if got.URL.RawQuery != tt.want {
				t.Errorf("query = %q, want %q", got.URL.RawQuery, tt.want)
			}
		})
	}
}

func TestDiscoverOptionsApplyKeepsExistingQuery(t *testing.T) {
	q := DiscoverOptions{Resource: "workflow"}.Apply(url.Values{"keep": {"1"}})
	if got, want := q.Encode(), "keep=1&resource=workflow"; got != want {
		t.Errorf("Apply() = %q, want %q", got, want)
	}
}

func TestDiscoverRejectsUnknownInclude(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.Discover(context.Background(), DiscoverOptions{Include: "everything"})
	if err == nil {
		t.Fatal("Discover() = nil, want an error for an undefined include value")
	}
	if !strings.Contains(err.Error(), "schemas") {
		t.Errorf("error = %q, want it to name the accepted value", err)
	}
	if called {
		t.Error("the request was sent; an invalid include must fail before the call")
	}
}

func TestDiscoverErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr func(error) bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`, wantErr: IsUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"forbidden"}`, wantErr: IsForbidden},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`, wantErr: IsNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := discoverServer(t, tt.status, tt.body)
			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			discovery, err := client.Discover(context.Background(), DiscoverOptions{})
			if !tt.wantErr(err) {
				t.Fatalf("Discover() = %v, want the documented status", err)
			}
			if discovery != nil {
				t.Errorf("Discover() = %+v, want nil on error", discovery)
			}
		})
	}
}

func TestDiscoverEmptyResponse(t *testing.T) {
	// A proxy that answers 204, or a body-less 200, must not crash the caller.
	for name, status := range map[string]int{"no content": http.StatusNoContent, "empty 200": http.StatusOK} {
		t.Run(name, func(t *testing.T) {
			server, _ := discoverServer(t, status, "")
			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			discovery, err := client.Discover(context.Background(), DiscoverOptions{})
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if len(discovery.Scopes) != 0 || len(discovery.Resources) != 0 {
				t.Errorf("Discover() = %+v, want the zero capability map", discovery)
			}
		})
	}
}

func TestDiscoverMalformedJSON(t *testing.T) {
	server, _ := discoverServer(t, http.StatusOK, `{"data": {`)
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Discover(context.Background(), DiscoverOptions{}); err == nil {
		t.Fatal("Discover() = nil, want a decode error")
	}
}

func TestDiscoverCancellation(t *testing.T) {
	server, _ := discoverServer(t, http.StatusOK, discoverBody)
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Discover(ctx, DiscoverOptions{}); !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Discover() = %v, want a cancellation error", err)
	}
}

func TestDiscoveryLookups(t *testing.T) {
	var envelope discoverEnvelope
	if err := json.Unmarshal([]byte(discoverBody), &envelope); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	d := &envelope.Data

	if !d.HasScope("workflow:read") {
		t.Error("HasScope(workflow:read) = false, want true")
	}
	if d.HasScope("workflow:delete") {
		t.Error("HasScope(workflow:delete) = true, want false")
	}
	if got, want := d.ResourceNames(), []string{"audit", "workflow"}; !cmp.Equal(got, want) {
		t.Errorf("ResourceNames() = %v, want %v", got, want)
	}
	if _, ok := d.Resource("workflow"); !ok {
		t.Error("Resource(workflow) not found")
	}
	if _, ok := d.Resource("credential"); ok {
		t.Error("Resource(credential) found, want missing")
	}
	if !d.HasOperation("workflow", "list") {
		t.Error("HasOperation(workflow, list) = false, want true")
	}
	if d.HasOperation("workflow", "delete") {
		t.Error("HasOperation(workflow, delete) = true, want false")
	}
	if d.HasOperation("nope", "list") {
		t.Error("HasOperation(nope, list) = true, want false")
	}
	endpoint, ok := d.Endpoint("get", "/api/v1/workflows/{id}")
	if !ok {
		t.Fatal("Endpoint(GET /api/v1/workflows/{id}) not found")
	}
	if endpoint.OperationID != "getWorkflow" {
		t.Errorf("operationId = %q, want getWorkflow", endpoint.OperationID)
	}
	if _, ok := d.Endpoint("DELETE", "/api/v1/workflows/{id}"); ok {
		t.Error("Endpoint(DELETE ...) found, want missing")
	}
}

// The API keys resources in lowercase; commands spell them with hyphens.
func TestDiscoveryResourceNameFolding(t *testing.T) {
	d := &Discovery{Resources: map[string]DiscoveredResource{"datatable": {Operations: []string{"list"}}}}
	for _, name := range []string{"datatable", "dataTable", "data-table", "DATA-TABLE"} {
		if _, ok := d.Resource(name); !ok {
			t.Errorf("Resource(%q) not found", name)
		}
	}
}
