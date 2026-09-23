package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	testAPIKey    = "n8n-api-key-that-must-never-be-printed"
	testBearerJWT = "n8n-bearer-token-that-must-never-be-printed"
)

// fakePrompter stands in for the no-echo terminal prompt, so authentication
// tests run without a TTY.
type fakePrompter struct {
	secret  n8n.Secret
	err     error
	prompts []string
}

func (p *fakePrompter) PromptSecret(prompt string) (n8n.Secret, error) {
	p.prompts = append(p.prompts, prompt)
	if p.err != nil {
		return "", p.err
	}
	return p.secret, nil
}

// unavailableKeyring is a machine with no working OS credential store.
type unavailableKeyring struct{}

func (unavailableKeyring) Available() error { return config.ErrKeyringUnavailable }

func (unavailableKeyring) Kind() config.Storage { return config.StorageKeyring }

func (unavailableKeyring) Get(string) (config.Credential, error) {
	return config.Credential{}, config.ErrKeyringUnavailable
}

func (unavailableKeyring) Set(string, config.Credential) error { return config.ErrKeyringUnavailable }

func (unavailableKeyring) Delete(string) error { return config.ErrKeyringUnavailable }

// fixture is one machine: a config directory, a credential store, an
// environment and an n8n instance. Each run() is a separate CLI process
// sharing that state, which is how persistence across runs is tested.
type fixture struct {
	t           *testing.T
	dir         string
	env         map[string]string
	keyring     config.CredentialStore
	prompt      *fakePrompter
	interactive bool
	stdin       string

	mu     sync.Mutex
	status int
	body   string
	// bodyFunc, when set, produces the response body from the zero-based
	// index of the request, which is how multi-page responses are staged.
	bodyFunc func(int) string
	requests []*http.Request
	// bodies holds the request body of each recorded request, read inside the
	// handler because a cloned request's body is no longer readable afterwards.
	bodies []string
	// route, when set, answers each request from its method and path with a
	// status and body, for commands that mix several endpoints in one run. It
	// takes precedence over body and bodyFunc, and its body is written for
	// every status.
	route func(r *http.Request, page int) (int, string)
	// pages counts the requests bodyFunc or route has answered.
	pages  int
	server *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		t:           t,
		dir:         filepath.Join(t.TempDir(), "n8n-cli"),
		env:         map[string]string{},
		keyring:     config.NewMemoryStore(config.StorageKeyring),
		prompt:      &fakePrompter{secret: testAPIKey},
		interactive: true,
		status:      http.StatusOK,
		body:        `{"scopes":["workflow:read"]}`,
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		f.mu.Lock()
		f.requests = append(f.requests, r.Clone(r.Context()))
		f.bodies = append(f.bodies, string(sent))
		status, body := f.status, f.body
		switch {
		case f.route != nil:
			status, body = f.route(r, f.pages)
			f.pages++
		case f.bodyFunc != nil:
			body = f.bodyFunc(f.pages)
			f.pages++
		}
		if status != http.StatusOK && f.route == nil {
			body = `{"message":"unauthorized"}`
		}
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fixture) run(args ...string) result {
	f.t.Helper()
	return f.runContext(context.Background(), args...)
}

