package contextops

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

type fixture struct {
	store   *config.Store
	keyring *config.MemoryStore
	file    config.CredentialStore
	service *Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{store: store, keyring: config.NewMemoryStore(config.StorageKeyring), file: config.NewFileStore(store)}
	f.service = New(store, func(kind config.Storage) (config.CredentialStore, error) {
		if kind == config.StorageFile {
			return f.file, nil
		}
		return f.keyring, nil
	})
	return f
}

func (f *fixture) load(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := f.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func savedContext(storage config.Storage) config.Context {
	return config.Context{URL: "https://example.com", AuthType: n8n.AuthAPIKey, Storage: storage, CredentialRef: "default"}
}

func (f *fixture) seed(t *testing.T, kind config.Storage) Snapshot {
	t.Helper()
	saved := savedContext(kind)
	backend, err := f.service.backend(kind)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Set(saved.CredentialRef, config.Credential{Type: n8n.AuthAPIKey, Value: "original-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Update(func(cfg *config.Config) error {
		if err := cfg.Put("default", saved); err != nil {
			return err
		}
		return cfg.Use("default")
	}); err != nil {
		t.Fatal(err)
	}
	return Capture(f.load(t), "default")
}

type faultStore struct {
	config.CredentialStore
	getErr, setErr, deleteErr error
	deleted                   []string
}

func (s *faultStore) Get(ref string) (config.Credential, error) {
	if s.getErr != nil {
		return config.Credential{}, s.getErr
	}
	return s.CredentialStore.Get(ref)
}
func (s *faultStore) Set(ref string, cred config.Credential) error {
	if s.setErr != nil {
		return s.setErr
	}
	return s.CredentialStore.Set(ref, cred)
}
func (s *faultStore) Delete(ref string) error {
	s.deleted = append(s.deleted, ref)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.CredentialStore.Delete(ref)
}

type failingMetadata struct{ metadataStore }

func (f failingMetadata) Update(func(*config.Config) error) error {
	return errors.New("metadata write failed")
}

func TestAllocate(t *testing.T) {
	t.Run("collision", func(t *testing.T) {
		f := newFixture(t)
		cfg := config.New()
		if err := cfg.Put("default", savedContext(config.StorageFile)); err != nil {
			t.Fatal(err)
		}
		cred := config.Credential{Type: n8n.AuthAPIKey, Value: "occupied-secret"}
		if err := f.keyring.Set("cred:occupied", cred); err != nil {
			t.Fatal(err)
		}
		candidates := []string{"default", "cred:occupied", "cred:free"}
		f.service.generate = func() (string, error) { ref := candidates[0]; candidates = candidates[1:]; return ref, nil }
		ref, err := f.service.allocate(cfg, f.keyring)
		if err != nil || ref != "cred:free" {
			t.Fatalf("allocate = %q, %v", ref, err)
		}
		got, err := f.keyring.Get("cred:occupied")
		if err != nil || got != cred {
			t.Fatal("occupied slot changed")
		}
	})
	for _, mode := range []string{"occupied", "backend error", "generator error"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			calls := 0
			f.service.generate = func() (string, error) {
				calls++
				if mode == "generator error" {
					return "", errors.New("entropy unavailable")
				}
				return "default", nil
			}
			backend := &faultStore{CredentialStore: f.keyring}
			if mode == "backend error" {
				backend.getErr = errors.New("secret-must-not-leak")
			}
			if mode == "occupied" {
				if err := f.keyring.Set("default", config.Credential{Type: n8n.AuthAPIKey, Value: "old"}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.service.allocate(config.New(), backend)
			if err == nil || strings.Contains(err.Error(), "secret-must-not-leak") {
				t.Fatalf("err = %v", err)
			}
			if mode == "occupied" && calls != 16 {
				t.Fatalf("attempts = %d", calls)
			}
		})
	}
}

func TestLoginFailures(t *testing.T) {
	for _, kind := range []config.Storage{config.StorageKeyring, config.StorageFile} {
		for _, mode := range []string{"get", "set", "metadata", "rollback", "cleanup"} {
			t.Run(string(kind)+"/"+mode, func(t *testing.T) {
				f := newFixture(t)
				snapshot := f.seed(t, kind)
				backend, _ := f.service.backend(kind)
				fault := &faultStore{CredentialStore: backend}
				f.service.backend = func(config.Storage) (config.CredentialStore, error) { return fault, nil }
				f.service.generate = func() (string, error) { return "cred:staged", nil }
				failure := errors.New("secret-must-not-leak")
				switch mode {
				case "get":
					fault.getErr = failure
				case "set":
					fault.setErr = failure
				case "metadata":
					f.service.store = failingMetadata{f.store}
				case "rollback":
					f.service.store = failingMetadata{f.store}
					fault.deleteErr = failure
				case "cleanup":
					fault.deleteErr = failure
				}
				warning, err := f.service.Login(snapshot, savedContext(kind), config.Credential{Type: n8n.AuthAPIKey, Value: "new-secret"})
				if strings.Contains(fmt.Sprint(warning, err), "secret-must-not-leak") {
					t.Fatal("secret leaked")
				}
				if mode == "cleanup" {
					if err != nil || warning == nil {
						t.Fatalf("warning=%v err=%v", warning, err)
					}
					if f.load(t).Contexts["default"].CredentialRef != "cred:staged" {
						t.Fatal("commit lost")
					}
					if cred, err := backend.Get("cred:staged"); err != nil || cred.Value.Reveal() != "new-secret" {
						t.Fatal("new credential lost")
					}
				} else {
					if err == nil {
						t.Fatal("expected failure")
					}
					if Capture(f.load(t), "default") != snapshot {
						t.Fatal("metadata changed on failure")
					}
				}
				if cred, err := backend.Get("default"); err != nil || cred.Value.Reveal() != "original-secret" {
					t.Fatal("old credential damaged")
				}
				if mode == "metadata" || mode == "get" || mode == "set" {
					if _, err := backend.Get("cred:staged"); !errors.Is(err, config.ErrCredentialNotFound) {
						t.Fatal("staged credential left behind")
					}
				}
				if mode == "rollback" && (len(fault.deleted) != 1 || fault.deleted[0] != "cred:staged") {
					t.Fatal("rollback targeted wrong credential")
				}
			})
		}
	}
}

func TestRenamePersistenceFailure(t *testing.T) {
	f := newFixture(t)
	snapshot := f.seed(t, config.StorageKeyring)
	f.service.store = failingMetadata{f.store}
	f.service.backend = func(config.Storage) (config.CredentialStore, error) {
		t.Fatal("rename accessed credentials")
		return nil, nil
	}
	if _, err := f.service.Rename("default", "work"); err == nil {
		t.Fatal("expected failure")
	}
	if Capture(f.load(t), "default") != snapshot {
		t.Fatal("failed rename changed metadata")
	}
}

func TestSharedCredentials(t *testing.T) {
	for _, purge := range []bool{false, true} {
		t.Run(fmt.Sprint(purge), func(t *testing.T) {
			f := newFixture(t)
			snapshot := f.seed(t, config.StorageKeyring)
			if err := f.store.Update(func(cfg *config.Config) error { return cfg.Put("other", snapshot.Saved) }); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Logout(snapshot, purge); err == nil || !strings.Contains(err.Error(), "shares") {
				t.Fatalf("logout = %v", err)
			}
			warning, err := f.service.Login(snapshot, savedContext(config.StorageKeyring), config.Credential{Type: n8n.AuthAPIKey, Value: "new"})
			if err != nil || warning != nil {
				t.Fatalf("login = %v, %v", warning, err)
			}
			if _, err := f.keyring.Get("default"); err != nil {
				t.Fatal("shared credential deleted")
			}
			if f.load(t).Contexts["default"].CredentialRef == "default" {
				t.Fatal("replacement still shares reference")
			}
		})
	}
}

func TestLogoutFailures(t *testing.T) {
	for _, mode := range []string{"metadata", "delete", "purge cleanup"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			snapshot := f.seed(t, config.StorageKeyring)
			fault := &faultStore{CredentialStore: f.keyring}
			f.service.backend = func(config.Storage) (config.CredentialStore, error) { return fault, nil }
			if mode == "metadata" {
				f.service.store = failingMetadata{f.store}
			} else {
				fault.deleteErr = errors.New("secret-must-not-leak")
			}
			_, err := f.service.Logout(snapshot, mode != "delete")
			if mode == "purge cleanup" {
				if err == nil || !strings.Contains(err.Error(), "context removed") {
					t.Fatalf("partial failure = %v", err)
				}
				if len(f.load(t).Contexts) != 0 {
					t.Fatal("purge not committed")
				}
			} else if err == nil || Capture(f.load(t), "default") != snapshot {
				t.Fatal("failure changed metadata")
			}
			if strings.Contains(fmt.Sprint(err), "secret-must-not-leak") {
				t.Fatal("secret leaked")
			}
			if _, err := f.keyring.Get("default"); err != nil {
				t.Fatal("failed operation deleted credential")
			}
		})
	}
}

