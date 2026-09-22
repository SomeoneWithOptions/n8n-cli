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

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/confirm"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/selfupdate"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
)

// Process exit codes.
const (
	ExitSuccess = 0
	ExitError   = 1
	// ExitDiffError distinguishes a failed comparison from detected differences.
	ExitDiffError = 2
	ExitCanceled  = 130
)

// Streams are the three standard streams a command tree reads and writes.
// Stdout stays machine-readable; diagnostics and prompts go to Stderr.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Options are the dependencies of the root command. Every field that talks to
// the outside world can be replaced, which is what lets authentication tests
// run with no terminal, no keyring and no network.
type Options struct {
	Streams Streams
	Version version.Info
	// ConfigDir overrides the user configuration directory. Empty resolves
	// through [config.DefaultDir].
	ConfigDir string
	// Env overrides environment lookup. Nil reads the process environment.
	Env config.Lookup
	// Keyring overrides the OS credential store. Nil uses the real one.
	Keyring config.CredentialStore
	// Prompt overrides the no-echo credential prompt. Nil reads the terminal.
	Prompt SecretPrompter
	// Interactive overrides terminal detection on Streams.In.
	Interactive *bool
	// HTTPClient overrides the transport used for API calls and for release
	// downloads.
	HTTPClient n8n.Doer
	// ExecutablePath overrides the binary `n8n update` replaces. Empty
	// resolves through [os.Executable].
	ExecutablePath string
	// ReleaseAPIURL overrides the GitHub API root release lookups use. Empty
	// uses [selfupdate.DefaultAPIBaseURL].
	ReleaseAPIURL string
	// ReleaseDownloadURL overrides the host release assets are downloaded
	// from. Empty uses [selfupdate.DefaultDownloadBaseURL].
	ReleaseDownloadURL string
}

// interactive reports whether the CLI may ask the user something.
func (o Options) interactive() bool {
	if o.Interactive != nil {
		return *o.Interactive
	}
	return isTerminal(o.Streams.In)
}

// prompter returns the secret prompt to use.
func (o Options) prompter() SecretPrompter {
	if o.Prompt != nil {
		return o.Prompt
	}
	return terminalPrompter{in: o.Streams.In, out: o.Streams.Err}
}

// lookupEnv reads one environment variable through the configured lookup.
func (o Options) lookupEnv(name string) string {
	if o.Env == nil {
		return config.OSEnv(name)
	}
	return o.Env(name)
}

// store opens the configuration directory.
func (o Options) store() (*config.Store, error) { return config.NewStore(o.ConfigDir) }

// resolver wires the credential stores for this run.
func (o Options) resolver() (*config.Resolver, error) {
	store, err := o.store()
	if err != nil {
		return nil, err
	}
	resolver := config.NewResolver(store)
	if o.Keyring != nil {
		resolver.Keyring = o.Keyring
	}
	if o.Env != nil {
		resolver.Env = o.Env
	}
	return resolver, nil
}

// confirmer builds the guard used by destructive commands.
func (o Options) confirmer(assumeYes bool) confirm.Confirmer {
	return confirm.Confirmer{
		In:          o.Streams.In,
		Out:         o.Streams.Err,
		AssumeYes:   assumeYes,
		Interactive: o.interactive(),
	}
}

// clientOptions are the transport options shared by every API call.
func (o Options) clientOptions() []n8n.Option {
	opts := []n8n.Option{n8n.WithUserAgent(o.Version.UserAgent())}
	if o.HTTPClient != nil {
		opts = append(opts, n8n.WithHTTPClient(o.HTTPClient))
	}
	return opts
}

// updater builds the self-update client for this run. It shares HTTPClient
// with the API transport: both are "the network" this process is allowed.
func (o Options) updater() *selfupdate.Updater {
	return selfupdate.New(selfupdate.Config{
		HTTP:            o.HTTPClient,
		APIBaseURL:      o.ReleaseAPIURL,
		DownloadBaseURL: o.ReleaseDownloadURL,
		UserAgent:       o.Version.UserAgent(),
		ExecPath:        o.ExecutablePath,
	})
}

