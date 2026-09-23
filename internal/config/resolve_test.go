package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// newTestResolver wires a resolver over a temporary directory, a fake keyring
// and an explicit environment.
func newTestResolver(t *testing.T, env map[string]string) *Resolver {
	t.Helper()
	store := newTestStore(t)
	return &Resolver{
		Store:   store,
		Keyring: NewMemoryStore(StorageKeyring),
		File:    NewFileStore(store),
		Env:     func(name string) string { return env[name] },
	}
}

// seed saves one context and its credential.
func seed(t *testing.T, r *Resolver, name string, saved Context, cred Credential) {
	t.Helper()
	if err := r.Store.Update(func(cfg *Config) error {
		if err := cfg.Put(name, saved); err != nil {
			return err
		}
		return cfg.Use(name)
	}); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if cred.Value.Empty() {
		return
	}
	backend, err := r.CredentialStore(saved.Storage)
	if err != nil {
		t.Fatalf("CredentialStore: %v", err)
	}
	if err := backend.Set(saved.CredentialRef, cred); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
}

func TestResolveUsesSavedContext(t *testing.T) {
	r := newTestResolver(t, nil)
	seed(t, r, "production", testContext("https://n8n.example.com"), testCredential())

	got, err := r.Resolve(Selection{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ContextName != "production" {
		t.Errorf("ContextName = %q, want %q", got.ContextName, "production")
	}
	if want := "https://n8n.example.com/api/v1"; got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
	if got.AuthType != n8n.AuthAPIKey || got.Source != SourceKeyring {
		t.Errorf("AuthType, Source = %q, %q; want %q, %q", got.AuthType, got.Source, n8n.AuthAPIKey, SourceKeyring)
	}
	if got.Credential.Value.Reveal() != testSecretValue {
		t.Error("resolved credential is not the stored one")
	}
}

func TestResolveURLPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		selection Selection
		env       map[string]string
		want      string
	}{
		{
			name:      "flag wins",
			selection: Selection{URL: "https://flag.example.com"},
			env:       map[string]string{EnvURL: "https://env.example.com"},
			want:      "https://flag.example.com/api/v1",
		},
		{
			name: "environment beats context",
			env:  map[string]string{EnvURL: "https://env.example.com"},
			want: "https://env.example.com/api/v1",
		},
		{
			name: "context is the fallback",
			want: "https://n8n.example.com/api/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, tt.env)
			seed(t, r, "production", testContext("https://n8n.example.com"), testCredential())

			got, err := r.Resolve(tt.selection)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.URL != tt.want {
				t.Errorf("URL = %q, want %q", got.URL, tt.want)
			}
		})
	}
}

