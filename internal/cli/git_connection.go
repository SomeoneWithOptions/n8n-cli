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
	// gitConnectionResource is the 'n8n discover' key for this group, spelled
	// exactly as the instance reports the lowercase tag.
	gitConnectionResource    = "gitconnections"
	maxGitConnectionDocument = 1 << 20
)

func newGitConnectionCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git-connection",
		Short: "Manage the Git connection, its projects, and push/pull sync",
		Long: "Manage the Git connection this instance syncs team projects through.\n" +
			"Start with 'list' to find the connection ID (an instance holds at most\n" +
			"one), then 'get' to read it, 'create' to add it, or 'update' to change\n" +
			"it. Clone the repository with 'clone', link team projects under\n" +
			"'project', then 'push' local projects to the remote or 'pull' the\n" +
			"remote into the instance.\n\n" +
			"'push' commits and pushes every team project to the remote branch;\n" +
			"'pull' resets the local clone to the branch tip and imports projects\n" +
			"into the instance, overwriting to match. Both mutate versioned\n" +
			"content and ask for confirmation. 'disconnect' removes the local\n" +
			"checkout while keeping the connection; 'delete' removes the\n" +
			"connection and its checkout. This is the target-only GitConnections\n" +
			"generation: an instance that serves Promotions instead answers 404\n" +
			"or 503, which reads as the availability diagnostic.",
		Example: "  n8n git-connection list\n" +
			"  n8n git-connection get CONNECTION_ID\n" +
			"  n8n git-connection create --input connection.json\n" +
			"  n8n git-connection clone CONNECTION_ID --branch main\n" +
			"  n8n git-connection project add CONNECTION_ID PROJECT_ID\n" +
			"  n8n git-connection push CONNECTION_ID --commit-message \"sync\" --yes\n" +
			"  n8n git-connection pull CONNECTION_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newGitConnectionListCommand(opts),
		newGitConnectionCreateCommand(opts),
		newGitConnectionGetCommand(opts),
		newGitConnectionUpdateCommand(opts),
		newGitConnectionDeleteCommand(opts),
		newGitConnectionCloneCommand(opts),
		newGitConnectionDisconnectCommand(opts),
		newGitConnectionProjectCommand(opts),
		newGitConnectionPushCommand(opts),
		newGitConnectionPullCommand(opts),
	)
	return cmd
}

type gitConnectionListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newGitConnectionListCommand(opts Options) *cobra.Command {
	var f gitConnectionListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Git connections",
		Long: "List the Git connections visible to the selected credential, one\n" +
			"cursor-paginated page at a time. An instance holds at most one\n" +
			"connection, so this usually carries zero or one row. Use --all to\n" +
			"follow every cursor, capped at 10,000 connections.\n\n" +
			"Use a returned ID with get, update, delete, clone, disconnect, project,\n" +
			"push, and pull. Requires gitConnection:list.",
		Example: "  n8n git-connection list\n" +
			"  n8n git-connection list --limit 50 --output json\n" +
			"  n8n git-connection list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGitConnectionList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "connections per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 connections)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runGitConnectionList(ctx context.Context, opts Options, f gitConnectionListFlags) error {
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
	var page n8n.Page[n8n.GitConnection]
	if f.all {
		page.Data, err = n8n.Collect(ctx, client.ListGitConnections, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListGitConnections(ctx, listOpts)
	}
	if err != nil {
		return gitConnectionAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeGitConnectionListText(opts, resolution, page)
}

func writeGitConnectionListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.GitConnection]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Connections:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tREPOSITORY\tBRANCH\tTYPE\tBASE COMMIT")
		for _, connection := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				emptyDash(connection.ID), emptyDash(connection.Name),
				emptyDash(connection.RepositoryURL),
				emptyDash(derefGitConnection(connection.BranchName)),
				emptyDash(connection.ConnectionType),
				emptyDash(derefGitConnection(connection.BaseCommit)))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No Git connections were returned. Create one with 'n8n git-connection create --input connection.json'.")
	}
	return nil
}

type gitConnectionInputFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func (f *gitConnectionInputFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "Git connection JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the connection returned by the API)")
}

