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
	// projectResource is the 'n8n discover' key for projects, which the API
	// spells in the plural.
	projectResource    = "projects"
	maxProjectDocument = 1 << 20
)

func newProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects and their members",
		Long: "Manage projects on an n8n instance. Start with 'list' to find project IDs,\n" +
			"then create, rename, or delete a project. Membership lives under the 'user'\n" +
			"subgroup: list members, add them with a project role, change a role, or remove\n" +
			"a member.\n\n" +
			"Projects are a licensed n8n feature: an instance without the entitlement answers\n" +
			"403 for every command here. Deleting a project also deletes the workflows,\n" +
			"credentials, and variables it owns, so it always requires confirmation.",
		Example: "  n8n project list\n" +
			"  n8n project create \"Billing automation\"\n" +
			"  n8n project update PROJECT_ID --name \"Billing\"\n" +
			"  n8n project user list PROJECT_ID\n" +
			"  n8n project user add PROJECT_ID --user USER_ID=project:viewer\n" +
			"  n8n project delete PROJECT_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newProjectListCommand(opts),
		newProjectCreateCommand(opts),
		newProjectUpdateCommand(opts),
		newProjectDeleteCommand(opts),
		newProjectUserCommand(opts),
	)
	return cmd
}

type projectListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newProjectListCommand(opts Options) *cobra.Command {
	var f projectListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Long: "List the projects visible to the selected credential, one cursor-paginated page\n" +
			"at a time. The page carries a next cursor; pass it to --cursor for the following\n" +
			"page, or use --all to follow every cursor, capped at 10,000 projects.\n\n" +
			"Use this to find the project ID that update, delete, and every 'project user'\n" +
			"command takes. Personal projects appear with type 'personal'. Requires\n" +
			"project:list and a licensed instance.",
		Example: "  n8n project list\n" +
			"  n8n project list --limit 50 --output json\n" +
			"  n8n project list --cursor NEXT_CURSOR\n" +
			"  n8n project list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProjectList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "projects per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 projects)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runProjectList(ctx context.Context, opts Options, f projectListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
	if err := validateProjectLimit(listOpts); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var page n8n.Page[n8n.Project]
	if f.all {
		page.Data, err = n8n.Collect(ctx, client.ListProjects, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListProjects(ctx, listOpts)
	}
	if err != nil {
		return projectAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeProjectListText(opts, resolution, page)
}

func writeProjectListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.Project]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Projects:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tTYPE")
		for _, project := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", emptyDash(project.ID), project.Name, emptyDash(project.Type))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No projects were returned. Create one with 'n8n project create NAME'.")
	}
	return nil
}

type projectWriteFlags struct {
	instance instanceFlags
	output   string
}

func (f *projectWriteFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the project object)")
}

func newProjectCreateCommand(opts Options) *cobra.Command {
	var f projectWriteFlags
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project",
		Long: "Create one project with the given name. Name is the only writable field; the ID\n" +
			"and type are assigned by the instance.\n\n" +
			"The API contract declares no response body for this call, so an instance that\n" +
			"returns none is reported as created without an ID; run 'n8n project list' to\n" +
			"recover it. Quote names containing spaces. Add members afterwards with\n" +
			"'n8n project user add'. Requires project:create and a licensed instance.",
		Example: "  n8n project create \"Billing automation\"\n" +
			"  n8n project create Staging --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectCreate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runProjectCreate(ctx context.Context, opts Options, name string, f projectWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateProjectName(name); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	project, err := client.CreateProject(ctx, name)
	if err != nil {
		return projectAPIError(err, resolution, "create")
	}
	if project.Name == "" {
		project.Name = name
	}
	return writeProjectResult(opts, resolution, "Created", project, f.output)
}

type projectUpdateFlags struct {
	write projectWriteFlags
	name  string
}

