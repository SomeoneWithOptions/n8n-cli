package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/spf13/cobra"
)

func newPromotionDiscoveryCommand(opts Options, branches bool) *cobra.Command {
	var f promotionProviderListFlags
	var search string
	name, use, short := "repositories", "repositories <provider-id>", "List repositories available to a GitLab provider"
	if branches {
		name, use, short = "branches", "branches <provider-id> <repository-path>", "List branches of a GitLab repository"
	}
	cmd := &cobra.Command{
		Use: use, Short: short,
		Long: "Discover GitLab repositories or branches using a saved provider with promotions:discover\n" +
			"(gitConnection:discover before n8n 2.47).\n" +
			"Start with 'n8n promotion provider list', then repositories, then branches.\n" +
			"Pass the raw fullPath (for example org/repo) or GitLab project ID to branches;\n" +
			"the CLI escapes it. Use remoteUrl from repositories as an environment remote\n" +
			"(n8n 2.46 and later) or a connection target (older). No saved environment\n" +
			"or connection is required and no content changes. Other provider types\n" +
			"do not support discovery. Text lists repository targets or branch names and\n" +
			"defaults; JSON returns data and nextCursor. Follow nextCursor even after a\n" +
			"short page. --all follows cursors up to 10,000 items.\n\n" + cursorCollectionHelp,
		Example: "  n8n promotion provider repositories PROVIDER_ID --search automation --all\n" +
			"  n8n promotion provider branches PROVIDER_ID org/repo --output json",
		Args: cobra.ExactArgs(1),
	}
	if branches {
		cmd.Args = cobra.ExactArgs(2)
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "items per API page, 1 to 250 (default: server default of 100; GitLab returns at most 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "cursor from a previous page (default: first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow all cursors, including short pages (default: one page; maximum 10,000 items)")
	cmd.Flags().StringVar(&search, "search", "", "repository or branch search string, at most 255 characters (default: no filter)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default: text; JSON includes data and nextCursor)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(f.output); err != nil {
			return err
		}
		list := n8n.PromotionDiscoveryOptions{ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}, Search: search}
		if err := list.Validate(); err != nil {
			return err
		}
		client, resolution, err := opts.apiClient(f.instance)
		if err != nil {
			return err
		}
		subject := promotionProviderSubject(args[0])
		if branches {
			subject.what += fmt.Sprintf(" or repository %q", args[1])
			subject.find += " and 'n8n promotion provider repositories'"
			fetch := func(ctx context.Context, page n8n.ListOptions) (n8n.Page[n8n.PromotionRepositoryBranch], error) {
				list.ListOptions = page
				return client.ListPromotionProviderRepositoryBranches(ctx, args[0], args[1], list)
			}
			return runPromotionDiscovery(cmd.Context(), opts, resolution, f, name, subject, fetch, "NAME\tDEFAULT", func(row n8n.PromotionRepositoryBranch) string { return fmt.Sprintf("%s\t%t", row.Name, row.IsDefault) })
		}
		fetch := func(ctx context.Context, page n8n.ListOptions) (n8n.Page[n8n.PromotionRepository], error) {
			list.ListOptions = page
			return client.ListPromotionProviderRepositories(ctx, args[0], list)
		}
		return runPromotionDiscovery(cmd.Context(), opts, resolution, f, name, subject, fetch, "ID\tNAME\tPATH\tREMOTE URL\tDEFAULT BRANCH", func(row n8n.PromotionRepository) string {
			branch := "-"
			if row.DefaultBranch != nil {
				branch = *row.DefaultBranch
			}
			return fmt.Sprintf("%s\t%s\t%s\t%s\t%s", row.ID, row.Name, row.FullPath, row.RemoteURL, branch)
		})
	}
	return cmd
}

func runPromotionDiscovery[T any](ctx context.Context, opts Options, resolution config.Resolution, f promotionProviderListFlags, name string, subject promotionSubject, fetch n8n.PageFunc[T], header string, rowText func(T) string) error {
	var page n8n.Page[T]
	var collection *listCollection
	var err error
	list := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
	if f.all {
		var collected n8n.Collection[T]
		collected, err = n8n.CollectPromotionDiscovery(ctx, fetch, list)
		page = n8n.Page[T]{Data: collected.Data, NextCursor: collected.NextCursor}
		collection = cursorCollection(collected.Truncated, collected.NextCursor, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, list)
	}
	if err != nil {
		return promotionAPIError(err, resolution, "provider "+name, subject)
	}
	if len(page.Data) == 0 {
		fmt.Fprintf(opts.Streams.Err, "No %s found; check the search filter and provider access.\n", name)
	}
	if f.output == outputJSON {
		err = writePageJSON(opts.Streams.Out, page, collection)
	} else {
		tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, header)
		for _, row := range page.Data {
			fmt.Fprintln(tw, rowText(row))
		}
		err = tw.Flush()
		if page.NextCursor != "" {
			fmt.Fprintf(opts.Streams.Err, "More %s available: use --cursor %q or --all.\n", name, page.NextCursor)
		}
	}
	return finishCollection(opts.Streams.Err, collection, err)
}