func newGitConnectionCreateCommand(opts Options) *cobra.Command {
	var f gitConnectionInputFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create the Git connection",
		Long: "Create the instance's Git connection from strict JSON supplied by\n" +
			"--input. Required fields are name (at most 128 characters),\n" +
			"repositoryUrl, and connectionType (ssh or https). Optional fields are\n" +
			"branchName (at most 255 characters), keyGeneratorType (ed25519 or rsa),\n" +
			"username, and password.\n\n" +
			"Credentials travel only as --input because secrets are never flags:\n" +
			"process lists and shell history can expose argv. Unknown fields and\n" +
			"trailing JSON documents are refused before transport. Only one\n" +
			"connection can exist; a second answers 409. Requires\n" +
			"gitConnection:create. Follow with 'n8n git-connection clone'.",
		Example: "  n8n git-connection create --input connection.json\n" +
			"  printf '%s\\n' '{\"name\":\"prod\",\"repositoryUrl\":\"git@example.com:org/repo.git\",\"connectionType\":\"ssh\"}' | n8n git-connection create --input -\n" +
			"  n8n git-connection create --input connection.json --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGitConnectionCreate(cmd.Context(), opts, f)
		},
	}
	f.register(cmd)
	return cmd
}

func runGitConnectionCreate(ctx context.Context, opts Options, f gitConnectionInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreateGitConnectionRequest
	if err := readGitConnectionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	created, err := client.CreateGitConnection(ctx, request)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "create")
	}
	return writeGitConnectionResult(opts, resolution, "Created", created, f.output)
}

type gitConnectionGetFlags struct {
	instance instanceFlags
	output   string
}

func newGitConnectionGetCommand(opts Options) *cobra.Command {
	var f gitConnectionGetFlags
	cmd := &cobra.Command{
		Use:   "get <connection-id>",
		Short: "Show one Git connection",
		Long: "Show one Git connection by ID. The response is the public connection\n" +
			"document: name, repository URL, branch, connection type, the SSH public\n" +
			"key to deploy, the key generator, the base commit, and timestamps.\n" +
			"Secrets are never returned. Find the ID with 'n8n git-connection\n" +
			"list'. Requires gitConnection:read.",
		Example: "  n8n git-connection get CONNECTION_ID\n" +
			"  n8n git-connection get CONNECTION_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the connection object)")
	return cmd
}

func runGitConnectionGet(ctx context.Context, opts Options, id string, f gitConnectionGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	connection, err := client.GetGitConnection(ctx, id)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "get")
	}
	return writeGitConnectionResult(opts, resolution, "Connection", connection, f.output)
}

func newGitConnectionUpdateCommand(opts Options) *cobra.Command {
	var f gitConnectionInputFlags
	cmd := &cobra.Command{
		Use:   "update <connection-id>",
		Short: "Update a Git connection",
		Long: "Update one Git connection from strict JSON supplied by --input. Only\n" +
			"the fields present are changed; at least one of name, repositoryUrl,\n" +
			"branchName, connectionType, keyGeneratorType, username, or password is\n" +
			"required.\n\n" +
			"Run 'n8n git-connection get --output json' first and edit the writable\n" +
			"fields: the response carries read-only id, publicKey, baseCommit, and\n" +
			"timestamps that the update document must not repeat. Credentials travel\n" +
			"only as --input because secrets are never flags. Requires\n" +
			"gitConnection:update.",
		Example: "  n8n git-connection update CONNECTION_ID --input connection-update.json\n" +
			"  printf '%s\\n' '{\"branchName\":\"main\"}' | n8n git-connection update CONNECTION_ID --input -\n" +
			"  n8n git-connection update CONNECTION_ID --input connection-update.json --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runGitConnectionUpdate(ctx context.Context, opts Options, id string, f gitConnectionInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	var request n8n.UpdateGitConnectionRequest
	if err := readGitConnectionDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	updated, err := client.UpdateGitConnection(ctx, id, request)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "update")
	}
	return writeGitConnectionResult(opts, resolution, "Updated", updated, f.output)
}

type gitConnectionDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newGitConnectionDeleteCommand(opts Options) *cobra.Command {
	var f gitConnectionDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <connection-id>",
		Short: "Permanently delete a Git connection",
		Long: "Permanently delete one Git connection together with its local\n" +
			"checkout. Linked projects stay on the instance but lose their remote;\n" +
			"push and pull stop working until a connection is created and cloned\n" +
			"again. This cannot be undone.\n\n" +
			"To keep the connection and drop only the checkout, use\n" +
			"'n8n git-connection disconnect' instead. Interactive runs ask for\n" +
			"confirmation; non-interactive runs require --yes. Requires\n" +
			"gitConnection:delete.",
		Example: "  n8n git-connection delete CONNECTION_ID\n" +
			"  n8n git-connection delete CONNECTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent connection and checkout deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deletion acknowledgement)")
	return cmd
}

