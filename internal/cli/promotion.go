package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	// promotionResource is the 'n8n discover' key for this group.
	promotionResource    = "promotions"
	maxPromotionDocument = 1 << 20
)

func newPromotionCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promotion",
		Short: "Manage promotion providers, connections, and apply/promote sync",
		Long: "Manage the upstream-only Promotions generation of Git sync.\n" +
			"Start with 'provider list' to find a provider, then 'connection list'\n" +
			"to find a connection. Create a provider first, then a connection on\n" +
			"it, then a config per direction under 'config', clone each checkout\n" +
			"under 'checkout', link team projects under 'project', then 'promote'\n" +
			"local projects to the remote or 'apply' the remote into the instance.\n\n" +
			"'promote' commits and pushes every team project to the remote;\n" +
			"'apply' resets the apply checkout and imports with overwrite behavior,\n" +
			"creating, updating, or removing content to match. Both mutate versioned\n" +
			"content and ask for confirmation. Config and connection removal can\n" +
			"remove local checkouts. This is the upstream Promotions generation: an\n" +
			"instance that serves GitConnections instead answers 404 or 503, which\n" +
			"reads as the availability diagnostic.",
		Example: "  n8n promotion provider list\n" +
			"  n8n promotion connection list\n" +
			"  n8n promotion provider create --input provider.json\n" +
			"  n8n promotion connection create --input connection.json\n" +
			"  n8n promotion config apply set CONNECTION_ID --input apply.json\n" +
			"  n8n promotion checkout clone CONNECTION_ID apply\n" +
			"  n8n promotion project add CONNECTION_ID PROJECT_ID\n" +
			"  n8n promotion promote CONNECTION_ID --commit-message \"sync\" --yes\n" +
			"  n8n promotion apply CONNECTION_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionProviderCommand(opts),
		newPromotionConnectionCommand(opts),
		newPromotionConfigCommand(opts),
		newPromotionCheckoutCommand(opts),
		newPromotionProjectCommand(opts),
		newPromotionPromoteCommand(opts),
		newPromotionApplyCommand(opts),
	)
	return cmd
}

// Providers.

func newPromotionProviderCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage promotion providers",
		Long: "Manage promotion providers: the stored Git credentials one or more\n" +
			"connections sync through. Start with 'list' to find the provider ID,\n" +
			"then 'get' to read it, 'create' to add one, or 'update' to rename it\n" +
			"or replace its credential.\n\n" +
			"Replacing a provider credential affects every connection attached to\n" +
			"it, so 'update' asks for confirmation. Deleting a provider removes it\n" +
			"while attached connections keep their content but lose their credential.",
		Example: "  n8n promotion provider list\n" +
			"  n8n promotion provider get PROVIDER_ID\n" +
			"  n8n promotion provider create --input provider.json\n" +
			"  n8n promotion provider update PROVIDER_ID --input provider-update.json --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionProviderListCommand(opts),
		newPromotionProviderCreateCommand(opts),
		newPromotionProviderGetCommand(opts),
		newPromotionProviderUpdateCommand(opts),
		newPromotionProviderDeleteCommand(opts),
	)
	return cmd
}

type promotionProviderListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newPromotionProviderListCommand(opts Options) *cobra.Command {
	var f promotionProviderListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List promotion providers",
		Long: "List the promotion providers visible to the selected credential, one\n" +
			"cursor-paginated page at a time. Use --all to follow every cursor,\n" +
			"capped at 10,000 providers.\n\n" +
			"Use a returned ID with get, update, delete, and 'connection create'.\n" +
			"Requires gitConnection:list.",
		Example: "  n8n promotion provider list\n" +
			"  n8n promotion provider list --limit 50 --output json\n" +
			"  n8n promotion provider list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionProviderList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "providers per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 providers)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runPromotionProviderList(ctx context.Context, opts Options, f promotionProviderListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	if listOpts.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", listOpts.Limit)
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var page n8n.Page[n8n.PromotionProvider]
	if f.all {
		page.Data, err = n8n.Collect(ctx, client.ListPromotionProviders, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListPromotionProviders(ctx, listOpts)
	}
	if err != nil {
		return promotionAPIError(err, resolution, "provider list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writePromotionProviderListText(opts, resolution, page)
}

func writePromotionProviderListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.PromotionProvider]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Providers:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tTYPE\tAUTH")
		for _, provider := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				emptyDash(provider.ID), emptyDash(provider.Name),
				emptyDash(provider.Type), emptyDash(provider.AuthType))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No promotion providers were returned. Create one with 'n8n promotion provider create --input provider.json'.")
	}
	return nil
}

type promotionProviderInputFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func (f *promotionProviderInputFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "promotion provider JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the provider returned by the API)")
}

func newPromotionProviderCreateCommand(opts Options) *cobra.Command {
	var f promotionProviderInputFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a promotion provider",
		Long: "Create one promotion provider from strict JSON supplied by --input.\n" +
			"Required fields are name (at most 128 characters), type (git), and an\n" +
			"auth object: {\"authType\":\"ssh-key\"} with an optional keyType\n" +
			"(ed25519 or rsa, default ed25519), or {\"authType\":\"token\",\n" +
			"\"username\":\"...\", \"password\":\"...\"}.\n\n" +
			"Credentials travel only as --input because secrets are never flags:\n" +
			"process lists and shell history can expose argv. Unknown fields and\n" +
			"trailing JSON documents are refused before transport. Requires\n" +
			"gitConnection:create. Follow with 'n8n promotion connection create'.",
		Example: "  n8n promotion provider create --input provider.json\n" +
			"  printf '%s\\n' '{\"name\":\"github\",\"type\":\"git\",\"auth\":{\"authType\":\"ssh-key\"}}' | n8n promotion provider create --input -\n" +
			"  n8n promotion provider create --input provider.json --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionProviderCreate(cmd.Context(), opts, f)
		},
	}
	f.register(cmd)
	return cmd
}

func runPromotionProviderCreate(ctx context.Context, opts Options, f promotionProviderInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreatePromotionProviderRequest
	if err := readPromotionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	created, err := client.CreatePromotionProvider(ctx, request)
	if err != nil {
		return promotionAPIError(err, resolution, "provider create")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, created)
	}
	return writePromotionProviderResult(opts, resolution, "Created", &created.Provider, f.output)
}

type promotionProviderGetFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionProviderGetCommand(opts Options) *cobra.Command {
	var f promotionProviderGetFlags
	cmd := &cobra.Command{
		Use:   "get <provider-id>",
		Short: "Show one promotion provider",
		Long: "Show one promotion provider by ID. The response is the public provider\n" +
			"document: name, type, auth type, configuration, and timestamps.\n" +
			"Secrets are never returned. Find the ID with 'n8n promotion provider\n" +
			"list'. Requires gitConnection:read.",
		Example: "  n8n promotion provider get PROVIDER_ID\n" +
			"  n8n promotion provider get PROVIDER_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionProviderGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the provider object)")
	return cmd
}

func runPromotionProviderGet(ctx context.Context, opts Options, id string, f promotionProviderGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("provider ID is required: pass the ID from 'n8n promotion provider list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("provider ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	provider, err := client.GetPromotionProvider(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "provider get")
	}
	return writePromotionProviderResult(opts, resolution, "Provider", provider, f.output)
}

type promotionProviderUpdateFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newPromotionProviderUpdateCommand(opts Options) *cobra.Command {
	var f promotionProviderUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <provider-id>",
		Short: "Update a promotion provider",
		Long: "Update one promotion provider from strict JSON supplied by --input.\n" +
			"Only the fields present are changed; at least one of name or auth is\n" +
			"required. Auth follows the create shape: ssh-key with an optional key\n" +
			"type, or token with a username and password.\n\n" +
			"Replacing the credential affects every connection attached to this\n" +
			"provider, so interactive runs ask for confirmation and non-interactive\n" +
			"runs require --yes. Credentials travel only as --input because secrets\n" +
			"are never flags. Requires gitConnection:update.",
		Example: "  n8n promotion provider update PROVIDER_ID --input provider-update.json\n" +
			"  printf '%s\\n' '{\"name\":\"github-new\"}' | n8n promotion provider update PROVIDER_ID --input -\n" +
			"  n8n promotion provider update PROVIDER_ID --input provider-update.json --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionProviderUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "promotion provider JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm credential replacement for every attached connection without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the provider returned by the API)")
	return cmd
}

func runPromotionProviderUpdate(ctx context.Context, opts Options, id string, f promotionProviderUpdateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("provider ID is required: pass the ID from 'n8n promotion provider list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("provider ID must not start or end with whitespace")
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the update confirmation: pass --yes after reviewing the document")
	}
	var request n8n.UpdatePromotionProviderRequest
	if err := readPromotionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Update promotion provider %q on %s? A replaced credential affects every connection attached to it.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	updated, err := client.UpdatePromotionProvider(ctx, id, request)
	if err != nil {
		return promotionAPIError(err, resolution, "provider update")
	}
	return writePromotionProviderResult(opts, resolution, "Updated", updated, f.output)
}

type promotionProviderDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionProviderDeleteCommand(opts Options) *cobra.Command {
	var f promotionProviderDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <provider-id>",
		Short: "Permanently delete a promotion provider",
		Long: "Permanently delete one promotion provider. Attached connections keep\n" +
			"their stored content but lose their credential, so promote, apply, and\n" +
			"checkout commands stop working until a provider is attached again.\n" +
			"This cannot be undone.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:delete.",
		Example: "  n8n promotion provider delete PROVIDER_ID\n" +
			"  n8n promotion provider delete PROVIDER_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionProviderDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent provider deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deletion acknowledgement)")
	return cmd
}

func runPromotionProviderDelete(ctx context.Context, opts Options, id string, f promotionProviderDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("provider ID is required: pass the ID from 'n8n promotion provider list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("provider ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete promotion provider %q from %s? Attached connections keep their content but lose their credential.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeletePromotionProvider(ctx, id); err != nil {
		return promotionAPIError(err, resolution, "provider delete")
	}
	return writePromotionMutation(opts, resolution, promotionMutation{Action: "deleted", ProviderID: id}, f.output)
}

// Connections.

func newPromotionConnectionCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "Manage promotion connections",
		Long: "Manage promotion connections: one remote plus its two direction\n" +
			"configs. Start with 'list' to find the connection ID, then 'get' to\n" +
			"read it, 'create' to add one on a provider, or 'update' to change its\n" +
			"name, target, or provider.\n\n" +
			"Deleting a connection can remove its local checkouts. Linked projects\n" +
			"stay on the instance but lose their remote.",
		Example: "  n8n promotion connection list\n" +
			"  n8n promotion connection get CONNECTION_ID\n" +
			"  n8n promotion connection create --input connection.json\n" +
			"  n8n promotion connection update CONNECTION_ID --input connection-update.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionConnectionListCommand(opts),
		newPromotionConnectionCreateCommand(opts),
		newPromotionConnectionGetCommand(opts),
		newPromotionConnectionUpdateCommand(opts),
		newPromotionConnectionDeleteCommand(opts),
	)
	return cmd
}

type promotionConnectionListFlags struct {
	instance   instanceFlags
	limit      int
	cursor     string
	scope      string
	providerID string
	all        bool
	output     string
}

