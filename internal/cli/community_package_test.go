package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliCommunityPackage = `{
  "packageName":"n8n-nodes-example",
  "installedVersion":"1.2.3",
  "authorName":"Example",
  "installedNodes":[{"name":"Example","type":"n8n-nodes-example.example","latestVersion":2}],
  "updateAvailable":"1.3.0",
  "failedLoading":false
}`

func communityPackageFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestCommunityPackageListText(t *testing.T) {
	f := communityPackageFixture(t, `[`+cliCommunityPackage+`]`)
	got := f.run("community-package", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{f.server.URL, "Packages:", "NAME", "INSTALLED", "UPDATE", "STATUS", "NODES", "n8n-nodes-example", "1.2.3", "1.3.0", "loaded"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.CommunityPackagesPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, n8n.BasePath+n8n.CommunityPackagesPath)
	}
	if req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
		t.Error("saved API key was not sent")
	}
}

func TestCommunityPackageListJSON(t *testing.T) {
	f := communityPackageFixture(t, `[`+cliCommunityPackage+`]`)
	got := f.run("community-package", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
	var packages []n8n.CommunityPackage
	if err := json.Unmarshal([]byte(got.stdout), &packages); err != nil {
		t.Fatalf("stdout is not package JSON: %v\n%s", err, got.stdout)
	}
	if len(packages) != 1 || packages[0].PackageName != "n8n-nodes-example" {
		t.Errorf("packages = %+v, want example package", packages)
	}
}

func TestCommunityPackageListEmpty(t *testing.T) {
	f := communityPackageFixture(t, `[]`)
	got := f.run("community-package", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d", got.code, ExitSuccess)
	}
	if !strings.Contains(got.stdout, "Packages:") || !strings.Contains(got.stderr, "No community packages") {
		t.Errorf("stdout, stderr = %q, %q; want empty-list summary", got.stdout, got.stderr)
	}
}

func TestCommunityPackageInstall(t *testing.T) {
	f := communityPackageFixture(t, cliCommunityPackage)
	f.stdin = "yes\n"
	got := f.run("community-package", "install", "n8n-nodes-example", "--version", "1.2.3", "--verify=false")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{"Installed:", "n8n-nodes-example", "Version:", "1.2.3", f.server.URL, "Nodes:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stderr, "Install community package") || !strings.Contains(got.stderr, "code will run") {
		t.Errorf("stderr = %q, want risk confirmation", got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.CommunityPackagesPath {
		t.Errorf("request = %s %s, want POST collection", req.Method, req.URL.Path)
	}
	assertCLIJSON(t, `{"name":"n8n-nodes-example","version":"1.2.3","verify":false}`, f.lastBody())
}

func TestCommunityPackageInstallJSONNonInteractive(t *testing.T) {
	f := communityPackageFixture(t, cliCommunityPackage)
	f.interactive = false
	got := f.run("community-package", "install", "n8n-nodes-example", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want --yes to be quiet", got.stderr)
	}
	var pkg n8n.CommunityPackage
	if err := json.Unmarshal([]byte(got.stdout), &pkg); err != nil || pkg.PackageName != "n8n-nodes-example" {
		t.Errorf("stdout = %q, want package JSON (error: %v)", got.stdout, err)
	}
	assertCLIJSON(t, `{"name":"n8n-nodes-example","verify":true}`, f.lastBody())
}

func TestCommunityPackageUpdateEscapesName(t *testing.T) {
	f := communityPackageFixture(t, cliCommunityPackage)
	name := "@scope/n8n-nodes-example?channel=next#node"
	got := f.run("community-package", "update", name, "--version", "1.3.0", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", req.Method)
	}
	want := n8n.BasePath + "/community-packages/@scope%2Fn8n-nodes-example%3Fchannel=next%23node"
	if req.URL.EscapedPath() != want {
		t.Errorf("escaped path = %q, want %q", req.URL.EscapedPath(), want)
	}
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty", req.URL.RawQuery)
	}
	assertCLIJSON(t, `{"version":"1.3.0","verify":true}`, f.lastBody())
}

func TestCommunityPackageUninstall(t *testing.T) {
	f := communityPackageFixture(t, cliCommunityPackage)
	f.status = http.StatusNoContent
	name := "@scope/n8n-nodes-example"
	got := f.run("community-package", "uninstall", name, "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", req.Method)
	}
	want := n8n.BasePath + "/community-packages/@scope%2Fn8n-nodes-example"
	if req.URL.EscapedPath() != want {
		t.Errorf("escaped path = %q, want %q", req.URL.EscapedPath(), want)
	}
	if body := f.lastBody(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	var result struct {
		Name        string `json:"name"`
		Uninstalled bool   `json:"uninstalled"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if result.Name != name || !result.Uninstalled {
		t.Errorf("result = %+v, want successful uninstall", result)
	}
}

func TestCommunityPackageMutationsRequireConfirmation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "install", args: []string{"community-package", "install", "n8n-nodes-example"}},
		{name: "update", args: []string{"community-package", "update", "n8n-nodes-example"}},
		{name: "uninstall", args: []string{"community-package", "uninstall", "n8n-nodes-example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" declined", func(t *testing.T) {
			f := communityPackageFixture(t, cliCommunityPackage)
			f.stdin = "no\n"
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
				t.Errorf("exit, stderr = %d, %q; want declined error", got.code, got.stderr)
			}
			if f.requestCount() != before {
				t.Error("declined action reached instance")
			}
		})
		t.Run(tt.name+" non-interactive", func(t *testing.T) {
			f := communityPackageFixture(t, cliCommunityPackage)
			f.interactive = false
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") {
				t.Errorf("exit, stderr = %d, %q; want --yes guidance", got.code, got.stderr)
			}
			if f.requestCount() != before {
				t.Error("unconfirmed action reached instance")
			}
		})
	}
}

func TestCommunityPackageRequiresAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		authType n8n.AuthType
		envName  string
	}{
		{name: "bearer", authType: n8n.AuthBearer, envName: config.EnvBearerToken},
		{name: "cookie", authType: n8n.AuthCookie, envName: config.EnvAuthCookie},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.env[config.EnvURL] = f.server.URL
			f.env[tt.envName] = "not-an-api-key"
			got := f.run("community-package", "list")
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d", got.code, ExitError)
			}
			for _, want := range []string{"API-key", string(tt.authType), config.EnvAPIKey} {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
			if f.requestCount() != 0 {
				t.Error("non-API-key command reached instance")
			}
		})
	}
}

func TestCommunityPackageRejectsInvalidInvocationBeforeRequest(t *testing.T) {
	tests := map[string][]string{
		"unknown output":         {"community-package", "list", "--output", "yaml"},
		"list arg":               {"community-package", "list", "extra"},
		"install no name":        {"community-package", "install"},
		"install wrong prefix":   {"community-package", "install", "example", "--yes"},
		"install padded version": {"community-package", "install", "n8n-nodes-example", "--version", " 1.0", "--yes"},
		"update no name":         {"community-package", "update"},
		"update too many names":  {"community-package", "update", "one", "two", "--yes"},
		"uninstall no name":      {"community-package", "uninstall"},
		"unknown action":         {"community-package", "upgrade", "n8n-nodes-example"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			f := communityPackageFixture(t, cliCommunityPackage)
			before := f.requestCount()
			got := f.run(args...)
			if got.code != ExitError || got.stderr == "" {
				t.Errorf("exit, stderr = %d, %q; want invocation error", got.code, got.stderr)
			}
			if f.requestCount() != before {
				t.Error("invalid invocation reached instance")
			}
		})
	}
}

func TestCommunityPackageAPIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"community-package", "list"}, want: []string{"401", "auth login"}},
		{name: "forbidden", status: http.StatusForbidden, args: []string{"community-package", "list"}, want: []string{"403", "scope", "discover --resource community-package"}},
		{name: "install bad request", status: http.StatusBadRequest, args: []string{"community-package", "install", "n8n-nodes-example", "--yes"}, want: []string{"400"}},
		{name: "update missing", status: http.StatusNotFound, args: []string{"community-package", "update", "n8n-nodes-missing", "--yes"}, want: []string{"404"}},
		{name: "uninstall missing", status: http.StatusNotFound, args: []string{"community-package", "uninstall", "n8n-nodes-missing", "--yes"}, want: []string{"404"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := communityPackageFixture(t, cliCommunityPackage)
			f.status = tt.status
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Errorf("exit, stdout = %d, %q; want failure and empty stdout", got.code, got.stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
		})
	}
}

func assertCLIJSON(t *testing.T, want, got string) {
	t.Helper()
	var wantValue, gotValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("expected JSON is invalid: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("actual JSON is invalid: %v: %s", err, got)
	}
	if diff := diffJSON(wantValue, gotValue); diff != "" {
		t.Errorf("JSON differs: %s", diff)
	}
}