func newProjectUpdateCommand(opts Options) *cobra.Command {
	var f projectUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <project-id>",
		Short: "Rename a project",
		Long: "Rename one project. The API replaces the whole project, but name is its only\n" +
			"writable field, so --name is the full replacement document and nothing else is\n" +
			"lost: the ID, type, members, and owned resources all stay as they are.\n\n" +
			"No read-modify-write step is needed. Find the ID with 'n8n project list'.\n" +
			"Requires project:update and a licensed instance.",
		Example: "  n8n project update PROJECT_ID --name \"Billing\"\n" +
			"  n8n project update PROJECT_ID --name \"Customer facing\" --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.write.register(cmd)
	cmd.Flags().StringVar(&f.name, "name", "", "new project name, replacing the current one (required)")
	return cmd
}

func runProjectUpdate(ctx context.Context, opts Options, id string, f projectUpdateFlags) error {
	if err := validateOutput(f.write.output); err != nil {
		return err
	}
	if err := validateProjectID(id); err != nil {
		return err
	}
	if strings.TrimSpace(f.name) == "" {
		return fmt.Errorf("--name is required: it is the new name the project is renamed to")
	}
	if err := validateProjectName(f.name); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.write.instance)
	if err != nil {
		return err
	}
	if err := client.UpdateProject(ctx, id, f.name); err != nil {
		return projectAPIError(err, resolution, "update")
	}
	return writeProjectResult(opts, resolution, "Updated", &n8n.Project{ID: id, Name: f.name}, f.write.output)
}

type projectDeleteFlags struct {
	write projectWriteFlags
	yes   bool
}

func newProjectDeleteCommand(opts Options) *cobra.Command {
	var f projectDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <project-id>",
		Short: "Permanently delete a project",
		Long: "Permanently delete one project together with the workflows, credentials, and\n" +
			"variables it owns. Members lose access; their user accounts are not deleted.\n" +
			"The endpoint takes no transfer target, so move anything worth keeping first,\n" +
			"for example with 'n8n credential transfer'. This cannot be undone.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires project:delete and a licensed instance.",
		Example: "  n8n project delete PROJECT_ID\n" +
			"  n8n project delete PROJECT_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.write.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent project and owned-resource deletion without prompting (required when stdin is not interactive)")
	return cmd
}

func runProjectDelete(ctx context.Context, opts Options, id string, f projectDeleteFlags) error {
	if err := validateOutput(f.write.output); err != nil {
		return err
	}
	if err := validateProjectID(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.write.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete project %q from %s? The workflows, credentials and variables it owns are deleted with it.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteProject(ctx, id); err != nil {
		return projectAPIError(err, resolution, "delete")
	}
	return writeProjectMutation(opts, resolution, projectMutation{Action: "deleted", ProjectID: id}, f.write.output)
}

func newProjectUserCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage project members",
		Long: "Manage which users belong to a project and with which project role. Start with\n" +
			"'list' to see current members and their roles, then add users, change a role, or\n" +
			"remove a member.\n\n" +
			"User IDs come from 'n8n user list'; project role slugs come from 'n8n role list'\n" +
			"(project roles such as project:viewer, project:editor, project:admin). Listing\n" +
			"members additionally requires the user:list scope; adding, changing and removing\n" +
			"require project:manageMembers.",
		Example: "  n8n project user list PROJECT_ID\n" +
			"  n8n project user add PROJECT_ID --user USER_ID=project:viewer\n" +
			"  n8n project user role set PROJECT_ID USER_ID project:editor\n" +
			"  n8n project user remove PROJECT_ID USER_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newProjectUserListCommand(opts),
		newProjectUserAddCommand(opts),
		newProjectUserRoleCommand(opts),
		newProjectUserRemoveCommand(opts),
	)
	return cmd
}

