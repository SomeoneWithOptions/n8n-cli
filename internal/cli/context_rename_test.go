package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

func seedLegacyContext(t *testing.T, f *fixture, kind config.Storage, typ n8n.AuthType) config.Context {
	t.Helper()
	store, err := config.NewStore(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	saved := config.Context{URL: f.server.URL, AuthType: typ, Storage: kind, CredentialRef: "default"}
	backend := f.keyring
	if kind == config.StorageFile {
		backend = config.NewFileStore(store)
	}
	if err := backend.Set("default", config.Credential{Type: typ, Value: testAPIKey}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(cfg *config.Config) error {
		if err := cfg.Put("default", saved); err != nil {
			return err
		}
		return cfg.Use("default")
	}); err != nil {
		t.Fatal(err)
	}
	return saved
}

func requireSuccess(t *testing.T, got result) {
	t.Helper()
	if got.code != ExitSuccess {
		t.Fatalf("command failed: %+v", got)
	}
}

type forbiddenKeyring struct{ t *testing.T }

func (f forbiddenKeyring) Kind() config.Storage {
	f.t.Fatal("rename touched keyring")
	return config.StorageKeyring
}
func (f forbiddenKeyring) Available() error { f.t.Fatal("rename probed keyring"); return nil }
func (f forbiddenKeyring) Get(string) (config.Credential, error) {
	f.t.Fatal("rename read keyring")
	return config.Credential{}, nil
}
func (f forbiddenKeyring) Set(string, config.Credential) error {
	f.t.Fatal("rename wrote keyring")
	return nil
}
func (f forbiddenKeyring) Delete(string) error { f.t.Fatal("rename deleted credential"); return nil }

func TestContextRename(t *testing.T) {
	f := newFixture(t)
	saved := seedLegacyContext(t, f, config.StorageKeyring, n8n.AuthAPIKey)
	f.keyring = forbiddenKeyring{t}
	f.interactive = false
	got := f.run("config", "context", "rename", "default", "work")
	requireSuccess(t, got)
	if got.stdout != "" || got.stderr != "Renamed context \"default\" to \"work\".\n" {
		t.Fatalf("output = %+v", got)
	}
	cfg := f.config()
	if cfg.CurrentContext != "work" || cfg.Contexts["work"] != saved || len(cfg.Contexts) != 1 {
		t.Fatalf("config = %+v", cfg)
	}
	if f.requestCount() != 0 {
		t.Fatal("rename made HTTP request")
	}
	same := f.run("config", "context", "rename", "work", "work")
	requireSuccess(t, same)
	if same.stdout != "" || !strings.Contains(same.stderr, "nothing changed") {
		t.Fatalf("no-op output = %+v", same)
	}
	for _, args := range [][]string{
		{}, {"work"}, {"work", "new", "extra"}, {"gone", "new"}, {"gone", "gone"}, {"work", ".bad"},
		{"work", strings.Repeat("x", 65)}, {"work", "bad name"}, {"", "new"}, {"work", ""},
	} {
		before, err := os.ReadFile(filepath.Join(f.dir, config.ConfigFileName))
		if err != nil {
			t.Fatal(err)
		}
		got := f.run(append([]string{"config", "context", "rename"}, args...)...)
		if got.code != ExitError || got.stdout != "" {
			t.Fatalf("invalid rename %v: %+v", args, got)
		}
		after, err := os.ReadFile(filepath.Join(f.dir, config.ConfigFileName))
		if err != nil || string(after) != string(before) {
			t.Fatal("invalid rename changed metadata")
		}
	}
	list := f.run("config", "context", "list", "--output", "json")
	requireSuccess(t, list)
	var reports []contextReport
	if err := json.Unmarshal([]byte(list.stdout), &reports); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Name != "work" || !reports[0].Current {
		t.Fatalf("list = %+v", reports)
	}
	text := f.run("config", "context", "list")
	requireSuccess(t, text)
	if !strings.Contains(text.stdout, "work") || strings.Contains(text.stdout, "default") {
		t.Fatalf("text list = %s", text.stdout)
	}
	completion := f.run("__complete", "config", "context", "rename", "")
	if !strings.Contains(completion.stdout, "work") || strings.Contains(completion.stdout, "default") {
		t.Fatalf("completion = %+v", completion)
	}
	for _, args := range [][]string{{"work", ""}, {"work", "new", ""}} {
		got := f.run(append([]string{"__complete", "config", "context", "rename"}, args...)...)
		if strings.Contains(got.stdout, "work") || !strings.Contains(got.stdout, ":4") {
			t.Fatalf("destination completion = %+v", got)
		}
	}
	if newContextRenameCommand(Options{}).Annotations["cliOnly"] != "true" {
		t.Fatal("missing CLI-only annotation")
	}
	_, directive := completeContextNames(Options{ConfigDir: f.dir}, []string{"work"})
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatal("filesystem completion enabled")
	}
	missing := f.run("auth", "status", "--context", "default")
	if missing.code != ExitError || !strings.Contains(missing.stderr, "not found") {
		t.Fatalf("old name resolved: %+v", missing)
	}
}

