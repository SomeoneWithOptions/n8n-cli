package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	packageResource    = "n8npackage"
	maxPackageDocument = 1 << 20
)

func newPackageCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Export and import n8n packages (beta)",
		Long: "Move workflows, folders, and projects between instances as gzipped tar\n" +
			"packages (.n8np). Start with 'export' to write a package file from this\n" +
			"instance, then 'import' to read one back into a project.\n\n" +
			"The whole group is beta: breaking changes may still occur without a major\n" +
			"version bump. Export needs the licensed Packages feature. Import can need\n" +
			"the Variables and Folders features as well, and it may publish, archive,\n" +
			"hard-delete workflows and executions, create credential stubs, and\n" +
			"overwrite folders, tags, variables, or tables, so it always asks for\n" +
			"confirmation first.",
		Example: "  n8n package export --workflow-id abc123 --out workflows.n8np\n" +
			"  n8n package export --project-id Ox8O54VQrmBrb4qL --out project.n8np --output json\n" +
			"  n8n package import --file workflows.n8np --workflow-conflict-policy new-version --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newPackageExportCommand(opts),
		newPackageImportCommand(opts),
	)
	return cmd
}

type packageExportFlags struct {
	instance              instanceFlags
	workflowIDs           []string
	folderIDs             []string
	projectIDs            []string
	includeVariableValues bool
	includeTags           bool
	includeArchived       bool
	missingPolicy         string
	versionPolicy         string
	credentialPolicy      string
	input                 string
	out                   string
	output                string
}

func newPackageExportCommand(opts Options) *cobra.Command {
	var f packageExportFlags
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export workflows, folders, or projects as an n8n package (beta)",
		Long: "Export workflows and/or folders, or whole projects, as a gzipped tar\n" +
			"archive (.n8np) streamed to a local file. Pass --workflow-id and/or\n" +
			"--folder-id for loose workflows and folders, or --project-id for whole\n" +
			"projects, but not both groups in the same request; at least one ID is\n" +
			"required. Each exported folder includes its nested folders.\n\n" +
			"Beta: breaking changes may still occur without a major version bump.\n" +
			"Requires the licensed Packages feature: workflow and folder exports need\n" +
			"workflow:export, project exports need project:export, and exports that\n" +
			"reference variables with values included also need variable:list.\n" +
			"Statically referenced sub-workflows must be included or the export is\n" +
			"rejected under the default missing-dependency policy. The archive may\n" +
			"bundle variable values and credential expressions, so it is written with\n" +
			"owner-only file permissions and the destination file is overwritten.\n\n" +
			"Pass the selection as flags, or the whole request document with --input\n" +
			"FILE or --input - (capped at 1 MiB, decoded strictly). --out is always a\n" +
			"local path and is never part of --input: a file path streams the archive\n" +
			"there, while --out - writes the raw gzip bytes to stdout and moves the\n" +
			"summary to stderr. Exporting reads instance content and changes nothing\n" +
			"server-side. Follow with 'n8n package import' to restore the archive.",
		Example: "  n8n package export --workflow-id 2tUt1wbLX592XDdX --out workflows.n8np\n" +
			"  n8n package export --workflow-id a --folder-id 9xKp2mNqRzAbCdEf --out mixed.n8np\n" +
			"  n8n package export --project-id Ox8O54VQrmBrb4qL --out project.n8np --output json\n" +
			"  n8n package export --input selection.json --out workflows.n8np",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			changed := map[string]bool{
				"include-variable-values": cmd.Flags().Changed("include-variable-values"),
				"include-tags":            cmd.Flags().Changed("include-tags"),
				"include-archived":        cmd.Flags().Changed("include-archived"),
			}
			return runPackageExport(cmd.Context(), opts, f, changed)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.workflowIDs, "workflow-id", nil, "workflow to include, repeatable up to 300 (cannot combine with --project-id or --input)")
	cmd.Flags().StringArrayVar(&f.folderIDs, "folder-id", nil, "folder to include with nested folders, repeatable up to 300 (cannot combine with --project-id or --input)")
	cmd.Flags().StringArrayVar(&f.projectIDs, "project-id", nil, "project to include, repeatable (cannot combine with --workflow-id, --folder-id, or --input)")
	cmd.Flags().BoolVar(&f.includeVariableValues, "include-variable-values", true, "bundle referenced variable values, else names only (default: true)")
	cmd.Flags().BoolVar(&f.includeTags, "include-tags", true, "bundle workflow tags, else no tag files or references (default: true)")
	cmd.Flags().BoolVar(&f.includeArchived, "include-archived", false, "include archived workflows in folder and project exports; listed workflows always export (default: false)")
	cmd.Flags().StringVar(&f.missingPolicy, "missing-workflow-dependency-policy", "", "missing static sub-workflow handling: fail, reference-only, or include-in-package (default: server default fail)")
	cmd.Flags().StringVar(&f.versionPolicy, "workflow-version-policy", "", "which workflow version travels: published-strict, prefer-published, ignore-unpublished, or latest (default: server default latest)")
	cmd.Flags().StringVar(&f.credentialPolicy, "credential-export-policy", "", "credential data handling: expression-values-only or no-values; literal values never travel (default: server default expression-values-only)")
	cmd.Flags().StringVar(&f.input, "input", "", "export selection as JSON: a file path, or - for stdin (cannot combine with selection or policy flags; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.out, "out", "", "local destination for the .n8np archive: a file path, or - for stdout raw bytes (required)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "summary format: text or json (JSON is the counts, filename, byte size, and local file)")
	return cmd
}