func TestStaleSnapshotAfterRename(t *testing.T) {
	for _, operation := range []string{"login", "logout", "purge"} {
		t.Run(operation, func(t *testing.T) {
			f := newFixture(t)
			snapshot := f.seed(t, config.StorageKeyring)
			ready, resume := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				close(ready) // snapshot captured; prompt/verification pending
				<-resume
				var err error
				if operation == "login" {
					_, err = f.service.Login(snapshot, snapshot.Saved, config.Credential{Type: n8n.AuthAPIKey, Value: "new"})
				} else {
					_, err = f.service.Logout(snapshot, operation == "purge")
				}
				done <- err
			}()
			<-ready
			if _, err := f.service.Rename("default", "work"); err != nil {
				t.Fatal(err)
			}
			close(resume)
			if err := <-done; err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("stale operation = %v", err)
			}
			if f.load(t).Contexts["work"] != snapshot.Saved || len(f.load(t).Contexts) != 1 {
				t.Fatal("stale writer changed metadata")
			}
			if cred, err := f.keyring.Get("default"); err != nil || cred.Value.Reveal() != "original-secret" {
				t.Fatal("stale writer changed credential")
			}
		})
	}
}

func TestConcurrentLogins(t *testing.T) {
	for _, sameName := range []bool{false, true} {
		t.Run(fmt.Sprint(sameName), func(t *testing.T) {
			f := newFixture(t)
			start := make(chan struct{})
			results := make(chan error, 2)
			var ready sync.WaitGroup
			ready.Add(2)
			for i := range 2 {
				name := fmt.Sprintf("context%d", i)
				if sameName {
					name = "same"
				}
				go func() {
					snapshot := Capture(config.New(), name)
					ready.Done()
					<-start
					_, err := f.service.Login(snapshot, savedContext(config.StorageFile), config.Credential{Type: n8n.AuthAPIKey, Value: n8n.Secret(name)})
					results <- err
				}()
			}
			ready.Wait()
			close(start)
			success := 0
			for range 2 {
				if err := <-results; err == nil {
					success++
				} else if !sameName || !strings.Contains(err.Error(), "changed") {
					t.Fatal(err)
				}
			}
			want := 2
			if sameName {
				want = 1
			}
			cfg := f.load(t)
			if success != want || len(cfg.Contexts) != want {
				t.Fatalf("success=%d contexts=%d", success, len(cfg.Contexts))
			}
			refs := map[string]bool{}
			for name, saved := range cfg.Contexts {
				if refs[saved.CredentialRef] {
					t.Fatal("duplicate reference")
				}
				refs[saved.CredentialRef] = true
				cred, err := f.file.Get(saved.CredentialRef)
				if err != nil || cred.Value.Reveal() != name {
					t.Fatal("wrong credential")
				}
			}
		})
	}
}
