package cli

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestEscapeScriptField(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"Invoice sync", "Invoice sync"},
		{"  padded  ", "  padded  "},
		{"Café ✓ 🚀 日本", "Café ✓ 🚀 日本"},
		{`quote "double" 'single' $HOME; rm -rf * | & > <`, `quote "double" 'single' $HOME; rm -rf * | & > <`},
		{"a\tb", `a\tb`},
		{"a\nb", `a\nb`},
		{"a\rb", `a\rb`},
		{`a\b`, `a\\b`},
		{`literal \n`, `literal \\n`},
		{"real \n and \\n", `real \n and \\n`},
		{"\\\t", `\\\t`},
		{"esc\x1b[31mred", `esc\u001b[31mred`},
		{"nul\x00bell\x07", `nul\u0000bell\u0007`},
		{"del\x7f", `del\u007f`},
		{"c1\u0085\u009b", `c1\u0085\u009b`},
		{"line\u2028para\u2029", `line\u2028para\u2029`},
	} {
		if got := escapeScriptField(tc.in); got != tc.want {
			t.Errorf("escapeScriptField(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := escapeScriptField(tc.in); strings.ContainsAny(got, "\t\n\r") {
			t.Errorf("escapeScriptField(%q) = %q still breaks a row", tc.in, got)
		}
	}
}

type pair struct{ id, name, kind string }

func pairRow(p pair) scriptListRow { return scriptListRow{ID: p.id, Fields: []string{p.name, p.kind}} }

func TestWriteScriptListFormats(t *testing.T) {
	items := []pair{
		{"wf-1", "Invoice sync", "team"},
		{"id:with/punct-ü", " spaced name ", ""},
		{"3", "tab\there\nnewline", `back\slash`},
	}
	var quiet strings.Builder
	if err := writeScriptList(&quiet, "workflow", items, true, pairRow); err != nil {
		t.Fatal(err)
	}
	if want := "wf-1\nid:with/punct-ü\n3\n"; quiet.String() != want {
		t.Errorf("quiet = %q, want %q", quiet.String(), want)
	}
	var brief strings.Builder
	if err := writeScriptList(&brief, "workflow", items, false, pairRow); err != nil {
		t.Fatal(err)
	}
	want := "wf-1\tInvoice sync\tteam\n" +
		"id:with/punct-ü\t spaced name \t\n" +
		"3\ttab\\there\\nnewline\tback\\\\slash\n"
	if brief.String() != want {
		t.Errorf("brief = %q, want %q", brief.String(), want)
	}
	for _, line := range strings.Split(strings.TrimSuffix(brief.String(), "\n"), "\n") {
		if n := strings.Count(line, "\t"); n != 2 {
			t.Errorf("row %q has %d tabs, want 2", line, n)
		}
	}

	for _, empty := range [][]pair{nil, {}} {
		var out strings.Builder
		if err := writeScriptList(&out, "workflow", empty, false, pairRow); err != nil || out.Len() != 0 {
			t.Errorf("empty list wrote %q, err %v; want zero bytes", out.String(), err)
		}
	}
}

func TestWriteScriptListRejectsMalformedIDsBeforeWriting(t *testing.T) {
	for _, bad := range []string{"", "a\tb", "a\nb", "a\rb", "esc\x1b", "del\x7f", "c1\u0085", "line\u2028", "para\u2029"} {
		for _, items := range [][]pair{{{bad, "first", ""}}, {{"ok-1", "first", ""}, {"ok-2", "second", ""}, {bad, "later", ""}}} {
			for _, idsOnly := range []bool{true, false} {
				var out strings.Builder
				err := writeScriptList(&out, "tag", items, idsOnly, pairRow)
				if err == nil {
					t.Errorf("ID %q was accepted", bad)
					continue
				}
				if out.Len() != 0 {
					t.Errorf("ID %q left partial output %q", bad, out.String())
				}
				if !strings.Contains(err.Error(), "--output json") || strings.ContainsFunc(err.Error(), isScriptControl) {
					t.Errorf("ID %q error = %q, want an escaped message pointing at JSON", bad, err)
				}
			}
		}
	}
}

// failingWriter accepts limit bytes and then fails, like a closed pipe.
type failingWriter struct {
	limit   int
	written strings.Builder
}

