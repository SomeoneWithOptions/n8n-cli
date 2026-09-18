package cli

import (
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

const maxLogStreamDocument = 1 << 20

func newLogStreamCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use: "log-stream", Short: "Manage licensed log streaming destinations",
		Long: "Stream n8n events to webhook, syslog, or Sentry destinations. Requires the Log\n" +
			"Streaming license and the action's eventBusDestination scope. Start with\n" +
			"'event-type list', then 'destination list', create a destination, and test it.\n\n" +
			"Destination credentials, URLs, headers, query parameters, certificates and HTTP\n" +
			"options are redacted in both text and JSON. Keep original configuration in a\n" +
			"protected file; never submit redacted output unchanged. Environment-managed\n" +
			"destinations can be read but not created, updated, or deleted.",
		Example: "  n8n log-stream event-type list\n  n8n log-stream destination list\n  n8n log-stream destination create --input destination.json\n  n8n log-stream destination test DESTINATION_ID",
		Args:    cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	events := &cobra.Command{
		Use: "event-type", Short: "Inspect streamable event types",
		Long:    "Start with 'list' to find event names for destination subscribedEvents. Group\nprefixes such as n8n.workflow subscribe to every event in that group. Requires\nthe Log Streaming license and eventBusDestination:list.",
		Example: "  n8n log-stream event-type list\n  n8n log-stream event-type list --output json",
		Args:    cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	events.AddCommand(newLogStreamEventListCommand(opts))
	destinations := &cobra.Command{
		Use: "destination", Short: "Create, inspect, replace, test, or delete destinations",
		Long: "Start with 'list', inspect with 'get', then create or fully replace a destination\n" +
			"from a protected JSON file. Use 'test' to send one test message. Update is PUT,\n" +
			"not a patch, and takes effect immediately. Update and delete require confirmation\n" +
			"or --yes. Read output is redacted: restore secrets before a GET-edit-PUT workflow.\n" +
			"All actions require the Log Streaming license and an eventBusDestination scope.",
		Example: "  n8n log-stream destination list\n  n8n log-stream destination get DESTINATION_ID --output json\n  n8n log-stream destination update DESTINATION_ID --input destination.json --yes\n  n8n log-stream destination test DESTINATION_ID",
		Args:    cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	for _, action := range []string{"list", "create", "get", "update", "test", "delete"} {
		destinations.AddCommand(newLogStreamDestinationCommand(opts, action))
	}
	cmd.AddCommand(events, destinations)
	return cmd
}

type logStreamFlags struct {
	instance      instanceFlags
	input, output string
	yes           bool
}

func newLogStreamEventListCommand(opts Options) *cobra.Command {
	var f logStreamFlags
	cmd := &cobra.Command{
		Use: "list", Short: "List event names available for streaming",
		Long:    "List all streamable event names without pagination. Use these names or group\nprefixes in destination subscribedEvents. Text prints one name per line; JSON\nreturns a data array. Requires the Log Streaming license and eventBusDestination:list.",
		Example: "  n8n log-stream event-type list\n  n8n log-stream event-type list --output json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutput(f.output); err != nil {
				return err
			}
			client, resolution, err := opts.apiClient(f.instance)
			if err != nil {
				return err
			}
			result, err := client.GetLogStreamEventTypes(cmd.Context())
			if err != nil {
				return logStreamAPIError(err, resolution, "list")
			}
			if f.output == outputJSON {
				return writeJSON(opts.Streams.Out, result)
			}
			if len(result.Data) == 0 {
				_, err := fmt.Fprintln(opts.Streams.Out, "No streamable event types.")
				return err
			}
			for _, name := range result.Data {
				if _, err := fmt.Fprintln(opts.Streams.Out, name); err != nil {
					return err
				}
			}
			return nil
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text)")
	return cmd
}

func newLogStreamDestinationCommand(opts Options, action string) *cobra.Command {
	var f logStreamFlags
	shorts := map[string]string{
		"list": "List configured log streaming destinations", "get": "Get a log streaming destination",
		"create": "Create a log streaming destination", "update": "Fully replace a log streaming destination",
		"test": "Send one test message to a destination", "delete": "Delete a log streaming destination",
	}
	details := map[string]string{
		"list":   "List every configured destination without pagination. Text shows a summary table;\nJSON returns a data array. Use 'get ID' to inspect one destination.",
		"get":    "Get a destination by ID. Text shows a summary; JSON shows known configuration\nfields with secrets redacted. For GET-edit-PUT, restore redacted fields from a\nprotected source and review the complete document before 'update'.",
		"create": "Create a webhook, syslog, or Sentry destination from a JSON object. The destination\ntakes effect immediately when enabled. Use 'test ID' afterwards to verify delivery.",
		"update": "Fully replace a destination by ID using PUT, not a patch. Omitted optional fields\nmay reset to server defaults. Start from 'get ID --output json', restore redacted\nsecrets from a protected source, then edit the complete document. The read-only id\nis stripped from input. Replacement applies immediately and may stop delivery.\nInteractive runs ask for confirmation; non-interactive runs require --yes.\n--input - requires --yes because stdin cannot also answer confirmation.",
		"test":   "Send one test message to the stored destination. This contacts the external\nreceiver but changes no stored configuration. No input body or confirmation is\nneeded. Text reports delivery; JSON returns success. success=false is a completed\nAPI call, not a CLI error. Check receiver configuration before testing again.",
		"delete": "Permanently remove this destination and stop future event delivery to it. Events\nalready delivered to the external receiver are not removed. Interactive runs ask\nfor confirmation; non-interactive runs require --yes. Use 'list' to verify removal.",
	}
	scope := action
	if action == "get" {
		scope = "read"
	}
	use, example := action, "  n8n log-stream destination "+action
	args := cobra.NoArgs
	if action != "list" && action != "create" {
		use += " ID"
		example += " DESTINATION_ID"
		args = cobra.ExactArgs(1)
	}
	if action == "create" || action == "update" {
		example += " --input destination.json"
	}
	long := details[action] + "\n\nRequires the Log Streaming license and eventBusDestination:" + scope + ".\nCredentials, connection URLs, headers, query parameters, certificates and HTTP\noptions are redacted in all output; unknown response fields are omitted.\nEnvironment-managed writes fail with 409 without changing anything."
	if action == "create" || action == "update" {
		long += "\n\nInput: type=webhook requires url; type=syslog requires host; type=sentry requires\ndsn. Optional common fields: label, enabled, subscribedEvents (names or group\nprefixes), anonymizeAuditMessages, circuitBreaker. Syslog protocol is udp/tcp/tls\nand facility is 0-23. Webhooks support method, sendHeaders, specifyHeaders,\nheaderParameters, jsonHeaders, sendQuery, specifyQuery, queryParameters, jsonQuery,\nand options. Parameter lists use {\"parameters\":[{\"name\":\"Authorization\",\"value\":\"...\"}]}.\nAdditional properties are forwarded as allowed by the API; protect input files\nand never pass credentials as flags. [REDACTED] placeholders are rejected."
	}
	if action == "update" || action == "delete" {
		example += "\n" + example + " --yes --output json"
	} else {
		example += "\n" + example + " --output json"
	}
	if action == "create" {
		example += "\n  printf '%s' '{\"type\":\"syslog\",\"host\":\"syslog.example.com\",\"enabled\":false}' | n8n log-stream destination create --input -"
	}
	cmd := &cobra.Command{
		Use: use, Short: shorts[action], Long: long, Example: example, Args: args,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			return runLogStreamDestination(cmd.Context(), opts, f, action, id)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text; secrets are always redacted)")
	if action == "create" || action == "update" {
		cmd.Flags().StringVar(&f.input, "input", "", "destination JSON file path, or - for stdin (required; maximum 1 MiB)")
	}
	if action == "update" || action == "delete" {
		cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm replacement or deletion without prompting (default false; required when non-interactive)")
	}
	return cmd
}

func runLogStreamDestination(ctx context.Context, opts Options, f logStreamFlags, action, id string) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if action != "list" && action != "create" && strings.TrimSpace(id) == "" {
		return fmt.Errorf("destination ID is required")
	}
	var request n8n.LogStreamDestinationRequest
	if action == "create" || action == "update" {
		if action == "update" && f.input == "-" && !f.yes {
			return fmt.Errorf("--input - consumes stdin: pass --yes after reviewing the full replacement")
		}
		if err := readLogStreamDocument(opts, f.input, &request); err != nil {
			return err
		}
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if action == "update" || action == "delete" {
		question := fmt.Sprintf("Permanently delete destination %q on %s and stop future delivery? Previously delivered events remain at the receiver.", id, resolution.URL)
		if action == "update" {
			question = fmt.Sprintf("Fully replace destination %q on %s? Changes apply immediately and may stop event delivery.", id, resolution.URL)
		}
		if err := opts.confirmer(f.yes).Confirm(question); err != nil {
			return err
		}
	}
	var destination *n8n.LogStreamDestination
	switch action {
	case "list":
		result, err := client.ListLogStreamDestinations(ctx)
		if err != nil {
			return logStreamAPIError(err, resolution, action)
		}
		if f.output == outputJSON {
			return writeJSON(opts.Streams.Out, result)
		}
		return writeLogStreamDestinations(opts, result.Data)
	case "test":
		result, err := client.TestLogStreamDestination(ctx, id)
		if err != nil {
			return logStreamAPIError(err, resolution, action)
		}
		if f.output == outputJSON {
			return writeJSON(opts.Streams.Out, result)
		}
		_, err = fmt.Fprintf(opts.Streams.Out, "Test message delivered: %s\n", yesNo(result.Success))
		return err
	case "get":
		destination, err = client.GetLogStreamDestination(ctx, id)
	case "create":
		destination, err = client.CreateLogStreamDestination(ctx, request)
	case "update":
		destination, err = client.UpdateLogStreamDestination(ctx, id, request)
	case "delete":
		destination, err = client.DeleteLogStreamDestination(ctx, id)
	}
	if err != nil {
		return logStreamAPIError(err, resolution, action)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, destination)
	}
	return writeLogStreamDestinations(opts, []n8n.LogStreamDestination{*destination})
}

