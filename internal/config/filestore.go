package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// FileStore keeps credentials in auth.json next to config.json.
//
// This is a fallback for machines with no working OS credential store. The file
// is plaintext: it is protected by file permissions, not by encryption. Anything
// running as this user can read it. It is never selected implicitly.
type FileStore struct {
	store *Store
}

// NewFileStore builds the plaintext fallback store for a config directory.
func NewFileStore(store *Store) *FileStore { return &FileStore{store: store} }

func (f *FileStore) Kind() Storage { return StorageFile }

// Path is the file holding the secrets, for help text and diagnostics.
func (f *FileStore) Path() string { return f.store.Path(SecretFileName) }

// secretFile is the whole of auth.json.
type secretFile struct {
	Version     int                         `json:"version"`
	Credentials map[string]storedCredential `json:"credentials"`
}

func (f *FileStore) Get(ref string) (Credential, error) {
	data, err := f.store.readFile(SecretFileName, true)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Credential{}, ErrCredentialNotFound
	case err != nil:
		return Credential{}, err
	}

	parsed, err := f.decode(data)
	if err != nil {
		return Credential{}, err
	}
	stored, ok := parsed.Credentials[ref]
	if !ok || stored.Value == "" {
		return Credential{}, ErrCredentialNotFound
	}
	return Credential{Type: stored.Type, Value: n8n.Secret(stored.Value)}, nil
}

func (f *FileStore) Set(ref string, cred Credential) error {
	encoded, err := encodeCredential(cred)
	if err != nil {
		return err
	}
	var stored storedCredential
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return err
	}
	return f.update(func(sf *secretFile) error {
		sf.Credentials[ref] = stored
		return nil
	})
}

func (f *FileStore) Delete(ref string) error {
	return f.update(func(sf *secretFile) error {
		if _, ok := sf.Credentials[ref]; !ok {
			return errNoChange
		}
		delete(sf.Credentials, ref)
		return nil
	})
}

// errNoChange tells update that the file already says what the caller wants,
// so deleting a credential that is not there writes nothing at all.
var errNoChange = errors.New("no change")

// update rewrites auth.json under the directory lock, so a concurrent login for
// another context cannot drop this one.
func (f *FileStore) update(fn func(*secretFile) error) error {
	return f.store.withLock(func(root *os.Root) error {
		data, err := f.store.readFileLocked(root, SecretFileName, true)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		parsed := &secretFile{Version: SchemaVersion, Credentials: map[string]storedCredential{}}
		if len(data) > 0 {
			parsed, err = f.decode(data)
			if err != nil {
				return err
			}
		}
		if err := fn(parsed); err != nil {
			if errors.Is(err, errNoChange) {
				return nil
			}
			return err
		}
		parsed.Version = SchemaVersion

		encoded, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s: %w", f.Path(), err)
		}
		return f.store.writeFileLocked(root, SecretFileName, append(encoded, '\n'), secretPerm, true)
	})
}

func (f *FileStore) decode(data []byte) (*secretFile, error) {
	parsed := &secretFile{}
	if err := json.Unmarshal(data, parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", f.Path(), err)
	}
	if parsed.Version > SchemaVersion {
		return nil, fmt.Errorf("%s version %d is newer than this CLI understands (%d): upgrade n8n-cli", f.Path(), parsed.Version, SchemaVersion)
	}
	if parsed.Credentials == nil {
		parsed.Credentials = map[string]storedCredential{}
	}
	return parsed, nil
}
