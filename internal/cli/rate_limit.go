package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	rateLimitResource    = "settingsratelimits"
	maxRateLimitDocument = 1 << 20
	rateLimitEnabledFlag = "enabled"
)

func newRateLimitCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rate-limit",
		Short: "Manage Public API rate limits",
		Long: "Read or replace the Public API rate-limit settings: whether limiting is on,\n" +
			"and per-API-key read and write limits across all endpoints (global) and on\n" +
			"one endpoint, each with its own window in seconds. A limit of 0 means no\n" +
			"limit. Requests to these routes count against the limits too.\n\n" +
			"Fields set by environment variables are listed in envManagedFields: they\n" +
			"report their effective value and cannot be changed through the API.\n" +
			"Requires securitySettings:manage and n8n 2.47 or later.",
		Example: "  n8n rate-limit get\n" +
			"  n8n rate-limit get --output json > rate-limits.json\n" +
			"  n8n rate-limit set --input rate-limits.json --global-write-limit 60",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newRateLimitGetCommand(opts),
		newRateLimitSetCommand(opts),
	)
	return cmd
}

type rateLimitGetFlags struct {
	instance instanceFlags
	output   string
}

func newRateLimitGetCommand(opts Options) *cobra.Command {
	var f rateLimitGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get Public API rate-limit settings",
		Long: "Get the effective Public API rate-limit settings and the fields environment\n" +
			"variables manage. --output json is the starting point for 'set --input': it\n" +
			"round-trips, and set drops the read-only envManagedFields before sending.\n\n" +
			"Requires securitySettings:manage and n8n 2.47 or later.",
		Example: "  n8n rate-limit get\n" +
			"  n8n rate-limit get --output json\n" +
			"  n8n rate-limit get --output json > rate-limits.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRateLimitGet(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the complete settings document with envManagedFields)")
	return cmd
}

func runRateLimitGet(ctx context.Context, opts Options, f rateLimitGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	settings, err := client.GetRateLimitSettings(ctx)
	if err != nil {
		return rateLimitAPIError(err, resolution, "read")
	}
	return writeRateLimitSettings(opts, resolution, "Rate-limit settings", settings, f.output)
}

type rateLimitSetFlags struct {
	instance instanceFlags
	input    string
	enabled  bool
	ints     map[string]*int
	yes      bool
	output   string
}

