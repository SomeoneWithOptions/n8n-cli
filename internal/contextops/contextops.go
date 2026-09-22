// Package contextops coordinates local metadata and credential mutations.
// Writers acquire the lifecycle lock before either file lock. Credentials are
// staged under fresh identities, then metadata is committed, then old secrets
// are cleaned up. A crash can leave an unused credential, never a reassigned one.
// Older CLI versions do not participate in this protocol.
package contextops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
)

type metadataStore interface {
	Load() (*config.Config, error)
	Update(func(*config.Config) error) error
	WithLifecycle(func() error) error
}

// Service serializes mutations within one config directory.
type Service struct {
	store    metadataStore
	backend  func(config.Storage) (config.CredentialStore, error)
	generate func() (string, error)
}

// New builds a lifecycle service. backend may be nil for metadata-only commands.
func New(store *config.Store, backend func(config.Storage) (config.CredentialStore, error)) *Service {
	return &Service{store: store, backend: backend, generate: randomReference}
}

// Snapshot records the context observed before prompts or remote verification.
type Snapshot struct {
	Name   string
	Saved  config.Context
	Exists bool
}

// Capture records either a saved context or its absence.
func Capture(cfg *config.Config, name string) Snapshot {
	saved, exists := cfg.Contexts[name]
	return Snapshot{Name: name, Saved: saved, Exists: exists}
}

func (s Snapshot) check(cfg *config.Config) error {
	if Capture(cfg, s.Name) != s {
		return fmt.Errorf("context %q changed while this command was waiting: retry after running 'n8n config context list'", s.Name)
	}
	return nil
}

func randomReference() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", errors.New("could not generate credential reference: retry login")
	}
	return "cred:" + hex.EncodeToString(buf[:]), nil
}

func (s *Service) allocate(cfg *config.Config, backend config.CredentialStore) (string, error) {
	for range 16 {
		ref, err := s.generate()
		if err != nil {
			return "", err
		}
		occupied := false
		for _, saved := range cfg.Contexts {
			if saved.CredentialRef == ref {
				occupied = true
				break
			}
		}
		if occupied {
			continue
		}
		_, err = backend.Get(ref)
		switch {
		case errors.Is(err, config.ErrCredentialNotFound):
			return ref, nil
		case err != nil:
			// Backends can include secret material in errors. Do not echo it.
			return "", errors.New("could not check credential storage: check backend availability and retry login")
		}
	}
	return "", errors.New("could not allocate an unused credential reference after 16 attempts: retry login")
}

func referenced(cfg *config.Config, saved config.Context, except string) bool {
	for name, other := range cfg.Contexts {
		if name != except && other.Storage == saved.Storage && other.CredentialRef == saved.CredentialRef {
			return true
		}
	}
	return false
}

// Login stages a fresh credential, commits its metadata and selects the context.
// A non-nil warning with nil error means the save succeeded but cleanup failed.
func (s *Service) Login(expected Snapshot, updated config.Context, cred config.Credential) (warning, err error) {
	err = s.store.WithLifecycle(func() error {
		cfg, err := s.store.Load()
		if err != nil {
			return err
		}
		if err := expected.check(cfg); err != nil {
			return err
		}
		backend, err := s.backend(updated.Storage)
		if err != nil {
			return err
		}
		ref, err := s.allocate(cfg, backend)
		if err != nil {
			return err
		}
		updated.CredentialRef = ref
		// Validate before staging, including callers outside the CLI.
		if err := cfg.Put(expected.Name, updated); err != nil {
			return err
		}
		if err := backend.Set(ref, cred); err != nil {
			return errors.New("could not save credential: check backend availability and retry login; configuration unchanged")
		}
		if err := s.store.Update(func(latest *config.Config) error {
			if err := expected.check(latest); err != nil {
				return err
			}
			if err := latest.Put(expected.Name, updated); err != nil {
				return err
			}
			return latest.Use(expected.Name)
		}); err != nil {
			if cleanupErr := backend.Delete(ref); cleanupErr != nil {
				return fmt.Errorf("%w; could not remove staged credential; an unused credential may remain in %s storage", err, updated.Storage)
			}
			return err
		}
		if expected.Exists && !referenced(cfg, expected.Saved, "") {
			old, backendErr := s.backend(expected.Saved.Storage)
			if backendErr != nil || old.Delete(expected.Saved.CredentialRef) != nil {
				warning = fmt.Errorf("credential saved; could not remove superseded credential from %s storage", expected.Saved.Storage)
			}
		}
		return nil
	})
	return warning, err
}

// Rename moves only metadata, without accessing credential backends.
func (s *Service) Rename(oldName, newName string) (changed bool, err error) {
	unchanged := errors.New("context name unchanged")
	err = s.store.WithLifecycle(func() error {
		return s.store.Update(func(cfg *config.Config) error {
			var renameErr error
			changed, renameErr = cfg.Rename(oldName, newName)
			if renameErr == nil && !changed {
				// Skip the metadata write for an existing same-name source.
				return unchanged
			}
			return renameErr
		})
	})
	if errors.Is(err, unchanged) {
		return false, nil
	}
	return changed && err == nil, err
}

// Use selects a context in lifecycle order.
func (s *Service) Use(name string) error {
	return s.store.WithLifecycle(func() error {
		return s.store.Update(func(cfg *config.Config) error { return cfg.Use(name) })
	})
}

// Logout removes a credential and optionally its metadata. Shared credentials
// are refused before mutation. Purge commits metadata before cleanup so a failed
// metadata write preserves access. Cleanup failure reports partial completion.
func (s *Service) Logout(expected Snapshot, purge bool) (hadCredential bool, err error) {
	err = s.store.WithLifecycle(func() error {
		cfg, err := s.store.Load()
		if err != nil {
			return err
		}
		if err := expected.check(cfg); err != nil {
			return err
		}
		if !expected.Exists {
			return &config.NotFoundError{Name: expected.Name}
		}
		if referenced(cfg, expected.Saved, expected.Name) {
			return fmt.Errorf("context %q shares its credential with another context: run 'n8n auth login --context %s' to give it a separate credential before deleting", expected.Name, expected.Name)
		}
		backend, err := s.backend(expected.Saved.Storage)
		if err != nil {
			return err
		}
		_, getErr := backend.Get(expected.Saved.CredentialRef)
		hadCredential = !errors.Is(getErr, config.ErrCredentialNotFound)
		if purge {
			if err := s.store.Update(func(latest *config.Config) error {
				if err := expected.check(latest); err != nil {
					return err
				}
				return latest.Remove(expected.Name)
			}); err != nil {
				return err
			}
		}
		if err := backend.Delete(expected.Saved.CredentialRef); err != nil {
			if purge {
				return fmt.Errorf("context removed; could not delete its credential from %s storage; the unused credential may remain", expected.Saved.Storage)
			}
			return errors.New("could not delete credential: check backend availability and retry logout")
		}
		return nil
	})
	return hadCredential, err
}