// runContext is run with a caller-owned context, so a test can cancel a
// command that would otherwise poll forever.
func (f *fixture) runContext(ctx context.Context, args ...string) result {
	f.t.Helper()
	var out, errOut strings.Builder
	interactive := f.interactive
	code := Run(ctx, args, Options{
		Streams:     Streams{In: strings.NewReader(f.stdin), Out: &out, Err: &errOut},
		Version:     testVersion,
		ConfigDir:   f.dir,
		Env:         func(name string) string { return f.env[name] },
		Keyring:     f.keyring,
		Prompt:      f.prompt,
		Interactive: &interactive,
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

// login runs a successful default login and fails the test if it does not.
func (f *fixture) login(args ...string) result {
	f.t.Helper()
	got := f.run(append([]string{"auth", "login", "--url", f.server.URL}, args...)...)
	if got.code != ExitSuccess {
		f.t.Fatalf("login exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	return got
}

func (f *fixture) config() *config.Config {
	f.t.Helper()
	store, err := config.NewStore(f.dir)
	if err != nil {
		f.t.Fatalf("NewStore: %v", err)
	}
	cfg, err := store.Load()
	if err != nil {
		f.t.Fatalf("Load: %v", err)
	}
	return cfg
}

func (f *fixture) lastRequest() *http.Request {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		f.t.Fatal("no request reached the instance")
	}
	return f.requests[len(f.requests)-1]
}

// lastBody returns the body of the most recent request, empty when it had none.
func (f *fixture) lastBody() string {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		f.t.Fatal("no request reached the instance")
	}
	return f.bodies[len(f.bodies)-1]
}

func (f *fixture) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func TestAuthLoginValidatesStoresAndReloads(t *testing.T) {
	f := newFixture(t)

	got := f.login("--context", "production")
	if !strings.Contains(got.stderr, `context "production"`) {
		t.Errorf("stderr = %q, want it to name the context", got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want login to keep stdout clean", got.stdout)
	}

	// The credential was validated before anything was written.
	if req := f.lastRequest(); req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
		t.Errorf("validation request sent %q, want the credential being logged in", req.Header.Get(n8n.HeaderAPIKey))
	}
	if want := n8n.BasePath + n8n.DiscoverPath; f.lastRequest().URL.Path != want {
		t.Errorf("validation path = %q, want %q", f.lastRequest().URL.Path, want)
	}

	cfg := f.config()
	if cfg.CurrentContext != "production" {
		t.Errorf("CurrentContext = %q, want %q", cfg.CurrentContext, "production")
	}
	saved := cfg.Contexts["production"]
	if saved.URL != f.server.URL || saved.AuthType != n8n.AuthAPIKey || saved.Storage != config.StorageKeyring {
		t.Errorf("saved context = %+v, want the instance, api-key and keyring storage", saved)
	}
	stored, err := f.keyring.Get(saved.CredentialRef)
	if err != nil {
		t.Fatalf("credential store: %v", err)
	}
	if stored.Value.Reveal() != testAPIKey {
		t.Error("the stored credential is not the one that was entered")
	}

	// A later process finds and uses the same credential.
	status := f.run("auth", "status", "--check")
	if status.code != ExitSuccess {
		t.Fatalf("status exit code = %d, want %d (stderr: %s)", status.code, ExitSuccess, status.stderr)
	}
	if req := f.lastRequest(); req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
		t.Error("a later run did not send the saved credential")
	}
	if !strings.Contains(status.stdout, "accepted by the instance") {
		t.Errorf("stdout = %q, want the validation result", status.stdout)
	}
}

func TestAuthLoginPromptsForEachAuthType(t *testing.T) {
	tests := []struct {
		name       string
		authType   string
		wantPrompt string
		assert     func(t *testing.T, r *http.Request)
	}{
		{
			name: "default is api key", wantPrompt: "API key",
			assert: func(t *testing.T, r *http.Request) {
				if r.Header.Get(n8n.HeaderAPIKey) == "" {
					t.Error("no API key header")
				}
			},
		},
		{
			name: "bearer", authType: "bearer", wantPrompt: "bearer token",
			assert: func(t *testing.T, r *http.Request) {
				if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					t.Error("no bearer authorization header")
				}
			},
		},
		{
			name: "cookie", authType: "cookie", wantPrompt: "cookie",
			assert: func(t *testing.T, r *http.Request) {
				if _, err := r.Cookie(n8n.CookieName); err != nil {
					t.Errorf("no %s cookie: %v", n8n.CookieName, err)
				}
			},
		},
		{
			name: "case insensitive", authType: "API-KEY", wantPrompt: "API key",
			assert: func(t *testing.T, r *http.Request) {
				if r.Header.Get(n8n.HeaderAPIKey) == "" {
					t.Error("no API key header")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			args := []string{}
			if tt.authType != "" {
				args = append(args, "--type", tt.authType)
			}
			f.login(args...)

			if len(f.prompt.prompts) != 1 || !strings.Contains(f.prompt.prompts[0], tt.wantPrompt) {
				t.Errorf("prompts = %q, want one mentioning %q", f.prompt.prompts, tt.wantPrompt)
			}
			tt.assert(t, f.lastRequest())
		})
	}
}

func TestAuthLoginReadsCredentialFromStdin(t *testing.T) {
	f := newFixture(t)
	f.interactive = false
	f.stdin = testAPIKey + "\n"
	f.prompt.err = errors.New("the prompt must not be used with --stdin")

	f.login("--stdin")

	if req := f.lastRequest(); req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
		t.Errorf("sent %q, want the credential from stdin", req.Header.Get(n8n.HeaderAPIKey))
	}
}

func TestAuthLoginFailures(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *fixture)
		args    []string
		wantErr string
	}{
		{
			name:    "instance rejects the credential",
			setup:   func(f *fixture) { f.status = http.StatusUnauthorized },
			wantErr: "nothing was saved",
		},
		{
			name:    "credential lacks the scope",
			setup:   func(f *fixture) { f.status = http.StatusForbidden },
			wantErr: "403",
		},
		{
			name:    "instance unreachable",
			setup:   func(f *fixture) { f.server.Close() },
			wantErr: "could not reach",
		},
		{
			name:    "unknown auth type",
			args:    []string{"--type", "basic"},
			wantErr: `unknown auth type "basic"`,
		},
		{
			name:    "unknown storage",
			args:    []string{"--storage", "vault"},
			wantErr: `unknown storage "vault"`,
		},
		{
			name:    "invalid context name",
			args:    []string{"--context", "my context"},
			wantErr: "may only contain",
		},
		{
			name:    "no terminal and no stdin",
			setup:   func(f *fixture) { f.interactive = false },
			wantErr: "--stdin",
		},
		{
			name:    "empty credential on stdin",
			setup:   func(f *fixture) { f.interactive = false; f.stdin = "\n" },
			args:    []string{"--stdin"},
			wantErr: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			url := f.server.URL
			if tt.setup != nil {
				tt.setup(f)
			}

			got := f.run(append([]string{"auth", "login", "--url", url}, tt.args...)...)
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitError, got.stderr)
			}
			if !strings.Contains(got.stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tt.wantErr)
			}

			// A failed login writes nothing at all.
			if cfg := f.config(); len(cfg.Contexts) != 0 {
				t.Errorf("contexts = %v, want none", cfg.Contexts)
			}
			if store, ok := f.keyring.(*config.MemoryStore); ok && store.Len() != 0 {
				t.Error("a failed login left a credential behind")
			}
		})
	}
}