func newProjectUserListCommand(opts Options) *cobra.Command {
	var f projectListFlags
	cmd := &cobra.Command{
		Use:   "list <project-id>",
		Short: "List the members of a project",
		Long: "List every member of one project with the project role each of them holds. The\n" +
			"response is cursor-paginated; pass a returned cursor to --cursor, or use --all to\n" +
			"follow every cursor, capped at 10,000 members.\n\n" +
			"This endpoint needs the user:list scope on top of project access, so a credential\n" +
			"that can read the project may still be refused here. Use the returned user IDs\n" +
			"with 'n8n project user role set' and 'n8n project user remove'.",
		Example: "  n8n project user list PROJECT_ID\n" +
			"  n8n project user list PROJECT_ID --limit 50 --output json\n" +
			"  n8n project user list PROJECT_ID --all",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectUserList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "members per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 members)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runProjectUserList(ctx context.Context, opts Options, projectID string, f projectListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	listOpts := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
	if err := validateProjectLimit(listOpts); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.ProjectMember], error) {
		return client.ListProjectUsers(ctx, projectID, pagination)
	}
	var page n8n.Page[n8n.ProjectMember]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts)
	}
	if err != nil {
		return projectAPIError(err, resolution, "member list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeProjectMemberListText(opts, resolution, projectID, page)
}

func writeProjectMemberListText(opts Options, resolution config.Resolution, projectID string, page n8n.Page[n8n.ProjectMember]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Project:\t%s\n", projectID)
	fmt.Fprintf(tw, "Members:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nUSER ID\tEMAIL\tNAME\tPROJECT ROLE")
		for _, member := range page.Data {
			name := strings.TrimSpace(member.FirstName + " " + member.LastName)
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", member.ID, member.Email, emptyDash(name), emptyDash(member.Role))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No members were returned. Add one with 'n8n project user add PROJECT_ID --user USER_ID=project:viewer'.")
	}
	return nil
}

type projectUserAddFlags struct {
	instance instanceFlags
	users    []string
	input    string
	output   string
}

func newProjectUserAddCommand(opts Options) *cobra.Command {
	var f projectUserAddFlags
	cmd := &cobra.Command{
		Use:   "add <project-id>",
		Short: "Add one or more users to a project",
		Long: "Add users to one project, each with a project role. Pass --user once per member\n" +
			"as USER_ID=ROLE, or supply a strict JSON array of {\"userId\",\"role\"} objects with\n" +
			"--input FILE or --input -. The two ways are mutually exclusive and one of them is\n" +
			"required.\n\n" +
			"User IDs come from 'n8n user list'; project role slugs come from 'n8n role list'.\n" +
			"Every member is sent in one request, so the API accepts or rejects the batch as a\n" +
			"whole, and a user already in the project is rejected rather than updated; change\n" +
			"an existing member with 'n8n project user role set'. Requires\n" +
			"project:manageMembers.",
		Example: "  n8n project user add PROJECT_ID --user USER_ID=project:viewer\n" +
			"  n8n project user add PROJECT_ID --user USER_ONE=project:editor --user USER_TWO=project:viewer\n" +
			"  n8n project user add PROJECT_ID --input members.json\n" +
			"  printf '%s\\n' '[{\"userId\":\"USER_ID\",\"role\":\"project:viewer\"}]' | n8n project user add PROJECT_ID --input - --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectUserAdd(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.users, "user", nil, "member to add as USER_ID=ROLE, repeatable (cannot combine with --input)")
	cmd.Flags().StringVar(&f.input, "input", "", "member JSON array file path, or - for stdin (cannot combine with --user; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement lists the members sent)")
	return cmd
}

func runProjectUserAdd(ctx context.Context, opts Options, projectID string, f projectUserAddFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	relations, err := projectRelations(opts, f)
	if err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := client.AddUsersToProject(ctx, projectID, relations); err != nil {
		return projectAPIError(err, resolution, "member add")
	}
	return writeProjectMutation(opts, resolution, projectMutation{
		Action:    "members added",
		ProjectID: projectID,
		Members:   relations,
	}, f.output)
}

// projectRelations builds the member list from --user pairs or --input JSON.
func projectRelations(opts Options, f projectUserAddFlags) ([]n8n.ProjectRelation, error) {
	hasUsers, hasInput := len(f.users) > 0, strings.TrimSpace(f.input) != ""
	switch {
	case hasUsers && hasInput:
		return nil, fmt.Errorf("--user and --input cannot be used together: pass the members one way")
	case !hasUsers && !hasInput:
		return nil, fmt.Errorf("--user or --input is required: pass --user USER_ID=ROLE, or a JSON array with --input")
	case hasInput:
		var relations []n8n.ProjectRelation
		if err := readProjectDocument(opts, f.input, &relations); err != nil {
			return nil, err
		}
		return relations, nil
	}
	relations := make([]n8n.ProjectRelation, 0, len(f.users))
	for _, pair := range f.users {
		userID, role, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("--user %q is not USER_ID=ROLE: pass the user ID, an equals sign, then the project role slug", pair)
		}
		relations = append(relations, n8n.ProjectRelation{UserID: userID, Role: role})
	}
	return relations, nil
}

