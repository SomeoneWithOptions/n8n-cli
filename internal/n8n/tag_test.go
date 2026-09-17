package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const tagResponse = `{
  "id":"tag/one?x=1",
  "name":"Production",
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "extraFieldAddedLater":"ignored"
}`

var wantTag = Tag{
	ID:        "tag/one?x=1",
	Name:      "Production",
	CreatedAt: "2026-01-01T00:00:00.000Z",
	UpdatedAt: "2026-01-02T00:00:00.000Z",
}

func TestListTags(t *testing.T) {
	body := `{"data":[` + tagResponse + `],"nextCursor":"next page"}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))

	page, err := server.client(t).ListTags(context.Background(), ListOptions{Limit: 25, Cursor: "start here"})
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	req, requestBody := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+TagsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+TagsPath)
	}
	if req.URL.Query().Get("limit") != "25" || req.URL.Query().Get("cursor") != "start here" {
		t.Errorf("query = %q, want the pagination parameters", req.URL.RawQuery)
	}
	if requestBody != "" {
		t.Errorf("body = %q, want empty", requestBody)
	}
	if diff := cmp.Diff([]Tag{wantTag}, page.Data); diff != "" {
		t.Errorf("page data mismatch (-want +got):\n%s", diff)
	}
	if page.NextCursor != "next page" || !page.HasMore() {
		t.Errorf("nextCursor = %q, HasMore = %v", page.NextCursor, page.HasMore())
	}
}

// TestListTagsCollectsEveryPage covers the --all path: the client follows
// nextCursor until the API stops sending one.
func TestListTagsCollectsEveryPage(t *testing.T) {
	pages := []string{
		`{"data":[{"id":"1","name":"first"}],"nextCursor":"page two"}`,
		`{"data":[{"id":"2","name":"second"}],"nextCursor":null}`,
	}
	var n int
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		if n >= len(pages) {
			t.Errorf("request %d is more than the %d pages available", n+1, len(pages))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(pages[n]))
		n++
	})

	client := server.client(t)
	tags, err := Collect(context.Background(), client.ListTags, ListOptions{}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if diff := cmp.Diff([]Tag{{ID: "1", Name: "first"}, {ID: "2", Name: "second"}}, tags); diff != "" {
		t.Errorf("tags mismatch (-want +got):\n%s", diff)
	}
	if len(server.requests) != 2 {
		t.Fatalf("%d requests, want 2", len(server.requests))
	}
	if got := server.requests[1].URL.Query().Get("cursor"); got != "page two" {
		t.Errorf("second request cursor = %q, want %q", got, "page two")
	}
}

func TestGetCreateUpdateAndDeleteTag(t *testing.T) {
	tests := []struct {
		name    string
		call    func(*Client) (*Tag, error)
		method  string
		escaped string
		body    string
	}{
		{name: "create", call: func(c *Client) (*Tag, error) {
			return c.CreateTag(context.Background(), "Production")
		}, method: http.MethodPost, escaped: TagsPath, body: `{"name":"Production"}`},
		{name: "get", call: func(c *Client) (*Tag, error) {
			return c.GetTag(context.Background(), "tag/one?x=1")
		}, method: http.MethodGet, escaped: "/tags/tag%2Fone%3Fx=1"},
		{name: "update", call: func(c *Client) (*Tag, error) {
			return c.UpdateTag(context.Background(), "tag/one?x=1", "Production")
		}, method: http.MethodPut, escaped: "/tags/tag%2Fone%3Fx=1", body: `{"name":"Production"}`},
		{name: "delete", call: func(c *Client) (*Tag, error) {
			return c.DeleteTag(context.Background(), "tag/one?x=1")
		}, method: http.MethodDelete, escaped: "/tags/tag%2Fone%3Fx=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tagResponse))
			tag, err := tt.call(server.client(t))
			if err != nil {
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
			if diff := cmp.Diff(&wantTag, tag); diff != "" {
				t.Errorf("tag mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestTagConflictIsReported covers the duplicate-name response both write
// operations can return.
func TestTagConflictIsReported(t *testing.T) {
	for _, update := range []bool{false, true} {
		name := map[bool]string{false: "create", true: "update"}[update]
		t.Run(name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `{"message":"Tag already exists"}`))
			client := server.client(t)
			var err error
			if update {
				_, err = client.UpdateTag(context.Background(), "id", "Production")
			} else {
				_, err = client.CreateTag(context.Background(), "Production")
			}
			if !IsConflict(err) {
				t.Fatalf("error = %v, want 409", err)
			}
			if got := err.Error(); !strings.Contains(got, "Tag already exists") {
				t.Errorf("error = %q, want the server message", got)
			}
		})
	}
}

// TestTagEmptyResponseBody covers a server that answers a delete with 204 and
// no document: decoding must not fail.
func TestTagEmptyResponseBody(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	tag, err := server.client(t).DeleteTag(context.Background(), "tag-id")
	if err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if diff := cmp.Diff(&Tag{}, tag); diff != "" {
		t.Errorf("tag mismatch (-want +got):\n%s", diff)
	}
}

func TestTagValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tagResponse))
	client := server.client(t)
	calls := map[string]func() error{
		"negative limit": func() error {
			_, err := client.ListTags(context.Background(), ListOptions{Limit: -1})
			return err
		},
		"empty create name":    func() error { _, err := client.CreateTag(context.Background(), " "); return err },
		"padded create name":   func() error { _, err := client.CreateTag(context.Background(), "name "); return err },
		"empty get ID":         func() error { _, err := client.GetTag(context.Background(), ""); return err },
		"padded update ID":     func() error { _, err := client.UpdateTag(context.Background(), " id", "name"); return err },
		"empty update name":    func() error { _, err := client.UpdateTag(context.Background(), "id", ""); return err },
		"whitespace delete ID": func() error { _, err := client.DeleteTag(context.Background(), "\t"); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: call succeeded, want a validation error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d invalid requests reached the server", len(server.requests))
	}
}
