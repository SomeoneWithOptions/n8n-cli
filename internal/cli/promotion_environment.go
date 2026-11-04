package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// Environments are the n8n 2.46+ generation of connections: one remote for one
// direction. Older instances answer 404 here and serve 'promotion connection'.

const (
	promotionEnvironmentAlt = "an instance older than n8n 2.46 serves connections instead: use 'n8n promotion connection'"
	promotionConnectionAlt  = "n8n 2.46 and later replaced connections with environments: use 'n8n promotion environment'"
)

// promotionEnvironmentReadOnly are the response fields an environment write
// never sends. A document copied from 'environment get --output json' carries
// them, so they are dropped with a stderr note instead of refused.
var promotionEnvironmentReadOnly = []string{"id", "provider", "checkout", "createdAt", "updatedAt"}

// promotionEnvironmentFixed are fields only a create sets: an update cannot
// change scope or direction, so they are read-only there.
var promotionEnvironmentFixed = []string{"scope", "direction"}

func newPromotionEnvironmentCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Manage promotion environments (n8n 2.46 and later)",
		Long: "Manage promotion environments: one remote, one direction (apply or\n" +
			"promote), its settings, and its local checkout. n8n 2.46 replaced\n" +
			"connections with environments; on older instances use 'n8n promotion\n" +
			"connection' instead.\n\n" +
			"Start with 'list' to find the environment ID, then 'get' to read it,\n" +
			"'create' to add one on a provider, or 'update' to change its name,\n" +
			"provider, remote, or settings. 'connect' prepares the local checkout\n" +
			"and 'disconnect' removes it. Link team projects to a projects-scope\n" +
			"environment under 'project'. The instance can have one environment for\n" +
			"each direction.\n\n" +
			"Sync the whole instance through the connected instance-scope\n" +
			"environment: 'promote' pushes every team project to a promote\n" +
			"environment; 'apply' imports an apply environment's branch, and\n" +
			"'apply-continue' finishes an apply that preflight blocked. For selected\n" +
			"workflows of one project use 'n8n promotion project changes', 'apply',\n" +
			"and 'promote'.\n\n" +
			"Deleting an environment removes its project links and local checkout;\n" +
			"its provider is kept and nothing in Git changes.",
		Example: "  n8n promotion environment list\n" +
			"  n8n promotion environment get ENVIRONMENT_ID\n" +
			"  n8n promotion environment create --input environment.json\n" +
			"  n8n promotion environment connect ENVIRONMENT_ID\n" +
			"  n8n promotion environment apply ENVIRONMENT_ID --yes\n" +
			"  n8n promotion environment promote ENVIRONMENT_ID --commit-message \"sync\" --yes\n" +
			"  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionEnvironmentListCommand(opts),
		newPromotionEnvironmentCreateCommand(opts),
		newPromotionEnvironmentGetCommand(opts),
		newPromotionEnvironmentUpdateCommand(opts),
		newPromotionEnvironmentDeleteCommand(opts),
		newPromotionEnvironmentConnectCommand(opts),
		newPromotionEnvironmentDisconnectCommand(opts),
		newPromotionEnvironmentProjectCommand(opts),
		newPromotionEnvironmentPromoteCommand(opts),
		newPromotionSelectionCommand(opts, "apply", promotionTargetEnvironment),
		newPromotionSelectionCommand(opts, "apply-continue", promotionTargetEnvironment),
	)
	return cmd
}

type promotionEnvironmentListFlags struct {
	instance   instanceFlags
	limit      int
	cursor     string
	scope      string
	direction  string
	providerID string
	all        bool
	output     string
}

func newPromotionEnvironmentListCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List promotion environments",
		Long: "List the promotion environments visible to the selected credential,\n" +
			"one cursor-paginated page at a time. Filter with --scope (instance or\n" +
			"projects), --direction (apply or promote), and --provider-id; filters\n" +
			"are preserved across --all. Use --all to follow every cursor, capped at\n" +
			"10,000 environments.\n\n" +
			"Use a returned ID with get, update, delete, connect, disconnect, and\n" +
			"project. Requires gitConnection:list.\n\n" + cursorCollectionHelp,
		Example: "  n8n promotion environment list\n" +
			"  n8n promotion environment list --scope instance --direction apply\n" +
			"  n8n promotion environment list --provider-id PROVIDER_ID --output json\n" +
			"  n8n promotion environment list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionEnvironmentList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "environments per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().StringVar(&f.scope, "scope", "", "filter by environment scope: instance or projects (default: all scopes)")
	cmd.Flags().StringVar(&f.direction, "direction", "", "filter by direction: apply or promote (default: both directions)")
	cmd.Flags().StringVar(&f.providerID, "provider-id", "", "filter by provider ID from 'n8n promotion provider list' (default: all providers)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 environments; hitting it warns and sets JSON collection.truncated)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all)")
	return cmd
}

func runPromotionEnvironmentList(ctx context.Context, opts Options, f promotionEnvironmentListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListPromotionEnvironmentsOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Scope:       strings.TrimSpace(f.scope),
		Direction:   strings.TrimSpace(f.direction),
		ProviderID:  strings.TrimSpace(f.providerID),
	}
	if f.scope != "" && listOpts.Scope == "" {
		return fmt.Errorf("--scope must not be blank: use instance or projects, or omit it for all scopes")
	}
	if f.direction != "" && listOpts.Direction == "" {
		return fmt.Errorf("--direction must not be blank: use apply or promote, or omit it for both directions")
	}
	if f.providerID != "" && listOpts.ProviderID == "" {
		return fmt.Errorf("--provider-id must not be blank: pass a provider ID from 'n8n promotion provider list'")
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var page n8n.Page[n8n.PromotionEnvironment]
	var collection *listCollection
	if f.all {
		collect := func(ctx context.Context, base n8n.ListOptions) (n8n.Page[n8n.PromotionEnvironment], error) {
			next := listOpts
			next.ListOptions = base
			return client.ListPromotionEnvironments(ctx, next)
		}
		page, collection, err = collectPage(ctx, collect, listOpts.ListOptions)
	} else {
		page, err = client.ListPromotionEnvironments(ctx, listOpts)
	}
	if err != nil {
		return promotionAPIError(err, resolution, "environment list", promotionSubject{alt: promotionEnvironmentAlt})
	}
	if f.output == outputJSON {
		err = writePageJSON(opts.Streams.Out, page, collection)
	} else {
		err = writePromotionEnvironmentListText(opts, resolution, page)
	}
	return finishCollection(opts.Streams.Err, collection, err)
}

func writePromotionEnvironmentListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.PromotionEnvironment]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Environments:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tSCOPE\tDIRECTION\tBRANCH\tREMOTE\tPROVIDER")
		for _, environment := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				emptyDash(environment.ID), emptyDash(environment.Name),
				emptyDash(environment.Scope), emptyDash(environment.Direction),
				emptyDash(promotionEnvironmentBranch(environment.Settings)),
				emptyDash(environment.Remote.RemoteURL), emptyDash(environment.Provider.Name))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No promotion environments were returned. Create one with 'n8n promotion environment create --input environment.json'.")
	}
	return nil
}

type promotionEnvironmentCreateFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func newPromotionEnvironmentCreateCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentCreateFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a promotion environment",
		Long: "Create one promotion environment from strict JSON supplied by --input.\n" +
			"Required fields are name (at most 128 characters), scope (instance or\n" +
			"projects), providerId, remote {\"schemaVersion\":1,\"remoteUrl\":\"...\"},\n" +
			"direction (apply or promote), and settings matching the direction:\n" +
			"apply {\"schemaVersion\":1,\"branchName\":\"main\"} or promote\n" +
			"{\"schemaVersion\":1,\"baseBranchName\":\"main\",\n" +
			"\"createBranchOnPromotion\":false}. The instance can have one\n" +
			"environment for each direction.\n\n" +
			"On a gitlab provider the remote URL must belong to the provider: the\n" +
			"environment stores only the repository path and derives remoteUrl from\n" +
			"the provider base URL. Read-only fields from 'environment get --output\n" +
			"json' (id, provider, checkout, createdAt, updatedAt,\n" +
			"remote.repositoryPath) are dropped with a note on stderr; other unknown\n" +
			"fields and trailing JSON documents are refused before transport.\n" +
			"Requires gitConnection:create. Follow with 'n8n promotion environment\n" +
			"connect ENVIRONMENT_ID'.",
		Example: "  n8n promotion environment create --input environment.json\n" +
			"  printf '%s\\n' '{\"name\":\"prod\",\"scope\":\"instance\",\"providerId\":\"PROVIDER_ID\",\"remote\":{\"schemaVersion\":1,\"remoteUrl\":\"git@example.com:org/repo.git\"},\"direction\":\"apply\",\"settings\":{\"schemaVersion\":1,\"branchName\":\"main\"}}' | n8n promotion environment create --input -",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionEnvironmentCreate(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "promotion environment JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the environment returned by the API)")
	return cmd
}

func runPromotionEnvironmentCreate(ctx context.Context, opts Options, f promotionEnvironmentCreateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreatePromotionEnvironmentRequest
	if _, err := readPromotionEnvironmentDocument(opts, f.input, false, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	created, err := client.CreatePromotionEnvironment(ctx, request)
	if err != nil {
		subject := promotionProviderSubject(request.ProviderID)
		subject.alt = promotionEnvironmentAlt
		return promotionAPIError(err, resolution, "environment create", subject)
	}
	return writePromotionEnvironmentResult(opts, resolution, "Created", created, f.output)
}

type promotionEnvironmentGetFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionEnvironmentGetCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentGetFlags
	cmd := &cobra.Command{
		Use:   "get <environment-id>",
		Short: "Show one promotion environment",
		Long: "Show one promotion environment by ID: its name, scope, direction,\n" +
			"remote, provider, settings, local checkout state, and timestamps.\n" +
			"Secrets are never returned. Find the ID with 'n8n promotion environment\n" +
			"list'. The --output json document can be edited and passed to 'update'\n" +
			"as is: its read-only fields are dropped. Requires gitConnection:read.",
		Example: "  n8n promotion environment get ENVIRONMENT_ID\n" +
			"  n8n promotion environment get ENVIRONMENT_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the environment object)")
	return cmd
}

func runPromotionEnvironmentGet(ctx context.Context, opts Options, id string, f promotionEnvironmentGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	environment, err := client.GetPromotionEnvironment(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "environment get", promotionEnvironmentSubject(id))
	}
	return writePromotionEnvironmentResult(opts, resolution, "Environment", environment, f.output)
}

type promotionEnvironmentUpdateFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newPromotionEnvironmentUpdateCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <environment-id>",
		Short: "Update a promotion environment",
		Long: "Update one promotion environment from strict JSON supplied by --input.\n" +
			"Only the fields present are changed; at least one of name, providerId,\n" +
			"remote, or settings is required. A remote replaces the whole saved\n" +
			"remote and settings replace the whole saved settings, which must match\n" +
			"the environment's direction (promote always needs\n" +
			"createBranchOnPromotion). Scope and direction cannot change.\n\n" +
			"Start from 'n8n promotion environment get ID --output json': read-only\n" +
			"fields (id, scope, direction, provider, checkout, createdAt, updatedAt,\n" +
			"remote.repositoryPath) are dropped with a note on stderr, and a\n" +
			"direction in the document is used to check the settings first. A 409\n" +
			"means the provider changed during the update: reload and retry.\n\n" +
			"A new remote or branch can leave the local checkout stale, so\n" +
			"interactive runs ask for confirmation; non-interactive runs and\n" +
			"--input - require --yes. Requires gitConnection:update.",
		Example: "  n8n promotion environment update ENVIRONMENT_ID --input environment-update.json\n" +
			"  printf '%s\\n' '{\"name\":\"staging\"}' | n8n promotion environment update ENVIRONMENT_ID --input - --yes\n" +
			"  n8n promotion environment update ENVIRONMENT_ID --input environment.json --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "promotion environment update JSON file path, or - for stdin (required; maximum 1 MiB; - requires --yes)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the environment update without prompting (required when stdin is not interactive or with --input -)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the environment returned by the API)")
	return cmd
}