func newPromotionConnectionListCommand(opts Options) *cobra.Command {
	var f promotionConnectionListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List promotion connections",
		Long: "List the promotion connections visible to the selected credential,\n" +
			"one cursor-paginated page at a time. Filter with --scope (instance or\n" +
			"projects) and --provider-id; filters are preserved across --all.\n" +
			"Use --all to follow every cursor, capped at 10,000 connections.\n\n" +
			"Use a returned ID with get, update, delete, config, checkout, project,\n" +
			"promote, and apply. Requires gitConnection:list.",
		Example: "  n8n promotion connection list\n" +
			"  n8n promotion connection list --scope projects --provider-id PROVIDER_ID\n" +
			"  n8n promotion connection list --limit 50 --output json\n" +
			"  n8n promotion connection list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionConnectionList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "connections per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().StringVar(&f.scope, "scope", "", "filter by connection scope: instance or projects (default: all scopes)")
	cmd.Flags().StringVar(&f.providerID, "provider-id", "", "filter by provider ID from 'n8n promotion provider list' (default: all providers)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 connections)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runPromotionConnectionList(ctx context.Context, opts Options, f promotionConnectionListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListPromotionConnectionsOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Scope:       strings.TrimSpace(f.scope),
		ProviderID:  strings.TrimSpace(f.providerID),
	}
	if f.scope != "" && listOpts.Scope == "" {
		return fmt.Errorf("--scope must not be blank: use instance or projects, or omit it for all scopes")
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
	var page n8n.Page[n8n.PromotionConnection]
	if f.all {
		collect := func(ctx context.Context, base n8n.ListOptions) (n8n.Page[n8n.PromotionConnection], error) {
			return client.ListPromotionConnections(ctx, n8n.ListPromotionConnectionsOptions{
				ListOptions: base, Scope: listOpts.Scope, ProviderID: listOpts.ProviderID,
			})
		}
		page.Data, err = n8n.Collect(ctx, collect, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListPromotionConnections(ctx, listOpts)
	}
	if err != nil {
		return promotionAPIError(err, resolution, "connection list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writePromotionConnectionListText(opts, resolution, page)
}

func writePromotionConnectionListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.PromotionConnection]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Connections:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tSCOPE\tREMOTE\tPROVIDER")
		for _, connection := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				emptyDash(connection.ID), emptyDash(connection.Name),
				emptyDash(connection.Scope), emptyDash(connection.Target.RemoteURL),
				emptyDash(connection.Provider.Name))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No promotion connections were returned. Create one with 'n8n promotion connection create --input connection.json'.")
	}
	return nil
}

type promotionConnectionInputFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func (f *promotionConnectionInputFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "promotion connection JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the connection returned by the API)")
}

func newPromotionConnectionCreateCommand(opts Options) *cobra.Command {
	var f promotionConnectionInputFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a promotion connection",
		Long: "Create one promotion connection from strict JSON supplied by --input.\n" +
			"Required fields are name (at most 128 characters), scope (instance or\n" +
			"projects), providerId, and target {\"schemaVersion\":1,\n" +
			"\"remoteUrl\":\"...\"}. An optional configs object carries the apply\n" +
			"config {\"settings\":{\"schemaVersion\":1,\"branchName\":\"main\"}} and\n" +
			"the promote config {\"settings\":{\"schemaVersion\":1,\n" +
			"\"baseBranchName\":\"main\",\"createBranchOnPromotion\":false}}.\n\n" +
			"Unknown fields and trailing JSON documents are refused before\n" +
			"transport. Requires gitConnection:create. Follow with\n" +
			"'n8n promotion config apply set' and 'n8n promotion config promote\n" +
			"set' when the configs were omitted.",
		Example: "  n8n promotion connection create --input connection.json\n" +
			"  n8n promotion connection create --input connection.json --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionConnectionCreate(cmd.Context(), opts, f)
		},
	}
	f.register(cmd)
	return cmd
}

func runPromotionConnectionCreate(ctx context.Context, opts Options, f promotionConnectionInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreatePromotionConnectionRequest
	if err := readPromotionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	created, err := client.CreatePromotionConnection(ctx, request)
	if err != nil {
		return promotionAPIError(err, resolution, "connection create")
	}
	return writePromotionConnectionResult(opts, resolution, "Created", created, f.output)
}

type promotionConnectionGetFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionConnectionGetCommand(opts Options) *cobra.Command {
	var f promotionConnectionGetFlags
	cmd := &cobra.Command{
		Use:   "get <connection-id>",
		Short: "Show one promotion connection",
		Long: "Show one promotion connection by ID: its name, scope, remote target,\n" +
			"provider, both direction configs, and timestamps. Secrets are never\n" +
			"returned. Find the ID with 'n8n promotion connection list'. Requires\n" +
			"gitConnection:read.",
		Example: "  n8n promotion connection get CONNECTION_ID\n" +
			"  n8n promotion connection get CONNECTION_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionConnectionGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the connection object)")
	return cmd
}

func runPromotionConnectionGet(ctx context.Context, opts Options, id string, f promotionConnectionGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	connection, err := client.GetPromotionConnection(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "connection get")
	}
	return writePromotionConnectionResult(opts, resolution, "Connection", connection, f.output)
}

func newPromotionConnectionUpdateCommand(opts Options) *cobra.Command {
	var f promotionConnectionInputFlags
	cmd := &cobra.Command{
		Use:   "update <connection-id>",
		Short: "Update a promotion connection",
		Long: "Update one promotion connection from strict JSON supplied by --input.\n" +
			"Only the fields present are changed; at least one of name, target, or\n" +
			"providerId is required. Direction configs are not patched here: use\n" +
			"'n8n promotion config apply set' and 'n8n promotion config promote\n" +
			"set' for those. Requires gitConnection:update.",
		Example: "  n8n promotion connection update CONNECTION_ID --input connection-update.json\n" +
			"  printf '%s\\n' '{\"name\":\"staging\"}' | n8n promotion connection update CONNECTION_ID --input -\n" +
			"  n8n promotion connection update CONNECTION_ID --input connection-update.json --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionConnectionUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runPromotionConnectionUpdate(ctx context.Context, opts Options, id string, f promotionConnectionInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	var request n8n.UpdatePromotionConnectionRequest
	if err := readPromotionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	updated, err := client.UpdatePromotionConnection(ctx, id, request)
	if err != nil {
		return promotionAPIError(err, resolution, "connection update")
	}
	return writePromotionConnectionResult(opts, resolution, "Updated", updated, f.output)
}

type promotionConnectionDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionConnectionDeleteCommand(opts Options) *cobra.Command {
	var f promotionConnectionDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <connection-id>",
		Short: "Permanently delete a promotion connection",
		Long: "Permanently delete one promotion connection. Its direction configs\n" +
			"and local checkouts can go with it. Linked projects stay on the\n" +
			"instance but lose their remote; promote and apply stop working until a\n" +
			"connection is created and cloned again. This cannot be undone.\n\n" +
			"To keep the connection and drop only a checkout, use 'n8n promotion\n" +
			"checkout disconnect' instead. Interactive runs ask for confirmation;\n" +
			"non-interactive runs require --yes. Requires gitConnection:delete.",
		Example: "  n8n promotion connection delete CONNECTION_ID\n" +
			"  n8n promotion connection delete CONNECTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionConnectionDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent connection and checkout deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deletion acknowledgement)")
	return cmd
}

