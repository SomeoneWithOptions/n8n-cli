package n8n

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
)

// ExecutionData is the detailed run record an execution carries when it was
// read with includeData. The OpenAPI document leaves it untyped, so only the
// fields the CLI renders are named; everything else is ignored on decode.
type ExecutionData struct {
	Version    int        `json:"version,omitempty"`
	ResultData ResultData `json:"resultData"`
}

// ResultData holds the per-node run records and the outcome of the whole run.
type ResultData struct {
	// RunData maps node name to that node's runs, in the order they happened.
	RunData          map[string][]NodeRun `json:"runData"`
	LastNodeExecuted string               `json:"lastNodeExecuted,omitempty"`
	// Error is present only when the run as a whole failed.
	Error *RunError `json:"error,omitempty"`
}

// NodeRun is one execution of one node.
type NodeRun struct {
	StartTime       int64           `json:"startTime"`      // epoch milliseconds
	ExecutionIndex  int             `json:"executionIndex"` // order across the whole run
	ExecutionTime   int64           `json:"executionTime"`  // milliseconds
	ExecutionStatus string          `json:"executionStatus"`
	Error           *RunError       `json:"error,omitempty"`
	Data            json.RawMessage `json:"data,omitempty"`
	Source          json.RawMessage `json:"source,omitempty"`
	Hints           json.RawMessage `json:"hints,omitempty"`
}

// RunError is the shape n8n uses for both node errors and whole-run errors.
// Name is the error class, for example NodeOperationError or NodeApiError.
// Node is the entire node definition and can be large, so it stays raw.
type RunError struct {
	Name        string          `json:"name"`
	Message     string          `json:"message"`
	Description string          `json:"description,omitempty"`
	Messages    []string        `json:"messages,omitempty"`
	Stack       string          `json:"stack,omitempty"`
	Node        json.RawMessage `json:"node,omitempty"`
	Context     json.RawMessage `json:"context,omitempty"`
}

// Text is the most useful one-line description of the error: the message,
// else the first of the messages, else the description, else the class name.
func (e *RunError) Text() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Message != "":
		return e.Message
	case len(e.Messages) > 0 && e.Messages[0] != "":
		return e.Messages[0]
	case e.Description != "":
		return e.Description
	default:
		return e.Name
	}
}

// NodeRunEntry is one node run flattened out of the RunData map.
type NodeRunEntry struct {
	Node string
	Run  NodeRun
}

// ParseRunData decodes the detailed data of an execution. It returns nil, nil
// when the execution carries no data, which is the normal case without
// includeData or when the server omitted oversized data. Unknown fields are
// ignored and absent fields are zero; only a payload that is not the expected
// object shape is an error.
func (e *Execution) ParseRunData() (*ExecutionData, error) {
	if e == nil || len(bytes.TrimSpace(e.Data)) == 0 || isJSONNull(e.Data) {
		return nil, nil
	}
	var data ExecutionData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, fmt.Errorf("execution %s: decode run data: %w", e.ID, err)
	}
	return &data, nil
}

// NodeRuns flattens RunData into execution order: by ExecutionIndex, then
// StartTime, then node name. Millisecond start times tie on fast nodes, which
// is why ExecutionIndex leads. A nil receiver or map yields an empty slice.
func (d *ExecutionData) NodeRuns() []NodeRunEntry {
	if d == nil || len(d.ResultData.RunData) == 0 {
		return []NodeRunEntry{}
	}
	entries := make([]NodeRunEntry, 0, len(d.ResultData.RunData))
	for node, runs := range d.ResultData.RunData {
		for _, run := range runs {
			entries = append(entries, NodeRunEntry{Node: node, Run: run})
		}
	}
	slices.SortFunc(entries, func(a, b NodeRunEntry) int {
		return cmp.Or(
			cmp.Compare(a.Run.ExecutionIndex, b.Run.ExecutionIndex),
			cmp.Compare(a.Run.StartTime, b.Run.StartTime),
			cmp.Compare(a.Node, b.Node),
		)
	})
	return entries
}

// NodesExecuted is the number of distinct nodes that ran at least once.
func (d *ExecutionData) NodesExecuted() int {
	if d == nil {
		return 0
	}
	n := 0
	for _, runs := range d.ResultData.RunData {
		if len(runs) > 0 {
			n++
		}
	}
	return n
}

// FirstError is the error that explains the run: the whole-run error when
// present, else the error of the earliest failed node run. Nil when none.
func (d *ExecutionData) FirstError() *RunError {
	if d == nil {
		return nil
	}
	if d.ResultData.Error != nil {
		return d.ResultData.Error
	}
	for _, entry := range d.NodeRuns() {
		if entry.Run.Error != nil {
			return entry.Run.Error
		}
	}
	return nil
}

// WorkflowNodeCount is the number of nodes in the workflow definition stored
// with the execution, or zero when the definition was not included.
func (e *Execution) WorkflowNodeCount() int {
	if e == nil || len(e.WorkflowData) == 0 {
		return 0
	}
	var workflow struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(e.WorkflowData, &workflow); err != nil {
		return 0
	}
	return len(workflow.Nodes)
}

// Terminal execution statuses: once an execution reports one of these its
// record no longer changes, so a poller can stop and a cache can keep it.
var terminalExecutionStatuses = []string{"success", "error", "crashed", "canceled"}

// IsTerminalExecutionStatus reports whether status marks a finished run.
func IsTerminalExecutionStatus(status string) bool {
	return slices.Contains(terminalExecutionStatuses, status)
}

// IsFinished reports whether the execution has reached a terminal state, by
// its status or by the finished flag the API sets on completed runs.
func (e *Execution) IsFinished() bool {
	return e != nil && (e.Finished || IsTerminalExecutionStatus(e.Status))
}