func runGitConnectionDelete(ctx context.Context, opts Options, id string, f gitConnectionDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete Git connection %q from %s? The connection and its local checkout go away; linked projects keep their instance content but lose their remote.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteGitConnection(ctx, id); err != nil {
		return gitConnectionAPIError(err, resolution, "delete")
	}
	return writeGitConnectionMutation(opts, resolution, gitConnectionMutation{Action: "deleted", ConnectionID: id}, f.output)
}

type gitConnectionCloneFlags struct {
	instance instanceFlags
	branch   string
	output   string
}

func newGitConnectionCloneCommand(opts Options) *cobra.Command {
	var f gitConnectionCloneFlags
	cmd := &cobra.Command{
		Use:   "clone <connection-id>",
		Short: "Clone the Git repository locally",
		Long: "Clone the connection's repository into local storage. Omit --branch\n" +
			"to clone the connection's configured branch, or pass it to clone a\n" +
			"different branch of at most 255 characters.\n\n" +
			"Cloning is safe to repeat and changes no project content by itself:\n" +
			"it only prepares the checkout that 'project add', 'push', and 'pull'\n" +
			"work against, so it needs no confirmation. Clone before pushing or\n" +
			"pulling. Requires gitConnection:clone.",
		Example: "  n8n git-connection clone CONNECTION_ID\n" +
			"  n8n git-connection clone CONNECTION_ID --branch main\n" +
			"  n8n git-connection clone CONNECTION_ID --branch release --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionClone(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.branch, "branch", "", "branch to clone, at most 255 characters (default: the connection's configured branch)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the connection returned by the API)")
	return cmd
}

func runGitConnectionClone(ctx context.Context, opts Options, id string, f gitConnectionCloneFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	request := n8n.CloneGitConnectionRequest{BranchName: strings.TrimSpace(f.branch)}
	if f.branch != "" && request.BranchName == "" {
		return fmt.Errorf("--branch must not be blank: pass a branch name of 1 to 255 characters, or omit it for the configured branch")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	cloned, err := client.CloneGitConnection(ctx, id, request)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "clone")
	}
	return writeGitConnectionResult(opts, resolution, "Cloned", cloned, f.output)
}

type gitConnectionDisconnectFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newGitConnectionDisconnectCommand(opts Options) *cobra.Command {
	var f gitConnectionDisconnectFlags
	cmd := &cobra.Command{
		Use:   "disconnect <connection-id>",
		Short: "Remove the local Git checkout",
		Long: "Remove the local clone of one Git connection. The connection and its\n" +
			"authentication material are retained, and linked projects stay linked,\n" +
			"but push and pull stop working until 'clone' runs again.\n\n" +
			"To remove the connection itself as well, use 'n8n git-connection\n" +
			"delete' instead. Interactive runs ask for confirmation;\n" +
			"non-interactive runs require --yes. Requires gitConnection:clone.",
		Example: "  n8n git-connection disconnect CONNECTION_ID\n" +
			"  n8n git-connection disconnect CONNECTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionDisconnect(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm checkout removal without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the connection returned by the API)")
	return cmd
}

func runGitConnectionDisconnect(ctx context.Context, opts Options, id string, f gitConnectionDisconnectFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Remove the local checkout of Git connection %q on %s? The connection stays, but push and pull stop working until it is cloned again.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	disconnected, err := client.DisconnectGitConnection(ctx, id)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "disconnect")
	}
	return writeGitConnectionResult(opts, resolution, "Disconnected", disconnected, f.output)
}

func newGitConnectionProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects linked to a Git connection",
		Long: "Manage which team projects one Git connection syncs. Start with\n" +
			"'list' to see the linked project IDs, then 'add' a project or\n" +
			"'remove' one.\n\n" +
			"A project can belong to only one connection, so adding a linked\n" +
			"project elsewhere answers 409. Removing unlinks the project; its\n" +
			"instance content is untouched. Project IDs come from 'n8n project\n" +
			"list'. Adding and listing need gitConnection:read for reads and\n" +
			"gitConnection:manageProjects for writes.",
		Example: "  n8n git-connection project list CONNECTION_ID\n" +
			"  n8n git-connection project add CONNECTION_ID PROJECT_ID\n" +
			"  n8n git-connection project remove CONNECTION_ID PROJECT_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newGitConnectionProjectListCommand(opts),
		newGitConnectionProjectAddCommand(opts),
		newGitConnectionProjectRemoveCommand(opts),
	)
	return cmd
}

