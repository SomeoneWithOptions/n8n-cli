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
	ldapResource    = "settingsldap"
	maxLDAPDocument = 1 << 20
)

func newLDAPCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ldap",
		Short: "Manage licensed LDAP settings and synchronization",
		Long: "Read and fully replace the instance LDAP configuration, inspect synchronization\n" +
			"history, and start dry or live synchronization runs. Start with 'get --output\n" +
			"json', keep the password placeholder unchanged unless replacing or clearing the\n" +
			"secret, then pass the complete edited document to 'set'. Protect any file where\n" +
			"you enter a replacement bind password.\n\n" +
			"Disabling LDAP login deletes stored LDAP identities and stops synchronization. A\n" +
			"live sync creates, updates, and may disable users. Both actions require explicit\n" +
			"confirmation. Configuration requires ldap:manage; sync actions require ldap:sync;\n" +
			"all commands require the LDAP license.",
		Example: "  n8n ldap get\n" +
			"  umask 077 && n8n ldap get --output json > ldap.json\n" +
			"  n8n ldap set --input ldap.json\n" +
			"  n8n ldap sync history\n" +
			"  n8n ldap sync run --type dry",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newLDAPGetCommand(opts), newLDAPSetCommand(opts), newLDAPSyncCommand(opts))
	return cmd
}

type ldapGetFlags struct {
	instance instanceFlags
	output   string
}

func newLDAPGetCommand(opts Options) *cobra.Command {
	var f ldapGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get LDAP configuration",
		Long: "Get every LDAP setting exposed by n8n. The binding admin password is never\n" +
			"returned: bindingAdminPassword is empty when unset or n8n's blanking placeholder\n" +
			"when configured. Keep that placeholder unchanged in a GET-edit-PUT workflow to\n" +
			"preserve the stored password. Text output only reports password status.\n\n" +
			"Requires ldap:manage and the LDAP license.",
		Example: "  n8n ldap get\n" +
			"  n8n ldap get --output json\n" +
			"  umask 077 && n8n ldap get --output json > ldap.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runLDAPGet(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON contains only a password placeholder, never the stored password)")
	return cmd
}

func runLDAPGet(ctx context.Context, opts Options, f ldapGetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	configuration, err := client.GetLDAPConfiguration(ctx)
	if err != nil {
		return ldapAPIError(err, resolution, "read configuration")
	}
	return writeLDAPConfiguration(opts, resolution, "LDAP configuration", configuration, f.output)
}

type ldapSetFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newLDAPSetCommand(opts Options) *cobra.Command {
	var f ldapSetFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace LDAP configuration",
		Long: "Fully replace the instance LDAP configuration from strict JSON. This is a PUT,\n" +
			"not a patch: all 20 fields are required, including false, zero, and empty values.\n" +
			"Start from 'n8n ldap get --output json'. Leave the returned password placeholder\n" +
			"unchanged to preserve the stored bind password, replace it to set a new password,\n" +
			"or use an empty string to clear it. Input files therefore require secret-safe\n" +
			"permissions. Response details are redacted on errors.\n\n" +
			"Every replacement asks for confirmation. Setting loginEnabled=false is high risk:\n" +
			"n8n deletes every stored LDAP identity and disables synchronization. Non-interactive\n" +
			"use and --input - require --yes. Requires ldap:manage and the LDAP license.",
		Example: "  umask 077 && n8n ldap get --output json > ldap.json\n" +
			"  n8n ldap set --input ldap.json\n" +
			"  n8n ldap set --input ldap.json --yes --output json\n" +
			"  n8n ldap set --input - --yes < ldap.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runLDAPSet(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full LDAP configuration JSON file path, or - for stdin (required; maximum 1 MiB; may contain a bind password)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm full replacement without prompting; also accepts destructive identity deletion when loginEnabled=false (required for non-interactive use)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (password is returned only as an n8n placeholder)")
	return cmd
}

func runLDAPSet(ctx context.Context, opts Options, f ldapSetFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the LDAP replacement confirmation: pass --yes after reviewing the complete configuration and destructive-disable risk")
	}
	var request n8n.UpdateLDAPConfigurationRequest
	if err := readLDAPConfigurationDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Fully replace LDAP configuration on %s?", resolution.URL)
	if !*request.LoginEnabled {
		question += " HIGH RISK: loginEnabled=false permanently deletes all stored LDAP identities and disables synchronization."
	} else {
		question += " This may change LDAP login and synchronization immediately."
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	configuration, err := client.UpdateLDAPConfiguration(ctx, request)
	if err != nil {
		return ldapAPIError(err, resolution, "update configuration")
	}
	return writeLDAPConfiguration(opts, resolution, "Updated LDAP configuration", configuration, f.output)
}