// Run builds the command tree, executes args against it, and returns the
// process exit code. It never calls os.Exit, so tests can use it directly.
func Run(ctx context.Context, args []string, opts Options) int {
	root := NewRootCommand(opts)
	root.SetArgs(args)

	cmd, err := root.ExecuteContextC(ctx)
	if errors.Is(err, errWorkflowDifferent) {
		return ExitError // A successful comparison with differences; no diagnostic.
	}
	var failedRun *executionFailedError
	if errors.As(err, &failedRun) {
		// The trace was printed; --fail-on-error only asks for the exit code,
		// so say why without the usage hint a real error gets.
		fmt.Fprintf(opts.Streams.Err, "n8n: %v\n", err)
		return ExitError
	}
	code := report(opts.Streams.Err, err)
	if code == ExitError && cmd != nil && cmd.Annotations["diffExitCodes"] == "true" {
		return ExitDiffError
	}
	return code
}

// report writes err to the diagnostic stream and returns the matching exit code.
// A nil stream (a zero Options in tests) falls back to discarding the message
// rather than panicking: the exit code still reports the failure.
func report(w io.Writer, err error) int {
	if w == nil {
		w = io.Discard
	}
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
		Long: "Command-line client for the n8n public API.\n\n" +
			"Start here: log in once, check the credential, then see what the instance and\n" +
			"that credential can do before running resource commands.\n" +
			"  n8n auth login --url https://n8n.example.com\n" +
			"  n8n auth status --check\n" +
			"  n8n discover\n\n" +
			"Context selection is --context, then N8N_CONTEXT, then the saved current context.\n" +
			"An empty N8N_CONTEXT is unset. Resource commands, auth status and auth logout\n" +
			"reject unknown names. Environment selection does not change the saved selection.\n" +
			"Auth login can create a context, falls back to default, and selects its context\n" +
			"when saving. URL precedence is --url, then N8N_URL, then the selected context URL.\n" +
			"Secrets are never flags: they come from a no-echo prompt, --stdin, the environment,\n" +
			"or the OS credential store.\n\n" +
			"Commands write machine-readable results to stdout and diagnostics, prompts,\n" +
			"and confirmations to stderr. Use --output json for scripting; use --help on\n" +
			"any group or action (for example 'n8n auth --help') for its workflow and flags.",
		Example: "  n8n auth login --url https://n8n.example.com\n" +
			"  n8n auth status --check\n" +
			"  n8n discover --resource workflow\n" +
			"  n8n audit generate\n" +
			"  n8n config context list\n" +
			"  n8n --help\n" +
			"  n8n auth login --help",
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
		newAuthCommand(opts),
		newDiscoverCommand(opts),
		newAuditCommand(opts),
		newCommunityPackageCommand(opts),
		newCredentialCommand(opts),
		newTagCommand(opts),
		newVariableCommand(opts),
		newRoleCommand(opts),
		newRoleMappingCommand(opts),
		newUserCommand(opts),
		newProjectCommand(opts),
		newFolderCommand(opts),
		newDataTableCommand(opts),
		newWorkflowCommand(opts),
		newExecutionCommand(opts),
		newEvaluationCommand(opts),
		newInsightCommand(opts),
		newNodePolicyCommand(opts),
		newSecurityPolicyCommand(opts),
		newOtelCommand(opts),
		newLogStreamCommand(opts),
		newLDAPCommand(opts),
		newOIDCCommand(opts),
		newSAMLCommand(opts),
		newSourceControlCommand(opts),
		newPackageCommand(opts),
		newGitConnectionCommand(opts),
		newPromotionCommand(opts),
		newConfigCommand(opts),
		newVersionCommand(opts),
		newUpdateCommand(opts),
		newCompletionCommand(),
	)
	return root
}
