package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/workflowdiff"
)

var errWorkflowDifferent = errors.New("workflow definitions differ")

// errWorkflowNameNotFound reports a name lookup that completed and matched
// nothing, as opposed to one that failed. Only a completed lookup may be read
// as "create it", which is what 'n8n workflow copy' does with it.
var errWorkflowNameNotFound = errors.New("no exact match")

type workflowSide struct {
	context, id, projectID string
}

type workflowDiffFlags struct {
	from, to           workflowSide
	name, output       string
	only               []string
	pins, state, quiet bool
}

func newWorkflowDiffCommand(opts Options) *cobra.Command {
	var f workflowDiffFlags
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare saved workflows across two contexts",
		Long: "Compare saved workflow definitions without modifying either instance.\n\n" +
			"Select both saved contexts explicitly. This command uses each context's saved URL\n" +
			"and credential, ignoring N8N_CONTEXT, N8N_URL and environment credentials.\n" +
			"Pass --from-id and --to-id, or --name to resolve one exact name independently\n" +
			"on each side. Name lookup follows every page (maximum 10,000 results); missing\n" +
			"or ambiguous matches fail. Project filters apply only to name lookup. Requires\n" +
			"workflow:read on both instances, plus workflow:list when using --name.\n\n" +
			"Defaults compare name, description, nodes, connections, settings and nodeGroups.\n" +
			"Runtime staticData and pinned samples are opt-in. Other top-level fields,\n" +
			"including IDs, versions, timestamps, tags, sharing and metadata, are ignored.\n" +
			"Nodes match by shared ID, then unique exact name; IDs and node order are ignored.\n" +
			"Node positions, credential references and other array order remain significant.\n" +
			"Missing/null descriptions and empty collections normalize to empty values;\n" +
			"staticData retains null. Unknown nested properties are compared.\n\n" +
			"Output describes changes from source to target: added means present only on the\n" +
			"target. JSON includes schemaVersion, endpoints, fields, equal, summary and changes\n" +
			"with JSON Pointer paths and before/after values. Normalized nodes are objects\n" +
			"keyed by id:ID for shared IDs, otherwise name:NAME. Changes describe differences,\n" +
			"not an API patch. Text includes a unified diff of the same normalized definitions.\n\n" +
			"Published/archived status is reported separately and does not affect equality.\n" +
			"This compares saved definitions, not necessarily the versions running in production.\n" +
			"Exit codes: 0 equal, 1 different, 2 error, 130 canceled. --quiet suppresses stdout\n" +
			"but preserves diagnostics. Workflow parameters and included samples appear in output.",
		Example: "  n8n workflow diff --from-context staging --from-id AAA --to-context prod --to-id BBB\n" +
			"  n8n workflow diff --name \"Invoice sync\" --from-context staging --to-context prod\n" +
			"  n8n workflow diff --name \"Invoice sync\" --from-context staging --from-project-id DEV --to-context prod --to-project-id PROD --output json\n" +
			"  n8n workflow diff --from-context staging --from-id AAA --to-context prod --to-id BBB --only nodes,connections,settings --quiet",
		Annotations: map[string]string{"diffExitCodes": "true", "cliOnly": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkflowDiff(cmd.Context(), opts, f)
		},
	}
	cmd.Flags().StringVar(&f.from.context, "from-context", "", "source saved context (required; ignores global environment overrides)")
	cmd.Flags().StringVar(&f.to.context, "to-context", "", "target saved context (required; ignores global environment overrides)")
	cmd.Flags().StringVar(&f.from.id, "from-id", "", "source workflow ID (required unless --name is given)")
	cmd.Flags().StringVar(&f.to.id, "to-id", "", "target workflow ID (required unless --name is given)")
	cmd.Flags().StringVar(&f.name, "name", "", "exact workflow name to resolve on both sides; cannot be combined with IDs")
	cmd.Flags().StringVar(&f.from.projectID, "from-project-id", "", "limit source name lookup to this project (requires --name)")
	cmd.Flags().StringVar(&f.to.projectID, "to-project-id", "", "limit target name lookup to this project (requires --name)")
	cmd.Flags().StringSliceVar(&f.only, "only", nil, "compare only these comma-separated definition fields, replacing defaults and inclusion flags")
	cmd.Flags().BoolVar(&f.pins, "include-pinned-data", false, "include pinned sample data in the default comparison fields")
	cmd.Flags().BoolVar(&f.state, "include-static-data", false, "include runtime static data in the default comparison fields")
	cmd.Flags().BoolVar(&f.quiet, "quiet", false, "suppress stdout; report differences through exit code (errors still go to stderr)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (versioned structural comparison)")
	cmd.MarkFlagsMutuallyExclusive("name", "from-id")
	cmd.MarkFlagsMutuallyExclusive("name", "to-id")
	return cmd
}

