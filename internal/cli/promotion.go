package cli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
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
			"under 'checkout', link team projects to a projects-scope connection\n" +
			"under 'project', then 'promote' the instance to the remote or 'apply'\n" +
			"the remote into the instance.\n\n" +
			"'promote' and 'apply' run on the instance connection. 'promote'\n" +
			"commits and pushes every team project to the remote; 'apply' checks the\n" +
			"package and imports with overwrite behavior, creating, updating, or\n" +
			"removing content to match, or reports why it is blocked and writes\n" +
			"nothing. Both mutate versioned\n" +
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
			"Requires gitConnection:list.\n\n" + cursorCollectionHelp,
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
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 providers; hitting it warns and sets JSON collection.truncated)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all)")
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
	var collection *listCollection
	if f.all {
		page, collection, err = collectPage(ctx, client.ListPromotionProviders, listOpts)
	} else {
		page, err = client.ListPromotionProviders(ctx, listOpts)
	}
	if err != nil {
		return promotionAPIError(err, resolution, "provider list", promotionSubject{})
	}
	if f.output == outputJSON {
		err = writePageJSON(opts.Streams.Out, page, collection)
	} else {
		err = writePromotionProviderListText(opts, resolution, page)
	}
	return finishCollection(opts.Streams.Err, collection, err)
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

type promotionProviderCreateFlags struct {
	instance      instanceFlags
	input         string
	name          string
	providerType  string
	baseURL       string
	authType      string
	keyType       string
	username      string
	passwordStdin bool
	output        string
}

// documentFlagsSet reports whether any flag that builds the creation document
// was passed; those flags and --input are mutually exclusive.
func (f promotionProviderCreateFlags) documentFlagsSet() bool {
	return f.name != "" || f.providerType != "" || f.baseURL != "" || f.authType != "" ||
		f.keyType != "" || f.username != "" || f.passwordStdin
}

func newPromotionProviderCreateCommand(opts Options) *cobra.Command {
	var f promotionProviderCreateFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a promotion provider",
		Long: "Create one promotion provider from flags or from strict JSON supplied by\n" +
			"--input; the two ways are mutually exclusive and one is required.\n\n" +
			"Type git is a plain Git remote. Type gitlab is a Git host provider: the\n" +
			"server checks API access before saving and can list repositories. Pass\n" +
			"--base-url for self-managed GitLab; it becomes config {\"schemaVersion\":1,\n" +
			"\"baseUrl\":\"...\"} and applies to gitlab providers only.\n\n" +
			"Auth is ssh-key (the response carries the generated public key; add it\n" +
			"to the remote as a deploy key) or token. A token is never a flag: the\n" +
			"flag way asks for it without echo, or reads it from stdin with\n" +
			"--password-stdin. A git provider needs --username with a token; GitLab\n" +
			"ignores the username and uses n8n for Git transport.\n\n" +
			"The --input document has name (at most 128 characters), type (git or\n" +
			"gitlab), auth {\"authType\":\"ssh-key\"} with an optional keyType\n" +
			"(ed25519 or rsa, default ed25519) or {\"authType\":\"token\",\n" +
			"\"password\":\"...\"} plus username for git, and the optional config.\n" +
			"Unknown fields and trailing JSON documents are refused before\n" +
			"transport. Requires gitConnection:create. Follow with 'n8n promotion\n" +
			"connection create'.",
		Example: "  n8n promotion provider create --name github --type git --auth-type ssh-key\n" +
			"  printf '%s\\n' \"$GITLAB_TOKEN\" | n8n promotion provider create --name gitlab --type gitlab --base-url https://gitlab.example.com --auth-type token --password-stdin\n" +
			"  n8n promotion provider create --input provider.json --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromotionProviderCreate(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "promotion provider JSON file path, or - for stdin (maximum 1 MiB; default: build the document from --name and the other flags)")
	cmd.Flags().StringVar(&f.name, "name", "", "provider name, 1 to 128 characters (required without --input)")
	cmd.Flags().StringVar(&f.providerType, "type", "", "provider type: git or gitlab (default: git)")
	cmd.Flags().StringVar(&f.baseURL, "base-url", "", "GitLab base URL for self-managed GitLab, e.g. https://gitlab.example.com; gitlab only (default: none, no config is sent)")
	cmd.Flags().StringVar(&f.authType, "auth-type", "", "auth type: ssh-key or token (default: ssh-key)")
	cmd.Flags().StringVar(&f.keyType, "key-type", "", "SSH key type for ssh-key auth: ed25519 or rsa (default: the server's ed25519)")
	cmd.Flags().StringVar(&f.username, "username", "", "Git username for token auth; required for git, ignored by gitlab (default: none)")
	cmd.Flags().BoolVar(&f.passwordStdin, "password-stdin", false, "read the token for token auth from stdin instead of prompting without echo (default false; required when stdin is not a terminal)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the creation result: provider plus publicKey)")
	return cmd
}

