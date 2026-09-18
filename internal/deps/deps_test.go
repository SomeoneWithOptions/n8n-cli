package deps_test

import (
	"os"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/deps"
)

// TestGoModHasNoUnapprovedDirectRequirement fails when go.mod gains a direct
// requirement the plan does not list. Adding a dependency means adding it to
// deps.Allowed with the reason, which makes the decision reviewable.
func TestGoModHasNoUnapprovedDirectRequirement(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	reqs, err := deps.Direct(gomod)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	if len(reqs) == 0 {
		t.Fatal("no direct requirements parsed from go.mod; the parser is wrong")
	}

	seen := map[string]bool{}
	for _, req := range reqs {
		seen[req.Path] = true
		if _, ok := deps.Allowed[req.Path]; !ok {
			t.Errorf("go.mod requires %s directly, which is not in deps.Allowed; add it there with a reason or drop the dependency", req.Path)
		}
	}
	for path := range deps.Allowed {
		if !seen[path] {
			t.Errorf("deps.Allowed lists %s, which go.mod no longer requires directly; remove it", path)
		}
	}
}

func TestDirectSkipsIndirectAndParsesBothForms(t *testing.T) {
	gomod := []byte(`module example.com/m

go 1.27.1

require github.com/single/one v1.2.3

require (
	github.com/kept/two v0.1.0
	github.com/dropped/three v0.2.0 // indirect
	github.com/kept/four v0.3.0 // some other note
)

tool example.com/tool
`)

	got, err := deps.Direct(gomod)
	if err != nil {
		t.Fatalf("Direct: %v", err)
	}
	want := []deps.Requirement{
		{Path: "github.com/single/one", Version: "v1.2.3"},
		{Path: "github.com/kept/two", Version: "v0.1.0"},
		{Path: "github.com/kept/four", Version: "v0.3.0"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("requirement %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestDirectRejectsMalformedRequirement(t *testing.T) {
	gomod := []byte("module example.com/m\n\nrequire (\n\tgithub.com/broken\n)\n")
	if _, err := deps.Direct(gomod); err == nil {
		t.Fatal("expected an error for a requirement without a version")
	}
}
