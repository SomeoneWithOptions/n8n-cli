package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

func diffFixtures(t *testing.T) (*fixture, *fixture) {
	t.Helper()
	from, to := newFixture(t), newFixture(t)
	from.login("--context", "staging")
	to.dir, to.keyring = from.dir, from.keyring
	to.prompt.secret = "production-key"
	to.login("--context", "prod")
	from.body = strings.Replace(cliWorkflow, `"id":"wf-1"`, `"id":"AAA"`, 1)
	to.body = strings.Replace(cliWorkflow, `"id":"wf-1"`, `"id":"BBB"`, 1)
	return from, to
}

func diffArgs(extra ...string) []string {
	return append([]string{"workflow", "diff", "--from-context", "staging", "--to-context", "prod"}, extra...)
}

func diffIDArgs(extra ...string) []string {
	return diffArgs(append([]string{"--from-id", "AAA", "--to-id", "BBB"}, extra...)...)
}

func TestWorkflowDiffUsesIndependentSavedContexts(t *testing.T) {
	from, to := diffFixtures(t)
	before, _ := json.Marshal(from.config())
	from.env = map[string]string{
		config.EnvURL: "https://wrong.invalid", config.EnvAPIKey: "wrong-key",
		config.EnvBearerToken: "wrong-token", config.EnvAuthCookie: "wrong-cookie",
	}
	to.body = strings.Replace(to.body, `"active":true`, `"active":false`, 1)
	got := from.run(diffIDArgs("--output", "json")...)
	if got.code != ExitSuccess || got.stderr != "" {
		t.Fatalf("result: %+v", got)
	}
	var result workflowDiffResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Equal || result.SchemaVersion != 1 || result.Mode != "saved" || result.Changes == nil {
		t.Fatalf("unexpected JSON contract: %s", got.stdout)
	}
	if result.From.WorkflowID != "AAA" || result.To.WorkflowID != "BBB" || result.From.Context != "staging" || result.To.Context != "prod" || !result.From.Published || result.To.Published {
		t.Fatalf("wrong endpoint metadata: %s", got.stdout)
	}
	for _, side := range []struct {
		f   *fixture
		key string
		id  string
	}{{from, testAPIKey, "AAA"}, {to, "production-key", "BBB"}} {
		req := side.f.lastRequest()
		if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+"/workflows/"+side.id || req.Header.Get(n8n.HeaderAPIKey) != side.key {
			t.Errorf("wrong endpoint, method or credential for %s", side.id)
		}
		if req.URL.Query().Get("excludePinnedData") != "true" {
			t.Error("default read did not exclude pins")
		}
		if strings.Contains(got.stdout+got.stderr, side.key) {
			t.Error("credential exposed in output")
		}
	}
	after, _ := json.Marshal(from.config())
	if string(before) != string(after) {
		t.Fatal("diff changed the selected context or saved config")
	}
}

func TestWorkflowDiffChangesTextJSONAndQuiet(t *testing.T) {
	from, to := diffFixtures(t)
	to.body = strings.Replace(to.body, `"path":"hook"`, `"path":"production"`, 1)
	got := from.run(diffIDArgs("--output", "json")...)
	var result workflowDiffResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
		t.Fatal(err)
	}
	if got.code != 1 || got.stderr != "" || result.Equal || len(result.Changes) != 1 || result.Summary.NodesChanged != 1 || result.Changes[0].Path != "/nodes/id:n-1/parameters/path" {
		t.Fatalf("unexpected diff: %+v", got)
	}
	got = from.run(diffIDArgs()...)
	for _, want := range []string{"saved definitions", "staging", "prod", "1 changed", "---", "+++", "@@", `-        "path": "hook"`, `+        "path": "production"`} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("text missing %q:\n%s", want, got.stdout)
		}
	}
	got = from.run(diffIDArgs("--quiet")...)
	if got.code != 1 || got.stdout != "" || got.stderr != "" {
		t.Fatalf("quiet drift: %+v", got)
	}
	got = from.run(diffIDArgs("--only", "settings", "--quiet")...)
	if got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Fatalf("quiet equality: %+v", got)
	}
	to.status = http.StatusForbidden
	got = from.run(diffIDArgs("--quiet")...)
	if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, "target") || !strings.Contains(got.stderr, "403") {
		t.Fatalf("quiet must retain errors: %+v", got)
	}
}

func TestWorkflowDiffNameLookupAcrossPagesAndProjects(t *testing.T) {
	from, to := diffFixtures(t)
	fromBody, toBody := from.body, to.body
	from.bodyFunc = func(page int) string {
		switch page {
		case 0:
			return `{"data":[{"id":"other","name":"Invoice sync extra"}],"nextCursor":"next"}`
		case 1:
			return `{"data":[{"id":"AAA","name":"Invoice sync"}]}`
		default:
			return fromBody
		}
	}
	to.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[{"id":"BBB","name":"Invoice sync"}]}`
		}
		return toBody
	}
	got := from.run(diffArgs("--name", "Invoice sync", "--from-project-id", "dev", "--to-project-id", "production", "--output", "json")...)
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("name lookup: %+v", got)
	}
	for _, side := range []struct {
		f       *fixture
		project string
	}{{from, "dev"}, {to, "production"}} {
		for _, req := range side.f.requests[1:] { // Skip authentication validation.
			if req.Method != http.MethodGet {
				t.Fatal("diff performed a write")
			}
			if req.URL.Path == n8n.BasePath+"/workflows" {
				q := req.URL.Query()
				if q.Get("name") != "Invoice sync" || q.Get("projectId") != side.project || q.Get("excludePinnedData") != "true" {
					t.Errorf("lost lookup filters: %v", q)
				}
			}
		}
	}
	if from.requests[2].URL.Query().Get("cursor") != "next" {
		t.Error("did not follow pagination cursor")
	}
}

func TestWorkflowDiffRejectsAmbiguousNameOnLaterPage(t *testing.T) {
	from, to := diffFixtures(t)
	from.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[{"id":"AAA","name":"Invoice sync","shared":[{"projectId":"p1"}]}],"nextCursor":"next"}`
		}
		return `{"data":[{"id":"CCC","name":"Invoice sync","shared":[{"projectId":"p2"}]}]}`
	}
	got := from.run(diffArgs("--name", "Invoice sync")...)
	if got.code != 2 || got.stdout != "" || to.requestCount() != 1 {
		t.Fatalf("ambiguous name: %+v", got)
	}
	for _, want := range []string{"source", "staging", "ambiguous", "AAA", "CCC", "p1", "p2", "--from-id", "--from-project-id"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("missing ambiguity detail %q: %s", want, got.stderr)
		}
	}
	if from.lastRequest().URL.Path != n8n.BasePath+"/workflows" {
		t.Error("ambiguous lookup fetched a guessed workflow")
	}
}