type gitConnectionProjectListFlags struct {
	instance instanceFlags
	output   string
}

func newGitConnectionProjectListCommand(opts Options) *cobra.Command {
	var f gitConnectionProjectListFlags
	cmd := &cobra.Command{
		Use:   "list <connection-id>",
		Short: "List projects linked to a Git connection",
		Long: "List the team projects added to one Git connection. These are the\n" +
			"projects 'push' exports and 'pull' overwrites. Find the connection ID\n" +
			"with 'n8n git-connection list'. Requires gitConnection:read.",
		Example: "  n8n git-connection project list CONNECTION_ID\n" +
			"  n8n git-connection project list CONNECTION_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionProjectList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the projectIds object)")
	return cmd
}

func runGitConnectionProjectList(ctx context.Context, opts Options, id string, f gitConnectionProjectListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	projects, err := client.ListGitConnectionProjects(ctx, id)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "project list")
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
		fmt.Fprintln(opts.Streams.Err, "No projects are linked. Add one with 'n8n git-connection project add CONNECTION_ID PROJECT_ID'.")
	}
	return nil
}

type gitConnectionProjectWriteFlags struct {
	instance instanceFlags
	output   string
}

func newGitConnectionProjectAddCommand(opts Options) *cobra.Command {
	var f gitConnectionProjectWriteFlags
	cmd := &cobra.Command{
		Use:   "add <connection-id> <project-id>",
		Short: "Add a project to a Git connection",
		Long: "Add one team project to one Git connection so push and pull include\n" +
			"it. A project can belong to only one connection: adding a project\n" +
			"that is already linked elsewhere answers 409. The repository must be\n" +
			"cloned first. Requires gitConnection:manageProjects.",
		Example: "  n8n git-connection project add CONNECTION_ID PROJECT_ID\n" +
			"  n8n git-connection project add CONNECTION_ID PROJECT_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionProjectAdd(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the project link returned by the API)")
	return cmd
}

