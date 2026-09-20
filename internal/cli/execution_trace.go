package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// executionFailedError is returned by `trace --fail-on-error` when the traced
// run ended in error or crashed. The trace was printed; only the exit code
// changes, and Run reports it without the usage hint a real error gets.
type executionFailedError struct {
	id, status string
}

func (e *executionFailedError) Error() string {
	return fmt.Sprintf("execution %s finished with status %s", e.id, e.status)
}

type executionTraceFlags struct {
	instance    instanceFlags
	data        executionDataFlags
	workflowID  string
	follow      bool
	interval    time.Duration
	verbose     bool
	failOnError bool
	noPreflight bool
	output      string
}

func newExecutionTraceCommand(opts Options) *cobra.Command {
	var f executionTraceFlags
	cmd := &cobra.Command{
		Use:   "trace [execution-id]",
		Short: "Show one execution node by node, optionally as it runs",
		Long: "Print the per-node trace of one execution: each node run in order with its\n" +
			"status, duration and error class and message. This is n8n's stored run record,\n" +
			"not console output. It never writes to the instance and always requests the\n" +
			"run's detailed data, so it needs execution:read; --workflow-id also needs\n" +
			"execution:list, and workflow names need workflow:read (the ID is shown\n" +
			"otherwise).\n\n" +
			"Pass an execution ID, or --workflow-id to trace that workflow's most recent run.\n" +
			"With --follow the command polls every --interval (default 2s; the API offers no\n" +
			"push) and prints node runs as they appear, exiting when the run reaches a\n" +
			"terminal status (success, error, crashed, canceled), so command termination is a\n" +
			"usable 'the run finished' signal. '--workflow-id ID --follow' attaches to the\n" +
			"active run of that workflow, or waits for the next one to start: start the\n" +
			"command, then trigger the workflow. Active runs are found through a\n" +
			"status=running query, because the API's unfiltered list omits them. Node\n" +
			"markers are [ok], [fail], [wait], [run] and [stop].\n\n" +
			"Node data may only appear when the run ends. n8n stores per-node data during a\n" +
			"run only when the workflow's 'Save execution progress' setting is on; otherwise\n" +
			"a followed run shows nothing until it finishes and the trace then prints in one\n" +
			"block. --follow checks that setting first and warns on stderr; the check is\n" +
			"advisory (--no-preflight skips it). Oversized data the server omits is reported\n" +
			"as such; pass --ignore-data-size-limit to fetch it anyway.\n\n" +
			"Exit codes: 0 once the trace is printed, whatever the run's outcome, and also on\n" +
			"Ctrl+C, including while waiting for a run. 1 for transport, credential,\n" +
			"not-found and validation errors, or when --fail-on-error is set and the run\n" +
			"ended error or crashed. --output json prints one indented object; with\n" +
			"--follow it streams NDJSON lines of type header, node and result, each with\n" +
			"schemaVersion. Warnings stay on stderr so stdout is pure data.",
		Example: "  n8n execution trace EXECUTION_ID\n" +
			"  n8n execution trace EXECUTION_ID --verbose\n" +
			"  n8n execution trace --workflow-id WORKFLOW_ID\n" +
			"  n8n execution trace --workflow-id WORKFLOW_ID --follow\n" +
			"  n8n execution trace EXECUTION_ID --output json\n" +
			"  n8n execution trace --workflow-id WORKFLOW_ID --follow --fail-on-error --output json",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			return runExecutionTrace(cmd.Context(), opts, id, f)
		},
	}
	f.instance.register(cmd)
	f.data.registerDetail(cmd)
	cmd.Flags().StringVar(&f.workflowID, "workflow-id", "", "trace this workflow's most recent run, or with --follow its active or next run; cannot be combined with an execution ID")
	cmd.Flags().BoolVar(&f.follow, "follow", false, "poll until the run reaches a terminal status, printing node runs as they appear (default: print once and exit)")
	cmd.Flags().DurationVar(&f.interval, "interval", defaultPollInterval, "time between polls with --follow, e.g. 2s; 1s to 1h (default 2s)")
	cmd.Flags().BoolVar(&f.verbose, "verbose", false, "also print each node run's input/output data as indented JSON (default: status, duration and error only)")
	cmd.Flags().BoolVar(&f.failOnError, "fail-on-error", false, "exit 1 when the traced run ends error or crashed (default: exit 0 whatever the outcome)")
	cmd.Flags().BoolVar(&f.noPreflight, "no-preflight", false, "skip the advisory check of the workflow's 'Save execution progress' setting before --follow (default: check and warn)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is one indented object, or NDJSON lines with --follow)")
	return cmd
}