func runPromotionConnectionDelete(ctx context.Context, opts Options, id string, f promotionConnectionDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete promotion connection %q from %s? Its configs and local checkouts can go away; linked projects keep their instance content but lose their remote.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeletePromotionConnection(ctx, id); err != nil {
		return promotionAPIError(err, resolution, "connection delete")
	}
	return writePromotionMutation(opts, resolution, promotionMutation{Action: "deleted", ConnectionID: id}, f.output)
}

// Configs.

func newPromotionConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage promotion direction configs",
		Long: "Manage the two direction configs of one promotion connection. Each\n" +
			"direction has its own checkout: 'apply' pulls the remote into the\n" +
			"instance, 'promote' pushes the instance to the remote.\n\n" +
			"Both 'set' actions are full replacements that create the config on\n" +
			"first write. Removing a config can remove its local checkout.",
		Example: "  n8n promotion config apply set CONNECTION_ID --input apply.json\n" +
			"  n8n promotion config promote set CONNECTION_ID --input promote.json\n" +
			"  n8n promotion config delete CONNECTION_ID apply --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionConfigApplyCommand(opts),
		newPromotionConfigPromoteCommand(opts),
		newPromotionConfigDeleteCommand(opts),
	)
	return cmd
}

func newPromotionConfigApplyCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Manage the apply direction config",
		Long: "Manage the apply direction config of one promotion connection: the\n" +
			"branch 'apply' resets to and imports from. Use 'set' to replace it.",
		Example: "  n8n promotion config apply set CONNECTION_ID --input apply.json",
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPromotionConfigApplySetCommand(opts))
	return cmd
}

func newPromotionConfigPromoteCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Manage the promote direction config",
		Long: "Manage the promote direction config of one promotion connection: the\n" +
			"base branch promotions push to. Use 'set' to replace it.",
		Example: "  n8n promotion config promote set CONNECTION_ID --input promote.json",
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPromotionConfigPromoteSetCommand(opts))
	return cmd
}

type promotionConfigSetFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func (f *promotionConfigSetFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "direction config JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the config replacement without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the stored config returned by the API)")
}

func newPromotionConfigApplySetCommand(opts Options) *cobra.Command {
	var f promotionConfigSetFlags
	cmd := &cobra.Command{
		Use:   "set <connection-id>",
		Short: "Replace the apply direction config",
		Long: "Replace the apply direction config of one promotion connection from\n" +
			"strict JSON supplied by --input. The settings object is required:\n" +
			"{\"settings\":{\"schemaVersion\":1,\"branchName\":\"main\"}} with an\n" +
			"optional name. This creates the config on first write.\n\n" +
			"Interactive runs ask for confirmation and non-interactive runs require\n" +
			"--yes. Requires gitConnection:update. Clone the checkout next with\n" +
			"'n8n promotion checkout clone CONNECTION_ID apply'.",
		Example: "  n8n promotion config apply set CONNECTION_ID --input apply.json\n" +
			"  printf '%s\\n' '{\"settings\":{\"schemaVersion\":1,\"branchName\":\"main\"}}' | n8n promotion config apply set CONNECTION_ID --input - --yes",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionConfigApplySet(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runPromotionConfigApplySet(ctx context.Context, opts Options, id string, f promotionConfigSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the replacement confirmation: pass --yes after reviewing the config")
	}
	var request n8n.UpsertPromotionApplyConfigRequest
	if err := readPromotionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Replace the apply config of promotion connection %q on %s with branch %q?", id, resolution.URL, request.Settings.BranchName)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	config, err := client.UpsertPromotionApplyConfig(ctx, id, request)
	if err != nil {
		return promotionAPIError(err, resolution, "config apply set")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, config)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Replaced:\t%s\n", config.ID)
	fmt.Fprintf(tw, "Connection:\t%s\n", id)
	fmt.Fprintf(tw, "Direction:\tapply\n")
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(config.Settings.BranchName))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func newPromotionConfigPromoteSetCommand(opts Options) *cobra.Command {
	var f promotionConfigSetFlags
	cmd := &cobra.Command{
		Use:   "set <connection-id>",
		Short: "Replace the promote direction config",
		Long: "Replace the promote direction config of one promotion connection from\n" +
			"strict JSON supplied by --input. The settings object is required:\n" +
			"{\"settings\":{\"schemaVersion\":1,\"baseBranchName\":\"main\",\n" +
			"\"createBranchOnPromotion\":false}} with an optional name. This creates\n" +
			"the config on first write.\n\n" +
			"Interactive runs ask for confirmation and non-interactive runs require\n" +
			"--yes. Requires gitConnection:update. Clone the checkout next with\n" +
			"'n8n promotion checkout clone CONNECTION_ID promote'.",
		Example: "  n8n promotion config promote set CONNECTION_ID --input promote.json\n" +
			"  n8n promotion config promote set CONNECTION_ID --input promote.json --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionConfigPromoteSet(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runPromotionConfigPromoteSet(ctx context.Context, opts Options, id string, f promotionConfigSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the replacement confirmation: pass --yes after reviewing the config")
	}
	var request n8n.UpsertPromotionPromoteConfigRequest
	if err := readPromotionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Replace the promote config of promotion connection %q on %s with base branch %q?", id, resolution.URL, request.Settings.BaseBranchName)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	config, err := client.UpsertPromotionPromoteConfig(ctx, id, request)
	if err != nil {
		return promotionAPIError(err, resolution, "config promote set")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, config)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Replaced:\t%s\n", config.ID)
	fmt.Fprintf(tw, "Connection:\t%s\n", id)
	fmt.Fprintf(tw, "Direction:\tpromote\n")
	fmt.Fprintf(tw, "Base branch:\t%s\n", emptyDash(config.Settings.BaseBranchName))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type promotionConfigDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionConfigDeleteCommand(opts Options) *cobra.Command {
	var f promotionConfigDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <connection-id> <direction>",
		Short: "Delete one direction config",
		Long: "Delete the apply or promote direction config of one promotion\n" +
			"connection. The direction argument is apply or promote. Removing a\n" +
			"config can remove its local checkout.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:update.",
		Example: "  n8n promotion config delete CONNECTION_ID apply\n" +
			"  n8n promotion config delete CONNECTION_ID promote --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionConfigDelete(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm config deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deletion acknowledgement)")
	return cmd
}

func runPromotionConfigDelete(ctx context.Context, opts Options, id, direction string, f promotionConfigDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	direction = strings.TrimSpace(direction)
	if direction != n8n.PromotionDirectionApply && direction != n8n.PromotionDirectionPromote {
		return fmt.Errorf("unknown promotion direction %q: use apply or promote", direction)
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Delete the %s config of promotion connection %q on %s? Its local checkout can go away.", direction, id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeletePromotionConfig(ctx, id, direction); err != nil {
		return promotionAPIError(err, resolution, "config delete")
	}
	return writePromotionMutation(opts, resolution, promotionMutation{Action: "config deleted", ConnectionID: id, Direction: direction}, f.output)
}

// Checkouts.

func newPromotionCheckoutCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkout",
		Short: "Manage promotion checkouts",
		Long: "Manage the local checkouts of one promotion connection, one direction\n" +
			"at a time. 'clone' prepares the checkout that 'project', 'promote',\n" +
			"and 'apply' work against; 'disconnect' removes it while keeping the\n" +
			"connection, its configs, and its credentials.\n\n" +
			"The direction argument is apply or promote.",
		Example: "  n8n promotion checkout clone CONNECTION_ID apply\n" +
			"  n8n promotion checkout clone CONNECTION_ID promote\n" +
			"  n8n promotion checkout disconnect CONNECTION_ID apply --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionCheckoutCloneCommand(opts),
		newPromotionCheckoutDisconnectCommand(opts),
	)
	return cmd
}

type promotionCheckoutCloneFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionCheckoutCloneCommand(opts Options) *cobra.Command {
	var f promotionCheckoutCloneFlags
	cmd := &cobra.Command{
		Use:   "clone <connection-id> <direction>",
		Short: "Clone one promotion checkout locally",
		Long: "Clone one direction checkout of a promotion connection into local\n" +
			"storage. The direction argument is apply or promote.\n\n" +
			"Cloning is safe to repeat and changes no project content by itself:\n" +
			"it only prepares the checkout that 'project add', 'promote', and\n" +
			"'apply' work against, so it needs no confirmation. Clone before\n" +
			"promoting or applying. Requires gitConnection:clone.",
		Example: "  n8n promotion checkout clone CONNECTION_ID apply\n" +
			"  n8n promotion checkout clone CONNECTION_ID promote --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionCheckoutClone(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the checkout returned by the API)")
	return cmd
}

func runPromotionCheckoutClone(ctx context.Context, opts Options, id, direction string, f promotionCheckoutCloneFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	direction = strings.TrimSpace(direction)
	if direction != n8n.PromotionDirectionApply && direction != n8n.PromotionDirectionPromote {
		return fmt.Errorf("unknown promotion direction %q: use apply or promote", direction)
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	checkout, err := client.ClonePromotionCheckout(ctx, id, direction)
	if err != nil {
		return promotionAPIError(err, resolution, "checkout clone")
	}
	return writePromotionCheckout(opts, resolution, "Cloned", checkout, f.output)
}

type promotionCheckoutDisconnectFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionCheckoutDisconnectCommand(opts Options) *cobra.Command {
	var f promotionCheckoutDisconnectFlags
	cmd := &cobra.Command{
		Use:   "disconnect <connection-id> <direction>",
		Short: "Remove one promotion checkout",
		Long: "Remove the local checkout of one direction of a promotion connection.\n" +
			"The direction argument is apply or promote. The connection, its\n" +
			"configs, and its credentials are retained, and linked projects stay\n" +
			"linked, but promote and apply stop working for that direction until\n" +
			"'clone' runs again.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:clone.",
		Example: "  n8n promotion checkout disconnect CONNECTION_ID apply\n" +
			"  n8n promotion checkout disconnect CONNECTION_ID promote --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionCheckoutDisconnect(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm checkout removal without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the checkout returned by the API)")
	return cmd
}

func runPromotionCheckoutDisconnect(ctx context.Context, opts Options, id, direction string, f promotionCheckoutDisconnectFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	direction = strings.TrimSpace(direction)
	if direction != n8n.PromotionDirectionApply && direction != n8n.PromotionDirectionPromote {
		return fmt.Errorf("unknown promotion direction %q: use apply or promote", direction)
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Remove the %s checkout of promotion connection %q on %s? The connection stays, but that direction stops working until it is cloned again.", direction, id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	checkout, err := client.DisconnectPromotionCheckout(ctx, id, direction)
	if err != nil {
		return promotionAPIError(err, resolution, "checkout disconnect")
	}
	return writePromotionCheckout(opts, resolution, "Disconnected", checkout, f.output)
}

// Projects.

func newPromotionProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects linked to a promotion connection",
		Long: "Manage which team projects one promotion connection syncs. Start with\n" +
			"'list' to see the linked project IDs, then 'add' a project or\n" +
			"'remove' one.\n\n" +
			"A project can belong to only one connection, so adding a linked\n" +
			"project elsewhere answers 409. Removing unlinks the project; its\n" +
			"instance content is untouched. Project IDs come from 'n8n project\n" +
			"list'. Adding and listing need gitConnection:read for reads and\n" +
			"gitConnection:manageProjects for writes.",
		Example: "  n8n promotion project list CONNECTION_ID\n" +
			"  n8n promotion project add CONNECTION_ID PROJECT_ID\n" +
			"  n8n promotion project remove CONNECTION_ID PROJECT_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPromotionProjectListCommand(opts),
		newPromotionProjectAddCommand(opts),
		newPromotionProjectRemoveCommand(opts),
	)
	return cmd
}

type promotionProjectListFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionProjectListCommand(opts Options) *cobra.Command {
	var f promotionProjectListFlags
	cmd := &cobra.Command{
		Use:   "list <connection-id>",
		Short: "List projects linked to a promotion connection",
		Long: "List the team projects added to one promotion connection. These are\n" +
			"the projects 'promote' exports and 'apply' overwrites. Find the\n" +
			"connection ID with 'n8n promotion connection list'. Requires\n" +
			"gitConnection:read.",
		Example: "  n8n promotion project list CONNECTION_ID\n" +
			"  n8n promotion project list CONNECTION_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionProjectList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the projectIds object)")
	return cmd
}