func readProjectDocument(opts Options, path string, dst any) error {
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open project input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxProjectDocument+1))
	if err != nil {
		return fmt.Errorf("read project JSON: %w", err)
	}
	if len(raw) > maxProjectDocument {
		return fmt.Errorf("project JSON exceeds %d bytes", maxProjectDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode project JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode project JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode project JSON: %w", err)
	}
	return nil
}

func newProjectUserRoleCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage members' project roles",
		Long: "Manage the project role of an existing project member. Use 'set' with the\n" +
			"project ID, the user ID, and a project role slug from 'n8n role list'. The role\n" +
			"applies inside this project only; it does not change the user's global role,\n" +
			"which 'n8n user role set' handles.",
		Example: "  n8n project user role set PROJECT_ID USER_ID project:editor\n" +
			"  n8n project user role set PROJECT_ID USER_ID project:admin --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newProjectUserRoleSetCommand(opts))
	return cmd
}

type projectMutationFlags struct {
	instance instanceFlags
	output   string
}

func newProjectUserRoleSetCommand(opts Options) *cobra.Command {
	var f projectMutationFlags
	cmd := &cobra.Command{
		Use:   "set <project-id> <user-id> <role-slug>",
		Short: "Set a member's project role",
		Long: "Change one member's role inside one project. The user must already be a member;\n" +
			"add them first with 'n8n project user add'. Run 'n8n project user list' for the\n" +
			"current members and 'n8n role list' for assignable project role slugs.\n\n" +
			"This takes effect immediately and changes what the member may do in the project,\n" +
			"but not their global role. Requires project:manageMembers.",
		Example: "  n8n project user role set PROJECT_ID USER_ID project:editor\n" +
			"  n8n project user role set PROJECT_ID USER_ID project:admin --output json",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectUserRoleSet(cmd.Context(), opts, args[0], args[1], args[2], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains project, user and new role)")
	return cmd
}

func runProjectUserRoleSet(ctx context.Context, opts Options, projectID, userID, role string, f projectMutationFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	if err := validateProjectUserID(userID); err != nil {
		return err
	}
	if err := validateProjectRole(role); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := client.ChangeUserRoleInProject(ctx, projectID, userID, role); err != nil {
		return projectAPIError(err, resolution, "member role change")
	}
	return writeProjectMutation(opts, resolution, projectMutation{
		Action:    "member role set",
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
	}, f.output)
}

type projectUserRemoveFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newProjectUserRemoveCommand(opts Options) *cobra.Command {
	var f projectUserRemoveFlags
	cmd := &cobra.Command{
		Use:   "remove <project-id> <user-id>",
		Short: "Remove a member from a project",
		Long: "Remove one member from one project. The user account, its personal project, and\n" +
			"everything the project itself owns are untouched; only this membership and the\n" +
			"access it granted go away. Re-add the member later with 'n8n project user add'.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires project:manageMembers.",
		Example: "  n8n project user remove PROJECT_ID USER_ID\n" +
			"  n8n project user remove PROJECT_ID USER_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectUserRemove(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm member removal without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains project and removed user)")
	return cmd
}

func runProjectUserRemove(ctx context.Context, opts Options, projectID, userID string, f projectUserRemoveFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	if err := validateProjectUserID(userID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Remove user %q from project %q on %s? They lose access to everything the project owns; the account itself stays.", userID, projectID, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteUserFromProject(ctx, projectID, userID); err != nil {
		return projectAPIError(err, resolution, "member removal")
	}
	return writeProjectMutation(opts, resolution, projectMutation{
		Action:    "member removed",
		ProjectID: projectID,
		UserID:    userID,
	}, f.output)
}

func writeProjectResult(opts Options, resolution config.Resolution, action string, project *n8n.Project, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, project)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, project.Name)
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(project.ID))
	if project.Type != "" {
		fmt.Fprintf(tw, "Type:\t%s\n", project.Type)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if err := tw.Flush(); err != nil {
		return err
	}
	if project.ID == "" {
		fmt.Fprintln(opts.Streams.Err, "The instance returned no project body. Run 'n8n project list' to read the assigned ID.")
	}
	return nil
}

