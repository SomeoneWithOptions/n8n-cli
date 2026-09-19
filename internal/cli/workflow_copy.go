package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

type workflowCopyFlags struct {
	from, to    workflowSide
	name        string
	toName      string
	toFolderID  string
	output      string
	publish     bool
	pins, state bool
	createOnly  bool
	updateOnly  bool
	yes         bool
}

func newWorkflowCopyCommand(opts Options) *cobra.Command {
	var f workflowCopyFlags
	cmd := &cobra.Command{
		Use:   "copy",
		Short: "Copy a saved workflow definition from one context to another",
		Long: "Copy one saved workflow definition between two saved contexts, for example\n" +
			"staging to production, or production back to staging for a rollback. The source\n" +
			"instance is only ever read; every write lands on the target.\n\n" +
			"No API operation copies a workflow between instances, so this composes the ones\n" +
			"that exist: it reads the source with 'get' (resolving --name through 'list'),\n" +
			"then creates or fully replaces the target, and publishes it only with --publish.\n\n" +
			"Preview the same pair with 'n8n workflow diff', which writes nothing. 'get' and\n" +
			"'update' are the single-instance edit loop for one context. 'n8n promotion\n" +
			"promote' and 'n8n promotion apply' are a different machine: Git-remote batch\n" +
			"sync of whole projects through a source-control connection, not this.\n\n" +
			"Both contexts must be saved ('n8n auth login --context NAME'); this command uses\n" +
			"each context's saved URL and credential and ignores N8N_URL and environment\n" +
			"credentials on both sides. Needs workflow:read on the source, plus workflow:list\n" +
			"when resolving --name, and workflow:read plus workflow:create and/or\n" +
			"workflow:update on the target. --publish additionally needs workflow:activate\n" +
			"and the project's workflow:publish permission on the target.\n\n" +
			"The target is created when it cannot be resolved by name, and otherwise fully\n" +
			"replaced: nodes, connections and settings the source omits are cleared on the\n" +
			"target. Pass --create-only or --update-only to allow only one of the two. An\n" +
			"existing target keeps its published version live unless --publish is given, so\n" +
			"a copy into production saves a draft by default. Tags, credentials, folders and\n" +
			"sharing do not transfer: their IDs are per instance. Pinned sample data and\n" +
			"runtime static data are excluded unless --include-pinned-data or\n" +
			"--include-static-data is passed. Refuses to run when both contexts resolve to\n" +
			"the same instance, and when either workflow is archived.\n\n" +
			"stdout carries the result only: a text summary, or with --output json a\n" +
			"versioned object with action, from, to, publish, droppedFields and warnings.\n" +
			"Fields not part of the write schema, the warnings and the prompt go to stderr.\n\n" +
			"Verify afterwards with 'n8n workflow diff', and put the copy live with\n" +
			"'n8n workflow publish' when --publish was not used.",
		Example: "  n8n workflow copy --from-context staging --from-id AAA --to-context prod --to-id BBB\n" +
			"  n8n workflow copy --name \"Invoice sync\" --from-context staging --to-context prod --to-project-id PROD --yes --output json\n" +
			"  n8n workflow copy --name \"Invoice sync\" --from-context prod --to-context staging --yes\n" +
			"  n8n workflow copy --from-context staging --from-id AAA --to-context prod --to-name \"Invoice sync (prod)\" --publish --yes",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkflowCopy(cmd.Context(), opts, f)
		},
	}
	cmd.Flags().StringVar(&f.from.context, "from-context", "", "source saved context (required; ignores global environment overrides)")
	cmd.Flags().StringVar(&f.to.context, "to-context", "", "target saved context (required; ignores global environment overrides)")
	cmd.Flags().StringVar(&f.from.id, "from-id", "", "source workflow ID (required unless --name is given)")
	cmd.Flags().StringVar(&f.to.id, "to-id", "", "target workflow ID; omit to resolve the target by name, creating it when absent")
	cmd.Flags().StringVar(&f.name, "name", "", "exact source workflow name; cannot be combined with --from-id")
	cmd.Flags().StringVar(&f.from.projectID, "from-project-id", "", "limit source name lookup to this project (requires --name)")
	cmd.Flags().StringVar(&f.to.projectID, "to-project-id", "", "limit target name lookup to this project, and the destination project when the target is created")
	cmd.Flags().StringVar(&f.toFolderID, "to-parent-folder-id", "", "destination folder when the target is created; rejected when the target exists")
	cmd.Flags().StringVar(&f.toName, "to-name", "", "name to write on the target (default: the source workflow name)")
	cmd.Flags().BoolVar(&f.publish, "publish", false, "publish the target after a successful write (default: write only, the live version is unchanged)")
	cmd.Flags().BoolVar(&f.pins, "include-pinned-data", false, "copy pinned sample data (default: excluded)")
	cmd.Flags().BoolVar(&f.state, "include-static-data", false, "copy runtime static data such as poll cursors (default: excluded)")
	cmd.Flags().BoolVar(&f.createOnly, "create-only", false, "fail instead of replacing an existing target workflow")
	cmd.Flags().BoolVar(&f.updateOnly, "update-only", false, "fail instead of creating a missing target workflow")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the copy without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (versioned copy result)")
	cmd.MarkFlagsMutuallyExclusive("name", "from-id")
	cmd.MarkFlagsMutuallyExclusive("create-only", "update-only")
	return cmd
}

