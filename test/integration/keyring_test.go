package integration_test

import (
	"errors"
	"os"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// envKeyring opts in to tests against the real OS credential store: Secret
// Service on Linux, Keychain on macOS, Credential Manager on Windows. They are
// opt-in because they write to the credential store of whoever runs them, and
// because an unattended machine may have no store at all.
const envKeyring = "N8N_INTEGRATION_KEYRING"

// TestKeyringStoreOnRealCredentialStore exercises the keyring backend against
// the store this machine actually has. The fake in internal/config cannot show
// that the account names and payloads the CLI sends are ones a real store
// accepts; only the real store can. Everything it writes is deleted again.
func TestKeyringStoreOnRealCredentialStore(t *testing.T) {
	if os.Getenv(envKeyring) != "1" {
		t.Skipf("set %s=1 to run tests against this machine's credential store", envKeyring)
	}

	store := config.NewKeyringStore()
	if err := store.Available(); err != nil {
		t.Fatalf("Available() = %v, want nil: the credential store must answer its own probe", err)
	}
	if got := store.Kind(); got != config.StorageKeyring {
		t.Fatalf("Kind() = %q, want %q", got, config.StorageKeyring)
	}

	// The reference is the context name the CLI would file the secret under.
	ref := integration.ResourcePrefix + "keyring"
	t.Cleanup(func() { _ = store.Delete(ref) })

	if _, err := store.Get(ref); !errors.Is(err, config.ErrCredentialNotFound) {
		t.Fatalf("Get(unset) = %v, want ErrCredentialNotFound", err)
	}
	if err := store.Delete(ref); err != nil {
		t.Fatalf("Delete(unset) = %v, want nil: logout must be idempotent", err)
	}

	cred := config.Credential{Type: n8n.AuthAPIKey, Value: n8n.Secret("n8n-cli-test-api-key")}
	if err := store.Set(ref, cred); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != cred {
		t.Errorf("Get() = %v, want the stored credential", got)
	}

	// Logging in again over an existing context replaces the secret in place.
	replacement := config.Credential{Type: n8n.AuthBearer, Value: n8n.Secret("n8n-cli-test-bearer-token")}
	if err := store.Set(ref, replacement); err != nil {
		t.Fatalf("Set(replacement): %v", err)
	}
	if got, err := store.Get(ref); err != nil || got != replacement {
		t.Errorf("Get() = %v, %v; want the replacement credential", got, err)
	}

	if err := store.Delete(ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ref); !errors.Is(err, config.ErrCredentialNotFound) {
		t.Errorf("Get(deleted) = %v, want ErrCredentialNotFound", err)
	}
}
