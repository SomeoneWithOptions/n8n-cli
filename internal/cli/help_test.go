package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestHelpIsDescriptive enforces the PLAN.md help and discoverability
// contract: a human or AI agent must be able to act from --help alone.
// Every command needs Short, Long, and Example; every flag needs a usage
// string; --help must succeed at every level.
func TestHelpIsDescriptive(t *testing.T) {
	root := NewRootCommand(Options{Streams: Streams{In: strings.NewReader(""), Out: nil, Err: nil}, Version: testVersion})

	var visit func(path string, c *cobra.Command)
	visit = func(path string, c *cobra.Command) {
		path = strings.TrimSpace(path + " " + c.Name())
		if c.Name() == "" {
			path = "n8n"
		}
		if strings.TrimSpace(c.Short) == "" {
			t.Errorf("%s: Short is empty", path)
		}
		if len(strings.TrimSpace(c.Long)) < 40 {
			t.Errorf("%s: Long too short (%d chars), want >= 40 describing workflow and next steps", path, len(strings.TrimSpace(c.Long)))
		}
		if strings.TrimSpace(c.Example) == "" {
			t.Errorf("%s: Example is empty; add copy-pasteable invocations", path)
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if strings.TrimSpace(f.Usage) == "" {
				t.Errorf("%s --%s: flag usage is empty", path, f.Name)
			}
		})
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if strings.TrimSpace(f.Usage) == "" {
				t.Errorf("%s --%s: flag usage is empty", path, f.Name)
			}
		})
		for _, sub := range c.Commands() {
			visit(path, sub)
		}
	}
	for _, sub := range root.Commands() {
		visit("n8n", sub)
	}
	// Root itself.
	if strings.TrimSpace(root.Short) == "" {
		t.Error("n8n: Short is empty")
	}
	if len(strings.TrimSpace(root.Long)) < 40 {
		t.Error("n8n: Long too short")
	}
	if strings.TrimSpace(root.Example) == "" {
		t.Error("n8n: Example is empty")
	}
}

func TestHelpSucceedsAtEveryLevel(t *testing.T) {
	paths := [][]string{
		{},
		{"auth"},
		{"auth", "login"},
		{"auth", "status"},
		{"auth", "logout"},
		{"config"},
		{"config", "context"},
		{"config", "context", "list"},
		{"config", "context", "use"},
		{"config", "context", "delete"},
		{"version"},
		{"completion"},
	}
	for _, p := range paths {
		args := append(append([]string{}, p...), "--help")
		got := run(t, args...)
		name := "n8n " + strings.Join(p, " ")
		if got.code != ExitSuccess {
			t.Errorf("%s --help: exit code = %d, want %d (stderr: %s)", name, got.code, ExitSuccess, got.stderr)
		}
		for _, want := range []string{"Usage:", "Examples:", "--help"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("%s --help: stdout missing %q\n--- stdout ---\n%s", name, want, got.stdout)
			}
		}
	}
}