func readLDAPConfigurationDocument(opts Options, path string, dst *n8n.UpdateLDAPConfigurationRequest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: start from 'n8n ldap get --output json' and pass the complete edited file, or --input - for stdin")
	}
	reader := opts.Streams.In
	var file *os.File
	if path != "-" {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("open LDAP configuration input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxLDAPDocument+1))
	if err != nil {
		return fmt.Errorf("read LDAP configuration JSON: %w", err)
	}
	if len(raw) > maxLDAPDocument {
		return fmt.Errorf("LDAP configuration JSON exceeds %d bytes", maxLDAPDocument)
	}
	if !json.Valid(raw) {
		return fmt.Errorf("LDAP configuration input must contain exactly one valid JSON document (details redacted)")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode LDAP configuration JSON: %w", err)
	}
	return nil
}

func writeLDAPConfiguration(opts Options, resolution config.Resolution, heading string, configuration *n8n.LDAPConfiguration, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, configuration)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	fmt.Fprintf(tw, "Login enabled:\t%s\n", yesNo(configuration.LoginEnabled))
	fmt.Fprintf(tw, "Login label:\t%s\n", emptyDash(configuration.LoginLabel))
	fmt.Fprintf(tw, "Connection URL:\t%s\n", emptyDash(configuration.ConnectionURL))
	fmt.Fprintf(tw, "Connection port:\t%d\n", configuration.ConnectionPort)
	fmt.Fprintf(tw, "Connection security:\t%s\n", configuration.ConnectionSecurity)
	fmt.Fprintf(tw, "Allow unauthorized certificates:\t%s\n", yesNo(configuration.AllowUnauthorizedCerts))
	fmt.Fprintf(tw, "Base DN:\t%s\n", emptyDash(configuration.BaseDN))
	fmt.Fprintf(tw, "Binding admin DN:\t%s\n", emptyDash(configuration.BindingAdminDN))
	fmt.Fprintf(tw, "Binding admin password:\t%s\n", ldapPasswordStatus(configuration.BindingAdminPassword))
	fmt.Fprintf(tw, "First-name attribute:\t%s\n", emptyDash(configuration.FirstNameAttribute))
	fmt.Fprintf(tw, "Last-name attribute:\t%s\n", emptyDash(configuration.LastNameAttribute))
	fmt.Fprintf(tw, "Email attribute:\t%s\n", emptyDash(configuration.EmailAttribute))
	fmt.Fprintf(tw, "Login-ID attribute:\t%s\n", emptyDash(configuration.LoginIDAttribute))
	fmt.Fprintf(tw, "LDAP-ID attribute:\t%s\n", emptyDash(configuration.LDAPIDAttribute))
	fmt.Fprintf(tw, "User filter:\t%s\n", emptyDash(configuration.UserFilter))
	fmt.Fprintf(tw, "Synchronization enabled:\t%s\n", yesNo(configuration.SynchronizationEnabled))
	fmt.Fprintf(tw, "Synchronization interval:\t%d minutes\n", configuration.SynchronizationInterval)
	fmt.Fprintf(tw, "Search page size:\t%d\n", configuration.SearchPageSize)
	fmt.Fprintf(tw, "Search timeout:\t%d seconds\n", configuration.SearchTimeout)
	fmt.Fprintf(tw, "Enforce email uniqueness:\t%s\n", yesNo(configuration.EnforceEmailUniqueness))
	return tw.Flush()
}

func ldapPasswordStatus(password string) string {
	if password == "" {
		return "not configured"
	}
	return "configured (value hidden)"
}

func newLDAPSyncCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Inspect or run LDAP synchronization",
		Long: "List cursor-paginated LDAP synchronization history or start one run. Use\n" +
			"'history' first to inspect recent outcomes. A dry run scans and reports proposed\n" +
			"changes without changing users. A live run creates, updates, and may disable users,\n" +
			"so it requires confirmation or --yes. Requires ldap:sync and the LDAP license.",
		Example: "  n8n ldap sync history\n" +
			"  n8n ldap sync history --all --output json\n" +
			"  n8n ldap sync run --type dry\n" +
			"  n8n ldap sync run --type live --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newLDAPSyncHistoryCommand(opts), newLDAPSyncRunCommand(opts))
	return cmd
}

type ldapSyncHistoryFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	output   string
}

