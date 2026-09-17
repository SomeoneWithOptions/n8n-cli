package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// folderResource is the 'n8n discover' key for folders, which the API spells
// in the plural.
const folderResource = "folders"

func newFolderCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folder",
		Short: "Manage the folders of a project",
		Long: "Manage folders inside one project. Every command takes the project ID first,\n" +
			"because folders live in a project and their IDs are only unique within one.\n\n" +
			"Start with 'list' to find folder IDs, then create, inspect, rename, move, or\n" +
			"delete a folder. Project IDs come from 'n8n project list'; 'create' also accepts\n" +
			"the literal 'personal' for the calling user's own personal project, which the\n" +
			"other commands do not. Deleting a folder moves or archives what it holds, so it\n" +
			"always requires confirmation.",
		Example: "  n8n folder list PROJECT_ID\n" +
			"  n8n folder create PROJECT_ID Invoices\n" +
			"  n8n folder get PROJECT_ID FOLDER_ID\n" +
			"  n8n folder update PROJECT_ID FOLDER_ID --name \"Paid invoices\"\n" +
			"  n8n folder delete PROJECT_ID FOLDER_ID --transfer-to-folder-id OTHER_FOLDER_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newFolderListCommand(opts),
		newFolderCreateCommand(opts),
		newFolderGetCommand(opts),
		newFolderUpdateCommand(opts),
		newFolderDeleteCommand(opts),
	)
	return cmd
}

type folderListFlags struct {
	instance instanceFlags
	skip     int
	take     int
	all      bool
	sortBy   string
	selected []string
	filter   string
	parent   string
	name     string
	tags     []string
	exclude  string
	output   string
}

func newFolderListCommand(opts Options) *cobra.Command {
	var f folderListFlags
	cmd := &cobra.Command{
		Use:   "list <project-id>",
		Short: "List the folders of a project",
		Long: "List folders in one project. This endpoint pages by offset, not by cursor: ask\n" +
			"for a page with --take, move through the result set with --skip, or use --all to\n" +
			"walk every page, capped at 10,000 folders. The reported count is the number of\n" +
			"folders matching the query, which is larger than one page.\n\n" +
			"Narrow the result set with --parent-folder-id, --name, --tag, and\n" +
			"--exclude-folder-id, or pass the whole documented filter object as JSON with\n" +
			"--filter; the two ways are mutually exclusive. --select limits the returned\n" +
			"fields, which is worth it on large projects; note that the 'project', 'tags',\n" +
			"and 'parentFolder' select fields are answered with a 500 by some instances, so\n" +
			"prefer leaving --select off when in doubt, since the full response already\n" +
			"carries them. Requires folder:list.",
		Example: "  n8n folder list PROJECT_ID\n" +
			"  n8n folder list PROJECT_ID --take 50 --sort-by name:asc\n" +
			"  n8n folder list PROJECT_ID --skip 50 --take 50 --output json\n" +
			"  n8n folder list PROJECT_ID --parent-folder-id FOLDER_ID\n" +
			"  n8n folder list PROJECT_ID --name Invoices --tag finance\n" +
			"  n8n folder list PROJECT_ID --filter '{\"name\":\"Invoices\"}'\n" +
			"  n8n folder list PROJECT_ID --select id --select name --all",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.skip, "skip", 0, "number of folders to skip before the page (default: start at the first folder)")
	cmd.Flags().IntVar(&f.take, "take", 0, "folders per API page (default: server default of 10; 100 with --all)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 folders)")
	cmd.Flags().StringVar(&f.sortBy, "sort-by", "", "sort order: one of "+strings.Join(n8n.FolderSortFields, ", ")+" (default: the server's order)")
	cmd.Flags().StringArrayVar(&f.selected, "select", nil, "field to return, repeatable: one of "+strings.Join(n8n.FolderSelectFields, ", ")+" (default: every field)")
	cmd.Flags().StringVar(&f.filter, "filter", "", "whole filter object as JSON, e.g. '{\"name\":\"Invoices\"}' (cannot combine with the single-filter flags)")
	cmd.Flags().StringVar(&f.parent, "parent-folder-id", "", "only folders directly inside this folder (cannot combine with --filter)")
	cmd.Flags().StringVar(&f.name, "name", "", "only folders whose name matches this value (cannot combine with --filter)")
	cmd.Flags().StringArrayVar(&f.tags, "tag", nil, "only folders carrying this tag name, repeatable (cannot combine with --filter)")
	cmd.Flags().StringVar(&f.exclude, "exclude-folder-id", "", "drop this folder and everything under it (cannot combine with --filter)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is an object with count and data)")
	return cmd
}

