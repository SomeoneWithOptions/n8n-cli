package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// watchSeenCap bounds the ID-to-status memory of a long-running watch. Pages
// hold at most 250 rows, so evicting the oldest IDs beyond this never drops an
// execution that can still appear on page one.
const watchSeenCap = 1000

// clearScreen is the escape sequence that homes the cursor and clears the
// terminal before a dashboard redraw. It is only ever sent to a terminal.
const clearScreen = "\033[H\033[2J"

type executionWatchFlags struct {
	instance   instanceFlags
	workflowID string
	projectID  string
	status     string
	limit      int
	interval   time.Duration
	follow     bool
	nodes      bool
	output     string
}

func newExecutionWatchCommand(opts Options) *cobra.Command {
	var f executionWatchFlags
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Show executions live as they start and finish",
		Long: "Poll the execution list and show it live. The n8n API offers no push channel, so\n" +
			"this refetches the newest executions every --interval (default 2s) and shows\n" +
			"what changed. It never writes to the instance and needs only execution:list.\n\n" +
			"By default the terminal is redrawn each tick as a dashboard of the newest --limit\n" +
			"executions. When stdout is not a terminal no escape codes are written: each\n" +
			"tick prints a timestamp line and the table. --follow instead appends one line\n" +
			"per execution the first time it is seen and again whenever its status changes,\n" +
			"which suits logs and pipes. Filter with --workflow-id, --project-id and\n" +
			"--status exactly as 'n8n execution list' does; without --workflow-id a WORKFLOW\n" +
			"column shows the name, or the ID when the credential lacks workflow:read. The\n" +
			"API's unfiltered list omits running executions, so without --status each tick\n" +
			"also asks for status=running and merges the result (two requests per tick);\n" +
			"pass --status running to see only active runs. DURATION is shown once a run\n" +
			"has finished.\n\n" +
			"--nodes adds NODES (executed/total) and ERROR columns. They need each run's\n" +
			"detailed data, so every new or changed execution costs one extra request;\n" +
			"finished runs are fetched once and cached. Oversized data the server omits\n" +
			"shows as '-'.\n\n" +
			"--output json streams NDJSON: one compact object per line, one line per\n" +
			"execution per change, each carrying schemaVersion, type \"execution\", id,\n" +
			"workflowId, workflowName, status, mode, startedAt, stoppedAt, durationMs, nodes\n" +
			"and error. Diagnostics go to stderr. The command runs until interrupted:\n" +
			"Ctrl+C (or SIGTERM) exits 0. Exit 1 is reserved for transport, credential and\n" +
			"validation errors; a poll that fails mid-run is reported on stderr and retried.\n" +
			"Use 'n8n execution trace' to inspect one run node by node.",
		Example: "  n8n execution watch\n" +
			"  n8n execution watch --workflow-id WORKFLOW_ID --nodes\n" +
			"  n8n execution watch --status error --interval 10s\n" +
			"  n8n execution watch --follow\n" +
			"  n8n execution watch --workflow-id WORKFLOW_ID --output json | jq -c 'select(.status==\"error\")'",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExecutionWatch(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.workflowID, "workflow-id", "", "only executions of this workflow, from 'n8n workflow list' (default: every workflow)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "only executions in this project, from 'n8n project list' (default: every accessible project)")
	cmd.Flags().StringVar(&f.status, "status", "", "execution status: canceled, crashed, error, new, running, success, unknown, or waiting (default: every status)")
	cmd.Flags().IntVar(&f.limit, "limit", 10, "rows in the dashboard, or the initial backfill with --follow, 1 to 250 (default 10)")
	cmd.Flags().DurationVar(&f.interval, "interval", defaultPollInterval, "time between polls, e.g. 2s, 500ms is rejected; 1s to 1h (default 2s)")
	cmd.Flags().BoolVar(&f.follow, "follow", false, "append a line per new or changed execution instead of redrawing the table (default: redraw)")
	cmd.Flags().BoolVar(&f.nodes, "nodes", false, "add NODES and ERROR columns from each run's detailed data; one extra request per new or changed execution (default: metadata only)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is NDJSON, one execution object per line per change)")
	return cmd
}

// executionWatcher is the state of one watch process: what it has already
// shown, and the detailed data it has fetched for finished runs.
type executionWatcher struct {
	opts       Options
	resolution config.Resolution
	client     *n8n.Client
	f          executionWatchFlags
	listOpts   n8n.ListExecutionsOptions
	resolver   *workflowResolver
	terminal   bool
	// stream is true when output is append-only: --follow or JSON.
	stream bool
	// seen maps execution ID to the status last shown for it.
	seen map[string]string
	// details caches detailed data by execution ID; finished runs never change.
	details map[string]executionNodeDetail
	// firstTick is true until the initial fetch has been rendered.
	firstTick bool
}

// executionNodeDetail is what --nodes adds to a row.
type executionNodeDetail struct {
	total, executed int
	err             *n8n.RunError
	// available is false when detailed data could not be fetched or was omitted.
	available bool
}

func runExecutionWatch(ctx context.Context, opts Options, f executionWatchFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.limit < 1 || f.limit > 250 {
		return fmt.Errorf("--limit must be between 1 and 250, got %d", f.limit)
	}
	if err := validatePollInterval(f.interval); err != nil {
		return err
	}
	listOpts := n8n.ListExecutionsOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit},
		Status:      f.status,
		WorkflowID:  f.workflowID,
		ProjectID:   f.projectID,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	w := &executionWatcher{
		opts: opts, resolution: resolution, client: client, f: f, listOpts: listOpts,
		resolver:  newWorkflowResolver(client),
		terminal:  isTerminal(opts.Streams.Out),
		stream:    f.follow || f.output == outputJSON,
		seen:      map[string]string{},
		details:   map[string]executionNodeDetail{},
		firstTick: true,
	}
	if w.stream && f.output == outputText {
		// The stream is append-only, so the context lines go to stderr once
		// and stdout carries only the column header and rows.
		w.writeHeader(ctx, opts.Streams.Err)
		fmt.Fprintln(opts.Streams.Out, w.formatStreamRow(w.columns()))
	}
	return ignoreCanceled(ctx, poll(ctx, f.interval, opts.Streams.Err, w.tick))
}

