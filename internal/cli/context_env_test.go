package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// envContexts saves two contexts with different URLs and credentials, selecting
// default on disk. Both serve resource lists and credential verification.
func envContexts(t *testing.T, kind config.Storage) (*fixture, *fixture) {
	t.Helper()
	f, work := newFixture(t), newFixture(t)
	route := func(r *http.Request, _ int) (int, string) {
		if r.URL.Path == "/api/v1/users" {
			return http.StatusOK, `{"data":[]}`
		}
		return http.StatusOK, `{"scopes":[]}`
	}
	f.route, work.route = route, route
	f.login("--storage", string(kind))
	f.prompt.secret = "work-secret"
	requireSuccess(t, f.run("auth", "login", "--context", "work", "--url", work.server.URL, "--storage", string(kind)))
	requireSuccess(t, f.run("config", "context", "use", "default"))
	f.env[config.EnvContext] = "work"
	return f, work
}

func TestEnvContextResourceAndStatus(t *testing.T) {
	for _, kind := range []config.Storage{config.StorageKeyring, config.StorageFile} {
		t.Run(string(kind), func(t *testing.T) {
			f, work := envContexts(t, kind)
			before, err := os.ReadFile(filepath.Join(f.dir, config.ConfigFileName))
			if err != nil {
				t.Fatal(err)
			}
			for _, tt := range []struct {
				flag, env, want string
				server          *fixture
				secret          string
			}{
				{"", "work", "work", work, "work-secret"},
				{"default", "work", "default", f, testAPIKey},
				{"default", ".invalid", "default", f, testAPIKey},
				{"", "", "default", f, testAPIKey},
			} {
				f.env[config.EnvContext] = tt.env
				args := []string{"user", "list"}
				if tt.flag != "" {
					args = append(args, "--context", tt.flag)
				}
				count := tt.server.requestCount()
				requireSuccess(t, f.run(args...))
				if tt.server.requestCount() != count+1 || tt.server.lastRequest().Header.Get(n8n.HeaderAPIKey) != tt.secret {
					t.Fatal("resource used wrong context")
				}
				args = []string{"auth", "status", "--check", "--output", "json"}
				if tt.flag != "" {
					args = append(args, "--context", tt.flag)
				}
				got := f.run(args...)
				requireSuccess(t, got)
				var report statusReport
				if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
					t.Fatal(err)
				}
				if report.Context != tt.want || report.Current != (tt.want == "default") || report.URL != tt.server.server.URL || report.Validated == nil || !*report.Validated {
					t.Fatalf("status = %+v", report)
				}
				if tt.server.lastRequest().Header.Get(n8n.HeaderAPIKey) != tt.secret || strings.Contains(got.stdout+got.stderr, tt.secret) {
					t.Fatal("wrong or leaked credential")
				}
			}
			after, err := os.ReadFile(filepath.Join(f.dir, config.ConfigFileName))
			if err != nil || string(before) != string(after) {
				t.Fatal("per-process selection persisted")
			}
		})
	}
}

func TestEnvContextLogin(t *testing.T) {
	for _, tt := range []struct{ name, env, flag, want string }{
		{"environment", "work", "", "work"},
		{"flag overrides env", "work", "explicit", "explicit"},
		{"flag ignores invalid env", ".bad", "explicit", "explicit"},
		{"empty env uses default", "", "", "default"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.login("--context", "current")
			original := f.config().Contexts["current"]
			f.env[config.EnvContext] = tt.env
			f.prompt.secret = "new-secret"
			args := []string{"auth", "login", "--url", f.server.URL}
			if tt.flag != "" {
				args = append(args, "--context", tt.flag)
			}
			requireSuccess(t, f.run(args...))
			cfg := f.config()
			if cfg.CurrentContext != tt.want || len(cfg.Contexts) != 2 || cfg.Contexts["current"] != original {
				t.Fatal("login selected or changed wrong context")
			}
			cred, err := f.keyring.Get(cfg.Contexts[tt.want].CredentialRef)
			if err != nil || cred.Value.Reveal() != "new-secret" {
				t.Fatal("login stored wrong credential")
			}
		})
	}
	t.Run("replacement consent and saved URL", func(t *testing.T) {
		f, work := envContexts(t, config.StorageKeyring)
		saved := f.config()
		f.prompt.secret = "replacement"
		f.interactive = false
		f.stdin = "replacement"
		requests, prompts := work.requestCount(), len(f.prompt.prompts)
		got := f.run("auth", "login", "--stdin")
		if got.code != ExitError || !strings.Contains(got.stderr, "--yes") {
			t.Fatalf("replacement guard = %+v", got)
		}
		if work.requestCount() != requests || len(f.prompt.prompts) != prompts || !reflect.DeepEqual(saved, f.config()) {
			t.Fatal("refused login changed state or made request")
		}
		requireSuccess(t, f.run("auth", "login", "--stdin", "--yes"))
		if f.config().CurrentContext != "work" || f.config().Contexts["default"] != saved.Contexts["default"] {
			t.Fatal("replacement touched wrong context")
		}
		if work.requestCount() != requests+1 || work.lastRequest().Header.Get(n8n.HeaderAPIKey) != "replacement" {
			t.Fatal("env login did not inherit selected context URL")
		}
		assertContextAuth(t, f, f, "default", n8n.AuthAPIKey, testAPIKey)
	})
}

