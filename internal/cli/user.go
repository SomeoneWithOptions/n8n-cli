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
	userResource    = "user"
	maxUserDocument = 1 << 20
)

func newUserCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage instance users and global roles",
		Long: "Manage users on an n8n instance. Start with 'list' to find user IDs and\n" +
			"emails, use 'get' for one account, then invite users, change a global role,\n" +
			"or permanently delete an account. List and get may require instance-owner\n" +
			"access.\n\n" +
			"Create accepts a strict JSON array and preserves every per-user success or\n" +
			"error. Role changes and deletion accept the exact user ID or email allowed by\n" +
			"the API. Deletion always requires confirmation.",
		Example: "  n8n user list --include-role\n" +
			"  n8n user get person@example.com --include-role\n" +
			"  n8n user create --input users.json\n" +
			"  n8n user role set USER_ID global:member\n" +
			"  n8n user delete USER_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newUserListCommand(opts),
		newUserCreateCommand(opts),
		newUserGetCommand(opts),
		newUserRoleCommand(opts),
		newUserDeleteCommand(opts),
	)
	return cmd
}

type userListFlags struct {
	instance    instanceFlags
	limit       int
	offset      int
	cursor      string
	all         bool
	includeRole bool
	projectID   string
	output      string
}

func newUserListCommand(opts Options) *cobra.Command {
	var f userListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List instance or project users",
		Long: "List users visible to the selected credential. Use --project-id to filter to\n" +
			"one project's members and --include-role to request global roles. The API is\n" +
			"cursor-paginated; --offset selects a starting position, while --cursor resumes\n" +
			"a previous response. They cannot be combined.\n\n" +
			"--all follows cursors and collects at most 10,000 users. Instance-wide listing\n" +
			"may require the instance owner. Requires user:list.",
		Example: "  n8n user list --include-role\n" +
			"  n8n user list --project-id PROJECT_ID --limit 50\n" +
			"  n8n user list --offset 100 --output json\n" +
			"  n8n user list --all --include-role --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUserList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "users per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().IntVar(&f.offset, "offset", 0, "users to skip before first page, zero or greater (default: 0; cannot combine with --cursor)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: first page; cannot combine with --offset)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 users)")
	cmd.Flags().BoolVar(&f.includeRole, "include-role", false, "include each user's global role (default: role is omitted by API)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "return members of this project ID (default: all visible instance users)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a paginated user object)")
	return cmd
}

func runUserList(ctx context.Context, opts Options, f userListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListUsersOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Offset:      f.offset,
		IncludeRole: f.includeRole,
		ProjectID:   f.projectID,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var page n8n.Page[n8n.User]
	if f.all {
		fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.User], error) {
			pageOpts := listOpts
			pageOpts.ListOptions = pagination
			if pagination.Cursor != "" {
				pageOpts.Offset = 0
			}
			return client.ListUsers(ctx, pageOpts)
		}
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListUsers(ctx, listOpts)
	}
	if err != nil {
		return userAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeUserListText(opts, resolution, page)
}

func writeUserListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.User]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Users:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tEMAIL\tNAME\tSTATUS\tROLE\tMFA")
		for _, user := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", user.ID, user.Email,
				userName(user), userStatus(user), emptyDash(user.Role), yesNo(user.MFAEnabled))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No users were returned. Check access with 'n8n discover --resource user'.")
	}
	return nil
}

type userReadFlags struct {
	instance    instanceFlags
	includeRole bool
	output      string
}

