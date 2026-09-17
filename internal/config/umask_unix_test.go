//go:build !windows

package config

import (
	"syscall"
	"testing"
)

// syscallUmask sets the process umask and returns a function restoring it, so a
// permission test asserts what the code requests rather than what the ambient
// umask happens to allow. The umask is process-wide: callers must not run in
// parallel with anything that creates files.
func syscallUmask(t *testing.T, mask int) func() {
	t.Helper()
	previous := syscall.Umask(mask)
	return func() { syscall.Umask(previous) }
}