// tick is one poll: fetch page one, then render or diff it.
func (w *executionWatcher) tick(ctx context.Context) (bool, error) {
	executions, err := listExecutionsWithRunning(ctx, w.client, w.listOpts)
	if err != nil {
		return false, apiError(err, w.resolution, executionResource)
	}
	if w.stream {
		err = w.emitChanges(ctx, executions)
	} else {
		err = w.redraw(ctx, executions)
	}
	w.firstTick = false
	return false, err
}

// redraw renders the whole page: a cleared terminal, or a timestamped block
// when stdout is a pipe or file.
func (w *executionWatcher) redraw(ctx context.Context, executions []n8n.Execution) error {
	out := w.opts.Streams.Out
	if w.terminal {
		if _, err := io.WriteString(out, clearScreen); err != nil {
			return err
		}
		w.writeHeader(ctx, out)
	} else {
		if w.firstTick {
			w.writeHeader(ctx, out)
		}
		fmt.Fprintf(out, "%s\n", time.Now().UTC().Format(time.RFC3339))
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(w.columns(), "\t"))
	for _, execution := range executions {
		fmt.Fprintln(tw, strings.Join(w.row(ctx, &execution), "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(executions) == 0 {
		fmt.Fprintln(out, "No executions match the filters yet.")
	}
	if !w.terminal {
		fmt.Fprintln(out)
	}
	return nil
}

// emitChanges prints executions not seen before or whose status changed,
// oldest first so the stream reads chronologically.
func (w *executionWatcher) emitChanges(ctx context.Context, executions []n8n.Execution) error {
	changed := make([]*n8n.Execution, 0, len(executions))
	for i := range executions {
		execution := &executions[i]
		if last, ok := w.seen[execution.ID]; !ok || last != execution.Status {
			changed = append(changed, execution)
		}
	}
	slices.SortFunc(changed, func(a, b *n8n.Execution) int { return compareExecutionIDs(a.ID, b.ID) })
	for _, execution := range changed {
		w.seen[execution.ID] = execution.Status
		var err error
		if w.f.output == outputJSON {
			err = writeNDJSON(w.opts.Streams.Out, w.record(ctx, execution))
		} else {
			_, err = fmt.Fprintln(w.opts.Streams.Out, w.formatStreamRow(w.row(ctx, execution)))
		}
		if err != nil {
			return err
		}
	}
	w.evict()
	return nil
}

// evict drops the oldest remembered IDs once the memory exceeds its cap.
func (w *executionWatcher) evict() {
	if len(w.seen) <= watchSeenCap {
		return
	}
	ids := make([]string, 0, len(w.seen))
	for id := range w.seen {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, compareExecutionIDs)
	for _, id := range ids[:len(ids)-watchSeenCap] {
		delete(w.seen, id)
		delete(w.details, id)
	}
}

// writeHeader prints the context lines above the table.
func (w *executionWatcher) writeHeader(ctx context.Context, out io.Writer) {
	tw := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", w.resolution.URL)
	if w.f.workflowID != "" {
		fmt.Fprintf(tw, "Workflow:\t%s\n", w.resolver.label(ctx, w.f.workflowID))
	}
	if w.f.projectID != "" {
		fmt.Fprintf(tw, "Project:\t%s\n", w.f.projectID)
	}
	if w.f.status != "" {
		fmt.Fprintf(tw, "Status:\t%s\n", w.f.status)
	}
	mode := "dashboard"
	if w.stream {
		mode = "follow"
	}
	fmt.Fprintf(tw, "Watching:\tevery %s, %s (Ctrl+C to exit)\n\n", w.f.interval, mode)
	_ = tw.Flush()
}

// columns are the table headings in row order. WORKFLOW is omitted when one
// workflow is selected, since the header already names it; NODES and ERROR
// exist only with --nodes.
func (w *executionWatcher) columns() []string {
	cols := []string{"ID", "STATUS", "MODE", "STARTED", "DURATION"}
	if w.f.nodes {
		cols = append(cols, "NODES")
	}
	if w.f.workflowID == "" {
		cols = append(cols, "WORKFLOW")
	}
	if w.f.nodes {
		cols = append(cols, "ERROR")
	}
	return cols
}

// streamWidths pad the fixed-width columns of an append-only row, in the same
// order as columns. Only the variable-width trailing columns are unpadded.
var streamWidths = map[string]int{"ID": 6, "STATUS": 8, "MODE": 9, "STARTED": 24, "DURATION": 9, "NODES": 7}

// formatStreamRow lays out one row without a tabwriter, which cannot align
// lines it has already flushed.
func (w *executionWatcher) formatStreamRow(cells []string) string {
	cols := w.columns()
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		if width, ok := streamWidths[cols[i]]; ok && i < len(cells)-1 {
			fmt.Fprintf(&b, "%-*s", width, cell)
		} else {
			b.WriteString(cell)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// row renders one execution in column order.
func (w *executionWatcher) row(ctx context.Context, execution *n8n.Execution) []string {
	cells := []string{
		emptyDash(execution.ID), emptyDash(execution.Status), emptyDash(execution.Mode),
		optionalText(execution.StartedAt), formatDurationMs(executionDurationMs(execution)),
	}
	var detail executionNodeDetail
	if w.f.nodes {
		detail = w.detail(ctx, execution)
		cells = append(cells, detail.nodesCell())
	}
	if w.f.workflowID == "" {
		cells = append(cells, w.resolver.name(ctx, execution.WorkflowID))
	}
	if w.f.nodes {
		cells = append(cells, emptyDash(singleLineText(detail.err.Text())))
	}
	return cells
}

func (d executionNodeDetail) nodesCell() string {
	switch {
	case !d.available:
		return "-"
	case d.total > 0:
		return fmt.Sprintf("%d/%d", d.executed, d.total)
	default:
		return fmt.Sprintf("%d", d.executed)
	}
}

// detail fetches the run data behind the NODES and ERROR columns. A finished
// run's record is immutable, so it is fetched once per process; a running one
// is refetched on every tick it is rendered.
func (w *executionWatcher) detail(ctx context.Context, execution *n8n.Execution) executionNodeDetail {
	if cached, ok := w.details[execution.ID]; ok {
		return cached
	}
	var detail executionNodeDetail
	full, err := w.client.GetExecution(ctx, execution.ID, n8n.ExecutionDataOptions{IncludeData: true})
	if err == nil {
		if data, parseErr := full.ParseRunData(); parseErr == nil && data != nil {
			detail = executionNodeDetail{
				available: true,
				executed:  data.NodesExecuted(),
				err:       data.FirstError(),
				total:     full.WorkflowNodeCount(),
			}
			if detail.total == 0 {
				detail.total = w.resolver.nodeCount(ctx, execution.WorkflowID)
			}
		}
	} else if ctx.Err() != nil {
		return detail
	} else {
		fmt.Fprintf(w.opts.Streams.Err, "Warning: execution %s: detailed data unavailable: %v\n", execution.ID, err)
	}
	if execution.IsFinished() {
		w.details[execution.ID] = detail
	}
	return detail
}

// watchRecord is one NDJSON line of `watch --output json`.
type watchRecord struct {
	SchemaVersion int             `json:"schemaVersion"`
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	WorkflowID    string          `json:"workflowId"`
	WorkflowName  *string         `json:"workflowName"`
	Status        string          `json:"status"`
	Mode          string          `json:"mode"`
	StartedAt     *string         `json:"startedAt"`
	StoppedAt     *string         `json:"stoppedAt"`
	DurationMs    *int64          `json:"durationMs"`
	Nodes         *watchNodesJSON `json:"nodes"`
	Error         *runErrorJSON   `json:"error"`
}

type watchNodesJSON struct {
	Total    int `json:"total"`
	Executed int `json:"executed"`
}

func (w *executionWatcher) record(ctx context.Context, execution *n8n.Execution) watchRecord {
	rec := watchRecord{
		SchemaVersion: pollSchemaVersion, Type: "execution",
		ID: execution.ID, WorkflowID: execution.WorkflowID,
		WorkflowName: w.resolver.resolvedName(ctx, execution.WorkflowID),
		Status:       execution.Status, Mode: execution.Mode,
		StartedAt: execution.StartedAt, StoppedAt: executionStoppedAt(execution),
		DurationMs: executionDurationMs(execution),
	}
	if w.f.nodes {
		if detail := w.detail(ctx, execution); detail.available {
			rec.Nodes = &watchNodesJSON{Total: detail.total, Executed: detail.executed}
			rec.Error = toRunErrorJSON(detail.err)
		}
	}
	return rec
}