func TestAuthLoginRequiresAURL(t *testing.T) {
	f := newFixture(t)
	f.interactive = false

	got := f.run("auth", "login", "--stdin")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, config.EnvURL) {
		t.Errorf("stderr = %q, want it to mention %s", got.stderr, config.EnvURL)
	}
}

func TestAuthLoginTakesURLFromEnvironmentAndPrompt(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		f := newFixture(t)
		f.env[config.EnvURL] = f.server.URL

		got := f.run("auth", "login")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if saved := f.config().Contexts[DefaultContextName]; saved.URL != f.server.URL {
			t.Errorf("saved URL = %q, want the one from %s", saved.URL, config.EnvURL)
		}
	})

	t.Run("prompt", func(t *testing.T) {
		f := newFixture(t)
		f.stdin = f.server.URL + "\n"

		got := f.run("auth", "login")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if !strings.Contains(got.stderr, "instance URL") {
			t.Errorf("stderr = %q, want the URL prompt", got.stderr)
		}
	})
}

func TestAuthLoginReplacesExistingCredentialOnlyWithConsent(t *testing.T) {
	t.Run("declined", func(t *testing.T) {
		f := newFixture(t)
		f.login()
		f.prompt.secret = "replacement-key"
		f.stdin = "n\n"

		got := f.run("auth", "login", "--url", f.server.URL)
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
			t.Fatalf("exit code = %d, stderr = %q; want an aborted replacement", got.code, got.stderr)
		}
		stored, err := f.keyring.Get(f.config().Contexts[DefaultContextName].CredentialRef)
		if err != nil {
			t.Fatalf("credential store: %v", err)
		}
		if stored.Value.Reveal() != testAPIKey {
			t.Error("a declined replacement changed the stored credential")
		}
		if len(f.prompt.prompts) != 1 {
			t.Errorf("prompts = %q, want the credential prompt to be skipped once declined", f.prompt.prompts)
		}
	})

	t.Run("confirmed", func(t *testing.T) {
		f := newFixture(t)
		f.login()
		f.prompt.secret = "replacement-key"
		f.stdin = "y\n"

		f.login()
		stored, err := f.keyring.Get(f.config().Contexts[DefaultContextName].CredentialRef)
		if err != nil {
			t.Fatalf("credential store: %v", err)
		}
		if stored.Value.Reveal() != "replacement-key" {
			t.Error("a confirmed replacement did not store the new credential")
		}
	})

	t.Run("non-interactive without --yes", func(t *testing.T) {
		f := newFixture(t)
		f.login()
		f.interactive = false
		f.stdin = "replacement-key\n"

		got := f.run("auth", "login", "--url", f.server.URL, "--stdin")
		if got.code != ExitError || !strings.Contains(got.stderr, "--yes") {
			t.Fatalf("exit code = %d, stderr = %q; want a refusal pointing at --yes", got.code, got.stderr)
		}
	})

	t.Run("non-interactive with --yes", func(t *testing.T) {
		f := newFixture(t)
		f.login()
		f.interactive = false
		f.stdin = "replacement-key\n"

		got := f.run("auth", "login", "--url", f.server.URL, "--stdin", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		stored, _ := f.keyring.Get(f.config().Contexts[DefaultContextName].CredentialRef)
		if stored.Value.Reveal() != "replacement-key" {
			t.Error("--yes did not replace the credential")
		}
	})
}