func runPromotionEnvironmentUpdate(ctx context.Context, opts Options, id string, f promotionEnvironmentUpdateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the update confirmation: pass --yes after reviewing the document")
	}
	var request n8n.UpdatePromotionEnvironmentRequest
	direction, err := readPromotionEnvironmentDocument(opts, f.input, true, &request)
	if err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Settings != nil && direction != "" {
		if err := request.Settings.Validate(direction); err != nil {
			return err
		}
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Update promotion environment %q on %s (%s)? A new remote, provider, or branch can leave its local checkout stale until 'connect' runs again.", id, resolution.URL, promotionEnvironmentChanges(request))
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	updated, err := client.UpdatePromotionEnvironment(ctx, id, request)
	if err != nil {
		return promotionAPIError(err, resolution, "environment update", promotionEnvironmentSubject(id))
	}
	return writePromotionEnvironmentResult(opts, resolution, "Updated", updated, f.output)
}

// promotionEnvironmentChanges names the fields one update replaces, for the
// confirmation question.
func promotionEnvironmentChanges(request n8n.UpdatePromotionEnvironmentRequest) string {
	var fields []string
	if request.Name != nil {
		fields = append(fields, "name")
	}
	if request.ProviderID != nil {
		fields = append(fields, "provider")
	}
	if request.Remote != nil {
		fields = append(fields, "remote")
	}
	if request.Settings != nil {
		fields = append(fields, "settings")
	}
	return "changes " + strings.Join(fields, ", ")
}

type promotionEnvironmentConfirmFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionEnvironmentDeleteCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentConfirmFlags
	cmd := &cobra.Command{
		Use:   "delete <environment-id>",
		Short: "Permanently delete a promotion environment",
		Long: "Permanently delete one promotion environment with its project links and\n" +
			"local checkout. Its provider is kept and nothing in Git changes, but\n" +
			"linked projects fall back to the instance environment, and promote or\n" +
			"apply for that direction stops working until an environment is\n" +
			"created and connected again. This cannot be undone.\n\n" +
			"To keep the environment and drop only the checkout, use 'n8n promotion\n" +
			"environment disconnect' instead. Interactive runs ask for\n" +
			"confirmation; non-interactive runs require --yes. Requires\n" +
			"gitConnection:delete.",
		Example: "  n8n promotion environment delete ENVIRONMENT_ID\n" +
			"  n8n promotion environment delete ENVIRONMENT_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent environment, project link, and checkout deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deletion acknowledgement)")
	return cmd
}

func runPromotionEnvironmentDelete(ctx context.Context, opts Options, id string, f promotionEnvironmentConfirmFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete promotion environment %q from %s? Its project links and local checkout go away; the provider and Git stay unchanged.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeletePromotionEnvironment(ctx, id); err != nil {
		return promotionAPIError(err, resolution, "environment delete", promotionEnvironmentSubject(id))
	}
	return writePromotionMutation(opts, resolution, promotionMutation{Action: "deleted", EnvironmentID: id}, f.output)
}

type promotionEnvironmentConnectFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionEnvironmentConnectCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentConnectFlags
	cmd := &cobra.Command{
		Use:   "connect <environment-id>",
		Short: "Prepare the local checkout of a promotion environment",
		Long: "Prepare one promotion environment on the instance that handles the\n" +
			"request; for a Git provider this clones the configured branch into\n" +
			"local storage. Connecting one environment does not make another\n" +
			"ready.\n\n" +
			"Connecting is safe to repeat and changes no project content by itself,\n" +
			"so it needs no confirmation. Connect again after 'get' reports a\n" +
			"checkout that does not match the environment. Requires\n" +
			"gitConnection:clone.",
		Example: "  n8n promotion environment connect ENVIRONMENT_ID\n" +
			"  n8n promotion environment connect ENVIRONMENT_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentConnect(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the checkout returned by the API)")
	return cmd
}

