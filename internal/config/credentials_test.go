package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const testSecretValue = "n8n_api_key_value_that_must_not_leak"

func testCredential() Credential {
	return Credential{Type: n8n.AuthAPIKey, Value: n8n.Secret(testSecretValue)}
}

func TestCredentialAuthenticator(t *testing.T) {
	tests := []struct {
		name    string
		cred    Credential
		want    n8n.AuthType
		wantErr bool
	}{
		{name: "api key", cred: Credential{Type: n8n.AuthAPIKey, Value: "k"}, want: n8n.AuthAPIKey},
		{name: "bearer", cred: Credential{Type: n8n.AuthBearer, Value: "k"}, want: n8n.AuthBearer},
		{name: "cookie", cred: Credential{Type: n8n.AuthCookie, Value: "k"}, want: n8n.AuthCookie},
		{name: "unknown type", cred: Credential{Type: "basic", Value: "k"}, wantErr: true},
		{name: "empty value", cred: Credential{Type: n8n.AuthAPIKey}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := tt.cred.Authenticator()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Authenticator() = %v, want an error", auth)
				}
				return
			}
			if err != nil {
				t.Fatalf("Authenticator: %v", err)
			}
			if got := auth.Type(); got != tt.want {
				t.Errorf("Type() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCredentialNeverRendersItsValue(t *testing.T) {
	cred := testCredential()

	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("auth", "credential", cred)

	// Formatted through an any so the verbs, not a direct String call, are
	// what the test exercises.
	var boxed any = cred

	renderings := map[string]string{
		"%v":     fmt.Sprintf("%v", boxed),
		"%s":     fmt.Sprintf("%s", boxed),
		"%+v":    fmt.Sprintf("%+v", boxed),
		"error":  fmt.Errorf("login failed with %v", boxed).Error(),
		"String": cred.String(),
		"slog":   logged.String(),
	}
	for name, rendered := range renderings {
		if strings.Contains(rendered, testSecretValue) {
			t.Errorf("%s rendered the credential value: %s", name, rendered)
		}
		if !strings.Contains(rendered, n8n.Redacted) {
			t.Errorf("%s = %q, want it to show %s", name, rendered, n8n.Redacted)
		}
	}
}

func TestEncodeDecodeCredential(t *testing.T) {
	encoded, err := encodeCredential(testCredential())
	if err != nil {
		t.Fatalf("encodeCredential: %v", err)
	}
	if !strings.Contains(string(encoded), testSecretValue) {
		t.Fatalf("encoded credential does not carry the value: %s", encoded)
	}

	decoded, err := decodeCredential(encoded)
	if err != nil {
		t.Fatalf("decodeCredential: %v", err)
	}
	if diff := cmp.Diff(testCredential(), decoded); diff != "" {
		t.Errorf("credential mismatch (-want +got):\n%s", diff)
	}

	if _, err := decodeCredential([]byte("{not json")); err == nil {
		t.Error("decodeCredential(garbage) = nil, want an error")
	}
	if _, err := decodeCredential([]byte(`{"type":"api-key","value":""}`)); !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("decodeCredential(empty value) = %v, want ErrCredentialNotFound", err)
	}
}

// credentialStoreContract is the behavior every backend must share.
func credentialStoreContract(t *testing.T, store CredentialStore) {
	t.Helper()

	if _, err := store.Get("missing"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrCredentialNotFound", err)
	}
	if err := store.Delete("missing"); err != nil {
		t.Fatalf("Delete(missing) = %v, want nil: logout must be idempotent", err)
	}

	if err := store.Set("production", testCredential()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	other := Credential{Type: n8n.AuthBearer, Value: "bearer-token"}
	if err := store.Set("staging", other); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get("production")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != testCredential() {
		t.Errorf("Get() = %v, want the stored credential", got)
	}

	if err := store.Delete("production"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("production"); !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("Get(deleted) = %v, want ErrCredentialNotFound", err)
	}
	if got, err := store.Get("staging"); err != nil || got != other {
		t.Errorf("Get(staging) = %v, %v; deleting one reference must not affect another", got, err)
	}

	if err := store.Set("production", Credential{Type: n8n.AuthAPIKey}); err == nil {
		t.Error("Set(empty value) = nil, want an error")
	}
}

func TestMemoryStoreContract(t *testing.T) {
	credentialStoreContract(t, NewMemoryStore(StorageKeyring))
}

func TestFileStoreContract(t *testing.T) {
	credentialStoreContract(t, NewFileStore(newTestStore(t)))
}

func TestFileStoreWritesOnlyTheReferencedSecret(t *testing.T) {
	store := newTestStore(t)
	file := NewFileStore(store)
	if err := file.Set("production", testCredential()); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, err := os.ReadFile(file.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{`"version": 1`, `"production"`, `"api-key"`, testSecretValue} {
		if !strings.Contains(string(data), want) {
			t.Errorf("auth.json missing %q:\n%s", want, data)
		}
	}
}

func TestFileStoreRejectsWorldReadableSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits do not apply on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores these permissions")
	}
	store := newTestStore(t)
	file := NewFileStore(store)
	if err := file.Set("production", testCredential()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := os.Chmod(file.Path(), 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, err := file.Get("production")
	if err == nil || !strings.Contains(err.Error(), "readable by group or world") {
		t.Fatalf("Get() = %v, want a permission refusal", err)
	}
	if err := file.Set("production", testCredential()); err == nil || !strings.Contains(err.Error(), "readable by group or world") {
		t.Fatalf("Set() = %v, want a permission refusal", err)
	}
}

func TestFileStoreRejectsSymlinkedSecretFile(t *testing.T) {
	store := newTestStore(t)
	file := NewFileStore(store)
	if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := os.DevNull
	if err := os.Symlink(target, file.Path()); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := file.Get("production"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Get() = %v, want a symlink refusal", err)
	}
	if err := file.Set("production", testCredential()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Set() = %v, want a symlink refusal", err)
	}
}

func TestFileStoreRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "malformed json", content: "{", wantErr: "parse"},
		{name: "newer version", content: `{"version":99,"credentials":{}}`, wantErr: "newer than this CLI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			file := NewFileStore(store)
			if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(file.Path(), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			if _, err := file.Get("production"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Get() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFileStoreGetWithoutFile(t *testing.T) {
	file := NewFileStore(newTestStore(t))
	if _, err := file.Get("production"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() = %v, want ErrCredentialNotFound", err)
	}
	if err := file.Delete("production"); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	if _, err := os.Stat(file.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Delete created %s (err %v); removing nothing must write nothing", file.Path(), err)
	}
}
