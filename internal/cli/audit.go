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

// auditResource is the `n8n discover` resource key this group belongs to, used
// to point a denied request at the right diagnostic.
const auditResource = "audit"

func newAuditCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Generate security audits of an n8n instance",
		Long: "Generate security audits of an n8n instance.\n\n" +
			"There is one action today: 'n8n audit generate', which asks the instance for\n" +
			"a risk report covering credentials, database, nodes, filesystem and instance\n" +
			"settings. Auditing is read-only, so it is safe to run against production.\n\n" +
			"Typical workflow: 'n8n auth login', then 'n8n audit generate' to see the\n" +
			"findings, then 'n8n audit generate --output json' to feed them to a script.",
		Example: "  n8n audit generate\n" +
			"  n8n audit generate --category credentials --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newAuditGenerateCommand(opts))
	return cmd
}

type auditFlags struct {
	instance      instanceFlags
	categories    []string
	daysAbandoned int
	output        string
}

func newAuditGenerateCommand(opts Options) *cobra.Command {
	var f auditFlags

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a security audit of the instance",
		Long: "Generate a security audit of the instance.\n\n" +
			"The instance inspects its own credentials, database nodes, community and\n" +
			"filesystem nodes, and instance settings, and reports the risks it finds as\n" +
			"one report per category. Nothing is created, changed or deleted: the report\n" +
			"is derived from data that already exists, so this is safe on production.\n\n" +
			"Narrow the audit with --category, repeated or comma-separated; the documented\n" +
			"categories are credentials, database, nodes, filesystem and instance. Use\n" +
			"--days-abandoned to say how long a workflow may go unexecuted before the\n" +
			"credentials and instance reports call it abandoned; omit it to keep the\n" +
			"instance default.\n\n" +
			"A clean audit reports no findings and still exits 0. Text output is a summary\n" +
			"table plus each finding's title, recommendation and locations; --output json\n" +
			"emits the instance's own report document, including the per-risk fields the\n" +
			"summary leaves out, and is the stable form for scripts and agents.\n\n" +
			"Requires the securityAudit:generate scope; check it with\n" +
			"'n8n discover --resource audit'.",
		Example: "  n8n audit generate\n" +
			"  n8n audit generate --category credentials --category nodes\n" +
			"  n8n audit generate --days-abandoned 30 --output json\n" +
			"  n8n audit generate --output json | jq -r '.[\"Nodes Risk Report\"].sections[].title'\n" +
			"  n8n audit generate --context production",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuditGenerate(cmd.Context(), opts, f)
		},
	}

	f.instance.register(cmd)
	cmd.Flags().StringSliceVar(&f.categories, "category", nil,
		"risk category to audit, repeatable or comma-separated: "+strings.Join(n8n.AuditCategories, ", ")+" (default: all categories)")
	cmd.Flags().IntVar(&f.daysAbandoned, "days-abandoned", 0,
		"days without an execution before a workflow counts as abandoned, e.g. 90 (default 0: use the instance setting)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text; json is the instance report verbatim and is stable for scripting)")

	return cmd
}

func runAuditGenerate(ctx context.Context, opts Options, f auditFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}

	auditOpts := n8n.AuditOptions{
		DaysAbandonedWorkflow: f.daysAbandoned,
		Categories:            f.categories,
	}
	// Fail on a bad category before resolving a credential: the mistake is in
	// the invocation, not in the configuration.
	if err := auditOpts.Validate(); err != nil {
		return err
	}

	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}

	audit, err := client.GenerateAudit(ctx, auditOpts)
	if err != nil {
		if n8n.IsForbidden(err) {
			return forbiddenScopeError(err, resolution, "audit generate", allOf("securityAudit:generate"),
				"a 403 can also mean the credential is not an instance owner or admin.", auditResource)
		}
		return apiError(err, resolution, auditResource)
	}

	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, audit)
	}
	return writeAuditText(opts, resolution, audit, f)
}