// executionTracer holds what one trace invocation needs across polls.
type executionTracer struct {
	opts       Options
	resolution config.Resolution
	client     *n8n.Client
	f          executionTraceFlags
	dataOpts   n8n.ExecutionDataOptions
	resolver   *workflowResolver
	// progress is the preflight result; Unknown until the preflight ran.
	progress n8n.ProgressSaving
	// preflighted is true once the progress-saving check ran or was skipped.
	preflighted bool
}

func runExecutionTrace(ctx context.Context, opts Options, id string, f executionTraceFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	switch {
	case id != "" && f.workflowID != "":
		return fmt.Errorf("pass either an execution ID or --workflow-id, not both")
	case id == "" && f.workflowID == "":
		return fmt.Errorf("an execution ID or --workflow-id is required: find IDs with 'n8n execution list'")
	case id != "":
		if err := validateExecutionArgument(id); err != nil {
			return err
		}
	default:
		if strings.TrimSpace(f.workflowID) != f.workflowID || f.workflowID == "" {
			return fmt.Errorf("--workflow-id must not be empty or start or end with whitespace")
		}
	}
	if f.follow {
		if err := validatePollInterval(f.interval); err != nil {
			return err
		}
	}
	f.data.includeData = true // there is no trace without the run data
	dataOpts, err := f.data.options()
	if err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	t := &executionTracer{
		opts: opts, resolution: resolution, client: client, f: f, dataOpts: dataOpts,
		resolver: newWorkflowResolver(client), preflighted: f.noPreflight || !f.follow,
	}
	if !f.follow {
		if id == "" {
			id, err = t.latestExecutionID(ctx)
			if err != nil {
				return err
			}
		}
		execution, err := client.GetExecution(ctx, id, dataOpts)
		if err != nil {
			return ignoreCanceled(ctx, executionAPIError(err, resolution, id, "read"))
		}
		return ignoreCanceled(ctx, t.printOneShot(ctx, execution))
	}
	if f.workflowID != "" {
		t.preflight(ctx, f.workflowID)
		id, err = t.awaitRun(ctx)
		if err != nil || id == "" {
			return ignoreCanceled(ctx, err)
		}
	}
	return ignoreCanceled(ctx, t.follow(ctx, id))
}

// awaitPageSize is how many of a workflow's newest executions each wait poll
// reads: enough to hold an active run alongside the runs that finished since.
const awaitPageSize = 5

// newestExecutions lists the workflow's newest runs, active ones included.
func (t *executionTracer) newestExecutions(ctx context.Context) ([]n8n.Execution, error) {
	executions, err := listExecutionsWithRunning(ctx, t.client, n8n.ListExecutionsOptions{
		ListOptions: n8n.ListOptions{Limit: awaitPageSize}, WorkflowID: t.f.workflowID,
	})
	if err != nil {
		return nil, executionAPIError(err, t.resolution, "", "list")
	}
	return executions, nil
}

// latestExecutionID returns the newest execution of the selected workflow.
func (t *executionTracer) latestExecutionID(ctx context.Context) (string, error) {
	executions, err := t.newestExecutions(ctx)
	if err != nil {
		return "", ignoreCanceled(ctx, err)
	}
	if len(executions) == 0 {
		return "", fmt.Errorf("%s has no executions for workflow %q: trigger the workflow first, or use --follow to wait for the next run", t.resolution.URL, t.f.workflowID)
	}
	return executions[0].ID, nil
}

// firstActive returns the newest unfinished execution, or nil.
func firstActive(executions []n8n.Execution) *n8n.Execution {
	for i := range executions {
		if !executions[i].IsFinished() {
			return &executions[i]
		}
	}
	return nil
}