func newPromotionProjectChangesCommand(opts Options) *cobra.Command {
	var instance instanceFlags
	var output string
	var query n8n.PromotionChangesOptions
	cmd := &cobra.Command{
		Use: "changes <project-id> <apply|promote>", Short: "Preview the changes of a project's promotion branch",
		Long: "Compare a team project with its resolved promotion branch without changing content.\n" +
			"Start with 'n8n project list' for the project ID. The project's own\n" +
			"environment (n8n 2.46 and later) or connection (older) for the direction\n" +
			"takes precedence over the instance one. Prepare the checkout first: 'n8n\n" +
			"promotion environment connect' on 2.46, 'n8n promotion checkout clone'\n" +
			"before. Apply requires promotions:apply; promote requires\n" +
			"promotions:promote (gitConnection:pull and gitConnection:push before\n" +
			"n8n 2.47).\n\n" +
			"n8n 2.49 and later list every changed project, folder, workflow, credential,\n" +
			"data table, variable, and tag, plus unchanged parents for context. Text\n" +
			"shows each resource's key, type, ID, name, change, whether it can be\n" +
			"selected (--workflow-id or --folder-id), and for workflows how many\n" +
			"resources they use. Apply previews also show what blocks the Apply and the\n" +
			"preview revision; promote previews show an open review that holds the\n" +
			"changes. Read the content of changed resources with 'n8n promotion project\n" +
			"change-details'. Older instances list changed workflows with their status\n" +
			"and dependency count, and accept --search, --sort, and --order.\n\n" +
			"JSON is the full response, including source (environmentId on 2.46,\n" +
			"configId before) and nullable commitSha. A workflow that moved between\n" +
			"projects is refused by promote until it moves back. Save apply JSON for\n" +
			"--expected-from to reject a branch, and on 2.49 an instance, that changes\n" +
			"after review.",
		Example: "  n8n promotion project changes PROJECT_ID apply --output json > changes.json\n" +
			"  n8n promotion project changes PROJECT_ID promote",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(output); err != nil {
				return err
			}
			if err := query.Validate(); err != nil {
				return err
			}
			client, resolution, err := opts.apiClient(instance)
			if err != nil {
				return err
			}
			result, err := client.GetPromotionChanges(cmd.Context(), args[0], args[1], query)
			if err != nil {
				if n8n.IsStatus(err, http.StatusBadRequest) && (query.Search != "" || query.Sort != "" || query.Order != "") {
					return fmt.Errorf("%s rejected the change preview filters (400): n8n 2.49 and later dropped --search, --sort, and --order; run it again without them: %w", resolution.URL, err)
				}
				return promotionAPIError(err, resolution, "project changes "+args[1], promotionProjectSubject(args[0]))
			}
			if result.ResourceView() {
				if result.Blocker != nil {
					fmt.Fprintf(opts.Streams.Err, "An open review (%s) already holds this project's changes (%s): merge or close it before promoting again.\n", emptyDash(result.Blocker.ReviewID), result.Blocker.Code)
				} else if len(result.Changed()) == 0 {
					fmt.Fprintln(opts.Streams.Err, "No changes found; review the other direction.")
				}
				for _, resource := range result.Resources {
					if note := promotionMovedResourceNote(resource); note != "" {
						fmt.Fprintln(opts.Streams.Err, note)
					}
				}
				if result.Check != nil && !result.Check.Empty() {
					fmt.Fprintln(opts.Streams.Err, "An Apply of these changes would be blocked: resolve the listed items first.")
				}
			} else if len(result.Changes) == 0 {
				fmt.Fprintln(opts.Streams.Err, "No workflow changes found; check the search filter or review the other direction.")
			}
			if output == outputJSON {
				return writeJSON(opts.Streams.Out, result)
			}
			return writePromotionChangesText(opts, result)
		},
	}
	instance.register(cmd)
	cmd.Flags().StringVar(&query.Search, "search", "", "workflow name search string, n8n 2.48 and earlier only (default: no filter)")
	cmd.Flags().StringVar(&query.Sort, "sort", "", "sort field: name, updatedAt, or status; n8n 2.48 and earlier only (default: server default, name)")
	cmd.Flags().StringVar(&query.Order, "order", "", "sort order: asc or desc; n8n 2.48 and earlier only (default: server default, asc)")
	// Deprecation notices are diagnostics: keep them off stdout.
	cmd.Flags().SetOutput(opts.Streams.Err)
	for _, name := range []string{"search", "sort", "order"} {
		_ = cmd.Flags().MarkDeprecated(name, "n8n 2.49 and later list every change and reject it; it works only on n8n 2.48 and earlier")
	}
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text or json (default: text; JSON includes the reviewed source)")
	return cmd
}

func writePromotionChangesText(opts Options, result *n8n.PromotionChanges) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	commit := "-"
	if result.CommitSHA != nil {
		commit = *result.CommitSHA
	}
	if result.Source.EnvironmentID != "" {
		fmt.Fprintf(tw, "Environment:\t%s\n", result.Source.EnvironmentID)
	} else {
		fmt.Fprintf(tw, "Config:\t%s\n", emptyDash(result.Source.ConfigID))
	}
	fmt.Fprintf(tw, "Branch:\t%s\nCommit:\t%s\n", result.Source.BranchName, commit)
	if !result.ResourceView() {
		fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tDEPENDENCIES")
		for _, row := range result.Changes {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", row.ID, row.Name, row.Status, row.DependencyCount)
		}
		return tw.Flush()
	}
	if result.Revision != "" {
		fmt.Fprintf(tw, "Preview revision:\t%s\n", result.Revision)
	}
	if result.Blocker != nil {
		fmt.Fprintf(tw, "Blocker:\t%s (review %s)\n", result.Blocker.Code, emptyDash(result.Blocker.ReviewID))
	}
	uses := map[string]int{}
	for _, dependency := range result.Dependencies {
		uses[dependency.WorkflowKey]++
	}
	fmt.Fprintln(tw, "\nKEY\tTYPE\tID\tNAME\tCHANGE\tSELECTABLE\tUSES")
	for _, resource := range result.Resources {
		name := resource.Name
		if resource.PreviousName != "" && resource.PreviousName != resource.Name {
			name = resource.PreviousName + " -> " + resource.Name
		}
		used := "-"
		if resource.Type == "workflow" {
			used = strconv.Itoa(uses[resource.Key])
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", resource.Key, resource.Type, resource.ID, emptyDash(name), resource.Change, yesNo(resource.Selectable), used)
	}
	if check := result.Check; check != nil {
		writePromotionCheck(tw, check.PromotionApplyPreflight)
		writePromotionMovedResources(tw, "Moved workflows", check.MovedWorkflows)
		writePromotionMovedResources(tw, "Moved folders", check.MovedFolders)
	}
	return tw.Flush()
}