func runGitConnectionProjectAdd(ctx context.Context, opts Options, id, projectID string, f gitConnectionProjectWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("connection ID and project ID are required: pass the IDs from 'n8n git-connection list' and 'n8n project list'")
	}
	if strings.TrimSpace(id) != id || strings.TrimSpace(projectID) != projectID {
		return fmt.Errorf("connection ID and project ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	link, err := client.AddProjectToGitConnection(ctx, id, projectID)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "project add")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, link)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Added:\t%s\n", link.ProjectID)
	fmt.Fprintf(tw, "Connection:\t%s\n", link.GitConnectionID)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type gitConnectionProjectRemoveFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newGitConnectionProjectRemoveCommand(opts Options) *cobra.Command {
	var f gitConnectionProjectRemoveFlags
	cmd := &cobra.Command{
		Use:   "remove <connection-id> <project-id>",
		Short: "Remove a project from a Git connection",
		Long: "Unlink one project from one Git connection. The project itself and\n" +
			"its instance content are untouched; only the sync link goes away, so\n" +
			"later pushes and pulls skip it. Re-add it later with 'n8n\n" +
			"git-connection project add'.\n\n" +
			"Interactive runs ask for confirmation; non-interactive runs require\n" +
			"--yes. Requires gitConnection:manageProjects.",
		Example: "  n8n git-connection project remove CONNECTION_ID PROJECT_ID\n" +
			"  n8n git-connection project remove CONNECTION_ID PROJECT_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionProjectRemove(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm project unlinking without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the unlinking acknowledgement)")
	return cmd
}

func runGitConnectionProjectRemove(ctx context.Context, opts Options, id, projectID string, f gitConnectionProjectRemoveFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("connection ID and project ID are required: pass the IDs from 'n8n git-connection list' and 'n8n project list'")
	}
	if strings.TrimSpace(id) != id || strings.TrimSpace(projectID) != projectID {
		return fmt.Errorf("connection ID and project ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Remove project %q from Git connection %q on %s? The project content stays; later pushes and pulls skip it.", projectID, id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.RemoveProjectFromGitConnection(ctx, id, projectID); err != nil {
		return gitConnectionAPIError(err, resolution, "project remove")
	}
	return writeGitConnectionMutation(opts, resolution, gitConnectionMutation{Action: "project removed", ConnectionID: id, ProjectID: projectID}, f.output)
}

type gitConnectionPushFlags struct {
	instance instanceFlags
	commit   string
	force    bool
	yes      bool
	output   string
}

func newGitConnectionPushCommand(opts Options) *cobra.Command {
	var f gitConnectionPushFlags
	cmd := &cobra.Command{
		Use:   "push <connection-id>",
		Short: "Push all team projects to the Git remote",
		Long: "Export all team projects, commit them, and push to the connection's\n" +
			"configured branch. Personal projects are ignored. The repository must\n" +
			"be cloned first: run 'n8n git-connection clone' and link projects\n" +
			"with 'n8n git-connection project add'.\n\n" +
			"Pass --commit-message once (1 to 1000 characters). Without --force a\n" +
			"push the remote would reject is refused with 409 and nothing is\n" +
			"pushed; --force pushes anyway and rewrites remote history. This\n" +
			"mutates the remote branch. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires gitConnection:push.",
		Example: "  n8n git-connection push CONNECTION_ID --commit-message \"sync workflows\"\n" +
			"  n8n git-connection push CONNECTION_ID --commit-message \"sync\" --force --yes\n" +
			"  n8n git-connection push CONNECTION_ID --commit-message \"sync\" --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionPush(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.commit, "commit-message", "", "commit message for the push, 1 to 1000 characters (required)")
	cmd.Flags().BoolVar(&f.force, "force", false, "push even when the remote would reject it, rewriting remote history (default: reject a rejected push with 409)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm pushing every team project to the remote without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the push result with counts and commit SHA)")
	return cmd
}

func runGitConnectionPush(ctx context.Context, opts Options, id string, f gitConnectionPushFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	request := n8n.PushGitConnectionRequest{CommitMessage: f.commit, Force: f.force}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Push every team project on %s to the remote of Git connection %q with commit message %q? Personal projects are ignored.", resolution.URL, id, request.CommitMessage)
	if request.Force {
		question = fmt.Sprintf("Force-push every team project on %s to the remote of Git connection %q with commit message %q? Remote history is rewritten.", resolution.URL, id, request.CommitMessage)
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.PushGitConnectionProjects(ctx, id, request)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "push")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writeGitPushResult(opts, resolution, result)
}

type gitConnectionPullFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newGitConnectionPullCommand(opts Options) *cobra.Command {
	var f gitConnectionPullFlags
	cmd := &cobra.Command{
		Use:   "pull <connection-id>",
		Short: "Pull the Git remote into this instance",
		Long: "Reset the local clone to the configured branch tip and import\n" +
			"projects into the instance, overwriting to match. The repository must\n" +
			"be cloned first. Preview what is linked with 'n8n git-connection\n" +
			"project list'.\n\n" +
			"This rewrites instance content from the remote and cannot be undone:\n" +
			"workflows, folders, credentials, data tables, variables, and tags are\n" +
			"created, updated, or removed to match the branch, and credential gaps\n" +
			"become stubs. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires gitConnection:pull.",
		Example: "  n8n git-connection pull CONNECTION_ID\n" +
			"  n8n git-connection pull CONNECTION_ID --yes\n" +
			"  n8n git-connection pull CONNECTION_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitConnectionPull(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm overwriting instance content from the remote without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the pull result with counts and commit SHA)")
	return cmd
}

func runGitConnectionPull(ctx context.Context, opts Options, id string, f gitConnectionPullFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("connection ID is required: pass the ID from 'n8n git-connection list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("connection ID must not start or end with whitespace")
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Pull the remote of Git connection %q into %s and overwrite instance projects to match? Workflows, folders, credentials, tables, variables, and tags change; this cannot be undone.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.PullGitConnectionProjects(ctx, id)
	if err != nil {
		return gitConnectionAPIError(err, resolution, "pull")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writeGitPullResult(opts, resolution, result)
}

func readGitConnectionDocument(opts Options, path string, dst any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass a JSON file path or --input - for stdin")
	}
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open git-connection input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxGitConnectionDocument+1))
	if err != nil {
		return fmt.Errorf("read git-connection JSON: %w", err)
	}
	if len(raw) > maxGitConnectionDocument {
		return fmt.Errorf("git-connection JSON exceeds %d bytes", maxGitConnectionDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode git-connection JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode git-connection JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode git-connection JSON: %w", err)
	}
	return nil
}

func writeGitConnectionResult(opts Options, resolution config.Resolution, action string, connection *n8n.GitConnection, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, connection)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, connection.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", emptyDash(connection.Name))
	fmt.Fprintf(tw, "Repository:\t%s\n", emptyDash(connection.RepositoryURL))
	fmt.Fprintf(tw, "Branch:\t%s\n", emptyDash(derefGitConnection(connection.BranchName)))
	fmt.Fprintf(tw, "Type:\t%s\n", emptyDash(connection.ConnectionType))
	fmt.Fprintf(tw, "Public key:\t%s\n", truncatedGitConnectionSecret(derefGitConnection(connection.PublicKey)))
	fmt.Fprintf(tw, "Key generator:\t%s\n", emptyDash(derefGitConnection(connection.KeyGeneratorType)))
	fmt.Fprintf(tw, "Base commit:\t%s\n", emptyDash(derefGitConnection(connection.BaseCommit)))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if connection.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", connection.CreatedAt)
	}
	if connection.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", connection.UpdatedAt)
	}
	return tw.Flush()
}

