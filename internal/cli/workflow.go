package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	// workflowResource is the 'n8n discover' key for workflows.
	workflowResource = "workflow"
	// maxWorkflowDocument caps a workflow JSON payload read from a file or
	// stdin. Workflows carrying pinned sample data are the large ones, which is
	// why this is well above the 1 MiB other resources allow.
	maxWorkflowDocument = 8 << 20
)

func newWorkflowCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage workflows, their versions and their tags",
		Long: "Manage n8n workflows: list and read them, create and replace their definitions,\n" +
			"publish and unpublish them, archive, transfer and delete them, and inspect the\n" +
			"version history under the 'version' subgroup and the tags under the 'tag'\n" +
			"subgroup.\n\n" +
			"Start with 'list' to find workflow IDs. The edit workflow is read, edit, write:\n" +
			"'get ID --output json' writes the definition, 'update ID --input FILE' sends it\n" +
			"back. 'update' is a full replacement, so always start from a fresh 'get'.\n\n" +
			"Across two saved contexts, 'diff' compares saved definitions without writing\n" +
			"and 'copy' writes one of them onto the other instance.\n\n" +
			"Publishing is what n8n v1 called activating: a published workflow runs its\n" +
			"triggers in production. 'archive' is the reversible soft delete and 'delete' is\n" +
			"permanent; delete, unpublish and transfer ask for confirmation.",
		Example: "  n8n workflow list --active true\n" +
			"  n8n workflow get WORKFLOW_ID --output json > workflow.json\n" +
			"  n8n workflow copy --name \"Invoice sync\" --from-context staging --to-context prod\n" +
			"  n8n workflow update WORKFLOW_ID --input workflow.json\n" +
			"  n8n workflow publish WORKFLOW_ID\n" +
			"  n8n workflow history WORKFLOW_ID\n" +
			"  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID\n" +
			"  n8n workflow delete WORKFLOW_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newWorkflowListCommand(opts),
		newWorkflowGetCommand(opts),
		newWorkflowDiffCommand(opts),
		newWorkflowCopyCommand(opts),
		newWorkflowCreateCommand(opts),
		newWorkflowUpdateCommand(opts),
		newWorkflowDeleteCommand(opts),
		newWorkflowArchiveCommand(opts),
		newWorkflowUnarchiveCommand(opts),
		newWorkflowPublishCommand(opts),
		newWorkflowUnpublishCommand(opts),
		newWorkflowTransferCommand(opts),
		newWorkflowHistoryCommand(opts),
		newWorkflowVersionCommand(opts),
		newWorkflowTagCommand(opts),
		newWorkflowActivateCommand(opts),
		newWorkflowDeactivateCommand(opts),
	)
	return cmd
}

type workflowListFlags struct {
	instance    instanceFlags
	limit       int
	cursor      string
	offset      int
	all         bool
	active      string
	tags        []string
	name        string
	projectID   string
	excludePins bool
	output      string
}

func newWorkflowListCommand(opts Options) *cobra.Command {
	var f workflowListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		Long: "List the workflows the credential can see, one cursor-paginated page at a time.\n" +
			"Pass a returned cursor to --cursor for the next page, or use --all to follow\n" +
			"every cursor, capped at 10,000 workflows.\n\n" +
			"Use this to find the workflow ID every other command takes. Narrow the result\n" +
			"set with --active, --name, --tag and --project-id. Each row carries the full\n" +
			"definition, so on a large instance prefer --exclude-pinned-data, which drops\n" +
			"the pinned sample data that dominates the response. Requires workflow:list.",
		Example: "  n8n workflow list\n" +
			"  n8n workflow list --active true --limit 50\n" +
			"  n8n workflow list --tag production --tag finance\n" +
			"  n8n workflow list --project-id PROJECT_ID --exclude-pinned-data\n" +
			"  n8n workflow list --all --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkflowList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "workflows per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().IntVar(&f.offset, "offset", 0, "number of workflows to skip before the page (default: start at the first workflow)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 workflows)")
	cmd.Flags().StringVar(&f.active, "active", "", "only published (true) or unpublished (false) workflows (default: both)")
	cmd.Flags().StringArrayVar(&f.tags, "tag", nil, "only workflows carrying this tag name, repeatable and combined with AND (default: any tag)")
	cmd.Flags().StringVar(&f.name, "name", "", "only workflows with exactly this name (default: any name)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "only workflows in this project, from 'n8n project list' (default: every project)")
	cmd.Flags().BoolVar(&f.excludePins, "exclude-pinned-data", false, "leave pinned sample data out of the response (default: include it)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runWorkflowList(ctx context.Context, opts Options, f workflowListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	active, err := workflowActiveFilter(f.active)
	if err != nil {
		return err
	}
	listOpts := n8n.ListWorkflowsOptions{
		ListOptions:       n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Offset:            f.offset,
		Active:            active,
		Tags:              f.tags,
		Name:              f.name,
		ProjectID:         f.projectID,
		ExcludePinnedData: f.excludePins,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.Workflow], error) {
		next := listOpts
		next.ListOptions = pagination
		return client.ListWorkflows(ctx, next)
	}
	var page n8n.Page[n8n.Workflow]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return workflowAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeWorkflowList(opts, resolution, page)
}

// workflowActiveFilter turns the tri-state --active flag into the optional
// boolean the API takes.
func workflowActiveFilter(value string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return nil, nil
	case "true":
		active := true
		return &active, nil
	case "false":
		active := false
		return &active, nil
	default:
		return nil, fmt.Errorf("unknown --active value %q: use true or false, or leave the flag off for both", value)
	}
}

