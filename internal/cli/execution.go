package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const executionResource = "execution"

func newExecutionCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execution",
		Short: "Inspect, stop, retry and delete workflow executions",
		Long: "Manage workflow executions and their annotation tags. Start with 'list' to\n" +
			"find execution IDs, then use 'get' for one run and --include-data only when its\n" +
			"node input and output are needed. Detailed run data can be large; list refuses\n" +
			"to combine it with --all, and the server omits oversized data unless explicitly\n" +
			"overridden.\n\n" +
			"'stop' cancels one active execution, 'stop-many' cancels a filtered set, and\n" +
			"'retry' creates a new run from a failed one. 'delete' permanently removes the\n" +
			"stored run. Stop and delete actions ask for confirmation. Execution annotation\n" +
			"tags are read and replaced under the 'tag' subgroup.",
		Example: "  n8n execution list --status error\n" +
			"  n8n execution get EXECUTION_ID --include-data --output json\n" +
			"  n8n execution retry EXECUTION_ID\n" +
			"  n8n execution stop EXECUTION_ID\n" +
			"  n8n execution tag list EXECUTION_ID\n" +
			"  n8n execution delete EXECUTION_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newExecutionListCommand(opts),
		newExecutionGetCommand(opts),
		newExecutionDeleteCommand(opts),
		newExecutionStopManyCommand(opts),
		newExecutionStopCommand(opts),
		newExecutionRetryCommand(opts),
		newExecutionTagCommand(opts),
	)
	return cmd
}

type executionDataFlags struct {
	includeData    bool
	ignoreDataSize bool
	redactData     string
}

func (f *executionDataFlags) register(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.includeData, "include-data", false, "include detailed node input/output and saved workflow data (default: metadata only)")
	cmd.Flags().BoolVar(&f.ignoreDataSize, "ignore-data-size-limit", false, "return detailed data even when it exceeds the instance display-size limit (default: omit oversized data)")
	cmd.Flags().StringVar(&f.redactData, "redact-execution-data", "", "detailed-data redaction: true always redacts, false reveals and needs execution:reveal (default: workflow policy)")
}

func (f executionDataFlags) options() (n8n.ExecutionDataOptions, error) {
	redact, err := optionalBool("--redact-execution-data", f.redactData)
	if err != nil {
		return n8n.ExecutionDataOptions{}, err
	}
	if f.ignoreDataSize && !f.includeData {
		return n8n.ExecutionDataOptions{}, fmt.Errorf("--ignore-data-size-limit requires --include-data: there is no detailed data to override otherwise")
	}
	return n8n.ExecutionDataOptions{
		IncludeData:         f.includeData,
		IgnoreDataSizeLimit: f.ignoreDataSize,
		RedactExecutionData: redact,
	}, nil
}

func optionalBool(flag, value string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return nil, nil
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	default:
		return nil, fmt.Errorf("unknown %s value %q: use true or false, or leave the flag off for the server policy", flag, value)
	}
}

type executionListFlags struct {
	instance      instanceFlags
	data          executionDataFlags
	limit         int
	cursor        string
	all           bool
	status        string
	workflowID    string
	projectID     string
	startedAfter  string
	startedBefore string
	output        string
}

