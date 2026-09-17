package config

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeyringService is the service name under which credentials are filed in the
// OS credential store. The account is the context's credential reference.
const KeyringService = "n8n-cli"

// probeRef is looked up to decide whether a credential store exists at all. It
// is never written.
const probeRef = "\x00probe"

// ErrKeyringUnavailable reports that no OS credential store answered. On Linux
// that usually means no Secret Service (gnome-keyring, kwallet) is running.
var ErrKeyringUnavailable = errors.New("no OS credential store is available")

// keyringAPI is the slice of go-keyring used here, so tests can substitute a
// fake without touching the user's real keyring.
type keyringAPI interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type systemKeyring struct{}

func (systemKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }
func (systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}
func (systemKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }

// KeyringStore keeps credentials in the OS credential store: Secret Service on
// Linux, Keychain on macOS, Credential Manager on Windows.
type KeyringStore struct {
	service string
	api     keyringAPI
}

// NewKeyringStore builds a store backed by the real OS credential store.
func NewKeyringStore() *KeyringStore {
	return &KeyringStore{service: KeyringService, api: systemKeyring{}}
}

func (k *KeyringStore) Kind() Storage { return StorageKeyring }

// Available reports whether the OS credential store answers at all. A missing
// entry counts as available: the store responded, it just holds nothing yet.
func (k *KeyringStore) Available() error {
	_, err := k.api.Get(k.service, probeRef)
	switch {
	case err == nil, errors.Is(err, keyring.ErrNotFound):
		return nil
	default:
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
}

func (k *KeyringStore) Get(ref string) (Credential, error) {
	value, err := k.api.Get(k.service, ref)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return Credential{}, ErrCredentialNotFound
	case err != nil:
		return Credential{}, fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return decodeCredential([]byte(value))
}

func (k *KeyringStore) Set(ref string, cred Credential) error {
	encoded, err := encodeCredential(cred)
	if err != nil {
		return err
	}
	if err := k.api.Set(k.service, ref, string(encoded)); err != nil {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return nil
}

func (k *KeyringStore) Delete(ref string) error {
	err := k.api.Delete(k.service, ref)
	switch {
	case err == nil, errors.Is(err, keyring.ErrNotFound):
		return nil
	default:
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
}
