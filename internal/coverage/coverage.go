// Package coverage holds the endpoint coverage manifest: one committed record
// per documented API operation, what phase owns it, and which CLI command
// implements it.
//
// The manifest is the artifact, not `openapi.yml`: the specification is served
// by an instance and gitignored locally, so it cannot be the thing tests read
// in CI. The opt-in test in this package cross-checks the two whenever the
// specification is present.
package coverage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed manifest.json
var manifestJSON []byte

// Status is how far an operation has got.
const (
	// StatusPlanned means a phase owns the operation but no command exists.
	StatusPlanned = "planned"
	// StatusImplemented means a CLI command covers the operation.
	StatusImplemented = "implemented"
)

// Manifest is the committed operation inventory.
type Manifest struct {
	SchemaVersion int         `json:"schemaVersion"`
	APIVersion    string      `json:"apiVersion"`
	BasePath      string      `json:"basePath"`
	Operations    []Operation `json:"operations"`
}

// Operation is one documented API operation.
//
// Method plus Path is the key, not OperationID: the specification reuses
// identifiers across paths, so an identifier alone cannot address an operation.
type Operation struct {
	OperationID string `json:"operationId"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Tag         string `json:"tag"`
	Scope       string `json:"scope"`
	// Phase is the PLAN.md implementation phase that owns this operation.
	Phase int `json:"phase"`
	// Command is the full CLI command path, e.g. "n8n discover". Empty while
	// the operation is planned.
	Command string `json:"command"`
	Status  string `json:"status"`
}

// Key addresses one operation as method plus path.
func (o Operation) Key() string { return strings.ToUpper(o.Method) + " " + o.Path }

// Load decodes the embedded manifest.
func Load() (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(manifestJSON)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode coverage manifest: %w", err)
	}
	return &m, nil
}

// ByKey indexes the operations by [Operation.Key].
func (m *Manifest) ByKey() map[string]Operation {
	index := make(map[string]Operation, len(m.Operations))
	for _, op := range m.Operations {
		index[op.Key()] = op
	}
	return index
}

// Implemented returns the operations a CLI command covers, sorted by key.
func (m *Manifest) Implemented() []Operation {
	var ops []Operation
	for _, op := range m.Operations {
		if op.Status == StatusImplemented {
			ops = append(ops, op)
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Key() < ops[j].Key() })
	return ops
}

// PhaseCounts reports total and implemented operations per phase, which is what
// a phase exit gate checks.
func (m *Manifest) PhaseCounts() map[int][2]int {
	counts := map[int][2]int{}
	for _, op := range m.Operations {
		c := counts[op.Phase]
		c[0]++
		if op.Status == StatusImplemented {
			c[1]++
		}
		counts[op.Phase] = c
	}
	return counts
}
