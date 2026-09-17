package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// File names inside the config directory.
const (
	ConfigFileName = "config.json"
	SecretFileName = "auth.json"
	lockFileName   = ".lock"
)

// Unix permissions. The directory is owner-only because auth.json can live in
// it; both files are created restricted before any byte is written.
const (
	dirPerm    fs.FileMode = 0o700
	filePerm   fs.FileMode = 0o600
	secretPerm fs.FileMode = 0o600
)

// Lock timing. A writer that dies without releasing the lock blocks other
// writers for at most lockStaleAfter. These are variables so tests can wait
// milliseconds instead of seconds.
var (
	lockTimeout    = 5 * time.Second
	lockRetryDelay = 10 * time.Millisecond
	lockStaleAfter = 30 * time.Second
)

// DefaultDir is the config directory for the current user, normally
// ${XDG_CONFIG_HOME:-~/.config}/n8n-cli on Linux.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "n8n-cli"), nil
}

// Store reads and writes the files in one config directory.
//
// Every write is atomic (same-directory temporary file, restrictive mode,
// fsync, rename) and serialized against other processes by a lock file. Every
// path is opened through [os.Root], so a symlinked component cannot redirect a
// write out of the directory.
type Store struct {
	dir string
}

// NewStore builds a store for dir. An empty dir resolves to [DefaultDir].
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		resolved, err := DefaultDir()
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	return &Store{dir: dir}, nil
}

// Dir returns the config directory path.
func (s *Store) Dir() string { return s.dir }

// Path returns the full path of a file in the config directory. It is for
// messages and tests; reads and writes go through [os.Root].
func (s *Store) Path(name string) string { return filepath.Join(s.dir, name) }

// Load reads config.json. A missing directory or file yields an empty
// configuration, so a first run needs no setup step.
func (s *Store) Load() (*Config, error) {
	data, err := s.readFile(ConfigFileName, false)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(), nil
	case err != nil:
		return nil, err
	}

	return decodeConfig(data, s.Path(ConfigFileName))
}

// Save validates and atomically replaces config.json.
func (s *Store) Save(cfg *Config) error {
	data, err := encodeConfig(cfg)
	if err != nil {
		return err
	}
	return s.withLock(func(root *os.Root) error {
		return s.writeFileLocked(root, ConfigFileName, data, filePerm, false)
	})
}

// Update applies fn to the stored configuration and writes the result, holding
// the directory lock across the read and the write so two concurrent logins
// cannot lose one another's context.
func (s *Store) Update(fn func(*Config) error) error {
	return s.withLock(func(root *os.Root) error {
		cfg, err := s.loadLocked(root)
		if err != nil {
			return err
		}
		if err := fn(cfg); err != nil {
			return err
		}
		data, err := encodeConfig(cfg)
		if err != nil {
			return err
		}
		return s.writeFileLocked(root, ConfigFileName, data, filePerm, false)
	})
}

func encodeConfig(cfg *Config) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.Version = SchemaVersion
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return append(data, '\n'), nil
}

// loadLocked decodes config.json through an already-open root.
func (s *Store) loadLocked(root *os.Root) (*Config, error) {
	data, err := s.readFileLocked(root, ConfigFileName, false)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(), nil
	case err != nil:
		return nil, err
	}
	return decodeConfig(data, s.Path(ConfigFileName))
}

func decodeConfig(data []byte, path string) (*Config, error) {
	cfg := New()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]Context{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// withLock opens the config directory, takes the write lock and runs fn.
func (s *Store) withLock(fn func(*os.Root) error) error {
	root, err := s.openRoot(true)
	if err != nil {
		return err
	}
	defer root.Close()

	unlock, err := s.lock(root)
	if err != nil {
		return err
	}
	defer unlock()

	return fn(root)
}

// readFile reads one file from the config directory. When secret is set, the
// file's mode and ownership must be owner-only or the read fails closed.
func (s *Store) readFile(name string, secret bool) ([]byte, error) {
	root, err := s.openRoot(false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return s.readFileLocked(root, name, secret)
}

func (s *Store) readFileLocked(root *os.Root, name string, secret bool) ([]byte, error) {
	if err := checkRegular(root, name, s.Path(name), secret); err != nil {
		return nil, err
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path(name), err)
	}
	return data, nil
}

// writeFileLocked replaces one file in the config directory atomically. The
// caller holds the directory lock.
func (s *Store) writeFileLocked(root *os.Root, name string, data []byte, perm fs.FileMode, secret bool) error {
	// An existing file with loose permissions is a problem even when we are
	// about to replace it: refuse before writing a secret next to it.
	if err := checkRegular(root, name, s.Path(name), secret); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	tmp, err := tempName(name)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmp) }()

	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", s.Path(tmp), err)
	}
	if err := restrictFile(f, s.Path(tmp)); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", s.Path(tmp), err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync %s: %w", s.Path(tmp), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", s.Path(tmp), err)
	}
	if err := root.Rename(tmp, name); err != nil {
		return fmt.Errorf("replace %s: %w", s.Path(name), err)
	}
	syncDir(s.dir)
	return nil
}

// openRoot opens the config directory, creating it with owner-only permissions
// when create is set.
func (s *Store) openRoot(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(s.dir, dirPerm); err != nil {
			return nil, fmt.Errorf("create %s: %w", s.dir, err)
		}
		if err := restrictDir(s.dir); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	if err := checkDirMode(s.dir); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

// checkRegular rejects anything that is not a plain file, and for secret files
// anything readable beyond its owner.
func checkRegular(root *os.Root, name, path string, secret bool) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link: refusing to follow it", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if secret {
		return checkSecretFile(info, path)
	}
	return nil
}

// lock takes the directory lock and returns its release function.
func (s *Store) lock(root *os.Root) (func(), error) {
	deadline := time.Now().Add(lockTimeout)
	for {
		f, err := root.OpenFile(lockFileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
		if err == nil {
			fmt.Fprintf(f, "{\"pid\":%d,\"time\":%q}\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = f.Close()
			return func() { _ = root.Remove(lockFileName) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("lock %s: %w", s.Path(lockFileName), err)
		}
		if s.breakStaleLock(root) {
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for %s: another n8n process is writing configuration", s.Path(lockFileName))
		}
		time.Sleep(lockRetryDelay)
	}
}

// breakStaleLock removes a lock file left behind by a process that died before
// releasing it, and reports whether it removed one.
func (s *Store) breakStaleLock(root *os.Root) bool {
	info, err := root.Lstat(lockFileName)
	if err != nil || time.Since(info.ModTime()) < lockStaleAfter {
		return false
	}
	return root.Remove(lockFileName) == nil
}

// tempName builds an unpredictable same-directory temporary name so a
// concurrent or hostile process cannot pre-create it.
func tempName(name string) (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate temporary file name: %w", err)
	}
	return "." + name + "." + hex.EncodeToString(buf[:]) + ".tmp", nil
}

// syncDir flushes the rename itself. Best effort: a filesystem that refuses to
// open a directory is not a reason to fail a completed write.
func syncDir(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = f.Sync()
	_ = f.Close()
}