// awaitRun attaches to the workflow's active run, else waits for the next one
// to start. It returns an empty ID when the wait was interrupted.
func (t *executionTracer) awaitRun(ctx context.Context) (string, error) {
	var (
		found    string
		baseline string // newest ID seen on the first poll; later runs are newer
		checked  bool
		waiting  bool
	)
	wait := func() {
		if !waiting {
			fmt.Fprintf(t.opts.Streams.Err, "Waiting for a run of %s... (Ctrl+C to exit)\n", t.resolver.label(ctx, t.f.workflowID))
			waiting = true
		}
	}
	err := poll(ctx, t.f.interval, t.opts.Streams.Err, func(ctx context.Context) (bool, error) {
		executions, err := t.newestExecutions(ctx)
		if err != nil {
			return false, err
		}
		defer func() { checked = true }()
		if active := firstActive(executions); active != nil && (!checked || baseline == "" || executionIDLess(baseline, active.ID)) {
			found = active.ID // an active run: attach to it
			return true, nil
		}
		if !checked {
			if len(executions) > 0 {
				baseline = executions[0].ID
			}
			wait()
			return false, nil
		}
		// No active run: a fast one may have started and finished between polls.
		if len(executions) > 0 && (baseline == "" || executionIDLess(baseline, executions[0].ID)) {
			found = executions[0].ID
			return true, nil
		}
		return false, nil
	})
	return found, err
}

// preflight reads the workflow's progress-saving setting and warns when a
// followed run will not stream. It is advisory: a failed read is Unknown, and
// nothing here can stop the follow.
func (t *executionTracer) preflight(ctx context.Context, workflowID string) {
	if t.preflighted {
		return
	}
	t.preflighted = true
	w := t.resolver.lookup(ctx, workflowID)
	t.progress = w.SaveExecutionProgress()
	name := t.resolver.name(ctx, workflowID)
	const remedy = "Enable Settings > \"Save execution progress\" in the workflow to stream node by node."
	switch {
	case t.progress == n8n.ProgressSavingOn:
	case t.progress == n8n.ProgressSavingOff:
		fmt.Fprintf(t.opts.Streams.Err, "Warning: workflow %q has saveExecutionProgress disabled, so n8n stores node data only when the run finishes. Waiting for completion; the trace prints in one block.\n%s\n", name, remedy)
	case w == nil:
		fmt.Fprintf(t.opts.Streams.Err, "Warning: could not read workflow %s to check saveExecutionProgress (workflow:read may be missing). If the instance default is disabled, node data appears only when the run finishes and the trace prints in one block.\n%s\n", workflowID, remedy)
	default:
		fmt.Fprintf(t.opts.Streams.Err, "Warning: workflow %q inherits the instance default for saveExecutionProgress, which may be disabled. If so, node data appears only when the run finishes and the trace prints in one block.\n%s\n", name, remedy)
	}
}

// follow polls one execution until it finishes, printing each node run once.
func (t *executionTracer) follow(ctx context.Context, id string) error {
	var (
		printed       = map[string]int{} // node name -> runs already printed
		headerPrinted bool
		final         *n8n.Execution
	)
	err := poll(ctx, t.f.interval, t.opts.Streams.Err, func(ctx context.Context) (bool, error) {
		execution, err := t.client.GetExecution(ctx, id, t.dataOpts)
		if err != nil {
			return false, executionAPIError(err, t.resolution, id, "read")
		}
		data, err := execution.ParseRunData()
		if err != nil {
			return false, err
		}
		if !headerPrinted {
			t.preflight(ctx, execution.WorkflowID)
			if err := t.writeHeader(ctx, execution, true); err != nil {
				return true, err
			}
			headerPrinted = true
		}
		seen := map[string]int{}
		for _, entry := range data.NodeRuns() {
			index := seen[entry.Node]
			seen[entry.Node]++
			if index < printed[entry.Node] {
				continue
			}
			printed[entry.Node] = index + 1
			if err := t.writeNode(execution.ID, entry, index); err != nil {
				return true, err
			}
		}
		if !execution.IsFinished() {
			return false, nil
		}
		final = execution
		return true, t.writeResult(execution, data, len(printed) > 0)
	})
	if err != nil {
		return err
	}
	return t.failOnError(final)
}