func TestAuthLoginMigratesBetweenStorageBackends(t *testing.T) {
	f := newFixture(t)
	f.login()

	// Move the same context to the plaintext fallback.
	f.login("--storage", "file", "--yes")

	if saved := f.config().Contexts[DefaultContextName]; saved.Storage != config.StorageFile {
		t.Fatalf("storage = %q, want %q", saved.Storage, config.StorageFile)
	}
	if store, ok := f.keyring.(*config.MemoryStore); ok && store.Len() != 0 {
		t.Error("the credential was left behind in the keyring after moving to file storage")
	}
	authFile := filepath.Join(f.dir, config.SecretFileName)
	data, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), testAPIKey) {
		t.Error("the credential was not written to the fallback file")
	}

	// And back again.
	f.login("--storage", "keyring", "--yes")
	if saved := f.config().Contexts[DefaultContextName]; saved.Storage != config.StorageKeyring {
		t.Fatalf("storage = %q, want %q", saved.Storage, config.StorageKeyring)
	}
	data, err = os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), testAPIKey) {
		t.Error("the credential was left behind in the fallback file after moving to the keyring")
	}
}

func TestAuthLoginWarnsAboutPlaintextStorage(t *testing.T) {
	f := newFixture(t)
	got := f.login("--storage", "file")

	if !strings.Contains(got.stderr, "plaintext") {
		t.Errorf("stderr = %q, want a plaintext storage warning", got.stderr)
	}
	if !strings.Contains(got.stderr, config.SecretFileName) {
		t.Errorf("stderr = %q, want it to name the file", got.stderr)
	}
}