func TestWorkflowDiffLookupFailures(t *testing.T) {
	cases := []struct {
		name string
		body func(int) string
		want string
	}{
		{"absent", func(int) string { return `{"data":[]}` }, "no exact match"},
		{"case sensitive", func(int) string { return `{"data":[{"id":"AAA","name":"invoice sync"}]}` }, "no exact match"},
		{"empty page with cursor", func(int) string { return `{"data":[],"nextCursor":"next"}` }, "incomplete name lookup"},
		{"repeated cursor", func(int) string { return `{"data":[{"id":"AAA","name":"Invoice sync"}],"nextCursor":"next"}` }, "repeats cursor"},
		{"missing ID", func(int) string { return `{"data":[{"name":"Invoice sync"}]}` }, "without an ID"},
		{"renamed after lookup", func(page int) string {
			if page == 0 {
				return `{"data":[{"id":"AAA","name":"Invoice sync"}]}`
			}
			return `{"id":"AAA","name":"Renamed"}`
		}, "identity changed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, _ := diffFixtures(t)
			from.bodyFunc = tc.body
			got := from.run(diffArgs("--name", "Invoice sync")...)
			if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, tc.want) {
				t.Fatalf("lookup error: %+v", got)
			}
		})
	}
}

func TestWorkflowDiffLookupLimitIsAnError(t *testing.T) {
	from, _ := diffFixtures(t)
	var body strings.Builder
	body.WriteString(`{"data":[`)
	for i := range n8n.DefaultCollectLimit {
		if i != 0 {
			body.WriteByte(',')
		}
		fmt.Fprintf(&body, `{"id":"%d","name":"Invoice sync"}`, i)
	}
	body.WriteString(`],"nextCursor":"more"}`)
	from.body = body.String()
	got := from.run(diffArgs("--name", "Invoice sync")...)
	if got.code != 2 || !strings.Contains(got.stderr, "limit before uniqueness was established") {
		t.Fatalf("truncated lookup: %+v", got)
	}
}

func TestWorkflowDiffValidationErrorsHaveDistinctExitCode(t *testing.T) {
	from, to := diffFixtures(t)
	cases := [][]string{
		{"workflow", "diff"},
		diffArgs("--name", "Invoice sync", "--from-id", "AAA"),
		diffArgs("--name", "Invoice sync", "--to-id", "BBB"),
		diffArgs("--from-id", "AAA"),
		diffArgs("--name", " "),
		diffIDArgs("--from-project-id", "p"),
		diffIDArgs("--only", "unknown"),
		diffIDArgs("--only="),
		diffIDArgs("--output", "yaml"),
		diffIDArgs("--nonexistent-flag"),
		diffIDArgs("unexpected-argument"),
		diffIDArgs("--from-context", "missing"),
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got := from.run(args...)
			if got.code != 2 || got.stdout != "" || got.stderr == "" {
				t.Fatalf("validation error: %+v", got)
			}
		})
	}
	if from.requestCount() != 1 || to.requestCount() != 1 {
		t.Fatal("invalid arguments made API requests")
	}
}

func TestWorkflowDiffIncludeDataAndMissingTarget(t *testing.T) {
	from, to := diffFixtures(t)
	to.body = strings.Replace(to.body, `"pinData":{}`, `"pinData":{"Webhook":[1]}`, 1)
	got := from.run(diffIDArgs("--include-pinned-data", "--quiet")...)
	if got.code != 1 || from.lastRequest().URL.Query().Has("excludePinnedData") || to.lastRequest().URL.Query().Has("excludePinnedData") {
		t.Fatalf("pins not included: %+v", got)
	}
	to.body = strings.Replace(to.body, `"staticData":null`, `"staticData":{"last":1}`, 1)
	got = from.run(diffIDArgs("--include-static-data", "--quiet")...)
	if got.code != 1 || to.lastRequest().URL.Query().Get("excludePinnedData") != "true" {
		t.Fatalf("state not included: %+v", got)
	}
	to.status = http.StatusNotFound
	got = from.run(diffIDArgs("--quiet")...)
	if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, "target") || !strings.Contains(got.stderr, "BBB") {
		t.Fatalf("missing target must be an error: %+v", got)
	}
}

func TestWorkflowDiffCancellation(t *testing.T) {
	from, _ := diffFixtures(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut strings.Builder
	code := Run(ctx, diffIDArgs(), Options{
		Streams:   Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		ConfigDir: from.dir, Keyring: from.keyring,
	})
	if code != ExitCanceled || out.Len() != 0 || !strings.Contains(errOut.String(), "canceled") {
		t.Fatalf("cancellation: code=%d out=%s err=%s", code, &out, &errOut)
	}
}