func writePromotionMovedResources(tw *tabwriter.Writer, title string, moved []n8n.PromotionMovedResource) {
	if len(moved) == 0 {
		return
	}
	fmt.Fprintf(tw, "\n%s (%d)\nID\tNAME\tBRANCH PROJECT\tINSTANCE PROJECT\n", strings.ToUpper(title), len(moved))
	for _, item := range moved {
		branch := promotionProjectRefText(item.Project)
		if !item.ProjectExists {
			branch += ", not on this instance"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.ID, emptyDash(item.Name), branch, promotionProjectRefText(item.InstanceProject))
	}
}

func promotionProjectRefText(project n8n.PromotionProjectRef) string {
	if project.Name == "" {
		return emptyDash(project.ID)
	}
	return project.Name + " (" + project.ID + ")"
}

// promotionMovedResourceNote explains a preview workflow that moved between
// projects: promote refuses it until it moves back.
func promotionMovedResourceNote(resource n8n.PromotionResource) string {
	switch {
	case resource.MovedToProject != nil:
		return fmt.Sprintf("Workflow %s moved to project %s; promoting it is refused until it moves back.", resource.ID, promotionProjectRefText(*resource.MovedToProject))
	case resource.MovedFromProject != nil:
		return fmt.Sprintf("Workflow %s moved here from project %s; promoting it is refused until it moves back.", resource.ID, promotionProjectRefText(*resource.MovedFromProject))
	}
	return ""
}

// What a selection or apply-continue command addresses. Connections exist only
// before n8n 2.46, environments only from 2.46; projects exist on both.
const (
	promotionTargetConnection  = "connection"
	promotionTargetProject     = "project"
	promotionTargetEnvironment = "environment"
)

type promotionSelectionFlags struct {
	instance                    instanceFlags
	input, output, expectedFrom string
	workflowIDs, folderIDs      []string
	source                      n8n.PromotionExpectedSource
	commit                      string
	previewRevision             string
	yes, destructive            bool
	idempotency                 idempotencyFlag
}

