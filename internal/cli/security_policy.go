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
	securityPolicyResource    = "security-policy"
	maxSecurityPolicyDocument = 1 << 20
)

func newSecurityPolicyCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security-policy",
		Short: "Manage instance security policy",
		Long: "Read or replace the instance security policy for personal-space publishing and\n" +
			"sharing plus the execution-data redaction floor. Start with 'get' to inspect\n" +
			"current values and read-only usage counts, edit that JSON, then pass the complete\n" +
			"document to 'set'.\n\n" +
			"This licensed Personal Space Policy feature requires securitySettings:manage.\n" +
			"Environment-managed policy can be read but cannot be changed through the API.",
		Example: "  n8n security-policy get\n" +
			"  n8n security-policy get --output json > security-policy.json\n" +
			"  n8n security-policy set --input security-policy.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newSecurityPolicyGetCommand(opts),
		newSecurityPolicySetCommand(opts),
	)
	return cmd
}

type securityPolicyGetFlags struct {
	instance instanceFlags
	output   string
}

func newSecurityPolicyGetCommand(opts Options) *cobra.Command {
	var f securityPolicyGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get effective security policy",
		Long: "Get the effective instance security policy and the read-only counts of published\n" +
			"and shared personal resources affected by it. Use --output json as the starting\n" +
			"point for a read-edit-set workflow; 'set' accepts the returned usage fields but\n" +
			"the server ignores them on write.\n\n" +
			"Requires securitySettings:manage and the licensed Personal Space Policy feature.\n" +
			"A policy managed by environment variables remains readable.",
		Example: "  n8n security-policy get\n" +
			"  n8n security-policy get --output json\n" +
			"  n8n security-policy get --output json > security-policy.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSecurityPolicyGet(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the complete policy plus read-only usage counts)")
	return cmd
}

func runSecurityPolicyGet(ctx context.Context, opts Options, f securityPolicyGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	policy, err := client.GetSecurityPolicy(ctx)
	if err != nil {
		return securityPolicyAPIError(err, resolution, "read")
	}
	return writeSecurityPolicy(opts, resolution, "Security policy", policy, f.output)
}

type securityPolicySetFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func newSecurityPolicySetCommand(opts Options) *cobra.Command {
	var f securityPolicySetFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace security policy",
		Long: "Fully replace every writable field in the instance security policy from strict\n" +
			"JSON supplied by --input. This is a PUT, not a patch: personalSpacePublishing,\n" +
			"personalSpaceSharing, and redactionEnforcement.floor are all required, even when\n" +
			"unchanged. floor must be off, production, or all.\n\n" +
			"Start from 'n8n security-policy get --output json'. Read-only usage counts in that\n" +
			"document are accepted and ignored by the server. Requires securitySettings:manage\n" +
			"and the licensed Personal Space Policy feature. Environment-managed policy is\n" +
			"rejected with 409 and no changes are made.",
		Example: "  n8n security-policy get --output json > security-policy.json\n" +
			"  n8n security-policy set --input security-policy.json\n" +
			"  n8n security-policy set --input - < security-policy.json\n" +
			"  n8n security-policy set --input security-policy.json --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSecurityPolicySet(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full security policy JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the effective policy returned after replacement)")
	return cmd
}

func runSecurityPolicySet(ctx context.Context, opts Options, f securityPolicySetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.UpdateSecurityPolicyRequest
	if err := readSecurityPolicyDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	policy, err := client.UpdateSecurityPolicy(ctx, request)
	if err != nil {
		return securityPolicyAPIError(err, resolution, "update")
	}
	return writeSecurityPolicy(opts, resolution, "Updated security policy", policy, f.output)
}

func readSecurityPolicyDocument(opts Options, path string, dst *n8n.UpdateSecurityPolicyRequest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: start from 'n8n security-policy get --output json' and pass the edited file, or --input - for stdin")
	}
	var reader io.Reader = opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open security policy input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSecurityPolicyDocument+1))
	if err != nil {
		return fmt.Errorf("read security policy JSON: %w", err)
	}
	if len(raw) > maxSecurityPolicyDocument {
		return fmt.Errorf("security policy JSON exceeds %d bytes", maxSecurityPolicyDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode security policy JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode security policy JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode security policy JSON: %w", err)
	}
	return nil
}

func writeSecurityPolicy(opts Options, resolution config.Resolution, heading string, policy *n8n.SecurityPolicy, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, policy)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	fmt.Fprintf(tw, "Personal-space publishing:\t%s\n", yesNo(policy.PersonalSpacePublishing))
	fmt.Fprintf(tw, "Personal-space sharing:\t%s\n", yesNo(policy.PersonalSpaceSharing))
	fmt.Fprintf(tw, "Redaction enforcement floor:\t%s\n", policy.RedactionEnforcement.Floor)
	fmt.Fprintf(tw, "Published personal workflows:\t%d\n", policy.PublishedPersonalWorkflowsCount)
	fmt.Fprintf(tw, "Shared personal workflows:\t%d\n", policy.SharedPersonalWorkflowsCount)
	fmt.Fprintf(tw, "Shared personal credentials:\t%d\n", policy.SharedPersonalCredentialsCount)
	return tw.Flush()
}

func securityPolicyAPIError(err error, resolution config.Resolution, action string) error {
	switch {
	case n8n.StatusCodeOf(err) == http.StatusBadRequest && action == "update":
		return fmt.Errorf("%s rejected the full security policy replacement (400): include personalSpacePublishing, personalSpaceSharing, and redactionEnforcement.floor set to off, production, or all", resolution.URL)
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s denied security policy %s (403): the credential needs securitySettings:manage and the instance needs the Personal Space Policy license; run %s", resolution.URL, action, discoverHint(securityPolicyResource))
	case n8n.IsConflict(err) && action == "update":
		return fmt.Errorf("%s refused the security policy update (409): policy is managed by environment variables, so no changes were made; change the environment configuration and restart n8n, then run 'n8n security-policy get'", resolution.URL)
	default:
		return apiError(err, resolution, securityPolicyResource)
	}
}