func TestEnvContextInvalidSelection(t *testing.T) {
	for _, name := range []string{"gone", ".invalid", " ", strings.Repeat("x", 65)} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.login()
			before := f.config()
			count, prompts := f.requestCount(), len(f.prompt.prompts)
			f.env[config.EnvContext] = name
			// Even full environment auth must not mask a bad context selection.
			f.env[config.EnvURL] = f.server.URL
			f.env[config.EnvAPIKey] = "environment-secret"
			commands := [][]string{{"user", "list"}, {"auth", "status", "--check"}, {"auth", "logout", "--purge", "--yes"}}
			if name != "gone" {
				commands = append(commands, []string{"auth", "login", "--skip-verify", "--yes"})
			}
			for _, args := range commands {
				got := f.run(args...)
				if got.code != ExitError || !strings.Contains(got.stderr, config.EnvContext) || strings.Contains(got.stderr, "environment-secret") {
					t.Fatalf("%v: %+v", args, got)
				}
			}
			if f.requestCount() != count || len(f.prompt.prompts) != prompts || !reflect.DeepEqual(before, f.config()) {
				t.Fatal("invalid selection performed work")
			}
		})
	}
}

func TestEnvContextLogout(t *testing.T) {
	for _, tt := range []struct {
		name, flag, target string
		purge, yes         bool
	}{
		{"env", "", "work", false, true},
		{"purge env", "", "work", true, true},
		{"flag overrides env", "default", "default", false, true},
		{"purge flag", "default", "default", true, true},
		{"refused", "", "work", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, work := envContexts(t, config.StorageKeyring)
			before := f.config()
			ref := before.Contexts[tt.target].CredentialRef
			f.interactive = false
			args := []string{"auth", "logout"}
			if tt.flag != "" {
				args = append(args, "--context", tt.flag)
			}
			if tt.purge {
				args = append(args, "--purge")
			}
			if tt.yes {
				args = append(args, "--yes")
			}
			got := f.run(args...)
			if !tt.yes {
				if got.code != ExitError || !strings.Contains(got.stderr, "--yes") || !reflect.DeepEqual(before, f.config()) {
					t.Fatalf("logout guard = %+v", got)
				}
				assertContextAuth(t, f, work, "work", n8n.AuthAPIKey, "work-secret")
				return
			}
			requireSuccess(t, got)
			if _, err := f.keyring.Get(ref); !errors.Is(err, config.ErrCredentialNotFound) {
				t.Fatal("selected credential remains")
			}
			wantCurrent := "default"
			if tt.purge && tt.target == "default" {
				wantCurrent = ""
			}
			if f.config().CurrentContext != wantCurrent {
				t.Fatal("logout changed wrong selection")
			}
			if tt.target == "work" {
				assertContextAuth(t, f, f, "default", n8n.AuthAPIKey, testAPIKey)
			} else {
				assertContextAuth(t, f, work, "work", n8n.AuthAPIKey, "work-secret")
			}
		})
	}
}

func TestEnvContextRepairHintsRespectExplicitDefault(t *testing.T) {
	f, _ := envContexts(t, config.StorageKeyring)
	f.mu.Lock()
	f.route = func(*http.Request, int) (int, string) { return http.StatusUnauthorized, `{"message":"unauthorized"}` }
	f.mu.Unlock()
	got := f.run("user", "list", "--context", "default")
	if got.code != ExitError || !strings.Contains(got.stderr, "auth login --context default") {
		t.Fatalf("repair hint targets env context: %+v", got)
	}
	requireSuccess(t, f.run("auth", "logout", "--context", "default", "--yes"))
	got = f.run("auth", "status", "--context", "default")
	requireSuccess(t, got)
	if !strings.Contains(got.stdout, "auth login --context default") {
		t.Fatalf("status repair hint = %+v", got)
	}
}

func requireSuccess(t *testing.T, got result) {
	t.Helper()
	if got.code != ExitSuccess {
		t.Fatalf("command failed: %+v", got)
	}
}

func assertContextAuth(t *testing.T, owner, server *fixture, name string, typ n8n.AuthType, secret string) {
	t.Helper()
	before := server.requestCount()
	got := owner.run("auth", "status", "--context", name, "--check")
	requireSuccess(t, got)
	if server.requestCount() != before+1 {
		t.Fatal("request sent to wrong instance")
	}
	req := server.lastRequest()
	header, want := n8n.HeaderAPIKey, secret
	switch typ {
	case n8n.AuthBearer:
		header, want = "Authorization", "Bearer "+secret
	case n8n.AuthCookie:
		header, want = "Cookie", n8n.CookieName+"="+secret
	}
	if req.Header.Get(header) != want {
		t.Fatal("wrong context credential sent")
	}
	for _, other := range []string{n8n.HeaderAPIKey, "Authorization", "Cookie"} {
		if other != header && req.Header.Get(other) != "" {
			t.Fatal("extra authentication header")
		}
	}
	if strings.Contains(got.stdout+got.stderr, secret) {
		t.Fatal("credential leaked in output")
	}
	data, err := os.ReadFile(filepath.Join(owner.dir, config.ConfigFileName))
	if err != nil || strings.Contains(string(data), secret) {
		t.Fatal("credential leaked in metadata")
	}
}