func runPackageExport(ctx context.Context, opts Options, f packageExportFlags, changed map[string]bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(f.out) == "" {
		return fmt.Errorf("--out is required: pass a file path for the .n8np archive, or - for stdout")
	}
	request, err := packageExportRequest(opts, f, changed)
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

	toStdout := strings.TrimSpace(f.out) == "-"
	if toStdout {
		result, err := client.ExportPackage(ctx, request, opts.Streams.Out)
		if err != nil {
			return packageAPIError(err, resolution, "export")
		}
		if f.output == outputJSON {
			return writeJSON(opts.Streams.Err, packageExportSummary{Result: result, File: "-"})
		}
		return writePackageExportText(opts, resolution, result, "-")
	}

	file, err := os.OpenFile(f.out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open package output %q: %w", f.out, err)
	}
	result, exportErr := client.ExportPackage(ctx, request, file)
	closeErr := file.Close()
	if exportErr != nil {
		_ = os.Remove(f.out)
		return packageAPIError(exportErr, resolution, "export")
	}
	if closeErr != nil {
		_ = os.Remove(f.out)
		return fmt.Errorf("write package output %q: %w", f.out, closeErr)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, packageExportSummary{Result: result, File: f.out})
	}
	return writePackageExportText(opts, resolution, result, f.out)
}

// packageExportSummary is the machine-readable export acknowledgement: the
// server counts and attachment name plus the local destination.
type packageExportSummary struct {
	Result *n8n.ExportPackageResult `json:"result"`
	File   string                   `json:"file"`
}