type workflowCopyPublish struct {
	Requested bool `json:"requested"`
	Performed bool `json:"performed"`
}

type workflowCopyResult struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Action        string              `json:"action"`
	From          workflowEndpoint    `json:"from"`
	To            workflowEndpoint    `json:"to"`
	Publish       workflowCopyPublish `json:"publish"`
	Nodes         int                 `json:"nodes"`
	DroppedFields []string            `json:"droppedFields"`
	Warnings      []string            `json:"warnings"`
}

func runWorkflowCopy(ctx context.Context, opts Options, f workflowCopyFlags) error {
	if err := validateCopyFlags(&f); err != nil {
		return err
	}

	// Scope the precedence exception to this command, as 'workflow diff' does.
	// A copy names both instances explicitly; an environment credential
	// silently retargeting one side would write to the wrong instance.
	opts.Env = func(string) string { return "" }
	fromClient, fromResolution, err := opts.apiClient(instanceFlags{context: f.from.context})
	if err != nil {
		return fmt.Errorf("source context %q: %w", f.from.context, err)
	}
	toClient, toResolution, err := opts.apiClient(instanceFlags{context: f.to.context})
	if err != nil {
		return fmt.Errorf("target context %q: %w", f.to.context, err)
	}
	// config.Resolution.URL is already normalized by n8n.NormalizeBaseURL
	// (lowercased scheme and host, no trailing slash), so comparing the
	// strings is comparing the instances. Guard before any transport.
	if fromResolution.URL == toResolution.URL {
		return fmt.Errorf("source and target contexts resolve to the same instance (%s); copy moves a workflow between instances — "+
			"duplicate inside one instance with 'n8n workflow get ID --output json' and 'n8n workflow create --input -'", fromResolution.URL)
	}

	source, err := readWorkflowSide(ctx, fromClient, fromResolution, f.from, f.name, !f.pins)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if source.IsArchived {
		return fmt.Errorf("source workflow %q (%s) is archived; unarchive it with 'n8n workflow unarchive %s --context %s' or copy a different workflow",
			source.Name, source.ID, source.ID, f.from.context)
	}

	targetName := source.Name
	if f.toName != "" {
		targetName = f.toName
	}
	target, err := readCopyTarget(ctx, toClient, toResolution, f, targetName)
	if err != nil {
		return err
	}
	if err := checkCopyTarget(f, target); err != nil {
		return err
	}

	doc, dropped, err := workflowCopyDocument(opts, source, f, targetName, target == nil)
	if err != nil {
		return err
	}

	if err := opts.confirmer(f.yes).Confirm(workflowCopyQuestion(f, source, target, targetName, fromResolution, toResolution)); err != nil {
		return err
	}

	action := "updated"
	var written *n8n.Workflow
	if target == nil {
		action = "created"
		written, err = toClient.CreateWorkflow(ctx, doc)
		if err != nil {
			return fmt.Errorf("target: %w", workflowAPIError(err, toResolution, "create"))
		}
	} else {
		// Never republish implicitly: a copy into production would go live
		// with no flag. --publish is the only way this command publishes.
		publishIfActive := false
		written, err = toClient.UpdateWorkflow(ctx, target.ID, doc, n8n.UpdateWorkflowOptions{PublishIfActive: &publishIfActive})
		if err != nil {
			return fmt.Errorf("target: %w", workflowAPIError(err, toResolution, "update"))
		}
	}

	result := workflowCopyResult{
		SchemaVersion: 1,
		Action:        action,
		From:          workflowEndpointFor(fromResolution, source),
		To:            workflowEndpointFor(toResolution, written),
		Nodes:         written.NodeCount(),
		DroppedFields: dropped,
		Warnings:      workflowCopyWarnings(f, written),
	}

	if f.publish {
		result.Publish.Requested = true
		if written.Active {
			fmt.Fprintf(opts.Streams.Err, "%s is already published; the copy was saved as a new version, which --publish did not put live. Publish it with 'n8n workflow publish %s --context %s'.\n",
				workflowCopyLabel(written), written.ID, f.to.context)
		} else {
			published, err := toClient.PublishWorkflow(ctx, written.ID, n8n.PublishWorkflowRequest{})
			if err != nil {
				// The definition landed. Report that before failing, so the
				// next run is not a second copy of the same definition.
				if writeErr := writeWorkflowCopy(opts, result, f.output); writeErr != nil {
					return writeErr
				}
				return workflowCopyPublishError(err, written, toResolution, f.to.context)
			}
			result.Publish.Performed = true
			result.To = workflowEndpointFor(toResolution, published)
			result.Nodes = published.NodeCount()
		}
	}
	return writeWorkflowCopy(opts, result, f.output)
}