func newUserGetCommand(opts Options) *cobra.Command {
	var f userReadFlags
	cmd := &cobra.Command{
		Use:   "get <user-id-or-email>",
		Short: "Get one user by ID or email",
		Long: "Get one user using the exact ID or email accepted by the n8n API. Pass\n" +
			"--include-role to request the user's global role; otherwise the API omits it.\n\n" +
			"Use 'n8n user list' to discover identifiers. This operation is read-only, may\n" +
			"require instance-owner access, and requires user:read.",
		Example: "  n8n user get USER_ID\n" +
			"  n8n user get person@example.com --include-role\n" +
			"  n8n user get USER_ID --include-role --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.includeRole, "include-role", false, "include user's global role (default: role is omitted by API)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is complete returned user object)")
	return cmd
}

func runUserGet(ctx context.Context, opts Options, identifier string, f userReadFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateUserIdentifier(identifier); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	user, err := client.GetUser(ctx, identifier, f.includeRole)
	if err != nil {
		return userAPIError(err, resolution, "get")
	}
	return writeUserResult(opts, resolution, "User", user, f.output)
}

type userInputFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func newUserCreateCommand(opts Options) *cobra.Command {
	var f userInputFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Invite one or more users",
		Long: "Create or invite users from a strict JSON array supplied by --input. Each item\n" +
			"requires email and may set a global role slug. Unknown fields, an empty array,\n" +
			"invalid emails, and trailing JSON documents are refused before transport.\n\n" +
			"n8n returns one result per invitation. Output preserves successful users, invite\n" +
			"URLs, email-delivery state, and individual errors even when a bulk request is\n" +
			"only partly successful. Invite URLs are sensitive. Requires user:create.",
		Example: "  n8n user create --input users.json\n" +
			"  printf '%s\\n' '[{\"email\":\"person@example.com\",\"role\":\"global:member\"}]' | n8n user create --input - --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUserCreate(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "user invitation JSON array file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (both preserve every per-user result)")
	return cmd
}

func runUserCreate(ctx context.Context, opts Options, f userInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var requests []n8n.CreateUserRequest
	if err := readUserDocument(opts, f.input, &requests); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	results, err := client.CreateUsers(ctx, requests)
	if err != nil {
		return userAPIError(err, resolution, "create")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, results)
	}
	return writeCreateUserResults(opts, resolution, results)
}

func newUserRoleCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage users' global roles",
		Long: "Manage global role assignments for instance users. Use 'set' with a user ID\n" +
			"or email and a role slug discovered through 'n8n role list'. Role changes can\n" +
			"alter instance-wide permissions and may require owner access or licensing.",
		Example: "  n8n user role set USER_ID global:member\n" +
			"  n8n user role set person@example.com global:admin --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newUserRoleSetCommand(opts))
	return cmd
}

type userMutationFlags struct {
	instance instanceFlags
	output   string
}

func newUserRoleSetCommand(opts Options) *cobra.Command {
	var f userMutationFlags
	cmd := &cobra.Command{
		Use:   "set <user-id-or-email> <role-slug>",
		Short: "Set a user's global role",
		Long: "Change one user's global role using the exact user ID or email accepted by the\n" +
			"API. The role slug must identify an assignable global role; run 'n8n role list'\n" +
			"to inspect licensing and available slugs. This can immediately change the user's\n" +
			"instance permissions. Requires user:changeRole.",
		Example: "  n8n user role set USER_ID global:member\n" +
			"  n8n user role set person@example.com global:admin --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserRoleSet(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains user identifier and new role)")
	return cmd
}

func runUserRoleSet(ctx context.Context, opts Options, identifier, role string, f userMutationFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateUserIdentifier(identifier); err != nil {
		return err
	}
	if err := validateUserRole(role); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := client.ChangeUserRole(ctx, identifier, role); err != nil {
		return userAPIError(err, resolution, "change role")
	}
	return writeUserMutation(opts, resolution, userMutation{Action: "role set", Identifier: identifier, Role: role}, f.output)
}

type userDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newUserDeleteCommand(opts Options) *cobra.Command {
	var f userDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <user-id-or-email>",
		Short: "Permanently delete a user",
		Long: "Permanently delete one user using the exact ID or email accepted by the API.\n" +
			"This removes the account and can delete resources owned by its personal project;\n" +
			"the documented endpoint provides no ownership-transfer option. It cannot be\n" +
			"undone and the instance owner cannot be deleted.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires user:delete.",
		Example: "  n8n user delete USER_ID\n" +
			"  n8n user delete person@example.com --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent user and owned-resource deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains only deleted user identifier)")
	return cmd
}

