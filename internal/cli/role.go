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
	roleResource    = "role"
	maxRoleDocument = 1 << 20
)

func newRoleCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage global and project roles",
		Long: "Manage global and project roles on an n8n instance. Start with 'list' to\n" +
			"inspect role slugs, licensing, scopes, and whether each role is built in. Use\n" +
			"'get' for complete details, then create, replace, or delete custom roles.\n\n" +
			"System roles are immutable. Create and update read strict JSON from --input;\n" +
			"update is a full replacement. Deleting a custom role can optionally reassign\n" +
			"its users first and always requires confirmation.",
		Example: "  n8n role list --with-usage-count\n" +
			"  n8n role get global:member\n" +
			"  n8n role create --input role.json\n" +
			"  n8n role update global:custom --input role-update.json\n" +
			"  n8n role delete global:custom --reassign-role global:member --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newRoleListCommand(opts),
		newRoleCreateCommand(opts),
		newRoleGetCommand(opts),
		newRoleUpdateCommand(opts),
		newRoleDeleteCommand(opts),
	)
	return cmd
}

type roleListFlags struct {
	instance       instanceFlags
	withUsageCount bool
	output         string
}

func newRoleListCommand(opts Options) *cobra.Command {
	var f roleListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List global and project roles",
		Long: "List every role visible to the selected credential, grouped into global and\n" +
			"project roles. Text and JSON output expose each role's licensed field so an\n" +
			"unlicensed role is not mistaken for an assignable one.\n\n" +
			"Pass --with-usage-count to include user and project assignment counts. Use a\n" +
			"returned slug with get, update, or delete. Requires role:list.",
		Example: "  n8n role list\n" +
			"  n8n role list --with-usage-count\n" +
			"  n8n role list --with-usage-count --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoleList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.withUsageCount, "with-usage-count", false, "include user and project assignment counts (default: counts are omitted by the API)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON has global and project role arrays)")
	return cmd
}

func runRoleList(ctx context.Context, opts Options, f roleListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	roles, err := client.ListRoles(ctx, f.withUsageCount)
	if err != nil {
		return apiError(err, resolution, roleResource)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, roles)
	}
	return writeRoleListText(opts, resolution, roles)
}

func writeRoleListText(opts Options, resolution config.Resolution, roles *n8n.Roles) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	total := len(roles.Global) + len(roles.Project)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Roles:\t%d (%d global, %d project)\n", total, len(roles.Global), len(roles.Project))
	if total > 0 {
		fmt.Fprintln(tw, "\nTYPE\tSLUG\tNAME\tLICENSED\tSYSTEM\tUSERS\tPROJECTS\tSCOPES")
		writeRoleRows(tw, roles.Global)
		writeRoleRows(tw, roles.Project)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if total == 0 {
		fmt.Fprintln(opts.Streams.Err, "No roles were returned. Check access with 'n8n discover --resource role'.")
	}
	return nil
}

func writeRoleRows(w io.Writer, roles []n8n.Role) {
	for _, role := range roles {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			role.RoleType, role.Slug, role.DisplayName, roleLicense(role.Licensed), yesNo(role.SystemRole),
			roleUsage(role.UsedByUsers), roleUsage(role.UsedByProjects), len(role.Scopes))
	}
}

type roleReadFlags struct {
	instance       instanceFlags
	withUsageCount bool
	output         string
}

func newRoleGetCommand(opts Options) *cobra.Command {
	var f roleReadFlags
	cmd := &cobra.Command{
		Use:   "get <role-slug>",
		Short: "Get one role",
		Long: "Get one global or project role by its exact slug. Output includes scopes, whether\n" +
			"the role is licensed, and whether it is an immutable system role. Pass\n" +
			"--with-usage-count to include user and project assignment counts.\n\n" +
			"Use 'n8n role list' to discover slugs. This call is read-only and requires\n" +
			"role:read.",
		Example: "  n8n role get global:member\n" +
			"  n8n role get project:admin --with-usage-count\n" +
			"  n8n role get global:member --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.withUsageCount, "with-usage-count", false, "include user and project assignment counts (default: counts are omitted by the API)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the complete role object)")
	return cmd
}

func runRoleGet(ctx context.Context, opts Options, slug string, f roleReadFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateRoleSlug(slug); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	role, err := client.GetRole(ctx, slug, f.withUsageCount)
	if err != nil {
		return apiError(err, resolution, roleResource)
	}
	return writeRoleResult(opts, resolution, "Role", role, f.output)
}

type roleInputFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func (f *roleInputFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "role JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the role returned by the API)")
}

func newRoleCreateCommand(opts Options) *cobra.Command {
	var f roleInputFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom role",
		Long: "Create one custom role from strict JSON supplied by --input. Required fields are\n" +
			"displayName (2-100 characters), roleType (global or project), and scopes (an\n" +
			"array that may be empty). description is optional and limited to 500 characters.\n\n" +
			"Unknown fields and trailing JSON documents are refused before transport. Global\n" +
			"roles require role:manage; project roles require role:manageProject.",
		Example: "  n8n role create --input role.json\n" +
			"  printf '%s\\n' '{\"displayName\":\"Auditor\",\"roleType\":\"global\",\"scopes\":[\"workflow:read\"]}' | n8n role create --input - --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoleCreate(cmd.Context(), opts, f)
		},
	}
	f.register(cmd)
	return cmd
}

func runRoleCreate(ctx context.Context, opts Options, f roleInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreateRoleRequest
	if err := readRoleDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	role, err := client.CreateRole(ctx, request)
	if err != nil {
		return roleAPIError(err, resolution, "create")
	}
	return writeRoleResult(opts, resolution, "Created", role, f.output)
}

