package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliPromotionProviderBody = `{
  "id":"prov-1",
  "name":"github",
  "type":"git",
  "authType":"ssh-key",
  "config":{"schemaVersion":1,"publicKey":"ssh-ed25519 AAAAEBBY","keyType":"ed25519"},
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const cliPromotionProviderCreateBody = `{
  "provider":` + cliPromotionProviderBody + `,
  "publicKey":"ssh-ed25519 AAAAEBBY"
}`

const cliPromotionConnectionBody = `{
  "id":"conn-1",
  "name":"prod",
  "scope":"projects",
  "target":{"schemaVersion":1,"remoteUrl":"git@example.com:org/repo.git"},
  "provider":{"id":"prov-1","name":"github","type":"git","authType":"ssh-key","createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z"},
  "configs":{
    "apply":{"id":"cfg-apply","name":"apply","createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z","settings":{"schemaVersion":1,"branchName":"main"}},
    "promote":{"id":"cfg-promote","name":"promote","createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z","settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":false}}
  },
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const cliPromotionApplyConfigBody = `{
  "id":"cfg-apply",
  "name":"apply",
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z",
  "settings":{"schemaVersion":1,"branchName":"main"}
}`

const cliPromotionPromoteConfigBody = `{
  "id":"cfg-promote",
  "name":"promote",
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z",
  "settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":true}
}`

const cliPromotionCheckoutBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "direction":"apply",
  "branchName":"main",
  "hasCheckout":true
}`

const cliPromotionPromoteBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-promote",
  "counts":{"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0,"tags":3},
  "git":{"commitSha":"deadbeef","branchName":"main"}
}`

const cliPromotionApplyBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "counts":{
    "projects":{"created":1,"updated":0,"skipped":0,"deleted":0},
    "folders":{"created":1,"skipped":0,"removed":0},
    "workflows":{"created":1,"updated":0,"skipped":0,"archived":0,"deleted":0,"publishing":{"published":1,"unpublished":0,"unchanged":0,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0},
    "dataTables":{"matched":0,"created":0},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":1,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "git":{"commitSha":"deadbeef","branchName":"main"}
}`

func promotionFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writePromotionInput(t *testing.T, f *fixture, name, body string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.dir), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write promotion input: %v", err)
	}
	return path
}

func TestPromotionProviderListTextAndJSON(t *testing.T) {
	f := promotionFixture(t, `{"data":[`+cliPromotionProviderBody+`],"nextCursor":"next"}`)

	got := f.run("promotion", "provider", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Providers:", "1", "prov-1", "github", "git", "ssh-key"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.PromotionProvidersPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("promotion", "provider", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.PromotionProvider]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 1 || page.NextCursor != "next" {
		t.Errorf("page = %+v, want one provider with a cursor", page)
	}
}

func TestPromotionProviderListEmptyAndPagination(t *testing.T) {
	f := promotionFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("promotion", "provider", "list")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "No promotion providers") {
		t.Errorf("result = %+v, want the empty hint", got)
	}

	f.bodyFunc = func(i int) string {
		if i == 0 {
			return `{"data":[` + cliPromotionProviderBody + `],"nextCursor":"next"}`
		}
		return `{"data":[],"nextCursor":null}`
	}
	got = f.run("promotion", "provider", "list", "--all", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("all exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.PromotionProvider]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 {
		t.Errorf("all = %q (error %v), want one collected provider", got.stdout, err)
	}

	before := f.requestCount()
	got = f.run("promotion", "provider", "list", "--limit", "251")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("large limit result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPromotionProviderCreateFromInput(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderCreateBody)

	path := writePromotionInput(t, f, "provider.json", `{"name":"github","type":"git","auth":{"authType":"ssh-key"}}`)
	got := f.run("promotion", "provider", "create", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.PromotionProvidersPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.CreatePromotionProviderRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent.Name != "github" {
		t.Errorf("body = %q (error %v)", f.lastBody(), err)
	}
	for _, want := range []string{"Created:", "prov-1", "github"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	f.stdin = `{"name":"github","type":"git","auth":{"authType":"token","username":"git","password":"secret"}}`
	got = f.run("promotion", "provider", "create", "--input", "-", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}
	var created n8n.PromotionProviderCreateResult
	if err := json.Unmarshal([]byte(got.stdout), &created); err != nil || created.Provider.ID != "prov-1" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionProviderCreateRejectsBadInputBeforeTransport(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderCreateBody)
	before := f.requestCount()

	path := writePromotionInput(t, f, "provider-bad.json", `{"name":"github","unknown":true}`)
	cases := [][]string{
		{"promotion", "provider", "create"},
		{"promotion", "provider", "create", "--input", path},
		{"promotion", "provider", "create", "--input", path, "--output", "yaml"},
	}
	f.stdin = `{"name":""}`
	cases = append(cases, []string{"promotion", "provider", "create", "--input", "-"})
	for _, args := range cases {
		got := f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v exit = %d, want %d (stderr: %s)", args, got.code, ExitError, got.stderr)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want %d before transport", f.requestCount(), before)
	}
}

func TestPromotionProviderGetTextAndJSON(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderBody)

	got := f.run("promotion", "provider", "get", "prov-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"prov-1", "github", "ssh-key"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stdout, "AAAA") {
		t.Errorf("stdout missing the public-key prefix:\n%s", got.stdout)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+"/promotions/providers/prov-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("promotion", "provider", "get", "prov-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var provider n8n.PromotionProvider
	if err := json.Unmarshal([]byte(got.stdout), &provider); err != nil || provider.ID != "prov-1" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionProviderGetEscapesID(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderBody)

	got := f.run("promotion", "provider", "get", "a/b c")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(f.lastRequest().URL.EscapedPath(), "a%2Fb%20c") {
		t.Errorf("path = %q, want the ID escaped", f.lastRequest().URL.EscapedPath())
	}
}

func TestPromotionProviderUpdateConfirmation(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderBody)
	before := f.requestCount()
	f.stdin = "no\n"

	path := writePromotionInput(t, f, "provider-update.json", `{"name":"github-new"}`)
	got := f.run("promotion", "provider", "update", "prov-1", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "every connection") {
		t.Errorf("prompt = %q, want the attached-connection consequence", got.stderr)
	}

	f.interactive = false
	got = f.run("promotion", "provider", "update", "prov-1", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "provider", "update", "prov-1", "--input", path, "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/promotions/providers/prov-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	before = f.requestCount()
	empty := writePromotionInput(t, f, "provider-empty.json", `{}`)
	got = f.run("promotion", "provider", "update", "prov-1", "--input", empty, "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("empty result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPromotionProviderDeleteConfirmation(t *testing.T) {
	f := promotionFixture(t, ``)
	f.status = http.StatusNoContent
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("promotion", "provider", "delete", "prov-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.interactive = false
	got = f.run("promotion", "provider", "delete", "prov-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "provider", "delete", "prov-1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/promotions/providers/prov-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPromotionConnectionListTextAndJSON(t *testing.T) {
	f := promotionFixture(t, `{"data":[`+cliPromotionConnectionBody+`],"nextCursor":"next"}`)

	got := f.run("promotion", "connection", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Connections:", "1", "conn-1", "prod", "projects", "git@example.com:org/repo.git"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.PromotionConnectionsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	got = f.run("promotion", "connection", "list", "--scope", "projects", "--provider-id", "prov-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("filtered exit = %d, stderr: %s", got.code, got.stderr)
	}
	query := f.lastRequest().URL.Query()
	if query.Get("scope") != "projects" || query.Get("providerId") != "prov-1" {
		t.Errorf("query = %q, want scope and providerId", f.lastRequest().URL.RawQuery)
	}
	var page n8n.Page[n8n.PromotionConnection]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	before := f.requestCount()
	got = f.run("promotion", "connection", "list", "--scope", "global")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("bad scope result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPromotionConnectionCreateGetUpdateDelete(t *testing.T) {
	f := promotionFixture(t, cliPromotionConnectionBody)

	path := writePromotionInput(t, f, "pconn.json", `{"name":"prod","scope":"projects","providerId":"prov-1","target":{"schemaVersion":1,"remoteUrl":"git@example.com:org/repo.git"}}`)
	got := f.run("promotion", "connection", "create", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("create exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.PromotionConnectionsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	for _, want := range []string{"Created:", "conn-1", "prod"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	got = f.run("promotion", "connection", "get", "conn-1")
	if got.code != ExitSuccess {
		t.Fatalf("get exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	update := writePromotionInput(t, f, "pconn-update.json", `{"name":"staging"}`)
	got = f.run("promotion", "connection", "update", "conn-1", "--input", update)
	if got.code != ExitSuccess {
		t.Fatalf("update exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	before := f.requestCount()
	empty := writePromotionInput(t, f, "pconn-empty.json", `{}`)
	got = f.run("promotion", "connection", "update", "conn-1", "--input", empty)
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("empty result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.status = http.StatusNoContent
	f.body = ``
	f.stdin = "no\n"
	got = f.run("promotion", "connection", "delete", "conn-1")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "connection", "delete", "conn-1", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("delete exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPromotionConfigSetAndDelete(t *testing.T) {
	f := promotionFixture(t, cliPromotionApplyConfigBody)

	apply := writePromotionInput(t, f, "apply.json", `{"settings":{"schemaVersion":1,"branchName":"main"}}`)
	f.stdin = "no\n"
	before := f.requestCount()
	got := f.run("promotion", "config", "apply", "set", "conn-1", "--input", apply)
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined apply result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "config", "apply", "set", "conn-1", "--input", apply, "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("apply exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/configs/apply" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	f.body = cliPromotionPromoteConfigBody
	promote := writePromotionInput(t, f, "promote.json", `{"settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":true}}`)
	got = f.run("promotion", "config", "promote", "set", "conn-1", "--input", promote, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("promote exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/configs/promote" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var stored n8n.PromotionPromoteConfig
	if err := json.Unmarshal([]byte(got.stdout), &stored); err != nil || stored.ID != "cfg-promote" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	f.status = http.StatusNoContent
	f.body = ``
	f.stdin = "no\n"
	before = f.requestCount()
	got = f.run("promotion", "config", "delete", "conn-1", "apply")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined delete result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "config", "delete", "conn-1", "apply", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("delete exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/configs/apply" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	before = f.requestCount()
	got = f.run("promotion", "config", "delete", "conn-1", "sideways", "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("bad direction result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPromotionCheckoutCloneDisconnect(t *testing.T) {
	f := promotionFixture(t, cliPromotionCheckoutBody)

	got := f.run("promotion", "checkout", "clone", "conn-1", "apply")
	if got.code != ExitSuccess {
		t.Fatalf("clone exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/apply/clone" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	for _, want := range []string{"conn-1", "apply", "main"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	before := f.requestCount()
	got = f.run("promotion", "checkout", "clone", "conn-1", "sideways")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("bad direction result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	f.stdin = "no\n"
	got = f.run("promotion", "checkout", "disconnect", "conn-1", "apply")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "checkout", "disconnect", "conn-1", "apply", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("disconnect exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/apply/disconnect" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPromotionProjectListAddRemove(t *testing.T) {
	f := promotionFixture(t, `{"projectIds":["p1","p2"]}`)

	got := f.run("promotion", "project", "list", "conn-1")
	if got.code != ExitSuccess {
		t.Fatalf("list exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"p1", "p2"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	f.body = `{"projectId":"p1","connectionId":"conn-1"}`
	got = f.run("promotion", "project", "add", "conn-1", "p1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("add exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}

	f.status = http.StatusNoContent
	f.body = ``
	before := f.requestCount()
	f.stdin = "no\n"
	got = f.run("promotion", "project", "remove", "conn-1", "p1")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	got = f.run("promotion", "project", "remove", "conn-1", "p1", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("remove exit = %d, stderr: %s", got.code, got.stderr)
	}
	req = f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPromotionPromoteConfirmationAndBody(t *testing.T) {
	f := promotionFixture(t, cliPromotionPromoteBody)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("promotion", "promote", "conn-1", "--commit-message", "sync")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "remote") {
		t.Errorf("prompt = %q, want the remote-side effect", got.stderr)
	}

	f.interactive = false
	got = f.run("promotion", "promote", "conn-1", "--commit-message", "sync")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "promote", "conn-1", "--commit-message", "sync", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/promote" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent n8n.PromotePromotionRequest
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent.CommitMessage != "sync" || sent.Force {
		t.Errorf("request = %q (error %v)", f.lastBody(), err)
	}
	var result n8n.PromotePromotionResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Git.CommitSHA != "deadbeef" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}

	before = f.requestCount()
	got = f.run("promotion", "promote", "conn-1", "--yes")
	if got.code != ExitError || f.requestCount() != before {
		t.Errorf("missing commit result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
}

func TestPromotionApplyConfirmationAndOutput(t *testing.T) {
	f := promotionFixture(t, cliPromotionApplyBody)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("promotion", "apply", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "overwrite") {
		t.Errorf("prompt = %q, want the overwrite warning", got.stderr)
	}

	f.interactive = false
	got = f.run("promotion", "apply", "conn-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("promotion", "apply", "conn-1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/promotions/connections/conn-1/apply" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var result n8n.ApplyPromotionResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Git.CommitSHA != "deadbeef" {
		t.Errorf("stdout = %q (error %v)", got.stdout, err)
	}
}

func TestPromotionDenialsAreReported(t *testing.T) {
	f := promotionFixture(t, `{"data":[]}`)

	f.status = http.StatusUnauthorized
	got := f.run("promotion", "provider", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "n8n auth login") {
		t.Errorf("401 result = %+v, want the credential hint", got)
	}

	f.status = http.StatusForbidden
	got = f.run("promotion", "promote", "conn-1", "--commit-message", "sync", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "gitConnection:push") || !strings.Contains(got.stderr, "n8n discover --resource promotions") {
		t.Errorf("403 result = %+v, want scope and discover hint", got)
	}

	f.status = http.StatusNotFound
	got = f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "does not serve promotions") || !strings.Contains(got.stderr, "n8n discover --resource promotions") {
		t.Errorf("404 result = %+v, want the availability hint", got)
	}

	f.status = http.StatusServiceUnavailable
	got = f.run("promotion", "provider", "get", "prov-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "does not serve promotions") {
		t.Errorf("503 result = %+v, want the availability hint", got)
	}

	f.status = http.StatusConflict
	f.body = `{"message":"exists"}`
	path := writePromotionInput(t, f, "provider-conflict.json", `{"name":"github","type":"git","auth":{"authType":"ssh-key"}}`)
	got = f.run("promotion", "provider", "create", "--input", path)
	if got.code != ExitError || !strings.Contains(got.stderr, "409") {
		t.Errorf("409 result = %+v, want the conflict guidance", got)
	}

	f.status = http.StatusBadRequest
	got = f.run("promotion", "provider", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "400") {
		t.Errorf("400 result = %+v, want the bad-request guidance", got)
	}

	f.status = http.StatusConflict
	f.body = `{"message":"conflicted"}`
	got = f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "409") {
		t.Errorf("apply 409 result = %+v, want the conflict guidance", got)
	}

	f.status = http.StatusUnprocessableEntity
	f.body = `{"message":"unresolvable"}`
	got = f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "422") {
		t.Errorf("apply 422 result = %+v, want the unprocessable guidance", got)
	}
}

func TestPromotionSecretsNeverReachOutput(t *testing.T) {
	f := promotionFixture(t, cliPromotionProviderCreateBody)

	path := writePromotionInput(t, f, "provider-secret.json", `{"name":"github","type":"git","auth":{"authType":"token","username":"git","password":"hunter2-super-secret"}}`)
	got := f.run("promotion", "provider", "create", "--input", path, "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, "hunter2") || strings.Contains(got.stderr, "hunter2") {
		t.Errorf("secret leaked into output:\n--- stdout ---\n%s\n--- stderr ---\n%s", got.stdout, got.stderr)
	}
	if !strings.Contains(f.lastBody(), "hunter2") {
		t.Errorf("request body lost the password: %q", f.lastBody())
	}
}
