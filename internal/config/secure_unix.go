//go:build !windows

package config

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// restrictDir tightens an existing config directory to owner-only. A directory
// others can write to lets them replace auth.json wholesale.
func restrictDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	if err := os.Chmod(dir, dirPerm); err != nil {
		return fmt.Errorf("restrict %s to %#o: %w", dir, dirPerm, err)
	}
	return nil
}

// checkDirMode fails closed when the config directory is writable by group or
// world, which would make every guarantee below it meaningless.
func checkDirMode(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if perm := info.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("%s has mode %#o: group- or world-writable config directory; run 'chmod 700 %s'", dir, perm, dir)
	}
	return nil
}

// restrictFile is a no-op on Unix: the file was created with an explicit mode
// and a umask can only remove bits, never add them.
func restrictFile(*os.File, string) error { return nil }

// checkSecretFile rejects a credential file that anyone but its owner can read.
func checkSecretFile(info fs.FileInfo, path string) error {
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s has mode %#o: credentials must not be readable by group or world; run 'chmod 600 %s'", path, perm, path)
	}
	// Ownership is best effort: a filesystem without Unix stat data (or a
	// container that remaps IDs) must not break an otherwise safe file.
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if uid := os.Getuid(); uid >= 0 && int(stat.Uid) != uid {
		return fmt.Errorf("%s is owned by uid %d, not by uid %d: refusing to read another user's credentials", path, stat.Uid, uid)
	}
	return nil
}
