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
		Long: "Discover GitLab repositories or branches using a saved provider with gitConnection:discover.\n" +
			"Start with 'n8n promotion provider list', then repositories, then branches.\n" +
			"Pass the raw fullPath (for example org/repo) or GitLab project ID to branches;\n" +
			"the CLI escapes it. Use remoteUrl from repositories as a connection target.\n" +
			"No saved connection is required and no content changes. Other provider types\n" +
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
			"Start with 'n8n project list' for the project ID. The project's own connection\n" +
			"takes precedence over the instance connection; clone the chosen direction\n" +
			"first with 'n8n promotion checkout clone'. Apply requires gitConnection:pull;\n" +
			"promote requires gitConnection:push. Text lists workflow IDs, statuses, and\n" +
			"dependency counts; JSON includes changes, source, and nullable commitSha.\n" +
			"Review the IDs before project apply or promote. Save apply JSON for\n" +
			"--expected-from to reject a branch that moves after review.",
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
			fmt.Fprintf(tw, "Config:\t%s\nBranch:\t%s\nCommit:\t%s\n", result.Source.ConfigID, result.Source.BranchName, commit)
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

type promotionSelectionFlags struct {
	instance                    instanceFlags
	input, output, expectedFrom string
	workflowIDs                 []string
	source                      n8n.PromotionExpectedSource
	commit                      string
	yes, destructive            bool
}

