package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
)

// result captures one in-process run of the command tree.
type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), args, Options{
		Streams: Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version: testVersion,
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

var testVersion = version.Info{
	Version:   "1.2.3",
	Commit:    "abcdef0",
	Date:      "2026-01-02T03:04:05Z",
	GoVersion: "go1.27.1",
	Platform:  "linux/amd64",
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {}} {
		t.Run(strings.Join(append([]string{"n8n"}, args...), " "), func(t *testing.T) {
			got := run(t, args...)
			if got.code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
			}
			for _, want := range []string{"Usage:", "n8n", "version", "completion"} {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
				}
			}
			if got.stderr != "" {
				t.Errorf("stderr = %q, want empty", got.stderr)
			}
		})
	}
}

func TestRunVersionCommand(t *testing.T) {
	got := run(t, "version")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{"1.2.3", "abcdef0", "2026-01-02T03:04:05Z", "go1.27.1", "linux/amd64"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
}

func TestRunVersionFlag(t *testing.T) {
	got := run(t, "--version")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if want := "n8n 1.2.3\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
}

func TestRunVersionRejectsArgs(t *testing.T) {
	got := run(t, "version", "extra")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	if !strings.Contains(got.stderr, "unknown command") && !strings.Contains(got.stderr, "arg") {
		t.Errorf("stderr = %q, want an argument error", got.stderr)
	}
}

func TestRunCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			got := run(t, "completion", shell)
			if got.code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
			}
			if len(got.stdout) == 0 {
				t.Fatal("stdout is empty, want a completion script")
			}
			if !strings.Contains(got.stdout, "n8n") {
				t.Errorf("completion script does not mention the binary\n%s", got.stdout)
			}
			if got.stderr != "" {
				t.Errorf("stderr = %q, want empty", got.stderr)
			}
		})
	}
}

func TestRunCompletionInvalidShell(t *testing.T) {
	tests := map[string][]string{
		"unsupported shell": {"completion", "elvish"},
		"no shell":          {"completion"},
		"two shells":        {"completion", "bash", "zsh"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			got := run(t, args...)
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d", got.code, ExitError)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty", got.stdout)
			}
			if got.stderr == "" {
				t.Error("stderr is empty, want an error message")
			}
		})
	}
}

func TestRunUnknownCommand(t *testing.T) {
	got := run(t, "workflows")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	if !strings.Contains(got.stderr, "unknown command") {
		t.Errorf("stderr = %q, want an unknown-command error", got.stderr)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	got := run(t, "version", "--nope")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "unknown flag") {
		t.Errorf("stderr = %q, want an unknown-flag error", got.stderr)
	}
}

func TestReportExitCodes(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string
	}{
		{name: "success", err: nil, wantCode: ExitSuccess},
		{name: "canceled", err: canceled.Err(), wantCode: ExitCanceled, wantErr: "canceled"},
		{name: "wrapped cancel", err: fmt.Errorf("get workflow: %w", canceled.Err()), wantCode: ExitCanceled, wantErr: "canceled"},
		{name: "deadline", err: expired.Err(), wantCode: ExitCanceled, wantErr: "canceled"},
		{name: "failure", err: errors.New("boom"), wantCode: ExitError, wantErr: "n8n: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if code := report(&errOut, tt.err); code != tt.wantCode {
				t.Errorf("report(%v) = %d, want %d", tt.err, code, tt.wantCode)
			}
			if tt.wantErr == "" {
				if errOut.String() != "" {
					t.Errorf("stderr = %q, want empty", errOut.String())
				}
				return
			}
			if !strings.Contains(errOut.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

func TestVersionFlagHasNoShorthand(t *testing.T) {
	// -v stays free for verbose output in a later phase.
	got := run(t, "-v")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d; -v must not be bound", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "shorthand") {
		t.Errorf("stderr = %q, want an unknown-shorthand error", got.stderr)
	}
}