func runFolderList(ctx context.Context, opts Options, projectID string, f folderListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateFolderProjectID(projectID); err != nil {
		return err
	}
	filter, err := folderFilter(f)
	if err != nil {
		return err
	}
	listOpts := n8n.ListFoldersOptions{
		Skip:   f.skip,
		Take:   f.take,
		Filter: filter,
		Select: f.selected,
		SortBy: f.sortBy,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pagination n8n.ListFoldersOptions) (n8n.FolderPage, error) {
		return client.ListFolders(ctx, projectID, pagination)
	}
	var page n8n.FolderPage
	if f.all {
		page.Data, err = n8n.CollectFolders(ctx, fetch, listOpts, n8n.DefaultCollectLimit)
		// Every matching folder was collected, so the count is what is here.
		page.Count = len(page.Data)
	} else {
		page, err = fetch(ctx, listOpts)
	}
	if err != nil {
		return folderAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeFolderListText(opts, resolution, projectID, page, f)
}

// folderFilter builds the filter from the single-filter flags or from the raw
// JSON document, which are mutually exclusive.
func folderFilter(f folderListFlags) (n8n.FolderFilter, error) {
	single := n8n.FolderFilter{
		ParentFolderID:                f.parent,
		Name:                          f.name,
		Tags:                          f.tags,
		ExcludeFolderIDAndDescendants: f.exclude,
	}
	raw := strings.TrimSpace(f.filter)
	if raw == "" {
		return single, nil
	}
	if !single.IsZero() {
		return n8n.FolderFilter{}, fmt.Errorf("--filter cannot be combined with --parent-folder-id, --name, --tag or --exclude-folder-id: pass the filter one way")
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	var filter n8n.FolderFilter
	if err := decoder.Decode(&filter); err != nil {
		return n8n.FolderFilter{}, fmt.Errorf("decode --filter JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return n8n.FolderFilter{}, fmt.Errorf("decode --filter JSON: multiple documents are not allowed")
		}
		return n8n.FolderFilter{}, fmt.Errorf("decode --filter JSON: %w", err)
	}
	if filter.IsZero() {
		return n8n.FolderFilter{}, fmt.Errorf("--filter narrows nothing: set parentFolderId, name, tags or excludeFolderIdAndDescendants, or leave the flag off")
	}
	return filter, nil
}

func writeFolderListText(opts Options, resolution config.Resolution, projectID string, page n8n.FolderPage, f folderListFlags) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Project:\t%s\n", projectID)
	fmt.Fprintf(tw, "Matching folders:\t%d\n", page.Count)
	fmt.Fprintf(tw, "Shown:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tPARENT FOLDER\tWORKFLOWS\tSUBFOLDERS")
		for _, folder := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				emptyDash(folder.ID), emptyDash(folder.Name), emptyDash(folder.ParentFolderID),
				folderCount(folder.WorkflowCount), folderCount(folder.SubFolderCount))
		}
	}
	if !f.all && page.Count > f.skip+len(page.Data) {
		fmt.Fprintf(tw, "\nNext page:\t--skip %d\n", f.skip+len(page.Data))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No folders were returned. Create one with 'n8n folder create PROJECT_ID NAME'.")
	}
	return nil
}

// folderCount renders an optional count: absent means the instance did not
// send it, which is not the same as zero.
func folderCount(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value)
}

type folderWriteFlags struct {
	instance instanceFlags
	parent   string
	output   string
}

func newFolderCreateCommand(opts Options) *cobra.Command {
	var f folderWriteFlags
	cmd := &cobra.Command{
		Use:   "create <project-id> <name>",
		Short: "Create a folder in a project",
		Long: "Create one folder in one project. Without --parent-folder-id the folder is\n" +
			"created at the project root; with it, the folder is created inside that folder.\n\n" +
			"This command also accepts the literal project ID 'personal', which creates the\n" +
			"folder in the calling user's own personal project; list, get, update and delete\n" +
			"need the real project ID instead. Quote names containing spaces. Requires\n" +
			"folder:create.",
		Example: "  n8n folder create PROJECT_ID Invoices\n" +
			"  n8n folder create PROJECT_ID \"Paid invoices\" --parent-folder-id FOLDER_ID\n" +
			"  n8n folder create personal Invoices --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderCreate(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.parent, "parent-folder-id", "", "create the folder inside this folder (default: the project root)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the created folder)")
	return cmd
}

func runFolderCreate(ctx context.Context, opts Options, projectID, name string, f folderWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateFolderProjectID(projectID); err != nil {
		return err
	}
	request := n8n.CreateFolderRequest{Name: name, ParentFolderID: f.parent}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	folder, err := client.CreateFolder(ctx, projectID, request)
	if err != nil {
		return folderAPIError(err, resolution, "create")
	}
	return writeFolderResult(opts, resolution, "Created", projectID, folder, f.output)
}

