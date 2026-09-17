package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		Long: "Print build information: version, commit, build date, toolchain, and platform.\n\n" +
			"Use this when reporting bugs or checking which release is installed.",
		Example: "  n8n version",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), opts.Version.String())
			return err
		},
	}
}
