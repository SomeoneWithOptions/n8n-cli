package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const evaluationResource = "evaluation"

func newEvaluationCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evaluation",
		Short: "Run and inspect workflow evaluations",
		Long: "Manage asynchronous evaluation test runs for workflows configured with an\n" +
			"evaluation trigger. Start with 'list WORKFLOW_ID' to find run IDs, use 'get'\n" +
			"for aggregate metrics, then 'case list' for per-case inputs, outputs and\n" +
			"metrics. Both lists support cursor pagination and --all.\n\n" +
			"'create' starts a test run and returns immediately in a new or running state;\n" +
			"poll 'get' until completed, error, or cancelled. 'cancel' asks for confirmation\n" +
			"and only applies while a run is cancellable. Creating and cancelling also need\n" +
			"workflow:execute permission in the workflow's project.",
		Example: "  n8n evaluation list WORKFLOW_ID\n" +
			"  n8n evaluation create WORKFLOW_ID --output json\n" +
			"  n8n evaluation get WORKFLOW_ID RUN_ID --output json\n" +
			"  n8n evaluation case list WORKFLOW_ID RUN_ID --all --output json\n" +
			"  n8n evaluation cancel WORKFLOW_ID RUN_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newEvaluationListCommand(opts),
		newEvaluationCreateCommand(opts),
		newEvaluationGetCommand(opts),
		newEvaluationCancelCommand(opts),
		newEvaluationCaseCommand(opts),
	)
	return cmd
}

type evaluationListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	status   string
	output   string
}

func newEvaluationListCommand(opts Options) *cobra.Command {
	var f evaluationListFlags
	cmd := &cobra.Command{
		Use:   "list <workflow-id>",
		Short: "List evaluation runs for a workflow",
		Long: "List asynchronous evaluation test runs for one workflow. Filter by new,\n" +
			"running, completed, error, or cancelled status. Pass nextCursor to --cursor,\n" +
			"or use --all to follow every page up to 10,000 runs.\n\n" +
			"Use 'n8n evaluation get WORKFLOW_ID RUN_ID' for aggregate metrics and final\n" +
			"result, then 'n8n evaluation case list' for individual cases. Requires\n" +
			"testRun:list and access to the workflow's project.",
		Example: "  n8n evaluation list WORKFLOW_ID\n" +
			"  n8n evaluation list WORKFLOW_ID --status running --limit 50\n" +
			"  n8n evaluation list WORKFLOW_ID --all --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvaluationList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "evaluation runs per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous run list (default: first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every run page instead of one (maximum 10,000 runs)")
	cmd.Flags().StringVar(&f.status, "status", "", "run status: new, running, completed, error, or cancelled (default: every status)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runEvaluationList(ctx context.Context, opts Options, workflowID string, f evaluationListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateEvaluationArgument("workflow ID", workflowID); err != nil {
		return err
	}
	listOpts := n8n.ListEvaluationRunsOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Status:      f.status,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pageOpts n8n.ListOptions) (n8n.Page[n8n.EvaluationRunSummary], error) {
		next := listOpts
		next.ListOptions = pageOpts
		return client.ListEvaluationRuns(ctx, workflowID, next)
	}
	var page n8n.Page[n8n.EvaluationRunSummary]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return evaluationAPIError(err, resolution, workflowID, "", "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeEvaluationRunList(opts, resolution, workflowID, page)
}

func writeEvaluationRunList(opts Options, resolution config.Resolution, workflowID string, page n8n.Page[n8n.EvaluationRunSummary]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", workflowID)
	fmt.Fprintf(tw, "Evaluation runs:\t%d\n", len(page.Data))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tSTATUS\tRESULT\tCASES\tSTARTED\tCOMPLETED")
		for _, run := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", emptyDash(run.ID), emptyDash(string(run.Status)),
				optionalText(run.FinalResult), run.TestCaseCount, optionalText(run.RunAt), optionalText(run.CompletedAt))
		}
	}
	if page.HasMore() {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No evaluation runs were returned. Check the workflow ID or widen the status filter; start one with 'n8n evaluation create WORKFLOW_ID'.")
	}
	return nil
}

type evaluationActionFlags struct {
	instance instanceFlags
	output   string
}

func newEvaluationCreateCommand(opts Options) *cobra.Command {
	var f evaluationActionFlags
	cmd := &cobra.Command{
		Use:   "create <workflow-id>",
		Short: "Start an asynchronous workflow evaluation",
		Long: "Start an evaluation test run for a workflow containing a configured evaluation\n" +
			"trigger. The request returns immediately with a run ID and a new or running\n" +
			"status; it does not wait for test cases to finish.\n\n" +
			"Poll 'n8n evaluation get WORKFLOW_ID RUN_ID' for completion and aggregate\n" +
			"metrics. A conflict can mean the workflow cannot start another test run.\n" +
			"Requires testRun:create plus workflow:execute in the workflow's project; the\n" +
			"instance must license evaluations.",
		Example: "  n8n evaluation create WORKFLOW_ID\n" +
			"  n8n evaluation create WORKFLOW_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvaluationCreate(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the accepted run with its initial asynchronous status)")
	return cmd
}

