package n8n

import (
	"encoding/json"
	"strings"
	"testing"
)

// executionRunData is a trimmed copy of a real failed run: two nodes tied on
// start time, one node run twice, and a node error with the class name.
const executionRunData = `{
  "version": 1,
  "startData": {},
  "resultData": {
    "runData": {
      "Get Rotation1": [{"startTime":1757975235024,"executionIndex":4,"source":[{"previousNode":"Get Optimizer1"}],"hints":[],
        "executionTime":4442,"executionStatus":"error",
        "error":{"level":"warning","tags":{},"context":{},"functionality":"regular","name":"NodeOperationError",
                 "node":{"parameters":{},"id":"n1","name":"Get Rotation1","type":"n8n-nodes-base.googleSheets"},
                 "messages":["Sheet with name Rotation not found"],"message":"Sheet with name Rotation not found","stack":"..."}}],
      "Slack Trigger1": [{"startTime":1757975234371,"executionIndex":0,"executionTime":0,"executionStatus":"success","data":{"main":[[{"json":{"a":1}}]]}}],
      "Is New Deal Post": [{"startTime":1757975234371,"executionIndex":1,"executionTime":11,"executionStatus":"success"}],
      "Extract Data1": [
        {"startTime":1757975234382,"executionIndex":2,"executionTime":6,"executionStatus":"success"},
        {"startTime":1757975234390,"executionIndex":3,"executionTime":2,"executionStatus":"success"}
      ]
    },
    "lastNodeExecuted": "Get Rotation1",
    "error": {"name":"NodeOperationError","message":"Sheet with name Rotation not found","node":{"name":"Get Rotation1"}},
    "pinData": {}
  },
  "executionData": {"contextData":{}},
  "unknownFutureField": true
}`

func TestParseRunDataOrdersAndDecodes(t *testing.T) {
	execution := &Execution{ID: "163", Data: json.RawMessage(executionRunData), Status: "error",
		WorkflowData: json.RawMessage(`{"nodes":[{"name":"a"},{"name":"b"},{"name":"c"},{"name":"d"},{"name":"e"}]}`)}
	data, err := execution.ParseRunData()
	if err != nil {
		t.Fatalf("ParseRunData: %v", err)
	}
	if data.Version != 1 || data.ResultData.LastNodeExecuted != "Get Rotation1" {
		t.Errorf("decoded = %+v", data)
	}
	var order []string
	for _, entry := range data.NodeRuns() {
		order = append(order, entry.Node)
	}
	want := "Slack Trigger1,Is New Deal Post,Extract Data1,Extract Data1,Get Rotation1"
	if got := strings.Join(order, ","); got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
	if got := data.NodesExecuted(); got != 4 {
		t.Errorf("NodesExecuted = %d, want 4", got)
	}
	if got := execution.WorkflowNodeCount(); got != 5 {
		t.Errorf("WorkflowNodeCount = %d, want 5", got)
	}
	failed := data.NodeRuns()[4].Run
	if failed.ExecutionStatus != "error" || failed.ExecutionTime != 4442 || failed.Error == nil {
		t.Fatalf("failed run = %+v", failed)
	}
	if failed.Error.Name != "NodeOperationError" || failed.Error.Message != "Sheet with name Rotation not found" || len(failed.Error.Messages) != 1 {
		t.Errorf("node error = %+v", failed.Error)
	}
	if len(failed.Error.Node) == 0 {
		t.Error("node error dropped the raw node object")
	}
	if first := data.FirstError(); first == nil || first.Text() != "Sheet with name Rotation not found" {
		t.Errorf("FirstError = %+v", first)
	}
	if !execution.IsFinished() {
		t.Error("an errored execution is finished")
	}
}

func TestParseRunDataToleratesAbsentAndEmpty(t *testing.T) {
	for name, raw := range map[string]string{
		"absent": "", "null": "null", "empty object": "{}",
		"empty runData": `{"resultData":{"runData":{}}}`,
		"null runData":  `{"resultData":{"runData":null}}`,
	} {
		t.Run(name, func(t *testing.T) {
			execution := &Execution{ID: "1", Data: json.RawMessage(raw)}
			data, err := execution.ParseRunData()
			if err != nil {
				t.Fatalf("ParseRunData: %v", err)
			}
			if raw == "" || raw == "null" {
				if data != nil {
					t.Fatalf("data = %+v, want nil for no data", data)
				}
			} else if data == nil {
				t.Fatal("data = nil, want a decoded (empty) record")
			}
			if runs := data.NodeRuns(); len(runs) != 0 {
				t.Errorf("NodeRuns = %v, want empty", runs)
			}
			if data.FirstError() != nil || data.NodesExecuted() != 0 {
				t.Error("empty data reported an error or executed nodes")
			}
		})
	}
	if (*Execution)(nil).WorkflowNodeCount() != 0 || (&Execution{WorkflowData: json.RawMessage(`[]`)}).WorkflowNodeCount() != 0 {
		t.Error("unreadable workflow data must count zero nodes")
	}
}

func TestParseRunDataMalformedNamesExecution(t *testing.T) {
	execution := &Execution{ID: "163", Data: json.RawMessage(`{"resultData":{"runData":[]}}`)}
	_, err := execution.ParseRunData()
	if err == nil || !strings.Contains(err.Error(), "execution 163") {
		t.Errorf("error = %v, want it to name the execution", err)
	}
}

func TestRunErrorText(t *testing.T) {
	for name, tc := range map[string]struct {
		err  *RunError
		want string
	}{
		"nil":         {nil, ""},
		"message":     {&RunError{Name: "X", Message: "m", Messages: []string{"n"}}, "m"},
		"messages":    {&RunError{Name: "X", Messages: []string{"n"}}, "n"},
		"description": {&RunError{Name: "X", Description: "d"}, "d"},
		"name only":   {&RunError{Name: "X"}, "X"},
	} {
		if got := tc.err.Text(); got != tc.want {
			t.Errorf("%s: Text() = %q, want %q", name, got, tc.want)
		}
	}
}

func TestTerminalStatuses(t *testing.T) {
	for status, want := range map[string]bool{
		"success": true, "error": true, "crashed": true, "canceled": true,
		"running": false, "waiting": false, "new": false, "unknown": false, "": false,
	} {
		if got := IsTerminalExecutionStatus(status); got != want {
			t.Errorf("IsTerminalExecutionStatus(%q) = %t, want %t", status, got, want)
		}
	}
	if !(&Execution{Finished: true, Status: "waiting"}).IsFinished() {
		t.Error("finished flag must count as finished")
	}
	if (*Execution)(nil).IsFinished() {
		t.Error("nil execution is not finished")
	}
}