func runPromotionProviderCreate(ctx context.Context, opts Options, f promotionProviderCreateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreatePromotionProviderRequest
	switch {
	case f.input != "" && f.documentFlagsSet():
		return fmt.Errorf("--input and the document flags (--name, --type, --base-url, --auth-type, --key-type, --username, --password-stdin) are mutually exclusive: pass one way")
	case f.input != "":
		if err := readPromotionDocument(opts, f.input, &request); err != nil {
			return err
		}
	case f.documentFlagsSet():
		var err error
		if request, err = promotionProviderFromFlags(opts, f); err != nil {
			return err
		}
	default:
		return fmt.Errorf("a provider document is required: pass --name with --type and --auth-type, or --input FILE (--input - for stdin)")
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
		return promotionAPIError(err, resolution, "provider create", promotionSubject{})
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, created)
	}
	return writePromotionProviderResult(opts, resolution, "Created", &created.Provider, f.output)
}

// promotionProviderFromFlags builds the creation document from flags. The
// document is checked before the token is read, so a typo never costs a
// prompt; the token itself comes from stdin or a no-echo prompt, never argv.
func promotionProviderFromFlags(opts Options, f promotionProviderCreateFlags) (n8n.CreatePromotionProviderRequest, error) {
	request := n8n.CreatePromotionProviderRequest{
		Name: f.name,
		Type: cmp.Or(f.providerType, n8n.PromotionProviderTypeGit),
		Auth: n8n.PromotionProviderAuth{
			AuthType: cmp.Or(f.authType, n8n.PromotionAuthTypeSSHKey),
			KeyType:  f.keyType,
			Username: f.username,
		},
	}
	if f.baseURL != "" {
		request.Config = &n8n.PromotionProviderHostConfig{SchemaVersion: 1, BaseURL: f.baseURL}
	}
	token := request.Auth.AuthType == n8n.PromotionAuthTypeToken
	if f.passwordStdin && !token {
		return request, fmt.Errorf("--password-stdin needs --auth-type token")
	}
	probe := request
	if token {
		probe.Auth.Password = "unread" // stands in until the token is read
	}
	if err := probe.Validate(); err != nil {
		return request, err
	}
	if !token {
		return request, nil
	}
	var secret n8n.Secret
	var err error
	switch {
	case f.passwordStdin:
		secret, err = readSecretFrom(opts.Streams.In)
	case opts.interactive():
		secret, err = opts.prompter().PromptSecret("Git provider token: ")
	default:
		return request, fmt.Errorf("no terminal available to read the provider token: pipe it with --password-stdin")
	}
	if err != nil {
		return request, err
	}
	request.Auth.Password = secret.Reveal()
	return request, nil
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
		return promotionAPIError(err, resolution, "provider get", promotionProviderSubject(id))
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
			"Only the fields present are changed; at least one of name, auth, or\n" +
			"config is required. Auth follows the create shape: ssh-key with an\n" +
			"optional key type, or token with a password (plus username for git\n" +
			"providers). The auth type cannot change. A gitlab provider can change\n" +
			"its base URL with config {\"schemaVersion\":1,\"baseUrl\":\"...\"}; a\n" +
			"different API destination needs auth in the same document.\n\n" +
			"Replacing the credential affects every connection attached to this\n" +
			"provider, so interactive runs ask for confirmation and non-interactive\n" +
			"runs require --yes. Credentials travel only as --input because secrets\n" +
			"are never flags. A 409 means another request changed the provider\n" +
			"first: reload it and retry. Requires gitConnection:update.",
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
		return promotionAPIError(err, resolution, "provider update", promotionProviderSubject(id))
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
		return promotionAPIError(err, resolution, "provider delete", promotionProviderSubject(id))
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
			"promote, and apply. Requires gitConnection:list.\n\n" + cursorCollectionHelp,
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
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 connections; hitting it warns and sets JSON collection.truncated)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all)")
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
	var collection *listCollection
	if f.all {
		collect := func(ctx context.Context, base n8n.ListOptions) (n8n.Page[n8n.PromotionConnection], error) {
			return client.ListPromotionConnections(ctx, n8n.ListPromotionConnectionsOptions{
				ListOptions: base, Scope: listOpts.Scope, ProviderID: listOpts.ProviderID,
			})
		}
		page, collection, err = collectPage(ctx, collect, listOpts.ListOptions)
	} else {
		page, err = client.ListPromotionConnections(ctx, listOpts)
	}
	if err != nil {
		return promotionAPIError(err, resolution, "connection list", promotionSubject{})
	}
	if f.output == outputJSON {
		err = writePageJSON(opts.Streams.Out, page, collection)
	} else {
		err = writePromotionConnectionListText(opts, resolution, page)
	}
	return finishCollection(opts.Streams.Err, collection, err)
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
			"On a gitlab provider the remote URL must belong to the provider: the\n" +
			"connection stores only the repository path (reported read-only as\n" +
			"target.repositoryPath, never sent) and derives remoteUrl from the\n" +
			"provider base URL. Only one instance-scope connection can exist.\n" +
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
		return promotionAPIError(err, resolution, "connection create", promotionProviderSubject(request.ProviderID))
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
		return promotionAPIError(err, resolution, "connection get", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "connection update", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "connection delete", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "config apply set", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "config promote set", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "config delete", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "checkout clone", promotionConnectionSubject(id))
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
		return promotionAPIError(err, resolution, "checkout disconnect", promotionConnectionSubject(id))
	}
	return writePromotionCheckout(opts, resolution, "Disconnected", checkout, f.output)
}

