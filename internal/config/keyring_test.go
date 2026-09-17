package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// fakeKeyring stands in for the OS credential store. No test may touch the
// real one: it belongs to the user running the suite.
type fakeKeyring struct {
	values map[string]string
	err    error // returned by every call, as an unavailable backend would
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{values: map[string]string{}} }

func (f *fakeKeyring) Get(_, user string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.values[user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (f *fakeKeyring) Set(_, user, password string) error {
	if f.err != nil {
		return f.err
	}
	f.values[user] = password
	return nil
}

func (f *fakeKeyring) Delete(_, user string) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.values[user]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, user)
	return nil
}

func newTestKeyringStore(api keyringAPI) *KeyringStore {
	return &KeyringStore{service: KeyringService, api: api}
}

func TestKeyringStoreContract(t *testing.T) {
	credentialStoreContract(t, newTestKeyringStore(newFakeKeyring()))
}

func TestKeyringStoreKind(t *testing.T) {
	if got := newTestKeyringStore(newFakeKeyring()).Kind(); got != StorageKeyring {
		t.Errorf("Kind() = %q, want %q", got, StorageKeyring)
	}
}

func TestKeyringStoreAvailable(t *testing.T) {
	t.Run("responds", func(t *testing.T) {
		if err := newTestKeyringStore(newFakeKeyring()).Available(); err != nil {
			t.Errorf("Available() = %v, want nil: a missing entry means the store answered", err)
		}
	})

	t.Run("no backend", func(t *testing.T) {
		fake := newFakeKeyring()
		fake.err = keyring.ErrUnsupportedPlatform
		err := newTestKeyringStore(fake).Available()
		if !errors.Is(err, ErrKeyringUnavailable) {
			t.Fatalf("Available() = %v, want ErrKeyringUnavailable", err)
		}
		if !strings.Contains(err.Error(), keyring.ErrUnsupportedPlatform.Error()) {
			t.Errorf("Available() = %q, want it to name the underlying cause", err)
		}
	})
}

func TestKeyringStoreReportsUnavailableBackend(t *testing.T) {
	fake := newFakeKeyring()
	fake.err = errors.New("dbus: no such service")
	store := newTestKeyringStore(fake)

	if _, err := store.Get("production"); !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("Get() = %v, want ErrKeyringUnavailable", err)
	}
	if err := store.Set("production", testCredential()); !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("Set() = %v, want ErrKeyringUnavailable", err)
	}
	if err := store.Delete("production"); !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("Delete() = %v, want ErrKeyringUnavailable", err)
	}
}

func TestKeyringStoreStoresEncodedCredential(t *testing.T) {
	fake := newFakeKeyring()
	if err := newTestKeyringStore(fake).Set("production", testCredential()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	stored, ok := fake.values["production"]
	if !ok {
		t.Fatal("credential was filed under a different account than its reference")
	}
	if !strings.Contains(stored, `"type":"api-key"`) {
		t.Errorf("stored value = %q, want the auth type to travel with it", stored)
	}
}
