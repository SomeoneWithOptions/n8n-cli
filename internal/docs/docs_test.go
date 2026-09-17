package docs_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/docs"
)

var update = flag.Bool("update", false, "rewrite the command reference under docs/")

// dir is the committed reference, relative to this package.
const dir = "../../" + docs.Dir

// TestDocsAreCurrent regenerates the reference from the command tree and fails
// when the committed pages differ. Run `make docs` to update them.
func TestDocsAreCurrent(t *testing.T) {
	cmd := cli.NewRootCommand(cli.Options{Streams: cli.Streams{In: strings.NewReader("")}})

	if *update {
		if err := docs.Write(cmd, dir); err != nil {
			t.Fatalf("docs.Write: %v", err)
		}
		return
	}

	want, err := docs.Render(cmd)
	if err != nil {
		t.Fatalf("docs.Render: %v", err)
	}

	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	got := map[string][]byte{}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		got[filepath.Base(path)] = content
	}

	for name, content := range want {
		existing, ok := got[name]
		if !ok {
			t.Errorf("%s is missing from %s; run 'make docs'", name, docs.Dir)
			continue
		}
		if diff := cmp.Diff(string(content), string(existing)); diff != "" {
			t.Errorf("%s is out of date; run 'make docs' (-generated +committed):\n%s", name, diff)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s documents a command that no longer exists; run 'make docs'", name)
		}
	}
}