// Projects.

func newPromotionProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects linked to a promotion connection",
		Long: "Manage which team projects a projects-scope promotion connection\n" +
			"syncs; a project's own connection takes precedence over the instance\n" +
			"connection for project-level sync. Start with\n" +
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
		Long: "List the team projects linked to one projects-scope promotion\n" +
			"connection. Connection-level 'promote' and 'apply' run on the instance\n" +
			"connection and do not use these links. Find the\n" +
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
		return promotionAPIError(err, resolution, "project list", promotionConnectionSubject(id))
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
		Long: "Link one team project to one projects-scope promotion connection, so\n" +
			"project-level sync uses that connection instead of the instance\n" +
			"connection. A project can belong to only one connection: adding a\n" +
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
		return promotionAPIError(err, resolution, "project add", promotionProjectLinkSubject(id, projectID))
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
			"so project-level sync falls back to the instance connection. Re-add it\n" +
			"later with 'n8n\n" +
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
	question := fmt.Sprintf("Remove project %q from promotion connection %q on %s? The project content stays; project-level sync falls back to the instance connection.", projectID, id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.RemoveProjectFromPromotionConnection(ctx, id, projectID); err != nil {
		return promotionAPIError(err, resolution, "project remove", promotionProjectLinkSubject(id, projectID))
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
		Long: "Export every team project, commit it, and push to the configured base\n" +
			"branch or a new timestamped branch (the result names it). Personal\n" +
			"projects are ignored. This runs on the instance connection only, so\n" +
			"project links from 'n8n promotion project add' do not select what is\n" +
			"promoted. The promote checkout must be cloned first: run 'n8n promotion\n" +
			"checkout clone CONNECTION_ID promote'. When workflows reference\n" +
			"variables, the API key also needs variable:list.\n\n" +
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
		return promotionAPIError(err, resolution, "promote", promotionConnectionSubject(id))
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
		Long: "Check the full package at the apply branch tip and import it into the\n" +
			"instance, overwriting to match. Runs on the instance connection; the\n" +
			"apply checkout must be cloned first.\n\n" +
			"The server answers one of three statuses, and only one writes:\n" +
			"  applied         content was imported; counts and warnings follow.\n" +
			"  blocked         preflight stopped before any write: missing projects,\n" +
			"                  missing credential or variable bindings, access\n" +
			"                  requirements, or conflicts (including project-deletion\n" +
			"                  and data-table destructive-change). Nothing was applied.\n" +
			"  source-changed  the branch moved since it was reviewed. Nothing was\n" +
			"                  applied.\n\n" +
			"An applied run rewrites instance content from the remote and cannot be\n" +
			"undone: workflows, folders, credentials, data tables, variables, and\n" +
			"tags are created, updated, or removed to match the branch, and\n" +
			"credential gaps become stubs. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires gitConnection:pull.\n\n" +
			"Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 any other error\n" +
			"or an unknown status. --output json prints the full result on stdout\n" +
			"in every status; the next step goes to stderr.",
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
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the full apply result: status, git, and counts or preflight)")
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
		return promotionAPIError(err, resolution, "apply", promotionConnectionSubject(id))
	}
	if f.output == outputJSON {
		err = writeJSON(opts.Streams.Out, result)
	} else {
		err = writePromotionApplyResult(opts, resolution, result)
	}
	if err != nil {
		return err
	}
	return finishPromotionApply(opts, id, result)
}