func validateCopyFlags(f *workflowCopyFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(f.from.context) == "" || strings.TrimSpace(f.to.context) == "" {
		return fmt.Errorf("--from-context and --to-context are required: copy names both instances explicitly")
	}
	if f.name != "" {
		if strings.TrimSpace(f.name) != f.name || strings.TrimSpace(f.name) == "" {
			return fmt.Errorf("--name must be nonempty and must not start or end with whitespace")
		}
	} else {
		if err := validateWorkflowArgument("source workflow ID", f.from.id); err != nil {
			return fmt.Errorf("%w: pass --from-id, or --name to resolve the source by name", err)
		}
		if f.from.projectID != "" {
			return fmt.Errorf("--from-project-id requires --name: it filters the source name lookup")
		}
	}
	if f.to.id != "" {
		if err := validateWorkflowArgument("target workflow ID", f.to.id); err != nil {
			return err
		}
	}
	if f.toName != "" && strings.TrimSpace(f.toName) == "" {
		return fmt.Errorf("--to-name must not be blank: omit it to keep the source workflow name")
	}
	return nil
}

// readCopyTarget resolves the workflow the copy replaces, or nil when the
// target has to be created. A missing --to-id is never a create: the API owns
// workflow IDs, so creating one would silently produce a different workflow
// than the user named.
func readCopyTarget(ctx context.Context, client *n8n.Client, r config.Resolution, f workflowCopyFlags, targetName string) (*n8n.Workflow, error) {
	id := f.to.id
	if id == "" {
		resolved, err := resolveWorkflowName(ctx, client, targetName, f.to.projectID)
		if errors.Is(err, errWorkflowNameNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("target: context %q (%s), workflow %q: %w", r.ContextName, r.URL, targetName, workflowAPIError(err, r, "list"))
		}
		id = resolved
	}
	// The target definition is never read for its content, only for its
	// identity, publish state and archived state, so pins stay excluded.
	w, err := client.GetWorkflow(ctx, id, true)
	if err != nil {
		if f.to.id != "" && n8n.IsNotFound(err) {
			return nil, fmt.Errorf("target workflow ID %q does not exist on %s; a new workflow cannot be given an ID — omit --to-id to create the target by name, or check 'n8n workflow list --context %s'",
				f.to.id, r.URL, f.to.context)
		}
		return nil, fmt.Errorf("target: context %q (%s), workflow ID %q: %w", r.ContextName, r.URL, id, workflowAPIError(err, r, "get"))
	}
	if w.ID != id {
		return nil, fmt.Errorf("target: context %q: workflow identity changed during lookup or read; expected ID %q, received ID %q; retry", r.ContextName, id, w.ID)
	}
	return w, nil
}

