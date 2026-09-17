package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const communityPackageBody = `{
  "packageName":"n8n-nodes-example",
  "installedVersion":"1.2.3",
  "authorName":"Example Author",
  "authorEmail":"author@example.com",
  "installedNodes":[{"name":"Example","type":"n8n-nodes-example.example","latestVersion":2}],
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "updateAvailable":"1.3.0",
  "failedLoading":false
}`

type communityPackageServer struct {
	*httptest.Server
	requests []*http.Request
	bodies   []string
}

func newCommunityPackageServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *communityPackageServer {
	t.Helper()
	s := &communityPackageServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		s.requests = append(s.requests, r.Clone(r.Context()))
		s.bodies = append(s.bodies, string(body))
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func packageJSONHandler(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func (s *communityPackageServer) client(t *testing.T) *Client {
	t.Helper()
	auth, err := NewAPIKeyAuth("community-secret")
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}
	client, err := New(s.URL, WithAuth(auth))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func (s *communityPackageServer) last(t *testing.T) (*http.Request, string) {
	t.Helper()
	if len(s.requests) == 0 {
		t.Fatal("no request reached the instance")
	}
	return s.requests[len(s.requests)-1], s.bodies[len(s.bodies)-1]
}

func TestListCommunityPackages(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `[`+communityPackageBody+`]`))

	packages, err := server.client(t).ListCommunityPackages(context.Background())
	if err != nil {
		t.Fatalf("ListCommunityPackages: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", req.Method)
	}
	if want := BasePath + CommunityPackagesPath; req.URL.Path != want {
		t.Errorf("path = %q, want %q", req.URL.Path, want)
	}
	if req.URL.RawQuery != "" || body != "" {
		t.Errorf("query, body = %q, %q; want both empty", req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}

	want := []CommunityPackage{{
		PackageName:      "n8n-nodes-example",
		InstalledVersion: "1.2.3",
		AuthorName:       "Example Author",
		AuthorEmail:      "author@example.com",
		InstalledNodes: []CommunityPackageNode{{
			Name: "Example", Type: "n8n-nodes-example.example", LatestVersion: 2,
		}},
		CreatedAt:       "2026-01-01T00:00:00.000Z",
		UpdatedAt:       "2026-01-02T00:00:00.000Z",
		UpdateAvailable: "1.3.0",
	}}
	if diff := cmp.Diff(want, packages); diff != "" {
		t.Errorf("packages mismatch (-want +got):\n%s", diff)
	}
}

func TestListCommunityPackagesEmpty(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `[]`))
	packages, err := server.client(t).ListCommunityPackages(context.Background())
	if err != nil {
		t.Fatalf("ListCommunityPackages: %v", err)
	}
	if len(packages) != 0 {
		t.Errorf("packages = %+v, want empty", packages)
	}
}

func TestInstallCommunityPackage(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, communityPackageBody))
	request := InstallCommunityPackageRequest{Name: "n8n-nodes-example", Version: "1.2.3", Verify: Bool(false)}

	installed, err := server.client(t).InstallCommunityPackage(context.Background(), request)
	if err != nil {
		t.Fatalf("InstallCommunityPackage: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+CommunityPackagesPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+CommunityPackagesPath)
	}
	if req.Header.Get("Content-Type") != contentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", req.Header.Get("Content-Type"), contentTypeJSON)
	}
	assertJSONEqual(t, `{"name":"n8n-nodes-example","version":"1.2.3","verify":false}`, body)
	if installed.PackageName != request.Name || installed.InstalledVersion != request.Version {
		t.Errorf("installed = %+v, want requested package and version", installed)
	}
}

func TestInstallCommunityPackageOptionalFields(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, communityPackageBody))
	_, err := server.client(t).InstallCommunityPackage(context.Background(), InstallCommunityPackageRequest{Name: "n8n-nodes-example"})
	if err != nil {
		t.Fatalf("InstallCommunityPackage: %v", err)
	}
	_, body := server.last(t)
	assertJSONEqual(t, `{"name":"n8n-nodes-example"}`, body)
}

func TestUpdateCommunityPackage(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, communityPackageBody))
	name := "@scope/n8n-nodes-example?channel=next#node"
	request := UpdateCommunityPackageRequest{Version: "1.3.0", Verify: Bool(false)}

	updated, err := server.client(t).UpdateCommunityPackage(context.Background(), name, request)
	if err != nil {
		t.Fatalf("UpdateCommunityPackage: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", req.Method)
	}
	wantPath := BasePath + "/community-packages/@scope%2Fn8n-nodes-example%3Fchannel=next%23node"
	if req.URL.EscapedPath() != wantPath {
		t.Errorf("escaped path = %q, want %q", req.URL.EscapedPath(), wantPath)
	}
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty; package punctuation belongs in the path", req.URL.RawQuery)
	}
	assertJSONEqual(t, `{"version":"1.3.0","verify":false}`, body)
	if updated.PackageName != "n8n-nodes-example" {
		t.Errorf("updated package = %+v", updated)
	}
}