// Separate commands keep the required source and selection explicit at continue time.
func newPromotionSelectionCommand(opts Options, action, target string) *cobra.Command {
	var f promotionSelectionFlags
	project := target == promotionTargetProject
	environment := target == promotionTargetEnvironment
	prefix := "n8n promotion " + target + " "
	if target == promotionTargetConnection {
		prefix = "n8n promotion "
	}
	isContinue := action == "apply-continue"
	isPromote := action == "promote"
	// Which expected-source identifier flags exist: each generation accepts
	// only its own field, and projects are served by both.
	configFlag := !environment
	environmentFlag := target != promotionTargetConnection
	short := "Apply selected workflow and folder changes from a project's branch"
	if environment {
		short = "Apply an environment's Git branch into this instance"
	}
	if isContinue {
		short = "Continue a reviewed " + target + " apply after binding setup"
	}
	if isPromote {
		short = "Promote selected project workflows and folders to the remote branch"
	}
	long := "Start with 'n8n promotion project changes PROJECT_ID apply' to review the changes.\n" +
		"The project's own apply environment (n8n 2.46 and later) or connection\n" +
		"(older) resolves first, otherwise the instance one. It must be prepared:\n" +
		"'n8n promotion environment connect' on 2.46, 'n8n promotion checkout\n" +
		"clone' before. Selected IDs on the branch are imported; selected workflows\n" +
		"owned by this project whose IDs are absent from the branch are removed.\n" +
		"On 2.46 a selected workflow also moves within the project to match the\n" +
		"branch, with the folders that hold it, which take the branch name; one\n" +
		"the branch and the instance hold in different projects is refused with\n" +
		"409 before any write. Unselected content stays unchanged. Duplicate or\n" +
		"invalid IDs reject the whole request. On n8n 2.49 and later --folder-id\n" +
		"creates a selected folder, or moves and renames it to match the branch,\n" +
		"and the project's name, icon, description, and telemetry tags always come\n" +
		"from the branch: without --workflow-id and --folder-id only they are\n" +
		"applied. Older instances need at least one --workflow-id and reject\n" +
		"--folder-id. Another operation on the same environment answers 423; retry\n" +
		"when it finishes. Requires promotions:apply (gitConnection:pull before\n" +
		"n8n 2.47).\n"
	switch {
	case target == promotionTargetConnection:
		long = "Continue a full instance-connection apply after resolving its preflight items.\n" +
			"Connections exist before n8n 2.46; on 2.46 use 'n8n promotion environment\n" +
			"apply-continue'. Start with 'n8n promotion apply CONNECTION_ID --output json'.\n" +
			"The apply checkout must be cloned. Every team project is synchronized: workflows,\n" +
			"folders, credentials, tables, variables, and tags can change or be removed.\n" +
			"Requires gitConnection:pull and the same permissions as the initial apply.\n"
	case environment && isContinue:
		long = "Continue a full environment apply (n8n 2.46 and later) after resolving its\n" +
			"check items. Start with 'n8n promotion environment apply ENVIRONMENT_ID\n" +
			"--output json'. The instance-scope apply environment must still be\n" +
			"connected. Every team project is synchronized: workflows, folders,\n" +
			"credentials, tables, variables, and tags can be created, updated, moved,\n" +
			"renamed, or removed to match. Requires promotions:apply (gitConnection:pull\n" +
			"before n8n 2.47) and the same permissions as the initial apply.\n\n"
	case environment:
		long = "Check the full package at the branch tip of an instance-scope apply\n" +
			"environment (n8n 2.46 and later) and import it into the instance,\n" +
			"overwriting to match. Before 2.46 use 'n8n promotion apply' with a\n" +
			"connection. Start with 'n8n promotion environment list --direction\n" +
			"apply' and connect it first with 'n8n promotion environment connect'.\n" +
			"Personal projects are ignored. An applied run rewrites every team\n" +
			"project from the branch and cannot be undone: workflows, folders,\n" +
			"credentials, data tables, variables, and tags are created, updated,\n" +
			"or removed to match, workflows and folders move within their project,\n" +
			"folders take the branch name, and credential gaps become stubs.\n" +
			"Existing variable values are kept. A workflow or folder the branch and\n" +
			"the instance hold in different projects is refused with 409 before any\n" +
			"write. Requires promotions:apply (gitConnection:pull before n8n 2.47).\n\n"
	}
	if isPromote {
		long = "Start with 'n8n promotion project changes PROJECT_ID promote' to review IDs.\n" +
			"Push the current state of selected workflows and, on n8n 2.49 and later,\n" +
			"folders (--folder-id; written with the folders above them, even when\n" +
			"empty) to the project's resolved\n" +
			"promote environment (n8n 2.46 and later) or connection (older): its own,\n" +
			"otherwise the instance one. Prepare it first: 'n8n promotion environment\n" +
			"connect' on 2.46, 'n8n promotion checkout clone' before. Archived\n" +
			"workflows remain archived on the branch; workflows gone from the instance\n" +
			"leave the branch. On 2.46 a workflow that moved into or out of the\n" +
			"project since the branch was written is refused with 400; move it back\n" +
			"first. Unknown deletions reject the whole request. On 2.49 the project's\n" +
			"own details are always written: without --workflow-id and --folder-id\n" +
			"only they are promoted; older instances need at least one --workflow-id.\n" +
			"Another operation on the same environment answers 423. Requires\n" +
			"promotions:promote (gitConnection:push before n8n 2.47), plus\n" +
			"variable:list when workflows use variables.\n" +
			"Commit message is optional; the server supplies a default. Force pushing\n" +
			"is not supported. The result can name a review the Git host opened;\n" +
			"advisory warnings go to stderr.\n"
	} else {
		var idFlags, idField string
		switch target {
		case promotionTargetConnection:
			idFlags, idField = "--expected-config-id", "configId"
		case promotionTargetEnvironment:
			idFlags, idField = "--expected-environment-id", "environmentId"
		default:
			idFlags = "--expected-environment-id (n8n 2.46 and later) or\n--expected-config-id (older; mutually exclusive)"
			idField = "environmentId (2.46) or configId (older)"
		}
		long += "Use --expected-from FILE|- with changes JSON or a blocked apply result,\n" +
			"or supply " + idFlags + ", --expected-branch, and\n" +
			"--expected-commit together (full lowercase 40- or 64-digit hex SHA). A\n" +
			"null commit cannot be used. --expected-from passes through the\n" +
			"identifier the instance returned: " + idField + ".\n"
		if isContinue {
			long += "Expected source is required; use the blocked result's " + idField + "\nand git. " +
				"The server keeps no session and provides no exactly-once guarantee.\n"
			if project {
				long += "Resend the same full workflow ID selection used for the initial apply.\n"
			}
			long += "--confirm-destructive-changes permits removal or retyping of data-table\n" +
				"columns and loss of their values; rows and other columns remain.\n"
			if !project {
				long += "It also permits deleting team projects with everything in them.\n"
			}
			long += "This covers every such change at continue time, including changes absent\nfrom the reviewed result.\n"
		} else {
			long += "Expected source is optional; supply it to detect changes after review.\n"
		}
		if project {
			long += "--expected-preview-revision (n8n 2.47 and later) pins the reviewed Apply\n" +
				"preview; when this instance changed since, the server answers\n" +
				"preview-changed and writes nothing. Older instances reject it. Apply\n" +
				"changes JSON from n8n 2.49 carries the revision: --expected-from sends\n" +
				"it unless --expected-preview-revision is set.\n"
		}
		if target == promotionTargetConnection {
			long += "Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 other error. Only\n" +
				"applied writes content. Resolve blocked bindings before apply-continue;\n" +
				"review source-changed results again. JSON prints the full status result;\n" +
				"text shows preflight details or counts and warnings. Hints go to stderr.\n"
		} else {
			long += "Exit codes: 0 applied, 3 blocked, 4 source-changed or preview-changed\n" +
				"(n8n 2.47 and later), 1 other error. Only applied writes content.\n" +
				"Resolve blocked items before apply-continue; review source-changed and\n" +
				"preview-changed results again. JSON prints the full status result; text\n" +
				"shows the blocking check (preflight before n8n 2.47) or counts and\n" +
				"warnings. Hints go to stderr.\n"
		}
	}
	if project {
		long += "\nPass repeated --workflow-id and --folder-id flags, or use a strict JSON\nrequest (workflowIds, folderIds).\n"
	}
	long += "Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are\n" +
		"mutually exclusive. Interactive runs ask for confirmation; non-interactive\n" +
		"runs and stdin input require --yes. JSON returns the server result; text\n" +
		"shows counts and the commit after success.\n\n" + idempotencyHelp
	example := "  " + prefix + action + " " + strings.ToUpper(target) + "_ID"
	flags := ""
	if project {
		flags += " --workflow-id WORKFLOW_ID"
	}
	if isContinue {
		flags += " --expected-from blocked.json"
	}
	cmd := &cobra.Command{Use: action + " <" + target + "-id>", Short: short, Long: long,
		Example: example + flags + "\n" + example + " --input request.json --yes --output json", Args: cobra.ExactArgs(1)}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default: text; JSON is the full server result)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)")
	f.idempotency.register(cmd)
	documentFlags := []string{}
	if project {
		cmd.Flags().StringArrayVar(&f.workflowIDs, "workflow-id", nil, "selected workflow ID, 1 to 36 characters; repeat for each ID (default: none; n8n 2.48 and earlier need at least one)")
		cmd.Flags().StringArrayVar(&f.folderIDs, "folder-id", nil, "selected folder ID, 1 to 36 characters; repeat for each ID (n8n 2.49 and later; default: none)")
		documentFlags = append(documentFlags, "workflow-id", "folder-id")
	}
	if isPromote {
		cmd.Flags().StringVar(&f.commit, "commit-message", "", "commit message, 1 to 1000 characters (default: server-generated message)")
		documentFlags = append(documentFlags, "commit-message")
	} else {
		cmd.Flags().StringVar(&f.expectedFrom, "expected-from", "", "changes or blocked-apply JSON file, or - for stdin (default: explicit expected-source flags; stdin requires --yes)")
		documentFlags = append(documentFlags, "expected-from")
		if environmentFlag {
			usage := "reviewed environment ID, 1 to 36 characters (default: omitted; requires expected branch and commit)"
			if project {
				usage = "reviewed environment ID on n8n 2.46 and later, 1 to 36 characters (default: omitted; requires expected branch and commit; excludes --expected-config-id)"
			}
			cmd.Flags().StringVar(&f.source.EnvironmentID, "expected-environment-id", "", usage)
			documentFlags = append(documentFlags, "expected-environment-id")
		}
		if configFlag {
			usage := "reviewed config ID, 1 to 36 characters (default: omitted; requires expected branch and commit)"
			if project {
				usage = "reviewed config ID on n8n 2.45 and earlier, 1 to 36 characters (default: omitted; requires expected branch and commit; excludes --expected-environment-id)"
			}
			cmd.Flags().StringVar(&f.source.ConfigID, "expected-config-id", "", usage)
			documentFlags = append(documentFlags, "expected-config-id")
		}
		cmd.Flags().StringVar(&f.source.BranchName, "expected-branch", "", "reviewed branch name (default: omitted; requires the expected ID and commit)")
		cmd.Flags().StringVar(&f.source.CommitSHA, "expected-commit", "", "reviewed lowercase full 40- or 64-digit hex commit SHA (default: omitted; requires the expected ID and branch)")
		documentFlags = append(documentFlags, "expected-branch", "expected-commit")
		if project {
			cmd.Flags().StringVar(&f.previewRevision, "expected-preview-revision", "", "revision of the reviewed Apply preview, 64 lowercase hex digits; a changed preview answers preview-changed and writes nothing (n8n 2.47 and later; default: omitted)")
			documentFlags = append(documentFlags, "expected-preview-revision")
		}
	}
	if isContinue {
		cmd.Flags().BoolVar(&f.destructive, "confirm-destructive-changes", false, "allow all destructive changes at continue time, including unreviewed column value loss and, for full apply, team-project deletion (default: false)")
		documentFlags = append(documentFlags, "confirm-destructive-changes")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(f.output); err != nil {
			return err
		}
		if environment {
			if err := validatePromotionEnvironmentArg(args[0]); err != nil {
				return err
			}
		} else if strings.TrimSpace(args[0]) != args[0] || args[0] == "" || utf8.RuneCountInString(args[0]) > 36 {
			return fmt.Errorf("%s ID must contain 1 to 36 characters without surrounding whitespace", target)
		}
		if cmd.Flags().Changed("input") {
			for _, name := range documentFlags {
				if cmd.Flags().Changed(name) {
					return fmt.Errorf("--input and document flags are mutually exclusive")
				}
			}
			if f.input == "" {
				return fmt.Errorf("--input requires a file path or -")
			}
		}
		if (f.input == "-" || f.expectedFrom == "-") && !f.yes {
			return fmt.Errorf("stdin input requires --yes because stdin cannot answer confirmation")
		}
		if err := f.idempotency.validate(); err != nil {
			return err
		}
		var source *n8n.PromotionExpectedSource
		previewRevision := f.previewRevision
		if !isPromote && f.input == "" {
			if cmd.Flags().Changed("expected-config-id") && cmd.Flags().Changed("expected-environment-id") {
				return fmt.Errorf("--expected-config-id and --expected-environment-id are mutually exclusive: use --expected-environment-id on n8n 2.46 and later, --expected-config-id on n8n 2.45 and earlier")
			}
			explicit := cmd.Flags().Changed("expected-config-id") || cmd.Flags().Changed("expected-environment-id") ||
				cmd.Flags().Changed("expected-branch") || cmd.Flags().Changed("expected-commit")
			if cmd.Flags().Changed("expected-from") {
				if explicit {
					return fmt.Errorf("--expected-from and explicit expected-source flags are mutually exclusive")
				}
				var revision string
				var err error
				source, revision, err = readPromotionExpectedSource(opts, f.expectedFrom)
				if err != nil {
					return err
				}
				// A 2.49 apply preview pins the instance state too.
				if project && revision != "" && !cmd.Flags().Changed("expected-preview-revision") {
					previewRevision = revision
					fmt.Fprintf(opts.Streams.Err, "Using the preview revision %s from --expected-from as the expected preview revision.\n", revision)
				}
			} else if explicit {
				source = &f.source
			}
		}
		var destructive *bool
		if cmd.Flags().Changed("confirm-destructive-changes") {
			destructive = &f.destructive
		}
		var request interface{ Validate() error }
		switch {
		case isPromote:
			r := &n8n.PromoteProjectSelectionRequest{WorkflowIDs: f.workflowIDs, FolderIDs: f.folderIDs}
			if cmd.Flags().Changed("commit-message") {
				r.CommitMessage = &f.commit
			}
			request = r
		case isContinue && project:
			r := &n8n.ContinueApplyProjectSelectionRequest{WorkflowIDs: f.workflowIDs, FolderIDs: f.folderIDs, ExpectedPreviewRevision: previewRevision, ConfirmDestructiveChanges: destructive}
			if source != nil {
				r.ExpectedSource = *source
			}
			request = r
		case isContinue && environment:
			r := &n8n.ContinueApplyPromotionEnvironmentRequest{ConfirmDestructiveChanges: destructive}
			if source != nil {
				r.ExpectedSource = *source
			}
			request = r
		case isContinue:
			r := &n8n.ContinueApplyPromotionRequest{ConfirmDestructiveChanges: destructive}
			if source != nil {
				r.ExpectedSource = *source
			}
			request = r
		case environment:
			request = &n8n.ApplyPromotionEnvironmentRequest{ExpectedSource: source}
		default:
			request = &n8n.ApplyProjectSelectionRequest{WorkflowIDs: f.workflowIDs, FolderIDs: f.folderIDs, ExpectedSource: source, ExpectedPreviewRevision: previewRevision}
		}
		if f.input != "" {
			if err := readPromotionDocument(opts, f.input, request); err != nil {
				return err
			}
		}
		if err := request.Validate(); err != nil {
			return err
		}
		return runPromotionSelection(cmd.Context(), opts, args[0], f, request)
	}
	return cmd
}