func runPromotionEnvironmentConnect(ctx context.Context, opts Options, id string, f promotionEnvironmentConnectFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	checkout, err := client.ConnectPromotionEnvironment(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "environment connect", promotionEnvironmentSubject(id))
	}
	return writePromotionEnvironmentCheckout(opts, resolution, "Connected", checkout, f.output)
}

func newPromotionEnvironmentDisconnectCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentConfirmFlags
	cmd := &cobra.Command{
		Use:   "disconnect <environment-id>",
		Short: "Remove the local checkout of a promotion environment",
		Long: "Remove the local checkout of one promotion environment. The\n" +
			"environment, its credentials, its trusted SSH host keys, and its\n" +
			"project links are kept, but promote and apply through it stop working\n" +
			"until 'connect' runs again.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:clone.",
		Example: "  n8n promotion environment disconnect ENVIRONMENT_ID\n" +
			"  n8n promotion environment disconnect ENVIRONMENT_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentDisconnect(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm checkout removal without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the checkout returned by the API)")
	return cmd
}

func runPromotionEnvironmentDisconnect(ctx context.Context, opts Options, id string, f promotionEnvironmentConfirmFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Remove the local checkout of promotion environment %q on %s? The environment stays, but promote and apply through it stop working until it is connected again.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	checkout, err := client.DisconnectPromotionEnvironment(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "environment disconnect", promotionEnvironmentSubject(id))
	}
	return writePromotionEnvironmentCheckout(opts, resolution, "Disconnected", checkout, f.output)
}

// Environment promote. Apply and apply-continue share the selection command,
// which already carries the expected-source and --input handling they need.

type promotionEnvironmentPromoteFlags struct {
	instance    instanceFlags
	commit      string
	force       bool
	yes         bool
	output      string
	idempotency idempotencyFlag
}

func newPromotionEnvironmentPromoteCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentPromoteFlags
	cmd := &cobra.Command{
		Use:   "promote <environment-id>",
		Short: "Promote team projects to an environment's Git remote",
		Long: "Export every team project, commit it, and push through an\n" +
			"instance-scope promote environment (n8n 2.46 and later) to its base\n" +
			"branch or a new timestamped branch (the result names it). Before 2.46\n" +
			"use 'n8n promotion promote' with a connection. Start with 'n8n\n" +
			"promotion environment list --direction promote' and connect it first\n" +
			"with 'n8n promotion environment connect'. Personal projects are\n" +
			"ignored, and project links do not select what is promoted; for one\n" +
			"project's workflows use 'n8n promotion project promote'. A workflow on\n" +
			"the branch that moved to a personal project, or to another team project\n" +
			"while its old project still exists, is refused with 400.\n\n" +
			"--commit-message is optional (1 to 1000 characters; default: a\n" +
			"server-generated message). Without --force a promote the remote would\n" +
			"reject is refused with 409 and nothing is pushed; --force pushes anyway\n" +
			"and rewrites remote history. This mutates the remote branch.\n" +
			"Interactive runs ask for confirmation and non-interactive runs require\n" +
			"--yes. Requires gitConnection:push, plus variable:list when workflows\n" +
			"reference variables.\n\n" +
			"Text shows the commit, branch, counts, and any review the Git host\n" +
			"opened; JSON is the full result. Advisory warnings go to stderr in both\n" +
			"formats.\n\n" +
			idempotencyHelp,
		Example: "  n8n promotion environment promote ENVIRONMENT_ID --commit-message \"sync workflows\"\n" +
			"  n8n promotion environment promote ENVIRONMENT_ID --yes\n" +
			"  n8n promotion environment promote ENVIRONMENT_ID --commit-message \"sync\" --force --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentPromote(cmd.Context(), opts, args[0], f, cmd.Flags().Changed("commit-message"))
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.commit, "commit-message", "", "commit message for the promote, 1 to 1000 characters (default: server-generated message)")
	cmd.Flags().BoolVar(&f.force, "force", false, "promote even when the remote would reject it, rewriting remote history (default: reject a rejected promote with 409)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm promoting every team project to the remote without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the promote result with counts, commit, and any review and warnings)")
	f.idempotency.register(cmd)
	return cmd
}

