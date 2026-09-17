// Package cli builds the n8n command tree.
//
// Nothing here registers itself: every command is constructed by an explicit
// call and receives its dependencies as arguments, so a test can build a whole
// command tree with its own streams and run it in-process.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
)

// Process exit codes.
const (
	ExitSuccess  = 0
	ExitError    = 1
	ExitCanceled = 130
)

// Streams are the three standard streams a command tree reads and writes.
// Stdout stays machine-readable; diagnostics and prompts go to Stderr.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Options are the dependencies of the root command.
type Options struct {
	Streams Streams
	Version version.Info
}

// Run builds the command tree, executes args against it, and returns the
// process exit code. It never calls os.Exit, so tests can use it directly.
func Run(ctx context.Context, args []string, opts Options) int {
	root := NewRootCommand(opts)
	root.SetArgs(args)

	return report(opts.Streams.Err, root.ExecuteContext(ctx))
}

// report writes err to the diagnostic stream and returns the matching exit code.
func report(w io.Writer, err error) int {
	switch {
	case err == nil:
		return ExitSuccess
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		fmt.Fprintln(w, "n8n: canceled")
		return ExitCanceled
	default:
		fmt.Fprintf(w, "n8n: %v\n", err)
		fmt.Fprintln(w, "Run 'n8n --help' for usage.")
		return ExitError
	}
}

// NewRootCommand builds the root command and its subcommands.
func NewRootCommand(opts Options) *cobra.Command {
	root := &cobra.Command{
		Use:   "n8n",
		Short: "Command-line client for the n8n API",
		Long: "n8n is a command-line client for the n8n public API.\n\n" +
			"Commands write machine-readable results to stdout and diagnostics to stderr.",
		Version:       opts.Version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Do not guess: an unknown command is an error, not a run of the root.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}

	root.SetIn(opts.Streams.In)
	root.SetOut(opts.Streams.Out)
	root.SetErr(opts.Streams.Err)
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	// Register --version without a shorthand before Cobra claims -v for it.
	// -v is reserved for verbose output in a later phase.
	root.Flags().Bool("version", false, "version for n8n")

	// Replaced by an explicitly constructed command below.
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newVersionCommand(opts),
		newCompletionCommand(),
	)
	return root
}