func TestRenameLoggedOutContext(t *testing.T) {
	for _, kind := range []config.Storage{config.StorageKeyring, config.StorageFile} {
		t.Run(string(kind), func(t *testing.T) {
			f := newFixture(t)
			saved := seedLegacyContext(t, f, kind, n8n.AuthAPIKey)
			requireSuccess(t, f.run("auth", "logout", "--yes"))
			f.keyring = unavailableKeyring{}
			requireSuccess(t, f.run("config", "context", "rename", "default", "work"))
			if f.config().Contexts["work"] != saved {
				t.Fatal("rename changed logged-out metadata")
			}
			f.keyring = config.NewMemoryStore(config.StorageKeyring)
			f.login("--storage", string(kind))
			f.prompt.secret = "work-restored-secret"
			f.login("--context", "work", "--storage", string(kind), "--yes")
			cfg := f.config()
			if cfg.Contexts["work"].CredentialRef == cfg.Contexts["default"].CredentialRef {
				t.Fatal("restored contexts share credential")
			}
			assertContextAuth(t, f, f, "default", n8n.AuthAPIKey, testAPIKey)
			assertContextAuth(t, f, f, "work", n8n.AuthAPIKey, "work-restored-secret")
		})
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

func TestLegacyRenameCredentialIsolation(t *testing.T) {
	scenarios := []string{
		"replace default", "replace work", "logout default", "restore default", "purge default", "delete default",
		"logout work", "purge work", "delete work", "migrate default", "migrate work",
		"repeat rename", "conflict", "reuse original name", "implicit selection",
	}
	for _, pair := range [][2]config.Storage{
		{config.StorageKeyring, config.StorageKeyring}, {config.StorageFile, config.StorageFile},
		{config.StorageKeyring, config.StorageFile}, {config.StorageFile, config.StorageKeyring},
	} {
		for _, typ := range []n8n.AuthType{n8n.AuthAPIKey, n8n.AuthBearer, n8n.AuthCookie} {
			for _, scenario := range scenarios {
				t.Run(string(pair[0])+"/"+string(pair[1])+"/"+string(typ)+"/"+scenario, func(t *testing.T) {
					f, b := newFixture(t), newFixture(t)
					saved := seedLegacyContext(t, f, pair[0], typ)
					requireSuccess(t, f.run("config", "context", "rename", "default", "work"))
					assertContextAuth(t, f, f, "work", typ, testAPIKey)
					f.prompt.secret = "instance-b-secret"
					got := f.run("auth", "login", "--url", b.server.URL, "--type", string(typ), "--storage", string(pair[1]))
					requireSuccess(t, got) // no --context: creates a fresh default
					cfg := f.config()
					if cfg.Contexts["work"] != saved || cfg.Contexts["default"].URL != b.server.URL || cfg.CurrentContext != "default" {
						t.Fatal("new default inherited or changed work")
					}
					ref := cfg.Contexts["default"].CredentialRef
					if !strings.HasPrefix(ref, "cred:") || len(ref) != 37 || ref == saved.CredentialRef {
						t.Fatal("new credential identity is not fresh")
					}
					assertContextAuth(t, f, b, "default", typ, "instance-b-secret")
					assertContextAuth(t, f, f, "work", typ, testAPIKey)
					login := func(name, secret string, kind config.Storage) {
						t.Helper()
						f.prompt.secret = n8n.Secret(secret)
						requireSuccess(t, f.run("auth", "login", "--context", name, "--type", string(typ), "--storage", string(kind), "--yes"))
					}
					checkOther := func(name string) {
						t.Helper()
						if name == "default" {
							assertContextAuth(t, f, f, "work", typ, testAPIKey)
						} else {
							assertContextAuth(t, f, b, "default", typ, "instance-b-secret")
						}
					}
					switch scenario {
					case "replace default":
						login("default", "default-replacement", pair[1])
						checkOther("default")
						assertContextAuth(t, f, b, "default", typ, "default-replacement")
					case "replace work":
						login("work", "work-replacement", pair[0])
						checkOther("work")
						assertContextAuth(t, f, f, "work", typ, "work-replacement")
					case "logout default", "restore default", "logout work", "purge default", "purge work", "delete default", "delete work":
						name := "default"
						if strings.HasSuffix(scenario, "work") {
							name = "work"
						}
						args := []string{"auth", "logout", "--context", name, "--yes"}
						remove := strings.HasPrefix(scenario, "purge") || strings.HasPrefix(scenario, "delete")
						if strings.HasPrefix(scenario, "purge") {
							args = append(args, "--purge")
						}
						if strings.HasPrefix(scenario, "delete") {
							args = []string{"config", "context", "delete", name, "--yes"}
						}
						previous := f.config().Contexts[name]
						requireSuccess(t, f.run(args...))
						checkOther(name)
						store, err := config.NewStore(f.dir)
						if err != nil {
							t.Fatal(err)
						}
						backend := f.keyring
						if previous.Storage == config.StorageFile {
							backend = config.NewFileStore(store)
						}
						if _, err := backend.Get(previous.CredentialRef); !errors.Is(err, config.ErrCredentialNotFound) {
							t.Fatal("credential was not removed")
						}
						cfg := f.config()
						_, exists := cfg.Contexts[name]
						if exists == remove {
							t.Fatal("incorrect metadata retention")
						}
						wantCurrent := "default"
						if remove && name == "default" {
							wantCurrent = ""
						}
						if cfg.CurrentContext != wantCurrent {
							t.Fatal("wrong selection after removal")
						}
						if scenario == "restore default" {
							login("default", "restored-default", pair[1])
							checkOther("default")
							assertContextAuth(t, f, b, "default", typ, "restored-default")
						}
					case "migrate default", "migrate work":
						name, secret, kind, server := "default", "instance-b-secret", pair[1], b
						if scenario == "migrate work" {
							name, secret, kind, server = "work", testAPIKey, pair[0], f
						}
						other := config.StorageFile
						if kind == config.StorageFile {
							other = config.StorageKeyring
						}
						for _, storage := range []config.Storage{other, kind} {
							login(name, secret, storage)
							checkOther(name)
							assertContextAuth(t, f, server, name, typ, secret)
						}
					case "repeat rename":
						requireSuccess(t, f.run("config", "context", "rename", "work", "personal"))
						requireSuccess(t, f.run("config", "context", "rename", "personal", "work"))
						if f.config().Contexts["work"] != saved {
							t.Fatal("repeated rename changed identity")
						}
						checkOther("default")
						checkOther("work")
					case "conflict":
						before := f.config()
						got := f.run("config", "context", "rename", "work", "default")
						if got.code != ExitError || !strings.Contains(got.stderr, "already exists") {
							t.Fatalf("conflict = %+v", got)
						}
						if !reflect.DeepEqual(before, f.config()) {
							t.Fatal("conflict changed metadata")
						}
						checkOther("default")
						checkOther("work")
					case "reuse original name":
						requireSuccess(t, f.run("config", "context", "delete", "default", "--yes"))
						requireSuccess(t, f.run("config", "context", "rename", "work", "default"))
						assertContextAuth(t, f, f, "default", typ, testAPIKey)
						login("default", "reused-secret", pair[0])
						assertContextAuth(t, f, f, "default", typ, "reused-secret")
						requireSuccess(t, f.run("auth", "logout", "--context", "default", "--yes"))
					case "implicit selection":
						for _, name := range []string{"work", "default"} {
							requireSuccess(t, f.run("config", "context", "use", name))
							// Resource calls resolve current selection, without --context.
							f.mu.Lock()
							f.body = `{"data":[]}`
							f.mu.Unlock()
							b.mu.Lock()
							b.body = `{"data":[]}`
							b.mu.Unlock()
							requireSuccess(t, f.run("user", "list"))
							server, secret := f, testAPIKey
							if name == "default" {
								server, secret = b, "instance-b-secret"
							}
							if server.lastRequest().URL.Path != "/api/v1/users" {
								t.Fatal("implicit resource called wrong instance")
							}
							header, want := n8n.HeaderAPIKey, secret
							if typ == n8n.AuthBearer {
								header, want = "Authorization", "Bearer "+secret
							}
							if typ == n8n.AuthCookie {
								header, want = "Cookie", n8n.CookieName+"="+secret
							}
							if server.lastRequest().Header.Get(header) != want {
								t.Fatal("implicit resource used wrong credential")
							}
							f.mu.Lock()
							f.body = `{"scopes":[]}`
							f.mu.Unlock()
							b.mu.Lock()
							b.body = `{"scopes":[]}`
							b.mu.Unlock()
							assertContextAuth(t, f, server, name, typ, secret)
						}
						requireSuccess(t, f.run("auth", "logout", "--yes"))
						checkOther("default")
					}
				})
			}
		}
	}
}

type pausedReader struct {
	ready, resume chan struct{}
	source        io.Reader
}

func (r *pausedReader) Read(p []byte) (int, error) {
	if r.ready != nil {
		close(r.ready)
		r.ready = nil
		<-r.resume
	}
	return r.source.Read(p)
}

func TestCLIRenameDuringConfirmationOrSecretInput(t *testing.T) {
	for _, operation := range []string{"login", "logout", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f := newFixture(t)
			saved := seedLegacyContext(t, f, config.StorageKeyring, n8n.AuthAPIKey)
			ready, resume := make(chan struct{}), make(chan struct{})
			reader := &pausedReader{ready: ready, resume: resume, source: strings.NewReader("y\n")}
			args := []string{"auth", "logout"}
			if operation == "login" {
				reader.source = strings.NewReader("new-secret")
				args = []string{"auth", "login", "--stdin", "--yes", "--skip-verify"}
			}
			if operation == "delete" {
				args = []string{"config", "context", "delete", "default"}
			}
			done := make(chan result, 1)
			go func() {
				var out, errOut strings.Builder
				interactive := true
				code := Run(context.Background(), args, Options{
					Streams:   Streams{In: reader, Out: &out, Err: &errOut},
					ConfigDir: f.dir, Keyring: f.keyring, Interactive: &interactive,
					Env: func(string) string { return "" },
				})
				done <- result{code: code, stdout: out.String(), stderr: errOut.String()}
			}()
			<-ready
			requireSuccess(t, f.run("config", "context", "rename", "default", "work"))
			close(resume)
			got := <-done
			if got.code != ExitError || !strings.Contains(got.stderr, "changed") {
				t.Fatalf("stale command = %+v", got)
			}
			if f.config().Contexts["work"] != saved || len(f.config().Contexts) != 1 {
				t.Fatal("stale command changed config")
			}
			assertContextAuth(t, f, f, "work", n8n.AuthAPIKey, testAPIKey)
		})
	}
}