func runEvaluationCreate(ctx context.Context, opts Options, workflowID string, f evaluationActionFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateEvaluationArgument("workflow ID", workflowID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	run, err := client.CreateEvaluationRun(ctx, workflowID)
	if err != nil {
		return evaluationAPIError(err, resolution, workflowID, "", "create")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, run)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Evaluation run:\t%s\n", emptyDash(run.ID))
	fmt.Fprintf(tw, "Workflow:\t%s\n", workflowID)
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(string(run.Status)))
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(run.CreatedAt))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Next:\tn8n evaluation get %s %s\n", workflowID, run.ID)
	return tw.Flush()
}

func newEvaluationGetCommand(opts Options) *cobra.Command {
	var f evaluationActionFlags
	cmd := &cobra.Command{
		Use:   "get <workflow-id> <run-id>",
		Short: "Show one evaluation run and its aggregate result",
		Long: "Show one evaluation run, including asynchronous status, aggregate metrics,\n" +
			"final result, error details and test-case count. New and running runs have no\n" +
			"final result yet; poll this command until completed, error, or cancelled.\n\n" +
			"Use 'n8n evaluation case list WORKFLOW_ID RUN_ID' for per-case inputs, outputs,\n" +
			"metrics and underlying execution IDs. Requires testRun:read.",
		Example: "  n8n evaluation get WORKFLOW_ID RUN_ID\n" +
			"  n8n evaluation get WORKFLOW_ID RUN_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvaluationGet(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON preserves workflow-defined metrics and error details)")
	return cmd
}

func runEvaluationGet(ctx context.Context, opts Options, workflowID, runID string, f evaluationActionFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateEvaluationArguments(workflowID, runID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	run, err := client.GetEvaluationRun(ctx, workflowID, runID)
	if err != nil {
		return evaluationAPIError(err, resolution, workflowID, runID, "read")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, run)
	}
	return writeEvaluationRun(opts, resolution, workflowID, run)
}