type workflowEndpoint struct {
	Context         string `json:"context"`
	URL             string `json:"url"`
	WorkflowID      string `json:"workflowId"`
	Name            string `json:"name"`
	VersionID       string `json:"versionId"`
	ActiveVersionID string `json:"activeVersionId"`
	Published       bool   `json:"published"`
	Archived        bool   `json:"archived"`
}

type workflowDiffResult struct {
	SchemaVersion int              `json:"schemaVersion"`
	Mode          string           `json:"mode"`
	From          workflowEndpoint `json:"from"`
	To            workflowEndpoint `json:"to"`
	Fields        []string         `json:"fields"`
	workflowdiff.Result
}

func runWorkflowDiff(ctx context.Context, opts Options, f workflowDiffFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if strings.TrimSpace(f.from.context) == "" || strings.TrimSpace(f.to.context) == "" {
		return fmt.Errorf("--from-context and --to-context are required")
	}
	if f.name != "" {
		if strings.TrimSpace(f.name) == "" || f.from.id != "" || f.to.id != "" {
			return fmt.Errorf("--name must be nonempty and cannot be combined with --from-id or --to-id")
		}
	} else {
		if strings.TrimSpace(f.from.id) == "" || strings.TrimSpace(f.to.id) == "" {
			return fmt.Errorf("pass --name or both --from-id and --to-id")
		}
		if f.from.projectID != "" || f.to.projectID != "" {
			return fmt.Errorf("project filters require --name")
		}
	}
	fields, err := workflowdiff.Fields(f.only, f.pins, f.state)
	if err != nil {
		return err
	}
	// Scope the precedence exception to this command. Ordinary resource commands
	// retain their documented environment-over-context behavior.
	opts.Env = func(string) string { return "" }
	fromClient, fromResolution, err := opts.apiClient(instanceFlags{context: f.from.context})
	if err != nil {
		return fmt.Errorf("source context %q: %w", f.from.context, err)
	}
	toClient, toResolution, err := opts.apiClient(instanceFlags{context: f.to.context})
	if err != nil {
		return fmt.Errorf("target context %q: %w", f.to.context, err)
	}
	excludePins := !slices.Contains(fields, "pinData")
	from, err := readWorkflowSide(ctx, fromClient, fromResolution, f.from, f.name, excludePins)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	to, err := readWorkflowSide(ctx, toClient, toResolution, f.to, f.name, excludePins)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}
	diff, err := workflowdiff.Compare(*from, *to, fields)
	if err != nil {
		return err
	}
	result := workflowDiffResult{
		SchemaVersion: 1, Mode: "saved", Fields: fields, Result: diff,
		From: workflowEndpointFor(fromResolution, from), To: workflowEndpointFor(toResolution, to),
	}
	if !f.quiet {
		if f.output == outputJSON {
			err = writeJSON(opts.Streams.Out, result)
		} else {
			err = writeWorkflowDiff(opts.Streams.Out, result)
		}
		if err != nil {
			return err
		}
	}
	if !diff.Equal {
		return errWorkflowDifferent
	}
	return nil
}

func workflowEndpointFor(r config.Resolution, w *n8n.Workflow) workflowEndpoint {
	return workflowEndpoint{
		Context: r.ContextName, URL: r.URL, WorkflowID: w.ID, Name: w.Name,
		VersionID: w.VersionID, ActiveVersionID: w.ActiveVersionID,
		Published: w.Active, Archived: w.IsArchived,
	}
}

func readWorkflowSide(ctx context.Context, client *n8n.Client, r config.Resolution, side workflowSide, name string, excludePins bool) (*n8n.Workflow, error) {
	id := side.id
	if name != "" {
		var err error
		id, err = resolveWorkflowName(ctx, client, name, side.projectID)
		if err != nil {
			return nil, fmt.Errorf("context %q (%s), workflow %q: %w", r.ContextName, r.URL, name, workflowAPIError(err, r, "list"))
		}
	}
	w, err := client.GetWorkflow(ctx, id, excludePins)
	if err != nil {
		return nil, fmt.Errorf("context %q (%s), workflow ID %q: %w", r.ContextName, r.URL, id, workflowAPIError(err, r, "get"))
	}
	if w.ID != id || (name != "" && w.Name != name) {
		return nil, fmt.Errorf("context %q: workflow identity changed during lookup or read; expected ID %q and name %q, received ID %q and name %q; retry", r.ContextName, id, name, w.ID, w.Name)
	}
	return w, nil
}

