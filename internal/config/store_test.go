package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "n8n-cli"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestStoreRoundTrip(t *testing.T) {
	store := newTestStore(t)

	cfg := New()
	if err := cfg.Put("production", testContext("https://n8n.example.com")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := cfg.Use("production"); err != nil {
		t.Fatalf("Use: %v", err)
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A second process reads the same directory from scratch.
	reopened, err := NewStore(store.Dir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	loaded, err := reopened.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if diff := cmp.Diff(cfg, loaded); diff != "" {
		t.Errorf("reloaded config mismatch (-saved +loaded):\n%s", diff)
	}
}

func TestStoreLoadMissingDirectoryIsEmpty(t *testing.T) {
	store := newTestStore(t)

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Contexts) != 0 || cfg.CurrentContext != "" {
		t.Errorf("Load() = %+v, want an empty configuration", cfg)
	}
	if _, err := os.Stat(store.Dir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load created %s; reading must not create anything", store.Dir())
	}
}

func TestStoreLoadRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "malformed json", content: "{not json", wantErr: "parse"},
		{name: "truncated json", content: `{"version":1,"contexts":{`, wantErr: "parse"},
		{name: "newer version", content: `{"version":99}`, wantErr: "newer than this CLI"},
		{name: "dangling current", content: `{"version":1,"currentContext":"gone"}`, wantErr: "not defined"},
		{name: "invalid context", content: `{"version":1,"contexts":{"c":{"url":"https://x","authType":"basic","credentialRef":"c","storage":"keyring"}}}`, wantErr: "unknown auth type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(store.Path(ConfigFileName), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			_, err := store.Load()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestStoreWritesOwnerOnlyFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits do not apply; Windows uses a DACL verified in the CI matrix")
	}
	store := newTestStore(t)

	// A permissive umask must not widen anything.
	defer syscallUmask(t, 0)()

	if err := store.Save(New()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := NewFileStore(store).Set("production", testCredential()); err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := map[string]fs.FileMode{
		store.Dir():                0o700,
		store.Path(ConfigFileName): 0o600,
		store.Path(SecretFileName): 0o600,
	}
	for path, mode := range want {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != mode {
			t.Errorf("%s has mode %#o, want %#o", path, got, mode)
		}
	}
}

func TestStoreRejectsSymlinkedFile(t *testing.T) {
	store := newTestStore(t)
	if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	if err := os.WriteFile(target, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, store.Path(ConfigFileName)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Load() = %v, want a symlink refusal", err)
	}
	if err := store.Save(New()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Save() = %v, want a symlink refusal", err)
	}
	// The write must not have followed the link.
	data, err := os.ReadFile(target)
	if err != nil || string(data) != `{"version":1}` {
		t.Errorf("symlink target = %q (err %v), want it untouched", data, err)
	}
}

func TestStoreRejectsGroupWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits do not apply on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores these permissions")
	}
	store := newTestStore(t)
	if err := os.MkdirAll(store.Dir(), 0o777); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(store.Dir(), 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("Load() = %v, want a directory permission refusal", err)
	}
}

func TestStoreTightensLooseDirectoryOnWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits do not apply on Windows")
	}
	store := newTestStore(t)
	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(store.Dir(), 0o755); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if err := store.Save(New()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(store.Dir())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %#o after a write, want 0700", got)
	}
}