func runPromotionProjectList(ctx context.Context, opts Options, id string, f promotionProjectListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	projects, err := client.ListPromotionConnectionProjects(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "project list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, projects)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Connection:\t%s\n", id)
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
		fmt.Fprintln(opts.Streams.Err, "No projects are linked. Add one with 'n8n promotion project add CONNECTION_ID PROJECT_ID'.")
	}
	return nil
}

type promotionProjectWriteFlags struct {
	instance instanceFlags
	output   string
}

func newPromotionProjectAddCommand(opts Options) *cobra.Command {
	var f promotionProjectWriteFlags
	cmd := &cobra.Command{
		Use:   "add <connection-id> <project-id>",
		Short: "Add a project to a promotion connection",
		Long: "Add one team project to one promotion connection so promote and apply\n" +
			"include it. A project can belong to only one connection: adding a\n" +
			"project that is already linked elsewhere answers 409. The checkout\n" +
			"must be cloned first. Requires gitConnection:manageProjects.",
		Example: "  n8n promotion project add CONNECTION_ID PROJECT_ID\n" +
			"  n8n promotion project add CONNECTION_ID PROJECT_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionProjectAdd(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the project link returned by the API)")
	return cmd
}

func runPromotionProjectAdd(ctx context.Context, opts Options, id, projectID string, f promotionProjectWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("connection ID and project ID are required: pass the IDs from 'n8n promotion connection list' and 'n8n project list'")
	}
	if strings.TrimSpace(id) != id || strings.TrimSpace(projectID) != projectID {
		return fmt.Errorf("connection ID and project ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	link, err := client.AddProjectToPromotionConnection(ctx, id, projectID)
	if err != nil {
		return promotionAPIError(err, resolution, "project add")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, link)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Added:\t%s\n", link.ProjectID)
	fmt.Fprintf(tw, "Connection:\t%s\n", link.ConnectionID)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type promotionProjectRemoveFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionProjectRemoveCommand(opts Options) *cobra.Command {
	var f promotionProjectRemoveFlags
	cmd := &cobra.Command{
		Use:   "remove <connection-id> <project-id>",
		Short: "Remove a project from a promotion connection",
		Long: "Unlink one project from one promotion connection. The project itself\n" +
			"and its instance content are untouched; only the sync link goes away,\n" +
			"so later promotes and applies skip it. Re-add it later with 'n8n\n" +
			"promotion project add'.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:manageProjects.",
		Example: "  n8n promotion project remove CONNECTION_ID PROJECT_ID\n" +
			"  n8n promotion project remove CONNECTION_ID PROJECT_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionProjectRemove(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm project unlinking without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the unlinking acknowledgement)")
	return cmd
}

func runPromotionProjectRemove(ctx context.Context, opts Options, id, projectID string, f promotionProjectRemoveFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("connection ID and project ID are required: pass the IDs from 'n8n promotion connection list' and 'n8n project list'")
	}
	if strings.TrimSpace(id) != id || strings.TrimSpace(projectID) != projectID {
		return fmt.Errorf("connection ID and project ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Remove project %q from promotion connection %q on %s? The project content stays; later promotes and applies skip it.", projectID, id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.RemoveProjectFromPromotionConnection(ctx, id, projectID); err != nil {
		return promotionAPIError(err, resolution, "project remove")
	}
	return writePromotionMutation(opts, resolution, promotionMutation{Action: "project removed", ConnectionID: id, ProjectID: projectID}, f.output)
}

// Promote and apply.

type promotionPromoteFlags struct {
	instance instanceFlags
	commit   string
	force    bool
	yes      bool
	output   string
}

func newPromotionPromoteCommand(opts Options) *cobra.Command {
	var f promotionPromoteFlags
	cmd := &cobra.Command{
		Use:   "promote <connection-id>",
		Short: "Promote team projects to the Git remote",
		Long: "Export all team projects, commit them, and push to the promote\n" +
			"checkout's branch. The promote checkout must be cloned first: run 'n8n\n" +
			"promotion checkout clone CONNECTION_ID promote' and link projects\n" +
			"with 'n8n promotion project add'.\n\n" +
			"Pass --commit-message once (1 to 1000 characters). Without --force a\n" +
			"promote the remote would reject is refused with 409 and nothing is\n" +
			"pushed; --force pushes anyway and rewrites remote history. This\n" +
			"mutates the remote branch. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires gitConnection:push.",
		Example: "  n8n promotion promote CONNECTION_ID --commit-message \"sync workflows\"\n" +
			"  n8n promotion promote CONNECTION_ID --commit-message \"sync\" --force --yes\n" +
			"  n8n promotion promote CONNECTION_ID --commit-message \"sync\" --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionPromote(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.commit, "commit-message", "", "commit message for the promote, 1 to 1000 characters (required)")
	cmd.Flags().BoolVar(&f.force, "force", false, "promote even when the remote would reject it, rewriting remote history (default: reject a rejected promote with 409)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm promoting every team project to the remote without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the promote result with counts and commit)")
	return cmd
}

func runPromotionPromote(ctx context.Context, opts Options, id string, f promotionPromoteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	request := n8n.PromotePromotionRequest{CommitMessage: f.commit, Force: f.force}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Promote every team project on %s to the remote of promotion connection %q with commit message %q? Linked projects are committed and pushed.", resolution.URL, id, request.CommitMessage)
	if request.Force {
		question = fmt.Sprintf("Force-promote every team project on %s to the remote of promotion connection %q with commit message %q? Remote history is rewritten.", resolution.URL, id, request.CommitMessage)
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.PromotePackage(ctx, id, request)
	if err != nil {
		return promotionAPIError(err, resolution, "promote")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writePromotionPromoteResult(opts, resolution, result)
}

type promotionApplyFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newPromotionApplyCommand(opts Options) *cobra.Command {
	var f promotionApplyFlags
	cmd := &cobra.Command{
		Use:   "apply <connection-id>",
		Short: "Apply the Git remote into this instance",
		Long: "Reset the apply checkout to the branch tip and import projects into\n" +
			"the instance, overwriting to match. The apply checkout must be cloned\n" +
			"first. Preview what is linked with 'n8n promotion project list'.\n\n" +
			"This rewrites instance content from the remote and cannot be undone:\n" +
			"workflows, folders, credentials, data tables, variables, and tags are\n" +
			"created, updated, or removed to match the branch, and credential gaps\n" +
			"become stubs. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires gitConnection:pull.",
		Example: "  n8n promotion apply CONNECTION_ID\n" +
			"  n8n promotion apply CONNECTION_ID --yes\n" +
			"  n8n promotion apply CONNECTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromotionApply(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm overwriting instance content from the remote without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the apply result with counts and commit)")
	return cmd
}

func runPromotionApply(ctx context.Context, opts Options, id string, f promotionApplyFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n promotion connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Apply the remote of promotion connection %q into %s and overwrite instance projects to match? Workflows, folders, credentials, tables, variables, and tags change; this cannot be undone.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.ApplyPackage(ctx, id)
	if err != nil {
		return promotionAPIError(err, resolution, "apply")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writePromotionApplyResult(opts, resolution, result)
}

// Shared helpers.

func readPromotionDocument(opts Options, path string, dst any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass a JSON file path or --input - for stdin")
	}
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open promotion input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPromotionDocument+1))
	if err != nil {
		return fmt.Errorf("read promotion JSON: %w", err)
	}
	if len(raw) > maxPromotionDocument {
		return fmt.Errorf("promotion JSON exceeds %d bytes", maxPromotionDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode promotion JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode promotion JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode promotion JSON: %w", err)
	}
	return nil
}

func writePromotionProviderResult(opts Options, resolution config.Resolution, action string, provider *n8n.PromotionProvider, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, provider)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, provider.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", emptyDash(provider.Name))
	fmt.Fprintf(tw, "Type:\t%s\n", emptyDash(provider.Type))
	fmt.Fprintf(tw, "Auth:\t%s\n", emptyDash(provider.AuthType))
	if provider.Config != nil && provider.Config.PublicKey != nil {
		fmt.Fprintf(tw, "Public key:\t%s\n", truncatedPromotionSecret(*provider.Config.PublicKey))
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if provider.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", provider.CreatedAt)
	}
	if provider.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", provider.UpdatedAt)
	}
	return tw.Flush()
}

func writePromotionConnectionResult(opts Options, resolution config.Resolution, action string, connection *n8n.PromotionConnection, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, connection)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, connection.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", emptyDash(connection.Name))
	fmt.Fprintf(tw, "Scope:\t%s\n", emptyDash(connection.Scope))
	fmt.Fprintf(tw, "Remote:\t%s\n", emptyDash(connection.Target.RemoteURL))
	fmt.Fprintf(tw, "Provider:\t%s\n", emptyDash(connection.Provider.Name))
	if connection.Configs.Apply != nil {
		fmt.Fprintf(tw, "Apply branch:\t%s\n", emptyDash(connection.Configs.Apply.Settings.BranchName))
	}
	if connection.Configs.Promote != nil {
		fmt.Fprintf(tw, "Promote base:\t%s\n", emptyDash(connection.Configs.Promote.Settings.BaseBranchName))
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if connection.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", connection.CreatedAt)
	}
	if connection.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", connection.UpdatedAt)
	}
	return tw.Flush()
}

type promotionMutation struct {
	Action       string `json:"action"`
	ProviderID   string `json:"providerId,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
	Direction    string `json:"direction,omitempty"`
}

func writePromotionMutation(opts Options, resolution config.Resolution, result promotionMutation, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	if result.ProviderID != "" {
		fmt.Fprintf(tw, "Provider:\t%s\n", result.ProviderID)
	}
	if result.ConnectionID != "" {
		fmt.Fprintf(tw, "Connection:\t%s\n", result.ConnectionID)
	}
	if result.ProjectID != "" {
		fmt.Fprintf(tw, "Project:\t%s\n", result.ProjectID)
	}
	if result.Direction != "" {
		fmt.Fprintf(tw, "Direction:\t%s\n", result.Direction)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func writePromotionCheckout(opts Options, resolution config.Resolution, action string, checkout *n8n.PromotionCheckout, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, checkout)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, checkout.ConnectionID)
	fmt.Fprintf(tw, "Direction:\t%s\n", emptyDash(checkout.Direction))
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(checkout.BranchName))
	fmt.Fprintf(tw, "Checkout:\t%v\n", checkout.HasCheckout)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func writePromotionPromoteResult(opts Options, resolution config.Resolution, result *n8n.PromotePromotionResult) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Promoted:\t%s\n", result.ConnectionID)
	fmt.Fprintf(tw, "Commit:\t%s\n", emptyDash(result.Git.CommitSHA))
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(result.Git.BranchName))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintln(tw, "\nENTITY\tCOUNT")
	fmt.Fprintf(tw, "workflows\t%d\n", result.Counts.Workflows)
	fmt.Fprintf(tw, "folders\t%d\n", result.Counts.Folders)
	fmt.Fprintf(tw, "credentials\t%d\n", result.Counts.Credentials)
	fmt.Fprintf(tw, "dataTables\t%d\n", result.Counts.DataTables)
	fmt.Fprintf(tw, "variables\t%d\n", result.Counts.Variables)
	fmt.Fprintf(tw, "tags\t%d\n", result.Counts.Tags)
	return tw.Flush()
}

func writePromotionApplyResult(opts Options, resolution config.Resolution, result *n8n.ApplyPromotionResult) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Applied:\t%s\n", result.ConnectionID)
	fmt.Fprintf(tw, "Commit:\t%s\n", emptyDash(result.Git.CommitSHA))
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(result.Git.BranchName))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintln(tw, "\nENTITY\tDETAIL\tCOUNT")
	fmt.Fprintf(tw, "projects\tcreated\t%d\n", result.Counts.Projects.Created)
	fmt.Fprintf(tw, "projects\tupdated\t%d\n", result.Counts.Projects.Updated)
	fmt.Fprintf(tw, "projects\tskipped\t%d\n", result.Counts.Projects.Skipped)
	fmt.Fprintf(tw, "projects\tdeleted\t%d\n", result.Counts.Projects.Deleted)
	fmt.Fprintf(tw, "folders\tcreated\t%d\n", result.Counts.Folders.Created)
	fmt.Fprintf(tw, "folders\tskipped\t%d\n", result.Counts.Folders.Skipped)
	fmt.Fprintf(tw, "folders\tremoved\t%d\n", result.Counts.Folders.Removed)
	fmt.Fprintf(tw, "workflows\tcreated\t%d\n", result.Counts.Workflows.Created)
	fmt.Fprintf(tw, "workflows\tupdated\t%d\n", result.Counts.Workflows.Updated)
	fmt.Fprintf(tw, "workflows\tskipped\t%d\n", result.Counts.Workflows.Skipped)
	fmt.Fprintf(tw, "workflows\tarchived\t%d\n", result.Counts.Workflows.Archived)
	fmt.Fprintf(tw, "workflows\tdeleted\t%d\n", result.Counts.Workflows.Deleted)
	fmt.Fprintf(tw, "workflows\tpublished\t%d\n", result.Counts.Workflows.Publishing.Published)
	fmt.Fprintf(tw, "workflows\tunpublished\t%d\n", result.Counts.Workflows.Publishing.Unpublished)
	fmt.Fprintf(tw, "workflows\tunchanged\t%d\n", result.Counts.Workflows.Publishing.Unchanged)
	fmt.Fprintf(tw, "workflows\tblocked\t%d\n", result.Counts.Workflows.Publishing.Blocked)
	fmt.Fprintf(tw, "workflows\tfailed\t%d\n", result.Counts.Workflows.Publishing.Failed)
	fmt.Fprintf(tw, "credentials\tmatched\t%d\n", result.Counts.Credentials.Matched)
	fmt.Fprintf(tw, "credentials\tstubbed\t%d\n", result.Counts.Credentials.Stubbed)
	fmt.Fprintf(tw, "dataTables\tmatched\t%d\n", result.Counts.DataTables.Matched)
	fmt.Fprintf(tw, "dataTables\tcreated\t%d\n", result.Counts.DataTables.Created)
	fmt.Fprintf(tw, "variables\tmatched\t%d\n", result.Counts.Variables.Matched)
	fmt.Fprintf(tw, "variables\tcreated\t%d\n", result.Counts.Variables.Created)
	fmt.Fprintf(tw, "variables\tupdated\t%d\n", result.Counts.Variables.Updated)
	fmt.Fprintf(tw, "variables\tstubbed\t%d\n", result.Counts.Variables.Stubbed)
	fmt.Fprintf(tw, "variables\tmissing\t%d\n", result.Counts.Variables.Missing)
	fmt.Fprintf(tw, "tags\tmatched\t%d\n", result.Counts.Tags.Matched)
	fmt.Fprintf(tw, "tags\tcreated\t%d\n", result.Counts.Tags.Created)
	fmt.Fprintf(tw, "tags\trenamed\t%d\n", result.Counts.Tags.Renamed)
	fmt.Fprintf(tw, "tags\treconciled\t%d\n", result.Counts.Tags.Reconciled)
	fmt.Fprintf(tw, "tags\tskipped\t%d\n", result.Counts.Tags.Skipped)
	if err := tw.Flush(); err != nil {
		return err
	}
	if result.Counts.Variables.Missing > 0 {
		fmt.Fprintf(opts.Streams.Err, "%d variable(s) still unresolved: workflows referencing them may fail until the variables exist.\n", result.Counts.Variables.Missing)
	}
	if result.Counts.Workflows.Publishing.Failed > 0 || result.Counts.Workflows.Publishing.Blocked > 0 {
		fmt.Fprintln(opts.Streams.Err, "Some workflows did not publish: see the publishing counts in --output json.")
	}
	return nil
}

// truncatedPromotionSecret keeps public keys out of full display: text output
// shows only a prefix while JSON keeps the complete value for scripting.
func truncatedPromotionSecret(value string) string {
	if value == "" {
		return "-"
	}
	const prefix = 24
	if len(value) <= prefix {
		return value
	}
	return value[:prefix] + "…"
}

func promotionAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsConflict(err) && action == "provider delete":
		return fmt.Errorf("%s rejected the promotion provider delete (409): the provider still has attached connections, so nothing changed; remove them first, then delete the provider: %w", resolution.URL, err)
	case n8n.IsConflict(err) && action == "project add":
		return fmt.Errorf("%s rejected the promotion project add (409): the project is already linked to a connection, so nothing changed; remove it there first, then add it here: %w", resolution.URL, err)
	case n8n.IsConflict(err):
		return fmt.Errorf("%s rejected the promotion %s (409): the connection or project is in a conflicting state, so nothing changed; list the current state and retry: %w", resolution.URL, action, err)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the promotion %s (400): check the IDs, the direction (apply or promote), the document fields, the branch names, and the commit message: %w", resolution.URL, action, err)
	case n8n.IsStatus(err, http.StatusUnprocessableEntity):
		return fmt.Errorf("%s rejected the promotion %s (422): the remote content cannot be imported as-is (missing references or unresolvable content); resolve the references and retry: %w", resolution.URL, action, err)
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied the promotion %s (403): the credential needs %s; run %s", resolution.URL, action, promotionScope(action), discoverHint(promotionResource))
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s does not serve promotions (%d): it needs the Promotions module (a GitConnections-generation instance answers here instead); run %s to see what this instance offers: %w",
			resolution.URL, n8n.StatusCodeOf(err), discoverHint(promotionResource), err)
	default:
		return apiError(err, resolution, promotionResource)
	}
}

func promotionScope(action string) string {
	switch action {
	case "provider list", "connection list":
		return "gitConnection:list"
	case "provider create", "connection create":
		return "gitConnection:create"
	case "provider get", "connection get", "project list":
		return "gitConnection:read"
	case "provider update", "connection update", "config apply set", "config promote set", "config delete":
		return "gitConnection:update"
	case "provider delete", "connection delete":
		return "gitConnection:delete"
	case "checkout clone", "checkout disconnect":
		return "gitConnection:clone"
	case "project add", "project remove":
		return "gitConnection:manageProjects"
	case "promote":
		return "gitConnection:push"
	case "apply":
		return "gitConnection:pull"
	default:
		return "gitConnection:*"
	}
}