func TestAuthLoginNeverDowngradesStorageSilently(t *testing.T) {
	t.Run("non-interactive", func(t *testing.T) {
		f := newFixture(t)
		f.keyring = unavailableKeyring{}
		f.interactive = false
		f.stdin = testAPIKey + "\n"

		got := f.run("auth", "login", "--url", f.server.URL, "--stdin")
		if got.code != ExitError {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitError, got.stderr)
		}
		if !strings.Contains(got.stderr, "--storage=file") {
			t.Errorf("stderr = %q, want it to explain how to accept plaintext storage", got.stderr)
		}
		if _, err := os.Stat(filepath.Join(f.dir, config.SecretFileName)); err == nil {
			t.Error("a credential was written to the fallback file without consent")
		}
	})

	t.Run("interactive consent", func(t *testing.T) {
		f := newFixture(t)
		f.keyring = unavailableKeyring{}
		f.stdin = "y\n"

		f.login()
		if saved := f.config().Contexts[DefaultContextName]; saved.Storage != config.StorageFile {
			t.Errorf("storage = %q, want %q after consent", saved.Storage, config.StorageFile)
		}
	})

	t.Run("interactive refusal", func(t *testing.T) {
		f := newFixture(t)
		f.keyring = unavailableKeyring{}
		f.stdin = "n\n"

		got := f.run("auth", "login", "--url", f.server.URL)
		if got.code != ExitError || !strings.Contains(got.stderr, "--storage=file") {
			t.Fatalf("exit code = %d, stderr = %q; want a refusal", got.code, got.stderr)
		}
	})

	t.Run("--yes is not consent to plaintext", func(t *testing.T) {
		f := newFixture(t)
		f.keyring = unavailableKeyring{}
		f.interactive = false
		f.stdin = testAPIKey + "\n"

		got := f.run("auth", "login", "--url", f.server.URL, "--stdin", "--yes")
		if got.code != ExitError {
			t.Fatalf("exit code = %d, want %d: --yes must not accept weaker storage", got.code, ExitError)
		}
	})
}