func newLDAPSyncHistoryCommand(opts Options) *cobra.Command {
	var f ldapSyncHistoryFlags
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List LDAP synchronization history",
		Long: "List LDAP synchronization records newest first, one cursor-paginated page at\n" +
			"a time. Pass nextCursor to --cursor, or use --all to follow pages up to the\n" +
			"10,000-record safety cap. Each record reports mode, status, timing, scanned users,\n" +
			"and create, update, and disable counts. Requires ldap:sync and the LDAP license.",
		Example: "  n8n ldap sync history\n" +
			"  n8n ldap sync history --limit 25 --cursor NEXT_CURSOR\n" +
			"  n8n ldap sync history --all --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runLDAPSyncHistory(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "records per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by previous history request (default: first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every history page instead of one (maximum 10,000 records)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runLDAPSyncHistory(ctx context.Context, opts Options, f ldapSyncHistoryFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListLDAPSyncHistoryOptions{ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pageOpts n8n.ListOptions) (n8n.Page[n8n.LDAPSyncHistory], error) {
		return client.ListLDAPSyncHistory(ctx, n8n.ListLDAPSyncHistoryOptions{ListOptions: pageOpts})
	}
	var page n8n.Page[n8n.LDAPSyncHistory]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return ldapAPIError(err, resolution, "read sync history")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeLDAPSyncHistory(opts, resolution, page)
}

func writeLDAPSyncHistory(opts Options, resolution config.Resolution, page n8n.Page[n8n.LDAPSyncHistory]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "LDAP synchronizations:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tMODE\tSTATUS\tSTARTED\tENDED\tSCANNED\tCREATED\tUPDATED\tDISABLED\tERROR")
		for _, history := range page.Data {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\n", history.ID, history.RunMode,
				emptyDash(history.Status), emptyDash(history.StartedAt), emptyDash(history.EndedAt), history.Scanned,
				history.Created, history.Updated, history.Disabled, emptyDash(history.Error))
		}
	}
	if page.HasMore() {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No LDAP synchronization history was returned. Run 'n8n ldap sync run --type dry' to test synchronization without changing users.")
	}
	return nil
}

type ldapSyncRunFlags struct {
	instance instanceFlags
	typeName string
	yes      bool
	output   string
}

func newLDAPSyncRunCommand(opts Options) *cobra.Command {
	var f ldapSyncRunFlags
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run LDAP synchronization",
		Long: "Start one LDAP synchronization and print its completed history record. --type dry\n" +
			"scans LDAP and reports proposed changes without persisting user changes. --type live\n" +
			"creates and updates users and may disable users missing from LDAP; live mode requires\n" +
			"interactive confirmation or --yes. Requires ldap:sync, configured LDAP settings,\n" +
			"and the LDAP license.",
		Example: "  n8n ldap sync run --type dry\n" +
			"  n8n ldap sync run --type dry --output json\n" +
			"  n8n ldap sync run --type live\n" +
			"  n8n ldap sync run --type live --yes --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runLDAPSyncRun(cmd.Context(), opts, f) },
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.typeName, "type", "", "synchronization mode: dry previews without user changes; live applies user changes (required)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm live synchronization without prompting, including possible user disablement (required for non-interactive live runs)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (result is one synchronization history record)")
	return cmd
}

func runLDAPSyncRun(ctx context.Context, opts Options, f ldapSyncRunFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	request := n8n.LDAPSyncRequest{Type: n8n.LDAPSyncMode(f.typeName)}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if request.Type == n8n.LDAPSyncLive {
		question := fmt.Sprintf("Run LIVE LDAP synchronization on %s? This creates and updates users and may disable users missing from LDAP.", resolution.URL)
		if err := opts.confirmer(f.yes).Confirm(question); err != nil {
			return err
		}
	}
	history, err := client.RunLDAPSync(ctx, request)
	if err != nil {
		return ldapAPIError(err, resolution, "run sync")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, history)
	}
	return writeLDAPSyncResult(opts, resolution, history)
}

func writeLDAPSyncResult(opts Options, resolution config.Resolution, history *n8n.LDAPSyncHistory) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "LDAP synchronization:\t%d\n", history.ID)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Mode:\t%s\n", history.RunMode)
	fmt.Fprintf(tw, "Status:\t%s\n", emptyDash(history.Status))
	fmt.Fprintf(tw, "Started:\t%s\n", emptyDash(history.StartedAt))
	fmt.Fprintf(tw, "Ended:\t%s\n", emptyDash(history.EndedAt))
	fmt.Fprintf(tw, "Scanned:\t%d\n", history.Scanned)
	fmt.Fprintf(tw, "Created:\t%d\n", history.Created)
	fmt.Fprintf(tw, "Updated:\t%d\n", history.Updated)
	fmt.Fprintf(tw, "Disabled:\t%d\n", history.Disabled)
	if history.Error != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", history.Error)
	}
	return tw.Flush()
}

func ldapAPIError(err error, resolution config.Resolution, action string) error {
	scope := "ldap:manage"
	if strings.Contains(action, "sync") {
		scope = "ldap:sync"
	}
	switch {
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "LDAP "+action, allOf(scope),
			"the instance also needs the LDAP license.", ldapResource)
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "update configuration":
		return fmt.Errorf("%s rejected the full LDAP replacement (400): include all 20 fields and use connectionSecurity none, tls, or startTls; response details redacted", resolution.URL)
	case n8n.IsStatus(err, http.StatusBadRequest) && action == "run sync":
		return fmt.Errorf("%s rejected the LDAP synchronization (400): check LDAP configuration and choose --type dry or live", resolution.URL)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s has no available LDAP endpoint (404): verify the LDAP feature and run %s", resolution.URL, discoverHint(ldapResource))
	case n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s has no available LDAP service (503): run %s and retry when the service is available", resolution.URL, discoverHint(ldapResource))
	default:
		return apiError(err, resolution, ldapResource)
	}
}
