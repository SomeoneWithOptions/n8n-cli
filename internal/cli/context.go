package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/contextops"
)

func newConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and edit stored CLI configuration",
		Long: "Inspect and edit stored CLI configuration.\n\n" +
			"Configuration is contexts: one instance URL plus auth type plus credential\n" +
			"reference each. Start with 'n8n config context list', switch with\n" +
			"'n8n config context use NAME'.",
		Example: "  n8n config context list\n" +
			"  n8n config context use production",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.NoArgs,
		RunE:        func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newContextCommand(opts))
	return cmd
}

func newContextCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage saved instance contexts",
		Long: "Manage saved instance contexts.\n\n" +
			"A context is one instance URL, one authentication type and a reference to the\n" +
			"credential stored for it. Contexts are kept apart so a credential for one\n" +
			"instance is never sent to another. List with 'list', switch with 'use NAME',\n" +
			"rename with 'rename OLD NEW', remove with 'delete NAME'. Create or repair one\n" +
			"with 'n8n auth login'.",
		Example: "  n8n config context list\n" +
			"  n8n config context use production\n" +
			"  n8n config context rename default work\n" +
			"  n8n config context delete old-staging --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newContextListCommand(opts),
		newContextUseCommand(opts),
		newContextRenameCommand(opts),
		newContextDeleteCommand(opts),
	)
	return cmd
}

// contextReport is the machine-readable shape of one row of `context list`.
type contextReport struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	URL      string `json:"url"`
	AuthType string `json:"authType"`
	Storage  string `json:"storage"`
}

func newContextListCommand(opts Options) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List saved contexts",
		Long: "List saved contexts.\n\n" +
			"Shows every saved context, which one is current (*), and its URL, auth\n" +
			"type, and storage. Prints a hint when none exists yet. Use --output json\n" +
			"for scripting.",
		Example: "  n8n config context list\n" +
			"  n8n config context list --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutput(output); err != nil {
				return err
			}
			store, err := opts.store()
			if err != nil {
				return err
			}
			cfg, err := store.Load()
			if err != nil {
				return err
			}

			reports := make([]contextReport, 0, len(cfg.Contexts))
			for _, name := range cfg.Names() {
				saved := cfg.Contexts[name]
				reports = append(reports, contextReport{
					Name:     name,
					Current:  name == cfg.CurrentContext,
					URL:      saved.URL,
					AuthType: string(saved.AuthType),
					Storage:  string(saved.Storage),
				})
			}

			if output == outputJSON {
				return writeJSON(opts.Streams.Out, reports)
			}
			if len(reports) == 0 {
				fmt.Fprintln(opts.Streams.Err, "No contexts yet. Run 'n8n auth login' to add one.")
				return nil
			}

			tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "CURRENT\tNAME\tURL\tAUTH\tSTORAGE")
			for _, r := range reports {
				marker := ""
				if r.Current {
					marker = "*"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", marker, r.Name, r.URL, r.AuthType, r.Storage)
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&output, "output", outputText, "output format: text or json (default text; json is stable for scripting)")
	return cmd
}

func newContextUseCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Select the context used by subsequent commands",
		Long: "Select the context used by subsequent commands.\n\n" +
			"NAME must already exist (see 'n8n config context list'). The choice takes\n" +
			"effect for later runs unless --context or N8N_CONTEXT overrides it. Unset\n" +
			"N8N_CONTEXT to use the saved selection. Create a new context with\n" +
			"'n8n auth login --context NAME'.",
		Example: "  n8n config context use production",
		Args:    cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			return completeContextNames(opts, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := opts.store()
			if err != nil {
				return err
			}
			if err := contextops.New(store, nil).Use(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(opts.Streams.Err, "Now using context %q.\n", args[0])
			return nil
		},
	}
}

func newContextDeleteCommand(opts Options) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a context and its stored credential",
		Long: "Delete a context and its stored credential.\n\n" +
			"This removes the entry from config.json and the credential it references from\n" +
			"the credential store. It changes nothing on the n8n instance: an API key deleted\n" +
			"here stays valid until it is revoked in n8n. Use --yes for scripts; without\n" +
			"a terminal the command fails unless --yes is given.",
		Example: "  n8n config context delete old-staging\n" +
			"  n8n config context delete old-staging --yes",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			return completeContextNames(opts, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			resolver, err := opts.resolver()
			if err != nil {
				return err
			}
			cfg, err := resolver.Store.Load()
			if err != nil {
				return err
			}
			saved, ok := cfg.Contexts[name]
			if !ok {
				return &config.NotFoundError{Name: name}
			}

			question := fmt.Sprintf("Delete context %q (%s) and its stored credential?", name, saved.URL)
			if err := opts.confirmer(yes).Confirm(question); err != nil {
				return err
			}

			_, err = contextops.New(resolver.Store, resolver.CredentialStore).Logout(contextops.Capture(cfg, name), true)
			if err != nil {
				return err
			}

			fmt.Fprintf(opts.Streams.Err, "Deleted context %q.\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "delete without asking (required in non-interactive use)")
	return cmd
}

// completeContextNames completes a context argument from config.json. A
// configuration that cannot be read completes nothing rather than failing.
func completeContextNames(opts Options, args []string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	store, err := opts.store()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := store.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cfg.Names(), cobra.ShellCompDirectiveNoFileComp
}

func newContextRenameCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "rename OLD NEW",
		Short: "Rename a saved context without changing its credentials",
		Long: "Rename a saved context locally.\n\n" +
			"OLD must exist and NEW must be unused; an existing same-name rename is a no-op.\n" +
			"Names use 1–64 ASCII letters, digits, '-', '_' or '.', with no leading dot.\n" +
			"URL, authentication, storage and credential identity are preserved. If OLD is\n" +
			"current, the selection follows NEW. This CLI-side metadata change needs no\n" +
			"network or credential-store access and never changes remote resources.\n\n" +
			"No confirmation is needed. Success prints a message to stderr, leaving stdout\n" +
			"empty. External scripts, --context and N8N_CONTEXT still refer to literal names:\n" +
			"update N8N_CONTEXT after renaming its selected context, or unset it. Update\n" +
			"references to OLD yourself; no alias is created. Avoid writing shared config\n" +
			"with older binaries that reuse names as credential references.\n" +
			"Next step: 'n8n config context list --output json' to inspect saved state.",
		Example: "  n8n config context rename default work\n" +
			"  n8n config context use work",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			return completeContextNames(opts, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := opts.store()
			if err != nil {
				return err
			}
			changed, err := contextops.New(store, nil).Rename(args[0], args[1])
			if err != nil {
				return err
			}
			if !changed {
				fmt.Fprintf(opts.Streams.Err, "Context %q already has that name; nothing changed.\n", args[0])
				return nil
			}
			fmt.Fprintf(opts.Streams.Err, "Renamed context %q to %q.\n", args[0], args[1])
			return nil
		},
	}
}