func writeWorkflowList(opts Options, resolution config.Resolution, page n8n.Page[n8n.Workflow]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Workflows:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tPUBLISHED\tARCHIVED\tNODES\tTRIGGERS\tTAGS\tUPDATED")
		for _, workflow := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%t\t%t\t%d\t%d\t%s\t%s\n",
				emptyDash(workflow.ID), emptyDash(workflow.Name), workflow.Active, workflow.IsArchived,
				workflow.NodeCount(), workflow.TriggerCount, emptyDash(workflowTagNames(workflow.Tags)),
				emptyDash(workflow.UpdatedAt))
		}
	}
	if page.HasMore() {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No workflows were returned. Widen the filters, or create one with 'n8n workflow create --name NAME'.")
	}
	return nil
}

func workflowTagNames(tags []n8n.Tag) string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return strings.Join(names, ", ")
}

type workflowGetFlags struct {
	instance    instanceFlags
	excludePins bool
	output      string
}

func newWorkflowGetCommand(opts Options) *cobra.Command {
	var f workflowGetFlags
	cmd := &cobra.Command{
		Use:   "get <workflow-id>",
		Short: "Show one workflow with its definition",
		Long: "Show one workflow: its state, its tags and its full node definition.\n\n" +
			"'--output json' is the faithful copy of the workflow and the first half of the\n" +
			"edit workflow: redirect it to a file, edit the file, then send it back with\n" +
			"'n8n workflow update ID --input FILE'. Text output summarizes instead, listing\n" +
			"the nodes rather than their parameters.\n\n" +
			"Use --exclude-pinned-data when the pinned sample data is not wanted; note that\n" +
			"a document saved that way no longer carries it, so updating from it clears the\n" +
			"pinned data on the server. Requires workflow:read.",
		Example: "  n8n workflow get WORKFLOW_ID\n" +
			"  n8n workflow get WORKFLOW_ID --output json > workflow.json\n" +
			"  n8n workflow get WORKFLOW_ID --exclude-pinned-data --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.excludePins, "exclude-pinned-data", false, "leave pinned sample data out of the response (default: include it)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the whole workflow, suitable for --input)")
	return cmd
}

func runWorkflowGet(ctx context.Context, opts Options, id string, f workflowGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	workflow, err := client.GetWorkflow(ctx, id, f.excludePins)
	if err != nil {
		return workflowAPIError(err, resolution, "read")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, workflow)
	}
	return writeWorkflowDetail(opts, resolution, workflow)
}

func writeWorkflowDetail(opts Options, resolution config.Resolution, workflow *n8n.Workflow) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", emptyDash(workflow.Name))
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(workflow.ID))
	fmt.Fprintf(tw, "Published:\t%t\n", workflow.Active)
	fmt.Fprintf(tw, "Archived:\t%t\n", workflow.IsArchived)
	fmt.Fprintf(tw, "Version:\t%s\n", emptyDash(workflow.VersionID))
	fmt.Fprintf(tw, "Published version:\t%s\n", emptyDash(workflow.ActiveVersionID))
	fmt.Fprintf(tw, "Nodes:\t%d\n", workflow.NodeCount())
	fmt.Fprintf(tw, "Triggers:\t%d\n", workflow.TriggerCount)
	fmt.Fprintf(tw, "Tags:\t%s\n", emptyDash(workflowTagNames(workflow.Tags)))
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(workflow.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", emptyDash(workflow.UpdatedAt))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if nodes := workflow.NodeSummaries(); len(nodes) > 0 {
		fmt.Fprintln(tw, "\nNODE\tTYPE\tDISABLED")
		for _, node := range nodes {
			fmt.Fprintf(tw, "%s\t%s\t%t\n", emptyDash(node.Name), emptyDash(node.Type), node.Disabled)
		}
	}
	return tw.Flush()
}

type workflowCreateFlags struct {
	instance  instanceFlags
	input     string
	name      string
	projectID string
	folderID  string
	output    string
}