func newFolderGetCommand(opts Options) *cobra.Command {
	var f folderWriteFlags
	cmd := &cobra.Command{
		Use:   "get <project-id> <folder-id>",
		Short: "Show one folder with its totals",
		Long: "Show one folder: its name, its parent folder, and how many sub-folders and\n" +
			"workflows it holds. Both totals are recursive, so they count everything in the\n" +
			"subtree, not only the direct children the list command reports.\n\n" +
			"Read those totals before deleting a folder: they are what the deletion moves,\n" +
			"archives or removes. Find folder IDs with 'n8n folder list'. Requires\n" +
			"folder:read.",
		Example: "  n8n folder get PROJECT_ID FOLDER_ID\n" +
			"  n8n folder get PROJECT_ID FOLDER_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderGet(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the folder object with its totals)")
	return cmd
}

func runFolderGet(ctx context.Context, opts Options, projectID, folderID string, f folderWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateFolderProjectID(projectID); err != nil {
		return err
	}
	if err := validateFolderID(folderID); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	folder, err := client.GetFolder(ctx, projectID, folderID)
	if err != nil {
		return folderAPIError(err, resolution, "read")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, folder)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Folder:\t%s\n", emptyDash(folder.Name))
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(folder.ID))
	fmt.Fprintf(tw, "Project:\t%s\n", projectID)
	fmt.Fprintf(tw, "Parent folder:\t%s\n", emptyDash(folder.ParentFolderID))
	fmt.Fprintf(tw, "Sub-folders:\t%d\n", folder.TotalSubFolders)
	fmt.Fprintf(tw, "Workflows:\t%d\n", folder.TotalWorkflows)
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(folder.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", emptyDash(folder.UpdatedAt))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type folderUpdateFlags struct {
	write folderWriteFlags
	name  string
}

func newFolderUpdateCommand(opts Options) *cobra.Command {
	var f folderUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <project-id> <folder-id>",
		Short: "Rename a folder or move it under another folder",
		Long: "Update one folder. This is a partial update: --name renames the folder,\n" +
			"--parent-folder-id moves it (with everything inside it) under another folder,\n" +
			"and passing both does both. At least one of them is required; anything left out\n" +
			"stays as it is.\n\n" +
			"The API has no way to clear a parent, so a folder cannot be moved back to the\n" +
			"project root with this command, and the target folder must be in the same\n" +
			"project. Requires folder:update.",
		Example: "  n8n folder update PROJECT_ID FOLDER_ID --name \"Paid invoices\"\n" +
			"  n8n folder update PROJECT_ID FOLDER_ID --parent-folder-id OTHER_FOLDER_ID\n" +
			"  n8n folder update PROJECT_ID FOLDER_ID --name Archive --parent-folder-id OTHER_FOLDER_ID --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderUpdate(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.write.instance.register(cmd)
	cmd.Flags().StringVar(&f.name, "name", "", "new folder name (required unless --parent-folder-id is given)")
	cmd.Flags().StringVar(&f.write.parent, "parent-folder-id", "", "move the folder inside this folder (required unless --name is given)")
	cmd.Flags().StringVar(&f.write.output, "output", outputText, "output format: text or json (JSON is the updated folder)")
	return cmd
}

func runFolderUpdate(ctx context.Context, opts Options, projectID, folderID string, f folderUpdateFlags) error {
	if err := validateOutput(f.write.output); err != nil {
		return err
	}
	if err := validateFolderProjectID(projectID); err != nil {
		return err
	}
	if err := validateFolderID(folderID); err != nil {
		return err
	}
	request := n8n.UpdateFolderRequest{Name: f.name, ParentFolderID: f.write.parent}
	if request.Name == "" && request.ParentFolderID == "" {
		return fmt.Errorf("--name or --parent-folder-id is required: one renames the folder, the other moves it")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.write.instance)
	if err != nil {
		return err
	}
	folder, err := client.UpdateFolder(ctx, projectID, folderID, request)
	if err != nil {
		return folderAPIError(err, resolution, "update")
	}
	return writeFolderResult(opts, resolution, "Updated", projectID, folder, f.write.output)
}

type folderDeleteFlags struct {
	instance instanceFlags
	transfer string
	yes      bool
	output   string
}

