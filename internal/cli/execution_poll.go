package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// Polling bounds shared by watch and trace --follow. The API offers no push
// channel, so both commands refetch on an interval; below one second the poll
// itself becomes load on the instance, above one hour the view is stale enough
// to be useless.
const (
	defaultPollInterval = 2 * time.Second
	minPollInterval     = time.Second
	maxPollInterval     = time.Hour
)

// pollSchemaVersion is the schemaVersion carried by every NDJSON record the
// streaming commands emit, so a consumer can detect a future shape change.
const pollSchemaVersion = 1

// tick returns a channel that fires every d, and a stop function. It is a
// variable so tests can drive the poll loop without real time passing.
var tick = func(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// validatePollInterval rejects an interval outside the supported bounds,
// naming the bound in the message so the fix is in the error.
func validatePollInterval(d time.Duration) error {
	if d < minPollInterval {
		return fmt.Errorf("--interval must be at least %s, got %s: polling faster only loads the instance", minPollInterval, d)
	}
	if d > maxPollInterval {
		return fmt.Errorf("--interval must be at most %s, got %s", maxPollInterval, d)
	}
	return nil
}

// pollStep is one iteration of a poll loop. It returns done when the loop
// should stop successfully, or an error. A transient error keeps the loop
// running; see [poll].
type pollStep func(ctx context.Context) (done bool, err error)

// poll runs step immediately, then once per interval, until step reports done,
// the context ends, or step fails in a way that cannot recover.
//
// The first step must succeed: an auth, validation or not-found error on the
// initial fetch is a real error and ends the command with exit 1. Later
// failures are reported on stderr and polling continues, which keeps a watch
// alive across a network blip, except for 401, 403 and 404, which mean the
// credential or the target itself went away. Cancellation returns nil: Ctrl+C
// is the designed way to leave a watch, and exit 0 keeps set -e scripts quiet.
func poll(ctx context.Context, interval time.Duration, errOut io.Writer, step pollStep) error {
	first := true
	run := func() (bool, error) {
		done, err := step(ctx)
		if ctx.Err() != nil {
			return true, nil
		}
		if err == nil {
			first = false
			return done, nil
		}
		if first || isFatalPollError(err) {
			return true, err
		}
		fmt.Fprintf(errOut, "Warning: poll failed, retrying in %s: %v\n", interval, err)
		return false, nil
	}
	if done, err := run(); done || err != nil {
		return err
	}
	ticks, stop := tick(interval)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticks:
			if done, err := run(); done || err != nil {
				return err
			}
		}
	}
}

// isFatalPollError reports whether a mid-stream failure means the poll can
// never succeed again: the credential was rejected, the scope was denied, or
// the polled resource no longer exists.
func isFatalPollError(err error) bool {
	return n8n.IsUnauthorized(err) || n8n.IsForbidden(err) || n8n.IsNotFound(err)
}

// ignoreCanceled maps a context cancellation to success. Both streaming
// commands document that Ctrl+C exits 0, and a request interrupted mid-flight
// surfaces as a wrapped context.Canceled that would otherwise exit 130.
func ignoreCanceled(ctx context.Context, err error) error {
	if err != nil && (ctx.Err() != nil || errors.Is(err, context.Canceled)) {
		return nil
	}
	return err
}

// workflowResolver caches workflow lookups for a whole process. Name lookup
// needs workflow:read, which an instance-wide watch will lack for some
// workflows, so a failure is cached too: each ID is attempted at most once,
// and a resolver failure never fails a tick.
type workflowResolver struct {
	client    *n8n.Client
	workflows map[string]*n8n.Workflow
	failed    map[string]bool
}

func newWorkflowResolver(client *n8n.Client) *workflowResolver {
	return &workflowResolver{client: client, workflows: map[string]*n8n.Workflow{}, failed: map[string]bool{}}
}

// lookup returns the workflow, or nil when it could not be read.
func (r *workflowResolver) lookup(ctx context.Context, id string) *n8n.Workflow {
	if id == "" || r.failed[id] {
		return nil
	}
	if w, ok := r.workflows[id]; ok {
		return w
	}
	w, err := r.client.GetWorkflow(ctx, id, true)
	if err != nil {
		r.failed[id] = true
		return nil
	}
	r.workflows[id] = w
	return w
}