// promotionApplyStoppedError is returned after an apply answered 200 without
// importing: blocked at preflight, the source moved, or a status this CLI does
// not know. The result was already printed; Run reports the message and maps
// the status to its own exit code so scripts can tell these apart from
// transport and validation errors.
type promotionApplyStoppedError struct {
	connectionID, status string
}

func (e *promotionApplyStoppedError) Error() string {
	switch e.status {
	case n8n.PromotionApplyStatusBlocked:
		return fmt.Sprintf("promotion apply of connection %s was blocked at preflight; nothing was applied", e.connectionID)
	case n8n.PromotionApplyStatusSourceChanged:
		return fmt.Sprintf("promotion apply of connection %s stopped because the source branch moved; nothing was applied", e.connectionID)
	default:
		return fmt.Sprintf("promotion apply of connection %s answered unknown status %q; check the instance before assuming anything was applied", e.connectionID, e.status)
	}
}

func (e *promotionApplyStoppedError) exitCode() int {
	switch e.status {
	case n8n.PromotionApplyStatusBlocked:
		return ExitPromotionBlocked
	case n8n.PromotionApplyStatusSourceChanged:
		return ExitPromotionSourceChanged
	default:
		return ExitError
	}
}

// finishPromotionApply writes the next-step hint for a result that wrote
// nothing and returns the matching error; an applied result returns nil.
func finishPromotionApply(opts Options, id string, result *n8n.ApplyPromotionResult) error {
	if result.Applied() {
		return nil
	}
	source := result.ExpectedSource()
	switch result.Status {
	case n8n.PromotionApplyStatusBlocked:
		fmt.Fprintln(opts.Streams.Err, "Nothing was applied: preflight found items to resolve first. Create the missing projects, bind the missing credentials and variables, and grant the listed access.")
		if promotionNeedsConfirmation(result.Preflight) {
			fmt.Fprintln(opts.Streams.Err, "project-deletion and destructive-change conflicts are applied only with explicit confirmation when the apply is continued.")
		}
		fmt.Fprintf(opts.Streams.Err, "Then continue the apply with expected source configId=%s branchName=%s commitSha=%s (an 'apply-continue' command is upcoming in this CLI).\n",
			emptyDash(source.ConfigID), emptyDash(source.BranchName), emptyDash(source.CommitSHA))
	case n8n.PromotionApplyStatusSourceChanged:
		fmt.Fprintf(opts.Streams.Err, "Nothing was applied: the branch moved since it was reviewed. Review the branch again, then re-run 'n8n promotion apply %s'.\n", id)
	default:
		fmt.Fprintln(opts.Streams.Err, "This CLI does not know the apply status; read the full result with --output json.")
	}
	return &promotionApplyStoppedError{connectionID: id, status: result.Status}
}

