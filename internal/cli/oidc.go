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
	oidcResource    = "settingsssooidc"
	maxOIDCDocument = 1 << 20
)

func newOIDCCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oidc",
		Short: "Manage licensed OIDC SSO settings",
		Long: "Read and fully replace the instance OIDC single sign-on configuration. Start\n" +
			"with 'get --output json', keep the client-secret placeholder unchanged unless\n" +
			"replacing the secret, then pass the complete edited document to 'set'. The\n" +
			"stored client secret is never rendered in plaintext. Protect any file where\n" +
			"you enter a replacement secret.\n\n" +
			"Every replacement requires confirmation because it can immediately change the\n" +
			"instance login flow. Requires oidc:manage and the OIDC license. Configuration\n" +
			"managed by environment variables is readable but cannot be replaced through\n" +
			"the API.",
		Example: "  n8n oidc get\n" +
			"  umask 077 && n8n oidc get --output json > oidc.json\n" +
			"  n8n oidc set --input oidc.json\n" +
			"  n8n oidc set --input oidc.json --yes --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newOIDCGetCommand(opts), newOIDCSetCommand(opts))
	return cmd
}

type oidcGetFlags struct {
	instance instanceFlags
	output   string
}

func newOIDCGetCommand(opts Options) *cobra.Command {
	var f oidcGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get OIDC SSO configuration",
		Long: "Get every OIDC SSO setting exposed by n8n. The client secret is never\n" +
			"returned: clientSecret is empty when unset or n8n's redacted placeholder when\n" +
			"configured. Keep that placeholder unchanged in a GET-edit-PUT workflow to\n" +
			"preserve the stored secret. Text output only reports secret status.\n\n" +
			"Requires oidc:manage and the OIDC license.",
		Example: "  n8n oidc get\n" +
			"  n8n oidc get --output json\n" +
			"  umask 077 && n8n oidc get --output json > oidc.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runOIDCGet(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON contains only a client-secret placeholder, never the stored secret)")
	return cmd
}

func runOIDCGet(ctx context.Context, opts Options, f oidcGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	configuration, err := client.GetOIDCConfiguration(ctx)
	if err != nil {
		return oidcAPIError(err, resolution, "read")
	}
	return writeOIDCConfiguration(opts, resolution, "OIDC configuration", configuration, f.output)
}

type oidcSetFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newOIDCSetCommand(opts Options) *cobra.Command {
	var f oidcSetFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace OIDC SSO configuration",
		Long: "Fully replace the instance OIDC SSO configuration from strict JSON. This is\n" +
			"a PUT, not a patch: all nine fields are required, including false, empty-string,\n" +
			"and empty-array values. Start from 'n8n oidc get --output json'. Leave the\n" +
			"returned client-secret placeholder unchanged to preserve the stored secret, or\n" +
			"replace it to set a new secret. An empty client secret is not accepted.\n\n" +
			"Every replacement asks for confirmation because changing OIDC settings can alter\n" +
			"the login flow immediately. Non-interactive use and --input - require --yes.\n" +
			"Input files containing a replacement secret need secret-safe permissions, and\n" +
			"server response details are redacted on errors. Requires oidc:manage and the\n" +
			"OIDC license. Environment-managed configuration is rejected with 409.",
		Example: "  umask 077 && n8n oidc get --output json > oidc.json\n" +
			"  n8n oidc set --input oidc.json\n" +
			"  n8n oidc set --input oidc.json --yes --output json\n" +
			"  n8n oidc set --input - --yes < oidc.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runOIDCSet(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full OIDC configuration JSON file path, or - for stdin (required; maximum 1 MiB; may contain a client secret)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm full replacement and possible login-flow changes without prompting (required for non-interactive use)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (client secret is returned only as an n8n placeholder)")
	return cmd
}

func runOIDCSet(ctx context.Context, opts Options, f oidcSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the OIDC replacement confirmation: pass --yes after reviewing the complete configuration and login-flow impact")
	}
	var request n8n.SetOIDCConfigurationRequest
	if err := readOIDCConfigurationDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Fully replace OIDC configuration on %s? This may change the instance login flow immediately.", resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	configuration, err := client.SetOIDCConfiguration(ctx, request)
	if err != nil {
		return oidcAPIError(err, resolution, "update")
	}
	return writeOIDCConfiguration(opts, resolution, "Updated OIDC configuration", configuration, f.output)
}

func readOIDCConfigurationDocument(opts Options, path string, dst *n8n.SetOIDCConfigurationRequest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: start from 'n8n oidc get --output json' and pass the complete edited file, or --input - for stdin")
	}
	reader := opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open OIDC configuration input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxOIDCDocument+1))
	if err != nil {
		return fmt.Errorf("read OIDC configuration JSON: %w", err)
	}
	if len(raw) > maxOIDCDocument {
		return fmt.Errorf("OIDC configuration JSON exceeds %d bytes", maxOIDCDocument)
	}
	if !json.Valid(raw) {
		return fmt.Errorf("OIDC configuration input must contain exactly one valid JSON document (details redacted)")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode OIDC configuration JSON: %w", err)
	}
	return nil
}

func writeOIDCConfiguration(opts Options, resolution config.Resolution, heading string, configuration *n8n.OIDCConfiguration, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, configuration)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	fmt.Fprintf(tw, "Client ID:\t%s\n", emptyDash(configuration.ClientID))
	fmt.Fprintf(tw, "Client secret:\t%s\n", oidcClientSecretStatus(configuration.ClientSecret))
	fmt.Fprintf(tw, "Discovery endpoint:\t%s\n", emptyDash(configuration.DiscoveryEndpoint))
	fmt.Fprintf(tw, "Login enabled:\t%s\n", yesNo(configuration.LoginEnabled))
	fmt.Fprintf(tw, "Prompt:\t%s\n", configuration.Prompt)
	fmt.Fprintf(tw, "Authentication context class references:\t%s\n", emptyDash(strings.Join(configuration.AuthenticationContextClassReference, ", ")))
	fmt.Fprintf(tw, "Additional scopes:\t%s\n", emptyDash(configuration.AdditionalScopes))
	fmt.Fprintf(tw, "Verified email required:\t%s\n", yesNo(configuration.EmailVerifiedRequired))
	fmt.Fprintf(tw, "RP-initiated logout enabled:\t%s\n", yesNo(configuration.RPInitiatedLogoutEnabled))
	return tw.Flush()
}

func oidcClientSecretStatus(secret string) string {
	if secret == "" {
		return "not configured"
	}
	return "configured (value hidden)"
}

func oidcAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "OIDC configuration "+action, allOf("oidc:manage"),
			"the instance also needs the OIDC license.", oidcResource)
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "update":
		return fmt.Errorf("%s rejected the full OIDC replacement (400): include all nine fields, a non-empty clientId and clientSecret (or redacted sentinel), a valid discoveryEndpoint, and a supported prompt; response details redacted", resolution.URL)
	case n8n.IsConflict(err) && action == "update":
		return fmt.Errorf("%s refused the OIDC replacement (409): OIDC SSO configuration is managed by environment variables, so no changes were made; change the environment configuration and restart n8n, then run 'n8n oidc get'", resolution.URL)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s has no available OIDC endpoint (404): verify the OIDC feature and run %s", resolution.URL, discoverHint(oidcResource))
	case n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s has no available OIDC service (503): run %s and retry when the service is available", resolution.URL, discoverHint(oidcResource))
	default:
		return apiError(err, resolution, oidcResource)
	}
}
