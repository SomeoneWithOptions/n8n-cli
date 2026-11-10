package cli

import (
	"context"
	"encoding/json"
	"fmt"
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
		Use: "changes <project-id> <apply|promote>", Short: "List workflow changes for a project's promotion branch",
		Long: "Compare a team project with its resolved promotion branch without changing content.\n" +
			"Start with 'n8n project list' for the project ID. The project's own\n" +
			"environment (n8n 2.46 and later) or connection (older) for the direction\n" +
			"takes precedence over the instance one. Prepare the checkout first: 'n8n\n" +
			"promotion environment connect' on 2.46, 'n8n promotion checkout clone'\n" +
			"before. Apply requires promotions:apply; promote requires\n" +
			"promotions:promote (gitConnection:pull and gitConnection:push before\n" +
			"n8n 2.47). Text lists workflow IDs, statuses, and dependency\n" +
			"counts; JSON includes changes, source (environmentId on 2.46, configId\n" +
			"before), and nullable commitSha. On 2.46 a deleted or new promote row can\n" +
			"be a workflow that moved between projects; promoting it is refused until\n" +
			"it moves back. Review the IDs before project apply or promote. Save apply\n" +
			"JSON for --expected-from to reject a branch that moves after review.",
		Example: "  n8n promotion project changes PROJECT_ID apply --output json > changes.json\n" +
			"  n8n promotion project changes PROJECT_ID promote --sort updatedAt --order desc",
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
				return promotionAPIError(err, resolution, "project changes "+args[1], promotionProjectSubject(args[0]))
			}
			if len(result.Changes) == 0 {
				fmt.Fprintln(opts.Streams.Err, "No workflow changes found; check the search filter or review the other direction.")
			}
			if output == outputJSON {
				return writeJSON(opts.Streams.Out, result)
			}
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
			fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tDEPENDENCIES")
			for _, row := range result.Changes {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", row.ID, row.Name, row.Status, row.DependencyCount)
			}
			return tw.Flush()
		},
	}
	instance.register(cmd)
	cmd.Flags().StringVar(&query.Search, "search", "", "workflow name search string (default: no filter)")
	cmd.Flags().StringVar(&query.Sort, "sort", "name", "sort field: name, updatedAt, or status (default: name)")
	cmd.Flags().StringVar(&query.Order, "order", "asc", "sort order: asc or desc (default: asc)")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text or json (default: text; JSON includes the reviewed source)")
	return cmd
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
	workflowIDs                 []string
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
	short := "Apply selected workflow changes from a project's branch"
	if environment {
		short = "Apply an environment's Git branch into this instance"
	}
	if isContinue {
		short = "Continue a reviewed " + target + " apply after binding setup"
	}
	if isPromote {
		short = "Promote selected project workflows to the remote branch"
	}
	long := "Start with 'n8n promotion project changes PROJECT_ID apply' to review workflow IDs.\n" +
		"The project's own apply environment (n8n 2.46 and later) or connection\n" +
		"(older) resolves first, otherwise the instance one. It must be prepared:\n" +
		"'n8n promotion environment connect' on 2.46, 'n8n promotion checkout\n" +
		"clone' before. Selected IDs on the branch are imported; selected workflows\n" +
		"owned by this project whose IDs are absent from the branch are removed.\n" +
		"On 2.46 a selected workflow also moves within the project to match the\n" +
		"branch, with the folders that hold it, which take the branch name; one\n" +
		"the branch and the instance hold in different projects is refused with\n" +
		"409 before any write. Unselected content stays unchanged. Duplicate or\n" +
		"invalid IDs reject the whole request. Requires promotions:apply\n" +
		"(gitConnection:pull before n8n 2.47).\n"
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
			"Push the current state of selected workflows to the project's resolved\n" +
			"promote environment (n8n 2.46 and later) or connection (older): its own,\n" +
			"otherwise the instance one. Prepare it first: 'n8n promotion environment\n" +
			"connect' on 2.46, 'n8n promotion checkout clone' before. Archived\n" +
			"workflows remain archived on the branch; workflows gone from the instance\n" +
			"leave the branch. On 2.46 a workflow that moved into or out of the\n" +
			"project since the branch was written is refused with 400; move it back\n" +
			"first. Unknown deletions reject the whole request. Requires\n" +
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
				"preview-changed and writes nothing. Older instances reject it.\n"
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
		long += "\nPass repeated --workflow-id flags, or use a strict JSON request.\n"
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
		cmd.Flags().StringArrayVar(&f.workflowIDs, "workflow-id", nil, "selected workflow ID, 1 to 36 characters; repeat for each ID (default: none; required unless --input is used)")
		documentFlags = append(documentFlags, "workflow-id")
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
				var err error
				source, err = readPromotionExpectedSource(opts, f.expectedFrom)
				if err != nil {
					return err
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
			r := &n8n.PromoteProjectSelectionRequest{WorkflowIDs: f.workflowIDs}
			if cmd.Flags().Changed("commit-message") {
				r.CommitMessage = &f.commit
			}
			request = r
		case isContinue && project:
			r := &n8n.ContinueApplyProjectSelectionRequest{WorkflowIDs: f.workflowIDs, ExpectedPreviewRevision: f.previewRevision, ConfirmDestructiveChanges: destructive}
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
			request = &n8n.ApplyProjectSelectionRequest{WorkflowIDs: f.workflowIDs, ExpectedSource: source, ExpectedPreviewRevision: f.previewRevision}
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
// request's own validation then checks it fits the route.
func readPromotionExpectedSource(opts Options, path string) (*n8n.PromotionExpectedSource, error) {
	var fields map[string]json.RawMessage
	if err := readPromotionDocument(opts, path, &fields); err != nil {
		return nil, err
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
	if raw, ok := fields["source"]; ok {
		var source map[string]json.RawMessage
		if err := decode(raw, &source); err != nil {
			return nil, err
		}
		if err := sourceID(source, "source."); err != nil {
			return nil, err
		}
		if err := decode(source["branchName"], &result.BranchName); err != nil {
			return nil, fmt.Errorf("source.branchName: %w", err)
		}
		if err := decode(fields["commitSha"], &result.CommitSHA); err != nil {
			return nil, fmt.Errorf("commitSha: %w", err)
		}
	} else {
		var status string
		if err := decode(fields["status"], &status); err != nil || status != n8n.PromotionApplyStatusBlocked {
			return nil, fmt.Errorf("--expected-from requires changes JSON or a blocked apply result")
		}
		if err := sourceID(fields, ""); err != nil {
			return nil, err
		}
		var git map[string]json.RawMessage
		if err := decode(fields["git"], &git); err != nil {
			return nil, err
		}
		for key, dst := range map[string]*string{"branchName": &result.BranchName, "commitSha": &result.CommitSHA} {
			if err := decode(git[key], dst); err != nil {
				return nil, fmt.Errorf("git.%s: %w", key, err)
			}
		}
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return &result, nil
}

func runPromotionSelection(ctx context.Context, opts Options, id string, f promotionSelectionFlags, request interface{ Validate() error }) error {
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	subject := promotionProjectSubject(id)
	target := promotionTargetProject
	action := "project apply"
	question := fmt.Sprintf("Apply selected workflow changes to project %q on %s? Selected workflows absent from the branch are deleted; selected content and dependencies are overwritten.", id, resolution.URL)
	fullApply := " Every team project's workflows, folders, credentials, tables, variables, and tags may be overwritten or removed."
	allDestructive := " ALL team-project deletions with everything in them and destructive data-table column changes at continue time are allowed, including unreviewed changes; column values are lost."
	switch r := request.(type) {
	case *n8n.PromoteProjectSelectionRequest:
		action = "project promote"
		question = fmt.Sprintf("Promote selected workflows of project %q on %s? The remote branch is committed and pushed; selected workflows no longer owned by this project leave the branch.", id, resolution.URL)
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

func promotionProjectSubject(id string) promotionSubject {
	return promotionSubject{what: fmt.Sprintf("project %q", id), find: "'n8n project list'"}
}
