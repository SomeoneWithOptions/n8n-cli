package workflowdiff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

func workflow(t *testing.T, raw string) n8n.Workflow {
	t.Helper()
	var w n8n.Workflow
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatal(err)
	}
	return w
}

func compare(t *testing.T, a, b string, only []string) Result {
	t.Helper()
	fields, err := Fields(only, false, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Compare(workflow(t, a), workflow(t, b), fields)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestIgnoresInstanceMetadataAndNodeOrdering(t *testing.T) {
	a := `{"id":"source","versionId":"v1","versionCounter":2,"name":"Sync","active":true,
		"nodes":[{"id":"1","name":"A","parameters":{"z":2,"a":1}},{"id":"2","name":"B"}],
		"pinData":{"A":[1]},"staticData":{"last":12},"tags":[{"id":"tag-a","name":"dev"}],
		"shared":[{"projectId":"dev"}],"meta":{"instanceId":"dev"},"activeVersion":{"nodes":[]}}`
	b := `{"id":"target","versionId":"v7","versionCounter":8,"name":"Sync","active":false,
		"nodes":[{"id":"different","name":"B"},{"id":"1","name":"A","parameters":{"a":1,"z":2}}],
		"description":null,"connections":{},"settings":{},"pinData":{},"staticData":null}`
	r := compare(t, a, b, nil)
	if !r.Equal || len(r.Changes) != 0 || string(r.From) != string(r.To) {
		t.Fatalf("expected normalized equality: %+v\n%s\n%s", r, r.From, r.To)
	}
}

func TestSemanticChangesRemainVisible(t *testing.T) {
	cases := []struct {
		name, before, after, path string
	}{
		{"parameters", `{"n":1}`, `{"n":2}`, "/parameters/n"},
		{"large integers", `{"n":9007199254740992}`, `{"n":9007199254740993}`, "/parameters/n"},
		{"array order", `{"list":[1,2]}`, `{"list":[2,1]}`, "/parameters/list/0"},
		{"null vs missing", `{}`, `{"n":null}`, "/parameters/n"},
		{"pointer escaping", `{"a/b~c":1}`, `{"a/b~c":2}`, "/parameters/a~1b~0c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := `{"nodes":[{"id":"n","name":"A","parameters":` + tc.before + `}]}`
			b := `{"nodes":[{"id":"n","name":"A","parameters":` + tc.after + `}]}`
			r := compare(t, a, b, nil)
			if r.Equal || r.Summary.NodesChanged != 1 || r.Changes[0].Path != "/nodes/id:n"+tc.path {
				t.Fatalf("unexpected changes: %+v", r)
			}
			if tc.name == "null vs missing" && (r.Changes[0].Kind != "added" || string(r.Changes[0].After) != "null" || r.Changes[0].Before != nil) {
				t.Fatalf("null and missing were conflated: %+v", r.Changes[0])
			}
		})
	}
	for _, field := range []string{"credentials", "position", "disabled", "typeVersion", "unknownFutureProperty"} {
		t.Run(field, func(t *testing.T) {
			a := `{"nodes":[{"name":"A","` + field + `":1}]}`
			b := strings.Replace(a, ":1", ":2", 1)
			if r := compare(t, a, b, nil); r.Equal || r.Summary.NodesChanged != 1 {
				t.Fatalf("%s ignored: %+v", field, r)
			}
		})
	}
}

func TestNodeIdentityAndSummary(t *testing.T) {
	cases := []struct {
		name, a, b string
		want       Summary
	}{
		{"same ID renamed", `[{"id":"1","name":"A"}]`, `[{"id":"1","name":"B"}]`, Summary{NodesChanged: 1}},
		{"different ID same name", `[{"id":"1","name":"A"}]`, `[{"id":"2","name":"A"}]`, Summary{}},
		{"no guessed rename", `[{"id":"1","name":"A"}]`, `[{"id":"2","name":"B"}]`, Summary{NodesAdded: 1, NodesRemoved: 1}},
		{"ID before name", `[{"id":"1","name":"A"},{"id":"2","name":"B"}]`, `[{"id":"1","name":"B"},{"id":"2","name":"A"}]`, Summary{NodesChanged: 2}},
		{"partial shared IDs", `[{"id":"1","name":"A"},{"id":"2","name":"B"}]`, `[{"id":"1","name":"B"},{"id":"3","name":"A"}]`, Summary{NodesAdded: 1, NodesRemoved: 1, NodesChanged: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := compare(t, `{"nodes":`+tc.a+`}`, `{"nodes":`+tc.b+`}`, nil)
			if r.Summary != tc.want {
				t.Fatalf("summary = %+v, want %+v", r.Summary, tc.want)
			}
		})
	}
	a := `{"connections":{"A":{"main":[[{"node":"B","type":"main","index":0},{"node":"C","type":"main","index":1}]]}}}`
	b := `{"connections":{"A":{"main":[[{"node":"C","type":"main","index":1},{"node":"B","type":"main","index":0}]]}}}`
	if r := compare(t, a, b, nil); r.Equal || !r.Summary.ConnectionsChanged {
		t.Fatal("connection order change was ignored")
	}
}

func TestSelectedFieldsAndOptInData(t *testing.T) {
	a := `{"name":"A","pinData":{"x":[1]},"staticData":{"x":1},"settings":{"x":1}}`
	b := `{"name":"B","pinData":{"x":[2]},"staticData":{"x":2},"settings":{"x":1}}`
	if r := compare(t, a, b, []string{"settings"}); !r.Equal {
		t.Fatal("--only did not restrict comparison")
	}
	for _, field := range []string{"pinData", "staticData"} {
		r := compare(t, a, b, []string{field})
		if r.Equal || !strings.HasPrefix(r.Changes[0].Path, "/"+field+"/") {
			t.Fatalf("opt-in field %s ignored: %+v", field, r)
		}
	}
	fields, err := Fields(nil, true, true)
	if err != nil || !strings.Contains(strings.Join(fields, ","), "pinData") || !strings.Contains(strings.Join(fields, ","), "staticData") {
		t.Fatalf("inclusion flags: %v, %v", fields, err)
	}
	for _, fields := range [][]string{{"id"}, {""}, {}} {
		if _, err := Fields(fields, false, false); err == nil {
			t.Fatalf("accepted invalid fields %v", fields)
		}
	}
}

func TestRejectsAmbiguousAndMalformedNodes(t *testing.T) {
	fields, _ := Fields(nil, false, false)
	for _, nodes := range []string{
		`{}`, `[null]`, `[{"name":""}]`,
		`[{"name":"A"},{"name":"A"}]`,
		`[{"id":"x","name":"A"},{"id":"x","name":"B"}]`,
	} {
		_, err := Compare(workflow(t, `{"nodes":`+nodes+`}`), workflow(t, `{}`), fields)
		if err == nil {
			t.Errorf("accepted malformed nodes %s", nodes)
		}
	}
}

func TestComparisonDoesNotMutateInputsAndIsDeterministic(t *testing.T) {
	a := workflow(t, `{"nodes":[{"id":"a","name":"A","parameters":{"z":1,"a":2}}]}`)
	b := workflow(t, `{"nodes":[{"id":"b","name":"A","parameters":{"a":3,"z":2}}]}`)
	beforeA, _ := json.Marshal(a)
	beforeB, _ := json.Marshal(b)
	fields, _ := Fields(nil, false, false)
	var previous string
	for range 10 {
		r, err := Compare(a, b, fields)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(r)
		if previous != "" && string(encoded) != previous {
			t.Fatal("nondeterministic output")
		}
		previous = string(encoded)
	}
	afterA, _ := json.Marshal(a)
	afterB, _ := json.Marshal(b)
	if string(beforeA) != string(afterA) || string(beforeB) != string(afterB) {
		t.Fatal("comparison mutated an input")
	}
}

func TestUnified(t *testing.T) {
	a := []byte("{\n  \"a\": 1\n}")
	b := []byte("{\n  \"a\": 2\n}")
	want := "--- source\n+++ target\n@@ -1,3 +1,3 @@\n {\n-  \"a\": 1\n+  \"a\": 2\n }\n"
	if got := Unified(a, b, "source", "target"); got != want {
		t.Fatalf("diff = %q, want %q", got, want)
	}
	if got := Unified(a, a, "source", "target"); got != "" {
		t.Fatalf("equal documents produced a diff: %s", got)
	}
}