type projectMutation struct {
	Action    string                `json:"action"`
	ProjectID string                `json:"projectId"`
	UserID    string                `json:"userId,omitempty"`
	Role      string                `json:"role,omitempty"`
	Members   []n8n.ProjectRelation `json:"members,omitempty"`
}

func writeProjectMutation(opts Options, resolution config.Resolution, result projectMutation, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	fmt.Fprintf(tw, "Project:\t%s\n", result.ProjectID)
	if result.UserID != "" {
		fmt.Fprintf(tw, "User:\t%s\n", result.UserID)
	}
	if result.Role != "" {
		fmt.Fprintf(tw, "Role:\t%s\n", result.Role)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(result.Members) > 0 {
		fmt.Fprintln(tw, "\nUSER ID\tPROJECT ROLE")
		for _, member := range result.Members {
			fmt.Fprintf(tw, "%s\t%s\n", member.UserID, member.Role)
		}
	}
	return tw.Flush()
}

func validateProjectLimit(opts n8n.ListOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", opts.Limit)
	}
	return nil
}

func validateProjectID(id string) error { return validateProjectArgument("project ID", id) }

func validateProjectName(name string) error { return validateProjectArgument("project name", name) }

func validateProjectUserID(id string) error { return validateProjectArgument("user ID", id) }

func validateProjectRole(role string) error {
	return validateProjectArgument("project role slug", role)
}

func validateProjectArgument(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	return nil
}

// projectAPIError explains the denials this resource produces. Projects are a
// licensed feature, and member listing needs a scope the rest of the group does
// not, so a bare 403 is ambiguous without saying which of the two it is.
func projectAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.IsForbidden(err) {
		if action == "member list" {
			return forbiddenScopeError(err, resolution, "project "+action, allOf("user:list"),
				"this endpoint needs project access on top of the scope, and projects require a licensed instance.", projectResource)
		}
		need := projectScope(action)
		return forbiddenScopeError(err, resolution, "project "+action, need,
			"a 403 can also mean the instance is not licensed for projects.", projectResource)
	}
	return apiError(err, resolution, projectResource)
}

// projectScope maps a command action to the scope the API requires for it.
// Member listing is handled separately: it needs user:list, not a project scope.
func projectScope(action string) scopeNeed {
	switch action {
	case "list":
		return allOf("project:list")
	case "create":
		return allOf("project:create")
	case "update":
		return allOf("project:update")
	case "delete":
		return allOf("project:delete")
	case "member add", "member removal", "member role change":
		return allOf("project:manageMembers")
	}
	return scopeNeed{}
}