// rateLimitFlagName is the kebab-case flag for a JSON field, e.g.
// globalReadWindowSeconds becomes global-read-window-seconds.
func rateLimitFlagName(field string) string {
	var b strings.Builder
	for _, r := range field {
		if unicode.IsUpper(r) {
			b.WriteByte('-')
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func newRateLimitSetCommand(opts Options) *cobra.Command {
	f := rateLimitSetFlags{ints: map[string]*int{}}
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace Public API rate-limit settings",
		Long: "Fully replace the Public API rate-limit settings. This is a PUT, not a patch:\n" +
			"enabled and all eight limit and window fields are required. Take them from\n" +
			"--input (start from 'n8n rate-limit get --output json'), from flags, or both;\n" +
			"a flag overrides the --input value. Limits are request counts per API key,\n" +
			"at least 0 (0 for no limit); windows are seconds, at least 1. When limiting\n" +
			"is enabled, at least one limit must be above 0.\n\n" +
			"envManagedFields in --input is read-only: it is dropped before sending and\n" +
			"reported on stderr. Fields set by environment variables can be repeated with\n" +
			"their current value but not changed; a different value answers 409 and\n" +
			"nothing changes.\n\n" +
			"New limits apply at once to every API key, including the one this CLI uses,\n" +
			"so interactive runs ask for confirmation and non-interactive runs require\n" +
			"--yes. --input - also requires --yes because stdin cannot hold both JSON and\n" +
			"a confirmation answer. Requires securitySettings:manage and n8n 2.47 or later.",
		Example: "  n8n rate-limit get --output json > rate-limits.json\n" +
			"  n8n rate-limit set --input rate-limits.json --global-write-limit 60\n" +
			"  n8n rate-limit set --input - --yes < rate-limits.json\n" +
			"  n8n rate-limit set --enabled=false --global-read-limit 0 --global-read-window-seconds 60 \\\n" +
			"      --global-write-limit 0 --global-write-window-seconds 60 --endpoint-read-limit 0 \\\n" +
			"      --endpoint-read-window-seconds 60 --endpoint-write-limit 0 --endpoint-write-window-seconds 60 --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			changed := map[string]bool{rateLimitEnabledFlag: cmd.Flags().Changed(rateLimitEnabledFlag)}
			for _, field := range n8n.RateLimitIntFields {
				changed[field.Name] = cmd.Flags().Changed(rateLimitFlagName(field.Name))
			}
			return runRateLimitSet(cmd.Context(), opts, f, changed)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full rate-limit settings JSON file path, or - for stdin, e.g. from 'n8n rate-limit get --output json' (default: none; flags supply every field; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.enabled, rateLimitEnabledFlag, false, "turn Public API rate limiting on or off; true or false, overrides enabled in --input (default: the --input value)")
	for _, field := range n8n.RateLimitIntFields {
		value := new(int)
		f.ints[field.Name] = value
		usage := "requests allowed per API key in the window, 0 for no limit"
		if field.Window {
			usage = "window in seconds, at least 1"
		}
		cmd.Flags().IntVar(value, rateLimitFlagName(field.Name), 0, fmt.Sprintf("%s; overrides %s in --input (default: the --input value)", usage, field.Name))
	}
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm replacing the limits, which apply at once to every API key, without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the effective settings returned after replacement)")
	return cmd
}

// rateLimitInput is the --input document: the writable fields plus the
// read-only envManagedFields a GET returns, accepted so the GET output
// round-trips and then dropped.
type rateLimitInput struct {
	n8n.UpdateRateLimitSettingsRequest
	EnvManagedFields json.RawMessage `json:"envManagedFields"`
}

func runRateLimitSet(ctx context.Context, opts Options, f rateLimitSetFlags, changed map[string]bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the replacement confirmation: pass --yes after reviewing the settings")
	}
	var input rateLimitInput
	if f.input != "" {
		if err := readRateLimitDocument(opts, f.input, &input); err != nil {
			return err
		}
		if input.EnvManagedFields != nil {
			fmt.Fprintln(opts.Streams.Err, "Dropped read-only field envManagedFields.")
		}
	}
	request := input.UpdateRateLimitSettingsRequest
	if changed[rateLimitEnabledFlag] {
		request.Enabled = &f.enabled
	}
	for _, field := range n8n.RateLimitIntFields {
		if changed[field.Name] {
			*request.Int(field.Name) = f.ints[field.Name]
		}
	}
	if err := rateLimitMissingField(request, f.input); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Fully replace the Public API rate-limit settings on %s? The new limits apply at once to every API key, including the one this CLI uses.", resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	settings, err := client.UpdateRateLimitSettings(ctx, request)
	if err != nil {
		return rateLimitAPIError(err, resolution, "update")
	}
	return writeRateLimitSettings(opts, resolution, "Updated rate-limit settings", settings, f.output)
}

// rateLimitMissingField names the first required field neither --input nor a
// flag supplied, with the flag that fixes it.
func rateLimitMissingField(request n8n.UpdateRateLimitSettingsRequest, input string) error {
	missing := ""
	if request.Enabled == nil {
		missing = rateLimitEnabledFlag
	} else {
		for _, field := range n8n.RateLimitIntFields {
			if *request.Int(field.Name) == nil {
				missing = field.Name
				break
			}
		}
	}
	if missing == "" {
		return nil
	}
	source := "no --input was given"
	if input != "" {
		source = "--input omits it"
	}
	return fmt.Errorf("%s is required and %s: pass --%s, or start from 'n8n rate-limit get --output json' and pass it with --input", missing, source, rateLimitFlagName(missing))
}

func readRateLimitDocument(opts Options, path string, dst *rateLimitInput) error {
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open rate-limit settings input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxRateLimitDocument+1))
	if err != nil {
		return fmt.Errorf("read rate-limit settings JSON: %w", err)
	}
	if len(raw) > maxRateLimitDocument {
		return fmt.Errorf("rate-limit settings JSON exceeds %d bytes", maxRateLimitDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode rate-limit settings JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode rate-limit settings JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode rate-limit settings JSON: %w", err)
	}
	return nil
}

func writeRateLimitSettings(opts Options, resolution config.Resolution, heading string, settings *n8n.RateLimitSettings, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, settings)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	fmt.Fprintf(tw, "Enabled:\t%s\n", yesNo(settings.Enabled))
	managed := "none"
	if len(settings.EnvManagedFields) > 0 {
		managed = strings.Join(settings.EnvManagedFields, ", ")
	}
	fmt.Fprintf(tw, "Env-managed fields:\t%s\n", managed)
	fmt.Fprintln(tw, "\nSCOPE\tACCESS\tLIMIT\tWINDOW")
	rows := []struct {
		scope, access string
		limit, window int
	}{
		{"global", "read", settings.GlobalReadLimit, settings.GlobalReadWindowSeconds},
		{"global", "write", settings.GlobalWriteLimit, settings.GlobalWriteWindowSeconds},
		{"endpoint", "read", settings.EndpointReadLimit, settings.EndpointReadWindowSeconds},
		{"endpoint", "write", settings.EndpointWriteLimit, settings.EndpointWriteWindowSeconds},
	}
	for _, row := range rows {
		limit := "none"
		if row.limit > 0 {
			limit = strconv.Itoa(row.limit)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%ds\n", row.scope, row.access, limit, row.window)
	}
	return tw.Flush()
}

func rateLimitAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "update":
		return fmt.Errorf("%s rejected the full rate-limit replacement (400): send enabled and all eight limit and window fields, limits at least 0, windows at least 1, and at least one limit above 0 when enabled: %w", resolution.URL, err)
	case n8n.IsConflict(err) && action == "update":
		return fmt.Errorf("%s refused the rate-limit update (409): a field set by environment variables has a different value, so nothing changed; repeat the current values of the fields 'n8n rate-limit get' lists as env-managed, or change the environment configuration and restart n8n: %w", resolution.URL, err)
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "rate-limit settings "+action, allOf("securitySettings:manage"), "", rateLimitResource)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s does not serve Public API rate-limit settings (404): they need n8n 2.47 or later; run %s to see what this instance offers: %w", resolution.URL, discoverHint(rateLimitResource), err)
	default:
		return apiError(err, resolution, rateLimitResource)
	}
}