// Separate commands keep the required source and selection explicit at continue time.
func newPromotionSelectionCommand(opts Options, action string, project bool) *cobra.Command {
	var f promotionSelectionFlags
	target, prefix := "project", "n8n promotion project "
	if !project {
		target, prefix = "connection", "n8n promotion "
	}
	isContinue := action == "apply-continue"
	isPromote := action == "promote"
	short := "Apply selected workflow changes from a project's branch"
	if isContinue {
		short = "Continue a reviewed " + target + " apply after binding setup"
	}
	if isPromote {
		short = "Promote selected project workflows to the remote branch"
	}
	long := "Start with 'n8n promotion project changes PROJECT_ID apply' to review workflow IDs.\n" +
		"The project's own connection resolves first, otherwise the instance connection.\n" +
		"The apply direction must be cloned. Selected IDs on the branch are imported;\n" +
		"selected workflows owned by this project whose IDs are absent from the branch\n" +
		"are removed. Unselected content stays unchanged. Duplicate or invalid IDs\n" +
		"reject the whole request. Requires gitConnection:pull.\n"
	if !project {
		long = "Continue a full instance-connection apply after resolving its preflight items.\n" +
			"Start with 'n8n promotion apply CONNECTION_ID --output json'. The apply\n" +
			"checkout must be cloned. Every team project is synchronized: workflows,\n" +
			"folders, credentials, tables, variables, and tags can change or be removed.\n" +
			"Requires gitConnection:pull and the same permissions as the initial apply.\n"
	}
	if isPromote {
		long = "Start with 'n8n promotion project changes PROJECT_ID promote' to review IDs.\n" +
			"Push the current state of selected workflows to the project's resolved\n" +
			"promotion connection (its own, otherwise the instance connection). Clone\n" +
			"the promote direction first. Archived workflows remain archived on the\n" +
			"branch; workflows no longer owned by the project leave the branch. Unknown\n" +
			"deletions reject the whole request. Requires gitConnection:push, plus\n" +
			"variable:list when workflows use variables. Commit message is optional;\n" +
			"the server supplies a default. Force pushing is not supported.\n"
	} else {
		long += "Use --expected-from FILE|- with changes JSON or a blocked apply result,\n" +
			"or supply all three --expected-config-id, --expected-branch, --expected-commit\n" +
			"flags (full lowercase 40- or 64-digit hex SHA). A null commit cannot be used.\n"
		if isContinue {
			long += "Expected source is required; use the blocked result's configId and git.\n" +
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
		long += "Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 other error. Only\n" +
			"applied writes content. Resolve blocked bindings before apply-continue;\n" +
			"review source-changed results again. JSON prints the full status result;\n" +
			"text shows preflight details or counts and warnings. Hints go to stderr.\n"
	}
	if project {
		long += "\nPass repeated --workflow-id flags, or use a strict JSON request.\n"
	}
	long += "Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are\n" +
		"mutually exclusive. Interactive runs ask for confirmation; non-interactive\n" +
		"runs and stdin input require --yes. JSON returns the server result; text\n" +
		"shows counts and the commit after success."
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
		cmd.Flags().StringVar(&f.source.ConfigID, "expected-config-id", "", "reviewed config ID, 1 to 36 characters (default: omitted; requires expected branch and commit)")
		cmd.Flags().StringVar(&f.source.BranchName, "expected-branch", "", "reviewed branch name (default: omitted; requires expected config ID and commit)")
		cmd.Flags().StringVar(&f.source.CommitSHA, "expected-commit", "", "reviewed lowercase full 40- or 64-digit hex commit SHA (default: omitted; requires expected config ID and branch)")
		documentFlags = append(documentFlags, "expected-from", "expected-config-id", "expected-branch", "expected-commit")
	}
	if isContinue {
		cmd.Flags().BoolVar(&f.destructive, "confirm-destructive-changes", false, "allow all destructive changes at continue time, including unreviewed column value loss and, for full apply, team-project deletion (default: false)")
		documentFlags = append(documentFlags, "confirm-destructive-changes")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(f.output); err != nil {
			return err
		}
		if strings.TrimSpace(args[0]) != args[0] || args[0] == "" || utf8.RuneCountInString(args[0]) > 36 {
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
		var source *n8n.PromotionExpectedSource
		if !isPromote && f.input == "" {
			explicit := cmd.Flags().Changed("expected-config-id") || cmd.Flags().Changed("expected-branch") || cmd.Flags().Changed("expected-commit")
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
			r := &n8n.ContinueApplyProjectSelectionRequest{WorkflowIDs: f.workflowIDs, ConfirmDestructiveChanges: destructive}
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
		default:
			request = &n8n.ApplyProjectSelectionRequest{WorkflowIDs: f.workflowIDs, ExpectedSource: source}
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
	if raw, ok := fields["source"]; ok {
		var source map[string]json.RawMessage
		if err := decode(raw, &source); err != nil {
			return nil, err
		}
		for key, dst := range map[string]*string{"configId": &result.ConfigID, "branchName": &result.BranchName} {
			if err := decode(source[key], dst); err != nil {
				return nil, fmt.Errorf("source.%s: %w", key, err)
			}
		}
		if err := decode(fields["commitSha"], &result.CommitSHA); err != nil {
			return nil, fmt.Errorf("commitSha: %w", err)
		}
	} else {
		var status string
		if err := decode(fields["status"], &status); err != nil || status != n8n.PromotionApplyStatusBlocked {
			return nil, fmt.Errorf("--expected-from requires changes JSON or a blocked apply result")
		}
		if err := decode(fields["configId"], &result.ConfigID); err != nil {
			return nil, fmt.Errorf("configId: %w", err)
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
	action := "project apply"
	question := fmt.Sprintf("Apply selected workflow changes to project %q on %s? Selected workflows absent from the branch are deleted; selected content and dependencies are overwritten.", id, resolution.URL)
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
		action, subject = "apply-continue", promotionConnectionSubject(id)
		question = fmt.Sprintf("Continue applying connection %q into %s? Every team project's workflows, folders, credentials, tables, variables, and tags may be overwritten or removed.", id, resolution.URL)
		if r.ConfirmDestructiveChanges != nil && *r.ConfirmDestructiveChanges {
			question += " ALL team-project deletions with everything in them and destructive data-table column changes at continue time are allowed, including unreviewed changes; column values are lost."
		}
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	var result *n8n.ApplyPromotionResult
	switch r := request.(type) {
	case *n8n.PromoteProjectSelectionRequest:
		promoted, err := client.PromoteProjectSelection(ctx, id, *r)
		if err != nil {
			return promotionAPIError(err, resolution, action, subject)
		}
		if f.output == outputJSON {
			return writeJSON(opts.Streams.Out, promoted)
		}
		return writePromotionPromoteResult(opts, resolution, promoted)
	case *n8n.ApplyProjectSelectionRequest:
		result, err = client.ApplyProjectSelection(ctx, id, *r)
	case *n8n.ContinueApplyProjectSelectionRequest:
		result, err = client.ContinueApplyProjectSelection(ctx, id, *r)
	case *n8n.ContinueApplyPromotionRequest:
		result, err = client.ContinueApplyPackage(ctx, id, *r)
	}
	if err != nil {
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
	return finishPromotionApplyTarget(opts, id, result, strings.HasPrefix(action, "project "))
}

func promotionProjectSubject(id string) promotionSubject {
	return promotionSubject{what: fmt.Sprintf("project %q", id), find: "'n8n project list'"}
}