func newWorkflowCreateCommand(opts Options) *cobra.Command {
	var f workflowCreateFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workflow from a definition or as an empty one",
		Long: "Create one workflow. With --input the definition is read from a JSON file or\n" +
			"from stdin ('--input -'); without it, --name creates an empty workflow with no\n" +
			"nodes, ready to be edited in the UI or filled in by a later update.\n\n" +
			"The input may be a document produced by 'n8n workflow get --output json': the\n" +
			"read-only fields (id, versionId, active, timestamps, tags, sharing) are dropped\n" +
			"before sending, because the API answers a read-only field with 400, and the\n" +
			"dropped names are reported on stderr. --name, --project-id and\n" +
			"--parent-folder-id override what the document says.\n\n" +
			"A created workflow is never published: publish it with 'n8n workflow publish'.\n" +
			"Requires workflow:create.",
		Example: "  n8n workflow create --name \"Invoice sync\"\n" +
			"  n8n workflow create --input workflow.json\n" +
			"  n8n workflow create --input - < workflow.json\n" +
			"  n8n workflow create --input workflow.json --name \"Invoice sync copy\" --project-id PROJECT_ID\n" +
			"  n8n workflow create --name Scratch --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkflowCreate(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "workflow definition as JSON: a file path, or - for stdin (default: an empty workflow built from --name)")
	cmd.Flags().StringVar(&f.name, "name", "", "workflow name; overrides the name in --input (required when --input is absent)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "project to create the workflow in, from 'n8n project list' (default: the caller's personal project)")
	cmd.Flags().StringVar(&f.folderID, "parent-folder-id", "", "folder to create the workflow in, from 'n8n folder list PROJECT_ID' (default: the project root)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the created workflow)")
	return cmd
}

func runWorkflowCreate(ctx context.Context, opts Options, f workflowCreateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	doc, err := workflowCreateDocument(opts, f)
	if err != nil {
		return err
	}
	if err := doc.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	created, err := client.CreateWorkflow(ctx, doc)
	if err != nil {
		return workflowAPIError(err, resolution, "create")
	}
	return writeWorkflowResult(opts, resolution, "Created", created, f.output)
}

// workflowCreateDocument builds the create body from --input, the empty
// scaffold, and the overriding flags.
func workflowCreateDocument(opts Options, f workflowCreateFlags) (n8n.WorkflowDocument, error) {
	var doc n8n.WorkflowDocument
	if f.input == "" {
		if strings.TrimSpace(f.name) == "" {
			return nil, fmt.Errorf("--input or --name is required: --input sends a definition, --name creates an empty workflow")
		}
		doc = n8n.WorkflowDocument{
			"nodes":       json.RawMessage(`[]`),
			"connections": json.RawMessage(`{}`),
			"settings":    json.RawMessage(`{"executionOrder":"v1"}`),
		}
	} else {
		read, err := readWorkflowDocument(opts, f.input)
		if err != nil {
			return nil, err
		}
		doc = read
	}

	doc, dropped := doc.ForCreate()
	reportDroppedWorkflowFields(opts, dropped)

	for field, value := range map[string]string{
		"name":           f.name,
		"projectId":      f.projectID,
		"parentFolderId": f.folderID,
	} {
		if value == "" {
			continue
		}
		if err := doc.Set(field, value); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

type workflowUpdateFlags struct {
	instance  instanceFlags
	input     string
	name      string
	noPublish bool
	output    string
}

func newWorkflowUpdateCommand(opts Options) *cobra.Command {
	var f workflowUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <workflow-id>",
		Short: "Replace the definition of a workflow",
		Long: "Replace one workflow's definition with the document given by --input, a JSON\n" +
			"file or stdin ('--input -').\n\n" +
			"This is a full replacement, not a patch: nodes, connections, settings and\n" +
			"pinned data that the document leaves out are cleared on the server. Always\n" +
			"start from a fresh read:\n" +
			"  n8n workflow get ID --output json > workflow.json\n" +
			"  n8n workflow update ID --input workflow.json\n" +
			"Read-only fields in the document (id, versionId, active, timestamps, tags,\n" +
			"sharing) are dropped before sending and reported on stderr, because the API\n" +
			"answers a read-only field with 400.\n\n" +
			"A published workflow is republished with the new version unless --no-publish is\n" +
			"given. Republishing additionally needs the workflow:activate scope and the\n" +
			"project's workflow:publish permission; without them the new version is still\n" +
			"saved as a draft and the command fails with 403, leaving the published version\n" +
			"live. Requires workflow:update.",
		Example: "  n8n workflow get WORKFLOW_ID --output json > workflow.json\n" +
			"  n8n workflow update WORKFLOW_ID --input workflow.json\n" +
			"  n8n workflow update WORKFLOW_ID --input - < workflow.json\n" +
			"  n8n workflow update WORKFLOW_ID --input workflow.json --no-publish\n" +
			"  n8n workflow update WORKFLOW_ID --input workflow.json --name \"Invoice sync v2\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "workflow definition as JSON: a file path, or - for stdin (required)")
	cmd.Flags().StringVar(&f.name, "name", "", "workflow name; overrides the name in --input (default: the name in the document)")
	cmd.Flags().BoolVar(&f.noPublish, "no-publish", false, "save the change as a draft instead of republishing a published workflow (default: republish)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the updated workflow)")
	return cmd
}

func runWorkflowUpdate(ctx context.Context, opts Options, id string, f workflowUpdateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	if f.input == "" {
		return fmt.Errorf("--input is required: update replaces the whole definition, so start from 'n8n workflow get %s --output json'", id)
	}
	doc, err := readWorkflowDocument(opts, f.input)
	if err != nil {
		return err
	}
	doc, dropped := doc.ForUpdate()
	reportDroppedWorkflowFields(opts, dropped)
	if f.name != "" {
		if err := doc.Set("name", f.name); err != nil {
			return err
		}
	}
	if err := doc.Validate(); err != nil {
		return err
	}

	updateOpts := n8n.UpdateWorkflowOptions{}
	if f.noPublish {
		publish := false
		updateOpts.PublishIfActive = &publish
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	updated, err := client.UpdateWorkflow(ctx, id, doc, updateOpts)
	if err != nil {
		if n8n.IsForbidden(err) && !f.noPublish {
			return fmt.Errorf("%s denied publishing the update to workflow %q (403): republishing needs the workflow:activate scope and the project's workflow:publish permission. "+
				"If the credential may update but not publish, the new version was saved as a draft and the previously published version is still live: check with 'n8n workflow get %s'. "+
				"Re-run with --no-publish to save a draft without this error",
				resolution.URL, id, id)
		}
		return workflowAPIError(err, resolution, "update")
	}
	return writeWorkflowResult(opts, resolution, "Updated", updated, f.output)
}

type workflowActionFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newWorkflowDeleteCommand(opts Options) *cobra.Command {
	var f workflowActionFlags
	cmd := &cobra.Command{
		Use:   "delete <workflow-id>",
		Short: "Permanently delete a workflow",
		Long: "Delete one workflow for good, together with its version history. This cannot be\n" +
			"undone, and the executions it produced lose the definition they refer to.\n\n" +
			"Prefer 'n8n workflow archive' when the workflow may be wanted later: archiving\n" +
			"hides it and is reversible with 'n8n workflow unarchive'. Back the definition\n" +
			"up first with 'n8n workflow get ID --output json > workflow.json'.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires workflow:delete.",
		Example: "  n8n workflow delete WORKFLOW_ID\n" +
			"  n8n workflow delete WORKFLOW_ID --yes\n" +
			"  n8n workflow delete WORKFLOW_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowAction(cmd.Context(), opts, args[0], f, workflowAction{
				name:    "delete",
				past:    "Deleted",
				confirm: "Permanently delete workflow %q on %s? Its version history goes with it and this cannot be undone. Use 'n8n workflow archive' for a reversible soft delete.",
				call: func(ctx context.Context, client *n8n.Client, id string) (*n8n.Workflow, error) {
					return client.DeleteWorkflow(ctx, id)
				},
			})
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the workflow as the server last saw it)")
	return cmd
}

func newWorkflowArchiveCommand(opts Options) *cobra.Command {
	var f workflowActionFlags
	cmd := &cobra.Command{
		Use:   "archive <workflow-id>",
		Short: "Archive a workflow, the reversible soft delete",
		Long: "Archive one workflow. The workflow is hidden from the normal workflow list and\n" +
			"stops being offered for editing, but nothing is destroyed: its definition,\n" +
			"version history and executions stay, and 'n8n workflow unarchive' brings it\n" +
			"back.\n\n" +
			"Archiving is idempotent: archiving an already archived workflow returns it\n" +
			"unchanged. A published workflow should be unpublished first, so its triggers\n" +
			"stop running. Requires workflow:delete.",
		Example: "  n8n workflow archive WORKFLOW_ID\n" +
			"  n8n workflow archive WORKFLOW_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowAction(cmd.Context(), opts, args[0], f, workflowAction{
				name: "archive",
				past: "Archived",
				call: func(ctx context.Context, client *n8n.Client, id string) (*n8n.Workflow, error) {
					return client.ArchiveWorkflow(ctx, id)
				},
			})
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the archived workflow)")
	return cmd
}

func newWorkflowUnarchiveCommand(opts Options) *cobra.Command {
	var f workflowActionFlags
	cmd := &cobra.Command{
		Use:   "unarchive <workflow-id>",
		Short: "Restore an archived workflow",
		Long: "Restore one archived workflow, undoing 'n8n workflow archive'. The workflow\n" +
			"becomes visible and editable again.\n\n" +
			"Restoring does not publish it: a workflow that was published before archiving\n" +
			"comes back unpublished, so run 'n8n workflow publish' when its triggers should\n" +
			"run again. Requires workflow:delete.",
		Example: "  n8n workflow unarchive WORKFLOW_ID\n" +
			"  n8n workflow unarchive WORKFLOW_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowAction(cmd.Context(), opts, args[0], f, workflowAction{
				name: "unarchive",
				past: "Unarchived",
				call: func(ctx context.Context, client *n8n.Client, id string) (*n8n.Workflow, error) {
					return client.UnarchiveWorkflow(ctx, id)
				},
			})
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the restored workflow)")
	return cmd
}