func readLogStreamDocument(opts Options, path string, dst *n8n.LogStreamDestinationRequest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass a destination JSON file or --input - for stdin")
	}
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open destination input: %w", err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxLogStreamDocument+1))
	if err != nil {
		return fmt.Errorf("read destination input: %w", err)
	}
	if len(raw) > maxLogStreamDocument {
		return fmt.Errorf("destination JSON exceeds %d bytes", maxLogStreamDocument)
	}
	// Validate the complete document before decoding so syntax errors, unknown
	// property names and trailing documents cannot echo secret input to stderr.
	if !json.Valid(raw) {
		return fmt.Errorf("destination input must contain exactly one valid JSON document (details redacted)")
	}
	return json.Unmarshal(raw, dst)
}

func writeLogStreamDestinations(opts Options, destinations []n8n.LogStreamDestination) error {
	if len(destinations) == 0 {
		_, err := fmt.Fprintln(opts.Streams.Out, "No log streaming destinations.")
		return err
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTYPE\tLABEL\tENABLED\tEVENTS")
	for _, d := range destinations {
		enabled := "unspecified"
		if d.Enabled != nil {
			enabled = yesNo(*d.Enabled)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.ID, d.Type, d.Label, enabled, strings.Join(d.SubscribedEvents, ", "))
	}
	return tw.Flush()
}

func logStreamAPIError(err error, resolution config.Resolution, action string) error {
	scope := action
	if scope == "get" {
		scope = "read"
	}
	switch {
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied log streaming %s (403): requires the Log Streaming license and eventBusDestination:%s; run 'n8n discover' to check available capabilities", resolution.URL, action, scope)
	case n8n.IsConflict(err):
		return fmt.Errorf("%s refused log streaming %s (409): destinations are managed by environment variables; nothing changed. Read with 'n8n log-stream destination list'; change the environment configuration and restart n8n instead", resolution.URL, action)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the destination (400): check type-specific required fields and configuration against 'n8n log-stream destination %s --help'; response details redacted", resolution.URL, action)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s could not find the destination or log streaming endpoint (404): check the ID with 'n8n log-stream destination list' and available capabilities with 'n8n discover'", resolution.URL)
	case n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s has no available log streaming service (503): run 'n8n discover' to check available capabilities, then retry when the service is available", resolution.URL)
	default:
		return apiError(err, resolution, "")
	}
}
