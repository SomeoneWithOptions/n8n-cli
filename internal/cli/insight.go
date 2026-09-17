package cli

import (
	"context"
	"fmt"
	"net/http"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const insightResource = "insight"

func newInsightCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "insight",
		Short: "Read aggregate execution insights",
		Long: "Read aggregate execution insights for a selected period and optional project.\n" +
			"The summary reports total and failed executions, failure ratio, estimated time\n" +
			"saved in minutes, average run time in milliseconds, and each metric's deviation\n" +
			"from the preceding period. A null deviation means no comparison is available.\n\n" +
			"Use 'summary' with RFC3339 date bounds and --project-id. Omitting dates uses the\n" +
			"server defaults of seven days ago through now. Requires insights:read.",
		Example: "  n8n insight summary\n" +
			"  n8n insight summary --start-date 2026-09-01T00:00:00Z --end-date 2026-09-17T00:00:00Z\n" +
			"  n8n insight summary --project-id PROJECT_ID --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newInsightSummaryCommand(opts))
	return cmd
}

type insightSummaryFlags struct {
	instance  instanceFlags
	startDate string
	endDate   string
	projectID string
	output    string
}

func newInsightSummaryCommand(opts Options) *cobra.Command {
	var f insightSummaryFlags
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show aggregate execution insights",
		Long: "Show total and failed executions, failure ratio, estimated time saved, and\n" +
			"average run time for a selected period. Each metric includes its deviation from\n" +
			"the preceding period; null means no comparison is available. Ratios remain raw\n" +
			"ratios rather than being converted to percentages.\n\n" +
			"--start-date and --end-date accept RFC3339 timestamps and may be used\n" +
			"independently. Omitting them lets the server select seven days ago through now.\n" +
			"--project-id limits the summary to one project; omitting it includes every\n" +
			"accessible project. Requires insights:read. JSON output preserves API field\n" +
			"names, numeric values, units, and null deviations for stable machine use.",
		Example: "  n8n insight summary\n" +
			"  n8n insight summary --start-date 2026-09-01T00:00:00Z\n" +
			"  n8n insight summary --start-date 2026-09-01T00:00:00Z --end-date 2026-09-17T23:59:59Z --project-id PROJECT_ID\n" +
			"  n8n insight summary --project-id PROJECT_ID --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInsightSummary(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.startDate, "start-date", "", "period start as an RFC3339 timestamp (default: server default of seven days ago)")
	cmd.Flags().StringVar(&f.endDate, "end-date", "", "period end as an RFC3339 timestamp (default: server default of now)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "only include this project, from 'n8n project list' (default: every accessible project)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON preserves metric values, units, and null deviations)")
	return cmd
}

func runInsightSummary(ctx context.Context, opts Options, f insightSummaryFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	getOpts := n8n.GetInsightsSummaryOptions{
		StartDate: f.startDate,
		EndDate:   f.endDate,
		ProjectID: f.projectID,
	}
	if err := getOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	summary, err := client.GetInsightsSummary(ctx, getOpts)
	if err != nil {
		return insightAPIError(err, resolution)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, summary)
	}
	return writeInsightsSummary(opts, resolution, summary, f)
}

func writeInsightsSummary(opts Options, resolution config.Resolution, summary *n8n.InsightsSummary, f insightSummaryFlags) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Insights period start:\t%s\n", insightFilterText(f.startDate, "server default (seven days ago)"))
	fmt.Fprintf(tw, "Insights period end:\t%s\n", insightFilterText(f.endDate, "server default (now)"))
	fmt.Fprintf(tw, "Project:\t%s\n", insightFilterText(f.projectID, "all accessible projects"))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintln(tw, "\nMETRIC\tVALUE\tUNIT\tDEVIATION")
	writeInsightMetric(tw, "Total executions", summary.Total)
	writeInsightMetric(tw, "Failed executions", summary.Failed)
	writeInsightMetric(tw, "Failure rate", summary.FailureRate)
	writeInsightMetric(tw, "Time saved", summary.TimeSaved)
	writeInsightMetric(tw, "Average run time", summary.AverageRunTime)
	return tw.Flush()
}

func writeInsightMetric(tw *tabwriter.Writer, name string, metric n8n.InsightMetric) {
	deviation := "-"
	if metric.Deviation != nil {
		deviation = fmt.Sprintf("%g", *metric.Deviation)
	}
	fmt.Fprintf(tw, "%s\t%g\t%s\t%s\n", name, metric.Value, metric.Unit, deviation)
}

func insightFilterText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func insightAPIError(err error, resolution config.Resolution) error {
	switch {
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the insight filters (400): check the RFC3339 date range and project ID", resolution.URL)
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied insight summary access (403): the credential needs insights:read and access to the selected project; run %s", resolution.URL, discoverHint(insightResource))
	default:
		return apiError(err, resolution, insightResource)
	}
}