func newWorkflowUnpublishCommand(opts Options) *cobra.Command {
	var f workflowActionFlags
	cmd := &cobra.Command{
		Use:   "unpublish <workflow-id>",
		Short: "Take the published version of a workflow offline",
		Long: "Unpublish one workflow, which n8n v1 called deactivating it. Its triggers stop\n" +
			"firing immediately: schedules stop running and webhook URLs stop responding, so\n" +
			"anything calling them starts failing.\n\n" +
			"Nothing is deleted and the definition is untouched, so 'n8n workflow publish'\n" +
			"puts it back live. Interactive runs ask for confirmation, because this stops\n" +
			"production automation; non-interactive runs require --yes. Requires\n" +
			"workflow:deactivate.",
		Example: "  n8n workflow unpublish WORKFLOW_ID\n" +
			"  n8n workflow unpublish WORKFLOW_ID --yes\n" +
			"  n8n workflow unpublish WORKFLOW_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowAction(cmd.Context(), opts, args[0], f, workflowAction{
				name:    "unpublish",
				past:    "Unpublished",
				confirm: "Unpublish workflow %q on %s? Its schedules stop running and its webhook URLs stop responding until it is published again.",
				call: func(ctx context.Context, client *n8n.Client, id string) (*n8n.Workflow, error) {
					return client.UnpublishWorkflow(ctx, id)
				},
			})
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm taking the workflow offline without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the unpublished workflow)")
	return cmd
}

func newWorkflowDeactivateCommand(opts Options) *cobra.Command {
	var f workflowActionFlags
	cmd := &cobra.Command{
		Use:    "deactivate <workflow-id>",
		Short:  "Deprecated alias for 'n8n workflow unpublish'",
		Hidden: true,
		Long: "Deprecated compatibility alias. It calls POST /workflows/{id}/deactivate, the\n" +
			"deprecated route the API still serves, and does exactly what\n" +
			"'n8n workflow unpublish' does through POST /workflows/{id}/unpublish.\n\n" +
			"Use 'n8n workflow unpublish' instead; this command exists only for scripts\n" +
			"written against an older instance and may stop working when the route is\n" +
			"removed. Requires workflow:deactivate.",
		Example: "  n8n workflow deactivate WORKFLOW_ID --yes",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(opts.Streams.Err, "'n8n workflow deactivate' is deprecated: use 'n8n workflow unpublish'.")
			return runWorkflowAction(cmd.Context(), opts, args[0], f, workflowAction{
				name:    "unpublish",
				past:    "Unpublished",
				confirm: "Unpublish workflow %q on %s? Its schedules stop running and its webhook URLs stop responding until it is published again.",
				call: func(ctx context.Context, client *n8n.Client, id string) (*n8n.Workflow, error) {
					return client.DeactivateWorkflow(ctx, id)
				},
			})
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm taking the workflow offline without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the unpublished workflow)")
	return cmd
}