func runPromotionEnvironmentPromote(ctx context.Context, opts Options, id string, f promotionEnvironmentPromoteFlags, hasCommit bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	request := n8n.PromotePromotionEnvironmentRequest{Force: f.force}
	message := "a server-generated commit message"
	if hasCommit {
		request.CommitMessage = &f.commit
		message = fmt.Sprintf("commit message %q", f.commit)
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if err := f.idempotency.validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Promote every team project on %s to the remote of promotion environment %q with %s? Team projects are committed and pushed.", resolution.URL, id, message)
	if request.Force {
		question = fmt.Sprintf("Force-promote every team project on %s to the remote of promotion environment %q with %s? Remote history is rewritten.", resolution.URL, id, message)
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.PromotePromotionEnvironment(ctx, id, request, f.idempotency.resolve(opts.Streams.Err)...)
	if err != nil {
		if refused := f.idempotency.refusal(err, resolution); refused != nil {
			return refused
		}
		return promotionAPIError(err, resolution, "environment promote", promotionEnvironmentSubject(id))
	}
	return writePromotionPromoteOutput(opts, resolution, result, f.output)
}

// Environment projects.

func newPromotionEnvironmentProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects linked to a promotion environment",
		Long: "Manage which team projects a projects-scope promotion environment\n" +
			"syncs. A project can have one environment for each direction; a\n" +
			"direction without a project environment uses the instance environment.\n" +
			"Start with 'list' to see the linked project IDs, then 'add' a project\n" +
			"or 'remove' one.\n\n" +
			"Removing unlinks the project; its instance content is untouched.\n" +
			"Project IDs come from 'n8n project list'. Listing needs\n" +
			"gitConnection:read; adding and removing need\n" +
			"gitConnection:manageProjects.",
		Example: "  n8n promotion environment project list ENVIRONMENT_ID\n" +
			"  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID\n" +
			"  n8n promotion environment project remove ENVIRONMENT_ID PROJECT_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionEnvironmentProjectListCommand(opts),
		newPromotionEnvironmentProjectAddCommand(opts),
		newPromotionEnvironmentProjectRemoveCommand(opts),
	)
	return cmd
}

func newPromotionEnvironmentProjectListCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentConnectFlags
	cmd := &cobra.Command{
		Use:   "list <environment-id>",
		Short: "List projects linked to a promotion environment",
		Long: "List the team projects linked to one projects-scope promotion\n" +
			"environment. Find the environment ID with 'n8n promotion environment\n" +
			"list --scope projects'. Requires gitConnection:read.",
		Example: "  n8n promotion environment project list ENVIRONMENT_ID\n" +
			"  n8n promotion environment project list ENVIRONMENT_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentProjectList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the projectIds object)")
	return cmd
}

func runPromotionEnvironmentProjectList(ctx context.Context, opts Options, id string, f promotionEnvironmentConnectFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	projects, err := client.ListPromotionEnvironmentProjects(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "environment project list", promotionEnvironmentSubject(id))
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, projects)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Environment:\t%s\n", id)
	fmt.Fprintf(tw, "Projects:\t%d\n", len(projects.ProjectIDs))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(projects.ProjectIDs) > 0 {
		fmt.Fprintln(tw, "\nPROJECT ID")
		for _, projectID := range projects.ProjectIDs {
			fmt.Fprintf(tw, "%s\n", emptyDash(projectID))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(projects.ProjectIDs) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No projects are linked. Add one with 'n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID'.")
	}
	return nil
}

func newPromotionEnvironmentProjectAddCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentConnectFlags
	cmd := &cobra.Command{
		Use:   "add <environment-id> <project-id>",
		Short: "Link a project to a promotion environment",
		Long: "Link one team project to one projects-scope promotion environment, so\n" +
			"project-level sync in that direction uses this environment instead of\n" +
			"the instance environment. A project can have one environment for each\n" +
			"direction: adding a project that already has one for this direction\n" +
			"answers 409. Requires gitConnection:manageProjects.",
		Example: "  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID\n" +
			"  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentProjectAdd(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the project link returned by the API)")
	return cmd
}