// name returns the workflow's name, or the ID itself when it cannot be read.
func (r *workflowResolver) name(ctx context.Context, id string) string {
	if w := r.lookup(ctx, id); w != nil && w.Name != "" {
		return w.Name
	}
	return id
}

// resolvedName is the name when known, else nil, for JSON output.
func (r *workflowResolver) resolvedName(ctx context.Context, id string) *string {
	if w := r.lookup(ctx, id); w != nil && w.Name != "" {
		return &w.Name
	}
	return nil
}

// label renders "Name (ID)" for headers, or just the ID when unresolved.
func (r *workflowResolver) label(ctx context.Context, id string) string {
	name := r.name(ctx, id)
	if name == id {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

// nodeCount is the node count of the workflow definition, or zero.
func (r *workflowResolver) nodeCount(ctx context.Context, id string) int {
	if w := r.lookup(ctx, id); w != nil {
		return w.NodeCount()
	}
	return 0
}

// listExecutionsWithRunning fetches page one for opts and, when no status
// filter is set, merges in the running executions: the unfiltered
// GET /executions omits status=running rows (verified against n8n on
// 2026-09-18; waiting and finished rows are included, and a run shows as
// "new" for about a second before it vanishes), so a poller reading only page
// one would never see an active run until it ended. The merged page is newest
// first and capped at opts.Limit when that is set.
func listExecutionsWithRunning(ctx context.Context, client *n8n.Client, opts n8n.ListExecutionsOptions) ([]n8n.Execution, error) {
	page, err := client.ListExecutions(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.Status != "" {
		return page.Data, nil
	}
	runningOpts := opts
	runningOpts.Status = "running"
	running, err := client.ListExecutions(ctx, runningOpts)
	if err != nil {
		return nil, err
	}
	if len(running.Data) == 0 {
		return page.Data, nil
	}
	// Active rows first, so a run listed as "new" and "running" at once keeps
	// the live status.
	merged, _ := mergeExecutionsNewestFirst(opts.Limit, running.Data, page.Data)
	return merged, nil
}

// compareExecutionIDs orders execution IDs. They are numeric strings on every
// known instance, so compare numerically and fall back to string order.
func compareExecutionIDs(a, b string) int {
	ai, aerr := strconv.ParseInt(a, 10, 64)
	bi, berr := strconv.ParseInt(b, 10, 64)
	if aerr == nil && berr == nil {
		return cmp.Compare(ai, bi)
	}
	return strings.Compare(a, b)
}

func executionIDLess(a, b string) bool { return compareExecutionIDs(a, b) < 0 }

// executionStoppedAt is the stop time of a finished run, or nil. n8n also sets
// stoppedAt on a waiting run (the moment it went to sleep), which would read
// as a completed run, so the field is only trusted once the run is finished.
func executionStoppedAt(execution *n8n.Execution) *string {
	if !execution.IsFinished() {
		return nil
	}
	return execution.StoppedAt
}

// executionDurationMs is the wall time of a finished run, or nil while it is
// still going or when either timestamp is missing or unreadable.
func executionDurationMs(execution *n8n.Execution) *int64 {
	if execution.StartedAt == nil || executionStoppedAt(execution) == nil {
		return nil
	}
	start, err := time.Parse(time.RFC3339Nano, *execution.StartedAt)
	if err != nil {
		return nil
	}
	stop, err := time.Parse(time.RFC3339Nano, *execution.StoppedAt)
	if err != nil {
		return nil
	}
	ms := stop.Sub(start).Milliseconds()
	return &ms
}

// formatDurationMs renders a millisecond duration for a table cell.
func formatDurationMs(ms *int64) string {
	if ms == nil {
		return "-"
	}
	return fmt.Sprintf("%dms", *ms)
}

// runErrorJSON is the compact error shape the streaming commands emit.
type runErrorJSON struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func toRunErrorJSON(err *n8n.RunError) *runErrorJSON {
	if err == nil {
		return nil
	}
	return &runErrorJSON{Name: err.Name, Message: err.Text()}
}

// writeNDJSON writes one compact JSON object followed by a newline, so a
// never-ending stream is readable incrementally by `jq -c` and friends.
func writeNDJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// singleLineText collapses whitespace so an error message fits one table cell.
func singleLineText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