// readPromotionExpectedSource reads the reviewed source from 'project changes'
// JSON or a blocked apply result. It passes through whichever identifier the
// instance returned (environmentId from n8n 2.46, configId before); the
// request's own validation then checks it fits the route. revision is the
// Apply preview revision n8n 2.49 changes JSON carries, empty otherwise.
func readPromotionExpectedSource(opts Options, path string) (*n8n.PromotionExpectedSource, string, error) {
	var fields map[string]json.RawMessage
	if err := readPromotionDocument(opts, path, &fields); err != nil {
		return nil, "", err
	}
	var result n8n.PromotionExpectedSource
	// Decode only source fields. Other response fields remain forward-compatible.
	decode := func(raw json.RawMessage, dst any) error {
		if len(raw) == 0 || string(raw) == "null" {
			return fmt.Errorf("reviewed source field is missing or null")
		}
		return json.Unmarshal(raw, dst)
	}
	// sourceID decodes environmentId or configId, whichever the object has.
	sourceID := func(object map[string]json.RawMessage, where string) error {
		_, hasEnvironment := object["environmentId"]
		_, hasConfig := object["configId"]
		switch {
		case hasEnvironment && hasConfig:
			return fmt.Errorf("%senvironmentId and %sconfigId are both present: keep the one the instance returned", where, where)
		case hasEnvironment:
			if err := decode(object["environmentId"], &result.EnvironmentID); err != nil {
				return fmt.Errorf("%senvironmentId: %w", where, err)
			}
		default:
			if err := decode(object["configId"], &result.ConfigID); err != nil {
				return fmt.Errorf("%sconfigId (or environmentId on n8n 2.46 and later): %w", where, err)
			}
		}
		return nil
	}
	var revision string
	if raw, ok := fields["source"]; ok {
		var source map[string]json.RawMessage
		if err := decode(raw, &source); err != nil {
			return nil, "", err
		}
		if err := sourceID(source, "source."); err != nil {
			return nil, "", err
		}
		if err := decode(source["branchName"], &result.BranchName); err != nil {
			return nil, "", fmt.Errorf("source.branchName: %w", err)
		}
		if err := decode(fields["commitSha"], &result.CommitSHA); err != nil {
			return nil, "", fmt.Errorf("commitSha: %w", err)
		}
		if raw, ok := fields["revision"]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &revision); err != nil {
				return nil, "", fmt.Errorf("revision: %w", err)
			}
			if err := n8n.ValidatePromotionPreviewRevision(revision); err != nil {
				return nil, "", err
			}
		}
	} else {
		var status string
		if err := decode(fields["status"], &status); err != nil || status != n8n.PromotionApplyStatusBlocked {
			return nil, "", fmt.Errorf("--expected-from requires changes JSON or a blocked apply result")
		}
		if err := sourceID(fields, ""); err != nil {
			return nil, "", err
		}
		var git map[string]json.RawMessage
		if err := decode(fields["git"], &git); err != nil {
			return nil, "", err
		}
		for key, dst := range map[string]*string{"branchName": &result.BranchName, "commitSha": &result.CommitSHA} {
			if err := decode(git[key], dst); err != nil {
				return nil, "", fmt.Errorf("git.%s: %w", key, err)
			}
		}
	}
	if err := result.Validate(); err != nil {
		return nil, "", err
	}
	return &result, revision, nil
}

