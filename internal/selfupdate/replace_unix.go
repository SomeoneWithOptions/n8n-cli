//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
)

// replaceExecutable moves staged over target in one step. Unix unlinks the old
// inode without disturbing the process still running from it, so the update
// takes effect on the next run and never leaves a partial binary behind.
// Both paths are in the same directory, so the rename cannot cross filesystems.
func replaceExecutable(staged, target string) error {
	if err := os.Rename(staged, target); err != nil {
		return fmt.Errorf("install %s: %w", target, err)
	}
	return nil
}

// cleanupLeftovers is a no-op on Unix: replacing the binary leaves nothing to
// clean up.
func cleanupLeftovers(string) {}