type failedCleanupStore struct{ config.CredentialStore }

func (s failedCleanupStore) Delete(string) error {
	return errors.New("backend echoed secret-must-not-leak")
}

func TestCredentialCleanupDiagnostics(t *testing.T) {
	for _, operation := range []string{"login", "purge", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f := newFixture(t)
			saved := seedLegacyContext(t, f, config.StorageKeyring, n8n.AuthAPIKey)
			f.keyring = failedCleanupStore{f.keyring}
			args := []string{"auth", "login", "--yes", "--skip-verify"}
			if operation == "purge" {
				args = []string{"auth", "logout", "--purge", "--yes"}
			}
			if operation == "delete" {
				args = []string{"config", "context", "delete", "default", "--yes"}
			}
			got := f.run(args...)
			if got.stdout != "" || strings.Contains(got.stderr, "secret-must-not-leak") {
				t.Fatalf("cleanup diagnostic = %+v", got)
			}
			if _, err := f.keyring.Get(saved.CredentialRef); err != nil {
				t.Fatal("failed cleanup lost credential")
			}
			if operation == "login" {
				requireSuccess(t, got)
				if !strings.Contains(got.stderr, "Warning:") {
					t.Fatal("cleanup warning missing")
				}
				if !strings.Contains(got.stderr, "Credential saved") {
					t.Fatal("successful save not reported")
				}
				assertContextAuth(t, f, f, "default", n8n.AuthAPIKey, testAPIKey)
			} else {
				if got.code != ExitError {
					t.Fatal("partial cleanup must fail")
				}
				if len(f.config().Contexts) != 0 || !strings.Contains(got.stderr, "context removed") || strings.Contains(got.stderr, "Deleted") {
					t.Fatalf("cleanup failure incorrectly reported deletion: %+v", got)
				}
			}
		})
	}
}

func TestRenameWhileLoginVerifies(t *testing.T) {
	f := newFixture(t)
	saved := seedLegacyContext(t, f, config.StorageKeyring, n8n.AuthAPIKey)
	ready, resume := make(chan struct{}), make(chan struct{})
	f.route = func(_ *http.Request, _ int) (int, string) {
		close(ready)
		<-resume
		return http.StatusOK, `{"scopes":[]}`
	}
	done := make(chan result, 1)
	go func() { done <- f.run("auth", "login", "--yes") }()
	<-ready
	requireSuccess(t, f.run("config", "context", "rename", "default", "work"))
	close(resume)
	got := <-done
	if got.code != ExitError || !strings.Contains(got.stderr, "changed") {
		t.Fatalf("stale verified login = %+v", got)
	}
	f.mu.Lock()
	f.route = nil
	f.mu.Unlock()
	if f.config().Contexts["work"] != saved {
		t.Fatal("stale login changed context")
	}
	assertContextAuth(t, f, f, "work", n8n.AuthAPIKey, testAPIKey)
}
