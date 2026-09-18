package config

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// Environment variable names. Non-secret values may also come from flags; the
// three credential variables exist so CI can authenticate without writing
// anything to disk.
const (
	EnvURL         = "N8N_URL"
	EnvAPIKey      = "N8N_API_KEY"
	EnvBearerToken = "N8N_BEARER_TOKEN"
	EnvAuthCookie  = "N8N_AUTH_COOKIE"
)

// ErrStorageConsent reports that the OS credential store is unavailable and the
// plaintext fallback was not explicitly chosen.
var ErrStorageConsent = errors.New("plaintext credential storage was not selected")

// Lookup reads environment variables. Production passes [os.Getenv]; tests pass
// a map so no test depends on the ambient environment.
type Lookup func(string) string

// OSEnv reads the process environment.
func OSEnv(name string) string { return os.Getenv(name) }

// Source reports where a resolved credential came from, for `auth status` and
// for error messages that must not send the user to the wrong place.
type Source string

const (
	SourceEnv     Source = "environment"
	SourceKeyring Source = "keyring"
	SourceFile    Source = "file"
)

// Selection is what the command line asked for.
type Selection struct {
	// Context names a saved context. Empty uses the current one.
	Context string
	// URL overrides the context's instance URL. Empty uses the context or
	// the environment.
	URL string
}

// Resolution is a ready-to-use instance and credential.
type Resolution struct {
	ContextName string
	URL         string
	AuthType    n8n.AuthType
	Source      Source
	Credential  Credential
}

// Client builds an API client for this resolution.
func (r Resolution) Client(opts ...n8n.Option) (*n8n.Client, error) {
	auth, err := r.Credential.Authenticator()
	if err != nil {
		return nil, err
	}
	return n8n.New(r.URL, append([]n8n.Option{n8n.WithAuth(auth)}, opts...)...)
}

// Resolver turns a [Selection] plus saved state plus environment into a
// [Resolution]. Precedence for non-secrets is flag, environment, context. A
// credential in the environment wins over a saved one and is never persisted.
type Resolver struct {
	Store   *Store
	Keyring CredentialStore
	File    CredentialStore
	Env     Lookup
}

// NewResolver wires the real stores for a config directory.
func NewResolver(store *Store) *Resolver {
	return &Resolver{
		Store:   store,
		Keyring: NewKeyringStore(),
		File:    NewFileStore(store),
		Env:     OSEnv,
	}
}

func (r *Resolver) env(name string) string {
	if r.Env == nil {
		return OSEnv(name)
	}
	return r.Env(name)
}

// CredentialStore returns the backend for a storage kind.
func (r *Resolver) CredentialStore(kind Storage) (CredentialStore, error) {
	switch kind {
	case StorageKeyring:
		return r.Keyring, nil
	case StorageFile:
		return r.File, nil
	default:
		return nil, fmt.Errorf("unknown credential storage %q", kind)
	}
}

// EnvCredential reads a credential from the environment. It returns an error
// when more than one mechanism is set, because guessing which one the user
// meant risks sending the wrong credential to the instance.
func (r *Resolver) EnvCredential() (Credential, bool, error) {
	candidates := []struct {
		name string
		typ  n8n.AuthType
	}{
		{EnvAPIKey, n8n.AuthAPIKey},
		{EnvBearerToken, n8n.AuthBearer},
		{EnvAuthCookie, n8n.AuthCookie},
	}

	var found []string
	var cred Credential
	for _, c := range candidates {
		if value := r.env(c.name); value != "" {
			found = append(found, c.name)
			cred = Credential{Type: c.typ, Value: n8n.Secret(value)}
		}
	}
	switch len(found) {
	case 0:
		return Credential{}, false, nil
	case 1:
		return cred, true, nil
	default:
		slices.Sort(found)
		return Credential{}, false, fmt.Errorf("%v are all set: unset all but one", found)
	}
}

// Resolve produces the instance and credential for a command.
func (r *Resolver) Resolve(sel Selection) (Resolution, error) {
	cfg, err := r.Store.Load()
	if err != nil {
		return Resolution{}, err
	}

	name, saved, lookupErr := cfg.Lookup(sel.Context)

	envCred, hasEnvCred, err := r.EnvCredential()
	if err != nil {
		return Resolution{}, err
	}

	url := firstNonEmpty(sel.URL, r.env(EnvURL), saved.URL)
	if url == "" {
		if lookupErr != nil {
			return Resolution{}, lookupErr
		}
		return Resolution{}, fmt.Errorf("no instance URL: pass --url or set %s", EnvURL)
	}
	normalized, err := n8n.NormalizeBaseURL(url)
	if err != nil {
		return Resolution{}, err
	}

	if hasEnvCred {
		return Resolution{
			ContextName: name,
			URL:         normalized.String(),
			AuthType:    envCred.Type,
			Source:      SourceEnv,
			Credential:  envCred,
		}, nil
	}
	if lookupErr != nil {
		return Resolution{}, lookupErr
	}

	store, err := r.CredentialStore(saved.Storage)
	if err != nil {
		return Resolution{}, err
	}
	cred, err := store.Get(saved.CredentialRef)
	if errors.Is(err, ErrCredentialNotFound) {
		return Resolution{}, fmt.Errorf("context %q has no stored credential: run 'n8n auth login --context %s'", name, name)
	}
	if err != nil {
		return Resolution{}, err
	}
	if cred.Type != saved.AuthType {
		return Resolution{}, fmt.Errorf("context %q expects %s authentication but the stored credential is %s: run 'n8n auth login --context %s'", name, saved.AuthType, cred.Type, name)
	}

	return Resolution{
		ContextName: name,
		URL:         normalized.String(),
		AuthType:    cred.Type,
		Source:      saved.Storage.Source(),
		Credential:  cred,
	}, nil
}

// Source reports where a credential in this backend comes from.
func (s Storage) Source() Source {
	if s == StorageFile {
		return SourceFile
	}
	return SourceKeyring
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
