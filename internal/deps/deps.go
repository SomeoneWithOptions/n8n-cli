// Package deps guards the module graph.
//
// The plan fixes the dependency set: cobra, x/term and go-keyring at runtime,
// go-cmp and yaml.v3 in tests, plus the two modules the toolchain promotes to
// direct requires because the code imports them (pflag through cobra's flag
// types, x/sys for the Windows ACL path). Anything else is a new dependency and
// must be a deliberate decision, not a side effect of a `go get`.
//
// The check lives here rather than in a CI script so it runs in `go test ./...`
// on every machine, and so the release workflow cannot be the first place a new
// module is noticed.
package deps

import (
	"fmt"
	"strings"
)

// Allowed lists every module that may appear as a direct requirement of this
// module, with the reason it is there. Indirect requirements are not listed:
// they are consequences of these, and `go mod tidy -diff` already pins them.
var Allowed = map[string]string{
	"github.com/spf13/cobra":        "runtime: command tree",
	"github.com/spf13/pflag":        "runtime: flag types cobra exposes directly",
	"github.com/zalando/go-keyring": "runtime: OS credential storage",
	"golang.org/x/sys":              "runtime: Windows ACLs on config and secret files",
	"golang.org/x/term":             "runtime: no-echo credential prompt",
	"github.com/google/go-cmp":      "test: diffs in assertions",
	"gopkg.in/yaml.v3":              "test: OpenAPI documents in the coverage tests",
}

// Requirement is one direct requirement of the main module.
type Requirement struct {
	Path    string
	Version string
}

// Direct returns the direct requirements declared in a go.mod file, in the
// order they appear. Indirect requirements are skipped.
//
// The parser is deliberately small: go.mod is a line-oriented file written by
// the toolchain, and reading it with the standard library keeps this check free
// of the golang.org/x/mod dependency it exists to prevent.
func Direct(gomod []byte) ([]Requirement, error) {
	var (
		reqs    []Requirement
		inBlock bool
	)
	for n, raw := range strings.Split(string(gomod), "\n") {
		line := strings.TrimSpace(raw)
		if inBlock && line == ")" {
			inBlock = false
			continue
		}
		switch {
		case !inBlock && line == "require (":
			inBlock = true
			continue
		case !inBlock && strings.HasPrefix(line, "require "):
			line = strings.TrimPrefix(line, "require ")
		case !inBlock:
			continue
		}

		body, comment, _ := strings.Cut(line, "//")
		if strings.TrimSpace(comment) == "indirect" {
			continue
		}
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		path, version, ok := strings.Cut(body, " ")
		if !ok {
			return nil, fmt.Errorf("go.mod:%d: cannot parse requirement %q", n+1, line)
		}
		reqs = append(reqs, Requirement{Path: path, Version: strings.TrimSpace(version)})
	}
	return reqs, nil
}