func runPromotionSelection(ctx context.Context, opts Options, id string, f promotionSelectionFlags, request interface{ Validate() error }) error {
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	subject := promotionProjectSubject(id)
	target := promotionTargetProject
	action := "project apply"
	question := fmt.Sprintf("Apply selected changes to project %q on %s? Selected workflows absent from the branch are deleted; selected content and dependencies are overwritten, and the project's details are taken from the branch.", id, resolution.URL)
	workflowIDs, folderIDs, selection := promotionSelectionOf(request)
	if selection && n8n.EmptyPromotionSelection(workflowIDs, folderIDs) {
		question = fmt.Sprintf("Apply only the details (name, icon, description, telemetry tags) of project %q from its branch on %s? No workflow or folder is selected.", id, resolution.URL)
	}
	fullApply := " Every team project's workflows, folders, credentials, tables, variables, and tags may be overwritten or removed."
	allDestructive := " ALL team-project deletions with everything in them and destructive data-table column changes at continue time are allowed, including unreviewed changes; column values are lost."
	switch r := request.(type) {
	case *n8n.PromoteProjectSelectionRequest:
		action = "project promote"
		question = fmt.Sprintf("Promote selected workflows and folders of project %q on %s? The remote branch is committed and pushed; selected workflows no longer owned by this project leave the branch.", id, resolution.URL)
		if n8n.EmptyPromotionSelection(r.WorkflowIDs, r.FolderIDs) {
			question = fmt.Sprintf("Promote only the details (name, icon, description, telemetry tags) of project %q on %s? No workflow or folder is selected; the remote branch is committed and pushed.", id, resolution.URL)
		}
	case *n8n.ContinueApplyProjectSelectionRequest:
		action = "project apply-continue"
		if r.ConfirmDestructiveChanges != nil && *r.ConfirmDestructiveChanges {
			question += " ALL destructive data-table column changes at continue time are allowed, including unreviewed changes; removed or retyped column values are lost."
		}
	case *n8n.ContinueApplyPromotionRequest:
		action, subject, target = "apply-continue", promotionConnectionSubject(id), promotionTargetConnection
		question = fmt.Sprintf("Continue applying connection %q into %s?", id, resolution.URL) + fullApply
		if r.ConfirmDestructiveChanges != nil && *r.ConfirmDestructiveChanges {
			question += allDestructive
		}
	case *n8n.ApplyPromotionEnvironmentRequest:
		action, subject, target = "environment apply", promotionEnvironmentSubject(id), promotionTargetEnvironment
		question = fmt.Sprintf("Apply the remote of promotion environment %q into %s and overwrite instance projects to match? Workflows, folders, credentials, tables, variables, and tags change; this cannot be undone.", id, resolution.URL)
	case *n8n.ContinueApplyPromotionEnvironmentRequest:
		action, subject, target = "environment apply-continue", promotionEnvironmentSubject(id), promotionTargetEnvironment
		question = fmt.Sprintf("Continue applying environment %q into %s?", id, resolution.URL) + fullApply
		if r.ConfirmDestructiveChanges != nil && *r.ConfirmDestructiveChanges {
			question += allDestructive
		}
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	callOpts := f.idempotency.resolve(opts.Streams.Err)
	var result *n8n.ApplyPromotionResult
	switch r := request.(type) {
	case *n8n.PromoteProjectSelectionRequest:
		promoted, err := client.PromoteProjectSelection(ctx, id, *r, callOpts...)
		if err != nil {
			if refused := f.idempotency.refusal(err, resolution); refused != nil {
				return refused
			}
			if older := promotionSelectionTooNew(err, resolution, action, workflowIDs, folderIDs); older != nil {
				return older
			}
			return promotionAPIError(err, resolution, action, subject)
		}
		return writePromotionPromoteOutput(opts, resolution, promoted, f.output)
	case *n8n.ApplyProjectSelectionRequest:
		result, err = client.ApplyProjectSelection(ctx, id, *r, callOpts...)
	case *n8n.ContinueApplyProjectSelectionRequest:
		result, err = client.ContinueApplyProjectSelection(ctx, id, *r, callOpts...)
	case *n8n.ContinueApplyPromotionRequest:
		result, err = client.ContinueApplyPackage(ctx, id, *r, callOpts...)
	case *n8n.ApplyPromotionEnvironmentRequest:
		result, err = client.ApplyPromotionEnvironment(ctx, id, *r, callOpts...)
	case *n8n.ContinueApplyPromotionEnvironmentRequest:
		result, err = client.ContinueApplyPromotionEnvironment(ctx, id, *r, callOpts...)
	}
	if err != nil {
		if refused := f.idempotency.refusal(err, resolution); refused != nil {
			return refused
		}
		if selection {
			if older := promotionSelectionTooNew(err, resolution, action, workflowIDs, folderIDs); older != nil {
				return older
			}
		}
		return promotionAPIError(err, resolution, action, subject)
	}
	if f.output == outputJSON {
		err = writeJSON(opts.Streams.Out, result)
	} else {
		err = writePromotionApplyResult(opts, resolution, result)
	}
	if err != nil {
		return err
	}
	return finishPromotionApplyTarget(opts, id, result, target)
}

// promotionSelectionOf returns the workflow and folder selection of a project
// request; ok is false for requests that select nothing by ID.
func promotionSelectionOf(request any) (workflowIDs, folderIDs []string, ok bool) {
	switch r := request.(type) {
	case *n8n.ApplyProjectSelectionRequest:
		return r.WorkflowIDs, r.FolderIDs, true
	case *n8n.ContinueApplyProjectSelectionRequest:
		return r.WorkflowIDs, r.FolderIDs, true
	case *n8n.PromoteProjectSelectionRequest:
		return r.WorkflowIDs, r.FolderIDs, true
	}
	return nil, nil, false
}

// promotionSelectionTooNew explains the 400 an instance before n8n 2.49
// answers to a selection only 2.49 accepts: folder IDs, or no workflow ID.
// It returns nil for any other error.
func promotionSelectionTooNew(err error, resolution config.Resolution, action string, workflowIDs, folderIDs []string) error {
	apiErr, ok := errors.AsType[*n8n.APIError](err)
	if !ok || apiErr.StatusCode != http.StatusBadRequest {
		return nil
	}
	names := func(field string) bool {
		return slices.Contains(apiErr.Fields, field) || strings.Contains(apiErr.Message, field)
	}
	switch {
	case len(folderIDs) > 0 && names("folderIds"):
		return fmt.Errorf("%s rejected the promotion %s (400), so nothing changed: selecting folders with --folder-id needs n8n 2.49 or later; on this instance select the workflows with --workflow-id instead: %w", resolution.URL, action, err)
	case len(workflowIDs) == 0 && names("workflowIds"):
		return fmt.Errorf("%s rejected the promotion %s (400), so nothing changed: n8n 2.48 and earlier need at least one --workflow-id; a selection of only the project's details needs n8n 2.49 or later: %w", resolution.URL, action, err)
	}
	return nil
}

func promotionProjectSubject(id string) promotionSubject {
	return promotionSubject{what: fmt.Sprintf("project %q", id), find: "'n8n project list'"}
}

type promotionChangeDetailsFlags struct {
	instance  instanceFlags
	from      string
	commitSHA string
	keys      []string
	output    string
}

func newPromotionProjectChangeDetailsCommand(opts Options) *cobra.Command {
	var f promotionChangeDetailsFlags
	cmd := &cobra.Command{
		Use:   "change-details <project-id> <apply|promote>",
		Short: "Read the content of changed resources in a project preview",
		Long: "Read the content before and after the change of up to 50 resources of a\n" +
			"change preview (n8n 2.49 and later), without changing anything. Start with\n" +
			"'n8n promotion project changes PROJECT_ID DIRECTION --output json >\n" +
			"changes.json'. --from changes.json takes the preview's commitSha and, without\n" +
			"--key, every changed resource; it also reports resources whose revision\n" +
			"differs from the preview, which changed after it. Or pass --commit-sha with\n" +
			"repeated --key values from the KEY column; a preview without commitSha needs\n" +
			"--from. The branch side is read at that commit, the instance side now.\n" +
			"Credential content holds expressions only. Variable values are left out\n" +
			"unless the API key and the user have variable:list. Apply requires\n" +
			"promotions:apply, promote promotions:promote.\n\n" +
			"Text lists key, type, name, change, and data-table column changes; JSON is\n" +
			"the full response with before and after content and missingKeys, the\n" +
			"requested keys neither side holds anymore.",
		Example: "  n8n promotion project change-details PROJECT_ID apply --from changes.json --output json\n" +
			"  n8n promotion project change-details PROJECT_ID promote --commit-sha SHA --key KEY",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionChangeDetails(cmd, opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.from, "from", "", "'promotion project changes' JSON file, or - for stdin: supplies commitSha and, without --key, every changed resource (default: none; --commit-sha and --key required)")
	cmd.Flags().StringVar(&f.commitSHA, "commit-sha", "", "commitSha of the preview, full lowercase 40- or 64-digit hex (default: from --from; required without it)")
	cmd.Flags().StringArrayVar(&f.keys, "key", nil, "resource key from the preview; repeat for each, at most 50 (default: every changed resource in --from)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the full response with before and after content)")
	return cmd
}

func runPromotionChangeDetails(cmd *cobra.Command, opts Options, projectID, direction string, f promotionChangeDetailsFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.PromotionChangeDetailsRequest
	var preview *n8n.PromotionChanges
	switch {
	case f.from != "" && cmd.Flags().Changed("commit-sha"):
		return fmt.Errorf("--from and --commit-sha are mutually exclusive: --from already supplies the preview's commitSha")
	case f.from != "":
		preview = &n8n.PromotionChanges{}
		if err := readPromotionChangesDocument(opts, f.from, preview); err != nil {
			return err
		}
		if !preview.ResourceView() {
			return fmt.Errorf("--from holds a preview from an instance before n8n 2.49, which has no resource keys: run 'n8n promotion project changes %s %s --output json' against n8n 2.49 or later", projectID, direction)
		}
		request.CommitSHA = preview.CommitSHA
		request.Keys = f.keys
		if len(request.Keys) == 0 {
			for _, resource := range preview.Changed() {
				request.Keys = append(request.Keys, resource.Key)
			}
			if len(request.Keys) == 0 {
				return fmt.Errorf("--from lists no changed resources: nothing to read")
			}
			if len(request.Keys) > n8n.MaxPromotionChangeDetailKeys {
				return fmt.Errorf("--from lists %d changed resources, but one request reads at most %d: pass the keys to read with repeated --key", len(request.Keys), n8n.MaxPromotionChangeDetailKeys)
			}
		}
	case cmd.Flags().Changed("commit-sha"):
		request.CommitSHA = &f.commitSHA
		request.Keys = f.keys
	default:
		return fmt.Errorf("pass --from changes.json (from 'n8n promotion project changes %s %s --output json'), or --commit-sha with --key", projectID, direction)
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	result, err := client.GetPromotionChangeDetails(cmd.Context(), projectID, direction, request)
	if err != nil {
		if n8n.IsNotFound(err) && promotionRouteMissing(err) {
			return fmt.Errorf("%s does not serve change details (404): they need n8n 2.49 or later; use 'n8n promotion project changes' on older instances: %w", resolution.URL, err)
		}
		return promotionAPIError(err, resolution, "project change-details "+direction, promotionProjectSubject(projectID))
	}
	var revisions map[string]n8n.PromotionResourceRevision
	if preview != nil {
		revisions = make(map[string]n8n.PromotionResourceRevision, len(preview.Resources))
		for _, resource := range preview.Resources {
			revisions[resource.Key] = resource.Revision
		}
	}
	for _, detail := range result.Details {
		if reviewed, ok := revisions[detail.Key]; ok && !sameRevision(reviewed, detail.Revision) {
			fmt.Fprintf(opts.Streams.Err, "%s changed after the preview: review the changes again before applying or promoting.\n", detail.Key)
		}
		if detail.ValueHidden {
			fmt.Fprintf(opts.Streams.Err, "%s: variable values are hidden; the API key and the user need variable:list to read them.\n", detail.Key)
		}
	}
	if len(result.MissingKeys) > 0 {
		fmt.Fprintf(opts.Streams.Err, "Neither side holds %s anymore: review the changes again.\n", strings.Join(result.MissingKeys, ", "))
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Project:\t%s\nDirection:\t%s\nInstance:\t%s\n", projectID, direction, resolution.URL)
	fmt.Fprintln(tw, "\nKEY\tTYPE\tNAME\tCHANGE\tDETAIL")
	for _, detail := range result.Details {
		var changes []string
		for _, change := range detail.DataTableChanges {
			text := promotionChangeText(change)
			if change.Destructive {
				text += " (destructive)"
			}
			changes = append(changes, text)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", detail.Key, detail.Type, emptyDash(detail.Name()), detail.Change(), emptyDash(strings.Join(changes, "; ")))
	}
	return tw.Flush()
}

func sameRevision(a, b n8n.PromotionResourceRevision) bool {
	return derefString(a.Before) == derefString(b.Before) && derefString(a.After) == derefString(b.After) &&
		(a.Before == nil) == (b.Before == nil) && (a.After == nil) == (b.After == nil)
}

// readPromotionChangesDocument reads saved 'promotion project changes' JSON.
// Unknown fields are kept: the preview grows with each n8n release.
func readPromotionChangesDocument(opts Options, path string, dst *n8n.PromotionChanges) error {
	var raw json.RawMessage
	if err := readPromotionDocument(opts, path, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode changes JSON: %w", err)
	}
	return nil
}