// workflowAction is one workflow-scoped call that answers with the workflow.
type workflowAction struct {
	// name is the verb used in error messages and scope hints.
	name string
	// past is the label of the text result, e.g. "Archived".
	past string
	// confirm is a format string taking the workflow ID and the instance URL.
	// Empty means the action runs without confirmation.
	confirm string
	call    func(context.Context, *n8n.Client, string) (*n8n.Workflow, error)
}

func runWorkflowAction(ctx context.Context, opts Options, id string, f workflowActionFlags, action workflowAction) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if action.confirm != "" {
		if err := opts.confirmer(f.yes).Confirm(fmt.Sprintf(action.confirm, id, resolution.URL)); err != nil {
			return err
		}
	}
	workflow, err := action.call(ctx, client, id)
	if err != nil {
		return workflowAPIError(err, resolution, action.name)
	}
	return writeWorkflowResult(opts, resolution, action.past, workflow, f.output)
}

type workflowPublishFlags struct {
	instance    instanceFlags
	versionID   string
	name        string
	description string
	output      string
}

func newWorkflowPublishCommand(opts Options) *cobra.Command {
	var f workflowPublishFlags
	cmd := &cobra.Command{
		Use:   "publish <workflow-id>",
		Short: "Publish a workflow so its triggers run",
		Long: "Publish one workflow, which n8n v1 called activating it. The published version\n" +
			"goes live: schedule triggers start running on their schedule and webhook URLs\n" +
			"start accepting requests, in production.\n\n" +
			"Without --version-id the latest saved version is published; pass a version ID\n" +
			"from 'n8n workflow history' to publish an older one instead. --name and\n" +
			"--description label the published version and do not rename the workflow.\n\n" +
			"A workflow with no trigger node cannot be published, and a publication blocked\n" +
			"by an open workflow review or by a webhook path already in use answers 409 with\n" +
			"the reason. Reverse it with 'n8n workflow unpublish'. Requires\n" +
			"workflow:activate.",
		Example: "  n8n workflow publish WORKFLOW_ID\n" +
			"  n8n workflow publish WORKFLOW_ID --version-id VERSION_ID\n" +
			"  n8n workflow publish WORKFLOW_ID --name \"Release 2026-09-17\" --description \"adds the retry branch\"\n" +
			"  n8n workflow publish WORKFLOW_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowPublish(cmd.Context(), opts, args[0], f, false)
		},
	}
	f.instance.register(cmd)
	registerWorkflowPublishFlags(cmd, &f)
	return cmd
}

func newWorkflowActivateCommand(opts Options) *cobra.Command {
	var f workflowPublishFlags
	cmd := &cobra.Command{
		Use:    "activate <workflow-id>",
		Short:  "Deprecated alias for 'n8n workflow publish'",
		Hidden: true,
		Long: "Deprecated compatibility alias. It calls POST /workflows/{id}/activate, the\n" +
			"deprecated route the API still serves, and does exactly what\n" +
			"'n8n workflow publish' does through POST /workflows/{id}/publish.\n\n" +
			"Use 'n8n workflow publish' instead; this command exists only for scripts\n" +
			"written against an older instance and may stop working when the route is\n" +
			"removed. Requires workflow:activate.",
		Example: "  n8n workflow activate WORKFLOW_ID",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(opts.Streams.Err, "'n8n workflow activate' is deprecated: use 'n8n workflow publish'.")
			return runWorkflowPublish(cmd.Context(), opts, args[0], f, true)
		},
	}
	f.instance.register(cmd)
	registerWorkflowPublishFlags(cmd, &f)
	return cmd
}

func registerWorkflowPublishFlags(cmd *cobra.Command, f *workflowPublishFlags) {
	cmd.Flags().StringVar(&f.versionID, "version-id", "", "version to publish, from 'n8n workflow history' (default: the latest saved version)")
	cmd.Flags().StringVar(&f.name, "name", "", "label for the published version; does not rename the workflow (default: the version's existing name)")
	cmd.Flags().StringVar(&f.description, "description", "", "description for the published version (default: none)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the published workflow)")
}

