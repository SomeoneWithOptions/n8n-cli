package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// dependencyAction is the action name the scope mappers know the dependency
// lookups by.
const dependencyAction = "dependencies"

// dependencyTarget is what differs between the workflow, credential and data
// table dependency lookups; the command shape, output and errors are shared.
type dependencyTarget struct {
	// noun names the resource in messages, e.g. "data table"; label is the
	// same as a text summary heading, e.g. "Data table".
	noun  string
	label string
	// group is the command group, e.g. "data-table".
	group string
	// idArg is the argument placeholder, e.g. "workflow-id".
	idArg string
	// placeholder is the ID in examples, e.g. "WORKFLOW_ID".
	placeholder string
	// listCommand finds valid IDs, e.g. "n8n workflow list".
	listCommand string
	// resource is the 'n8n discover' resource key.
	resource string
	// scope is what the API requires; the manifest records the same scope.
	scope scopeNeed
	// forbiddenCause is the other reason a 403 can have, besides the scope.
	forbiddenCause string
	short          string
	long           string
	// validate checks the ID argument before any request.
	validate func(string) error
	// client resolves the API client, enforcing any auth restriction.
	client func(Options, instanceFlags) (*n8n.Client, config.Resolution, error)
	// fetch selects the client method for one page.
	fetch func(*n8n.Client) n8n.DependencyFunc
}

type dependencyFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

// dependencyTypesHelp explains every type the API reports. Each lookup only
// returns the subset that applies to its resource.
const dependencyTypesHelp = "TYPE is the relation to the requested resource:\n" +
	"  credential, dataTable, subWorkflow, errorWorkflow\n" +
	"      the requested workflow uses this resource\n" +
	"  parentWorkflow       this workflow uses the requested resource\n" +
	"  errorWorkflowParent  this workflow uses the requested workflow as its error\n" +
	"                       workflow\n" +
	"  agent                this agent uses the requested resource"

// dependencyAccessHelp explains inaccessibleCount and the pagination flags.
const dependencyAccessHelp = "Dependencies the account in use cannot access are not listed; the summary\n" +
	"counts them as Inaccessible (JSON: inaccessibleCount, over the whole list, not\n" +
	"one page). Pass a returned cursor to --cursor for the next page, or use --all\n" +
	"to follow every cursor, capped at 10,000 dependencies.\n\n" + cursorCollectionHelp

func newDependencyCommand(opts Options, target dependencyTarget) *cobra.Command {
	var f dependencyFlags
	cmd := &cobra.Command{
		Use:   "dependencies <" + target.idArg + ">",
		Short: target.short,
		Long:  target.long + "\n\n" + dependencyTypesHelp + "\n\n" + dependencyAccessHelp,
		Example: "  n8n " + target.group + " dependencies " + target.placeholder + "\n" +
			"  n8n " + target.group + " dependencies " + target.placeholder + " --all --output json\n" +
			"  n8n " + target.group + " dependencies " + target.placeholder + " --limit 50 --cursor CURSOR",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDependencies(cmd.Context(), opts, target, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, fmt.Sprintf("dependencies per API page, 1 to %d (default: server default of 100)", n8n.MaxDependencyLimit))
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous page (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 dependencies; hitting it warns and sets JSON collection.truncated)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data, nextCursor and inaccessibleCount, plus collection with --all)")
	return cmd
}

func runDependencies(ctx context.Context, opts Options, target dependencyTarget, id string, f dependencyFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := target.validate(id); err != nil {
		return err
	}
	listOpts := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
	if err := n8n.ValidateDependencyOptions(listOpts); err != nil {
		return err
	}
	client, resolution, err := target.client(opts, f.instance)
	if err != nil {
		return err
	}
	lookup := target.fetch(client)
	// inaccessibleCount counts the whole list, so every page repeats it; the
	// largest value seen is kept in case a page reports none.
	inaccessible := 0
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.Dependency], error) {
		page, err := lookup(ctx, id, pagination)
		if err != nil {
			return n8n.Page[n8n.Dependency]{}, err
		}
		inaccessible = max(inaccessible, page.InaccessibleCount)
		return page.Page(), nil
	}
	var page n8n.Page[n8n.Dependency]
	var collection *listCollection
	if f.all {
		page, collection, err = collectPage(ctx, fetch, listOpts)
	} else {
		page, err = fetch(ctx, listOpts)
	}
	if err != nil {
		return dependencyAPIError(err, resolution, target)
	}
	result := n8n.DependencyPage{Data: page.Data, NextCursor: page.NextCursor, InaccessibleCount: inaccessible}
	if result.Data == nil {
		result.Data = []n8n.Dependency{}
	}
	if f.output == outputJSON {
		err = writeDependencyJSON(opts.Streams.Out, result, collection)
	} else {
		err = writeDependencyText(opts, resolution, target, id, result)
	}
	return finishCollection(opts.Streams.Err, collection, err)
}

// collectedDependencyPage is the JSON of a bounded dependency collection: the
// API page shape plus the collection report.
type collectedDependencyPage struct {
	Data              []n8n.Dependency `json:"data"`
	NextCursor        string           `json:"nextCursor"`
	InaccessibleCount int              `json:"inaccessibleCount"`
	Collection        collectionInfo   `json:"collection"`
}

func writeDependencyJSON(w io.Writer, page n8n.DependencyPage, c *listCollection) error {
	if c == nil {
		return writeJSON(w, page)
	}
	return writeJSON(w, collectedDependencyPage{
		Data: page.Data, NextCursor: page.NextCursor, InaccessibleCount: page.InaccessibleCount, Collection: c.info,
	})
}

