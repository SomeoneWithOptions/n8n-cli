//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

// oldSuffix marks the binary moved aside by an update.
const oldSuffix = ".old"

// replaceExecutable installs staged as target on Windows, which refuses to
// overwrite or delete the file a running process was started from but does
// allow renaming it. So the running binary is moved aside first and the new one
// takes its place; a failure in between is rolled back. The file moved aside
// cannot be deleted until the process exits, so it is only best-effort removed
// here and swept by [cleanupLeftovers] on the next update.
func replaceExecutable(staged, target string) error {
	old := target + oldSuffix
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("move the running binary aside (%s): %w", target, err)
	}
	if err := os.Rename(staged, target); err != nil {
		if back := os.Rename(old, target); back != nil {
			return fmt.Errorf("install %s: %w; the previous binary is at %s and could not be restored (%v): rename it back by hand", target, err, old, back)
		}
		return fmt.Errorf("install %s: %w", target, err)
	}
	_ = os.Remove(old)
	return nil
}

// cleanupLeftovers removes the binary a previous update moved aside, which is
// deletable once that process has exited.
func cleanupLeftovers(target string) { _ = os.Remove(target + oldSuffix) }
