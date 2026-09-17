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
	credentialResource    = "credential"
	maxCredentialDocument = 1 << 20
)

func newCredentialCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage node credentials without exposing secrets",
		Long: "Manage credentials stored by an n8n instance.\n\n" +
			"List and get return metadata only; n8n omits stored credential data. Create and\n" +
			"update accept secret-bearing JSON only from --file or explicit --stdin, never from\n" +
			"flags or arguments. --stdin must be piped so terminal input cannot echo secrets.\n\n" +
			"Listing is restricted by n8n to instance owners and admins. Updates, deletes,\n" +
			"tests and transfers also depend on credential ownership and granted scopes.",
		Example: "  n8n credential list\n" +
			"  n8n credential get CREDENTIAL_ID\n" +
			"  n8n credential schema githubApi\n" +
			"  n8n credential create --file credential.json\n" +
			"  generate-json | n8n credential update CREDENTIAL_ID --stdin",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newCredentialListCommand(opts),
		newCredentialCreateCommand(opts),
		newCredentialGetCommand(opts),
		newCredentialUpdateCommand(opts),
		newCredentialDeleteCommand(opts),
		newCredentialTestCommand(opts),
		newCredentialSchemaCommand(opts),
		newCredentialTransferCommand(opts),
	)
	return cmd
}

type credentialListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newCredentialListCommand(opts Options) *cobra.Command {
	var f credentialListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credential metadata",
		Long: "List credential metadata from the selected instance. Stored credential data is\n" +
			"never returned or printed. n8n restricts this endpoint to instance owners and\n" +
			"admins. Use --all to follow cursors, capped at 10,000 credentials.\n\n" +
			"Requires credential:list. A 403 can mean either missing scope or insufficient\n" +
			"instance role; inspect capabilities with 'n8n discover --resource credential'.",
		Example: "  n8n credential list\n" +
			"  n8n credential list --limit 50 --output json\n" +
			"  n8n credential list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCredentialList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "credentials per API page (default: server default)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow all pages (maximum 10,000 credentials)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runCredentialList(ctx context.Context, opts Options, f credentialListFlags) error {
	if err := validateOutput(f.output); err != nil {
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
	var page n8n.Page[n8n.Credential]
	if f.all {
		page.Data, err = n8n.Collect(ctx, client.ListCredentials, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListCredentials(ctx, listOpts)
	}
	if err != nil {
		if n8n.IsForbidden(err) {
			return fmt.Errorf("%s denied credential listing (403): this endpoint requires credential:list and an instance owner or admin role; run %s to inspect scopes", resolution.URL, discoverHint(credentialResource))
		}
		return apiError(err, resolution, credentialResource)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeCredentialListText(opts, resolution, page)
}

func writeCredentialListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.Credential]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Credentials:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tTYPE\tSHARED\tUPDATED")
		for _, credential := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", credential.ID, credential.Name, credential.Type, len(credential.Shared), credential.UpdatedAt)
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No credentials were returned. This endpoint includes metadata only, never secret data.")
	}
	return nil
}

type credentialInputFlags struct {
	file  string
	stdin bool
}

func (f *credentialInputFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.file, "file", "", "read secret-bearing JSON from file (protect and delete the file after use)")
	cmd.Flags().BoolVar(&f.stdin, "stdin", false, "read secret-bearing JSON from piped stdin (refused for an interactive terminal)")
}

func (f credentialInputFlags) read(opts Options, dst any) error {
	if (f.file == "" && !f.stdin) || (f.file != "" && f.stdin) {
		return fmt.Errorf("choose exactly one credential input: --file PATH or --stdin")
	}
	if f.stdin {
		if opts.interactive() {
			return fmt.Errorf("refusing to read credential JSON from an interactive terminal because input may echo; pipe data to --stdin or use --file")
		}
		return decodeCredentialDocument(opts.Streams.In, dst)
	}
	file, err := os.Open(f.file)
	if err != nil {
		return fmt.Errorf("open credential input %q: %w", f.file, err)
	}
	defer file.Close()
	if err := decodeCredentialDocument(file, dst); err != nil {
		return fmt.Errorf("read credential input %q: %w", f.file, err)
	}
	return nil
}

func decodeCredentialDocument(r io.Reader, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(r, maxCredentialDocument+1))
	if err != nil {
		return fmt.Errorf("read credential JSON: %w", err)
	}
	defer clear(raw)
	if len(raw) > maxCredentialDocument {
		return fmt.Errorf("credential JSON exceeds %d bytes", maxCredentialDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode credential JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode credential JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode credential JSON: %w", err)
	}
	return nil
}

type credentialWriteFlags struct {
	instance instanceFlags
	input    credentialInputFlags
	output   string
}

