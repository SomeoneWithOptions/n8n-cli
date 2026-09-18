package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

type discoverFlags struct {
	instance  instanceFlags
	resource  string
	operation string
	schemas   bool
	output    string
}

func newDiscoverCommand(opts Options) *cobra.Command {
	var f discoverFlags

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "List the API capabilities available to the current credential",
		Long: "List the API capabilities available to the current credential.\n\n" +
			"Use this to find out what this instance and this credential can actually do\n" +
			"before running a resource command, and to explain a 403: the response holds\n" +
			"the credential's scopes plus every resource, operation and endpoint it may\n" +
			"reach. A resource missing from the output means it is unavailable to you,\n" +
			"which covers an unlicensed feature and an unscoped credential alike.\n\n" +
			"Narrow the output with --resource and --operation. Add --schemas to inline\n" +
			"each endpoint's request body schema, which is enough to build a request\n" +
			"without fetching the full OpenAPI document.\n\n" +
			"Text output is a summary and a resource table; --output json emits scopes,\n" +
			"endpoints and schemas in full and is the stable form for scripts and agents.\n" +
			"This is a read-only diagnostic: it changes nothing on the instance.\n\n" +
			"Prerequisite: 'n8n auth login'. Next step: run the resource command you\n" +
			"found here.",
		Example: "  n8n discover\n" +
			"  n8n discover --resource workflow\n" +
			"  n8n discover --resource workflow --operation create --schemas --output json\n" +
			"  n8n discover --output json | jq -r '.scopes[]'\n" +
			"  n8n discover --context production",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiscover(cmd.Context(), opts, f)
		},
	}

	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.resource, "resource", "", "resource key to filter by, e.g. workflow, credential, datatable (default: all resources)")
	cmd.Flags().StringVar(&f.operation, "operation", "", "operation to filter by, e.g. read, list, create (default: all operations)")
	cmd.Flags().BoolVar(&f.schemas, "schemas", false, "include each endpoint's request body schema (visible with --output json)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text; json is stable for scripting)")

	return cmd
}

func runDiscover(ctx context.Context, opts Options, f discoverFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}

	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}

	discoverOpts := n8n.DiscoverOptions{Resource: f.resource, Operation: f.operation}
	if f.schemas {
		discoverOpts.Include = n8n.DiscoverIncludeSchemas
	}

	discovery, err := client.Discover(ctx, discoverOpts)
	if err != nil {
		return apiError(err, resolution, f.resource)
	}

	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, discovery)
	}
	return writeDiscoveryText(opts, resolution, discovery, f)
}

// writeDiscoveryText renders the human summary. Anything it leaves out is in
// --output json, and the footer says so, so nobody has to guess.
func writeDiscoveryText(opts Options, resolution config.Resolution, d *n8n.Discovery, f discoverFlags) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)

	contextName := resolution.ContextName
	if contextName == "" {
		contextName = "(none)"
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Context:\t%s\n", contextName)
	fmt.Fprintf(tw, "Auth type:\t%s (%s)\n", resolution.AuthType, resolution.Source)
	fmt.Fprintf(tw, "Scopes:\t%d\n", len(d.Scopes))
	fmt.Fprintf(tw, "Resources:\t%d\n", len(d.Resources))
	if d.SpecURL != "" {
		fmt.Fprintf(tw, "OpenAPI spec:\t%s\n", d.SpecURL)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	names := d.ResourceNames()
	if len(names) == 0 {
		fmt.Fprintln(opts.Streams.Err, discoveryEmptyNote(f))
		return nil
	}

	fmt.Fprintln(opts.Streams.Out)
	tw = tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RESOURCE\tENDPOINTS\tOPERATIONS")
	for _, name := range names {
		resource := d.Resources[name]
		operations := append([]string(nil), resource.Operations...)
		slices.Sort(operations)
		fmt.Fprintf(tw, "%s\t%d\t%s\n", name, len(resource.Endpoints), strings.Join(operations, ", "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// One resource means the user already narrowed the question, so answer the
	// obvious follow-up instead of making them re-run with --output json.
	if len(names) == 1 {
		resource := d.Resources[names[0]]
		fmt.Fprintln(opts.Streams.Out)
		tw = tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "METHOD\tPATH\tOPERATION ID")
		for _, e := range resource.Endpoints {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Method, e.Path, e.OperationID)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}

	fmt.Fprintln(opts.Streams.Err, "Scopes and request schemas are in --output json.")
	return nil
}

// discoveryEmptyNote explains an empty capability map in terms of what the user
// asked for.
func discoveryEmptyNote(f discoverFlags) string {
	var filters []string
	if f.resource != "" {
		filters = append(filters, fmt.Sprintf("--resource %s", f.resource))
	}
	if f.operation != "" {
		filters = append(filters, fmt.Sprintf("--operation %s", f.operation))
	}
	if len(filters) == 0 {
		return "This credential can reach no resources. Check its scopes with 'n8n auth status --check'."
	}
	return fmt.Sprintf("No resource matches %s for this credential. Run 'n8n discover' without filters to see what is available.", strings.Join(filters, " "))
}