// Do not use Collect here: its successful truncation at max cannot establish
// uniqueness. Fail closed on a limit or an incomplete/repeating page stream.
func resolveWorkflowName(ctx context.Context, client *n8n.Client, name, projectID string) (string, error) {
	opts := n8n.ListWorkflowsOptions{Name: name, ProjectID: projectID, ExcludePinnedData: true}
	seen := map[string]bool{}
	matches := map[string]string{}
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		page, err := client.ListWorkflows(ctx, opts)
		if err != nil {
			return "", err
		}
		total += len(page.Data)
		for _, w := range page.Data {
			if w.Name == name {
				if w.ID == "" {
					return "", fmt.Errorf("name lookup returned a workflow without an ID")
				}
				matches[w.ID] = fmt.Sprintf("ID %q, projects %s", w.ID, diffProjects(w, projectID))
			}
		}
		if total > n8n.DefaultCollectLimit || (total == n8n.DefaultCollectLimit && page.HasMore()) {
			return "", fmt.Errorf("name lookup reached the %d-result limit before uniqueness was established; use project filters or explicit IDs", n8n.DefaultCollectLimit)
		}
		if !page.HasMore() {
			break
		}
		if len(page.Data) == 0 || seen[page.NextCursor] {
			return "", fmt.Errorf("incomplete name lookup: pagination is empty or repeats cursor %q", page.NextCursor)
		}
		seen[page.NextCursor] = true
		opts.Cursor = page.NextCursor
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w; check the name, project filter and credential visibility", errWorkflowNameNotFound)
	case 1:
		for id := range matches {
			return id, nil
		}
	}
	var candidates []string
	for _, description := range matches {
		candidates = append(candidates, description)
	}
	slices.Sort(candidates)
	return "", fmt.Errorf("ambiguous name: %s; use --from-id/--to-id or --from-project-id/--to-project-id", strings.Join(candidates, "; "))
}

func diffProjects(w n8n.Workflow, filter string) string {
	var shared []struct {
		ProjectID string `json:"projectId"`
	}
	_ = json.Unmarshal(w.Shared, &shared)
	var ids []string
	for _, entry := range shared {
		if entry.ProjectID != "" {
			ids = append(ids, entry.ProjectID)
		}
	}
	var direct string
	_ = json.Unmarshal(w.Extra["projectId"], &direct)
	if direct != "" {
		ids = append(ids, direct)
	}
	if len(ids) == 0 && filter != "" {
		ids = append(ids, filter)
	}
	if len(ids) == 0 {
		return "(not returned by API)"
	}
	slices.Sort(ids)
	return fmt.Sprintf("%q", slices.Compact(ids))
}

func writeWorkflowDiff(out io.Writer, result workflowDiffResult) error {
	var text strings.Builder
	fmt.Fprintln(&text, "Comparing saved definitions (source -> target)")
	for _, side := range []struct {
		label string
		value workflowEndpoint
	}{{"From", result.From}, {"To", result.To}} {
		v := side.value
		fmt.Fprintf(&text, "%s: context %q, %s, workflow %q (%q)\n", side.label, v.Context, v.URL, v.WorkflowID, v.Name)
		fmt.Fprintf(&text, "  Published: %t; archived: %t; saved version: %q; active version: %q\n", v.Published, v.Archived, v.VersionID, v.ActiveVersionID)
	}
	fmt.Fprintf(&text, "Fields: %s\n", strings.Join(result.Fields, ", "))
	if result.Equal {
		fmt.Fprintln(&text, "Saved definitions are equal for the selected fields.")
	} else {
		s := result.Summary
		fmt.Fprintf(&text, "Nodes: %d added, %d removed, %d changed; connections changed: %t\n", s.NodesAdded, s.NodesRemoved, s.NodesChanged, s.ConnectionsChanged)
		fmt.Fprintf(&text, "Changes: %d\n\n", len(result.Changes))
		text.WriteString(workflowdiff.Unified(result.Result.From, result.Result.To,
			fmt.Sprintf("%q/%q (saved)", result.From.Context, result.From.WorkflowID),
			fmt.Sprintf("%q/%q (saved)", result.To.Context, result.To.WorkflowID)))
	}
	_, err := io.WriteString(out, text.String())
	return err
}