func newCredentialCreateCommand(opts Options) *cobra.Command {
	var f credentialWriteFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a credential from secret JSON",
		Long: "Create a credential from one JSON document. Required fields are name, type and\n" +
			"data; optional fields are projectId and isResolvable. Because data contains\n" +
			"secrets, JSON is accepted only through --file or piped --stdin, never argv.\n\n" +
			"The request body is excluded from debug logs. Output contains metadata only.\n" +
			"Requires credential:create; project creation can have additional role restrictions.",
		Example: "  n8n credential create --file credential.json\n" +
			"  secret-generator | n8n credential create --stdin --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCredentialCreate(cmd.Context(), opts, f)
		},
	}
	registerCredentialWriteFlags(cmd, &f)
	return cmd
}

func runCredentialCreate(ctx context.Context, opts Options, f credentialWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreateCredentialRequest
	if err := f.input.read(opts, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	credential, err := client.CreateCredential(ctx, request)
	if err != nil {
		return apiError(err, resolution, credentialResource)
	}
	return writeCredentialResult(opts, resolution, "Created", credential, f.output)
}

func newCredentialUpdateCommand(opts Options) *cobra.Command {
	var f credentialWriteFlags
	cmd := &cobra.Command{
		Use:   "update <credential-id>",
		Short: "Update a credential from secret JSON",
		Long: "Update an owned credential from one JSON document. Accepted fields are name, type,\n" +
			"data, isGlobal, isResolvable and isPartialData. Set isPartialData=true to merge\n" +
			"provided data with stored data; false replaces the full data object. Changing type\n" +
			"requires data. Managed credentials cannot be edited.\n\n" +
			"Secret JSON is accepted only through --file or piped --stdin and is excluded from\n" +
			"debug logs and output. Requires credential:update and ownership.",
		Example: "  n8n credential update CREDENTIAL_ID --file update.json\n" +
			"  secret-generator | n8n credential update CREDENTIAL_ID --stdin --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	registerCredentialWriteFlags(cmd, &f)
	return cmd
}

func runCredentialUpdate(ctx context.Context, opts Options, id string, f credentialWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateCredentialArgument(id); err != nil {
		return err
	}
	var request n8n.UpdateCredentialRequest
	if err := f.input.read(opts, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	credential, err := client.UpdateCredential(ctx, id, request)
	if err != nil {
		return apiError(err, resolution, credentialResource)
	}
	return writeCredentialResult(opts, resolution, "Updated", credential, f.output)
}

func registerCredentialWriteFlags(cmd *cobra.Command, f *credentialWriteFlags) {
	f.instance.register(cmd)
	f.input.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (output never contains credential data)")
}

type credentialReadFlags struct {
	instance instanceFlags
	output   string
}

func newCredentialGetCommand(opts Options) *cobra.Command {
	var f credentialReadFlags
	cmd := &cobra.Command{
		Use:   "get <credential-id>",
		Short: "Get credential metadata",
		Long: "Get one credential by ID. n8n omits stored credential data, and the CLI drops\n" +
			"any unexpected data field before output. Requires credential:read and access to\n" +
			"the credential.",
		Example: "  n8n credential get CREDENTIAL_ID\n" +
			"  n8n credential get CREDENTIAL_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialGet(cmd.Context(), opts, args[0], f)
		},
	}
	registerCredentialReadFlags(cmd, &f)
	return cmd
}

func runCredentialGet(ctx context.Context, opts Options, id string, f credentialReadFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateCredentialArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	credential, err := client.GetCredential(ctx, id)
	if err != nil {
		return apiError(err, resolution, credentialResource)
	}
	return writeCredentialResult(opts, resolution, "Credential", credential, f.output)
}

func registerCredentialReadFlags(cmd *cobra.Command, f *credentialReadFlags) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (output never contains credential data)")
}

func writeCredentialResult(opts Options, resolution config.Resolution, action string, credential *n8n.Credential, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, credential)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, credential.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", credential.Name)
	fmt.Fprintf(tw, "Type:\t%s\n", credential.Type)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if credential.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", credential.UpdatedAt)
	}
	if credential.IsManaged {
		fmt.Fprintln(tw, "Managed:\tyes (cannot be edited through this API)")
	}
	fmt.Fprintln(tw, "Secret data:\tomitted")
	return tw.Flush()
}

type credentialDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newCredentialDeleteCommand(opts Options) *cobra.Command {
	var f credentialDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <credential-id>",
		Short: "Permanently delete a credential",
		Long: "Permanently delete an owned credential. Workflows that use it may stop working;\n" +
			"the operation does not delete those workflows. Interactive runs ask for\n" +
			"confirmation. Non-interactive runs require --yes. Requires credential:delete.",
		Example: "  n8n credential delete CREDENTIAL_ID\n" +
			"  n8n credential delete CREDENTIAL_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (output never contains credential data)")
	return cmd
}