func writeEvaluationRun(opts Options, resolution config.Resolution, workflowID string, run *n8n.EvaluationRunSummary) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Evaluation run:\t%s\n", emptyDash(run.ID))
	fmt.Fprintf(tw, "Workflow:\t%s\n", workflowID)
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(string(run.Status)))
	fmt.Fprintf(tw, "Final result:\t%s\n", optionalText(run.FinalResult))
	fmt.Fprintf(tw, "Test cases:\t%d\n", run.TestCaseCount)
	fmt.Fprintf(tw, "Started:\t%s\n", optionalText(run.RunAt))
	fmt.Fprintf(tw, "Completed:\t%s\n", optionalText(run.CompletedAt))
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(run.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", emptyDash(run.UpdatedAt))
	fmt.Fprintf(tw, "Metrics:\t%s\n", rawEvaluationValue(run.Metrics))
	fmt.Fprintf(tw, "Error code:\t%s\n", optionalText(run.ErrorCode))
	fmt.Fprintf(tw, "Error details:\t%s\n", rawEvaluationValue(run.ErrorDetails))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func rawEvaluationValue(value []byte) string {
	text := strings.TrimSpace(string(value))
	if text == "" || text == "null" {
		return "-"
	}
	return text
}

type evaluationCancelFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newEvaluationCancelCommand(opts Options) *cobra.Command {
	var f evaluationCancelFlags
	cmd := &cobra.Command{
		Use:   "cancel <workflow-id> <run-id>",
		Short: "Cancel a new or running evaluation",
		Long: "Request cancellation of one new or running evaluation. Work already performed\n" +
			"by its cases is not rolled back, and the same run cannot resume. A run that\n" +
			"already reached completed, error, or cancelled returns a conflict.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require --yes. Use\n" +
			"'n8n evaluation get WORKFLOW_ID RUN_ID' first to inspect current state.\n" +
			"Requires testRun:cancel plus workflow:execute in the workflow's project.",
		Example: "  n8n evaluation cancel WORKFLOW_ID RUN_ID\n" +
			"  n8n evaluation cancel WORKFLOW_ID RUN_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvaluationCancel(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm cancellation without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the cancellation acknowledgement)")
	return cmd
}

func runEvaluationCancel(ctx context.Context, opts Options, workflowID, runID string, f evaluationCancelFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateEvaluationArguments(workflowID, runID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Cancel evaluation run %q for workflow %q on %s? Its test cases stop and this run cannot resume.", runID, workflowID, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.CancelEvaluationRun(ctx, workflowID, runID)
	if err != nil {
		return evaluationAPIError(err, resolution, workflowID, runID, "cancel")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Evaluation run:\t%s\n", emptyDash(result.ID))
	fmt.Fprintf(tw, "Workflow:\t%s\n", workflowID)
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(string(result.Status)))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func newEvaluationCaseCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "case",
		Short: "Inspect cases within an evaluation run",
		Long: "Inspect per-case results within one evaluation run. Start with 'list' after\n" +
			"finding a run through 'n8n evaluation list'. Case JSON includes workflow-defined\n" +
			"inputs, outputs and metrics plus an underlying execution ID when retained.\n\n" +
			"Case lists use their own cursor independently from the parent run list. Use\n" +
			"--output json when full case payloads are needed.",
		Example: "  n8n evaluation case list WORKFLOW_ID RUN_ID\n" +
			"  n8n evaluation case list WORKFLOW_ID RUN_ID --all --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newEvaluationCaseListCommand(opts))
	return cmd
}

type evaluationCaseListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newEvaluationCaseListCommand(opts Options) *cobra.Command {
	var f evaluationCaseListFlags
	cmd := &cobra.Command{
		Use:   "list <workflow-id> <run-id>",
		Short: "List per-case results for an evaluation run",
		Long: "List one evaluation run's per-case statuses, timestamps and retained execution\n" +
			"IDs. JSON output additionally preserves each case's workflow-defined inputs,\n" +
			"outputs, metrics and error details.\n\n" +
			"Pass nextCursor to --cursor, or use --all to follow every case page up to 10,000\n" +
			"cases. This pagination is independent from 'n8n evaluation list'. Requires\n" +
			"testRun:read and access to the workflow's project.",
		Example: "  n8n evaluation case list WORKFLOW_ID RUN_ID\n" +
			"  n8n evaluation case list WORKFLOW_ID RUN_ID --limit 50 --cursor NEXT_CURSOR\n" +
			"  n8n evaluation case list WORKFLOW_ID RUN_ID --all --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvaluationCaseList(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "test cases per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous case list (default: first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every test-case page instead of one (maximum 10,000 cases)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON includes case inputs, outputs, metrics and errors)")
	return cmd
}

func runEvaluationCaseList(ctx context.Context, opts Options, workflowID, runID string, f evaluationCaseListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateEvaluationArguments(workflowID, runID); err != nil {
		return err
	}
	listOpts := n8n.ListEvaluationCasesOptions{ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pageOpts n8n.ListOptions) (n8n.Page[n8n.EvaluationTestCase], error) {
		return client.ListEvaluationCases(ctx, workflowID, runID, n8n.ListEvaluationCasesOptions{ListOptions: pageOpts})
	}
	var page n8n.Page[n8n.EvaluationTestCase]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return evaluationAPIError(err, resolution, workflowID, runID, "list cases for")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeEvaluationCaseList(opts, resolution, workflowID, runID, page)
}

func writeEvaluationCaseList(opts Options, resolution config.Resolution, workflowID, runID string, page n8n.Page[n8n.EvaluationTestCase]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", workflowID)
	fmt.Fprintf(tw, "Evaluation run:\t%s\n", runID)
	fmt.Fprintf(tw, "Test cases:\t%d\n", len(page.Data))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tSTATUS\tEXECUTION\tSTARTED\tCOMPLETED\tERROR")
		for _, testCase := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", emptyDash(testCase.ID), emptyDash(testCase.Status),
				optionalText(testCase.ExecutionID), optionalText(testCase.RunAt), optionalText(testCase.CompletedAt), optionalText(testCase.ErrorCode))
		}
	}
	if page.HasMore() {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	return tw.Flush()
}

func validateEvaluationArguments(workflowID, runID string) error {
	if err := validateEvaluationArgument("workflow ID", workflowID); err != nil {
		return err
	}
	return validateEvaluationArgument("evaluation run ID", runID)
}

func validateEvaluationArgument(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	return nil
}

func evaluationAPIError(err error, resolution config.Resolution, workflowID, runID, action string) error {
	switch {
	case n8n.IsStatus(err, http.StatusPaymentRequired):
		return fmt.Errorf("%s cannot use evaluations (402): this instance does not license the feature; check the instance license and run %s", resolution.URL, discoverHint(evaluationResource))
	case n8n.IsForbidden(err):
		if action == "create" || action == "cancel" {
			return fmt.Errorf("%s denied evaluation %s (403): the credential needs the test-run scope and workflow:execute permission in workflow %q's project; run %s", resolution.URL, action, workflowID, discoverHint(evaluationResource))
		}
		return fmt.Errorf("%s denied evaluation %s (403): the credential lacks the test-run scope or access to workflow %q's project; run %s", resolution.URL, action, workflowID, discoverHint(evaluationResource))
	case n8n.IsNotFound(err):
		if runID == "" {
			return fmt.Errorf("%s has no accessible workflow %q (404): check it with 'n8n workflow list'", resolution.URL, workflowID)
		}
		return fmt.Errorf("%s has no evaluation run %q for workflow %q (404): find it with 'n8n evaluation list %s'", resolution.URL, runID, workflowID, workflowID)
	case n8n.IsConflict(err):
		if action == "cancel" {
			return fmt.Errorf("%s cannot cancel evaluation run %q (409): it already finished, was cancelled, or is otherwise not cancellable; inspect it with 'n8n evaluation get %s %s'", resolution.URL, runID, workflowID, runID)
		}
		return fmt.Errorf("%s cannot %s an evaluation for workflow %q (409): its evaluation configuration or current run state conflicts; inspect runs with 'n8n evaluation list %s'", resolution.URL, action, workflowID, workflowID)
	default:
		return apiError(err, resolution, evaluationResource)
	}
}