func newExecutionListCommand(opts Options) *cobra.Command {
	var f executionListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List executions with status, workflow and time filters",
		Long: "List executions one cursor-paginated page at a time. Pass nextCursor to\n" +
			"--cursor, or use --all to follow every page, capped at 10,000 metadata rows.\n\n" +
			"Filter by status, workflow, project, or an RFC3339 start-time range. By default\n" +
			"the response contains metadata only. --include-data adds potentially large node\n" +
			"input/output and workflow data, so it cannot be combined with --all; page through\n" +
			"such results explicitly. Oversized data remains omitted unless\n" +
			"--ignore-data-size-limit is given. Requesting unredacted data additionally needs\n" +
			"execution:reveal. Listing requires execution:list.",
		Example: "  n8n execution list\n" +
			"  n8n execution list --status error --workflow-id WORKFLOW_ID\n" +
			"  n8n execution list --started-after 2026-09-17T00:00:00Z --limit 50\n" +
			"  n8n execution list --include-data --limit 10 --output json\n" +
			"  n8n execution list --all --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExecutionList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	f.data.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "executions per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every metadata page instead of one (maximum 10,000 executions; incompatible with --include-data)")
	cmd.Flags().StringVar(&f.status, "status", "", "execution status: canceled, crashed, error, new, running, success, unknown, or waiting (default: every status)")
	cmd.Flags().StringVar(&f.workflowID, "workflow-id", "", "only executions of this workflow, from 'n8n workflow list' (default: every workflow)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "only executions in this project, from 'n8n project list' (default: every accessible project)")
	cmd.Flags().StringVar(&f.startedAfter, "started-after", "", "only executions started after this RFC3339 timestamp, e.g. 2026-09-17T00:00:00Z (default: no lower bound)")
	cmd.Flags().StringVar(&f.startedBefore, "started-before", "", "only executions started before this RFC3339 timestamp, e.g. 2026-09-18T00:00:00Z (default: no upper bound)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runExecutionList(ctx context.Context, opts Options, f executionListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	dataOpts, err := f.data.options()
	if err != nil {
		return err
	}
	if f.all && dataOpts.IncludeData {
		return fmt.Errorf("--all cannot be combined with --include-data: detailed execution data can be unbounded; use --limit and --cursor to page explicitly")
	}
	listOpts := n8n.ListExecutionsOptions{
		ListOptions:          n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		ExecutionDataOptions: dataOpts,
		Status:               f.status,
		WorkflowID:           f.workflowID,
		ProjectID:            f.projectID,
		StartedAfter:         f.startedAfter,
		StartedBefore:        f.startedBefore,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pageOpts n8n.ListOptions) (n8n.Page[n8n.Execution], error) {
		next := listOpts
		next.ListOptions = pageOpts
		return client.ListExecutions(ctx, next)
	}
	var page n8n.Page[n8n.Execution]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return apiError(err, resolution, executionResource)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeExecutionList(opts, resolution, page)
}

func writeExecutionList(opts Options, resolution config.Resolution, page n8n.Page[n8n.Execution]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Executions:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tWORKFLOW\tSTATUS\tMODE\tSTARTED\tSTOPPED\tSIZE")
		for _, execution := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				emptyDash(execution.ID), emptyDash(execution.WorkflowID), emptyDash(execution.Status),
				emptyDash(execution.Mode), optionalText(execution.StartedAt), optionalText(execution.StoppedAt),
				formatBytes(execution.JSONSizeBytes))
		}
	}
	if page.HasMore() {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No executions were returned. Widen the status, workflow, project, or time filters.")
	}
	return nil
}

func optionalText(value *string) string {
	if value == nil {
		return "-"
	}
	return emptyDash(*value)
}

func formatBytes(value float64) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f B", value)
}

type executionGetFlags struct {
	instance instanceFlags
	data     executionDataFlags
	output   string
}

func newExecutionGetCommand(opts Options) *cobra.Command {
	var f executionGetFlags
	cmd := &cobra.Command{
		Use:   "get <execution-id>",
		Short: "Show one execution",
		Long: "Show one execution and its run metadata. Add --include-data to include node\n" +
			"input/output, custom data and the workflow definition saved with the run. That\n" +
			"payload may be large; the server omits it when over its display-size limit unless\n" +
			"--ignore-data-size-limit is explicitly given.\n\n" +
			"Redaction follows the workflow policy by default. Setting\n" +
			"--redact-execution-data=false requests revealed data and needs execution:reveal.\n" +
			"Reading the execution itself requires execution:read.",
		Example: "  n8n execution get EXECUTION_ID\n" +
			"  n8n execution get EXECUTION_ID --output json\n" +
			"  n8n execution get EXECUTION_ID --include-data --output json\n" +
			"  n8n execution get EXECUTION_ID --include-data --ignore-data-size-limit --redact-execution-data=false --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecutionGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	f.data.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON includes detailed data only with --include-data)")
	return cmd
}