type gitConnectionMutation struct {
	Action       string `json:"action"`
	ConnectionID string `json:"connectionId"`
	ProjectID    string `json:"projectId,omitempty"`
}

func writeGitConnectionMutation(opts Options, resolution config.Resolution, result gitConnectionMutation, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	fmt.Fprintf(tw, "Connection:\t%s\n", result.ConnectionID)
	if result.ProjectID != "" {
		fmt.Fprintf(tw, "Project:\t%s\n", result.ProjectID)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func writeGitPushResult(opts Options, resolution config.Resolution, result *n8n.PushGitConnectionResult) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Pushed:\t%s\n", result.ConnectionID)
	fmt.Fprintf(tw, "Commit:\t%s\n", emptyDash(result.CommitSHA))
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

func writeGitPullResult(opts Options, resolution config.Resolution, result *n8n.PullGitConnectionResult) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Pulled:\t%s\n", result.ConnectionID)
	fmt.Fprintf(tw, "Commit:\t%s\n", emptyDash(result.CommitSHA))
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

func derefGitConnection(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// truncatedGitConnectionSecret keeps public keys out of full display: the key
// is deploy material, so text output shows only a prefix while JSON keeps the
// complete value for scripting.
func truncatedGitConnectionSecret(value string) string {
	if value == "" {
		return "-"
	}
	const prefix = 24
	if len(value) <= prefix {
		return value
	}
	return value[:prefix] + "…"
}

func gitConnectionAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsConflict(err) && action == "create":
		return fmt.Errorf("%s rejected the git-connection create (409): only one connection can exist, so nothing changed; update or delete the existing one from 'n8n git-connection list': %w", resolution.URL, err)
	case n8n.IsConflict(err) && action == "project add":
		return fmt.Errorf("%s rejected the git-connection project add (409): the project is already linked to a connection, so nothing changed; remove it there first, then add it here: %w", resolution.URL, err)
	case n8n.IsConflict(err):
		return fmt.Errorf("%s rejected the git-connection %s (409): the connection or project is in a conflicting state, so nothing changed; list the current state and retry: %w", resolution.URL, action, err)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the git-connection %s (400): check the IDs, the connection document fields, the branch name, and the commit message: %w", resolution.URL, action, err)
	case n8n.IsStatus(err, http.StatusUnprocessableEntity):
		return fmt.Errorf("%s rejected the git-connection %s (422): the remote content cannot be imported as-is (missing references or unresolvable content); resolve the references and retry: %w", resolution.URL, action, err)
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied the git-connection %s (403): the credential needs %s; run %s", resolution.URL, action, gitConnectionScope(action), discoverHint(gitConnectionResource))
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s does not serve git connections (%d): it needs the GitConnections module (a Promotions-generation instance answers here instead); run %s to see what this instance offers: %w",
			resolution.URL, n8n.StatusCodeOf(err), discoverHint(gitConnectionResource), err)
	default:
		return apiError(err, resolution, gitConnectionResource)
	}
}

func gitConnectionScope(action string) string {
	switch action {
	case "list", "project list":
		return "gitConnection:list"
	case "create":
		return "gitConnection:create"
	case "get":
		return "gitConnection:read"
	case "update":
		return "gitConnection:update"
	case "delete":
		return "gitConnection:delete"
	case "clone", "disconnect":
		return "gitConnection:clone"
	case "project add", "project remove":
		return "gitConnection:manageProjects"
	case "push":
		return "gitConnection:push"
	case "pull":
		return "gitConnection:pull"
	default:
		return "gitConnection:*"
	}
}
