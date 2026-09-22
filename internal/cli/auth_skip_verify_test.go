package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

func TestAuthLoginSkipVerifyAllowsResourceAccess(t *testing.T) {
	for _, discoveryStatus := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(discoveryStatus), func(t *testing.T) {
			f := newFixture(t)
			f.route = func(r *http.Request, _ int) (int, string) {
				if r.Method != http.MethodGet || r.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
					t.Error("request must use GET and the stored API key")
				}
				switch r.URL.Path {
				case n8n.BasePath + n8n.DiscoverPath:
					return discoveryStatus, `{"message":"discovery unavailable"}`
				case n8n.BasePath + n8n.UsersPath:
					return http.StatusOK, `{"data":[` + cliUser + `],"nextCursor":null}`
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					return http.StatusNotFound, `{}`
				}
			}

			// Default login still verifies, with an actionable error and no writes.
			got := f.run("auth", "login", "--url", f.server.URL, "--context", "test")
			if got.code != ExitError || !strings.Contains(got.stderr, "--skip-verify") || !strings.Contains(got.stderr, "nothing was saved") {
				t.Fatalf("default login: %+v", got)
			}
			if f.requestCount() != 1 || len(f.config().Contexts) != 0 || f.keyring.(*config.MemoryStore).Len() != 0 {
				t.Fatal("default login must verify once and save nothing on failure")
			}

			f.interactive = false
			f.stdin = testAPIKey + "\n"
			got = f.login("--context", "test", "--stdin", "--skip-verify")
			if f.requestCount() != 1 {
				t.Fatal("--skip-verify sent an HTTP request")
			}
			if got.stdout != "" || !strings.Contains(got.stderr, "verification skipped") || strings.Contains(got.stderr, "Logged in") || strings.Contains(got.stderr, testAPIKey) {
				t.Fatalf("misleading or unsafe login output: %+v", got)
			}
			cfg := f.config()
			if cfg.CurrentContext != "test" || cfg.Contexts["test"].URL != f.server.URL {
				t.Fatalf("context was not saved and selected: %+v", cfg)
			}

			status := f.run("auth", "status", "--context", "test", "--output", "json")
			var report statusReport
			if err := json.Unmarshal([]byte(status.stdout), &report); err != nil {
				t.Fatal(err)
			}
			if status.code != ExitSuccess || report.Credential != "present" || report.Validated != nil || f.requestCount() != 1 {
				t.Fatalf("local status must not claim verification: %+v", status)
			}

			users := f.run("user", "list", "--context", "test", "--limit", "1", "--output", "json")
			var page n8n.Page[n8n.User]
			if err := json.Unmarshal([]byte(users.stdout), &page); err != nil {
				t.Fatal(err)
			}
			if users.code != ExitSuccess || len(page.Data) != 1 || page.Data[0].Email != "person@example.com" || f.requestCount() != 2 {
				t.Fatalf("saved context must allow user listing directly: %+v", users)
			}
			if checked := f.run("auth", "status", "--context", "test", "--check"); checked.code != ExitError {
				t.Fatal("--check must still fail when discovery is unavailable")
			}
			if discover := f.run("discover", "--context", "test"); discover.code != ExitError {
				t.Fatal("discovery must still fail")
			}
		})
	}
}

func TestAuthLoginSkipVerifyWorksOffline(t *testing.T) {
	for _, authType := range []string{"api-key", "bearer", "cookie"} {
		t.Run(authType, func(t *testing.T) {
			f := newFixture(t)
			f.server.Close()
			f.login("--skip-verify", "--type", authType, "--storage", "file")
			if f.requestCount() != 0 {
				t.Fatal("offline login sent an HTTP request")
			}
			got := f.run("auth", "status", "--output", "json")
			var report statusReport
			if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
				t.Fatal(err)
			}
			if got.code != ExitSuccess || report.AuthType != authType || report.Storage != "file" || report.Credential != "present" {
				t.Fatalf("credential did not survive file storage: %+v", got)
			}
		})
	}
}

func TestAuthLoginSkipVerifyKeepsLocalGuards(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fixture)
		args  []string
		want  string
	}{
		{name: "URL", args: []string{"--url", "ftp://example.com"}, want: "http"},
		{name: "context", args: []string{"--context", "bad name"}, want: "may only contain"},
		{name: "auth type", args: []string{"--type", "basic"}, want: "unknown auth type"},
		{name: "storage", args: []string{"--storage", "vault"}, want: "unknown storage"},
		{name: "empty prompt", setup: func(f *fixture) { f.prompt.secret = "" }, want: "empty"},
		{name: "empty stdin", args: []string{"--stdin"}, want: "empty"},
		{name: "no terminal", setup: func(f *fixture) { f.interactive = false }, want: "--stdin"},
		{name: "storage consent", setup: func(f *fixture) {
			f.keyring = unavailableKeyring{}
			f.interactive = false
			f.stdin = testAPIKey
		}, args: []string{"--stdin", "--yes"}, want: "--storage=file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			args := append([]string{"auth", "login", "--skip-verify", "--url", f.server.URL}, tt.args...)
			got := f.run(args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) || strings.Contains(got.stderr, testAPIKey) {
				t.Fatalf("guard failed: %+v", got)
			}
			if f.requestCount() != 0 || len(f.config().Contexts) != 0 {
				t.Fatal("invalid login contacted server or wrote config")
			}
			if store, ok := f.keyring.(*config.MemoryStore); ok && store.Len() != 0 {
				t.Fatal("invalid login stored a credential")
			}
			if _, err := os.Stat(filepath.Join(f.dir, config.SecretFileName)); !os.IsNotExist(err) {
				t.Fatalf("invalid login must not create auth.json: %v", err)
			}
		})
	}
}

func TestAuthLoginSkipVerifyKeepsReplacementConsent(t *testing.T) {
	f := newFixture(t)
	f.login("--skip-verify")
	f.interactive = false
	f.stdin = "replacement-key\n"
	got := f.run("auth", "login", "--skip-verify", "--stdin")
	if got.code != ExitError || !strings.Contains(got.stderr, "--yes") {
		t.Fatalf("replacement should require consent: %+v", got)
	}
	stored, err := f.keyring.Get(DefaultContextName)
	if err != nil || stored.Value.Reveal() != testAPIKey {
		t.Fatal("refused replacement changed credential")
	}
	got = f.run("auth", "login", "--skip-verify", "--stdin", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed replacement failed: %+v", got)
	}
	stored, err = f.keyring.Get(DefaultContextName)
	if err != nil || stored.Value.Reveal() != "replacement-key" || f.requestCount() != 0 {
		t.Fatal("confirmed replacement must save locally without HTTP")
	}
}