func runWorkflowPublish(ctx context.Context, opts Options, id string, f workflowPublishFlags, deprecated bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	request := n8n.PublishWorkflowRequest{
		VersionID:   f.versionID,
		Name:        f.name,
		Description: f.description,
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	call := client.PublishWorkflow
	if deprecated {
		call = client.ActivateWorkflow
	}
	workflow, err := call(ctx, id, request)
	if err != nil {
		if n8n.IsConflict(err) {
			return fmt.Errorf("%s refused to publish workflow %q (409): an open workflow review or a webhook path already in use is blocking it: %w", resolution.URL, id, err)
		}
		return workflowAPIError(err, resolution, "publish")
	}
	return writeWorkflowResult(opts, resolution, "Published", workflow, f.output)
}

type workflowTransferFlags struct {
	instance    instanceFlags
	destination string
	yes         bool
	output      string
}

func newWorkflowTransferCommand(opts Options) *cobra.Command {
	var f workflowTransferFlags
	cmd := &cobra.Command{
		Use:   "transfer <workflow-id>",
		Short: "Move a workflow to another project",
		Long: "Move one workflow into another project, given by --destination-project-id.\n\n" +
			"Access follows the project, so everyone with access to the old project loses it\n" +
			"and everyone with access to the new one gains it. Credentials do not move with\n" +
			"the workflow: a credential that is not shared with the destination project\n" +
			"stops resolving, which makes a published workflow fail at run time. Check the\n" +
			"credentials first with 'n8n credential list'.\n\n" +
			"The API answers with no content, so the workflow is read back afterwards to\n" +
			"report where it ended up. Interactive runs ask for confirmation;\n" +
			"non-interactive runs require --yes. Requires workflow:move.",
		Example: "  n8n workflow transfer WORKFLOW_ID --destination-project-id PROJECT_ID\n" +
			"  n8n workflow transfer WORKFLOW_ID --destination-project-id PROJECT_ID --yes\n" +
			"  n8n workflow transfer WORKFLOW_ID --destination-project-id PROJECT_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowTransfer(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.destination, "destination-project-id", "", "project to move the workflow into, from 'n8n project list' (required)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the move without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON names the workflow, the destination project and the action)")
	return cmd
}

func runWorkflowTransfer(ctx context.Context, opts Options, id string, f workflowTransferFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	if err := validateWorkflowArgument("destination project ID", f.destination); err != nil {
		return fmt.Errorf("%w: pass --destination-project-id, from 'n8n project list'", err)
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Move workflow %q into project %q on %s? Access follows the project, and credentials that are not shared with it stop resolving.",
		id, f.destination, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.TransferWorkflow(ctx, id, f.destination); err != nil {
		return workflowAPIError(err, resolution, "transfer")
	}
	result := workflowTransferResult{
		Action:               "transferred",
		WorkflowID:           id,
		DestinationProjectID: f.destination,
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	fmt.Fprintf(tw, "Workflow:\t%s\n", result.WorkflowID)
	fmt.Fprintf(tw, "Destination project:\t%s\n", result.DestinationProjectID)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type workflowTransferResult struct {
	Action               string `json:"action"`
	WorkflowID           string `json:"workflowId"`
	DestinationProjectID string `json:"destinationProjectId"`
}

type workflowHistoryFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newWorkflowHistoryCommand(opts Options) *cobra.Command {
	var f workflowHistoryFlags
	cmd := &cobra.Command{
		Use:   "history <workflow-id>",
		Short: "List the saved versions of a workflow",
		Long: "List the version history of one workflow, newest first, one cursor-paginated\n" +
			"page at a time. Each entry is metadata only: the version ID, who saved it, and\n" +
			"when.\n\n" +
			"Use it to find the version ID that 'n8n workflow version get' reads and that\n" +
			"'n8n workflow publish --version-id' puts live. Pass a returned cursor to\n" +
			"--cursor for the next page, or use --all to follow every cursor, capped at\n" +
			"10,000 versions. Requires workflow:read.",
		Example: "  n8n workflow history WORKFLOW_ID\n" +
			"  n8n workflow history WORKFLOW_ID --limit 5\n" +
			"  n8n workflow history WORKFLOW_ID --all --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowHistory(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "versions per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous page (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 versions)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runWorkflowHistory(ctx context.Context, opts Options, id string, f workflowHistoryFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	listOpts := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.WorkflowVersionSummary], error) {
		return client.ListWorkflowHistory(ctx, id, pagination)
	}
	var page n8n.Page[n8n.WorkflowVersionSummary]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts)
	}
	if err != nil {
		return workflowAPIError(err, resolution, "read")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", id)
	fmt.Fprintf(tw, "Versions:\t%d\n", len(page.Data))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nVERSION ID\tNAME\tAUTHORS\tCREATED\tUPDATED")
		for _, version := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				emptyDash(version.VersionID), emptyDash(version.Name), emptyDash(version.Authors),
				emptyDash(version.CreatedAt), emptyDash(version.UpdatedAt))
		}
	}
	if page.HasMore() {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "This workflow has no saved version history.")
	}
	return nil
}

func newWorkflowVersionCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Read a stored version of a workflow",
		Long: "Read one stored version of a workflow definition. Versions are listed by\n" +
			"'n8n workflow history', which reports the version IDs this group takes.\n\n" +
			"Start with 'n8n workflow history WORKFLOW_ID', then 'version get' to see what\n" +
			"a version contains. Publishing an older version is 'n8n workflow publish\n" +
			"--version-id'; restoring one as the current draft means sending its nodes and\n" +
			"connections through 'n8n workflow update'.",
		Example: "  n8n workflow history WORKFLOW_ID\n" +
			"  n8n workflow version get WORKFLOW_ID VERSION_ID\n" +
			"  n8n workflow version get WORKFLOW_ID VERSION_ID --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newWorkflowVersionGetCommand(opts),
		newWorkflowVersionGetLegacyCommand(opts),
	)
	return cmd
}

type workflowVersionFlags struct {
	instance instanceFlags
	output   string
}

func newWorkflowVersionGetCommand(opts Options) *cobra.Command {
	var f workflowVersionFlags
	cmd := &cobra.Command{
		Use:   "get <workflow-id> <version-id>",
		Short: "Show one stored version of a workflow",
		Long: "Show one stored version of a workflow: who saved it, when, and the nodes and\n" +
			"connections it holds.\n\n" +
			"Version IDs come from 'n8n workflow history WORKFLOW_ID'. The response is the\n" +
			"stored definition, not a workflow object, so it cannot be fed to\n" +
			"'n8n workflow update' as it stands. Requires workflow:read.",
		Example: "  n8n workflow version get WORKFLOW_ID VERSION_ID\n" +
			"  n8n workflow version get WORKFLOW_ID VERSION_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowVersionGet(cmd.Context(), opts, args[0], args[1], f, false)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the stored version with its nodes)")
	return cmd
}