func runPromotionEnvironmentProjectAdd(ctx context.Context, opts Options, id, projectID string, f promotionEnvironmentConnectFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentProjectArgs(id, projectID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	link, err := client.AddProjectToPromotionEnvironment(ctx, id, projectID)
	if err != nil {
		return promotionAPIError(err, resolution, "environment project add", promotionEnvironmentProjectSubject(id, projectID))
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, link)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Added:\t%s\n", link.ProjectID)
	fmt.Fprintf(tw, "Environment:\t%s\n", link.EnvironmentID)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func newPromotionEnvironmentProjectRemoveCommand(opts Options) *cobra.Command {
	var f promotionEnvironmentConfirmFlags
	cmd := &cobra.Command{
		Use:   "remove <environment-id> <project-id>",
		Short: "Unlink a project from a promotion environment",
		Long: "Unlink one project from one promotion environment. The project itself\n" +
			"and its instance content are untouched; only the sync link goes away,\n" +
			"so project-level sync in that direction falls back to the instance\n" +
			"environment. Re-link it later with 'n8n promotion environment project\n" +
			"add'.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:manageProjects.",
		Example: "  n8n promotion environment project remove ENVIRONMENT_ID PROJECT_ID\n" +
			"  n8n promotion environment project remove ENVIRONMENT_ID PROJECT_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionEnvironmentProjectRemove(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm project unlinking without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the unlinking acknowledgement)")
	return cmd
}

func runPromotionEnvironmentProjectRemove(ctx context.Context, opts Options, id, projectID string, f promotionEnvironmentConfirmFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validatePromotionEnvironmentProjectArgs(id, projectID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Unlink project %q from promotion environment %q on %s? The project content stays; project-level sync in that direction falls back to the instance environment.", projectID, id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.RemoveProjectFromPromotionEnvironment(ctx, id, projectID); err != nil {
		return promotionAPIError(err, resolution, "environment project remove", promotionEnvironmentProjectSubject(id, projectID))
	}
	return writePromotionMutation(opts, resolution, promotionMutation{Action: "project removed", EnvironmentID: id, ProjectID: projectID}, f.output)
}

// Environment helpers.

func validatePromotionEnvironmentArg(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("environment ID is required: pass the ID from 'n8n promotion environment list'")
	}
	if err := n8n.ValidatePromotionEnvironmentID(id); err != nil {
		return fmt.Errorf("%w: pass the ID from 'n8n promotion environment list'", err)
	}
	return nil
}

func validatePromotionEnvironmentProjectArgs(id, projectID string) error {
	if err := validatePromotionEnvironmentArg(id); err != nil {
		return err
	}
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("project ID is required: pass the ID from 'n8n project list'")
	}
	if strings.TrimSpace(projectID) != projectID {
		return fmt.Errorf("project ID must not start or end with whitespace")
	}
	return nil
}

// readPromotionEnvironmentDocument reads one strict environment document into
// dst after dropping the read-only response fields, so a document copied from
// 'environment get --output json' round-trips. Dropped fields are named on
// stderr. For an update it also drops scope and direction, returning a valid
// direction so the caller can check the settings shape against it.
func readPromotionEnvironmentDocument(opts Options, path string, update bool, dst any) (string, error) {
	var doc map[string]json.RawMessage
	if err := readPromotionDocument(opts, path, &doc); err != nil {
		return "", err
	}
	if doc == nil {
		return "", fmt.Errorf("decode promotion JSON: the document must be a JSON object")
	}
	readOnly := promotionEnvironmentReadOnly
	var direction string
	if update {
		readOnly = append(append([]string(nil), readOnly...), promotionEnvironmentFixed...)
		if raw, ok := doc["direction"]; ok {
			var value string
			if json.Unmarshal(raw, &value) == nil && (value == n8n.PromotionDirectionApply || value == n8n.PromotionDirectionPromote) {
				direction = value
			}
		}
	}
	var dropped []string
	for _, field := range readOnly {
		if _, ok := doc[field]; ok {
			delete(doc, field)
			dropped = append(dropped, field)
		}
	}
	if raw, ok := doc["remote"]; ok {
		var remote map[string]json.RawMessage
		if json.Unmarshal(raw, &remote) == nil {
			if _, ok := remote["repositoryPath"]; ok {
				delete(remote, "repositoryPath")
				dropped = append(dropped, "remote.repositoryPath")
				encoded, err := json.Marshal(remote)
				if err != nil {
					return "", err
				}
				doc["remote"] = encoded
			}
		}
	}
	if len(dropped) > 0 {
		fmt.Fprintf(opts.Streams.Err, "Not sent (read-only): %s\n", strings.Join(dropped, ", "))
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return "", fmt.Errorf("decode promotion JSON: %w", err)
	}
	return direction, nil
}