// writeAuditText renders the human summary: a table of what each report found,
// then every finding with its recommendation and locations. Descriptions and
// the per-risk fields the models do not cover stay in --output json.
func writeAuditText(opts Options, resolution config.Resolution, audit *n8n.Audit, f auditFlags) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)

	contextName := resolution.ContextName
	if contextName == "" {
		contextName = "(none)"
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Context:\t%s\n", contextName)
	if len(f.categories) > 0 {
		fmt.Fprintf(tw, "Categories:\t%s\n", strings.Join(f.categories, ", "))
	}
	if f.daysAbandoned > 0 {
		fmt.Fprintf(tw, "Abandoned after:\t%d days\n", f.daysAbandoned)
	}
	fmt.Fprintf(tw, "Reports:\t%d\n", len(audit.Reports))
	fmt.Fprintf(tw, "Findings:\t%d\n", audit.Findings())
	if err := tw.Flush(); err != nil {
		return err
	}

	names := audit.ReportNames()
	if len(names) == 0 {
		// The instance answered and found nothing. That is a result, not a
		// failure, so the exit code stays 0.
		fmt.Fprintln(opts.Streams.Err, auditEmptyNote(f))
		return nil
	}

	fmt.Fprintln(opts.Streams.Out)
	tw = tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RISK\tREPORT\tFINDINGS\tLOCATIONS")
	for _, name := range names {
		report := audit.Reports[name]
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", riskOrUnknown(report.Risk), name, len(report.Sections), report.Locations())
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	for _, name := range names {
		report := audit.Reports[name]
		fmt.Fprintf(opts.Streams.Out, "\n%s (%s)\n", name, riskOrUnknown(report.Risk))
		for _, section := range report.Sections {
			fmt.Fprintf(opts.Streams.Out, "  %s\n", section.Title)
			if section.Recommendation != "" {
				fmt.Fprintf(opts.Streams.Out, "    Recommendation: %s\n", singleLine(section.Recommendation))
			}
			for _, location := range section.Location {
				fmt.Fprintf(opts.Streams.Out, "    - %s\n", describeLocation(location))
			}
		}
	}

	fmt.Fprintln(opts.Streams.Err, "Full descriptions and per-risk details are in --output json.")
	return nil
}

// riskOrUnknown keeps the table aligned when a report omits its risk field.
func riskOrUnknown(risk string) string {
	if risk == "" {
		return "-"
	}
	return risk
}

// describeLocation renders one finding location in one line. Which fields are
// set depends on the kind, so take the most specific identification available.
func describeLocation(l n8n.RiskLocation) string {
	var parts []string
	switch {
	case l.NodeName != "" && l.WorkflowName != "":
		parts = append(parts, fmt.Sprintf("%s in workflow %s", l.NodeName, l.WorkflowName))
	case l.NodeName != "":
		parts = append(parts, l.NodeName)
	case l.Name != "":
		parts = append(parts, l.Name)
	case l.NodeType != "":
		parts = append(parts, l.NodeType)
	}
	if l.NodeType != "" && (l.NodeName != "" || l.Name != "") {
		parts = append(parts, "("+l.NodeType+")")
	}
	if id := firstNonEmpty(l.WorkflowID, l.ID); id != "" {
		parts = append(parts, "id "+id)
	}
	if l.PackageURL != "" {
		parts = append(parts, l.PackageURL)
	}
	if len(parts) == 0 {
		if l.Kind != "" {
			return l.Kind
		}
		return "(unidentified location)"
	}
	if l.Kind != "" {
		return l.Kind + " " + strings.Join(parts, " ")
	}
	return strings.Join(parts, " ")
}

// singleLine keeps a multi-line recommendation on one output line.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// auditEmptyNote explains an empty report in terms of what was asked for.
func auditEmptyNote(f auditFlags) string {
	if len(f.categories) == 0 {
		return "No risks reported: the instance found nothing in any category."
	}
	return fmt.Sprintf("No risks reported for %s. Run 'n8n audit generate' without --category to audit everything.", strings.Join(f.categories, ", "))
}