func newWorkflowVersionGetLegacyCommand(opts Options) *cobra.Command {
	var f workflowVersionFlags
	cmd := &cobra.Command{
		Use:    "get-legacy <workflow-id> <version-id>",
		Short:  "Deprecated alias for 'n8n workflow version get'",
		Hidden: true,
		Long: "Deprecated compatibility alias. It reads the version through\n" +
			"GET /workflows/{id}/{versionId}, the deprecated route the API still serves,\n" +
			"instead of GET /workflows/{id}/versions/{versionId}.\n\n" +
			"Use 'n8n workflow version get' instead; this command exists only for instances\n" +
			"too old to serve the current route and may stop working when the deprecated one\n" +
			"is removed. Requires workflow:read.",
		Example: "  n8n workflow version get-legacy WORKFLOW_ID VERSION_ID",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(opts.Streams.Err, "'n8n workflow version get-legacy' uses a deprecated API route: use 'n8n workflow version get'.")
			return runWorkflowVersionGet(cmd.Context(), opts, args[0], args[1], f, true)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the stored version with its nodes)")
	return cmd
}

func runWorkflowVersionGet(ctx context.Context, opts Options, workflowID, versionID string, f workflowVersionFlags, legacy bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", workflowID); err != nil {
		return err
	}
	if err := validateWorkflowArgument("version ID", versionID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	read := client.GetWorkflowVersion
	if legacy {
		read = client.GetWorkflowVersionLegacy
	}
	version, err := read(ctx, workflowID, versionID)
	if err != nil {
		if n8n.IsNotFound(err) {
			return fmt.Errorf("%s has no such workflow or version (404): list the versions with 'n8n workflow history %s'", resolution.URL, workflowID)
		}
		return workflowAPIError(err, resolution, "read")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, version)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Version:\t%s\n", emptyDash(version.VersionID))
	fmt.Fprintf(tw, "Workflow:\t%s\n", emptyDash(version.WorkflowID))
	fmt.Fprintf(tw, "Name:\t%s\n", emptyDash(version.Name))
	fmt.Fprintf(tw, "Description:\t%s\n", emptyDash(version.Description))
	fmt.Fprintf(tw, "Authors:\t%s\n", emptyDash(version.Authors))
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(version.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", emptyDash(version.UpdatedAt))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if nodes := workflowVersionNodes(version); len(nodes) > 0 {
		fmt.Fprintln(tw, "\nNODE\tTYPE\tDISABLED")
		for _, node := range nodes {
			fmt.Fprintf(tw, "%s\t%s\t%t\n", emptyDash(node.Name), emptyDash(node.Type), node.Disabled)
		}
	}
	return tw.Flush()
}

// workflowVersionNodes decodes the identifying part of a stored version's
// nodes. Unreadable nodes render as none: this is display detail.
func workflowVersionNodes(version *n8n.WorkflowVersion) []n8n.WorkflowNode {
	if len(version.Nodes) == 0 {
		return nil
	}
	var nodes []n8n.WorkflowNode
	if err := json.Unmarshal(version.Nodes, &nodes); err != nil {
		return nil
	}
	return nodes
}

func newWorkflowTagCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Read and replace the tags of a workflow",
		Long: "Read and replace the tags attached to one workflow. Tags themselves are managed\n" +
			"with 'n8n tag', which is where tag IDs come from; this group only decides which\n" +
			"of them a workflow carries.\n\n" +
			"Start with 'list' to see the current tags, then 'set' to replace them. 'set' is\n" +
			"a replacement, not an addition: whatever is not passed is removed from the\n" +
			"workflow.",
		Example: "  n8n workflow tag list WORKFLOW_ID\n" +
			"  n8n tag list\n" +
			"  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID --tag-id OTHER_TAG_ID\n" +
			"  n8n workflow tag set WORKFLOW_ID --clear",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newWorkflowTagListCommand(opts),
		newWorkflowTagSetCommand(opts),
	)
	return cmd
}

type workflowTagListFlags struct {
	instance instanceFlags
	output   string
}

func newWorkflowTagListCommand(opts Options) *cobra.Command {
	var f workflowTagListFlags
	cmd := &cobra.Command{
		Use:   "list <workflow-id>",
		Short: "List the tags attached to a workflow",
		Long: "List the tags one workflow carries, with their IDs and names. The response is\n" +
			"the whole set, not a page: workflow tags are not paginated.\n\n" +
			"Run this before 'n8n workflow tag set', because setting replaces the whole set\n" +
			"and the current IDs are what a partial change has to repeat. Requires\n" +
			"workflowTags:list.",
		Example: "  n8n workflow tag list WORKFLOW_ID\n" +
			"  n8n workflow tag list WORKFLOW_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowTagList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the array of tags)")
	return cmd
}

func runWorkflowTagList(ctx context.Context, opts Options, id string, f workflowTagListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	tags, err := client.GetWorkflowTags(ctx, id)
	if err != nil {
		return workflowAPIError(err, resolution, "tag read")
	}
	return writeWorkflowTags(opts, resolution, id, tags, f.output)
}

type workflowTagSetFlags struct {
	instance instanceFlags
	tagIDs   []string
	clear    bool
	output   string
}