func promotionEnvironmentBranch(settings n8n.PromotionEnvironmentSettings) string {
	if settings.BranchName != "" {
		return settings.BranchName
	}
	return settings.BaseBranchName
}

func writePromotionEnvironmentResult(opts Options, resolution config.Resolution, action string, environment *n8n.PromotionEnvironment, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, environment)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, environment.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", emptyDash(environment.Name))
	fmt.Fprintf(tw, "Scope:\t%s\n", emptyDash(environment.Scope))
	fmt.Fprintf(tw, "Direction:\t%s\n", emptyDash(environment.Direction))
	fmt.Fprintf(tw, "Remote:\t%s\n", emptyDash(environment.Remote.RemoteURL))
	if environment.Remote.RepositoryPath != "" {
		fmt.Fprintf(tw, "Repository path:\t%s\n", environment.Remote.RepositoryPath)
	}
	fmt.Fprintf(tw, "Provider:\t%s\n", emptyDash(environment.Provider.Name))
	if environment.Settings.BranchName != "" {
		fmt.Fprintf(tw, "Branch:\t%s\n", environment.Settings.BranchName)
	}
	if environment.Settings.BaseBranchName != "" {
		fmt.Fprintf(tw, "Base branch:\t%s\n", environment.Settings.BaseBranchName)
	}
	if create := environment.Settings.CreateBranchOnPromotion; create != nil {
		fmt.Fprintf(tw, "New branch per promote:\t%v\n", *create)
	}
	if checkout := environment.Checkout; checkout != nil {
		fmt.Fprintf(tw, "Checkout:\t%s\n", promotionEnvironmentCheckoutState(*checkout))
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if environment.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", environment.CreatedAt)
	}
	if environment.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", environment.UpdatedAt)
	}
	return tw.Flush()
}

// promotionEnvironmentCheckoutState says whether the environment is connected
// and whether the checkout still matches it (a stale one needs 'connect').
func promotionEnvironmentCheckoutState(checkout n8n.PromotionEnvironmentCheckoutState) string {
	switch {
	case !checkout.HasCheckout:
		return "not connected"
	case checkout.MatchesEnvironment:
		return "connected"
	default:
		return "connected, does not match the environment; connect again"
	}
}

func writePromotionEnvironmentCheckout(opts Options, resolution config.Resolution, action string, checkout *n8n.PromotionEnvironmentCheckout, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, checkout)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, checkout.EnvironmentID)
	fmt.Fprintf(tw, "Direction:\t%s\n", emptyDash(checkout.Direction))
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(checkout.BranchName))
	fmt.Fprintf(tw, "Checkout:\t%v\n", checkout.HasCheckout)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func promotionEnvironmentSubject(id string) promotionSubject {
	return promotionSubject{
		what: fmt.Sprintf("promotion environment %q", id),
		find: "'n8n promotion environment list'",
		alt:  promotionEnvironmentAlt,
	}
}

func promotionEnvironmentProjectSubject(id, projectID string) promotionSubject {
	return promotionSubject{
		what: fmt.Sprintf("promotion environment %q or project %q", id, projectID),
		find: "'n8n promotion environment list' and 'n8n project list'",
		alt:  promotionEnvironmentAlt,
	}
}