var errBrokenPipe = errors.New("broken pipe")

func (w *failingWriter) Write(p []byte) (int, error) {
	room := w.limit - w.written.Len()
	if room >= len(p) {
		w.written.Write(p)
		return len(p), nil
	}
	if room > 0 {
		w.written.Write(p[:room])
	}
	return max(room, 0), errBrokenPipe
}

func TestWriteScriptListPropagatesWriterErrors(t *testing.T) {
	items := []pair{{"1", "one", ""}, {"2", "two", ""}, {"3", "three", ""}}
	for _, limit := range []int{0, 5} {
		w := &failingWriter{limit: limit}
		if err := writeScriptList(w, "tag", items, false, pairRow); !errors.Is(err, errBrokenPipe) {
			t.Errorf("limit %d: err = %v, want the writer error", limit, err)
		}
	}
}

func TestListViewValidate(t *testing.T) {
	for _, tc := range []struct {
		view   listViewFlags
		output string
		want   string
	}{
		{listViewFlags{}, outputText, ""},
		{listViewFlags{}, outputJSON, ""},
		{listViewFlags{quiet: true}, outputText, ""},
		{listViewFlags{brief: true}, outputText, ""},
		{listViewFlags{quiet: true, brief: true}, outputText, "--quiet cannot be combined with --brief"},
		{listViewFlags{quiet: true}, outputJSON, "cannot be combined with --output json"},
		{listViewFlags{brief: true}, outputJSON, "cannot be combined with --output json"},
		{listViewFlags{quiet: true}, "yaml", "unknown output format"},
		{listViewFlags{}, "yaml", "unknown output format"},
	} {
		err := tc.view.validate(tc.output)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%+v %s: err = %v, want nil", tc.view, tc.output, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%+v %s: err = %v, want %q", tc.view, tc.output, err, tc.want)
		}
	}
}

// scriptListCase is one core list command and a two-row page for it.
type scriptListCase struct {
	name      string
	args      []string
	rows      string // two JSON items, comma-separated
	quiet     string
	brief     string
	emptyHint string
	table     string // a header only the default table prints
}

func scriptListCases() []scriptListCase {
	return []scriptListCase{
		{
			name: "workflow", args: []string{"workflow", "list"},
			rows:  cliWorkflow + `,{"id":"wf-2","name":"Other\tname"}`,
			quiet: "wf-1\nwf-2\n", brief: "wf-1\tInvoice sync\nwf-2\tOther\\tname\n",
			emptyHint: "No workflows were returned", table: "PUBLISHED",
		},
		{
			name: "execution", args: []string{"execution", "list"},
			rows:  cliExecution + `,{"id":"1001","status":"error","mode":"trigger","finished":false}`,
			quiet: "1000\n1001\n", brief: "1000\twf-1\tsuccess\n1001\t\terror\n",
			emptyHint: "No executions were returned", table: "STARTED",
		},
		{
			name: "credential", args: []string{"credential", "list"},
			rows:  cliCredential + `,{"id":"cred-2","name":"Slack","type":"slackApi"}`,
			quiet: "credential-id\ncred-2\n", brief: "credential-id\tGitHub production\tgithubApi\ncred-2\tSlack\tslackApi\n",
			emptyHint: "No credentials were returned", table: "SHARED",
		},
		{
			name: "tag", args: []string{"tag", "list"},
			rows:  cliTag + `,{"id":"tag-2","name":"Finance"}`,
			quiet: "tag-id\ntag-2\n", brief: "tag-id\tProduction\ntag-2\tFinance\n",
			emptyHint: "No tags were returned", table: "CREATED",
		},
		{
			name: "project", args: []string{"project", "list"},
			rows:  cliProject + `,{"id":"pr-2","name":"Personal"}`,
			quiet: "pr-1\npr-2\n", brief: "pr-1\tBilling automation\tteam\npr-2\tPersonal\t\n",
			emptyHint: "No projects were returned", table: "TYPE",
		},
	}
}

func scriptListFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestScriptListViewsOnCoreLists(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := scriptListFixture(t, `{"data":[`+tc.rows+`],"nextCursor":"next/page?x=1"}`)
			for _, view := range []struct {
				args []string
				want string
			}{
				{[]string{"--quiet"}, tc.quiet},
				{[]string{"--brief"}, tc.brief},
				{[]string{"--quiet", "--output", "text"}, tc.quiet},
				{[]string{"--brief", "--output", "text"}, tc.brief},
			} {
				before := f.requestCount()
				got := f.run(append(tc.args, view.args...)...)
				if got.code != ExitSuccess {
					t.Fatalf("%v exit = %d, stderr: %s", view.args, got.code, got.stderr)
				}
				if got.stdout != view.want {
					t.Errorf("%v stdout = %q, want %q", view.args, got.stdout, view.want)
				}
				if want := "More " + tc.name + "s: pass --cursor next/page?x=1 for the next page"; !strings.Contains(got.stderr, want) {
					t.Errorf("%v stderr = %q, want %q", view.args, got.stderr, want)
				}
				if f.requestCount()-before != 1 {
					t.Errorf("%v sent %d requests, want 1", view.args, f.requestCount()-before)
				}
			}

			for _, off := range [][]string{{"--quiet=false"}, {"--brief=false"}, {"--quiet=false", "--brief=false"}} {
				got := f.run(append(tc.args, off...)...)
				if got.code != ExitSuccess || !strings.Contains(got.stdout, tc.table) || !strings.Contains(got.stdout, "Next cursor:") {
					t.Errorf("%v result = %+v, want the default table", off, got)
				}
			}
		})
	}
}

func TestScriptListViewsEmptyResult(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := scriptListFixture(t, `{"data":[],"nextCursor":null}`)
			for _, flag := range []string{"--quiet", "--brief"} {
				got := f.run(append(tc.args, flag)...)
				if got.code != ExitSuccess || got.stdout != "" {
					t.Errorf("%s result = %+v, want exit 0 and zero stdout bytes", flag, got)
				}
				if !strings.Contains(got.stderr, tc.emptyHint) || strings.Contains(got.stderr, "--cursor") {
					t.Errorf("%s stderr = %q, want only the empty hint", flag, got.stderr)
				}
			}
		})
	}
}

func TestScriptListViewsFollowAllPages(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			first, second, _ := strings.Cut(tc.rows, `,{"id"`)
			second = `{"id"` + second
			f := scriptListFixture(t, "")
			f.bodyFunc = func(page int) string {
				if page == 0 {
					return `{"data":[` + first + `],"nextCursor":"c2"}`
				}
				return `{"data":[` + second + `],"nextCursor":null}`
			}
			before := f.requestCount()
			got := f.run(append(tc.args, "--all", "--limit", "1", "--brief")...)
			if got.code != ExitSuccess || got.stdout != tc.brief {
				t.Fatalf("result = %+v, want %q", got, tc.brief)
			}
			if strings.Contains(got.stderr, "--cursor") {
				t.Errorf("stderr = %q, want no cursor hint after --all", got.stderr)
			}
			if f.requestCount()-before != 2 {
				t.Fatalf("requests = %d, want 2", f.requestCount()-before)
			}
			if cursor := f.request(before + 1).URL.Query().Get("cursor"); cursor != "c2" {
				t.Errorf("second page cursor = %q, want c2", cursor)
			}
			if limit := f.request(before + 1).URL.Query().Get("limit"); limit != "1" {
				t.Errorf("second page limit = %q, want the page size kept", limit)
			}
		})
	}
}

func TestScriptListViewsRejectConflictsBeforeTransport(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := scriptListFixture(t, `{"data":[],"nextCursor":null}`)
			before := f.requestCount()
			for _, c := range []struct {
				args []string
				want string
			}{
				{[]string{"--quiet", "--brief"}, "--quiet cannot be combined with --brief"},
				{[]string{"--quiet", "--output", "json"}, "cannot be combined with --output json"},
				{[]string{"--brief", "--output", "json"}, "cannot be combined with --output json"},
				{[]string{"--quiet", "--output", "yaml"}, "unknown output format"},
			} {
				got := f.run(append(tc.args, c.args...)...)
				if got.code != ExitError || got.stdout != "" || !strings.Contains(got.stderr, c.want) {
					t.Errorf("%v result = %+v, want %q", c.args, got, c.want)
				}
			}
			if f.requestCount() != before {
				t.Error("a conflicting view flag reached the instance")
			}
		})
	}
}

