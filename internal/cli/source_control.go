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
	sourceControlResource    = "sourcecontrol"
	maxSourceControlDocument = 1 << 20
)

func newSourceControlCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source-control",
		Short: "Preview, push, and pull source-controlled instance changes",
		Long: "Preview and move the instance content tracked by the licensed Source Control\n" +
			"feature and its connected Git repository. Start with 'status' to preview the\n" +
			"pending changes in one direction, then 'push' to commit local files to the\n" +
			"remote branch or 'pull' to rewrite local content from the remote.\n\n" +
			"'push' mutates the remote Git branch; 'pull' mutates this instance and can\n" +
			"discard local changes when forced. Both ask for confirmation, and previewing\n" +
			"first is the workflow: 'status --direction push' before a push and\n" +
			"'status --direction pull' before a pull.",
		Example: "  n8n source-control status --direction push\n" +
			"  n8n source-control status --direction pull --output json\n" +
			"  n8n source-control push --commit-message \"sync workflows\" --file abc:workflow\n" +
			"  n8n source-control pull --auto-publish none --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newSourceControlStatusCommand(opts),
		newSourceControlPushCommand(opts),
		newSourceControlPullCommand(opts),
	)
	return cmd
}

type sourceControlStatusFlags struct {
	instance  instanceFlags
	direction string
	output    string
}

func newSourceControlStatusCommand(opts Options) *cobra.Command {
	var f sourceControlStatusFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Preview pending source-control changes",
		Long: "Preview the pending changes between the instance and the connected Git\n" +
			"repository in one direction, without changing anything. --direction push\n" +
			"previews what a push would send; --direction pull previews what a pull would\n" +
			"bring in, including files marked with a conflict.\n\n" +
			"Run this before 'push' to select file entries and before 'pull' to see what\n" +
			"the remote would overwrite. A file entry carries its repository file path, its\n" +
			"object ID and type, its change status, and whether it conflicts. Requires\n" +
			"sourceControl:read and a licensed, connected Source Control feature.",
		Example: "  n8n source-control status --direction push\n" +
			"  n8n source-control status --direction pull\n" +
			"  n8n source-control status --direction push --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSourceControlStatus(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.direction, "direction", "", "preview direction: push or pull (required)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the preview object with data)")
	return cmd
}

func runSourceControlStatus(ctx context.Context, opts Options, f sourceControlStatusFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	direction := n8n.SourceControlDirection(strings.TrimSpace(f.direction))
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	status, err := client.GetSourceControlStatus(ctx, direction)
	if err != nil {
		return sourceControlAPIError(err, resolution, "preview")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, status)
	}
	return writeSourceControlFiles(opts, resolution, "Pending "+string(direction)+" changes", status.Data, string(direction))
}

type sourceControlPushFlags struct {
	instance      instanceFlags
	commitMessage string
	files         []string
	input         string
	force         bool
	yes           bool
	output        string
}

func newSourceControlPushCommand(opts Options) *cobra.Command {
	var f sourceControlPushFlags
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Commit and push local changes to the Git remote",
		Long: "Commit and push the selected local files to the connected Git repository.\n" +
			"Each entry in the selection is resolved against a fresh preview server-side,\n" +
			"so preview first with 'n8n source-control status --direction push' and pass the\n" +
			"file IDs and types it reports.\n\n" +
			"Pass --commit-message once and --file once per file as ID:TYPE, e.g.\n" +
			"--file abc:workflow, or supply the whole request document with --input FILE or\n" +
			"--input -. The two selection ways are mutually exclusive except that --force\n" +
			"may combine with either: without it a push containing unresolved conflicts is\n" +
			"rejected with 409 and nothing is pushed.\n\n" +
			"This mutates the remote Git branch. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires sourceControl:push and a licensed,\n" +
			"connected Source Control feature.",
		Example: "  n8n source-control status --direction push\n" +
			"  n8n source-control push --commit-message \"sync workflows\" --file abc:workflow\n" +
			"  n8n source-control push --commit-message \"sync\" --file abc:workflow --file cred-1:credential --force --yes\n" +
			"  n8n source-control push --input push.json --yes --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSourceControlPush(cmd.Context(), opts, f, cmd.Flags().Changed("force"))
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.commitMessage, "commit-message", "", "commit message for the push, 1 to 1000 characters (required unless --input is used)")
	cmd.Flags().StringArrayVar(&f.files, "file", nil, "file to push as ID:TYPE, repeatable (cannot combine with --input; types: credential, workflow, tags, variables, file, folders, project, datatable)")
	cmd.Flags().StringVar(&f.input, "input", "", "push JSON file path with commitMessage and fileNames, or - for stdin (cannot combine with --commit-message or --file; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.force, "force", false, "push despite unresolved conflicts (default: reject a conflicted push with 409)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm pushing to the remote Git branch without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the pushed files returned by the API)")
	return cmd
}

