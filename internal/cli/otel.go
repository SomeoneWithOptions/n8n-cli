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
	otelResource    = "settingsotel"
	maxOtelDocument = 1 << 20
)

func newOtelCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "otel",
		Short: "Manage OpenTelemetry tracing settings",
		Long: "Read, fully replace, and test the instance OpenTelemetry tracing configuration.\n" +
			"Start with 'get --output json', edit the complete document, then use 'set'.\n" +
			"Exporter headers may contain collector credentials: text output only reports\n" +
			"whether they are configured, while JSON includes them for the edit workflow.\n\n" +
			"Set applies a successful replacement to the running instance immediately and\n" +
			"therefore requires confirmation. Test-trace sends one span without changing\n" +
			"stored settings. Every action requires otel:manage.",
		Example: "  n8n otel get\n" +
			"  n8n otel get --output json > otel.json\n" +
			"  n8n otel set --input otel.json\n" +
			"  n8n otel test-trace --input collector.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newOtelGetCommand(opts),
		newOtelSetCommand(opts),
		newOtelTestTraceCommand(opts),
	)
	return cmd
}

type otelGetFlags struct {
	instance instanceFlags
	output   string
}

func newOtelGetCommand(opts Options) *cobra.Command {
	var f otelGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get effective OpenTelemetry settings",
		Long: "Get every effective OpenTelemetry setting exposed by n8n. Omitted protocol\n" +
			"values are reported as the server default, http/protobuf. Use JSON as the\n" +
			"starting point for a GET-edit-PUT workflow.\n\n" +
			"Text output never prints exporter headers. JSON output includes them because a\n" +
			"full replacement requires exporterHeaders; protect redirected files as secrets.\n" +
			"Requires otel:manage.",
		Example: "  n8n otel get\n" +
			"  n8n otel get --output json\n" +
			"  umask 077 && n8n otel get --output json > otel.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOtelGet(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON includes possibly sensitive exporter headers)")
	return cmd
}

func runOtelGet(ctx context.Context, opts Options, f otelGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	settings, err := client.GetOtelSettings(ctx)
	if err != nil {
		return otelAPIError(err, resolution, "read")
	}
	return writeOtelSettings(opts, resolution, "OpenTelemetry settings", settings, f.output, false)
}

type otelSetFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newOtelSetCommand(opts Options) *cobra.Command {
	var f otelSetFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace OpenTelemetry settings",
		Long: "Fully replace the instance OpenTelemetry configuration from strict JSON. This\n" +
			"is a PUT, not a patch: every field is required except exporterProtocol. Omitting\n" +
			"that field selects http/protobuf. Start from 'n8n otel get --output json'.\n\n" +
			"A successful update is applied to the running instance immediately and can\n" +
			"change or stop exported traces, so interactive runs ask for confirmation and\n" +
			"non-interactive runs require --yes. --input - also requires --yes because stdin\n" +
			"cannot hold both JSON and a confirmation answer.\n\n" +
			"Environment-managed fields are read-only. Re-submit their effective GET values\n" +
			"unchanged, or change the environment configuration. Exporter headers may be\n" +
			"credentials; response details are redacted on errors. Requires otel:manage.",
		Example: "  umask 077 && n8n otel get --output json > otel.json\n" +
			"  n8n otel set --input otel.json\n" +
			"  n8n otel set --input otel.json --yes --output json\n" +
			"  n8n otel set --input - --yes < otel.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOtelSet(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full OpenTelemetry settings JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm full replacement and immediate runtime application without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON includes possibly sensitive exporter headers)")
	return cmd
}

func runOtelSet(ctx context.Context, opts Options, f otelSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the replacement confirmation: pass --yes after reviewing the full configuration")
	}
	var request n8n.UpdateOtelSettingsRequest
	if err := readOtelDocument(opts, f.input, &request, "settings"); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Fully replace OpenTelemetry settings on %s? A successful update applies to the running instance immediately and may change or stop exported traces.", resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	settings, err := client.UpdateOtelSettings(ctx, request)
	if err != nil {
		return otelAPIError(err, resolution, "update")
	}
	return writeOtelSettings(opts, resolution, "Updated OpenTelemetry settings", settings, f.output, true)
}

type otelTestTraceFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func newOtelTestTraceCommand(opts Options) *cobra.Command {
	var f otelTestTraceFlags
	cmd := &cobra.Command{
		Use:   "test-trace",
		Short: "Send one test span to an OTLP collector",
		Long: "Send one test span using collector connection details from strict JSON. This\n" +
			"does not change stored OpenTelemetry settings and needs no confirmation. Required\n" +
			"fields are exporterEndpoint, exporterTracingPath, exporterServiceName,\n" +
			"exporterHeaders, and startupConnectivityTimeoutMs. exporterProtocol is optional\n" +
			"and defaults to http/protobuf.\n\n" +
			"Environment-managed fields override supplied values before n8n sends the span.\n" +
			"A result with success=false is still a successful API call and is printed for\n" +
			"inspection. Exporter headers may be credentials and are never printed. Requires\n" +
			"otel:manage.",
		Example: "  n8n otel test-trace --input collector.json\n" +
			"  n8n otel test-trace --input collector.json --output json\n" +
			"  n8n otel test-trace --input - < collector.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOtelTestTrace(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "collector connection JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (request connection details are never printed)")
	return cmd
}

func runOtelTestTrace(ctx context.Context, opts Options, f otelTestTraceFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.OtelTestTraceRequest
	if err := readOtelDocument(opts, f.input, &request, "test-trace request"); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	result, err := client.TestOtelTrace(ctx, request)
	if err != nil {
		return otelAPIError(err, resolution, "test")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Test span accepted:\t%s\n", yesNo(result.Success))
	if result.Error != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", result.Error)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

func readOtelDocument(opts Options, path string, dst any, kind string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass an OpenTelemetry %s JSON file, or --input - for stdin", kind)
	}
	reader := opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open OpenTelemetry %s input %q: %w", kind, path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxOtelDocument+1))
	if err != nil {
		return fmt.Errorf("read OpenTelemetry %s JSON: %w", kind, err)
	}
	if len(raw) > maxOtelDocument {
		return fmt.Errorf("OpenTelemetry %s JSON exceeds %d bytes", kind, maxOtelDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode OpenTelemetry %s JSON: %w", kind, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode OpenTelemetry %s JSON: multiple documents are not allowed", kind)
		}
		return fmt.Errorf("decode OpenTelemetry %s JSON: %w", kind, err)
	}
	return nil
}

func writeOtelSettings(opts Options, resolution config.Resolution, heading string, settings *n8n.OtelSettings, output string, applied bool) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, settings)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	fmt.Fprintf(tw, "Enabled:\t%s\n", yesNo(settings.Enabled))
	fmt.Fprintf(tw, "Exporter protocol:\t%s\n", settings.ExporterProtocol)
	fmt.Fprintf(tw, "Exporter endpoint:\t%s\n", settings.ExporterEndpoint)
	fmt.Fprintf(tw, "Exporter tracing path:\t%s\n", settings.ExporterTracingPath)
	fmt.Fprintf(tw, "Exporter service name:\t%s\n", settings.ExporterServiceName)
	fmt.Fprintf(tw, "Exporter headers:\t%s\n", otelHeadersStatus(settings.ExporterHeaders))
	fmt.Fprintf(tw, "Trace sample rate:\t%g\n", settings.TracesSampleRate)
	fmt.Fprintf(tw, "Startup connectivity timeout:\t%d ms\n", settings.StartupConnectivityTimeoutMs)
	fmt.Fprintf(tw, "Include node spans:\t%s\n", yesNo(settings.IncludeNodeSpans))
	fmt.Fprintf(tw, "Inject outbound trace context:\t%s\n", yesNo(settings.InjectOutbound))
	fmt.Fprintf(tw, "Production executions only:\t%s\n", yesNo(settings.ProductionExecutionsOnly))
	if applied {
		fmt.Fprintln(tw, "Applied immediately to running instance:\tyes")
	}
	return tw.Flush()
}

func otelHeadersStatus(headers string) string {
	if headers == "" {
		return "not configured"
	}
	return "configured (value hidden)"
}

func otelAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "update":
		return fmt.Errorf("%s rejected the full OpenTelemetry replacement (400): include every field except optional exporterProtocol, which defaults to http/protobuf; collector response details were redacted", resolution.URL)
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "test":
		return fmt.Errorf("%s rejected the OpenTelemetry test connection details (400): check the endpoint, protocol, service name, headers, and timeout; collector response details were redacted", resolution.URL)
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "OpenTelemetry settings "+action, allOf("otel:manage"), "", otelResource)
	case n8n.IsConflict(err) && action == "update":
		return fmt.Errorf("%s refused the OpenTelemetry replacement (409): one or more changed fields are managed by environment variables, so no changes were made; run 'n8n otel get', re-submit environment-managed values unchanged, or change the environment configuration and restart n8n", resolution.URL)
	default:
		return apiError(err, resolution, otelResource)
	}
}
