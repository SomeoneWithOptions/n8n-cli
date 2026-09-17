package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// folderResponse is the shape the live instance returns for a list entry: the
// documented fields plus the relations and counts it adds on top of them.
const folderResponse = `{
  "id":"fo-1",
  "name":"Invoices",
  "parentFolderId":null,
  "createdAt":"2026-09-17T19:11:28.303Z",
  "updatedAt":"2026-09-17T19:11:28.303Z",
  "homeProject":{"id":"pr-1","name":"Personal","type":"personal"},
  "parentFolder":null,
  "tags":[{"id":"ta-1","name":"finance"}],
  "workflowCount":2,
  "subFolderCount":1,
  "fieldAddedLater":"ignored"
}`

func TestListFoldersQueryAuthAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"count":7,"data":[`+folderResponse+`]}`))
	page, err := server.client(t).ListFolders(context.Background(), "pr/one?x=1", ListFoldersOptions{
		Skip:   10,
		Take:   25,
		SortBy: "name:asc",
		Select: []string{"id", "name", "workflowCount"},
		Filter: FolderFilter{ParentFolderID: "fo-0", Name: "Inv", Tags: []string{"finance"}},
	})
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+"/projects/pr%2Fone%3Fx=1/folders" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	query := req.URL.Query()
	if query.Get("skip") != "10" || query.Get("take") != "25" || query.Get("sortBy") != "name:asc" {
		t.Errorf("pagination query = %q", req.URL.RawQuery)
	}
	// The colon in sortBy is a reserved character the API rejects unencoded.
	if strings.Contains(req.URL.RawQuery, "name:asc") {
		t.Errorf("sortBy is not percent-encoded: %q", req.URL.RawQuery)
	}
	assertJSONEqual(t, `["id","name","workflowCount"]`, query.Get("select"))
	assertJSONEqual(t, `{"parentFolderId":"fo-0","name":"Inv","tags":["finance"]}`, query.Get("filter"))

	if page.Count != 7 || len(page.Data) != 1 {
		t.Fatalf("page = %+v, want one folder of seven", page)
	}
	folder := page.Data[0]
	if folder.ID != "fo-1" || folder.Name != "Invoices" || folder.ParentFolderID != "" {
		t.Errorf("folder = %+v", folder)
	}
	if folder.HomeProject == nil || folder.HomeProject.ID != "pr-1" || folder.HomeProject.Type != "personal" {
		t.Errorf("home project = %+v", folder.HomeProject)
	}
	if len(folder.Tags) != 1 || folder.Tags[0].Name != "finance" {
		t.Errorf("tags = %+v", folder.Tags)
	}
	if folder.WorkflowCount == nil || *folder.WorkflowCount != 2 || folder.SubFolderCount == nil || *folder.SubFolderCount != 1 {
		t.Errorf("counts = %+v %+v", folder.WorkflowCount, folder.SubFolderCount)
	}
}

func TestListFoldersDefaultQuery(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"count":0,"data":[]}`))
	if _, err := server.client(t).ListFolders(context.Background(), "pr-1", ListFoldersOptions{}); err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if got := server.requests[0].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want empty", got)
	}
}

func TestListFoldersSelectedFieldsOnly(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK,
		`{"count":1,"data":[{"id":"fo-1","path":["Clients","Invoices"]}]}`))
	page, err := server.client(t).ListFolders(context.Background(), "pr-1", ListFoldersOptions{Select: []string{"id", "path"}})
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].Name != "" || len(page.Data[0].Path) != 2 || page.Data[0].Path[1] != "Invoices" {
		t.Errorf("page = %+v, want the selected fields only", page)
	}
}