func TestResolveEnvironmentCredentialOverridesStoredOne(t *testing.T) {
	r := newTestResolver(t, map[string]string{EnvBearerToken: "env-bearer-token"})
	seed(t, r, "production", testContext("https://n8n.example.com"), testCredential())

	got, err := r.Resolve(Selection{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != SourceEnv || got.AuthType != n8n.AuthBearer {
		t.Fatalf("Source, AuthType = %q, %q; want %q, %q", got.Source, got.AuthType, SourceEnv, n8n.AuthBearer)
	}
	if got.Credential.Value.Reveal() != "env-bearer-token" {
		t.Error("resolved credential is not the one from the environment")
	}

	// Nothing about an environment credential is written down.
	for _, name := range []string{ConfigFileName, SecretFileName} {
		data, err := os.ReadFile(r.Store.Path(name))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "env-bearer-token") {
			t.Errorf("%s contains the environment credential:\n%s", name, data)
		}
	}
	saved, err := r.Store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.Contexts["production"].AuthType != n8n.AuthAPIKey {
		t.Error("an environment credential changed the saved context")
	}
}

func TestResolveEnvironmentCredentialWithoutContext(t *testing.T) {
	r := newTestResolver(t, map[string]string{
		EnvURL:    "https://n8n.example.com",
		EnvAPIKey: "env-api-key",
	})

	got, err := r.Resolve(Selection{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ContextName != "" || got.Source != SourceEnv || got.AuthType != n8n.AuthAPIKey {
		t.Errorf("Resolve() = %+v, want an environment-only resolution", got)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		selection Selection
		seed      bool
		context   Context
		cred      Credential
		wantErr   string
	}{
		{
			name:    "no context and no environment",
			wantErr: "no context selected",
		},
		{
			name:      "unknown context",
			selection: Selection{Context: "gone"},
			seed:      true,
			context:   testContext("https://n8n.example.com"),
			cred:      testCredential(),
			wantErr:   `context "gone" not found`,
		},
		{
			name: "ambiguous environment credentials",
			env: map[string]string{
				EnvURL:         "https://n8n.example.com",
				EnvAPIKey:      "key",
				EnvBearerToken: "token",
			},
			wantErr: "unset all but one",
		},
		{
			name:    "context without a credential",
			seed:    true,
			context: testContext("https://n8n.example.com"),
			wantErr: "no stored credential",
		},
		{
			name:    "credential type does not match the context",
			seed:    true,
			context: testContext("https://n8n.example.com"),
			cred:    Credential{Type: n8n.AuthBearer, Value: "token"},
			wantErr: "expects api-key authentication but the stored credential is bearer",
		},
		{
			name:      "url without a host",
			selection: Selection{URL: "https://"},
			env:       map[string]string{EnvAPIKey: "key"},
			wantErr:   "no host",
		},
		{
			name:    "environment credential without a url",
			env:     map[string]string{EnvAPIKey: "key"},
			wantErr: "no context selected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, tt.env)
			if tt.seed {
				seed(t, r, "production", tt.context, tt.cred)
			}

			_, err := r.Resolve(tt.selection)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Resolve() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveReadsFileStorageContext(t *testing.T) {
	r := newTestResolver(t, nil)
	saved := testContext("https://n8n.example.com")
	saved.Storage = StorageFile
	seed(t, r, "production", saved, testCredential())

	got, err := r.Resolve(Selection{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != SourceFile {
		t.Errorf("Source = %q, want %q", got.Source, SourceFile)
	}
}

func TestResolutionClientSendsExactlyOneCredential(t *testing.T) {
	tests := []struct {
		name   string
		cred   Credential
		assert func(t *testing.T, r *http.Request)
	}{
		{
			name: "api key",
			cred: Credential{Type: n8n.AuthAPIKey, Value: "key-value"},
			assert: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get(n8n.HeaderAPIKey); got != "key-value" {
					t.Errorf("%s = %q, want the API key", n8n.HeaderAPIKey, got)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("request carried more than one credential")
				}
			},
		},
		{
			name: "bearer",
			cred: Credential{Type: n8n.AuthBearer, Value: "token-value"},
			assert: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer token-value" {
					t.Errorf("Authorization = %q, want the bearer token", got)
				}
				if r.Header.Get(n8n.HeaderAPIKey) != "" || r.Header.Get("Cookie") != "" {
					t.Error("request carried more than one credential")
				}
			},
		},
		{
			name: "cookie",
			cred: Credential{Type: n8n.AuthCookie, Value: "cookie-value"},
			assert: func(t *testing.T, r *http.Request) {
				cookie, err := r.Cookie(n8n.CookieName)
				if err != nil || cookie.Value != "cookie-value" {
					t.Errorf("cookie %s = %v (err %v), want the session value", n8n.CookieName, cookie, err)
				}
				if r.Header.Get(n8n.HeaderAPIKey) != "" || r.Header.Get("Authorization") != "" {
					t.Error("request carried more than one credential")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen *http.Request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Clone(r.Context())
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			resolution := Resolution{URL: server.URL, AuthType: tt.cred.Type, Credential: tt.cred}
			client, err := resolution.Client()
			if err != nil {
				t.Fatalf("Client: %v", err)
			}
			if err := client.Verify(context.Background()); err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if seen == nil {
				t.Fatal("no request reached the server")
			}
			if want := n8n.BasePath + n8n.DiscoverPath; seen.URL.Path != want {
				t.Errorf("path = %q, want %q", seen.URL.Path, want)
			}
			tt.assert(t, seen)
		})
	}
}

func TestResolutionClientRejectsUnknownCredential(t *testing.T) {
	_, err := Resolution{URL: "https://n8n.example.com", Credential: Credential{Type: "basic", Value: "x"}}.Client()
	if err == nil {
		t.Fatal("Client() = nil error, want a rejection of an unknown auth type")
	}
	if _, err := (Resolution{URL: "://", Credential: testCredential()}).Client(); err == nil {
		t.Fatal("Client() = nil error, want a URL rejection")
	}
}

func TestCredentialStoreSelection(t *testing.T) {
	r := newTestResolver(t, nil)
	if _, err := r.CredentialStore(StorageKeyring); err != nil {
		t.Errorf("CredentialStore(keyring) = %v", err)
	}
	if _, err := r.CredentialStore(StorageFile); err != nil {
		t.Errorf("CredentialStore(file) = %v", err)
	}
	if _, err := r.CredentialStore("vault"); err == nil {
		t.Error("CredentialStore(vault) = nil error, want a rejection")
	}
}

func TestNewResolverUsesTheRealBackends(t *testing.T) {
	store := newTestStore(t)
	r := NewResolver(store)
	if _, ok := r.Keyring.(*KeyringStore); !ok {
		t.Errorf("Keyring = %T, want *KeyringStore", r.Keyring)
	}
	if _, ok := r.File.(*FileStore); !ok {
		t.Errorf("File = %T, want *FileStore", r.File)
	}
	// Env defaults to the process environment without being set explicitly.
	r.Env = nil
	t.Setenv(EnvURL, "https://from-process-env.example.com")
	if got := r.env(EnvURL); got != "https://from-process-env.example.com" {
		t.Errorf("env(%s) = %q, want the process environment", EnvURL, got)
	}
}

func TestResolveContextPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name, flag, env, current, want string
	}{
		{"flag wins", "flagged", "work", "current", "flagged"},
		{"flag ignores invalid env", "flagged", ".invalid", "current", "flagged"},
		{"flag ignores unknown env", "flagged", "gone", "current", "flagged"},
		{"env wins", "", "work", "current", "work"},
		{"env without current", "", "work", "", "work"},
		{"empty env falls back", "", "", "current", "current"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, map[string]string{EnvContext: tt.env})
			for _, name := range []string{"current", "work", "flagged"} {
				saved := testContext("https://" + name + ".example.com")
				saved.CredentialRef = name
				seed(t, r, name, saved, Credential{Type: n8n.AuthAPIKey, Value: n8n.Secret(name + "-secret")})
			}
			if err := r.Store.Update(func(cfg *Config) error { cfg.CurrentContext = tt.current; return nil }); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(r.Store.Path(ConfigFileName))
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Resolve(Selection{Context: tt.flag})
			if err != nil {
				t.Fatal(err)
			}
			if got.ContextName != tt.want || got.URL != "https://"+tt.want+".example.com/api/v1" || got.Credential.Value.Reveal() != tt.want+"-secret" {
				t.Fatal("context, URL or credential did not follow selection precedence")
			}
			after, err := os.ReadFile(r.Store.Path(ConfigFileName))
			if err != nil || string(before) != string(after) {
				t.Fatal("environment selection changed saved metadata")
			}
		})
	}
}

func TestResolveRejectsBadSelectedContextWithEnvironmentAuth(t *testing.T) {
	for _, name := range []string{"gone", ".bad", " ", " work ", strings.Repeat("x", 65), "é"} {
		for _, flag := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q/flag=%v", name, flag), func(t *testing.T) {
				env := map[string]string{EnvURL: "https://env.example.com", EnvAPIKey: "env-secret", EnvContext: name}
				r := newTestResolver(t, env)
				seed(t, r, "current", testContext("https://current.example.com"), testCredential())
				var sel Selection
				if flag {
					sel.Context = name
					env[EnvContext] = "current"
				}
				_, err := r.Resolve(sel)
				source := EnvContext
				if flag {
					source = "--context"
				}
				if err == nil || !strings.Contains(err.Error(), source) || strings.Contains(err.Error(), "env-secret") {
					t.Fatalf("bad selection error = %v", err)
				}
				if name == "gone" && !IsNotFound(err) {
					t.Fatalf("missing-context error lost: %v", err)
				}
			})
		}
	}
}

func TestResolveEnvContextWithEnvOverrides(t *testing.T) {
	r := newTestResolver(t, map[string]string{
		EnvContext: "work", EnvURL: "https://override.example.com", EnvBearerToken: "env-token",
	})
	seed(t, r, "work", testContext("https://saved.example.com"), testCredential())
	got, err := r.Resolve(Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextName != "work" || got.URL != "https://override.example.com/api/v1" || got.Source != SourceEnv || got.AuthType != n8n.AuthBearer || got.Credential.Value.Reveal() != "env-token" {
		t.Fatal("env context changed URL/credential override semantics")
	}
}
