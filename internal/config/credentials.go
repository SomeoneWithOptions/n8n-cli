package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// ErrCredentialNotFound is returned by a [CredentialStore] when a reference has
// no stored value. It is not an error condition by itself: a context whose
// credential was removed externally still exists.
var ErrCredentialNotFound = errors.New("no saved credential for this context")

// Credential is one stored secret and the mechanism it belongs to. The
// mechanism travels with the value so a store can answer without config.json.
type Credential struct {
	Type  n8n.AuthType
	Value n8n.Secret
}

// Authenticator builds the transport authenticator for this credential.
func (c Credential) Authenticator() (n8n.Authenticator, error) {
	switch c.Type {
	case n8n.AuthAPIKey:
		return n8n.NewAPIKeyAuth(c.Value)
	case n8n.AuthBearer:
		return n8n.NewBearerAuth(c.Value)
	case n8n.AuthCookie:
		return n8n.NewCookieAuth(c.Value)
	default:
		return nil, fmt.Errorf("unknown auth type %q", c.Type)
	}
}

// LogValue keeps a credential redacted wherever it is logged as a whole.
func (c Credential) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("type", string(c.Type)),
		slog.String("value", n8n.Redacted),
	)
}

func (c Credential) String() string { return string(c.Type) + " " + n8n.Redacted }

// CredentialStore persists secret material outside config.json. Implementations
// must treat a reference as an opaque key and must never log a value.
type CredentialStore interface {
	// Kind reports the backend, for `n8n auth status` and for logout.
	Kind() Storage
	// Get returns the credential for ref, or [ErrCredentialNotFound].
	Get(ref string) (Credential, error)
	// Set stores or replaces the credential for ref.
	Set(ref string, cred Credential) error
	// Delete removes the credential for ref. Deleting a missing reference
	// succeeds, so logout is idempotent.
	Delete(ref string) error
}

// storedCredential is the wire shape shared by both backends. Value is a plain
// string because [n8n.Secret] deliberately marshals to a redaction.
type storedCredential struct {
	Type  n8n.AuthType `json:"type"`
	Value string       `json:"value"`
}

func encodeCredential(c Credential) ([]byte, error) {
	if c.Value.Empty() {
		return nil, n8n.ErrEmptyCredential
	}
	switch c.Type {
	case n8n.AuthAPIKey, n8n.AuthBearer, n8n.AuthCookie:
	default:
		return nil, fmt.Errorf("unknown auth type %q", c.Type)
	}
	return json.Marshal(storedCredential{Type: c.Type, Value: c.Value.Reveal()})
}

func decodeCredential(data []byte) (Credential, error) {
	var stored storedCredential
	if err := json.Unmarshal(data, &stored); err != nil {
		return Credential{}, fmt.Errorf("parse stored credential: %w", err)
	}
	if stored.Value == "" {
		return Credential{}, ErrCredentialNotFound
	}
	return Credential{Type: stored.Type, Value: n8n.Secret(stored.Value)}, nil
}

// MemoryStore is an in-process [CredentialStore]. It exists so command tests
// never touch the real keyring or write a secret to disk.
type MemoryStore struct {
	mu     sync.Mutex
	kind   Storage
	values map[string]Credential
}

// NewMemoryStore builds an empty in-memory store reporting the given kind.
func NewMemoryStore(kind Storage) *MemoryStore {
	if !kind.Valid() {
		kind = StorageKeyring
	}
	return &MemoryStore{kind: kind, values: map[string]Credential{}}
}

func (m *MemoryStore) Kind() Storage { return m.kind }

func (m *MemoryStore) Get(ref string) (Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cred, ok := m.values[ref]
	if !ok {
		return Credential{}, ErrCredentialNotFound
	}
	return cred, nil
}

func (m *MemoryStore) Set(ref string, cred Credential) error {
	if _, err := encodeCredential(cred); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[ref] = cred
	return nil
}

func (m *MemoryStore) Delete(ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, ref)
	return nil
}

// Len reports how many credentials are held. For tests.
func (m *MemoryStore) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.values)
}