func packageExportRequest(opts Options, f packageExportFlags, changed map[string]bool) (n8n.ExportPackageRequest, error) {
	var request n8n.ExportPackageRequest
	hasInput := strings.TrimSpace(f.input) != ""
	hasFlags := len(f.workflowIDs) > 0 || len(f.folderIDs) > 0 || len(f.projectIDs) > 0 ||
		strings.TrimSpace(f.missingPolicy) != "" || strings.TrimSpace(f.versionPolicy) != "" ||
		strings.TrimSpace(f.credentialPolicy) != "" ||
		changed["include-variable-values"] || changed["include-tags"] || changed["include-archived"]
	switch {
	case hasInput && hasFlags:
		return request, fmt.Errorf("--input cannot be used with selection or policy flags: pass the export selection one way")
	case hasInput:
		return readPackageExportDocument(opts, f.input)
	default:
		request = n8n.ExportPackageRequest{
			WorkflowIDs:             f.workflowIDs,
			FolderIDs:               f.folderIDs,
			ProjectIDs:              f.projectIDs,
			MissingDependencyPolicy: strings.TrimSpace(f.missingPolicy),
			WorkflowVersionPolicy:   strings.TrimSpace(f.versionPolicy),
			CredentialExportPolicy:  strings.TrimSpace(f.credentialPolicy),
		}
		if changed["include-variable-values"] {
			request.IncludeVariableValues = n8n.Bool(f.includeVariableValues)
		}
		if changed["include-tags"] {
			request.IncludeTags = n8n.Bool(f.includeTags)
		}
		if changed["include-archived"] {
			request.IncludeArchived = n8n.Bool(f.includeArchived)
		}
		return request, nil
	}
}

