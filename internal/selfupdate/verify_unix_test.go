//go:build !windows

package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// script writes an executable file that answers `version` like the real binary
// does. A shell script is enough: the check only runs the file and reads its
// first line, which is exactly what makes a wrong-platform or truncated
// download detectable before it is installed.
func script(t *testing.T, reported string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "staged")
	body := "#!/bin/sh\necho \"" + reported + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestVerifyBinaryAcceptsTheExpectedVersion(t *testing.T) {
	updater := New(Config{})
	staged := script(t, "n8n v1.3.0\ncommit: abcdef0")
	if err := updater.VerifyBinary(context.Background(), staged, "v1.3.0"); err != nil {
		t.Errorf("VerifyBinary: %v", err)
	}
}

func TestVerifyBinaryRejectsAWrongOrBrokenBinary(t *testing.T) {
	updater := New(Config{})
	tests := map[string]struct {
		path string
		want string
	}{
		"wrong version": {path: script(t, "n8n v1.2.0"), want: "reports"},
		"no output":     {path: script(t, ""), want: "reports"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := updater.VerifyBinary(context.Background(), tt.path, "v1.3.0")
			if err == nil {
				t.Fatal("VerifyBinary succeeded, want a failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}

	// A file that is not a program at all: the download is unusable and must
	// not reach the installed path.
	notAProgram := filepath.Join(t.TempDir(), "staged")
	if err := os.WriteFile(notAProgram, []byte("\x00\x01not a binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := updater.VerifyBinary(context.Background(), notAProgram, "v1.3.0"); err == nil {
		t.Error("VerifyBinary on a non-program succeeded, want a failure")
	}
}

func TestDestinationFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "n8n-v1.3.0")
	if err := os.WriteFile(real, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "n8n")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	dest, err := New(Config{ExecPath: link}).Destination()
	if err != nil {
		t.Fatalf("Destination: %v", err)
	}
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	// Replacing the symlink instead of its target would detach the install
	// from whatever manages the link.
	if dest.Path != want {
		t.Errorf("Path = %q, want the link target %q", dest.Path, want)
	}
}
