package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const tagResource = "tags"

func newTagCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage workflow tags",
		Long: "Manage the workflow tags of an n8n instance.\n\n" +
			"Tags are instance-wide labels attached to workflows. Start with 'list' to find\n" +
			"the tag ID other actions need, then get, create, update or delete. A tag name is\n" +
			"unique per instance: creating or renaming to a name already in use fails with a\n" +
			"conflict.\n\n" +
			"Deleting a tag also removes it from every workflow that carries it; the workflows\n" +
			"themselves are untouched. Attaching tags to a workflow is not part of this group,\n" +
			"it belongs to the workflow commands.",
		Example: "  n8n tag list\n" +
			"  n8n tag create Production\n" +
			"  n8n tag get TAG_ID\n" +
			"  n8n tag update TAG_ID --name Staging\n" +
			"  n8n tag delete TAG_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newTagListCommand(opts),
		newTagCreateCommand(opts),
		newTagGetCommand(opts),
		newTagUpdateCommand(opts),
		newTagDeleteCommand(opts),
	)
	return cmd
}

type tagListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newTagListCommand(opts Options) *cobra.Command {
	var f tagListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags",
		Long: "List the tags of the selected instance, one cursor-paginated page at a time.\n" +
			"The page carries a next cursor; pass it to --cursor for the following page, or\n" +
			"use --all to follow every cursor, capped at 10,000 tags.\n\n" +
			"Use this to find the tag ID that get, update and delete take. Requires tag:list.",
		Example: "  n8n tag list\n" +
			"  n8n tag list --limit 50 --output json\n" +
			"  n8n tag list --cursor NEXT_CURSOR\n" +
			"  n8n tag list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTagList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "tags per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 tags)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runTagList(ctx context.Context, opts Options, f tagListFlags) error {
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
	var page n8n.Page[n8n.Tag]
	if f.all {
		page.Data, err = n8n.Collect(ctx, client.ListTags, listOpts, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListTags(ctx, listOpts)
	}
	if err != nil {
		return tagAPIError(err, resolution, "", "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeTagListText(opts, resolution, page)
}

func writeTagListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.Tag]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Tags:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tCREATED\tUPDATED")
		for _, tag := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", tag.ID, tag.Name, tag.CreatedAt, tag.UpdatedAt)
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No tags were returned. Create one with 'n8n tag create NAME'.")
	}
	return nil
}

type tagWriteFlags struct {
	instance instanceFlags
	output   string
}

func (f *tagWriteFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the tag object)")
}

func newTagCreateCommand(opts Options) *cobra.Command {
	var f tagWriteFlags
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a tag",
		Long: "Create one tag with the given name. Name is the only field a tag has; the ID and\n" +
			"timestamps are assigned by the instance and printed on success.\n\n" +
			"Names are unique per instance, so reusing an existing name fails with a conflict.\n" +
			"n8n also limits how long a name may be (24 characters on current versions) and\n" +
			"reports a longer one as the same conflict. Run 'n8n tag list' first when unsure,\n" +
			"and quote names containing spaces. Requires tag:create.",
		Example: "  n8n tag create Production\n" +
			"  n8n tag create \"Customer facing\" --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagCreate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runTagCreate(ctx context.Context, opts Options, name string, f tagWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateTagName(name); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	tag, err := client.CreateTag(ctx, name)
	if err != nil {
		return tagAPIError(err, resolution, name, "create")
	}
	return writeTagResult(opts, resolution, "Created", tag, f.output)
}

