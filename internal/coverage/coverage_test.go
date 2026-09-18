package coverage_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// The OpenAPI documents at the repository root. Both are gitignored: refresh
// them with `make spec` and `make spec-upstream`. No single server serves the
// whole API, so the manifest is checked against whichever documents are
// present, and the spec tests skip when neither is.
const (
	targetSpecPath   = "../../openapi.yml"
	upstreamSpecPath = "../../openapi.upstream.yml"
)

// specs pairs a document with the availability values whose operations it must
// declare.
var specs = []struct {
	name       string
	path       string
	availables []string
}{
	{"target", targetSpecPath, []string{coverage.AvailabilityBoth, coverage.AvailabilityTargetOnly}},
	{"upstream", upstreamSpecPath, []string{coverage.AvailabilityBoth, coverage.AvailabilityUpstreamOnly}},
}

// pathParameter matches one OpenAPI path parameter, so keys can be compared
// across documents that name the same parameter differently: /tags/{id} and
// /tags/{tagId} are the same operation.
var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

// normalizeKey reduces a method and path to the form both documents share.
func normalizeKey(method, path string) string {
	return strings.ToUpper(method) + " " + pathParameter.ReplaceAllString(path, "{}")
}

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
		switch op.Availability {
		case coverage.AvailabilityBoth, coverage.AvailabilityTargetOnly, coverage.AvailabilityUpstreamOnly:
		default:
			t.Errorf("%s: unknown availability %q", op.Key(), op.Availability)
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

// specOperations reads normalized method+path keys from one OpenAPI document,
// mapped to the identifiers that document offers for the operation. It reports
// false when the document is absent, because both are gitignored.
//
// Both identifiers are kept because the documents disagree: POST
// /source-control/pull carries operationId pullSourceControl upstream and only
// x-eov-operation-id pull on the target instance.
func specOperations(t *testing.T, path string) (map[string][]string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
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
		t.Fatalf("parse %s: %v", path, err)
	}

	methods := map[string]bool{"get": true, "post": true, "put": true, "delete": true, "patch": true}
	ops := map[string][]string{}
	for p, item := range doc.Paths {
		for method, op := range item {
			if !methods[method] {
				continue
			}
			var ids []string
			for _, id := range []string{op.OperationID, op.EOVOperationID} {
				// "unreachable" is the placeholder the document uses where the
				// real identifier lives in operationId.
				if id != "" && id != "unreachable" {
					ids = append(ids, id)
				}
			}
			ops[normalizeKey(method, p)] = ids
		}
	}
	return ops, true
}

// TestManifestMatchesSpec fails when a documented operation disappears or
// changes, and reports operations a document has gained. New operations are a
// manifest edit, never a silently generated command.
//
// Each document is checked against the operations it is supposed to declare.
// An operation absent from one server is not missing: it belongs to the other
// document, which the availability field records.
func TestManifestMatchesSpec(t *testing.T) {
	m := load(t)
	manifest := m.ByKey()
	normalized := make(map[string]coverage.Operation, len(m.Operations))
	for _, op := range m.Operations {
		normalized[normalizeKey(op.Method, op.Path)] = op
	}

	// documented collects every identifier any present document uses for an
	// operation. The documents disagree on some of them, so the manifest must
	// match one of them rather than all of them.
	documented := map[string][]string{}
	checked := 0
	for _, spec := range specs {
		ops, present := specOperations(t, spec.path)
		if !present {
			continue
		}
		checked++
		t.Run(spec.name, func(t *testing.T) {
			var removed, added []string
			for _, op := range m.Operations {
				if !slices.Contains(spec.availables, op.Availability) {
					continue
				}
				if _, ok := ops[normalizeKey(op.Method, op.Path)]; !ok {
					removed = append(removed, op.Key()+" ("+op.OperationID+", phase "+strconv.Itoa(op.Phase)+", "+op.Availability+")")
				}
			}
			for key, ids := range ops {
				if _, ok := normalized[key]; !ok {
					added = append(added, key+" ("+strings.Join(ids, ", ")+")")
				}
			}
			sort.Strings(removed)
			sort.Strings(added)

			if len(removed) > 0 {
				t.Errorf("the %s document no longer describes %d manifest operation(s); update the manifest, its availability, and the owning phase:\n  %s",
					spec.name, len(removed), strings.Join(removed, "\n  "))
			}
			if len(added) > 0 {
				t.Errorf("the %s document describes %d operation(s) the manifest does not list; add them to the manifest and to a phase:\n  %s",
					spec.name, len(added), strings.Join(added, "\n  "))
			}

			for key, ids := range ops {
				documented[key] = append(documented[key], ids...)
			}
		})
	}
	if checked == 0 {
		target, _ := filepath.Abs(targetSpecPath)
		upstream, _ := filepath.Abs(upstreamSpecPath)
		t.Skipf("no OpenAPI document at %s or %s: run 'make spec' and 'make spec-upstream' to fetch them", target, upstream)
	}
	// Identifiers move around between releases and between documents; the key
	// stays the contract. The manifest records one identifier, which must be
	// one some document actually uses.
	for key, ids := range documented {
		op, ok := normalized[key]
		if !ok || len(ids) == 0 || slices.Contains(ids, op.OperationID) {
			continue
		}
		sort.Strings(ids)
		t.Errorf("%s: the documents call this operation %s, the manifest calls it %q",
			key, strings.Join(slices.Compact(ids), " or "), op.OperationID)
	}

	if len(manifest) != len(normalized) {
		t.Errorf("%d operations collapse to %d normalized keys; two entries differ only in path parameter names", len(manifest), len(normalized))
	}
}

// TestAvailabilityMatchesSpecs checks the recorded availability against reality
// once both documents are present. It is the guard that keeps an upstream-only
// group from being quietly dropped when the target instance is re-fetched.
func TestAvailabilityMatchesSpecs(t *testing.T) {
	target, targetPresent := specOperations(t, targetSpecPath)
	upstream, upstreamPresent := specOperations(t, upstreamSpecPath)
	if !targetPresent || !upstreamPresent {
		t.Skip("availability needs both documents: run 'make spec' and 'make spec-upstream'")
	}

	for _, op := range load(t).Operations {
		key := normalizeKey(op.Method, op.Path)
		_, inTarget := target[key]
		_, inUpstream := upstream[key]
		want := coverage.AvailabilityBoth
		switch {
		case inTarget && inUpstream:
		case inTarget:
			want = coverage.AvailabilityTargetOnly
		case inUpstream:
			want = coverage.AvailabilityUpstreamOnly
		default:
			continue // reported by TestManifestMatchesSpec
		}
		if op.Availability != want {
			t.Errorf("%s: availability = %q, want %q", op.Key(), op.Availability, want)
		}
	}
}

// deliveredThrough is the last PLAN.md phase whose operations are all
// implemented. Raise it when a phase is finished, never before.
//
// A phase is never excused because one server does not serve its resource:
// every phase owns operations from the union of the documents.
const deliveredThrough = 26

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