func newRoleUpdateCommand(opts Options) *cobra.Command {
	var f roleInputFlags
	cmd := &cobra.Command{
		Use:   "update <role-slug>",
		Short: "Fully replace a custom role",
		Long: "Fully replace a custom role's writable fields from strict JSON supplied by\n" +
			"--input. This is a PUT: displayName, description, and scopes are all required,\n" +
			"even when unchanged. Set description to null to remove it and scopes to [] to\n" +
			"remove every scope. roleType and slug cannot be changed.\n\n" +
			"System roles are immutable. Global roles require role:manage; project roles\n" +
			"require role:manageProject. Run 'n8n role get SLUG --output json' before building\n" +
			"the replacement document.",
		Example: "  n8n role update global:custom --input role-update.json\n" +
			"  printf '%s\\n' '{\"displayName\":\"Auditor\",\"description\":null,\"scopes\":[]}' | n8n role update global:custom --input - --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runRoleUpdate(ctx context.Context, opts Options, slug string, f roleInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateRoleSlug(slug); err != nil {
		return err
	}
	var request n8n.UpdateRoleRequest
	if err := readRoleDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	role, err := client.UpdateRole(ctx, slug, request)
	if err != nil {
		return roleAPIError(err, resolution, "update")
	}
	return writeRoleResult(opts, resolution, "Updated", role, f.output)
}

type roleDeleteFlags struct {
	instance     instanceFlags
	reassignRole string
	yes          bool
	output       string
}

func newRoleDeleteCommand(opts Options) *cobra.Command {
	var f roleDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <role-slug>",
		Short: "Permanently delete a custom role",
		Long: "Permanently delete one custom role. System roles cannot be deleted. A role with\n" +
			"assigned users is refused unless --reassign-role names another compatible role;\n" +
			"n8n moves those users before deleting the role. Users themselves are not deleted.\n\n" +
			"Deletion cannot be undone. Interactive runs ask for confirmation; non-interactive\n" +
			"runs require --yes. Requires role:manage or role:manageProject as appropriate.",
		Example: "  n8n role delete global:custom\n" +
			"  n8n role delete global:custom --reassign-role global:member --yes\n" +
			"  n8n role delete project:custom --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.reassignRole, "reassign-role", "", "role slug receiving assigned users before deletion (default: do not reassign; deletion fails when users remain)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent role deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deleted role returned by the API)")
	return cmd
}

func runRoleDelete(ctx context.Context, opts Options, slug string, f roleDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateRoleSlug(slug); err != nil {
		return err
	}
	if f.reassignRole != "" {
		if err := validateRoleSlug(f.reassignRole); err != nil {
			return fmt.Errorf("invalid --reassign-role: %w", err)
		}
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	consequence := "Assigned users are not reassigned, so n8n will refuse deletion if any remain."
	if f.reassignRole != "" {
		consequence = fmt.Sprintf("Assigned users will first move to %q.", f.reassignRole)
	}
	question := fmt.Sprintf("Permanently delete custom role %q from %s? %s", slug, resolution.URL, consequence)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	role, err := client.DeleteRole(ctx, slug, f.reassignRole)
	if err != nil {
		return roleAPIError(err, resolution, "delete")
	}
	return writeRoleResult(opts, resolution, "Deleted", role, f.output)
}

func readRoleDocument(opts Options, path string, dst any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass a JSON file path or --input - for stdin")
	}
	var reader io.Reader = opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open role input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxRoleDocument+1))
	if err != nil {
		return fmt.Errorf("read role JSON: %w", err)
	}
	if len(raw) > maxRoleDocument {
		return fmt.Errorf("role JSON exceeds %d bytes", maxRoleDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode role JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode role JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode role JSON: %w", err)
	}
	return nil
}

func writeRoleResult(opts Options, resolution config.Resolution, action string, role *n8n.Role, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, role)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, role.Slug)
	fmt.Fprintf(tw, "Name:\t%s\n", role.DisplayName)
	fmt.Fprintf(tw, "Type:\t%s\n", role.RoleType)
	fmt.Fprintf(tw, "Licensed:\t%s\n", roleLicense(role.Licensed))
	fmt.Fprintf(tw, "System role:\t%s\n", yesNo(role.SystemRole))
	fmt.Fprintf(tw, "Description:\t%s\n", roleDescription(role.Description))
	fmt.Fprintf(tw, "Scopes:\t%d\n", len(role.Scopes))
	for _, scope := range role.Scopes {
		fmt.Fprintf(tw, "  -\t%s\n", scope)
	}
	if role.UsedByUsers != nil {
		fmt.Fprintf(tw, "Used by users:\t%s\n", roleUsage(role.UsedByUsers))
	}
	if role.UsedByProjects != nil {
		fmt.Fprintf(tw, "Used by projects:\t%s\n", roleUsage(role.UsedByProjects))
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if role.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", role.CreatedAt)
	}
	if role.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", role.UpdatedAt)
	}
	return tw.Flush()
}

func roleLicense(licensed *bool) string {
	if licensed == nil {
		return "unknown"
	}
	return yesNo(*licensed)
}

func roleUsage(count *float64) string {
	if count == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *count)
}

func roleDescription(description *string) string {
	if description == nil {
		return "(none)"
	}
	return *description
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func validateRoleSlug(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("role slug is required")
	}
	if strings.TrimSpace(slug) != slug {
		return fmt.Errorf("role slug must not start or end with whitespace")
	}
	return nil
}

func roleAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.StatusCodeOf(err) == 400 && (action == "update" || action == "delete") {
		return fmt.Errorf("%w; only custom roles can be %sd: built-in/system roles are immutable; run 'n8n role get SLUG' to inspect the role", err, action)
	}
	return apiError(err, resolution, roleResource)
}