func writeDependencyText(opts Options, resolution config.Resolution, target dependencyTarget, id string, page n8n.DependencyPage) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", target.label, id)
	fmt.Fprintf(tw, "Dependencies:\t%d\n", len(page.Data))
	fmt.Fprintf(tw, "Inaccessible:\t%d\n", page.InaccessibleCount)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nRELATION\tTYPE\tID\tNAME\tPROJECT")
		for _, dep := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				dependencyRelation(dep.Type), emptyDash(dep.Type), emptyDash(dep.ID), emptyDash(dep.Name), emptyDash(dep.ProjectID))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 && page.InaccessibleCount == 0 {
		fmt.Fprintf(opts.Streams.Err, "No dependencies: nothing this %s uses or is used by was found.\n", target.noun)
	}
	if page.InaccessibleCount > 0 {
		fmt.Fprintf(opts.Streams.Err, "%d %s not listed because the account in use cannot access %s; a user with access to their projects, or an owner or admin, sees them.\n",
			page.InaccessibleCount, plural(page.InaccessibleCount, "dependency is", "dependencies are"), plural(page.InaccessibleCount, "it", "them"))
	}
	return nil
}

// dependencyRelation says which way a dependency points: something the
// requested resource uses, or something that uses it.
func dependencyRelation(kind string) string {
	switch kind {
	case n8n.DependencyCredential, n8n.DependencyDataTable, n8n.DependencySubWorkflow, n8n.DependencyErrorWorkflow:
		return "uses"
	case n8n.DependencyParentWorkflow, n8n.DependencyErrorWorkflowParent, n8n.DependencyAgent:
		return "used by"
	}
	return "-"
}

// dependencyAPIError names the scope on a 403 and, on a 404, both causes: an
// unknown ID, or an instance that predates the dependency endpoints.
func dependencyAPIError(err error, resolution config.Resolution, target dependencyTarget) error {
	switch {
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, target.noun+" dependency lookup", target.scope, target.forbiddenCause, target.resource)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s answered 404 for the %s dependency lookup: either the %s does not exist (check the ID with '%s') or the instance does not serve dependency lookups yet (run %s to see what it offers): %w",
			resolution.URL, target.noun, target.noun, target.listCommand, discoverHint(target.resource), err)
	default:
		return apiError(err, resolution, target.resource)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func newWorkflowDependenciesCommand(opts Options) *cobra.Command {
	return newDependencyCommand(opts, dependencyTarget{
		noun:           "workflow",
		label:          "Workflow",
		group:          "workflow",
		idArg:          "workflow-id",
		placeholder:    "WORKFLOW_ID",
		listCommand:    "n8n workflow list",
		resource:       workflowResource,
		scope:          workflowScope(dependencyAction),
		forbiddenCause: "a 403 can also mean no access to the workflow's project.",
		short:          "List what a workflow uses and what uses it",
		long: "List the dependencies of one workflow in both directions: the credentials, data\n" +
			"tables, sub-workflows and error workflow it uses, and the workflows and agents\n" +
			"that use it, as a sub-workflow or as their error workflow. Check this before\n" +
			"archiving, deleting or transferring a workflow. Requires workflow:read.",
		validate: func(id string) error { return validateWorkflowArgument("workflow ID", id) },
		client:   Options.apiClient,
		fetch:    func(c *n8n.Client) n8n.DependencyFunc { return c.WorkflowDependencies },
	})
}

func newCredentialDependenciesCommand(opts Options) *cobra.Command {
	return newDependencyCommand(opts, dependencyTarget{
		noun:           "credential",
		label:          "Credential",
		group:          "credential",
		idArg:          "credential-id",
		placeholder:    "CREDENTIAL_ID",
		listCommand:    "n8n credential list",
		resource:       credentialResource,
		scope:          credentialScope(dependencyAction),
		forbiddenCause: "a 403 can also mean no access to the credential's project.",
		short:          "List the workflows and agents that use a credential",
		long: "List the workflows and agents that use one credential. Check this before\n" +
			"deleting, transferring or rotating a credential. Only metadata is returned;\n" +
			"credential data is never requested. Requires credential:read.",
		validate: validateCredentialArgument,
		client:   Options.apiClient,
		fetch:    func(c *n8n.Client) n8n.DependencyFunc { return c.CredentialDependencies },
	})
}

func newDataTableDependenciesCommand(opts Options) *cobra.Command {
	return newDependencyCommand(opts, dependencyTarget{
		noun:           "data table",
		label:          "Data table",
		group:          "data-table",
		idArg:          "table-id",
		placeholder:    "TABLE_ID",
		listCommand:    "n8n data-table list",
		resource:       dataTableResource,
		scope:          dataTableScope(dependencyAction),
		forbiddenCause: "a 403 can also mean the instance is not licensed for data tables.",
		short:          "List the workflows that use a data table",
		long: "List the workflows that use one data table. Check this before deleting the\n" +
			"table, clearing its rows or changing its columns. Like every data-table\n" +
			"command it requires API-key authentication, and it requires dataTable:read.",
		validate: func(id string) error { return validateDataTableArgument("data table ID", id) },
		client:   Options.dataTableClient,
		fetch:    func(c *n8n.Client) n8n.DependencyFunc { return c.DataTableDependencies },
	})
}
