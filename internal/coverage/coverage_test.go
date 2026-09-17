package coverage_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/coverage"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// specPath is the OpenAPI document at the repository root. It is gitignored:
// refresh it with `make spec`, and every test here skips without it.
const specPath = "../../openapi.yml"

func load(t *testing.T) *coverage.Manifest {
	t.Helper()
	m, err := coverage.Load()
	if err != nil {
		t.Fatalf("coverage.Load: %v", err)
	}
	return m
}

func TestManifestIsWellFormed(t *testing.T) {
	m := load(t)

	if m.SchemaVersion != 1 {
		t.Errorf("schemaVersion = %d, want 1", m.SchemaVersion)
	}
	if m.BasePath != n8n.BasePath {
		t.Errorf("basePath = %q, want %q", m.BasePath, n8n.BasePath)
	}
	if len(m.Operations) == 0 {
		t.Fatal("the manifest lists no operations")
	}

	seen := map[string]bool{}
	for _, op := range m.Operations {
		if seen[op.Key()] {
			t.Errorf("%s: duplicate operation key", op.Key())
		}
		seen[op.Key()] = true

		if op.OperationID == "" {
			t.Errorf("%s: operationId is empty", op.Key())
		}
		if !strings.HasPrefix(op.Path, "/") {
			t.Errorf("%s: path must be relative to %s and start with /", op.Key(), n8n.BasePath)
		}
		if op.Tag == "" {
			t.Errorf("%s: tag is empty", op.Key())
		}
		if op.Phase <= 0 {
			t.Errorf("%s: phase = %d, want the PLAN.md phase that owns it", op.Key(), op.Phase)
		}
		switch op.Status {
		case coverage.StatusPlanned:
			if op.Command != "" {
				t.Errorf("%s: status is planned but command is %q", op.Key(), op.Command)
			}
		case coverage.StatusImplemented:
			if op.Command == "" {
				t.Errorf("%s: status is implemented but no command is recorded", op.Key())
			}
		default:
			t.Errorf("%s: unknown status %q", op.Key(), op.Status)
		}
	}
}

// TestImplementedOperationsHaveCommands is the "lacks a CLI mapping" half of
// the coverage gate: a manifest entry may not claim a command that the binary
// does not actually expose.
func TestImplementedOperationsHaveCommands(t *testing.T) {
	m := load(t)
	root := cli.NewRootCommand(cli.Options{Streams: cli.Streams{In: strings.NewReader("")}})

	implemented := m.Implemented()
	if len(implemented) == 0 {
		t.Fatal("no operation is implemented; phase 3 delivers at least 'n8n discover'")
	}
	for _, op := range implemented {
		cmd, ok := find(root, op.Command)
		if !ok {
			t.Errorf("%s: command %q does not exist in the CLI", op.Key(), op.Command)
			continue
		}
		if cmd.RunE == nil && cmd.Run == nil {
			t.Errorf("%s: command %q is a group, not a runnable command", op.Key(), op.Command)
		}
	}
}

// find resolves a full command path such as "n8n discover".
func find(root *cobra.Command, path string) (*cobra.Command, bool) {
	fields := strings.Fields(path)
	if len(fields) == 0 || fields[0] != root.Name() {
		return nil, false
	}
	cmd := root
	for _, name := range fields[1:] {
		next, _, err := cmd.Find([]string{name})
		if err != nil || next == cmd {
			return nil, false
		}
		cmd = next
	}
	return cmd, true
}

// specOperations reads method+path keys from the OpenAPI document. It returns
// nil when the document is absent, because it is gitignored.
func specOperations(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(specPath)
	if os.IsNotExist(err) {
		abs, _ := filepath.Abs(specPath)
		t.Skipf("no OpenAPI document at %s: run 'make spec' to fetch it", abs)
	}
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}

	var doc struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
		Paths map[string]map[string]struct {
			OperationID    string `yaml:"operationId"`
			EOVOperationID string `yaml:"x-eov-operation-id"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}

	methods := map[string]bool{"get": true, "post": true, "put": true, "delete": true, "patch": true}
	ops := map[string]string{}
	for path, item := range doc.Paths {
		for method, op := range item {
			if !methods[method] {
				continue
			}
			id := op.OperationID
			if id == "" {
				id = op.EOVOperationID
			}
			ops[strings.ToUpper(method)+" "+path] = id
		}
	}
	return ops
}

// TestManifestMatchesSpec fails when a documented operation disappears or
// changes, and reports operations the specification has gained. New operations
// are a manifest edit, never a silently generated command.
func TestManifestMatchesSpec(t *testing.T) {
	spec := specOperations(t)
	m := load(t)
	manifest := m.ByKey()

	var removed, added []string
	for key, op := range manifest {
		if _, ok := spec[key]; !ok {
			removed = append(removed, key+" ("+op.OperationID+", phase "+strconv.Itoa(op.Phase)+")")
		}
	}
	for key, id := range spec {
		if _, ok := manifest[key]; !ok {
			added = append(added, key+" ("+id+")")
		}
	}
	sort.Strings(removed)
	sort.Strings(added)

	if len(removed) > 0 {
		t.Errorf("the API no longer documents %d manifest operation(s); the manifest and the owning phase need updating:\n  %s",
			len(removed), strings.Join(removed, "\n  "))
	}
	if len(added) > 0 {
		t.Errorf("the API documents %d operation(s) the manifest does not list; add them to the manifest and to a phase:\n  %s",
			len(added), strings.Join(added, "\n  "))
	}

	// Identifiers move around between releases; the key stays the contract.
	for key, id := range spec {
		if op, ok := manifest[key]; ok && id != "" && op.OperationID != id {
			t.Errorf("%s: operationId = %q in the spec, %q in the manifest", key, id, op.OperationID)
		}
	}
}

// deliveredThrough is the last PLAN.md phase whose operations are all
// implemented. Raise it when a phase is finished, never before.
const deliveredThrough = 17

// TestPhaseCounts is how a phase exit gate reads the manifest: every operation
// a delivered phase owns must be implemented, and no later phase may have
// started, because phases run in order.
func TestPhaseCounts(t *testing.T) {
	counts := load(t).PhaseCounts()

	// Phases 0 to 2 add no operation, so the first phase with operations is 3.
	for phase := 3; phase <= deliveredThrough; phase++ {
		c, ok := counts[phase]
		if !ok {
			t.Errorf("phase %d owns no operation, but it is marked delivered", phase)
			continue
		}
		if c[1] != c[0] {
			t.Errorf("phase %d: %d of %d operations implemented, want all of them", phase, c[1], c[0])
		}
	}
	for phase, c := range counts {
		if phase > deliveredThrough && c[1] != 0 {
			t.Errorf("phase %d reports %d implemented operations, but phases run in order", phase, c[1])
		}
	}
}