func runUserDelete(ctx context.Context, opts Options, identifier string, f userDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateUserIdentifier(identifier); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete user %q from %s? Their account and owned personal-project resources may be deleted.", identifier, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteUser(ctx, identifier); err != nil {
		return userAPIError(err, resolution, "delete")
	}
	return writeUserMutation(opts, resolution, userMutation{Action: "deleted", Identifier: identifier}, f.output)
}

func readUserDocument(opts Options, path string, dst any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass a JSON file path or --input - for stdin")
	}
	reader := opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open user input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxUserDocument+1))
	if err != nil {
		return fmt.Errorf("read user JSON: %w", err)
	}
	if len(raw) > maxUserDocument {
		return fmt.Errorf("user JSON exceeds %d bytes", maxUserDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode user JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode user JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode user JSON: %w", err)
	}
	return nil
}

func writeUserResult(opts Options, resolution config.Resolution, action string, user *n8n.User, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, user)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, user.ID)
	fmt.Fprintf(tw, "Email:\t%s\n", user.Email)
	fmt.Fprintf(tw, "Name:\t%s\n", userName(*user))
	fmt.Fprintf(tw, "Status:\t%s\n", userStatus(*user))
	fmt.Fprintf(tw, "Role:\t%s\n", emptyDash(user.Role))
	fmt.Fprintf(tw, "MFA enabled:\t%s\n", yesNo(user.MFAEnabled))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if user.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", user.CreatedAt)
	}
	if user.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", user.UpdatedAt)
	}
	return tw.Flush()
}

func writeCreateUserResults(opts Options, resolution config.Resolution, results []n8n.CreateUserResult) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Results:\t%d\n", len(results))
	if len(results) > 0 {
		fmt.Fprintln(tw, "\n#\tEMAIL\tID\tSTATUS\tEMAIL SENT\tROLE\tINVITE URL / ERROR")
	}
	for i, result := range results {
		email, id, sent, role, detail, status := "-", "-", "-", "-", "-", "created"
		if result.User != nil {
			email, id, sent, role = result.User.Email, result.User.ID, yesNo(result.User.EmailSent), emptyDash(result.User.Role)
			if result.User.InviteAcceptURL != "" {
				detail = result.User.InviteAcceptURL
			}
		}
		if result.Error != "" {
			status = "error"
			detail = result.Error
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", i+1, email, id, status, sent, role, detail)
	}
	return tw.Flush()
}

type userMutation struct {
	Action     string `json:"action"`
	Identifier string `json:"identifier"`
	Role       string `json:"role,omitempty"`
}

func writeUserMutation(opts Options, resolution config.Resolution, result userMutation, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	fmt.Fprintf(tw, "User:\t%s\n", result.Identifier)
	if result.Role != "" {
		fmt.Fprintf(tw, "Role:\t%s\n", result.Role)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func userName(user n8n.User) string {
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	return emptyDash(name)
}

func userStatus(user n8n.User) string {
	if user.IsPending {
		return "pending"
	}
	return "active"
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func validateUserIdentifier(identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		return fmt.Errorf("user ID or email is required")
	}
	if strings.TrimSpace(identifier) != identifier {
		return fmt.Errorf("user ID or email must not start or end with whitespace")
	}
	return nil
}

func validateUserRole(role string) error {
	if strings.TrimSpace(role) == "" {
		return fmt.Errorf("user role slug is required")
	}
	if strings.TrimSpace(role) != role {
		return fmt.Errorf("user role slug must not start or end with whitespace")
	}
	return nil
}

func userAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.IsForbidden(err) {
		need := userScope(action)
		return forbiddenScopeError(err, resolution, "user "+action, need,
			"this endpoint can also require the instance owner or admin role.", userResource)
	}
	return apiError(err, resolution, userResource)
}

// userScope maps a command action to the scope the API requires for it.
func userScope(action string) scopeNeed {
	switch action {
	case "list":
		return allOf("user:list")
	case "get":
		return allOf("user:read")
	case "create":
		return allOf("user:create")
	case "delete":
		return allOf("user:delete")
	case "change role":
		return allOf("user:changeRole")
	}
	return scopeNeed{}
}
