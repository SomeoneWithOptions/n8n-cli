//go:build windows

package config

import "testing"

// syscallUmask has no Windows counterpart; the tests that use it skip there.
func syscallUmask(t *testing.T, _ int) func() {
	t.Helper()
	return func() {}
}
