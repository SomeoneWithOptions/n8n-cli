package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate a shell completion script",
		Long: "Generate a shell completion script for n8n.\n\n" +
			"The script is written to stdout; redirect or source it as your shell expects.\n" +
			"Completions cover commands, flags, and context names where known.\n\n" +
			"Examples:\n" +
			"  bash:       source <(n8n completion bash)\n" +
			"  zsh:        n8n completion zsh > \"${fpath[1]}/_n8n\"\n" +
			"  fish:       n8n completion fish > ~/.config/fish/completions/n8n.fish\n" +
			"  powershell: n8n completion powershell | Out-String | Invoke-Expression",
		Example: "  source <(n8n completion bash)\n" +
			"  n8n completion zsh > \"${fpath[1]}/_n8n\"",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:   []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			default:
				// Unreachable: OnlyValidArgs rejects anything else.
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}
