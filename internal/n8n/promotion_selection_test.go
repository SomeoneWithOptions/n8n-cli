package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const selectionSHA = "0123456789012345678901234567890123456789"
const selectionSourceJSON = `{"configId":"cfg","branchName":"main","commitSha":"` + selectionSHA + `"}`
const selectionChangesJSON = `{"commitSha":"` + selectionSHA + `","source":{"configId":"cfg","branchName":"main"},"changes":[{"id":"wf","name":"Workflow","type":"workflow","status":"renamed-and-modified","version":null,"updatedAt":null,"updatedBy":null,"dependencyCount":2,"future":true}],"future":{"ok":true}}`

func selectionSource() PromotionExpectedSource {
	return PromotionExpectedSource{ConfigID: "cfg", BranchName: "main", CommitSHA: selectionSHA}
}

func TestPromotionSelectionTransport(t *testing.T) {
	yes, no := true, false
	message := "sync"
	cases := []struct {
		name, path, body, response string
		call                       func(context.Context, *Client) (any, error)
	}{
		{"repositories", "/promotions/providers/prov%2F1/repositories", "", `{"data":[{"id":"1","name":"repo","fullPath":"org/repo","remoteUrl":"https://gitlab.com/org/repo.git","defaultBranch":null}],"nextCursor":"next"}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ListPromotionProviderRepositories(ctx, "prov/1", PromotionDiscoveryOptions{ListOptions: ListOptions{Limit: 20, Cursor: "a+b"}, Search: "a & b"})
		}},
		{"branches", "/promotions/providers/prov%2F1/repositories/org%2Frepo/branches", "", `{"data":[{"name":"main","isDefault":true}],"nextCursor":"next"}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ListPromotionProviderRepositoryBranches(ctx, "prov/1", "org/repo", PromotionDiscoveryOptions{ListOptions: ListOptions{Limit: 20, Cursor: "a+b"}, Search: "a & b"})
		}},
		{"changes", "/promotions/projects/proj%2F1/changes/apply", "", selectionChangesJSON, func(ctx context.Context, c *Client) (any, error) {
			return c.GetPromotionChanges(ctx, "proj/1", "apply", PromotionChangesOptions{Search: "a & b", Sort: "updatedAt", Order: "desc"})
		}},
		{"apply", "/promotions/projects/proj%2F1/apply", `{"workflowIds":["wf"],"expectedSource":` + selectionSourceJSON + `}`, `{"status":"blocked","configId":"cfg","git":{"branchName":"main","commitSha":"` + selectionSHA + `"},"preflight":{"future":true}}`, func(ctx context.Context, c *Client) (any, error) {
			source := selectionSource()
			return c.ApplyProjectSelection(ctx, "proj/1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}, ExpectedSource: &source})
		}},
		{"continue project", "/promotions/projects/proj%2F1/apply/continue", `{"workflowIds":["wf"],"expectedSource":` + selectionSourceJSON + `,"confirmDestructiveChanges":false}`, `{"status":"source-changed","configId":"cfg","git":{"branchName":"main","commitSha":"` + selectionSHA + `"}}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ContinueApplyProjectSelection(ctx, "proj/1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}, ExpectedSource: selectionSource(), ConfirmDestructiveChanges: &no})
		}},
		{"continue connection", "/promotions/connections/conn%2F1/apply/continue", `{"expectedSource":` + selectionSourceJSON + `,"confirmDestructiveChanges":true}`, `{"status":"applied","counts":{},"warnings":[]}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ContinueApplyPackage(ctx, "conn/1", ContinueApplyPromotionRequest{ExpectedSource: selectionSource(), ConfirmDestructiveChanges: &yes})
		}},
		{"promote", "/promotions/projects/proj%2F1/promote", `{"workflowIds":["wf"],"commitMessage":"sync"}`, promotionPromoteResultBody, func(ctx context.Context, c *Client) (any, error) {
			return c.PromoteProjectSelection(ctx, "proj/1", PromoteProjectSelectionRequest{WorkflowIDs: []string{"wf"}, CommitMessage: &message})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tc.response))
			result, err := tc.call(context.Background(), server.client(t))
			if err != nil {
				t.Fatal(err)
			}
			req, body := server.last(t)
			method := http.MethodGet
			if tc.body != "" {
				method = http.MethodPost
			}
			if req.Method != method || req.URL.EscapedPath() != BasePath+tc.path || req.Header.Get(HeaderAPIKey) != "community-secret" {
				t.Errorf("request = %s %s auth=%q", req.Method, req.URL.EscapedPath(), req.Header.Get(HeaderAPIKey))
			}
			if body != tc.body {
				t.Errorf("body = %s, want %s", body, tc.body)
			}
			if method == http.MethodGet {
				if req.URL.Query().Get("search") != "a & b" {
					t.Errorf("query=%s", req.URL.RawQuery)
				}
				if tc.name == "changes" {
					if req.URL.Query().Get("sort") != "updatedAt" || req.URL.Query().Get("order") != "desc" {
						t.Errorf("query=%s", req.URL.RawQuery)
					}
				} else if req.URL.Query().Get("limit") != "20" || req.URL.Query().Get("cursor") != "a+b" {
					t.Errorf("query=%s", req.URL.RawQuery)
				}
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.response), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("response not preserved: %s", encoded)
			}

			for _, status := range []int{400, 401, 403, 404, 409, 415, 422, 503} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					s := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"failure"}`))
					_, err := tc.call(context.Background(), s.client(t))
					if !IsStatus(err, status) {
						t.Errorf("error=%v", err)
					}
				})
			}
			for _, status := range []int{201, 204} {
				t.Run(fmt.Sprintf("empty%d", status), func(t *testing.T) {
					s := newCommunityPackageServer(t, packageJSONHandler(status, ""))
					if _, err := tc.call(context.Background(), s.client(t)); err != nil {
						t.Fatal(err)
					}
				})
			}
			t.Run("malformed", func(t *testing.T) {
				s := newCommunityPackageServer(t, packageJSONHandler(200, `{bad`))
				if _, err := tc.call(context.Background(), s.client(t)); err == nil {
					t.Fatal("accepted malformed JSON")
				}
			})
			t.Run("canceled", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				before := len(server.requests)
				if _, err := tc.call(ctx, server.client(t)); !errors.Is(err, context.Canceled) {
					t.Errorf("error=%v", err)
				}
				if len(server.requests) != before {
					t.Error("canceled request reached transport")
				}
			})
		})
	}
}

func TestPromotionSelectionValidationBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(200, `{}`))
	c := server.client(t)
	ctx := context.Background()
	invalid := []func() error{
		func() error {
			_, e := c.ListPromotionProviderRepositories(ctx, "", PromotionDiscoveryOptions{})
			return e
		},
		func() error {
			_, e := c.ListPromotionProviderRepositories(ctx, "p", PromotionDiscoveryOptions{ListOptions: ListOptions{Limit: 251}})
			return e
		},
		func() error {
			_, e := c.ListPromotionProviderRepositories(ctx, "p", PromotionDiscoveryOptions{ListOptions: ListOptions{Limit: -1}})
			return e
		},
		func() error {
			_, e := c.ListPromotionProviderRepositories(ctx, "p", PromotionDiscoveryOptions{Search: strings.Repeat("é", 256)})
			return e
		},
		func() error {
			_, e := c.ListPromotionProviderRepositoryBranches(ctx, "p", "", PromotionDiscoveryOptions{})
			return e
		},
		func() error {
			_, e := c.ListPromotionProviderRepositoryBranches(ctx, "p", strings.Repeat("x", 257), PromotionDiscoveryOptions{})
			return e
		},
		func() error { _, e := c.GetPromotionChanges(ctx, "p", "pull", PromotionChangesOptions{}); return e },
		func() error {
			_, e := c.GetPromotionChanges(ctx, "p", "apply", PromotionChangesOptions{Sort: "id"})
			return e
		},
		func() error {
			_, e := c.GetPromotionChanges(ctx, "p", "apply", PromotionChangesOptions{Order: "up"})
			return e
		},
		func() error { _, e := c.ApplyProjectSelection(ctx, "p", ApplyProjectSelectionRequest{}); return e },
		func() error {
			_, e := c.ApplyProjectSelection(ctx, "p", ApplyProjectSelectionRequest{WorkflowIDs: []string{"w", "w"}})
			return e
		},
		func() error {
			_, e := c.ApplyProjectSelection(ctx, "p", ApplyProjectSelectionRequest{WorkflowIDs: []string{strings.Repeat("w", 37)}})
			return e
		},
		func() error {
			_, e := c.ApplyProjectSelection(ctx, "p", ApplyProjectSelectionRequest{WorkflowIDs: []string{" w"}})
			return e
		},
		func() error {
			_, e := c.ApplyProjectSelection(ctx, "p", ApplyProjectSelectionRequest{WorkflowIDs: []string{"w"}, ExpectedSource: &PromotionExpectedSource{}})
			return e
		},
		func() error { _, e := c.ContinueApplyPackage(ctx, "c", ContinueApplyPromotionRequest{}); return e },
		func() error {
			_, e := c.ContinueApplyProjectSelection(ctx, "p", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"w"}})
			return e
		},
		func() error { _, e := c.PromoteProjectSelection(ctx, "p", PromoteProjectSelectionRequest{}); return e },
	}
	for i, call := range invalid {
		if err := call(); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	for _, message := range []string{"", " ", strings.Repeat("x", 1001)} {
		if _, err := c.PromoteProjectSelection(ctx, "p", PromoteProjectSelectionRequest{WorkflowIDs: []string{"w"}, CommitMessage: &message}); err == nil {
			t.Error("invalid commit accepted")
		}
	}
	if len(server.requests) != 0 {
		t.Fatalf("validation sent %d requests", len(server.requests))
	}
}

func TestPromotionDiscoveryCollectShortPages(t *testing.T) {
	for _, branches := range []bool{false, true} {
		t.Run(fmt.Sprint(branches), func(t *testing.T) {
			calls := 0
			server := newCommunityPackageServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("search") != "space & slash/" || r.URL.Query().Get("limit") != "100" {
					t.Errorf("filters=%v", r.URL.Query())
				}
				cursor := "next"
				if calls == 2 {
					cursor = ""
					if r.URL.Query().Get("cursor") != "next" {
						t.Errorf("cursor=%s", r.URL.RawQuery)
					}
				}
				fmt.Fprintf(w, `{"data":[{"id":"%d","name":"main","isDefault":true}],"nextCursor":%q}`, calls, cursor)
			})
			c := server.client(t)
			opts := PromotionDiscoveryOptions{Search: "space & slash/"}
			if branches {
				result, err := CollectPromotionDiscovery(context.Background(), func(ctx context.Context, p ListOptions) (Page[PromotionRepositoryBranch], error) {
					opts.ListOptions = p
					return c.ListPromotionProviderRepositoryBranches(ctx, "p", "org/repo", opts)
				}, ListOptions{Limit: 100})
				if err != nil || len(result.Data) != 2 || result.Truncated {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else {
				result, err := CollectPromotionDiscovery(context.Background(), func(ctx context.Context, p ListOptions) (Page[PromotionRepository], error) {
					opts.ListOptions = p
					return c.ListPromotionProviderRepositories(ctx, "p", opts)
				}, ListOptions{Limit: 100})
				if err != nil || len(result.Data) != 2 || result.Truncated {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			if calls != 2 {
				t.Errorf("calls=%d", calls)
			}
		})
	}
}

func TestPromotionSelectionOptionalFieldsOmitted(t *testing.T) {
	for _, request := range []any{ApplyProjectSelectionRequest{WorkflowIDs: []string{"w"}}, PromoteProjectSelectionRequest{WorkflowIDs: []string{"w"}}} {
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != `{"workflowIds":["w"]}` {
			t.Errorf("request=%s", data)
		}
	}
	request := ContinueApplyPromotionRequest{ExpectedSource: selectionSource()}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "confirmDestructiveChanges") {
		t.Errorf("request=%s", data)
	}
}

func TestPromotionDiscoveryCollectionBounds(t *testing.T) {
	t.Run("empty nonfinal page", func(t *testing.T) {
		calls := 0
		result, err := CollectPromotionDiscovery(context.Background(), func(_ context.Context, opts ListOptions) (Page[int], error) {
			calls++
			switch opts.Cursor {
			case "":
				return Page[int]{Data: []int{}, NextCursor: "empty"}, nil
			case "empty":
				return Page[int]{Data: []int{1}, NextCursor: "last"}, nil
			case "last":
				return Page[int]{Data: []int{2}}, nil
			default:
				t.Fatalf("unexpected cursor %q", opts.Cursor)
				return Page[int]{}, nil
			}
		}, ListOptions{})
		if err != nil || calls != 3 || len(result.Data) != 2 || result.Truncated {
			t.Errorf("result=%+v calls=%d error=%v", result, calls, err)
		}
	})
	t.Run("cursor loop across empty page", func(t *testing.T) {
		calls := 0
		_, err := CollectPromotionDiscovery(context.Background(), func(_ context.Context, opts ListOptions) (Page[int], error) {
			calls++
			if opts.Cursor == "start" {
				return Page[int]{NextCursor: "next"}, nil
			}
			return Page[int]{Data: []int{1}, NextCursor: "start"}, nil
		}, ListOptions{Cursor: "start"})
		if err == nil || !strings.Contains(err.Error(), "repeated") || calls != 2 {
			t.Errorf("calls=%d err=%v", calls, err)
		}
	})
	for _, clip := range []bool{false, true} {
		t.Run(fmt.Sprintf("cap clip=%v", clip), func(t *testing.T) {
			calls := 0
			size := DefaultCollectLimit
			if clip {
				size++
			}
			result, err := CollectPromotionDiscovery(context.Background(), func(context.Context, ListOptions) (Page[int], error) {
				calls++
				return Page[int]{Data: make([]int, size), NextCursor: "next"}, nil
			}, ListOptions{})
			wantCursor := "next"
			if clip {
				wantCursor = ""
			}
			if err != nil || calls != 1 || len(result.Data) != DefaultCollectLimit || !result.Truncated || result.NextCursor != wantCursor {
				t.Errorf("len=%d calls=%d truncated=%t cursor=%q err=%v", len(result.Data), calls, result.Truncated, result.NextCursor, err)
			}
		})
	}
	t.Run("endless empty pages", func(t *testing.T) {
		calls := 0
		_, err := CollectPromotionDiscovery(context.Background(), func(context.Context, ListOptions) (Page[int], error) {
			calls++
			return Page[int]{NextCursor: fmt.Sprint(calls)}, nil
		}, ListOptions{})
		if err == nil || !strings.Contains(err.Error(), "exceeded") || calls != DefaultCollectLimit {
			t.Errorf("calls=%d err=%v", calls, err)
		}
	})
}
