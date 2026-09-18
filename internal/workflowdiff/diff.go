// Package workflowdiff compares saved workflow definitions without instance metadata.
package workflowdiff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// Fields validates and resolves the selected definition fields. Explicit fields
// replace the defaults and can opt into pinned samples or runtime state.
func Fields(only []string, pins, state bool) ([]string, error) {
	fields := []string{"name", "description", "nodes", "connections", "settings", "nodeGroups"}
	if pins {
		fields = append(fields, "pinData")
	}
	if state {
		fields = append(fields, "staticData")
	}
	if only != nil {
		fields = slices.Clone(only)
	}
	for i, field := range fields {
		field = strings.TrimSpace(field)
		switch field {
		case "name", "description", "nodes", "connections", "settings", "nodeGroups", "pinData", "staticData":
			fields[i] = field
		default:
			return nil, fmt.Errorf("unknown comparison field %q: use name, description, nodes, connections, settings, nodeGroups, pinData or staticData", field)
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("at least one comparison field is required")
	}
	slices.Sort(fields)
	return slices.Compact(fields), nil
}

// Change describes a change from the source to the target. Path is a JSON
// Pointer into the normalized definition, where nodes are keyed by identity.
// Before/After are omitted for additions/removals; present JSON null is retained.
type Change struct {
	Kind   string          `json:"kind"`
	Path   string          `json:"path"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

// Summary counts nodes affected, not the number of changed properties.
type Summary struct {
	NodesAdded         int  `json:"nodesAdded"`
	NodesRemoved       int  `json:"nodesRemoved"`
	NodesChanged       int  `json:"nodesChanged"`
	ConnectionsChanged bool `json:"connectionsChanged"`
}

// Result contains deterministic changes and canonical documents for rendering.
type Result struct {
	Equal   bool     `json:"equal"`
	Changes []Change `json:"changes"`
	Summary Summary  `json:"summary"`
	From    []byte   `json:"-"`
	To      []byte   `json:"-"`
}

// Compare ignores top-level fields outside fields, ignores node IDs after
// matching, and preserves array order everywhere except the node collection.
// Call Fields first to validate the selected fields.
func Compare(from, to n8n.Workflow, fields []string) (Result, error) {
	a, err := normalize(from, fields)
	if err != nil {
		return Result{}, fmt.Errorf("source: %w", err)
	}
	b, err := normalize(to, fields)
	if err != nil {
		return Result{}, fmt.Errorf("target: %w", err)
	}
	if slices.Contains(fields, "nodes") {
		an, err := readNodes(a["nodes"])
		if err != nil {
			return Result{}, fmt.Errorf("source: %w", err)
		}
		bn, err := readNodes(b["nodes"])
		if err != nil {
			return Result{}, fmt.Errorf("target: %w", err)
		}
		a["nodes"], b["nodes"] = matchNodes(an, bn)
	}
	result := Result{Changes: []Change{}}
	walk(&result.Changes, "", a, true, b, true)
	result.Equal = len(result.Changes) == 0
	result.Summary = summarize(a, b)
	result.From, err = json.MarshalIndent(a, "", "  ")
	if err != nil {
		return Result{}, err
	}
	result.To, err = json.MarshalIndent(b, "", "  ")
	return result, err
}

func normalize(w n8n.Workflow, fields []string) (map[string]any, error) {
	doc, err := w.Document()
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		var value any
		if raw := doc[field]; len(raw) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber() // Never round large integer node parameters through float64.
			if err := decoder.Decode(&value); err != nil {
				return nil, fmt.Errorf("decode %s: %w", field, err)
			}
		}
		if value == nil {
			switch field {
			case "name", "description":
				value = ""
			case "nodes", "nodeGroups":
				value = []any{}
			case "connections", "settings", "pinData":
				value = map[string]any{}
			}
		}
		result[field] = value
	}
	return result, nil
}

type node struct {
	id, name string
	value    map[string]any
}

func readNodes(value any) ([]node, error) {
	array, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("nodes must be an array")
	}
	result := make([]node, 0, len(array))
	ids, names := map[string]bool{}, map[string]bool{}
	for _, v := range array {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each node must be an object")
		}
		name, _ := obj["name"].(string)
		id, _ := obj["id"].(string)
		if name == "" || names[name] || (id != "" && ids[id]) {
			return nil, fmt.Errorf("nodes require unique nonempty names and unique IDs when present (node %q)", name)
		}
		names[name], ids[id] = true, true
		delete(obj, "id")
		result = append(result, node{id: id, name: name, value: obj})
	}
	return result, nil
}

// Shared IDs take precedence. Remaining nodes use exact unique names; renames
// without a shared ID are additions/removals, never guessed from parameters.
func matchNodes(a, b []node) (map[string]any, map[string]any) {
	ids := map[string]bool{}
	for _, n := range a {
		if n.id != "" {
			ids[n.id] = true
		}
	}
	shared := map[string]bool{}
	for _, n := range b {
		if ids[n.id] {
			shared[n.id] = true
		}
	}
	index := func(nodes []node) map[string]any {
		result := make(map[string]any, len(nodes))
		for _, n := range nodes {
			key := "name:" + n.name
			if shared[n.id] {
				key = "id:" + n.id
			}
			result[key] = n.value
		}
		return result
	}
	return index(a), index(b)
}

func walk(changes *[]Change, path string, a any, hasA bool, b any, hasB bool) {
	if hasA && hasB && reflect.DeepEqual(a, b) {
		return
	}
	if hasA && hasB {
		am, aMap := a.(map[string]any)
		bm, bMap := b.(map[string]any)
		if aMap && bMap {
			keys := slices.AppendSeq(slices.Collect(maps.Keys(am)), maps.Keys(bm))
			slices.Sort(keys)
			for _, key := range slices.Compact(keys) {
				av, ap := am[key]
				bv, bp := bm[key]
				walk(changes, path+"/"+pointerToken(key), av, ap, bv, bp)
			}
			return
		}
		aa, aArray := a.([]any)
		ba, bArray := b.([]any)
		if aArray && bArray {
			for i := 0; i < max(len(aa), len(ba)); i++ {
				var av, bv any
				if i < len(aa) {
					av = aa[i]
				}
				if i < len(ba) {
					bv = ba[i]
				}
				walk(changes, path+"/"+strconv.Itoa(i), av, i < len(aa), bv, i < len(ba))
			}
			return
		}
	}
	change := Change{Kind: "changed", Path: path}
	if hasA {
		change.Before, _ = json.Marshal(a) // Values came from JSON decoding.
	} else {
		change.Kind = "added"
	}
	if hasB {
		change.After, _ = json.Marshal(b)
	} else {
		change.Kind = "removed"
	}
	*changes = append(*changes, change)
}

func pointerToken(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func summarize(a, b map[string]any) Summary {
	result := Summary{ConnectionsChanged: !reflect.DeepEqual(a["connections"], b["connections"])}
	an, _ := a["nodes"].(map[string]any)
	bn, _ := b["nodes"].(map[string]any)
	for key, before := range an {
		after, exists := bn[key]
		switch {
		case !exists:
			result.NodesRemoved++
		case !reflect.DeepEqual(before, after):
			result.NodesChanged++
		}
	}
	for key := range bn {
		if _, exists := an[key]; !exists {
			result.NodesAdded++
		}
	}
	return result
}