func newFolderDeleteCommand(opts Options) *cobra.Command {
	var f folderDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <project-id> <folder-id>",
		Short: "Delete a folder and move or archive what it holds",
		Long: "Delete one folder. What happens to its contents depends on the transfer target.\n\n" +
			"With --transfer-to-folder-id, the workflows and sub-folders are moved into that\n" +
			"folder first and nothing is archived or lost. Without it, the workflows are\n" +
			"moved to the project root and archived, and every folder underneath is deleted\n" +
			"with it. Run 'n8n folder get' first to see how much that is; the totals it\n" +
			"reports are recursive.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires folder:delete.",
		Example: "  n8n folder delete PROJECT_ID FOLDER_ID\n" +
			"  n8n folder delete PROJECT_ID FOLDER_ID --transfer-to-folder-id OTHER_FOLDER_ID --yes\n" +
			"  n8n folder delete PROJECT_ID FOLDER_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderDelete(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.transfer, "transfer-to-folder-id", "", "move the workflows and sub-folders into this folder first (default: archive the workflows at the project root and delete the child folders)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains the project, folder and transfer target)")
	return cmd
}

func runFolderDelete(ctx context.Context, opts Options, projectID, folderID string, f folderDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateFolderProjectID(projectID); err != nil {
		return err
	}
	if err := validateFolderID(folderID); err != nil {
		return err
	}
	if err := validateFolderTransferTarget(f.transfer); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := opts.confirmer(f.yes).Confirm(folderDeleteQuestion(projectID, folderID, f.transfer, resolution)); err != nil {
		return err
	}
	if err := client.DeleteFolder(ctx, projectID, folderID, f.transfer); err != nil {
		return folderAPIError(err, resolution, "delete")
	}
	return writeFolderMutation(opts, resolution, folderMutation{
		Action:             "deleted",
		ProjectID:          projectID,
		FolderID:           folderID,
		TransferToFolderID: f.transfer,
	}, f.output)
}

// folderDeleteQuestion states what the deletion does to the contents, which is
// the whole reason this command is guarded: the two variants differ.
func folderDeleteQuestion(projectID, folderID, transfer string, resolution config.Resolution) string {
	if transfer != "" {
		return fmt.Sprintf("Delete folder %q in project %q on %s? Its workflows and sub-folders are moved into folder %q first.",
			folderID, projectID, resolution.URL, transfer)
	}
	return fmt.Sprintf("Delete folder %q in project %q on %s? Its workflows are moved to the project root and archived, and every folder underneath it is deleted. Pass --transfer-to-folder-id to move them into another folder instead.",
		folderID, projectID, resolution.URL)
}

func writeFolderResult(opts Options, resolution config.Resolution, action, projectID string, folder *n8n.Folder, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, folder)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, emptyDash(folder.Name))
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(folder.ID))
	fmt.Fprintf(tw, "Project:\t%s\n", projectID)
	fmt.Fprintf(tw, "Parent folder:\t%s\n", emptyDash(folder.ParentFolderID))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type folderMutation struct {
	Action             string `json:"action"`
	ProjectID          string `json:"projectId"`
	FolderID           string `json:"folderId"`
	TransferToFolderID string `json:"transferToFolderId,omitempty"`
}

func writeFolderMutation(opts Options, resolution config.Resolution, result folderMutation, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	fmt.Fprintf(tw, "Project:\t%s\n", result.ProjectID)
	fmt.Fprintf(tw, "Folder:\t%s\n", result.FolderID)
	if result.TransferToFolderID != "" {
		fmt.Fprintf(tw, "Contents moved to:\t%s\n", result.TransferToFolderID)
	} else {
		fmt.Fprintln(tw, "Contents:\tworkflows archived at the project root, child folders deleted")
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func validateFolderProjectID(id string) error { return validateFolderArgument("project ID", id) }

func validateFolderID(id string) error { return validateFolderArgument("folder ID", id) }

func validateFolderTransferTarget(id string) error {
	if id == "" {
		return nil
	}
	return validateFolderArgument("transfer target folder ID", id)
}

func validateFolderArgument(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	return nil
}

// folderAPIError explains the two denials this resource produces that a bare
// status code does not: a missing folder scope, and a 404 that can mean either
// the project or the folder.
func folderAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.IsForbidden(err) {
		return fmt.Errorf("%s denied folder %s (403): the credential lacks the folder:%s scope or access to the project; run %s to inspect access",
			resolution.URL, action, folderScope(action), discoverHint(folderResource))
	}
	if n8n.IsNotFound(err) {
		return fmt.Errorf("%s has no such folder or project (404): check the project ID with 'n8n project list' and the folder ID with 'n8n folder list PROJECT_ID'; note that only 'n8n folder create' accepts the literal project ID 'personal'", resolution.URL)
	}
	return apiError(err, resolution, folderResource)
}

// folderScope maps a command action to the scope the API requires for it.
func folderScope(action string) string {
	switch action {
	case "read":
		return "read"
	case "create":
		return "create"
	case "update":
		return "update"
	case "delete":
		return "delete"
	default:
		return "list"
	}
}