func TestFolderRequests(t *testing.T) {
	projectID := "pr/one?x=1"
	folderID := "fo/two?y=2"
	tests := []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		query  string
		body   string
		status int
		result string
	}{
		{name: "create", call: func(c *Client) error {
			folder, err := c.CreateFolder(context.Background(), projectID, CreateFolderRequest{Name: "Invoices", ParentFolderID: "fo-0"})
			if err == nil && (folder.ID != "fo-1" || folder.Name != "Invoices") {
				t.Errorf("folder = %+v", folder)
			}
			return err
		}, method: http.MethodPost, path: "/projects/pr%2Fone%3Fx=1/folders",
			body: `{"name":"Invoices","parentFolderId":"fo-0"}`, status: http.StatusCreated, result: folderResponse},
		{name: "create at project root", call: func(c *Client) error {
			_, err := c.CreateFolder(context.Background(), "personal", CreateFolderRequest{Name: "Invoices"})
			return err
		}, method: http.MethodPost, path: "/projects/personal/folders",
			body: `{"name":"Invoices"}`, status: http.StatusCreated, result: folderResponse},
		{name: "get", call: func(c *Client) error {
			folder, err := c.GetFolder(context.Background(), projectID, folderID)
			if err == nil && (folder.TotalSubFolders != 3 || folder.TotalWorkflows != 4 || folder.Name != "Invoices") {
				t.Errorf("folder = %+v", folder)
			}
			return err
		}, method: http.MethodGet, path: "/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2", status: http.StatusOK,
			result: `{"id":"fo-1","name":"Invoices","parentFolderId":null,"totalSubFolders":3,"totalWorkflows":4}`},
		{name: "update", call: func(c *Client) error {
			folder, err := c.UpdateFolder(context.Background(), projectID, folderID, UpdateFolderRequest{Name: "Paid invoices"})
			if err == nil && folder.ID != "fo-1" {
				t.Errorf("folder = %+v", folder)
			}
			return err
		}, method: http.MethodPatch, path: "/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2",
			body: `{"name":"Paid invoices"}`, status: http.StatusOK, result: folderResponse},
		{name: "update moves folder", call: func(c *Client) error {
			_, err := c.UpdateFolder(context.Background(), projectID, folderID, UpdateFolderRequest{ParentFolderID: "fo-9"})
			return err
		}, method: http.MethodPatch, path: "/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2",
			body: `{"parentFolderId":"fo-9"}`, status: http.StatusOK, result: folderResponse},
		{name: "delete", call: func(c *Client) error {
			return c.DeleteFolder(context.Background(), projectID, folderID, "")
		}, method: http.MethodDelete, path: "/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2", status: http.StatusNoContent},
		{name: "delete with transfer target", call: func(c *Client) error {
			return c.DeleteFolder(context.Background(), projectID, folderID, "fo/three?z=3")
		}, method: http.MethodDelete, path: "/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2",
			query: "transferToFolderId=fo%2Fthree%3Fz%3D3", status: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, tt.result))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, gotBody := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.path {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.path)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if tt.body == "" {
				if gotBody != "" {
					t.Errorf("body = %q, want empty", gotBody)
				}
			} else {
				assertJSONEqual(t, tt.body, gotBody)
			}
		})
	}
}

func TestDeleteFolderToleratesEmptyBody(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := server.client(t).DeleteFolder(context.Background(), "pr-1", "fo-1", ""); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
}

func TestCollectFoldersWalksEveryOffsetPage(t *testing.T) {
	var skips []string
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, r *http.Request) {
		skips = append(skips, r.URL.Query().Get("skip"))
		w.Header().Set("Content-Type", contentTypeJSON)
		if r.URL.Query().Get("skip") == "" {
			_, _ = w.Write([]byte(`{"count":3,"data":[{"id":"fo-1"},{"id":"fo-2"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"count":3,"data":[{"id":"fo-3"}]}`))
	})
	client := server.client(t)
	fetch := func(ctx context.Context, opts ListFoldersOptions) (FolderPage, error) {
		return client.ListFolders(ctx, "pr-1", opts)
	}
	folders, err := CollectFolders(context.Background(), fetch, ListFoldersOptions{Take: 2}, 0)
	if err != nil {
		t.Fatalf("CollectFolders: %v", err)
	}
	if len(folders) != 3 || folders[2].ID != "fo-3" {
		t.Errorf("folders = %+v, want all three", folders)
	}
	if len(skips) != 2 || skips[1] != "2" {
		t.Errorf("skips = %v, want the second page to start at 2", skips)
	}
}

func TestCollectFoldersHonoursCountAndCap(t *testing.T) {
	t.Run("stops on the reported total", func(t *testing.T) {
		requests := 0
		server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
			requests++
			w.Header().Set("Content-Type", contentTypeJSON)
			_, _ = w.Write([]byte(`{"count":2,"data":[{"id":"fo-1"},{"id":"fo-2"}]}`))
		})
		client := server.client(t)
		folders, err := CollectFolders(context.Background(), func(ctx context.Context, opts ListFoldersOptions) (FolderPage, error) {
			return client.ListFolders(ctx, "pr-1", opts)
		}, ListFoldersOptions{Take: 2}, 0)
		if err != nil {
			t.Fatalf("CollectFolders: %v", err)
		}
		if len(folders) != 2 || requests != 1 {
			t.Errorf("folders = %d after %d requests, want 2 after 1", len(folders), requests)
		}
	})

	t.Run("caps a server that ignores skip", func(t *testing.T) {
		server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK,
			`{"count":1000,"data":[{"id":"fo-1"},{"id":"fo-2"}]}`))
		client := server.client(t)
		folders, err := CollectFolders(context.Background(), func(ctx context.Context, opts ListFoldersOptions) (FolderPage, error) {
			return client.ListFolders(ctx, "pr-1", opts)
		}, ListFoldersOptions{Take: 2}, 5)
		if err != nil {
			t.Fatalf("CollectFolders: %v", err)
		}
		if len(folders) != 5 {
			t.Errorf("folders = %d, want the cap of 5", len(folders))
		}
	})
}