func runCredentialDelete(ctx context.Context, opts Options, id string, f credentialDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateCredentialArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete credential %q from %s? Workflows using it may stop working.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	credential, err := client.DeleteCredential(ctx, id)
	if err != nil {
		return apiError(err, resolution, credentialResource)
	}
	return writeCredentialResult(opts, resolution, "Deleted", credential, f.output)
}

func newCredentialTestCommand(opts Options) *cobra.Command {
	var f credentialReadFlags
	cmd := &cobra.Command{
		Use:   "test <credential-id>",
		Short: "Test stored credential data",
		Long: "Ask n8n to test one credential using its stored data. No credential secret is sent\n" +
			"by the CLI or returned in output. Requires credential:read and access to the\n" +
			"credential. A test result with status Error is still a successful API call.",
		Example: "  n8n credential test CREDENTIAL_ID\n" +
			"  n8n credential test CREDENTIAL_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialTest(cmd.Context(), opts, args[0], f)
		},
	}
	registerCredentialReadFlags(cmd, &f)
	return cmd
}

func runCredentialTest(ctx context.Context, opts Options, id string, f credentialReadFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateCredentialArgument(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	result, err := client.TestCredential(ctx, id)
	if err != nil {
		return apiError(err, resolution, credentialResource)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Credential:\t%s\n", id)
	fmt.Fprintf(tw, "Status:\t%s\n", result.Status)
	fmt.Fprintf(tw, "Message:\t%s\n", result.Message)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type credentialSchemaFlags struct {
	instance instanceFlags
}

func newCredentialSchemaCommand(opts Options) *cobra.Command {
	var f credentialSchemaFlags
	cmd := &cobra.Command{
		Use:   "schema <credential-type>",
		Short: "Show credential data JSON Schema",
		Long: "Show the JSON Schema for a credential type. Use it to construct the data object\n" +
			"for create or update without putting secret values in argv. Schema output is always\n" +
			"pretty-printed JSON. This endpoint declares no additional scope.",
		Example: "  n8n credential schema githubApi\n" +
			"  n8n credential schema slackOAuth2Api --context production",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialSchema(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	return cmd
}

func runCredentialSchema(ctx context.Context, opts Options, credentialType string, f credentialSchemaFlags) error {
	if strings.TrimSpace(credentialType) == "" || strings.TrimSpace(credentialType) != credentialType {
		return fmt.Errorf("credential type is required and must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	schema, err := client.CredentialSchema(ctx, credentialType)
	if err != nil {
		return apiError(err, resolution, credentialResource)
	}
	var value any
	if err := json.Unmarshal(schema, &value); err != nil {
		return fmt.Errorf("decode credential schema: %w", err)
	}
	return writeJSON(opts.Streams.Out, value)
}

type credentialTransferFlags struct {
	instance           instanceFlags
	destinationProject string
	yes                bool
	output             string
}

func newCredentialTransferCommand(opts Options) *cobra.Command {
	var f credentialTransferFlags
	cmd := &cobra.Command{
		Use:   "transfer <credential-id>",
		Short: "Transfer a credential to another project",
		Long: "Transfer an owned credential to another project. This changes who can access it and\n" +
			"may break workflows in the source project. Interactive runs ask for confirmation;\n" +
			"non-interactive runs require --yes. Requires credential:move and suitable project\n" +
			"permissions.",
		Example: "  n8n credential transfer CREDENTIAL_ID --destination-project PROJECT_ID\n" +
			"  n8n credential transfer CREDENTIAL_ID --destination-project PROJECT_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialTransfer(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.destinationProject, "destination-project", "", "destination project ID (required)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm transfer without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json")
	return cmd
}

func runCredentialTransfer(ctx context.Context, opts Options, id string, f credentialTransferFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateCredentialArgument(id); err != nil {
		return err
	}
	if strings.TrimSpace(f.destinationProject) == "" {
		return fmt.Errorf("--destination-project is required")
	}
	if strings.TrimSpace(f.destinationProject) != f.destinationProject {
		return fmt.Errorf("destination project ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Transfer credential %q to project %q on %s? Access from the source project may be lost.", id, f.destinationProject, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.TransferCredential(ctx, id, f.destinationProject); err != nil {
		return apiError(err, resolution, credentialResource)
	}
	result := struct {
		ID                   string `json:"id"`
		DestinationProjectID string `json:"destinationProjectId"`
		Transferred          bool   `json:"transferred"`
	}{id, f.destinationProject, true}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	fmt.Fprintf(opts.Streams.Out, "Transferred credential %s to project %s on %s.\n", id, f.destinationProject, resolution.URL)
	return nil
}

func validateCredentialArgument(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("credential ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("credential ID must not start or end with whitespace")
	}
	return nil
}