func runExecutionGet(ctx context.Context, opts Options, id string, f executionGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateExecutionArgument(id); err != nil {
		return err
	}
	dataOpts, err := f.data.options()
	if err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	execution, err := client.GetExecution(ctx, id, dataOpts)
	if err != nil {
		return executionAPIError(err, resolution, id, "read")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, execution)
	}
	return writeExecutionDetail(opts, resolution, "Execution", execution)
}

func writeExecutionDetail(opts Options, resolution config.Resolution, label string, execution *n8n.Execution) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", label, emptyDash(execution.ID))
	fmt.Fprintf(tw, "Workflow:\t%s\n", emptyDash(execution.WorkflowID))
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(execution.Status))
	fmt.Fprintf(tw, "Mode:\t%s\n", emptyDash(execution.Mode))
	fmt.Fprintf(tw, "Started:\t%s\n", optionalText(execution.StartedAt))
	fmt.Fprintf(tw, "Stopped:\t%s\n", optionalText(execution.StoppedAt))
	fmt.Fprintf(tw, "Retry of:\t%s\n", optionalText(execution.RetryOf))
	fmt.Fprintf(tw, "Storage:\t%s\n", emptyDash(execution.StoredAt))
	fmt.Fprintf(tw, "JSON size:\t%s\n", formatBytes(execution.JSONSizeBytes))
	fmt.Fprintf(tw, "Data included:\t%t\n", len(execution.Data) > 0)
	fmt.Fprintf(tw, "Data too large:\t%t\n", execution.DataTooLargeToDisplay)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type executionConfirmedFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newExecutionDeleteCommand(opts Options) *cobra.Command {
	var f executionConfirmedFlags
	cmd := &cobra.Command{
		Use:   "delete <execution-id>",
		Short: "Permanently delete one stored execution",
		Long: "Permanently delete one execution record and its stored run data. The workflow\n" +
			"it ran is not changed or deleted, but this execution can no longer be inspected,\n" +
			"retried, or used for debugging. This cannot be undone.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes. Use\n" +
			"'n8n execution get ID --output json' first when a record must be retained.\n" +
			"Requires execution:delete.",
		Example: "  n8n execution delete EXECUTION_ID\n" +
			"  n8n execution delete EXECUTION_ID --yes\n" +
			"  n8n execution delete EXECUTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecutionDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deleted execution as last stored)")
	return cmd
}

func runExecutionDelete(ctx context.Context, opts Options, id string, f executionConfirmedFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateExecutionArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete execution %q and its stored run data from %s? This cannot be undone; its workflow is not deleted.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	deleted, err := client.DeleteExecution(ctx, id)
	if err != nil {
		return executionAPIError(err, resolution, id, "delete")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, deleted)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Deleted execution:\t%s\n", deleted.ID.String())
	fmt.Fprintf(tw, "Workflow:\t%s\n", emptyDash(deleted.WorkflowID))
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(deleted.Status))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type executionStopManyFlags struct {
	instance      instanceFlags
	statuses      []string
	workflowID    string
	all           bool
	startedAfter  string
	startedBefore string
	yes           bool
	output        string
}

