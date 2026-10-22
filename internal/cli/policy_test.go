package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const cliPolicyDoc = `{"id":"p1","kind":"future-kind","rules":[],"version":2,"updatedBy":"u1","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`
const cliPolicyResult = `{"policy":` + cliPolicyDoc + `,"warnings":[{"kind":"notLoaded","ruleId":"r1"}]}`
const cliAttachments = `{"attachments":[{"policyId":"p1","rules":[],"priority":0,"isFloor":false}],"version":2}`

type policyCLIOperation struct {
	resource, verb, id, method, path, input, response string
	status                                            int
}

func policyCLIOperations() []policyCLIOperation {
	return []policyCLIOperation{
		{"document", "list", "", "GET", "/policies", "", `{"data":[` + cliPolicyDoc + `],"nextCursor":""}`, 200},
		{"document", "get", "a/b c", "GET", "/policies/a%2Fb%20c", "", cliPolicyDoc, 200},
		{"document", "create", "", "POST", "/policies", `{"rules":[]}`, cliPolicyResult, 201},
		{"document", "set", "a/b c", "PUT", "/policies/a%2Fb%20c", `{"rules":[],"version":2}`, cliPolicyResult, 200},
		{"document", "delete", "a/b c", "DELETE", "/policies/a%2Fb%20c", "", "", 204},
		{"attachment", "get", "a/b c", "GET", "/scopes/a%2Fb%20c/attachments", "", cliAttachments, 200},
		{"attachment", "set", "a/b c", "PUT", "/scopes/a%2Fb%20c/attachments", `{"attachments":[{"policyId":"p1","priority":0,"isFloor":false}],"version":2}`, cliAttachments, 200},
		{"instance", "get", "", "GET", "/instance", "", cliNodePolicyUnconfigured, 200},
		{"instance", "set", "", "PUT", "/instance", `{"rules":[],"defaultAction":"allow","version":0}`, cliNodePolicyWriteResult, 200},
		{"project", "get", "a/b c", "GET", "/projects/a%2Fb%20c", "", cliNodePolicyUnconfigured, 200},
		{"project", "set", "a/b c", "PUT", "/projects/a%2Fb%20c", `{"rules":[],"defaultAction":"allow","version":0}`, cliNodePolicyWriteResult, 200},
	}
}
func policyCLIArgs(group string, op policyCLIOperation) []string {
	args := []string{group, op.resource, op.verb}
	if op.id != "" {
		args = append(args, op.id)
	}
	return args
}
func TestPolicyCommands(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		for _, op := range policyCLIOperations() {
			if group == "node-policy" && (op.resource == "instance" || op.resource == "project") {
				continue
			}
			for _, output := range []string{"text", "json"} {
				t.Run(group+"/"+op.resource+"/"+op.verb+"/"+output, func(t *testing.T) {
					f := nodePolicyFixture(t, op.response)
					f.route = func(*http.Request, int) (int, string) { return op.status, op.response }
					args := policyCLIArgs(group, op)
					if op.input != "" {
						args = append(args, "--input", writeNodePolicyInput(t, op.input))
					}
					if op.verb == "set" || op.verb == "delete" {
						args = append(args, "--yes")
					}
					args = append(args, "--output", output)
					got := f.run(args...)
					if got.code != ExitSuccess {
						t.Fatalf("exit %d: %s", got.code, got.stderr)
					}
					if output == "json" {
						if !json.Valid([]byte(got.stdout)) {
							t.Fatalf("invalid JSON %s", got.stdout)
						}
					} else if op.verb != "delete" && got.stdout == "" {
						t.Error("missing text output")
					}
					base := "node-type-policies"
					if group == "credential-policy" {
						base = "credential-type-policies"
					}
					r := f.lastRequest()
					if r.Method != op.method || r.URL.EscapedPath() != "/api/v1/"+base+op.path {
						t.Errorf("request %s %s", r.Method, r.URL.EscapedPath())
					}
					if op.input != "" {
						assertCLIJSON(t, op.input, f.lastBody())
					}
					if output == "text" && (op.verb == "create" || op.verb == "set") && op.resource != "attachment" && !strings.Contains(got.stderr, "notLoaded") {
						t.Errorf("missing warning: %s", got.stderr)
					}
				})
			}
		}
	}
}
func TestPolicyMutationConsent(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		for _, op := range policyCLIOperations() {
			if op.verb != "set" && op.verb != "delete" && op.verb != "create" {
				continue
			}
			if group == "node-policy" && (op.resource == "instance" || op.resource == "project") {
				continue
			}
			if op.verb != "create" {
				for _, interactive := range []bool{true, false} {
					t.Run(fmt.Sprintf("%s/%s/%s/interactive=%t", group, op.resource, op.verb, interactive), func(t *testing.T) {
						f := nodePolicyFixture(t, op.response)
						f.interactive = interactive
						f.stdin = "no\n"
						args := policyCLIArgs(group, op)
						if op.input != "" {
							args = append(args, "--input", writeNodePolicyInput(t, op.input))
						}
						before := f.requestCount()
						got := f.run(args...)
						if got.code == ExitSuccess || f.requestCount() != before || got.stdout != "" {
							t.Fatalf("refusal failed: %+v requests %d", got, f.requestCount()-before)
						}
					})
				}
			}
			if op.input != "" {
				t.Run(group+"/"+op.resource+"/"+op.verb+"/stdin", func(t *testing.T) {
					f := nodePolicyFixture(t, op.response)
					f.stdin = op.input
					f.route = func(*http.Request, int) (int, string) { return op.status, op.response }
					args := append(policyCLIArgs(group, op), "--input", "-")
					before := f.requestCount()
					got := f.run(args...)
					if got.code == ExitSuccess || !strings.Contains(got.stderr, "--yes") || f.requestCount() != before {
						t.Fatalf("stdin consent: %+v", got)
					}
					got = f.run(append(args, "--yes", "--output", "json")...)
					if got.code != ExitSuccess {
						t.Fatalf("stdin with consent: %+v", got)
					}
				})
			}
		}
	}
}
func TestPolicyStrictInputAndSelectors(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		for _, tt := range []struct{ name, resource, verb, input, want string }{
			{"unknown", "document", "create", `{"rules":[],"bad":true}`, "unknown field"},
			{"nested unknown", "document", "create", `{"rules":[{"id":"r","action":"deny","selector":{"kind":"name","value":"x","bad":1}}]}`, "unknown field"},
			{"read fields on create", "document", "create", cliPolicyDoc, "unknown field"},
			{"multiple docs", "document", "create", `{"rules":[]} {"rules":[]}`, "one document"},
			{"malformed", "document", "create", `{`, "decode"},
			{"too large", "document", "create", strings.Repeat(" ", (1<<20)+1), "exceeds"},
			{"null", "document", "create", `null`, "rules is required"},
			{"missing rules", "document", "create", `{}`, "rules is required"},
			{"null rules", "document", "create", `{"rules":null}`, "rules is required"},
			{"missing version", "document", "set", `{"rules":[]}`, "version"},
			{"duplicate rule", "document", "create", `{"rules":[{"id":"r","action":"allow","selector":{"kind":"name","value":"x"}},{"id":"r","action":"deny","selector":{"kind":"package","value":"x"}}]}`, "unique"},
			{"missing attachments", "attachment", "set", `{"version":1}`, "attachments is required"},
			{"null attachments", "attachment", "set", `{"attachments":null,"version":1}`, "attachments is required"},
			{"no attachment version", "attachment", "set", `{"attachments":[]}`, "version"},
			{"missing floor", "attachment", "set", `{"attachments":[{"policyId":"p","priority":0}],"version":1}`, "isFloor"},
			{"missing priority", "attachment", "set", `{"attachments":[{"policyId":"p","isFloor":false}],"version":1}`, "priority"},
			{"duplicate attachment", "attachment", "set", `{"attachments":[{"policyId":"p","priority":0,"isFloor":false},{"policyId":"p","priority":1,"isFloor":false}],"version":1}`, "unique"},
			{"duplicate ordering", "attachment", "set", `{"attachments":[{"policyId":"p","priority":0,"isFloor":false},{"policyId":"q","priority":0,"isFloor":false}],"version":1}`, "unique"},
		} {
			t.Run(group+"/"+tt.name, func(t *testing.T) {
				f := nodePolicyFixture(t, `{}`)
				args := []string{group, tt.resource, tt.verb}
				if tt.verb == "set" {
					args = append(args, "p")
				}
				args = append(args, "--input", writeNodePolicyInput(t, tt.input), "--yes")
				before := f.requestCount()
				got := f.run(args...)
				if got.code == ExitSuccess || !strings.Contains(got.stderr, tt.want) || f.requestCount() != before {
					t.Fatalf("got %+v; want %q, zero requests", got, tt.want)
				}
			})
		}
		for _, resource := range []string{"document", "instance", "project"} {
			if group == "node-policy" && resource != "document" {
				continue
			}
			t.Run(group+"/extends/"+resource, func(t *testing.T) {
				input := `{"rules":[{"id":"r","action":"allow","selector":{"kind":"extends","value":"base"}}]}`
				args := []string{group, resource, "create"}
				response := cliPolicyResult
				if resource != "document" {
					input = strings.TrimSuffix(input, "}") + `,"defaultAction":"allow","version":0}`
					args = []string{group, resource, "set"}
					if resource == "project" {
						args = append(args, "p")
					}
					response = cliNodePolicyWriteResult
				}
				f := nodePolicyFixture(t, response)
				before := f.requestCount()
				got := f.run(append(args, "--input", writeNodePolicyInput(t, input), "--yes")...)
				if group == "node-policy" {
					if got.code == ExitSuccess || f.requestCount() != before {
						t.Fatalf("node accepted extends: %+v", got)
					}
				} else if got.code != ExitSuccess {
					t.Fatalf("credential rejected extends: %+v", got)
				}
			})
		}
	}
}
func TestPolicyDroppedReadFields(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		for _, resource := range []string{"document", "attachment"} {
			t.Run(group+"/"+resource, func(t *testing.T) {
				input := cliPolicyDoc
				response := cliPolicyResult
				wantBody := `{"rules":[],"version":2}`
				wantDropped := "id"
				if resource == "attachment" {
					input = cliAttachments
					response = cliAttachments
					wantBody = `{"attachments":[{"policyId":"p1","priority":0,"isFloor":false}],"version":2}`
					wantDropped = "attachments[0].rules"
				}
				f := nodePolicyFixture(t, response)
				got := f.run(group, resource, "set", "p", "--input", writeNodePolicyInput(t, input), "--yes", "--output", "json")
				if got.code != ExitSuccess || !strings.Contains(got.stderr, "Dropped read-only field "+wantDropped) {
					t.Fatalf("got %+v", got)
				}
				assertCLIJSON(t, wantBody, f.lastBody())
			})
		}
	}
	f := nodePolicyFixture(t, cliNodePolicyWriteResult)
	got := f.run("credential-policy", "instance", "set", "--input", writeNodePolicyInput(t, cliNodePolicyInput), "--yes")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "Dropped read-only field scopeId") {
		t.Fatalf("scope fields: %+v", got)
	}
}
func TestPolicyListPagination(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		t.Run(group, func(t *testing.T) {
			f := nodePolicyFixture(t, "")
			calls := 0
			f.route = func(r *http.Request, _ int) (int, string) {
				calls++
				if r.URL.Query().Get("limit") != "1" {
					t.Error("lost limit")
				}
				cursor := r.URL.Query().Get("cursor")
				next := ""
				if calls == 1 {
					if cursor != "start +" {
						t.Error("lost starting cursor")
					}
					next = "next +"
				} else if cursor != "next +" {
					t.Error("lost cursor")
				}
				return 200, `{"data":[` + cliPolicyDoc + `],"nextCursor":"` + next + `"}`
			}
			got := f.run(group, "document", "list", "--all", "--limit", "1", "--cursor", "start +", "--output", "json")
			if got.code != ExitSuccess {
				t.Fatal(got.stderr)
			}
			var page struct {
				Data       []json.RawMessage `json:"data"`
				Collection collectionInfo    `json:"collection"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 || calls != 2 || page.Collection.Truncated {
				t.Fatalf("pagination %+v %d %v", page, calls, err)
			}
		})
	}
}
func TestPolicyErrorsAndArguments(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		for _, op := range policyCLIOperations() {
			if group == "node-policy" && (op.resource == "instance" || op.resource == "project") {
				continue
			}
			for _, status := range []int{401, 403, 404, 409, 503} {
				t.Run(fmt.Sprintf("%s/%s/%s/%d", group, op.resource, op.verb, status), func(t *testing.T) {
					f := nodePolicyFixture(t, "")
					f.status = status
					args := policyCLIArgs(group, op)
					if op.input != "" {
						args = append(args, "--input", writeNodePolicyInput(t, op.input))
					}
					if op.input != "" || op.verb == "delete" {
						args = append(args, "--yes")
					}
					got := f.run(args...)
					if got.code == ExitSuccess || got.stdout != "" {
						t.Fatalf("error result %+v", got)
					}
					want := fmt.Sprint(status)
					if status == 401 {
						want = "n8n auth login"
					}
					if !strings.Contains(got.stderr, want) {
						t.Errorf("missing %q in %s", want, got.stderr)
					}
					if status == 403 || status == 404 || status == 503 {
						resource := "nodetypepolicy"
						if group == "credential-policy" {
							resource = "credentialtypepolicy"
						}
						if !strings.Contains(got.stderr, "--resource "+resource) {
							t.Errorf("bad discover key %s", got.stderr)
						}
					}
					if status == 409 && op.verb == "delete" && !strings.Contains(got.stderr, "attachment set") {
						t.Errorf("missing detach guidance %s", got.stderr)
					}
				})
			}
			t.Run(group+"/"+op.resource+"/"+op.verb+"/arguments", func(t *testing.T) {
				f := nodePolicyFixture(t, "")
				before := f.requestCount()
				args := policyCLIArgs(group, op)
				got := f.run(append(args, "--output", "invalid")...)
				if got.code == ExitSuccess || f.requestCount() != before {
					t.Fatalf("output validation %+v", got)
				}
				if op.id != "" {
					got = f.run(group, op.resource, op.verb)
					if got.code == ExitSuccess || f.requestCount() != before {
						t.Fatalf("arg validation %+v", got)
					}
				}
			})
		}
	}
}
func TestPolicyPreviewHelp(t *testing.T) {
	root := NewRootCommand(Options{})
	for _, name := range []string{"node-policy", "credential-policy"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		var visit func(*cobra.Command)
		visit = func(c *cobra.Command) {
			if !strings.Contains(c.Long, "Preview API") {
				t.Errorf("%s missing Preview API", c.CommandPath())
			}
			for _, child := range c.Commands() {
				visit(child)
			}
		}
		visit(cmd)
	}
}

func TestPolicyListBoundAndEmpty(t *testing.T) {
	for _, group := range []string{"node-policy", "credential-policy"} {
		t.Run(group+"/bound", func(t *testing.T) {
			f := nodePolicyFixture(t, "")
			calls := 0
			data := strings.TrimSuffix(strings.Repeat(cliPolicyDoc+",", 250), ",")
			f.route = func(r *http.Request, _ int) (int, string) {
				calls++
				if calls > 40 {
					t.Error("collection exceeded 10,000")
				}
				if r.URL.Query().Get("limit") != "250" {
					t.Error("limit changed")
				}
				return 200, fmt.Sprintf(`{"data":[%s],"nextCursor":"page-%d"}`, data, calls)
			}
			got := f.run(group, "document", "list", "--all", "--limit", "250", "--output", "json")
			var page struct {
				Data       []json.RawMessage `json:"data"`
				Collection collectionInfo    `json:"collection"`
				NextCursor string            `json:"nextCursor"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || got.code != ExitSuccess || len(page.Data) != 10000 || !page.Collection.Truncated || page.NextCursor != "page-40" || calls != 40 || !strings.Contains(got.stderr, "10,000") && !strings.Contains(got.stderr, "10000") {
				t.Fatalf("bound: code=%d len=%d calls=%d collection=%+v stderr=%s err=%v", got.code, len(page.Data), calls, page.Collection, got.stderr, err)
			}
		})
		t.Run(group+"/empty", func(t *testing.T) {
			f := nodePolicyFixture(t, `{"data":[],"nextCursor":""}`)
			got := f.run(group, "document", "list", "--output", "json")
			if got.code != ExitSuccess || !strings.Contains(got.stdout, `"data": []`) || !strings.Contains(got.stderr, "No policy documents") {
				t.Fatalf("empty: %+v", got)
			}
		})
	}
}

func TestCredentialProjectRejectsDelegateBeforeTransport(t *testing.T) {
	for _, input := range []string{
		`{"rules":[],"defaultAction":"delegate","version":0}`,
		`{"rules":[{"id":"r","action":"delegate","selector":{"kind":"extends","value":"base"}}],"defaultAction":"allow","version":0}`,
	} {
		f := nodePolicyFixture(t, "")
		before := f.requestCount()
		got := f.run("credential-policy", "project", "set", "p", "--input", writeNodePolicyInput(t, input), "--yes")
		if got.code == ExitSuccess || !strings.Contains(got.stderr, "delegate") || f.requestCount() != before {
			t.Fatalf("delegate rejection: %+v", got)
		}
	}
}