func TestStoreUpdateSerializesConcurrentWriters(t *testing.T) {
	store := newTestStore(t)
	const writers = 8

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Go(func() {
			name := "ctx" + strconv.Itoa(i)
			errs[i] = store.Update(func(cfg *Config) error {
				return cfg.Put(name, testContext("https://n8n.example.com"))
			})
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Contexts) != writers {
		t.Errorf("stored %d contexts, want %d: a concurrent write was lost", len(cfg.Contexts), writers)
	}
}

func TestLockHeldReportsContention(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// Unix reports the lock race as Exists.
		{name: "exists", err: fmt.Errorf("openat .lock: %w", fs.ErrExist), want: true},
		// Windows reports the same race as Access Denied (ErrPermission).
		{name: "permission", err: fmt.Errorf("openat .lock: Access is denied.: %w", fs.ErrPermission), want: true},
		{name: "other", err: errors.New("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lockHeld(tt.err); got != tt.want {
				t.Errorf("lockHeld(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestStoreLockIsHeldAndReleased(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(New()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(store.Path(lockFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lock file still present after a completed write (err %v)", err)
	}

	// A live lock from another process blocks, then times out with a message
	// that names the cause.
	restore := lockTimeout
	lockTimeout = 200 * time.Millisecond
	t.Cleanup(func() { lockTimeout = restore })
	if err := os.WriteFile(store.Path(lockFileName), []byte(`{"pid":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	start := time.Now()
	err := store.Save(New())
	if err == nil || !strings.Contains(err.Error(), "another n8n process") {
		t.Fatalf("Save() = %v, want a lock timeout", err)
	}
	if elapsed := time.Since(start); elapsed < lockRetryDelay {
		t.Errorf("Save returned after %v, want it to retry first", elapsed)
	}
}

func TestStoreBreaksStaleLock(t *testing.T) {
	store := newTestStore(t)
	if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lock := store.Path(lockFileName)
	if err := os.WriteFile(lock, []byte(`{"pid":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	stale := time.Now().Add(-2 * lockStaleAfter)
	if err := os.Chtimes(lock, stale, stale); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if err := store.Save(New()); err != nil {
		t.Fatalf("Save over a stale lock: %v", err)
	}
}

func TestStoreWriteLeavesNoTemporaryFiles(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(New()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := NewFileStore(store).Set("production", testCredential()); err != nil {
		t.Fatalf("Set: %v", err)
	}

	entries, err := os.ReadDir(store.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if diff := cmp.Diff([]string{SecretFileName, ConfigFileName}, names); diff != "" {
		t.Errorf("directory contents mismatch (-want +got):\n%s", diff)
	}
}

func TestStoreSurvivesInterruptedWrite(t *testing.T) {
	store := newTestStore(t)
	cfg := New()
	if err := cfg.Put("production", testContext("https://n8n.example.com")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// What a process killed between create and rename leaves behind.
	leftover := filepath.Join(store.Dir(), ".config.json.deadbeef.tmp")
	if err := os.WriteFile(leftover, []byte("half written"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load after an interrupted write: %v", err)
	}
	if diff := cmp.Diff(cfg, loaded); diff != "" {
		t.Errorf("config mismatch (-want +got):\n%s", diff)
	}
	if err := store.Save(loaded); err != nil {
		t.Fatalf("Save after an interrupted write: %v", err)
	}
}

func TestDefaultDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	switch runtime.GOOS {
	case "windows":
		// Keep this distinct from XDG_CONFIG_HOME to verify Windows ignores it.
		base = filepath.Join(base, "AppData")
		t.Setenv("AppData", base)
	case "darwin", "ios":
		// macOS ignores XDG_CONFIG_HOME. Resolving the path does not read
		// or write any files in the user's actual config directory.
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir: %v", err)
		}
		base = filepath.Join(home, "Library", "Application Support")
	}

	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if want := filepath.Join(base, "n8n-cli"); dir != want {
		t.Errorf("DefaultDir() = %q, want %q", dir, want)
	}

	// An empty directory resolves the same way.
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store.Dir() != dir {
		t.Errorf("NewStore(\"\").Dir() = %q, want %q", store.Dir(), dir)
	}
	if want := filepath.Join(dir, ConfigFileName); store.Path(ConfigFileName) != want {
		t.Errorf("Path() = %q, want %q", store.Path(ConfigFileName), want)
	}
}

func TestCredentialStoreKinds(t *testing.T) {
	store := newTestStore(t)
	kinds := map[string]Storage{
		"file":   NewFileStore(store).Kind(),
		"memory": NewMemoryStore(StorageFile).Kind(),
		// An unknown kind falls back to the default rather than producing a
		// store that reports something no context can reference.
		"memory with an unknown kind": NewMemoryStore("vault").Kind(),
	}
	want := map[string]Storage{
		"file":                        StorageFile,
		"memory":                      StorageFile,
		"memory with an unknown kind": StorageKeyring,
	}
	for name, got := range kinds {
		if got != want[name] {
			t.Errorf("%s: Kind() = %q, want %q", name, got, want[name])
		}
	}
}

func TestLifecycleLockNeverEvictedByAge(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	restore := lockTimeout
	lockTimeout = 30 * time.Millisecond
	t.Cleanup(func() { lockTimeout = restore })
	path := store.Path(".lifecycle.lock")
	if err := os.WriteFile(path, []byte("active writer"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-2 * lockStaleAfter)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatal(err)
	}
	called := false
	err = store.WithLifecycle(func() error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "timed out") || called {
		t.Fatalf("active old lock bypassed: called=%v, err=%v", called, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock removed: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("callback failed")
	if err := store.WithLifecycle(func() error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if err := store.WithLifecycle(func() error { return store.Update(func(*Config) error { return nil }) }); err != nil {
		t.Fatalf("lock not released or nested file lock failed: %v", err)
	}
}