// printOneShot renders a finished (or not) execution fetched once.
func (t *executionTracer) printOneShot(ctx context.Context, execution *n8n.Execution) error {
	data, err := execution.ParseRunData()
	if err != nil {
		return err
	}
	if t.f.output == outputJSON {
		if err := writeJSON(t.opts.Streams.Out, t.document(ctx, execution, data)); err != nil {
			return err
		}
		return t.failOnError(execution)
	}
	if err := t.writeHeader(ctx, execution, false); err != nil {
		return err
	}
	out := t.opts.Streams.Out
	runs := data.NodeRuns()
	if len(runs) == 0 {
		fmt.Fprintln(out, traceDataNotice(execution, data))
	}
	for i, entry := range runs {
		index := 0
		for _, earlier := range runs[:i] {
			if earlier.Node == entry.Node {
				index++
			}
		}
		if err := t.writeNode(execution.ID, entry, index); err != nil {
			return err
		}
	}
	if runErr := data.FirstError(); runErr != nil && data != nil && data.ResultData.Error != nil && !anyNodeFailed(runs) {
		fmt.Fprintf(out, "\nRun error: %s\nType:      %s\n", singleLineText(runErr.Text()), emptyDash(runErr.Name))
	}
	return t.failOnError(execution)
}

// failOnError turns a failed run into exit 1 when asked to.
func (t *executionTracer) failOnError(execution *n8n.Execution) error {
	if !t.f.failOnError || execution == nil {
		return nil
	}
	if execution.Status == "error" || execution.Status == "crashed" {
		return &executionFailedError{id: execution.ID, status: execution.Status}
	}
	return nil
}

func anyNodeFailed(runs []n8n.NodeRunEntry) bool {
	for _, entry := range runs {
		if entry.Run.Error != nil || entry.Run.ExecutionStatus == "error" {
			return true
		}
	}
	return false
}

// traceDataNotice explains an empty trace: which of the three reasons applies.
func traceDataNotice(execution *n8n.Execution, data *n8n.ExecutionData) string {
	switch {
	case execution.DataTooLargeToDisplay:
		return fmt.Sprintf("Execution %s's detailed data exceeds the instance display-size limit and was omitted; rerun with --ignore-data-size-limit to fetch it.", execution.ID)
	case data == nil && !execution.IsFinished():
		return fmt.Sprintf("Execution %s is still %s and has no stored node data yet: n8n saves it when the run ends unless the workflow saves execution progress. Use --follow to wait for it.", execution.ID, emptyDash(execution.Status))
	case data == nil:
		return fmt.Sprintf("Execution %s has no stored run data: the workflow's execution-data settings may not keep data for %s runs.", execution.ID, emptyDash(execution.Status))
	case !execution.IsFinished():
		return fmt.Sprintf("Execution %s is %s and no node has finished yet.", execution.ID, emptyDash(execution.Status))
	default:
		return fmt.Sprintf("Execution %s finished %s without running any node.", execution.ID, emptyDash(execution.Status))
	}
}

const traceRule = "================================================================================"

// writeHeader prints the execution summary above the trace. streaming omits
// the duration and stop time, which the result line supplies once known.
func (t *executionTracer) writeHeader(ctx context.Context, execution *n8n.Execution, streaming bool) error {
	out := t.opts.Streams.Out
	if t.f.output == outputJSON {
		return writeNDJSON(out, traceHeaderRecord{
			SchemaVersion: pollSchemaVersion, Type: "header", ExecutionID: execution.ID,
			WorkflowID: execution.WorkflowID, WorkflowName: t.resolver.resolvedName(ctx, execution.WorkflowID),
			Status: execution.Status, Mode: execution.Mode, StartedAt: execution.StartedAt,
			SaveExecutionProgress: t.progress.String(),
		})
	}
	var b strings.Builder
	fmt.Fprintln(&b, traceRule)
	fmt.Fprintf(&b, "Workflow:  %s\n", t.resolver.label(ctx, execution.WorkflowID))
	fmt.Fprintf(&b, "Execution: %s [%s] | Mode: %s", emptyDash(execution.ID), emptyDash(execution.Status), emptyDash(execution.Mode))
	if !streaming {
		fmt.Fprintf(&b, " | Duration: %s", formatDurationMs(executionDurationMs(execution)))
	}
	fmt.Fprintf(&b, "\nStarted:   %s\n", optionalText(execution.StartedAt))
	if !streaming {
		fmt.Fprintf(&b, "Stopped:   %s\n", optionalText(executionStoppedAt(execution)))
	}
	fmt.Fprintln(&b, traceRule)
	fmt.Fprintln(&b, "NODE EXECUTION TRACE:")
	_, err := io.WriteString(out, b.String())
	return err
}