// checkCopyTarget rejects every combination that must not reach a write.
func checkCopyTarget(f workflowCopyFlags, target *n8n.Workflow) error {
	if target == nil {
		if f.updateOnly {
			return fmt.Errorf("--update-only: no workflow to replace on the target; create it with 'n8n workflow create --context %s', or re-run without --update-only to create it here", f.to.context)
		}
		return nil
	}
	if f.createOnly {
		return fmt.Errorf("--create-only: workflow %q (%s) already exists on the target; compare it with 'n8n workflow diff', or re-run without --create-only to replace it",
			target.Name, target.ID)
	}
	if target.IsArchived {
		return fmt.Errorf("target workflow %q (%s) is archived; unarchive it with 'n8n workflow unarchive %s --context %s' before copying onto it",
			target.Name, target.ID, target.ID, f.to.context)
	}
	if f.toFolderID != "" {
		return fmt.Errorf("--to-parent-folder-id applies only when the target is created; move an existing workflow with 'n8n workflow transfer'")
	}
	return nil
}

// workflowCopyDocument reduces the source read to what the target write
// accepts. Order matters: filter, then override, then validate, the same order
// 'workflow create' uses.
func workflowCopyDocument(opts Options, source *n8n.Workflow, f workflowCopyFlags, targetName string, creating bool) (n8n.WorkflowDocument, []string, error) {
	doc, err := source.Document()
	if err != nil {
		return nil, nil, err
	}
	var dropped []string
	if creating {
		doc, dropped = doc.ForCreate()
	} else {
		doc, dropped = doc.ForUpdate()
	}
	reportDroppedWorkflowFields(opts, dropped)

	// The source read already excluded pins unless --include-pinned-data was
	// given; deleting the key again costs nothing and cannot be wrong.
	// staticData is the opposite: the API always returns it.
	if !f.pins {
		delete(doc, "pinData")
		fmt.Fprintln(opts.Streams.Err, "Pinned sample data was not copied; pass --include-pinned-data to copy it.")
	}
	if !f.state {
		if _, ok := doc["staticData"]; ok {
			delete(doc, "staticData")
			fmt.Fprintln(opts.Streams.Err, "Runtime static data was not copied; pass --include-static-data to copy it.")
		}
	}

	if err := doc.Set("name", targetName); err != nil {
		return nil, nil, err
	}
	if creating {
		for field, value := range map[string]string{"projectId": f.to.projectID, "parentFolderId": f.toFolderID} {
			if value == "" {
				continue
			}
			if err := doc.Set(field, value); err != nil {
				return nil, nil, err
			}
		}
	}
	if err := doc.Validate(); err != nil {
		return nil, nil, err
	}
	return doc, dropped, nil
}