func runSourceControlPush(ctx context.Context, opts Options, f sourceControlPushFlags, forceSet bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the push confirmation: pass --yes after reviewing the full selection")
	}
	request, err := sourceControlPushRequest(opts, f, forceSet)
	if err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Push %s to the connected Git repository on %s with commit message %q? This commits and pushes to the remote branch; without --force a conflicted file rejects the whole push.",
		sourceControlFileCount(len(request.FileNames)), resolution.URL, request.CommitMessage)
	if request.Force {
		question = fmt.Sprintf("Force-push %s to the connected Git repository on %s with commit message %q? Unresolved conflicts are pushed anyway and the remote branch changes.",
			sourceControlFileCount(len(request.FileNames)), resolution.URL, request.CommitMessage)
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.PushSourceControl(ctx, request)
	if err != nil {
		return sourceControlAPIError(err, resolution, "push")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writeSourceControlFiles(opts, resolution, "Pushed", result.Data, "")
}

type sourceControlPullFlags struct {
	instance    instanceFlags
	force       bool
	autoPublish string
	yes         bool
	output      string
}

func newSourceControlPullCommand(opts Options) *cobra.Command {
	var f sourceControlPullFlags
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull remote Git changes into this instance",
		Long: "Fetch the connected Git branch into this instance, rewriting local instance\n" +
			"content from the remote. Preview first with\n" +
			"'n8n source-control status --direction pull' and check the conflict markers:\n" +
			"without --force a pull blocked by uncommitted local changes or merge conflicts\n" +
			"is rejected with 409 and nothing changes.\n\n" +
			"--force discards those local changes to complete the pull and cannot be undone.\n" +
			"--auto-publish controls imported workflows: none keeps every workflow in its\n" +
			"local published state, all publishes every imported workflow, and published\n" +
			"publishes only workflows that were published locally before the import.\n\n" +
			"This mutates the local instance. Interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. Requires sourceControl:pull and a licensed,\n" +
			"connected Source Control feature.",
		Example: "  n8n source-control status --direction pull\n" +
			"  n8n source-control pull\n" +
			"  n8n source-control pull --auto-publish published --yes\n" +
			"  n8n source-control pull --force --yes --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSourceControlPull(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.force, "force", false, "discard local changes and force the pull to complete (default: reject a conflicted pull with 409)")
	cmd.Flags().StringVar(&f.autoPublish, "auto-publish", "none", "workflow publishing after import: none, all, or published (default: none)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm rewriting local instance content without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the array of pulled files)")
	return cmd
}

func runSourceControlPull(ctx context.Context, opts Options, f sourceControlPullFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	request := n8n.PullSourceControlRequest{
		Force:       f.force,
		AutoPublish: n8n.SourceControlAutoPublish(strings.TrimSpace(f.autoPublish)),
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var question string
	switch request.AutoPublish {
	case n8n.SourceControlAutoPublishAll:
		question = fmt.Sprintf("Pull the connected Git branch into %s and rewrite local content? Every imported workflow is published.", resolution.URL)
	case n8n.SourceControlAutoPublishPublished:
		question = fmt.Sprintf("Pull the connected Git branch into %s and rewrite local content? Workflows published locally before the import are published again.", resolution.URL)
	default:
		question = fmt.Sprintf("Pull the connected Git branch into %s and rewrite local content? Imported workflows keep their local published state.", resolution.URL)
	}
	if request.Force {
		question += " Uncommitted local changes are discarded and cannot be recovered."
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	files, err := client.PullSourceControl(ctx, request)
	if err != nil {
		return sourceControlAPIError(err, resolution, "pull")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, files)
	}
	return writeSourceControlFiles(opts, resolution, "Pulled", files, "")
}

func sourceControlPushRequest(opts Options, f sourceControlPushFlags, forceSet bool) (n8n.PushSourceControlRequest, error) {
	var request n8n.PushSourceControlRequest
	hasInput := strings.TrimSpace(f.input) != ""
	hasFlags := strings.TrimSpace(f.commitMessage) != "" || len(f.files) > 0
	switch {
	case hasInput && hasFlags:
		return request, fmt.Errorf("--input cannot be used with --commit-message or --file: pass the selection one way")
	case hasInput:
		document, err := readSourceControlPushDocument(opts, f.input)
		if err != nil {
			return request, err
		}
		request = document
		if forceSet {
			request.Force = f.force
		}
		return request, nil
	default:
		if strings.TrimSpace(f.commitMessage) == "" {
			return request, fmt.Errorf("--commit-message is required: pass a 1 to 1000 character message, or the whole document with --input")
		}
		if len(f.files) == 0 {
			return request, fmt.Errorf("--file is required: pass at least one ID:TYPE from 'n8n source-control status --direction push', or the whole document with --input")
		}
		selectors := make([]n8n.SourceControlFileSelector, 0, len(f.files))
		for _, raw := range f.files {
			selector, err := parseSourceControlFile(raw)
			if err != nil {
				return request, err
			}
			selectors = append(selectors, selector)
		}
		return n8n.PushSourceControlRequest{
			CommitMessage: f.commitMessage,
			FileNames:     selectors,
			Force:         f.force,
		}, nil
	}
}

func parseSourceControlFile(raw string) (n8n.SourceControlFileSelector, error) {
	id, fileType, ok := strings.Cut(raw, ":")
	if !ok {
		return n8n.SourceControlFileSelector{}, fmt.Errorf("--file %q is not ID:TYPE: pass the file ID, a colon, then the object type", raw)
	}
	selector := n8n.SourceControlFileSelector{
		ID:   strings.TrimSpace(id),
		Type: n8n.SourceControlledFileType(strings.TrimSpace(fileType)),
	}
	if selector.ID == "" || string(selector.Type) == "" {
		return n8n.SourceControlFileSelector{}, fmt.Errorf("--file %q is not ID:TYPE: both the file ID and the object type are required", raw)
	}
	return selector, nil
}

func readSourceControlPushDocument(opts Options, path string) (n8n.PushSourceControlRequest, error) {
	var request n8n.PushSourceControlRequest
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return request, fmt.Errorf("open source-control push input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSourceControlDocument+1))
	if err != nil {
		return request, fmt.Errorf("read source-control push JSON: %w", err)
	}
	if len(raw) > maxSourceControlDocument {
		return request, fmt.Errorf("source-control push JSON exceeds %d bytes", maxSourceControlDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("decode source-control push JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return request, fmt.Errorf("decode source-control push JSON: multiple documents are not allowed")
		}
		return request, fmt.Errorf("decode source-control push JSON: %w", err)
	}
	return request, nil
}

func writeSourceControlFiles(opts Options, resolution config.Resolution, heading string, files []n8n.SourceControlledFile, direction string) error {
	conflicts := 0
	for _, file := range files {
		if file.Conflict {
			conflicts++
		}
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%d\n", heading, len(files))
	if direction != "" {
		fmt.Fprintf(tw, "Direction:\t%s\n", direction)
	}
	fmt.Fprintf(tw, "Conflicts:\t%d\n", conflicts)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(files) > 0 {
		fmt.Fprintln(tw, "\nFILE\tID\tNAME\tTYPE\tSTATUS\tLOCATION\tCONFLICT\tUPDATED")
		for _, file := range files {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\n",
				emptyDash(file.File), emptyDash(file.ID), emptyDash(file.Name),
				emptyDash(string(file.Type)), emptyDash(file.Status),
				emptyDash(file.Location), file.Conflict, emptyDash(file.UpdatedAt))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No source-control files were returned. Nothing is waiting in this direction.")
		return nil
	}
	if conflicts > 0 {
		fmt.Fprintf(opts.Streams.Err, "%d of %d files conflict. Resolve them, or retry the mutation with --force to proceed anyway.\n", conflicts, len(files))
	}
	return nil
}

func sourceControlFileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func sourceControlAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsConflict(err) && action == "push":
		return fmt.Errorf("%s rejected the source-control push (409): it includes files with unresolved conflicts, so nothing was pushed; resolve them from 'n8n source-control status --direction push' or retry with --force: %w", resolution.URL, err)
	case n8n.IsConflict(err) && action == "pull":
		return fmt.Errorf("%s rejected the source-control pull (409): uncommitted local changes or merge conflicts block it, so nothing changed; resolve them from 'n8n source-control status --direction pull' or retry with --force to discard local changes: %w", resolution.URL, err)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the source-control %s (400): check the direction, the commit message, the selected file IDs and types, and the auto-publish value: %w", resolution.URL, action, err)
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied the source-control %s (403): the credential needs %s and the instance needs the licensed Source Control feature connected to Git; run %s", resolution.URL, action, sourceControlScope(action), discoverHint(sourceControlResource))
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s does not serve source control (%d): it needs the licensed Source Control feature connected to a Git repository; run %s to see what this instance offers: %w",
			resolution.URL, n8n.StatusCodeOf(err), discoverHint(sourceControlResource), err)
	default:
		return apiError(err, resolution, sourceControlResource)
	}
}

func sourceControlScope(action string) string {
	switch action {
	case "push":
		return "sourceControl:push"
	case "pull":
		return "sourceControl:pull"
	default:
		return "sourceControl:read"
	}
}
