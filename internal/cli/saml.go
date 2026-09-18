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
	samlResource    = "saml"
	maxSAMLDocument = 1 << 20
)

func newSAMLCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "saml",
		Short: "Manage licensed SAML SSO settings",
		Long: "Read and fully replace the instance SAML single sign-on configuration. Start\n" +
			"with 'get --output json', leave redacted metadata, private-key, and certificate\n" +
			"placeholders unchanged to preserve stored values, then pass the complete edited\n" +
			"document to 'set'. Read-only entityID and returnUrl values are accepted but not\n" +
			"sent. Stored secret-bearing values are never rendered in plaintext.\n\n" +
			"Every replacement requires confirmation because it can immediately change the\n" +
			"instance login flow. Requires saml:manage and the SAML license. Configuration\n" +
			"managed by environment variables is readable but cannot be replaced via API.",
		Example: "  n8n saml get\n" +
			"  umask 077 && n8n saml get --output json > saml.json\n" +
			"  n8n saml set --input saml.json\n" +
			"  n8n saml set --input saml.json --yes --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newSAMLGetCommand(opts), newSAMLSetCommand(opts))
	return cmd
}

type samlGetFlags struct {
	instance instanceFlags
	output   string
}

func newSAMLGetCommand(opts Options) *cobra.Command {
	var f samlGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get SAML SSO configuration",
		Long: "Get every SAML SSO setting exposed by n8n, including the read-only service\n" +
			"provider entity ID and ACS return URL. Identity-provider metadata, signing\n" +
			"private keys, and signing certificates are empty when unset or replaced by\n" +
			"n8n's redacted placeholder when configured. Keep placeholders unchanged in a\n" +
			"GET-edit-PUT workflow. Text output only reports secret status.\n\n" +
			"Requires saml:manage and the SAML license.",
		Example: "  n8n saml get\n" +
			"  n8n saml get --output json\n" +
			"  umask 077 && n8n saml get --output json > saml.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runSAMLGet(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (secret-bearing values are placeholders, never stored plaintext)")
	return cmd
}

func runSAMLGet(ctx context.Context, opts Options, f samlGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	configuration, err := client.GetSAMLConfiguration(ctx)
	if err != nil {
		return samlAPIError(err, resolution, "read")
	}
	return writeSAMLConfiguration(opts, resolution, "SAML configuration", configuration, f.output)
}

type samlSetFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newSAMLSetCommand(opts Options) *cobra.Command {
	var f samlSetFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace SAML SSO configuration",
		Long: "Fully replace the instance SAML SSO configuration from strict JSON. This is\n" +
			"a PUT, not a patch: all 15 writable top-level fields and every nested mapping\n" +
			"and signature field are required, including false, empty-string, and empty-array\n" +
			"values. Start from 'n8n saml get --output json'. Read-only entityID and returnUrl\n" +
			"are accepted for that round trip but ignored on set. Leave returned redaction\n" +
			"placeholders unchanged to preserve metadata, private key, and certificate; use\n" +
			"empty strings to clear supported values or plaintext to replace them.\n\n" +
			"Every replacement asks for confirmation because changing SAML settings can alter\n" +
			"the login flow immediately. Non-interactive use and --input - require --yes.\n" +
			"Protect files containing replacement metadata, keys, or certificates. Server\n" +
			"error details are redacted. Requires saml:manage and the SAML license.\n" +
			"Environment-managed configuration is rejected with 409.",
		Example: "  umask 077 && n8n saml get --output json > saml.json\n" +
			"  n8n saml set --input saml.json\n" +
			"  n8n saml set --input saml.json --yes --output json\n" +
			"  n8n saml set --input - --yes < saml.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runSAMLSet(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full SAML configuration JSON file path, or - for stdin (required; maximum 1 MiB; may contain private material)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm full replacement and possible login-flow changes without prompting (required for non-interactive use)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (secret-bearing values are returned only as n8n placeholders)")
	return cmd
}

func runSAMLSet(ctx context.Context, opts Options, f samlSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the SAML replacement confirmation: pass --yes after reviewing the complete configuration and login-flow impact")
	}
	var request n8n.SetSAMLConfigurationRequest
	if err := readSAMLConfigurationDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Fully replace SAML configuration on %s? This may change the instance login flow immediately.", resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	configuration, err := client.SetSAMLConfiguration(ctx, request)
	if err != nil {
		return samlAPIError(err, resolution, "update")
	}
	return writeSAMLConfiguration(opts, resolution, "Updated SAML configuration", configuration, f.output)
}