func TestCollectFoldersDefaultsPageSize(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"count":1,"data":[{"id":"fo-1"}]}`))
	client := server.client(t)
	if _, err := CollectFolders(context.Background(), func(ctx context.Context, opts ListFoldersOptions) (FolderPage, error) {
		return client.ListFolders(ctx, "pr-1", opts)
	}, ListFoldersOptions{}, 0); err != nil {
		t.Fatalf("CollectFolders: %v", err)
	}
	if got := server.requests[0].URL.Query().Get("take"); got != "100" {
		t.Errorf("take = %q, want the collection default of 100", got)
	}
}

func TestFolderErrorsRemainStructured(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusNotFound,
		`{"message":"Project with ID \"personal\" not found"}`))
	_, err := server.client(t).ListFolders(context.Background(), "personal", ListFoldersOptions{})
	if StatusCodeOf(err) != http.StatusNotFound || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want a structured 404", err)
	}
}

func TestFolderFilterEncodesOnlySetFields(t *testing.T) {
	encoded, err := json.Marshal(FolderFilter{Name: "Invoices"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{"name":"Invoices"}` {
		t.Errorf("filter = %s, want only the set field", encoded)
	}
	if !(FolderFilter{}).IsZero() || (FolderFilter{Tags: []string{"x"}}).IsZero() {
		t.Error("IsZero does not report an empty filter correctly")
	}
}

func TestFolderValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	calls := map[string]func() error{
		"blank project": func() error {
			_, err := client.ListFolders(context.Background(), " ", ListFoldersOptions{})
			return err
		},
		"negative skip": func() error {
			_, err := client.ListFolders(context.Background(), "pr-1", ListFoldersOptions{Skip: -1})
			return err
		},
		"negative take": func() error {
			_, err := client.ListFolders(context.Background(), "pr-1", ListFoldersOptions{Take: -1})
			return err
		},
		"unknown sort order": func() error {
			_, err := client.ListFolders(context.Background(), "pr-1", ListFoldersOptions{SortBy: "name"})
			return err
		},
		"unknown select field": func() error {
			_, err := client.ListFolders(context.Background(), "pr-1", ListFoldersOptions{Select: []string{"nope"}})
			return err
		},
		"padded filter name": func() error {
			_, err := client.ListFolders(context.Background(), "pr-1", ListFoldersOptions{Filter: FolderFilter{Name: " Inv"}})
			return err
		},
		"blank filter tag": func() error {
			_, err := client.ListFolders(context.Background(), "pr-1", ListFoldersOptions{Filter: FolderFilter{Tags: []string{" "}}})
			return err
		},
		"blank create name": func() error {
			_, err := client.CreateFolder(context.Background(), "pr-1", CreateFolderRequest{Name: " "})
			return err
		},
		"padded create parent": func() error {
			_, err := client.CreateFolder(context.Background(), "pr-1", CreateFolderRequest{Name: "Invoices", ParentFolderID: "fo-1 "})
			return err
		},
		"blank get folder": func() error {
			_, err := client.GetFolder(context.Background(), "pr-1", "")
			return err
		},
		"empty update": func() error {
			_, err := client.UpdateFolder(context.Background(), "pr-1", "fo-1", UpdateFolderRequest{})
			return err
		},
		"padded update name": func() error {
			_, err := client.UpdateFolder(context.Background(), "pr-1", "fo-1", UpdateFolderRequest{Name: "Invoices "})
			return err
		},
		"blank delete folder": func() error {
			return client.DeleteFolder(context.Background(), "pr-1", " ", "")
		},
		"padded transfer target": func() error {
			return client.DeleteFolder(context.Background(), "pr-1", "fo-1", " fo-2")
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Error("call succeeded, want validation error")
			}
		})
	}
	if len(server.requests) != 0 {
		t.Errorf("request count = %d, want 0", len(server.requests))
	}
}