func newExecutionStopManyCommand(opts Options) *cobra.Command {
	var f executionStopManyFlags
	cmd := &cobra.Command{
		Use:   "stop-many",
		Short: "Stop active executions matching filters",
		Long: "Cancel queued, running, or waiting executions matching the supplied statuses and\n" +
			"RFC3339 time range within one workflow scope. At least one --status is required.\n\n" +
			"Scope is explicit: pass --workflow-id for one workflow or --all for every\n" +
			"accessible workflow. Omitting both is rejected so an empty filter can never stop\n" +
			"workflows globally by accident; the literal workflow ID 'all' is rejected, use\n" +
			"--all instead. Stopping is irreversible for each matched run, though a failed\n" +
			"or canceled run may later be retried. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires execution:stop.",
		Example: "  n8n execution stop-many --status running --workflow-id WORKFLOW_ID\n" +
			"  n8n execution stop-many --status queued --status waiting --all --yes\n" +
			"  n8n execution stop-many --status running --workflow-id WORKFLOW_ID --started-after 2026-09-17T00:00:00Z --yes --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExecutionStopMany(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.statuses, "status", nil, "status to stop: queued, running, or waiting; repeatable (at least one required)")
	cmd.Flags().StringVar(&f.workflowID, "workflow-id", "", "only executions of this workflow, from 'n8n workflow list' (required unless --all)")
	cmd.Flags().BoolVar(&f.all, "all", false, "stop matching executions across every accessible workflow (cannot combine with --workflow-id)")
	cmd.Flags().StringVar(&f.startedAfter, "started-after", "", "only executions started after this RFC3339 timestamp, e.g. 2026-09-17T00:00:00Z (default: no lower bound)")
	cmd.Flags().StringVar(&f.startedBefore, "started-before", "", "only executions started before this RFC3339 timestamp, e.g. 2026-09-18T00:00:00Z (default: no upper bound)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm stopping every match without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON contains the number stopped)")
	return cmd
}

func runExecutionStopMany(ctx context.Context, opts Options, f executionStopManyFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	request := n8n.StopManyExecutionsRequest{
		Status:        f.statuses,
		WorkflowID:    f.workflowID,
		StartedAfter:  f.startedAfter,
		StartedBefore: f.startedBefore,
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if f.all && f.workflowID != "" {
		return fmt.Errorf("--all cannot be combined with --workflow-id: --all spans every accessible workflow, --workflow-id scopes to one workflow")
	}
	if f.workflowID == "all" {
		return fmt.Errorf("--workflow-id \"all\" is not accepted: use --all to stop executions across every accessible workflow")
	}
	if !f.all && strings.TrimSpace(f.workflowID) == "" {
		return fmt.Errorf("--workflow-id or --all is required: pass the workflow ID to scope the stop, or --all to stop across every accessible workflow")
	}
	if f.all {
		request.WorkflowID = ""
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	scope := "every accessible workflow"
	if !f.all {
		scope = fmt.Sprintf("workflow %q", f.workflowID)
	}
	question := fmt.Sprintf("Stop all %s executions in %s on %s? Every matched run is canceled and cannot resume.", strings.Join(f.statuses, ", "), scope, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.StopManyExecutions(ctx, request)
	if err != nil {
		return apiError(err, resolution, executionResource)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Stopped:\t%d\n", result.Stopped)
	fmt.Fprintf(tw, "Scope:\t%s\n", scope)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func newExecutionStopCommand(opts Options) *cobra.Command {
	var f executionConfirmedFlags
	cmd := &cobra.Command{
		Use:   "stop <execution-id>",
		Short: "Stop one queued, running, or waiting execution",
		Long: "Cancel one queued, running, or waiting execution. The current run cannot resume;\n" +
			"use 'n8n execution retry ID' afterwards when it should run again as a new\n" +
			"execution. A run already finished or otherwise not stoppable returns a conflict.\n\n" +
			"Interactive runs ask for confirmation and non-interactive runs require --yes.\n" +
			"The workflow definition is untouched. Requires execution:stop.",
		Example: "  n8n execution stop EXECUTION_ID\n" +
			"  n8n execution stop EXECUTION_ID --yes\n" +
			"  n8n execution stop EXECUTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecutionStop(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm canceling the execution without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the resulting stop state)")
	return cmd
}

func runExecutionStop(ctx context.Context, opts Options, id string, f executionConfirmedFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateExecutionArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Stop execution %q on %s? Its current run is canceled and cannot resume; the workflow is untouched.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.StopExecution(ctx, id)
	if err != nil {
		if n8n.IsConflict(err) {
			return fmt.Errorf("%s cannot stop execution %q (409): it is already finished or is not in a stoppable state", resolution.URL, id)
		}
		return executionAPIError(err, resolution, id, "stop")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Stopped execution:\t%s\n", id)
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(result.Status))
	fmt.Fprintf(tw, "Mode:\t%s\n", emptyDash(result.Mode))
	fmt.Fprintf(tw, "Started:\t%s\n", emptyDash(result.StartedAt))
	fmt.Fprintf(tw, "Stopped:\t%s\n", emptyDash(result.StoppedAt))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type executionRetryFlags struct {
	instance       instanceFlags
	latestWorkflow bool
	output         string
}

func newExecutionRetryCommand(opts Options) *cobra.Command {
	var f executionRetryFlags
	cmd := &cobra.Command{
		Use:   "retry <execution-id>",
		Short: "Retry a failed execution as a new run",
		Long: "Start a new execution from an existing failed run. By default n8n runs the\n" +
			"workflow definition stored with the original execution, making the retry\n" +
			"reproducible. --latest-workflow instead loads the workflow currently saved on\n" +
			"the instance.\n\n" +
			"The original execution is retained and the response identifies the new run. A\n" +
			"run that cannot be retried returns a conflict. This starts workflow work and may\n" +
			"repeat its external side effects. Requires execution:retry.",
		Example: "  n8n execution retry EXECUTION_ID\n" +
			"  n8n execution retry EXECUTION_ID --latest-workflow\n" +
			"  n8n execution retry EXECUTION_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecutionRetry(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.latestWorkflow, "latest-workflow", false, "run the workflow currently saved instead of the definition stored with the original execution (default: stored definition)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the newly started execution)")
	return cmd
}

func runExecutionRetry(ctx context.Context, opts Options, id string, f executionRetryFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateExecutionArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	execution, err := client.RetryExecution(ctx, id, n8n.RetryExecutionOptions{LoadWorkflow: f.latestWorkflow})
	if err != nil {
		if n8n.IsConflict(err) {
			return fmt.Errorf("%s cannot retry execution %q (409): its state or stored data does not permit a retry", resolution.URL, id)
		}
		return executionAPIError(err, resolution, id, "retry")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, execution)
	}
	return writeExecutionDetail(opts, resolution, "New execution", execution)
}

func newExecutionTagCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Read and replace annotation tags on an execution",
		Long: "Read and replace the annotation tags attached to an execution. Tags themselves\n" +
			"are managed with 'n8n tag'; this subgroup only controls which existing tag IDs\n" +
			"annotate one execution.\n\n" +
			"Start with 'list', then use 'set'. Setting is replacement, not addition: every\n" +
			"current tag that is not repeated is removed. Use --clear to remove all tags.",
		Example: "  n8n execution tag list EXECUTION_ID\n" +
			"  n8n tag list\n" +
			"  n8n execution tag set EXECUTION_ID --tag-id TAG_ID\n" +
			"  n8n execution tag set EXECUTION_ID --clear",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newExecutionTagListCommand(opts), newExecutionTagSetCommand(opts))
	return cmd
}

type executionTagListFlags struct {
	instance instanceFlags
	output   string
}

func newExecutionTagListCommand(opts Options) *cobra.Command {
	var f executionTagListFlags
	cmd := &cobra.Command{
		Use:   "list <execution-id>",
		Short: "List annotation tags on one execution",
		Long: "List the complete, unpaginated set of annotation tags attached to one\n" +
			"execution. Run this before 'n8n execution tag set', because setting replaces the\n" +
			"whole set and a partial change must repeat every tag ID to keep. Requires\n" +
			"executionTags:list.",
		Example: "  n8n execution tag list EXECUTION_ID\n" +
			"  n8n execution tag list EXECUTION_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecutionTagList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the complete array of tags)")
	return cmd
}

func runExecutionTagList(ctx context.Context, opts Options, id string, f executionTagListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateExecutionArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	tags, err := client.GetExecutionTags(ctx, id)
	if err != nil {
		return executionAPIError(err, resolution, id, "tag read")
	}
	return writeExecutionTags(opts, resolution, id, tags, f.output)
}

type executionTagSetFlags struct {
	instance instanceFlags
	tagIDs   []string
	clear    bool
	output   string
}

func newExecutionTagSetCommand(opts Options) *cobra.Command {
	var f executionTagSetFlags
	cmd := &cobra.Command{
		Use:   "set <execution-id>",
		Short: "Replace annotation tags on one execution",
		Long: "Replace the complete annotation-tag set on one execution with repeated\n" +
			"--tag-id values, or remove all tags with --clear. This is replacement, not\n" +
			"addition: omitted current tags are removed.\n\n" +
			"List current IDs with 'n8n execution tag list EXECUTION_ID' and available IDs\n" +
			"with 'n8n tag list'. Tag names are not accepted. Requires executionTags:update.",
		Example: "  n8n execution tag set EXECUTION_ID --tag-id TAG_ID\n" +
			"  n8n execution tag set EXECUTION_ID --tag-id TAG_ID --tag-id OTHER_TAG_ID\n" +
			"  n8n execution tag set EXECUTION_ID --clear\n" +
			"  n8n execution tag set EXECUTION_ID --tag-id TAG_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecutionTagSet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.tagIDs, "tag-id", nil, "tag ID the execution should carry, repeatable, from 'n8n tag list' (required unless --clear)")
	cmd.Flags().BoolVar(&f.clear, "clear", false, "remove every annotation tag from the execution (cannot combine with --tag-id)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the resulting array of tags)")
	return cmd
}

func runExecutionTagSet(ctx context.Context, opts Options, id string, f executionTagSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateExecutionArgument(id); err != nil {
		return err
	}
	switch {
	case f.clear && len(f.tagIDs) > 0:
		return fmt.Errorf("--clear cannot be combined with --tag-id: --clear removes every tag, --tag-id names the tags to keep")
	case !f.clear && len(f.tagIDs) == 0:
		return fmt.Errorf("--tag-id or --clear is required: pass the tag IDs to keep, or --clear to remove them all")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	tags, err := client.SetExecutionTags(ctx, id, f.tagIDs)
	if err != nil {
		return executionAPIError(err, resolution, id, "tag update")
	}
	return writeExecutionTags(opts, resolution, id, tags, f.output)
}

func writeExecutionTags(opts Options, resolution config.Resolution, id string, tags []n8n.Tag, output string) error {
	if output == outputJSON {
		if tags == nil {
			tags = []n8n.Tag{}
		}
		return writeJSON(opts.Streams.Out, tags)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Execution:\t%s\n", id)
	fmt.Fprintf(tw, "Tags:\t%d\n", len(tags))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(tags) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tCREATED\tUPDATED")
		for _, tag := range tags {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", tag.ID, tag.Name, tag.CreatedAt, tag.UpdatedAt)
		}
	}
	return tw.Flush()
}

func validateExecutionArgument(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("execution ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("execution ID must not start or end with whitespace")
	}
	return nil
}

func executionAPIError(err error, resolution config.Resolution, id, action string) error {
	if n8n.IsNotFound(err) {
		return fmt.Errorf("%s has no execution %q (404): find an ID with 'n8n execution list'", resolution.URL, id)
	}
	if n8n.IsConflict(err) {
		return fmt.Errorf("%s could not %s execution %q because its state changed (409): read it again with 'n8n execution get %s'", resolution.URL, action, id, id)
	}
	return apiError(err, resolution, executionResource)
}