func readSAMLConfigurationDocument(opts Options, path string, dst *n8n.SetSAMLConfigurationRequest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: start from 'n8n saml get --output json' and pass the complete edited file, or --input - for stdin")
	}
	reader := opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open SAML configuration input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSAMLDocument+1))
	if err != nil {
		return fmt.Errorf("read SAML configuration JSON: %w", err)
	}
	if len(raw) > maxSAMLDocument {
		return fmt.Errorf("SAML configuration JSON exceeds %d bytes", maxSAMLDocument)
	}
	if !json.Valid(raw) {
		return fmt.Errorf("SAML configuration input must contain exactly one valid JSON document (details redacted)")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode SAML configuration JSON: %w", err)
	}
	return nil
}

func writeSAMLConfiguration(opts Options, resolution config.Resolution, heading string, configuration *n8n.SAMLConfiguration, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, configuration)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	fmt.Fprintf(tw, "Entity ID:\t%s\n", emptyDash(configuration.EntityID))
	fmt.Fprintf(tw, "ACS return URL:\t%s\n", emptyDash(configuration.ReturnURL))
	fmt.Fprintf(tw, "Mapping email:\t%s\n", emptyDash(configuration.Mapping.Email))
	fmt.Fprintf(tw, "Mapping first name:\t%s\n", emptyDash(configuration.Mapping.FirstName))
	fmt.Fprintf(tw, "Mapping last name:\t%s\n", emptyDash(configuration.Mapping.LastName))
	fmt.Fprintf(tw, "Mapping user principal name:\t%s\n", emptyDash(configuration.Mapping.UserPrincipalName))
	fmt.Fprintf(tw, "Mapping instance role:\t%s\n", emptyDash(configuration.Mapping.N8nInstanceRole))
	fmt.Fprintf(tw, "Mapping project roles:\t%s\n", emptyDash(strings.Join(configuration.Mapping.N8nProjectRoles, ", ")))
	fmt.Fprintf(tw, "Identity provider metadata:\t%s\n", samlSecretStatus(configuration.Metadata))
	fmt.Fprintf(tw, "Metadata URL:\t%s\n", emptyDash(configuration.MetadataURL))
	fmt.Fprintf(tw, "Ignore metadata URL SSL errors:\t%s\n", yesNo(configuration.IgnoreSSL))
	fmt.Fprintf(tw, "Login binding:\t%s\n", configuration.LoginBinding)
	fmt.Fprintf(tw, "Login enabled:\t%s\n", yesNo(configuration.LoginEnabled))
	fmt.Fprintf(tw, "Login label:\t%s\n", emptyDash(configuration.LoginLabel))
	fmt.Fprintf(tw, "Authentication requests signed:\t%s\n", yesNo(configuration.AuthnRequestsSigned))
	fmt.Fprintf(tw, "Signed assertions required:\t%s\n", yesNo(configuration.WantAssertionsSigned))
	fmt.Fprintf(tw, "Signed messages required:\t%s\n", yesNo(configuration.WantMessageSigned))
	fmt.Fprintf(tw, "Signing private key:\t%s\n", samlSecretStatus(configuration.SigningPrivateKey))
	fmt.Fprintf(tw, "Signing certificate:\t%s\n", samlSecretStatus(configuration.SigningCertificate))
	fmt.Fprintf(tw, "ACS binding:\t%s\n", configuration.ACSBinding)
	fmt.Fprintf(tw, "Signature prefix:\t%s\n", emptyDash(configuration.SignatureConfig.Prefix))
	fmt.Fprintf(tw, "Signature location reference:\t%s\n", emptyDash(configuration.SignatureConfig.Location.Reference))
	fmt.Fprintf(tw, "Signature location action:\t%s\n", configuration.SignatureConfig.Location.Action)
	fmt.Fprintf(tw, "Relay state:\t%s\n", emptyDash(configuration.RelayState))
	return tw.Flush()
}

func samlSecretStatus(value string) string {
	if value == "" {
		return "not configured"
	}
	return "configured (value hidden)"
}

func samlAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied SAML configuration %s (403): the credential needs saml:manage and the instance needs the SAML license; run %s", resolution.URL, action, discoverHint(samlResource))
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "update":
		return fmt.Errorf("%s rejected the full SAML replacement (400): include all 15 writable fields and every mapping and signature field, valid redirect/post bindings, and a supported signature action; response details redacted", resolution.URL)
	case n8n.IsConflict(err) && action == "update":
		return fmt.Errorf("%s refused the SAML replacement (409): SAML SSO configuration is managed by environment variables, so no changes were made; change the environment configuration and restart n8n, then run 'n8n saml get'", resolution.URL)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s has no available SAML endpoint (404): verify the SAML feature and run %s", resolution.URL, discoverHint(samlResource))
	case n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s has no available SAML service (503): run %s and retry when the service is available", resolution.URL, discoverHint(samlResource))
	default:
		return apiError(err, resolution, samlResource)
	}
}