func newTagGetCommand(opts Options) *cobra.Command {
	var f tagWriteFlags
	cmd := &cobra.Command{
		Use:   "get <tag-id>",
		Short: "Get one tag",
		Long: "Get one tag by ID, as listed in the ID column of 'n8n tag list'. A tag that does\n" +
			"not exist answers 404. This is a read-only call; requires tag:read.",
		Example: "  n8n tag get TAG_ID\n" +
			"  n8n tag get TAG_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runTagGet(ctx context.Context, opts Options, id string, f tagWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateTagID(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	tag, err := client.GetTag(ctx, id)
	if err != nil {
		return tagAPIError(err, resolution, id, "read")
	}
	return writeTagResult(opts, resolution, "Tag", tag, f.output)
}

type tagUpdateFlags struct {
	write tagWriteFlags
	name  string
}

func newTagUpdateCommand(opts Options) *cobra.Command {
	var f tagUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <tag-id>",
		Short: "Rename a tag",
		Long: "Rename one tag. The API replaces the whole tag, but name is its only writable\n" +
			"field, so --name is the full replacement document and nothing else is lost: the\n" +
			"ID stays the same and every workflow keeps the tag.\n\n" +
			"No read-modify-write step is needed. Names are unique per instance, so a name\n" +
			"already in use fails with a conflict, as does a name longer than the n8n limit\n" +
			"(24 characters on current versions). Requires tag:update.",
		Example: "  n8n tag update TAG_ID --name Staging\n" +
			"  n8n tag update TAG_ID --name \"Customer facing\" --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.write.register(cmd)
	cmd.Flags().StringVar(&f.name, "name", "", "new tag name, replacing the current one (required)")
	return cmd
}

func runTagUpdate(ctx context.Context, opts Options, id string, f tagUpdateFlags) error {
	if err := validateOutput(f.write.output); err != nil {
		return err
	}
	if err := validateTagID(id); err != nil {
		return err
	}
	if strings.TrimSpace(f.name) == "" {
		return fmt.Errorf("--name is required: it is the new name the tag is renamed to")
	}
	if err := validateTagName(f.name); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.write.instance)
	if err != nil {
		return err
	}
	tag, err := client.UpdateTag(ctx, id, f.name)
	if err != nil {
		return tagAPIError(err, resolution, f.name, "update")
	}
	return writeTagResult(opts, resolution, "Updated", tag, f.write.output)
}

type tagDeleteFlags struct {
	write tagWriteFlags
	yes   bool
}

func newTagDeleteCommand(opts Options) *cobra.Command {
	var f tagDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <tag-id>",
		Short: "Permanently delete a tag",
		Long: "Permanently delete one tag. The tag is removed from every workflow that carries\n" +
			"it; those workflows keep working and are not deleted. The deletion cannot be\n" +
			"undone, and recreating the name produces a new ID.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires tag:delete.",
		Example: "  n8n tag delete TAG_ID\n" +
			"  n8n tag delete TAG_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.write.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion without prompting (required when stdin is not interactive)")
	return cmd
}

func runTagDelete(ctx context.Context, opts Options, id string, f tagDeleteFlags) error {
	if err := validateOutput(f.write.output); err != nil {
		return err
	}
	if err := validateTagID(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.write.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete tag %q from %s? It is removed from every workflow that carries it.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	tag, err := client.DeleteTag(ctx, id)
	if err != nil {
		return tagAPIError(err, resolution, id, "delete")
	}
	return writeTagResult(opts, resolution, "Deleted", tag, f.write.output)
}

func writeTagResult(opts Options, resolution config.Resolution, action string, tag *n8n.Tag, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, tag)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, tag.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", tag.Name)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if tag.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", tag.CreatedAt)
	}
	if tag.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", tag.UpdatedAt)
	}
	return tw.Flush()
}

// tagAPIError adds the duplicate-name remediation the two write operations
// share, names the scope the denied action needs, and otherwise defers to the
// shared credential and scope guidance.
func tagAPIError(err error, resolution config.Resolution, name, action string) error {
	if n8n.IsConflict(err) {
		return fmt.Errorf("%s rejected the tag name %q (409): names are unique per instance, and n8n reports a name longer than its limit (24 characters on current versions) the same way; shorten the name or run 'n8n tag list' to find the existing tag", resolution.URL, name)
	}
	if n8n.IsForbidden(err) {
		need := tagScope(action)
		return forbiddenScopeError(err, resolution, "tag "+action, need, "", tagResource)
	}
	return apiError(err, resolution, tagResource)
}

// tagScope maps a command action to the scope the API requires for it.
func tagScope(action string) scopeNeed {
	switch action {
	case "list":
		return allOf("tag:list")
	case "read":
		return allOf("tag:read")
	case "create":
		return allOf("tag:create")
	case "update":
		return allOf("tag:update")
	case "delete":
		return allOf("tag:delete")
	}
	return scopeNeed{}
}

func validateTagID(id string) error { return validateTagArgument("ID", id) }

func validateTagName(name string) error { return validateTagArgument("name", name) }

func validateTagArgument(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("tag %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("tag %s must not start or end with whitespace", field)
	}
	return nil
}
