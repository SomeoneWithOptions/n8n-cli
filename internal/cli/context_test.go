package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
)

func TestContextList(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		f := newFixture(t)
		got := f.run("config", "context", "list")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing on the machine-readable stream", got.stdout)
		}
		if !strings.Contains(got.stderr, "auth login") {
			t.Errorf("stderr = %q, want guidance to log in", got.stderr)
		}
	})

	t.Run("empty json", func(t *testing.T) {
		f := newFixture(t)
		got := f.run("config", "context", "list", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		if strings.TrimSpace(got.stdout) != "[]" {
			t.Errorf("stdout = %q, want an empty JSON array", got.stdout)
		}
	})

	t.Run("text", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		f.stdin = "y\n"
		f.login("--context", "staging")

		got := f.run("config", "context", "list")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		for _, want := range []string{"CURRENT", "NAME", "production", "staging", "api-key", "keyring"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}
		// The most recent login is the current context and the only marked row.
		if count := strings.Count(got.stdout, "*"); count != 1 {
			t.Errorf("stdout marks %d current contexts, want 1:\n%s", count, got.stdout)
		}
		lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
		if len(lines) != 3 {
			t.Fatalf("stdout has %d lines, want a header and two rows:\n%s", len(lines), got.stdout)
		}
		// Rows are sorted by name, and only the current one is marked.
		if !strings.Contains(lines[1], "production") || strings.HasPrefix(lines[1], "*") {
			t.Errorf("rows are not sorted by name, or the wrong row is marked:\n%s", got.stdout)
		}
		if !strings.HasPrefix(lines[2], "*") || !strings.Contains(lines[2], "staging") {
			t.Errorf("current marker is on the wrong row:\n%s", got.stdout)
		}
	})

	t.Run("json", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")

		got := f.run("config", "context", "list", "--output", "json")
		var reports []contextReport
		if err := json.Unmarshal([]byte(got.stdout), &reports); err != nil {
			t.Fatalf("decode %q: %v", got.stdout, err)
		}
		if len(reports) != 1 {
			t.Fatalf("reports = %+v, want one context", reports)
		}
		want := contextReport{
			Name:     "production",
			Current:  true,
			URL:      f.server.URL,
			AuthType: "api-key",
			Storage:  "keyring",
		}
		if reports[0] != want {
			t.Errorf("report = %+v, want %+v", reports[0], want)
		}
	})
}

func TestContextUse(t *testing.T) {
	f := newFixture(t)
	f.login("--context", "production")
	f.stdin = "y\n"
	f.login("--context", "staging")

	got := f.run("config", "context", "use", "production")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if cfg := f.config(); cfg.CurrentContext != "production" {
		t.Errorf("CurrentContext = %q, want %q", cfg.CurrentContext, "production")
	}

	unknown := f.run("config", "context", "use", "gone")
	if unknown.code != ExitError || !strings.Contains(unknown.stderr, `context "gone" not found`) {
		t.Fatalf("exit code = %d, stderr = %q", unknown.code, unknown.stderr)
	}
	if cfg := f.config(); cfg.CurrentContext != "production" {
		t.Errorf("CurrentContext = %q after a failed switch, want it unchanged", cfg.CurrentContext)
	}
}

func TestContextDelete(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		ref := f.config().Contexts["production"].CredentialRef
		f.stdin = "y\n"

		got := f.run("config", "context", "delete", "production")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		cfg := f.config()
		if _, ok := cfg.Contexts["production"]; ok {
			t.Error("the context is still stored")
		}
		if cfg.CurrentContext != "" {
			t.Errorf("CurrentContext = %q, want it cleared", cfg.CurrentContext)
		}
		if _, err := f.keyring.Get(ref); !errors.Is(err, config.ErrCredentialNotFound) {
			t.Errorf("credential store = %v, want the credential deleted with the context", err)
		}
	})

	t.Run("declined", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		f.stdin = "n\n"

		got := f.run("config", "context", "delete", "production")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
		if _, ok := f.config().Contexts["production"]; !ok {
			t.Error("a declined delete removed the context")
		}
		if _, err := f.keyring.Get(f.config().Contexts["production"].CredentialRef); err != nil {
			t.Error("a declined delete removed the credential")
		}
	})

	t.Run("non-interactive without --yes", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production")
		f.interactive = false

		got := f.run("config", "context", "delete", "production")
		if got.code != ExitError || !strings.Contains(got.stderr, "--yes") {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
	})

	t.Run("unknown context", func(t *testing.T) {
		f := newFixture(t)
		got := f.run("config", "context", "delete", "gone", "--yes")
		if got.code != ExitError || !strings.Contains(got.stderr, `context "gone" not found`) {
			t.Fatalf("exit code = %d, stderr = %q", got.code, got.stderr)
		}
	})

	t.Run("file storage", func(t *testing.T) {
		f := newFixture(t)
		f.login("--context", "production", "--storage", "file")

		got := f.run("config", "context", "delete", "production", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
		}
		data, err := os.ReadFile(filepath.Join(f.dir, config.SecretFileName))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if strings.Contains(string(data), testAPIKey) {
			t.Errorf("auth.json still holds the credential:\n%s", data)
		}
	})
}

func TestContextCompletion(t *testing.T) {
	f := newFixture(t)
	f.login("--context", "production")

	got := f.run("__complete", "config", "context", "use", "")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stdout, "production") {
		t.Errorf("stdout = %q, want the saved context name", got.stdout)
	}

	// A second argument has nothing to complete.
	extra := f.run("__complete", "config", "context", "use", "production", "")
	if strings.Contains(extra.stdout, "production") {
		t.Errorf("stdout = %q, want no completions after the first argument", extra.stdout)
	}
}

func TestConfigCommandHelp(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{{"config"}, {"config", "context"}, {"auth"}} {
		got := f.run(args...)
		if got.code != ExitSuccess {
			t.Fatalf("%v exit code = %d, want %d (stderr: %s)", args, got.code, ExitSuccess, got.stderr)
		}
		if !strings.Contains(got.stdout, "Usage:") {
			t.Errorf("%v stdout = %q, want help", args, got.stdout)
		}
	}
}