func readPackageExportDocument(opts Options, path string) (n8n.ExportPackageRequest, error) {
	var request n8n.ExportPackageRequest
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return request, fmt.Errorf("open package export input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPackageDocument+1))
	if err != nil {
		return request, fmt.Errorf("read package export JSON: %w", err)
	}
	if len(raw) > maxPackageDocument {
		return request, fmt.Errorf("package export JSON exceeds %d bytes", maxPackageDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("decode package export JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return request, fmt.Errorf("decode package export JSON: multiple documents are not allowed")
		}
		return request, fmt.Errorf("decode package export JSON: %w", err)
	}
	return request, nil
}

func writePackageExportText(opts Options, resolution config.Resolution, result *n8n.ExportPackageResult, file string) error {
	writer := io.Writer(opts.Streams.Out)
	if file == "-" {
		// Stdout carries the raw archive; the summary must not corrupt it.
		writer = opts.Streams.Err
	}
	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	if result == nil {
		fmt.Fprintf(tw, "Exported:\t%s\n", emptyDash(file))
		fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
		return tw.Flush()
	}
	fmt.Fprintf(tw, "Exported:\t%s\n", emptyDash(file))
	fmt.Fprintf(tw, "Bytes:\t%d\n", result.Bytes)
	fmt.Fprintf(tw, "Archive name:\t%s\n", emptyDash(result.Filename))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(result.Counts) > 0 {
		fmt.Fprintln(tw, "\nENTITY\tCOUNT")
		for _, entity := range sortedPackageCounts(result.Counts) {
			fmt.Fprintf(tw, "%s\t%d\n", entity, result.Counts[entity])
		}
	}
	return tw.Flush()
}

// sortedPackageCounts orders the export counts so text output is stable.
func sortedPackageCounts(counts map[string]int) []string {
	entities := make([]string, 0, len(counts))
	for entity := range counts {
		entities = append(entities, entity)
	}
	for i := 1; i < len(entities); i++ {
		for j := i; j > 0 && entities[j] < entities[j-1]; j-- {
			entities[j], entities[j-1] = entities[j-1], entities[j]
		}
	}
	return entities
}

type packageImportFlags struct {
	instance           instanceFlags
	file               string
	projectID          string
	folderID           string
	credentialMatching string
	credentialMissing  string
	bindings           string
	workflowConflict   string
	workflowIDPolicy   string
	missingNodeMode    string
	publishingPolicy   string
	projectConflict    string
	folderConflict     string
	deletionPolicy     string
	dataTableMatching  string
	dataTableMissing   string
	dataTableSchema    string
	variableMissing    string
	variableConflict   string
	variableParent     string
	tagMissing         string
	tagConflict        string
	yes                bool
	output             string
}

func newPackageImportCommand(opts Options) *cobra.Command {
	var f packageImportFlags
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import an n8n package into a project (beta)",
		Long: "Import a gzip-compressed tar package (.n8np) into the target project.\n" +
			"Send the archive with --file FILE or --file - for stdin, route it with\n" +
			"--project-id and --folder-id (omit both for the caller's personal project\n" +
			"root), and state --workflow-conflict-policy explicitly: new-version\n" +
			"updates matching workflows, fail rejects the import when any matches, and\n" +
			"skip leaves matching workflows unchanged.\n\n" +
			"Beta: breaking changes may still occur without a major version bump.\n" +
			"This mutates the instance: imported workflows arrive per\n" +
			"--workflow-publishing-policy, new-version follows the package archive\n" +
			"state (archiving needs workflow:delete), folder overwrite can remove\n" +
			"workflows the package does not contain (archive is recoverable,\n" +
			"hard-delete also drops executions permanently, so prefer archive),\n" +
			"missing credentials become empty stubs unless --credential-missing-mode\n" +
			"must-preexist, and folders, tags, variables, or tables may be created or\n" +
			"overwritten. Interactive runs ask for confirmation and non-interactive\n" +
			"runs require --yes; --file - also requires --yes because stdin cannot\n" +
			"answer. Needs workflow:import plus, depending on the package and the\n" +
			"policies, variable, tag, data-table, folder, and delete scopes, and the\n" +
			"Packages, Variables, and Folders features where the contents require\n" +
			"them.",
		Example: "  n8n package import --file workflows.n8np --workflow-conflict-policy new-version --yes\n" +
			"  n8n package import --file project.n8np --workflow-conflict-policy fail --project-id Ox8O54VQrmBrb4qL --yes --output json\n" +
			"  n8n package import --file update.n8np --workflow-conflict-policy skip --workflow-publishing-policy publish-all --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPackageImport(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.file, "file", "", "package archive (.n8np) to import: a file path, or - for stdin (required; stdin requires --yes)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "target project ID, from 'n8n project list' (default: the caller's personal project)")
	cmd.Flags().StringVar(&f.folderID, "folder-id", "", "folder within the target project, from 'n8n folder list PROJECT_ID' (default: the project root)")
	cmd.Flags().StringVar(&f.credentialMatching, "credential-matching-mode", "", "how package credential references match: id-only, name-and-type, or type-only (default: server default id-only)")
	cmd.Flags().StringVar(&f.credentialMissing, "credential-missing-mode", "", "unresolvable credential handling: must-preexist rejects, create-stub creates empty placeholders (default: server default create-stub)")
	cmd.Flags().StringVar(&f.bindings, "bindings", "", "explicit credential bindings as JSON, e.g. '{\"credentials\":{\"pkg-id\":\"target-id\"}}' (default: server default {})")
	cmd.Flags().StringVar(&f.workflowConflict, "workflow-conflict-policy", "", "matching workflow handling, required: new-version, fail, or skip")
	cmd.Flags().StringVar(&f.workflowIDPolicy, "workflow-id-policy", n8n.ImportWorkflowIDSource, "ID for newly created workflows, always sent: source reuses the package ID for promotions, new mints a fresh ID for repeat imports (default: source)")
	cmd.Flags().StringVar(&f.missingNodeMode, "missing-node-type-mode", "", "unknown node type handling: fail rejects before anything is written, import-anyway imports without publishing those workflows (default: server default fail)")
	cmd.Flags().StringVar(&f.publishingPolicy, "workflow-publishing-policy", "", "post-import publishing: preserve-published-state, match-source, publish-all, or unpublish-all (default: server default preserve-published-state)")
	cmd.Flags().StringVar(&f.projectConflict, "project-conflict-policy", "", "existing project handling for project packages: merge, fail, or overwrite (default: server default merge)")
	cmd.Flags().StringVar(&f.folderConflict, "folder-conflict-policy", "", "existing folder handling: merge, fail, or overwrite; empty follows --project-conflict-policy on project packages, merge otherwise (default: follow)")
	cmd.Flags().StringVar(&f.deletionPolicy, "overwrite-deletion-policy", "", "how folder overwrite removes missing workflows: archive (recoverable) or hard-delete (also drops executions; prefer archive) (default: server default archive)")
	cmd.Flags().StringVar(&f.dataTableMatching, "data-table-matching-mode", "", "data-table matching: by-id, the only mode (default: server default by-id)")
	cmd.Flags().StringVar(&f.dataTableMissing, "data-table-missing-mode", "", "missing data-table handling: create, must-preexist, or do-nothing (default: server default create)")
	cmd.Flags().StringVar(&f.dataTableSchema, "data-table-schema-policy", "", "matched table schema strictness: keep-existing or fail (default: server default keep-existing)")
	cmd.Flags().StringVar(&f.variableMissing, "variable-missing-mode", "", "missing variable handling: do-nothing, must-preexist, create-stub, or create-with-value (default: server default create-with-value)")
	cmd.Flags().StringVar(&f.variableConflict, "variable-conflict-policy", "", "differing variable value handling: keep-existing, overwrite (may rewrite globals), or fail (default: server default keep-existing)")
	cmd.Flags().StringVar(&f.variableParent, "variable-parent-policy", "", "where workflow and folder packages create missing variables: project or global; omit for project packages, which reject it (default: import target project)")
	cmd.Flags().StringVar(&f.tagMissing, "tag-missing-mode", "", "missing tag handling: create or do-nothing (default: server default create)")
	cmd.Flags().StringVar(&f.tagConflict, "tag-conflict-policy", "", "conflicted tag handling: skip, fail, or rename (default: server default skip)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm importing, publishing, archiving, deleting, and overwriting without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the full import result)")
	return cmd
}

func runPackageImport(ctx context.Context, opts Options, f packageImportFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(f.file) == "" {
		return fmt.Errorf("--file is required: pass the .n8np package archive, or - for stdin")
	}
	if f.file == "-" && !f.yes {
		return fmt.Errorf("--file - consumes stdin and cannot answer the import confirmation: pass --yes after reviewing the package and policies")
	}
	request := n8n.ImportPackageRequest{
		ProjectID:                strings.TrimSpace(f.projectID),
		FolderID:                 strings.TrimSpace(f.folderID),
		CredentialMatchingMode:   strings.TrimSpace(f.credentialMatching),
		CredentialMissingMode:    strings.TrimSpace(f.credentialMissing),
		Bindings:                 strings.TrimSpace(f.bindings),
		WorkflowConflictPolicy:   strings.TrimSpace(f.workflowConflict),
		WorkflowIDPolicy:         strings.TrimSpace(f.workflowIDPolicy),
		MissingNodeTypeMode:      strings.TrimSpace(f.missingNodeMode),
		WorkflowPublishingPolicy: strings.TrimSpace(f.publishingPolicy),
		ProjectConflictPolicy:    strings.TrimSpace(f.projectConflict),
		FolderConflictPolicy:     strings.TrimSpace(f.folderConflict),
		OverwriteDeletionPolicy:  strings.TrimSpace(f.deletionPolicy),
		DataTableMatchingMode:    strings.TrimSpace(f.dataTableMatching),
		DataTableMissingMode:     strings.TrimSpace(f.dataTableMissing),
		DataTableSchemaConflict:  strings.TrimSpace(f.dataTableSchema),
		VariableMissingMode:      strings.TrimSpace(f.variableMissing),
		VariableConflictPolicy:   strings.TrimSpace(f.variableConflict),
		VariableParentPolicy:     strings.TrimSpace(f.variableParent),
		TagMissingMode:           strings.TrimSpace(f.tagMissing),
		TagConflictPolicy:        strings.TrimSpace(f.tagConflict),
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := opts.confirmer(f.yes).Confirm(packageImportQuestion(f, resolution)); err != nil {
		return err
	}
	archive, filename, cleanup, err := openPackageArchive(opts, f.file)
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := client.ImportPackage(ctx, request, archive, filename)
	if err != nil {
		return packageAPIError(err, resolution, "import")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writePackageImportText(opts, resolution, result)
}

// openPackageArchive opens the archive for streaming. Stdin has no name, so
// it travels under the default filename; files travel under their base name.
func openPackageArchive(opts Options, path string) (io.Reader, string, func(), error) {
	if path == "-" {
		return opts.Streams.In, n8n.DefaultImportPackageFilename, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", func() {}, fmt.Errorf("open package archive %q: %w", path, err)
	}
	return file, filepath.Base(path), func() { _ = file.Close() }, nil
}

func packageImportQuestion(f packageImportFlags, resolution config.Resolution) string {
	target := "the caller's personal project root"
	switch {
	case strings.TrimSpace(f.projectID) != "" && strings.TrimSpace(f.folderID) != "":
		target = fmt.Sprintf("folder %q in project %q", strings.TrimSpace(f.folderID), strings.TrimSpace(f.projectID))
	case strings.TrimSpace(f.projectID) != "":
		target = fmt.Sprintf("project %q", strings.TrimSpace(f.projectID))
	case strings.TrimSpace(f.folderID) != "":
		target = fmt.Sprintf("folder %q", strings.TrimSpace(f.folderID))
	}
	question := fmt.Sprintf("Import package %q into %s on %s? Matching workflows use %q and new workflows take %q IDs.",
		displayPackageName(f.file), target, resolution.URL, conflictOrUnset(f.workflowConflict), idPolicyOrUnset(f.workflowIDPolicy))
	if strings.TrimSpace(f.publishingPolicy) != "" {
		question += fmt.Sprintf(" Publishing uses %q.", strings.TrimSpace(f.publishingPolicy))
	}
	switch {
	case strings.TrimSpace(f.folderConflict) == n8n.ImportFolderOverwrite && strings.TrimSpace(f.deletionPolicy) == n8n.ImportDeletionHardDelete:
		question += " Folder overwrite with hard-delete permanently deletes workflows the package does not contain, with their executions; prefer archive."
	case strings.TrimSpace(f.folderConflict) == n8n.ImportFolderOverwrite || strings.TrimSpace(f.projectConflict) == n8n.ImportProjectOverwrite:
		question += " Overwrite removes workflows the package does not contain (archived by default) and reconciles folders, projects, tags, variables, and tables."
	default:
		question += " Missing credentials become stubs, and folders, tags, variables, or tables may be created or overwritten."
	}
	return question
}

func displayPackageName(path string) string {
	if path == "-" {
		return "stdin"
	}
	return filepath.Base(path)
}

func conflictOrUnset(policy string) string {
	if strings.TrimSpace(policy) == "" {
		return "unset"
	}
	return strings.TrimSpace(policy)
}

func idPolicyOrUnset(policy string) string {
	if strings.TrimSpace(policy) == "" {
		return "unset"
	}
	return strings.TrimSpace(policy)
}

func writePackageImportText(opts Options, resolution config.Resolution, result *n8n.ImportPackageResult) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	created, updated, skipped := 0, 0, 0
	for _, workflow := range result.Workflows {
		switch workflow.Status {
		case "created":
			created++
		case "updated":
			updated++
		default:
			skipped++
		}
	}
	fmt.Fprintf(tw, "Imported:\t%d workflows (%d created, %d updated, %d skipped)\n", len(result.Workflows), created, updated, skipped)
	fmt.Fprintf(tw, "Removed:\t%d workflows, %d folders\n", len(result.RemovedWorkflows), len(result.RemovedFolders))
	fmt.Fprintf(tw, "Shells:\t%d folders, %d projects\n", len(result.Folders), len(result.Projects))
	fmt.Fprintf(tw, "Credentials:\t%d matched, %d stubbed\n", len(result.Credentials.Matched), len(result.Credentials.Stubbed))
	fmt.Fprintf(tw, "Data tables:\t%d matched, %d created\n", result.DataTables.Matched, result.DataTables.Created)
	fmt.Fprintf(tw, "Variables:\t%d matched, %d created, %d stubbed, %d updated, %d missing\n",
		len(result.Variables.Matched), len(result.Variables.Created), len(result.Variables.Stubbed), len(result.Variables.Updated), len(result.Variables.Missing))
	fmt.Fprintf(tw, "Tags:\t%d matched, %d created, %d renamed, %d reconciled, %d skipped\n",
		len(result.Tags.Matched), len(result.Tags.Created), len(result.Tags.Renamed), len(result.Tags.Reconciled), len(result.Tags.Skipped))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(result.Workflows) > 0 {
		fmt.Fprintln(tw, "\nSOURCE WORKFLOW\tLOCAL ID\tNAME\tSTATUS")
		for _, workflow := range result.Workflows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				emptyDash(workflow.SourceWorkflowID), emptyDash(workflow.LocalID),
				emptyDash(workflow.Name), emptyDash(workflow.Status))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(result.Variables.Missing) > 0 {
		fmt.Fprintf(opts.Streams.Err, "%d variable(s) still unresolved: workflows referencing them may fail until the variables exist.\n", len(result.Variables.Missing))
	}
	if len(result.Tags.Skipped) > 0 {
		fmt.Fprintf(opts.Streams.Err, "%d tag(s) dropped from the import: see tags.skipped in --output json.\n", len(result.Tags.Skipped))
	}
	archived := 0
	for _, removed := range result.RemovedWorkflows {
		if removed.Deletion == "archived" {
			archived++
		}
	}
	if archived < len(result.RemovedWorkflows) {
		fmt.Fprintln(opts.Streams.Err, "Some removed workflows were hard-deleted with their executions; this cannot be undone.")
	}
	return nil
}

func packageAPIError(err error, resolution config.Resolution, action string) error {
	if blocked, ok := n8n.AsImportBlocked(err); ok && action == "import" {
		if blocked.StatusCode == http.StatusConflict {
			return fmt.Errorf("%s rejected the package import (409): %d conflict issue(s); nothing matched with fail stays unchanged, so adjust a conflict policy such as --workflow-conflict-policy, --project-conflict-policy, --folder-conflict-policy, --tag-conflict-policy, or --variable-conflict-policy and retry: %w", resolution.URL, len(blocked.Issues), err)
		}
		return fmt.Errorf("%s rejected the package import (422): %d blocking issue(s) such as unresolved credentials or variables, unknown node types, or variable quota; create the missing references, pass --credential-missing-mode, --missing-node-type-mode import-anyway, or a variable mode that creates them, and retry: %w", resolution.URL, len(blocked.Issues), err)
	}
	switch {
	case n8n.IsStatus(err, http.StatusBadRequest):
		if action == "export" {
			return fmt.Errorf("%s rejected the package export (400): check the selected IDs (unknown, mixed project and loose groups, or missing sub-workflows under fail) and the version policy: %w", resolution.URL, err)
		}
		return fmt.Errorf("%s rejected the package import (400): the archive is malformed, the manifest is missing, or a routing or policy field is invalid (bindings JSON, --variable-parent-policy on a project package): %w", resolution.URL, err)
	case n8n.IsForbidden(err):
		if action == "export" {
			return fmt.Errorf("%s denied the package export (403): the credential needs workflow:export, project:export for projects, and variable:list when values are included, and the instance needs the licensed Packages feature; run %s", resolution.URL, discoverHint(packageResource))
		}
		return fmt.Errorf("%s denied the package import (403): the credential needs workflow:import plus the variable, tag, data-table, folder, and delete scopes the package and policies require, and the instance needs the licensed Packages feature (Variables and Folders where the contents require them); run %s", resolution.URL, discoverHint(packageResource))
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s does not serve n8n packages (404): it needs the licensed Packages feature; run %s to see what this instance offers: %w",
			resolution.URL, discoverHint(packageResource), err)
	default:
		return apiError(err, resolution, packageResource)
	}
}