func TestUpdateCommunityPackageOmittedOptions(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, communityPackageBody))
	_, err := server.client(t).UpdateCommunityPackage(context.Background(), "n8n-nodes-example", UpdateCommunityPackageRequest{})
	if err != nil {
		t.Fatalf("UpdateCommunityPackage: %v", err)
	}
	req, body := server.last(t)
	if body != "" {
		t.Errorf("body = %q, want none for zero options", body)
	}
	if req.Header.Get("Content-Type") != "" {
		t.Errorf("Content-Type = %q, want none without a body", req.Header.Get("Content-Type"))
	}
}

func TestUninstallCommunityPackage(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	name := "@scope/n8n-nodes-example/name with space"

	if err := server.client(t).UninstallCommunityPackage(context.Background(), name); err != nil {
		t.Fatalf("UninstallCommunityPackage: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", req.Method)
	}
	wantPath := BasePath + "/community-packages/@scope%2Fn8n-nodes-example%2Fname%20with%20space"
	if req.URL.EscapedPath() != wantPath {
		t.Errorf("escaped path = %q, want %q", req.URL.EscapedPath(), wantPath)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestCommunityPackageErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		call   func(*Client) error
		method string
		path   string
	}{
		{name: "list unauthorized", status: http.StatusUnauthorized, call: func(c *Client) error { _, err := c.ListCommunityPackages(context.Background()); return err }, method: http.MethodGet, path: CommunityPackagesPath},
		{name: "install bad request", status: http.StatusBadRequest, call: func(c *Client) error {
			_, err := c.InstallCommunityPackage(context.Background(), InstallCommunityPackageRequest{Name: "n8n-nodes-example"})
			return err
		}, method: http.MethodPost, path: CommunityPackagesPath},
		{name: "update not found", status: http.StatusNotFound, call: func(c *Client) error {
			_, err := c.UpdateCommunityPackage(context.Background(), "n8n-nodes-missing", UpdateCommunityPackageRequest{})
			return err
		}, method: http.MethodPatch, path: "/community-packages/n8n-nodes-missing"},
		{name: "uninstall not found", status: http.StatusNotFound, call: func(c *Client) error { return c.UninstallCommunityPackage(context.Background(), "n8n-nodes-missing") }, method: http.MethodDelete, path: "/community-packages/n8n-nodes-missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, `{"message":"package error"}`))
			err := tt.call(server.client(t))
			if err == nil || !IsStatus(err, tt.status) {
				t.Fatalf("error = %v, want status %d", err, tt.status)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.Method != tt.method || apiErr.Path != tt.path {
				t.Errorf("error request = %s %s, want %s %s", apiErr.Method, apiErr.Path, tt.method, tt.path)
			}
		})
	}
}

func TestCommunityPackageValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
		want string
	}{
		{name: "install missing name", call: func(c *Client) error {
			_, err := c.InstallCommunityPackage(context.Background(), InstallCommunityPackageRequest{})
			return err
		}, want: "name is required"},
		{name: "install wrong prefix", call: func(c *Client) error {
			_, err := c.InstallCommunityPackage(context.Background(), InstallCommunityPackageRequest{Name: "example"})
			return err
		}, want: "must start"},
		{name: "install padded name", call: func(c *Client) error {
			_, err := c.InstallCommunityPackage(context.Background(), InstallCommunityPackageRequest{Name: " n8n-nodes-example"})
			return err
		}, want: "whitespace"},
		{name: "install padded version", call: func(c *Client) error {
			_, err := c.InstallCommunityPackage(context.Background(), InstallCommunityPackageRequest{Name: "n8n-nodes-example", Version: " 1.0.0"})
			return err
		}, want: "version"},
		{name: "update missing name", call: func(c *Client) error {
			_, err := c.UpdateCommunityPackage(context.Background(), "", UpdateCommunityPackageRequest{})
			return err
		}, want: "name is required"},
		{name: "update padded version", call: func(c *Client) error {
			_, err := c.UpdateCommunityPackage(context.Background(), "n8n-nodes-example", UpdateCommunityPackageRequest{Version: "1.0.0 "})
			return err
		}, want: "version"},
		{name: "uninstall padded name", call: func(c *Client) error { return c.UninstallCommunityPackage(context.Background(), " n8n-nodes-example") }, want: "whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, communityPackageBody))
			err := tt.call(server.client(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want text %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d requests reached server, want none", len(server.requests))
			}
		})
	}
}

func assertJSONEqual(t *testing.T, want, got string) {
	t.Helper()
	var wantValue, gotValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("bad expected JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("request body is not JSON: %v: %s", err, got)
	}
	if diff := cmp.Diff(wantValue, gotValue); diff != "" {
		t.Errorf("JSON mismatch (-want +got):\n%s", diff)
	}
}