// workflowCopyQuestion names both workflows and the exact action, because the
// update path cannot be undone and the create path lands on a second instance.
func workflowCopyQuestion(f workflowCopyFlags, source, target *n8n.Workflow, targetName string, from, to config.Resolution) string {
	origin := fmt.Sprintf("%q (%s) on %s (context %q)", source.Name, source.ID, from.URL, from.ContextName)
	var question string
	if target == nil {
		question = fmt.Sprintf("Create workflow %q on %s (context %q) from %s? Tags, credentials and sharing are not copied; check them afterwards.",
			targetName, to.URL, to.ContextName, origin)
	} else {
		question = fmt.Sprintf("Replace workflow %q (%s) on %s (context %q) with the definition of %s? Nodes, connections and settings the source omits are cleared on the target and this cannot be undone.",
			target.Name, target.ID, to.URL, to.ContextName, origin)
	}
	if f.publish {
		question += " The target is then published, putting its triggers live."
	}
	return question
}

func workflowCopyWarnings(f workflowCopyFlags, written *n8n.Workflow) []string {
	return []string{
		fmt.Sprintf("Tags were not copied; tag IDs differ per instance. Reconcile with 'n8n workflow tag list %s --context %s'.", written.ID, f.to.context),
		fmt.Sprintf("Credentials do not transfer; a credential not shared with the destination project stops resolving at run time. Check with 'n8n credential list --context %s'.", f.to.context),
	}
}

func workflowCopyPublishError(err error, written *n8n.Workflow, r config.Resolution, contextName string) error {
	if n8n.IsForbidden(err) {
		return fmt.Errorf("%s denied publishing workflow %q (%s): publishing needs the workflow:activate scope and the project's workflow:publish permission. "+
			"The definition was copied and saved; only publication failed. Re-run without --publish, or publish after fixing permissions with 'n8n workflow publish %s --context %s'",
			r.URL, written.Name, written.ID, written.ID, contextName)
	}
	if n8n.IsConflict(err) {
		return fmt.Errorf("%s refused to publish workflow %q (409): an open workflow review or a webhook path already in use is blocking it. "+
			"The definition was copied and saved; only publication failed: %w", r.URL, written.ID, err)
	}
	return fmt.Errorf("target: %w", workflowAPIError(err, r, "publish"))
}

// workflowCopyLabel names a workflow the way the warnings read best.
func workflowCopyLabel(w *n8n.Workflow) string {
	return fmt.Sprintf("Workflow %q (%s)", w.Name, w.ID)
}

func writeWorkflowCopy(opts Options, result workflowCopyResult, output string) error {
	if output == outputJSON {
		if err := writeJSON(opts.Streams.Out, result); err != nil {
			return err
		}
	} else {
		tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
		fmt.Fprintf(tw, "Workflow:\t%s\n", emptyDash(result.To.Name))
		fmt.Fprintf(tw, "Source:\tcontext %q, workflow %s\n", result.From.Context, emptyDash(result.From.WorkflowID))
		fmt.Fprintf(tw, "Target:\tcontext %q, workflow %s\n", result.To.Context, emptyDash(result.To.WorkflowID))
		fmt.Fprintf(tw, "Instance:\t%s\n", result.To.URL)
		fmt.Fprintf(tw, "Published:\t%s\n", workflowCopyPublishedLine(result.To))
		fmt.Fprintf(tw, "Version:\t%s\n", emptyDash(result.To.VersionID))
		fmt.Fprintf(tw, "Nodes:\t%d\n", result.Nodes)
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(opts.Streams.Err, warning)
	}
	fmt.Fprintf(opts.Streams.Err, "Verify the result with: n8n workflow diff --from-context %s --from-id %s --to-context %s --to-id %s\n",
		result.From.Context, result.From.WorkflowID, result.To.Context, result.To.WorkflowID)
	return nil
}

// workflowCopyPublishedLine says when a published target kept an older version
// live, which is the default: the copy is saved as a draft.
func workflowCopyPublishedLine(to workflowEndpoint) string {
	if to.Published && to.ActiveVersionID != "" && to.ActiveVersionID != to.VersionID {
		return fmt.Sprintf("yes (live version %s unchanged; new version saved as draft)", to.ActiveVersionID)
	}
	if to.Published {
		return "yes"
	}
	return "no"
}