// writeNode prints one node run. index is which run of that node this is.
func (t *executionTracer) writeNode(executionID string, entry n8n.NodeRunEntry, index int) error {
	out := t.opts.Streams.Out
	if t.f.output == outputJSON {
		return writeNDJSON(out, t.nodeRecord(executionID, entry, index))
	}
	var b strings.Builder
	name := entry.Node
	if index > 0 {
		name = fmt.Sprintf("%s (run %d)", name, index+1)
	}
	fmt.Fprintf(&b, " %-6s %-24s (%dms)\n", nodeStatusMarker(entry.Run.ExecutionStatus), name, entry.Run.ExecutionTime)
	if entry.Run.Error != nil {
		fmt.Fprintf(&b, "        Error: %s\n", singleLineText(entry.Run.Error.Text()))
		fmt.Fprintf(&b, "        Type:  %s\n", emptyDash(entry.Run.Error.Name))
	}
	if t.f.verbose && len(entry.Run.Data) > 0 {
		var data bytes.Buffer
		if err := json.Indent(&data, entry.Run.Data, "        ", "  "); err == nil {
			fmt.Fprintf(&b, "        Data:  %s\n", data.String())
		} else {
			fmt.Fprintf(&b, "        Data:  %s\n", string(entry.Run.Data))
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// writeResult closes a followed trace once the run has finished.
func (t *executionTracer) writeResult(execution *n8n.Execution, data *n8n.ExecutionData, nodesPrinted bool) error {
	out := t.opts.Streams.Out
	runErr := data.FirstError()
	if t.f.output == outputJSON {
		return writeNDJSON(out, traceResultRecord{
			SchemaVersion: pollSchemaVersion, Type: "result", ExecutionID: execution.ID,
			Status: execution.Status, DurationMs: executionDurationMs(execution), StoppedAt: executionStoppedAt(execution),
			Error: toRunErrorJSON(runErr),
		})
	}
	var b strings.Builder
	if !nodesPrinted {
		fmt.Fprintln(&b, traceDataNotice(execution, data))
	}
	fmt.Fprintln(&b, strings.Repeat("-", len(traceRule)))
	fmt.Fprintf(&b, "Result:    %s | Duration: %s | Stopped: %s\n", emptyDash(execution.Status), formatDurationMs(executionDurationMs(execution)), optionalText(executionStoppedAt(execution)))
	if runErr != nil && (data.ResultData.Error != nil && !anyNodeFailed(data.NodeRuns()) || !nodesPrinted) {
		fmt.Fprintf(&b, "Error:     %s\nType:      %s\n", singleLineText(runErr.Text()), emptyDash(runErr.Name))
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// nodeStatusMarker is the ASCII status column of a node line, at most six
// characters so the columns stay aligned. Windows consoles are supported, so
// no glyphs.
func nodeStatusMarker(status string) string {
	switch status {
	case "success":
		return "[ok]"
	case "error", "crashed":
		return "[fail]"
	case "waiting":
		return "[wait]"
	case "running", "new":
		return "[run]"
	case "canceled":
		return "[stop]"
	case "":
		return "[?]"
	default:
		return "[" + status[:min(4, len(status))] + "]"
	}
}

// JSON shapes shared by the one-shot document and the NDJSON stream.

type traceHeaderRecord struct {
	SchemaVersion         int     `json:"schemaVersion"`
	Type                  string  `json:"type"`
	ExecutionID           string  `json:"executionId"`
	WorkflowID            string  `json:"workflowId"`
	WorkflowName          *string `json:"workflowName"`
	Status                string  `json:"status"`
	Mode                  string  `json:"mode"`
	StartedAt             *string `json:"startedAt"`
	SaveExecutionProgress string  `json:"saveExecutionProgress"`
}

type traceNodeRecord struct {
	SchemaVersion int             `json:"schemaVersion,omitempty"`
	Type          string          `json:"type,omitempty"`
	ExecutionID   string          `json:"executionId,omitempty"`
	Name          string          `json:"name"`
	Index         int             `json:"index"`
	Run           int             `json:"run"`
	Status        string          `json:"status"`
	DurationMs    int64           `json:"durationMs"`
	StartedAt     string          `json:"startedAt"`
	Error         *runErrorJSON   `json:"error"`
	Data          json.RawMessage `json:"data,omitempty"`
}

type traceResultRecord struct {
	SchemaVersion int           `json:"schemaVersion"`
	Type          string        `json:"type"`
	ExecutionID   string        `json:"executionId"`
	Status        string        `json:"status"`
	DurationMs    *int64        `json:"durationMs"`
	StoppedAt     *string       `json:"stoppedAt"`
	Error         *runErrorJSON `json:"error"`
}

// traceDocument is the one-shot `--output json` object.
type traceDocument struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Execution     traceExecutionDocument `json:"execution"`
	Data          traceDataDocument      `json:"data"`
	Nodes         []traceNodeRecord      `json:"nodes"`
	Error         *runErrorJSON          `json:"error"`
}

type traceExecutionDocument struct {
	ID           string  `json:"id"`
	WorkflowID   string  `json:"workflowId"`
	WorkflowName *string `json:"workflowName"`
	Status       string  `json:"status"`
	Finished     bool    `json:"finished"`
	Mode         string  `json:"mode"`
	StartedAt    *string `json:"startedAt"`
	StoppedAt    *string `json:"stoppedAt"`
	DurationMs   *int64  `json:"durationMs"`
}

type traceDataDocument struct {
	Included bool   `json:"included"`
	TooLarge bool   `json:"tooLarge"`
	Notice   string `json:"notice,omitempty"`
}

func (t *executionTracer) nodeRecord(executionID string, entry n8n.NodeRunEntry, index int) traceNodeRecord {
	rec := traceNodeRecord{
		Name: entry.Node, Index: entry.Run.ExecutionIndex, Run: index,
		Status: entry.Run.ExecutionStatus, DurationMs: entry.Run.ExecutionTime,
		Error: toRunErrorJSON(entry.Run.Error),
	}
	if entry.Run.StartTime > 0 {
		rec.StartedAt = time.UnixMilli(entry.Run.StartTime).UTC().Format("2006-01-02T15:04:05.000Z")
	}
	if t.f.follow {
		rec.SchemaVersion, rec.Type, rec.ExecutionID = pollSchemaVersion, "node", executionID
	}
	if t.f.verbose && len(entry.Run.Data) > 0 {
		rec.Data = entry.Run.Data
	}
	return rec
}

func (t *executionTracer) document(ctx context.Context, execution *n8n.Execution, data *n8n.ExecutionData) traceDocument {
	doc := traceDocument{
		SchemaVersion: pollSchemaVersion,
		Execution: traceExecutionDocument{
			ID: execution.ID, WorkflowID: execution.WorkflowID,
			WorkflowName: t.resolver.resolvedName(ctx, execution.WorkflowID),
			Status:       execution.Status, Finished: execution.IsFinished(), Mode: execution.Mode,
			StartedAt: execution.StartedAt, StoppedAt: executionStoppedAt(execution),
			DurationMs: executionDurationMs(execution),
		},
		Data:  traceDataDocument{Included: data != nil, TooLarge: execution.DataTooLargeToDisplay},
		Nodes: []traceNodeRecord{},
		Error: toRunErrorJSON(data.FirstError()),
	}
	runs := data.NodeRuns()
	if len(runs) == 0 {
		doc.Data.Notice = traceDataNotice(execution, data)
	}
	seen := map[string]int{}
	for _, entry := range runs {
		doc.Nodes = append(doc.Nodes, t.nodeRecord(execution.ID, entry, seen[entry.Node]))
		seen[entry.Node]++
	}
	return doc
}