func TestAuthCommandsNeverLeakTheCredential(t *testing.T) {
	f := newFixture(t)
	f.prompt.secret = testAPIKey

	outputs := []string{}
	record := func(got result) { outputs = append(outputs, got.stdout, got.stderr) }

	record(f.login("--context", "production"))
	record(f.run("auth", "status", "--check"))
	record(f.run("auth", "status", "--output", "json"))
	record(f.run("config", "context", "list"))
	record(f.run("config", "context", "list", "--output", "json"))
	record(f.run("auth", "logout", "--yes"))

	for _, out := range outputs {
		if strings.Contains(out, testAPIKey) {
			t.Errorf("command output contains the credential:\n%s", out)
		}
	}

	data, err := os.ReadFile(filepath.Join(f.dir, config.ConfigFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), testAPIKey) {
		t.Errorf("config.json contains the credential:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(f.dir, config.SecretFileName)); err == nil {
		t.Error("auth.json was created even though the credential went to the keyring")
	}
}

func TestAuthStatus(t *testing.T) {
	f := newFixture(t)
	f.login("--context", "production")

	t.Run("text", func(t *testing.T) {
		got := f.run("auth", "status")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		for _, want := range []string{"production (current)", f.server.URL, "api-key", "keyring", "present"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		got := f.run("auth", "status", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		var report struct {
			Context    string `json:"context"`
			Current    bool   `json:"current"`
			URL        string `json:"url"`
			AuthType   string `json:"authType"`
			Storage    string `json:"storage"`
			Source     string `json:"source"`
			Credential string `json:"credential"`
			Validated  *bool  `json:"validated"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
			t.Fatalf("decode %q: %v", got.stdout, err)
		}
		if report.Context != "production" || !report.Current || report.URL != f.server.URL {
			t.Errorf("report = %+v, want the current production context", report)
		}
		if report.AuthType != "api-key" || report.Storage != "keyring" || report.Credential != "present" {
			t.Errorf("report = %+v, want an api-key credential in the keyring", report)
		}
		if report.Validated != nil {
			t.Error("validated is reported without --check")
		}
	})

	t.Run("unknown output format", func(t *testing.T) {
		got := f.run("auth", "status", "--output", "yaml")
		if got.code != ExitError || !strings.Contains(got.stderr, "unknown output format") {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
	})

	t.Run("unknown context", func(t *testing.T) {
		got := f.run("auth", "status", "--context", "gone")
		if got.code != ExitError || !strings.Contains(got.stderr, `context "gone" not found`) {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
	})
}

func TestAuthStatusWithoutAnyContext(t *testing.T) {
	f := newFixture(t)
	got := f.run("auth", "status")
	if got.code != ExitError || !strings.Contains(got.stderr, "auth login") {
		t.Fatalf("exit code = %d, stderr = %q; want guidance to log in", got.code, got.stderr)
	}
}

func TestAuthStatusReportsAMissingCredential(t *testing.T) {
	f := newFixture(t)
	f.login()
	if err := f.keyring.Delete(f.config().Contexts[DefaultContextName].CredentialRef); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got := f.run("auth", "status")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stdout, "missing") || !strings.Contains(got.stdout, "auth login") {
		t.Errorf("stdout = %q, want it to report the missing credential", got.stdout)
	}

	checked := f.run("auth", "status", "--check")
	if checked.code != ExitError {
		t.Errorf("--check exit code = %d, want %d when no credential is stored", checked.code, ExitError)
	}
}

func TestAuthStatusCheckFailsOnARejectedCredential(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.status = http.StatusUnauthorized

	got := f.run("auth", "status", "--check")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stdout, "rejected by the instance") {
		t.Errorf("stdout = %q, want the rejection in the report", got.stdout)
	}
	if !strings.Contains(got.stderr, "not usable") {
		t.Errorf("stderr = %q, want a failure message", got.stderr)
	}
}

func TestAuthStatusReportsEnvironmentCredentials(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.env[config.EnvBearerToken] = testBearerJWT

	got := f.run("auth", "status", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
		t.Fatalf("decode %q: %v", got.stdout, err)
	}
	if report["source"] != string(config.SourceEnv) || report["authType"] != string(n8n.AuthBearer) {
		t.Errorf("report = %v, want the environment credential to win", report)
	}
	if strings.Contains(got.stdout, testBearerJWT) {
		t.Error("the environment credential was printed")
	}

	// It is used, and it is not written anywhere.
	if checked := f.run("auth", "status", "--check"); checked.code != ExitSuccess {
		t.Fatalf("--check exit code = %d, want %d (stderr: %s)", checked.code, ExitSuccess, checked.stderr)
	}
	if auth := f.lastRequest().Header.Get("Authorization"); auth != "Bearer "+testBearerJWT {
		t.Errorf("Authorization = %q, want the environment bearer token", auth)
	}
	stored, err := f.keyring.Get(f.config().Contexts[DefaultContextName].CredentialRef)
	if err != nil || stored.Value.Reveal() != testAPIKey {
		t.Error("the environment credential replaced the stored one")
	}
}

func TestAuthStatusRejectsAmbiguousEnvironmentCredentials(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.env[config.EnvAPIKey] = "key"
	f.env[config.EnvBearerToken] = "token"

	got := f.run("auth", "status")
	if got.code != ExitError || !strings.Contains(got.stderr, "unset all but one") {
		t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
	}
}

func TestAuthLogout(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		ref := f.config().Contexts["production"].CredentialRef
		f.stdin = "y\n"

		got := f.run("auth", "logout")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if _, err := f.keyring.Get(ref); !errors.Is(err, config.ErrCredentialNotFound) {
			t.Errorf("credential store = %v, want the credential gone", err)
		}
		if _, ok := f.config().Contexts["production"]; !ok {
			t.Error("logout removed the context; it must only remove the credential")
		}
		if !strings.Contains(got.stderr, "Deleted the keyring credential") {
			t.Errorf("stderr = %q, want it to report the deletion", got.stderr)
		}

		again := f.run("auth", "logout", "--yes")
		if again.code != ExitSuccess {
			t.Errorf("second logout exit code = %d, want it to be idempotent", again.code)
		}
		if !strings.Contains(again.stderr, "No credential was stored") {
			t.Errorf("second logout stderr = %q, want it to say nothing was there", again.stderr)
		}
	})

	t.Run("purge", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		ref := f.config().Contexts["production"].CredentialRef

		got := f.run("auth", "logout", "--context", "production", "--purge", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if _, err := f.keyring.Get(ref); !errors.Is(err, config.ErrCredentialNotFound) {
			t.Errorf("credential store = %v, want the credential gone", err)
		}
		cfg := f.config()
		if _, ok := cfg.Contexts["production"]; ok {
			t.Error("--purge kept the context; it must remove it from config.json")
		}
		if cfg.CurrentContext == "production" {
			t.Error("--purge left the removed context selected")
		}
		if !strings.Contains(got.stderr, "removed the context") {
			t.Errorf("stderr = %q, want it to report the context removal", got.stderr)
		}
	})

	t.Run("purge without a stored credential", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		f.run("auth", "logout", "--context", "production", "--yes")

		got := f.run("auth", "logout", "--context", "production", "--purge", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if _, ok := f.config().Contexts["production"]; ok {
			t.Error("--purge kept the context when no credential was stored")
		}
		if !strings.Contains(got.stderr, "No credential was stored") {
			t.Errorf("stderr = %q, want it to say nothing was stored", got.stderr)
		}
	})

	t.Run("declined purge keeps both", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		f.stdin = "n\n"

		got := f.run("auth", "logout", "--context", "production", "--purge")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
		if _, err := f.keyring.Get(f.config().Contexts["production"].CredentialRef); err != nil {
			t.Errorf("a declined purge deleted the credential: %v", err)
		}
		if _, ok := f.config().Contexts["production"]; !ok {
			t.Error("a declined purge removed the context")
		}
	})

	t.Run("declined", func(t *testing.T) {
		f := newFixture(t)
		f.login()
		f.stdin = "n\n"

		got := f.run("auth", "logout")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
		if _, err := f.keyring.Get(f.config().Contexts[DefaultContextName].CredentialRef); err != nil {
			t.Error("a declined logout deleted the credential")
		}
	})

	t.Run("non-interactive without --yes", func(t *testing.T) {
		f := newFixture(t)
		f.login()
		f.interactive = false

		got := f.run("auth", "logout")
		if got.code != ExitError || !strings.Contains(got.stderr, "--yes") {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
	})

	t.Run("unknown context", func(t *testing.T) {
		f := newFixture(t)
		f.login()

		got := f.run("auth", "logout", "--context", "gone", "--yes")
		if got.code != ExitError || !strings.Contains(got.stderr, `context "gone" not found`) {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
	})
}

func TestAuthLoginDoesNotReachTheInstanceWhenFlagsAreWrong(t *testing.T) {
	f := newFixture(t)
	got := f.run("auth", "login", "--url", f.server.URL, "--type", "basic")

	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if f.requestCount() != 0 {
		t.Error("an invalid flag combination still contacted the instance")
	}
	if len(f.prompt.prompts) != 0 {
		t.Error("an invalid flag combination still asked for a credential")
	}
}