func newWorkflowTagSetCommand(opts Options) *cobra.Command {
	var f workflowTagSetFlags
	cmd := &cobra.Command{
		Use:   "set <workflow-id>",
		Short: "Replace the tags of a workflow",
		Long: "Replace the whole set of tags on one workflow with the tag IDs given by\n" +
			"--tag-id, or remove every tag with --clear.\n\n" +
			"This is a replacement, not an addition: a tag the workflow carries today and\n" +
			"that is not repeated here is removed from it. To add one tag, run\n" +
			"'n8n workflow tag list WORKFLOW_ID' first and pass the existing IDs along with\n" +
			"the new one. The API takes tag IDs, never tag names; find them with\n" +
			"'n8n tag list'. Requires workflowTags:update.",
		Example: "  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID\n" +
			"  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID --tag-id OTHER_TAG_ID\n" +
			"  n8n workflow tag set WORKFLOW_ID --clear\n" +
			"  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowTagSet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.tagIDs, "tag-id", nil, "tag ID the workflow should carry, repeatable, from 'n8n tag list' (required unless --clear)")
	cmd.Flags().BoolVar(&f.clear, "clear", false, "remove every tag from the workflow (cannot combine with --tag-id)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the resulting array of tags)")
	return cmd
}

func runWorkflowTagSet(ctx context.Context, opts Options, id string, f workflowTagSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateWorkflowArgument("workflow ID", id); err != nil {
		return err
	}
	switch {
	case f.clear && len(f.tagIDs) > 0:
		return fmt.Errorf("--clear cannot be combined with --tag-id: --clear removes every tag, --tag-id names the ones to keep")
	case !f.clear && len(f.tagIDs) == 0:
		return fmt.Errorf("--tag-id or --clear is required: pass the tag IDs the workflow should carry, or --clear to remove them all")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	tags, err := client.SetWorkflowTags(ctx, id, f.tagIDs)
	if err != nil {
		if n8n.IsNotFound(err) {
			return fmt.Errorf("%s has no such workflow or tag (404): check the workflow with 'n8n workflow list' and the tag IDs with 'n8n tag list'", resolution.URL)
		}
		return workflowAPIError(err, resolution, "tag update")
	}
	return writeWorkflowTags(opts, resolution, id, tags, f.output)
}

func writeWorkflowTags(opts Options, resolution config.Resolution, id string, tags []n8n.Tag, output string) error {
	if output == outputJSON {
		if tags == nil {
			tags = []n8n.Tag{}
		}
		return writeJSON(opts.Streams.Out, tags)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", id)
	fmt.Fprintf(tw, "Tags:\t%d\n", len(tags))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(tags) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME")
		for _, tag := range tags {
			fmt.Fprintf(tw, "%s\t%s\n", emptyDash(tag.ID), emptyDash(tag.Name))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(tags) == 0 {
		fmt.Fprintln(opts.Streams.Err, "This workflow carries no tags. Attach one with 'n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID'.")
	}
	return nil
}

func writeWorkflowResult(opts Options, resolution config.Resolution, action string, workflow *n8n.Workflow, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, workflow)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, emptyDash(workflow.Name))
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(workflow.ID))
	fmt.Fprintf(tw, "Published:\t%t\n", workflow.Active)
	fmt.Fprintf(tw, "Archived:\t%t\n", workflow.IsArchived)
	fmt.Fprintf(tw, "Version:\t%s\n", emptyDash(workflow.VersionID))
	fmt.Fprintf(tw, "Nodes:\t%d\n", workflow.NodeCount())
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

// readWorkflowDocument reads and strictly decodes a workflow JSON document from
// a file or stdin, capped so a wrong path cannot pull an arbitrarily large file
// into memory.
func readWorkflowDocument(opts Options, path string) (n8n.WorkflowDocument, error) {
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open workflow input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxWorkflowDocument+1))
	if err != nil {
		return nil, fmt.Errorf("read workflow JSON: %w", err)
	}
	if len(raw) > maxWorkflowDocument {
		return nil, fmt.Errorf("workflow JSON exceeds %d bytes", maxWorkflowDocument)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	var doc n8n.WorkflowDocument
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode workflow JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode workflow JSON: multiple documents are not allowed")
		}
		return nil, fmt.Errorf("decode workflow JSON: %w", err)
	}
	if len(doc) == 0 {
		return nil, fmt.Errorf("the workflow JSON document is empty: it must at least carry %s", strings.Join(n8n.RequiredWorkflowFields, ", "))
	}
	return doc, nil
}

// reportDroppedWorkflowFields tells the user which fields were not sent. A
// definition read from the API always carries read-only fields, so this is the
// normal path, not an error: staying silent would hide what was skipped.
func reportDroppedWorkflowFields(opts Options, dropped []string) {
	if len(dropped) == 0 {
		return
	}
	fmt.Fprintf(opts.Streams.Err, "Not sent (read-only or not part of the write schema): %s\n", strings.Join(dropped, ", "))
}

func validateWorkflowArgument(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	return nil
}

// workflowAPIError explains the denials this resource produces that a bare
// status code does not: workflow scopes are per-action, and a 404 can mean the
// workflow is gone, archived out of view, or in a project the credential cannot
// see.
func workflowAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.IsForbidden(err) {
		return fmt.Errorf("%s denied workflow %s (403): the credential lacks the scope for it, or it has no access to the workflow's project; run %s to inspect access",
			resolution.URL, action, discoverHint(workflowResource))
	}
	if n8n.IsNotFound(err) {
		return fmt.Errorf("%s has no such workflow (404): check the ID with 'n8n workflow list'; an archived workflow is still listed, one in another project may not be visible to this credential", resolution.URL)
	}
	return apiError(err, resolution, workflowResource)
}