func TestScriptListViewsRejectMalformedServerID(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			first, _, _ := strings.Cut(tc.rows, `,{"id"`)
			f := scriptListFixture(t, `{"data":[`+first+`,{"id":"bad\nid","name":"x"}],"nextCursor":null}`)
			for _, flag := range []string{"--quiet", "--brief"} {
				got := f.run(append(tc.args, flag)...)
				if got.code != ExitError || got.stdout != "" {
					t.Errorf("%s result = %+v, want an error with no rows", flag, got)
				}
				if !strings.Contains(got.stderr, `"bad\nid"`) || !strings.Contains(got.stderr, "--output json") {
					t.Errorf("%s stderr = %q, want the escaped ID and a JSON pointer", flag, got.stderr)
				}
			}
		})
	}
}

func TestScriptListViewsKeepAPIErrors(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := scriptListFixture(t, "")
			f.status = http.StatusForbidden
			for _, flag := range []string{"--quiet", "--brief"} {
				got := f.run(append(tc.args, flag)...)
				if got.code != ExitError || got.stdout != "" || !strings.Contains(got.stderr, "discover --resource") {
					t.Errorf("%s result = %+v, want the scope diagnostic and no rows", flag, got)
				}
			}
			f.status = http.StatusUnauthorized
			got := f.run(append(tc.args, "--quiet")...)
			if got.code != ExitError || got.stdout != "" || !strings.Contains(got.stderr, "n8n auth login") {
				t.Errorf("401 result = %+v, want the login remediation", got)
			}
		})
	}
}

func TestScriptListViewsReportWriteFailure(t *testing.T) {
	for _, tc := range scriptListCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := scriptListFixture(t, `{"data":[`+tc.rows+`],"nextCursor":null}`)
			var errOut strings.Builder
			interactive := false
			code := Run(context.Background(), append(tc.args, "--brief"), Options{
				Streams:     Streams{In: strings.NewReader(""), Out: &failingWriter{limit: 3}, Err: &errOut},
				Version:     testVersion,
				ConfigDir:   f.dir,
				Env:         func(name string) string { return f.env[name] },
				Keyring:     f.keyring,
				Prompt:      f.prompt,
				Interactive: &interactive,
			})
			if code != ExitError || !strings.Contains(errOut.String(), errBrokenPipe.Error()) {
				t.Errorf("exit = %d stderr %q, want the write failure reported", code, errOut.String())
			}
		})
	}
}

func TestCredentialScriptViewsNeverPrintSecrets(t *testing.T) {
	f := credentialFixture(t, `{"data":[`+cliCredential+`],"nextCursor":null}`)
	for _, flag := range []string{"--quiet", "--brief"} {
		got := f.run("credential", "list", flag)
		if got.code != ExitSuccess {
			t.Fatalf("%s exit = %d, stderr: %s", flag, got.code, got.stderr)
		}
		assertNoCredentialSecret(t, got)
	}
}

// Script views change rendering only: the request a view sends matches the
// one the default table sends.
func TestScriptListViewsSendTheSameRequest(t *testing.T) {
	for _, tc := range []struct {
		body string
		args []string
	}{
		{`{"data":[` + cliWorkflow + `],"nextCursor":null}`, []string{"workflow", "list", "--exclude-pinned-data", "--tag", "production", "--active", "true"}},
		{`{"data":[` + cliExecution + `],"nextCursor":null}`, []string{"execution", "list", "--include-data", "--status", "error", "--workflow-id", "wf-1", "--limit", "5"}},
	} {
		f := scriptListFixture(t, tc.body)
		got := f.run(tc.args...)
		if got.code != ExitSuccess {
			t.Fatalf("%v exit = %d, stderr: %s", tc.args, got.code, got.stderr)
		}
		want := f.lastRequest().URL.RawQuery
		for _, flag := range []string{"--quiet", "--brief"} {
			before := f.requestCount()
			got := f.run(append(tc.args, flag)...)
			if got.code != ExitSuccess || f.requestCount()-before != 1 {
				t.Fatalf("%v %s result = %+v, requests %d", tc.args, flag, got, f.requestCount()-before)
			}
			if query := f.lastRequest().URL.RawQuery; query != want {
				t.Errorf("%v %s query = %q, want %q", tc.args, flag, query, want)
			}
		}
	}
}