func promotionNeedsConfirmation(preflight *n8n.PromotionApplyPreflight) bool {
	if preflight == nil {
		return false
	}
	for _, conflict := range preflight.Conflicts {
		if conflict.Code == n8n.PromotionConflictProjectDeletion || conflict.Code == n8n.PromotionConflictDestructiveChange {
			return true
		}
	}
	return false
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
	if provider.Config != nil && provider.Config.BaseURL != nil {
		fmt.Fprintf(tw, "Base URL:\t%s\n", emptyDash(*provider.Config.BaseURL))
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
	if connection.Target.RepositoryPath != "" {
		fmt.Fprintf(tw, "Repository path:\t%s\n", connection.Target.RepositoryPath)
	}
	fmt.Fprintf(tw, "Provider:\t%s\n", emptyDash(connection.Provider.Name))
	if connection.Configs.Apply != nil {
		fmt.Fprintf(tw, "Apply branch:\t%s\n", emptyDash(connection.Configs.Apply.Settings.BranchName))
		if checkout := connection.Configs.Apply.Checkout; checkout != nil {
			fmt.Fprintf(tw, "Apply checkout:\t%s\n", promotionCheckoutState(*checkout))
		}
	}
	if connection.Configs.Promote != nil {
		fmt.Fprintf(tw, "Promote base:\t%s\n", emptyDash(connection.Configs.Promote.Settings.BaseBranchName))
		if checkout := connection.Configs.Promote.Checkout; checkout != nil {
			fmt.Fprintf(tw, "Promote checkout:\t%s\n", promotionCheckoutState(*checkout))
		}
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

// promotionCheckoutState says whether a direction is cloned and whether the
// clone still matches its config (a stale clone needs 'checkout clone' again).
func promotionCheckoutState(checkout n8n.PromotionConfigCheckout) string {
	switch {
	case !checkout.HasCheckout:
		return "not cloned"
	case checkout.MatchesConfig:
		return "cloned"
	default:
		return "cloned, does not match the config; clone again"
	}
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
	switch {
	case result.Applied():
		return writePromotionAppliedResult(opts, resolution, result)
	case result.Status == n8n.PromotionApplyStatusBlocked:
		return writePromotionBlockedResult(opts, resolution, result)
	default:
		tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
		writePromotionApplyHeader(tw, resolution, result)
		return tw.Flush()
	}
}

func writePromotionApplyHeader(tw *tabwriter.Writer, resolution config.Resolution, result *n8n.ApplyPromotionResult) {
	label := "Not applied"
	if result.Applied() {
		label = "Applied"
	}
	fmt.Fprintf(tw, "%s:\t%s\n", label, result.ConnectionID)
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(result.Status))
	fmt.Fprintf(tw, "Config:\t%s\n", emptyDash(result.ConfigID))
	fmt.Fprintf(tw, "Commit:\t%s\n", emptyDash(result.Git.CommitSHA))
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(result.Git.BranchName))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
}

func writePromotionAppliedResult(opts Options, resolution config.Resolution, result *n8n.ApplyPromotionResult) error {
	var counts n8n.PromotionApplyCounts
	if result.Counts != nil {
		counts = *result.Counts
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	writePromotionApplyHeader(tw, resolution, result)
	fmt.Fprintln(tw, "\nENTITY\tDETAIL\tCOUNT")
	fmt.Fprintf(tw, "projects\tcreated\t%d\n", counts.Projects.Created)
	fmt.Fprintf(tw, "projects\tupdated\t%d\n", counts.Projects.Updated)
	fmt.Fprintf(tw, "projects\tskipped\t%d\n", counts.Projects.Skipped)
	fmt.Fprintf(tw, "projects\tdeleted\t%d\n", counts.Projects.Deleted)
	fmt.Fprintf(tw, "folders\tcreated\t%d\n", counts.Folders.Created)
	fmt.Fprintf(tw, "folders\tskipped\t%d\n", counts.Folders.Skipped)
	fmt.Fprintf(tw, "folders\tremoved\t%d\n", counts.Folders.Removed)
	fmt.Fprintf(tw, "workflows\tcreated\t%d\n", counts.Workflows.Created)
	fmt.Fprintf(tw, "workflows\tupdated\t%d\n", counts.Workflows.Updated)
	fmt.Fprintf(tw, "workflows\tskipped\t%d\n", counts.Workflows.Skipped)
	fmt.Fprintf(tw, "workflows\tarchived\t%d\n", counts.Workflows.Archived)
	fmt.Fprintf(tw, "workflows\tdeleted\t%d\n", counts.Workflows.Deleted)
	fmt.Fprintf(tw, "workflows\tpublished\t%d\n", counts.Workflows.Publishing.Published)
	fmt.Fprintf(tw, "workflows\tunpublished\t%d\n", counts.Workflows.Publishing.Unpublished)
	fmt.Fprintf(tw, "workflows\tunchanged\t%d\n", counts.Workflows.Publishing.Unchanged)
	fmt.Fprintf(tw, "workflows\tblocked\t%d\n", counts.Workflows.Publishing.Blocked)
	fmt.Fprintf(tw, "workflows\tfailed\t%d\n", counts.Workflows.Publishing.Failed)
	fmt.Fprintf(tw, "credentials\tmatched\t%d\n", counts.Credentials.Matched)
	fmt.Fprintf(tw, "credentials\tstubbed\t%d\n", counts.Credentials.Stubbed)
	fmt.Fprintf(tw, "dataTables\tmatched\t%d\n", counts.DataTables.Matched)
	fmt.Fprintf(tw, "dataTables\tcreated\t%d\n", counts.DataTables.Created)
	fmt.Fprintf(tw, "dataTables\tupdated\t%d\n", counts.DataTables.Updated)
	fmt.Fprintf(tw, "variables\tmatched\t%d\n", counts.Variables.Matched)
	fmt.Fprintf(tw, "variables\tcreated\t%d\n", counts.Variables.Created)
	fmt.Fprintf(tw, "variables\tupdated\t%d\n", counts.Variables.Updated)
	fmt.Fprintf(tw, "variables\tstubbed\t%d\n", counts.Variables.Stubbed)
	fmt.Fprintf(tw, "variables\tmissing\t%d\n", counts.Variables.Missing)
	fmt.Fprintf(tw, "tags\tmatched\t%d\n", counts.Tags.Matched)
	fmt.Fprintf(tw, "tags\tcreated\t%d\n", counts.Tags.Created)
	fmt.Fprintf(tw, "tags\trenamed\t%d\n", counts.Tags.Renamed)
	fmt.Fprintf(tw, "tags\treconciled\t%d\n", counts.Tags.Reconciled)
	fmt.Fprintf(tw, "tags\tskipped\t%d\n", counts.Tags.Skipped)
	writePromotionIssues(tw, "Warnings", result.Warnings)
	if err := tw.Flush(); err != nil {
		return err
	}
	if counts.Variables.Missing > 0 {
		fmt.Fprintf(opts.Streams.Err, "%d variable(s) still unresolved: workflows referencing them may fail until the variables exist.\n", counts.Variables.Missing)
	}
	if counts.Workflows.Publishing.Failed > 0 || counts.Workflows.Publishing.Blocked > 0 {
		fmt.Fprintln(opts.Streams.Err, "Some workflows did not publish: see the publishing counts in --output json.")
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintf(opts.Streams.Err, "The apply finished with %d warning(s): see the WARNINGS table.\n", len(result.Warnings))
	}
	return nil
}

func writePromotionBlockedResult(opts Options, resolution config.Resolution, result *n8n.ApplyPromotionResult) error {
	var preflight n8n.PromotionApplyPreflight
	if result.Preflight != nil {
		preflight = *result.Preflight
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	writePromotionApplyHeader(tw, resolution, result)
	if len(preflight.MissingProjects) > 0 {
		fmt.Fprintf(tw, "\nMISSING PROJECTS (%d)\nID\tNAME\n", len(preflight.MissingProjects))
		for _, project := range preflight.MissingProjects {
			fmt.Fprintf(tw, "%s\t%s\n", emptyDash(project.ID), emptyDash(project.Name))
		}
	}
	writePromotionIssues(tw, "Missing bindings", preflight.MissingBindings)
	writePromotionIssues(tw, "Access requirements", preflight.AccessRequirements)
	writePromotionIssues(tw, "Conflicts", preflight.Conflicts)
	writePromotionIssues(tw, "Warnings", preflight.Warnings)
	if preflight.Empty() {
		fmt.Fprintln(tw, "\nPreflight:\tno items reported")
	}
	return tw.Flush()
}

// writePromotionIssues renders one preflight group as a table: kind, code,
// name, a one-line detail, and which workflows use the item.
func writePromotionIssues(tw *tabwriter.Writer, title string, issues []n8n.PromotionApplyIssue) {
	if len(issues) == 0 {
		return
	}
	fmt.Fprintf(tw, "\n%s (%d)\nKIND\tCODE\tNAME\tDETAIL\tUSED BY\n", strings.ToUpper(title), len(issues))
	for _, issue := range issues {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", emptyDash(issue.Kind), emptyDash(issue.Code),
			emptyDash(promotionIssueName(issue)), emptyDash(promotionIssueDetail(issue)), emptyDash(promotionIssueConsumers(issue)))
	}
}

func promotionIssueName(issue n8n.PromotionApplyIssue) string {
	if issue.Name == "" && issue.Project != nil {
		return issue.Project.Name
	}
	return issue.Name
}

func promotionIssueDetail(issue n8n.PromotionApplyIssue) string {
	var parts []string
	switch issue.Code {
	case n8n.PromotionConflictProjectDeletion:
		parts = append(parts, "the apply deletes this team project and everything in it")
	case n8n.PromotionConflictDestructiveChange:
		for _, change := range issue.Changes {
			if change.Destructive {
				parts = append(parts, promotionChangeText(change))
			}
		}
	}
	if issue.CredentialType != "" {
		parts = append(parts, "type "+issue.CredentialType)
	}
	if len(issue.ExpectedTypes) > 0 {
		parts = append(parts, "expected "+strings.Join(issue.ExpectedTypes, "|"))
	}
	if issue.TargetType != "" {
		parts = append(parts, "instance has "+issue.TargetType)
	}
	if issue.UsageScope != "" {
		parts = append(parts, "usage scope "+issue.UsageScope)
	}
	if issue.Scope != nil {
		scope := issue.Scope.Kind
		if issue.Scope.Project != nil {
			scope += " " + issue.Scope.Project.Name
		}
		parts = append(parts, "scope "+scope)
	}
	if issue.SourceID != nil && *issue.SourceID != "" {
		parts = append(parts, "source ID "+*issue.SourceID)
	}
	if issue.Project != nil && issue.Code != n8n.PromotionConflictProjectDeletion {
		parts = append(parts, "project "+issue.Project.ID)
	}
	if issue.OwnerProject != nil {
		parts = append(parts, "owner "+issue.OwnerProject.Name)
	}
	return strings.Join(parts, "; ")
}

func promotionChangeText(change n8n.PromotionDataTableChange) string {
	switch {
	case change.Column != "" && change.From != "" && change.To != "":
		return fmt.Sprintf("%s %s %s->%s", change.Kind, change.Column, change.From, change.To)
	case change.Column != "":
		return fmt.Sprintf("%s %s", change.Kind, change.Column)
	default:
		return change.Kind
	}
}

func promotionIssueConsumers(issue n8n.PromotionApplyIssue) string {
	workflows := len(issue.Workflows)
	for _, consumer := range issue.Consumers {
		workflows += len(consumer.Workflows)
	}
	if workflows == 0 {
		return ""
	}
	if len(issue.Consumers) == 0 {
		return fmt.Sprintf("%d workflow(s)", workflows)
	}
	return fmt.Sprintf("%d workflow(s) in %d project(s)", workflows, len(issue.Consumers))
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

// promotionSubject names what one promotion route addresses, so a 404 can say
// which ID is missing. The zero value is a collection route, where a 404 can
// only mean the instance does not serve the module.
type promotionSubject struct {
	what string // e.g. promotion connection "conn-1"
	find string // the command that lists valid IDs
}

func promotionProviderSubject(id string) promotionSubject {
	return promotionSubject{what: fmt.Sprintf("promotion provider %q", id), find: "'n8n promotion provider list'"}
}

func promotionConnectionSubject(id string) promotionSubject {
	return promotionSubject{what: fmt.Sprintf("promotion connection %q", id), find: "'n8n promotion connection list'"}
}

func promotionProjectLinkSubject(id, projectID string) promotionSubject {
	return promotionSubject{
		what: fmt.Sprintf("promotion connection %q or project %q", id, projectID),
		find: "'n8n promotion connection list' and 'n8n project list'",
	}
}

func promotionAPIError(err error, resolution config.Resolution, action string, subject promotionSubject) error {
	switch {
	case n8n.IsConflict(err) && action == "provider delete":
		return fmt.Errorf("%s rejected the promotion provider delete (409): the provider still has attached connections, so nothing changed; remove them first, then delete the provider: %w", resolution.URL, err)
	case n8n.IsConflict(err) && action == "project add":
		return fmt.Errorf("%s rejected the promotion project add (409): the project is already linked to a connection, so nothing changed; remove it there first, then add it here: %w", resolution.URL, err)
	case n8n.IsConflict(err) && action == "provider update":
		return fmt.Errorf("%s rejected the promotion provider update (409): another request changed the provider's credential or configuration first, so nothing changed; reload it with 'n8n promotion provider get' and retry: %w", resolution.URL, err)
	case n8n.IsConflict(err):
		return fmt.Errorf("%s rejected the promotion %s (409): the connection or project is in a conflicting state, so nothing changed; list the current state and retry: %w", resolution.URL, action, err)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the promotion %s (400): check the IDs, the direction (apply or promote), the document fields, the branch names, and the commit message: %w", resolution.URL, action, err)
	case n8n.IsStatus(err, http.StatusUnprocessableEntity):
		return fmt.Errorf("%s rejected the promotion %s (422): the remote content cannot be imported as-is (missing references or unresolvable content); resolve the references and retry: %w", resolution.URL, action, err)
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "promotion "+action, allOf(promotionScope(action)), "", promotionResource)
	case n8n.IsNotFound(err) && subject.what != "" && !promotionRouteMissing(err):
		return fmt.Errorf("%s has no %s (404): check the ID with %s: %w", resolution.URL, subject.what, subject.find, err)
	case n8n.IsNotFound(err) && subject.what != "":
		return fmt.Errorf("%s answered 404 for %s: either the ID does not exist (check it with %s) or the instance does not serve promotions (run %s): %w",
			resolution.URL, subject.what, subject.find, discoverHint(promotionResource), err)
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s does not serve promotions (%d): it needs the Promotions module (a GitConnections-generation instance answers here instead); run %s to see what this instance offers: %w",
			resolution.URL, n8n.StatusCodeOf(err), discoverHint(promotionResource), err)
	default:
		return apiError(err, resolution, promotionResource)
	}
}

// promotionRouteMissing reports whether a 404 carries nothing beyond the
// status: no code and no message, or only a generic "not found" or framework
// "Cannot POST ..." text. That is what an unmounted route answers; a missing
// connection, provider or project answers with its own message.
func promotionRouteMissing(err error) bool {
	apiErr, ok := errors.AsType[*n8n.APIError](err)
	if !ok {
		return true
	}
	if apiErr.Code != "" {
		return false
	}
	message := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(apiErr.Message), "."))
	return message == "" || message == "not found" || strings.HasPrefix(message, "cannot ")
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
	case "provider repositories", "provider branches":
		return "gitConnection:discover"
	default:
		return "gitConnection:*"
	}
}
