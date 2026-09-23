package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/contextops"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// DefaultContextName is used when --context and N8N_CONTEXT are unset, so the common
// single-instance case needs no naming decision.
const DefaultContextName = "default"

func newAuthCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in to an n8n instance and inspect stored credentials",
		Long: "Log in to an n8n instance and inspect stored credentials.\n\n" +
			"Typical workflow: 'n8n auth login' once per instance, 'n8n auth status'\n" +
			"to inspect what is saved, 'n8n auth status --check' to verify the\n" +
			"credential still works, 'n8n auth logout' to forget it. Run resource\n" +
			"commands with a saved credential or an environment credential.",
		Example: "  n8n auth login --url https://n8n.example.com\n" +
			"  n8n auth status --check\n" +
			"  n8n auth logout",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.NoArgs,
		RunE:        func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newAuthLoginCommand(opts),
		newAuthStatusCommand(opts),
		newAuthLogoutCommand(opts),
	)
	return cmd
}

type loginFlags struct {
	url        string
	context    string
	authType   string
	storage    string
	fromStdin  bool
	skipVerify bool
	yes        bool
}

func newAuthLoginCommand(opts Options) *cobra.Command {
	var f loginFlags

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a credential for an n8n instance",
		Long: "Store a credential for an n8n instance and select its context.\n\n" +
			"The context name comes from --context, then N8N_CONTEXT, then default. Unlike\n" +
			"resource commands, login does not fall back to the saved current context. A\n" +
			"successful login selects the saved context, including when named by the environment.\n\n" +
			"Use this to save a credential; environment credentials also work without login. The\n" +
			"credential is read from a no-echo prompt, or from stdin with --stdin. It is\n" +
			"never accepted as a flag or an argument, because process lists and shell history\n" +
			"would expose it. It is validated against GET /api/v1/discover before anything\n" +
			"is saved, so a typo or revoked key saves nothing by default.\n\n" +
			"Use --skip-verify for offline setup, instances without /discover, or keys that\n" +
			"lack discovery access. This skips only the remote check: local validation,\n" +
			"storage consent and replacement confirmation still apply. A wrong URL or invalid\n" +
			"credential can be saved; access is checked on the first resource request.\n" +
			"This command saves local configuration and never changes remote resources.\n\n" +
			"Credentials are stored in the operating system credential store. With\n" +
			"--storage=file they are stored in auth.json instead, which is plaintext protected\n" +
			"only by file permissions, not by encryption. Environment credentials\n" +
			"(N8N_API_KEY and friends) override the saved one for a single run and are\n" +
			"never persisted by this command.\n\n" +
			"Each login saves a fresh credential reference, independent of context names.\n" +
			"A crash during saving or cleanup can leave an unused credential in storage.\n" +
			"Older CLI versions can reuse name-based references: avoid writing this shared\n" +
			"configuration with older binaries after renaming contexts.\n\n" +
			"Next step: 'n8n auth status --check'. After --skip-verify, inspect saved state\n" +
			"with 'n8n auth status', then run a resource command your key permits, such as\n" +
			"'n8n user list --limit 1' for user:list. Discovery and auth status --check\n" +
			"still require access to /discover.",
		Example: "  # Interactive login (prompts for the API key):\n" +
			"  n8n auth login --url https://n8n.example.com\n\n" +
			"  # Non-interactive login from a secret manager or pipe:\n" +
			"  printf '%s' \"$N8N_API_KEY\" | n8n auth login --url https://n8n.example.com --stdin\n\n" +
			"  # Named context with an explicit auth type and plaintext fallback:\n" +
			"  n8n auth login --url https://n8n.example.com --context production --type api-key --storage=file\n\n" +
			"  # Save a narrowly scoped key without discovery verification:\n" +
			"  n8n auth login --url https://n8n.example.com --context limited --skip-verify",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthLogin(cmd.Context(), opts, f)
		},
	}

	cmd.Flags().StringVar(&f.url, "url", "", "instance URL, e.g. https://n8n.example.com (falls back to N8N_URL, then saved context URL)")
	cmd.Flags().StringVar(&f.context, "context", "", "context name to create or replace (falls back to N8N_CONTEXT, then \""+DefaultContextName+"\")")
	cmd.Flags().StringVar(&f.authType, "type", string(n8n.AuthAPIKey), "authentication type: api-key, bearer or cookie (default api-key)")
	cmd.Flags().StringVar(&f.storage, "storage", "", "credential storage: keyring or file (default keyring; file is plaintext, permissions-only protection)")
	cmd.Flags().BoolVar(&f.fromStdin, "stdin", false, "read the credential from stdin instead of prompting (use for scripts and AI agents)")
	cmd.Flags().BoolVar(&f.skipVerify, "skip-verify", false, "skip the remote /discover credential check (default false; saves without verifying access)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "replace the existing credential without asking")

	return cmd
}

func runAuthLogin(ctx context.Context, opts Options, f loginFlags) error {
	authType, err := parseAuthType(f.authType)
	if err != nil {
		return err
	}

	resolver, err := opts.resolver()
	if err != nil {
		return err
	}
	name, err := resolver.ContextName(f.context)
	if err != nil {
		return err
	}
	if name == "" {
		name = DefaultContextName
	}
	cfg, err := resolver.Store.Load()
	if err != nil {
		return err
	}
	existing, replacing := cfg.Contexts[name]

	rawURL := firstNonEmpty(f.url, opts.lookupEnv(config.EnvURL), existing.URL)
	if rawURL == "" {
		if !opts.interactive() {
			return fmt.Errorf("no instance URL: pass --url or set %s", config.EnvURL)
		}
		if rawURL, err = promptLine(opts.Streams.In, opts.Streams.Err, "n8n instance URL: "); err != nil {
			return err
		}
	}
	base, err := n8n.NormalizeBaseURL(rawURL)
	if err != nil {
		return err
	}
	instanceURL := n8n.InstanceURL(base)

	// Ask before reading a new secret, so a declined replacement never made
	// the user type a credential for nothing.
	if replacing {
		question := fmt.Sprintf("Context %q already points at %s. Replace its credential?", name, existing.URL)
		if err := opts.confirmer(f.yes).Confirm(question); err != nil {
			return err
		}
	}

	secret, err := readLoginSecret(opts, f, authType)
	if err != nil {
		return err
	}
	cred := config.Credential{Type: authType, Value: secret}

	backend, err := selectStorage(opts, resolver, f.storage)
	if err != nil {
		return err
	}

	auth, err := cred.Authenticator()
	if err != nil {
		return err
	}
	if !f.skipVerify {
		client := n8n.NewWithBaseURL(base, append(opts.clientOptions(), n8n.WithAuth(auth))...)
		if err := client.Verify(ctx); err != nil {
			return loginError(instanceURL, err)
		}
	}

	warning, err := contextops.New(resolver.Store, resolver.CredentialStore).Login(
		contextops.Capture(cfg, name),
		config.Context{URL: instanceURL, AuthType: authType, Storage: backend.Kind()}, cred)
	if err != nil {
		return err
	}
	if warning != nil {
		fmt.Fprintf(opts.Streams.Err, "Warning: %v.\n", warning)
	}

	if f.skipVerify {
		fmt.Fprintf(opts.Streams.Err, "Credential saved for %s as context %q using %s authentication (%s storage); verification skipped.\n",
			instanceURL, name, authType, backend.Kind())
		return nil
	}

	fmt.Fprintf(opts.Streams.Err, "Logged in to %s as context %q using %s authentication (%s storage).\n",
		instanceURL, name, authType, backend.Kind())
	return nil
}

// readLoginSecret reads credential material. Environment credentials are
// deliberately not accepted here: they apply to a single process and are never
// persisted behind the user's back.
func readLoginSecret(opts Options, f loginFlags, authType n8n.AuthType) (n8n.Secret, error) {
	if f.fromStdin {
		return readSecretFrom(opts.Streams.In)
	}
	if !opts.interactive() {
		return "", errNoTerminal
	}
	return opts.prompter().PromptSecret(secretPromptFor(authType))
}

func secretPromptFor(authType n8n.AuthType) string {
	switch authType {
	case n8n.AuthBearer:
		return "n8n bearer token: "
	case n8n.AuthCookie:
		return "n8n auth cookie value: "
	default:
		return "n8n API key: "
	}
}

// selectStorage picks the credential backend. A missing OS credential store is
// never downgraded silently: the user either asked for --storage=file or is
// asked, on a terminal, to accept plaintext storage.
func selectStorage(opts Options, resolver *config.Resolver, requested string) (config.CredentialStore, error) {
	if requested != "" {
		kind := config.Storage(requested)
		if !kind.Valid() {
			return nil, fmt.Errorf("unknown storage %q: use %s or %s", requested, config.StorageKeyring, config.StorageFile)
		}
		backend, err := resolver.CredentialStore(kind)
		if err != nil {
			return nil, err
		}
		if kind == config.StorageFile {
			fmt.Fprintf(opts.Streams.Err, "Storing the credential in plaintext at %s. File permissions are its only protection.\n", filePath(resolver))
		}
		return backend, nil
	}

	prober, ok := resolver.Keyring.(interface{ Available() error })
	if !ok {
		return resolver.Keyring, nil
	}
	unavailable := prober.Available()
	if unavailable == nil {
		return resolver.Keyring, nil
	}

	question := fmt.Sprintf("No OS credential store is available (%v).\nStore the credential in plaintext at %s instead?", unavailable, filePath(resolver))
	// Not opts.confirmer(f.yes): --yes covers replacing a credential, not
	// accepting a weaker form of storage.
	if err := opts.confirmer(false).Confirm(question); err != nil {
		return nil, fmt.Errorf("%w: %v; re-run with --storage=file to accept plaintext storage", config.ErrStorageConsent, unavailable)
	}
	return resolver.CredentialStore(config.StorageFile)
}

func filePath(resolver *config.Resolver) string {
	if fs, ok := resolver.File.(interface{ Path() string }); ok {
		return fs.Path()
	}
	return resolver.Store.Path(config.SecretFileName)
}

// loginError turns a failed validation into guidance. Nothing has been saved at
// this point, and the message must say so.
func loginError(instanceURL string, err error) error {
	switch {
	case n8n.IsUnauthorized(err):
		return fmt.Errorf("%s rejected the credential (401): check that it is correct and has not expired or been revoked; nothing was saved", instanceURL)
	case n8n.IsForbidden(err):
		return fmt.Errorf("%s accepted the credential but denied access (403): it lacks the scope for %s; use --skip-verify to save without checking discovery access, then run a resource command your key permits; nothing was saved", instanceURL, n8n.DiscoverPath)
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("credential verification at %s%s is unavailable: %w; use --skip-verify to save without the remote check; nothing was saved", instanceURL, n8n.BasePath+n8n.DiscoverPath, err)
	default:
		return fmt.Errorf("could not reach %s: %w; nothing was saved", instanceURL, err)
	}
}

type statusFlags struct {
	context string
	output  string
	check   bool
}

// statusReport is the machine-readable shape of `auth status`. It never carries
// credential material, only where the credential came from.
type statusReport struct {
	Context    string `json:"context"`
	Current    bool   `json:"current"`
	URL        string `json:"url"`
	AuthType   string `json:"authType"`
	Storage    string `json:"storage,omitempty"`
	Source     string `json:"source"`
	Credential string `json:"credential"`
	Validated  *bool  `json:"validated,omitempty"`
	Error      string `json:"error,omitempty"`
}

func newAuthStatusCommand(opts Options) *cobra.Command {
	var f statusFlags

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the active context, instance and credential state",
		Long: "Show the active context, instance and credential state.\n\n" +
			"Use this to answer 'why did my command fail with auth errors': it shows\n" +
			"which context is current, which URL and auth type it points at, where the\n" +
			"credential came from (keyring, file, or environment), and whether one is\n" +
			"present. No credential value is ever printed. With --check the credential\n" +
			"is used once against GET /api/v1/discover to report whether the instance\n" +
			"still accepts it; --check fails the command when the credential is missing\n" +
			"or rejected, so scripts and agents can gate on it.\n\n" +
			"Context selection is --context, then N8N_CONTEXT, then the saved current context.\n" +
			"The current marker reports the saved selection, even when the environment selects\n" +
			"another context for this run. This command does not change that saved selection.",
		Example: "  n8n auth status\n" +
			"  n8n auth status --check\n" +
			"  N8N_CONTEXT=production n8n auth status --check\n" +
			"  n8n auth status --context production --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthStatus(cmd.Context(), opts, f)
		},
	}

	cmd.Flags().StringVar(&f.context, "context", "", "saved context name to describe (falls back to N8N_CONTEXT, then the saved current context)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text; json is stable for scripting)")
	cmd.Flags().BoolVar(&f.check, "check", false, "validate the credential against the instance; exit non-zero when unusable")

	return cmd
}

func runAuthStatus(ctx context.Context, opts Options, f statusFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	resolver, err := opts.resolver()
	if err != nil {
		return err
	}
	cfg, err := resolver.Store.Load()
	if err != nil {
		return err
	}

	envCred, fromEnv, err := resolver.EnvCredential()
	if err != nil {
		return err
	}
	name, saved, lookupErr := resolver.LookupContext(cfg, f.context)
	if lookupErr != nil && (!fromEnv || !errors.Is(lookupErr, config.ErrNoCurrentContext)) {
		return lookupErr
	}

	report := statusReport{
		Context:    name,
		Current:    name != "" && name == cfg.CurrentContext,
		URL:        saved.URL,
		AuthType:   string(saved.AuthType),
		Storage:    string(saved.Storage),
		Credential: "missing",
	}

	var cred config.Credential
	switch {
	case fromEnv:
		cred = envCred
		report.Source = string(config.SourceEnv)
		report.AuthType = string(envCred.Type)
		report.Storage = ""
		report.Credential = "present"
		if envURL := opts.lookupEnv(config.EnvURL); envURL != "" {
			report.URL = envURL
		}
	default:
		report.Source = string(saved.Storage.Source())
		backend, err := resolver.CredentialStore(saved.Storage)
		if err != nil {
			return err
		}
		cred, err = backend.Get(saved.CredentialRef)
		switch {
		case errors.Is(err, config.ErrCredentialNotFound):
			report.Error = fmt.Sprintf("no stored credential: run 'n8n auth login --context %s'", name)
		case err != nil:
			report.Error = err.Error()
		default:
			report.Credential = "present"
		}
	}

	if report.URL == "" {
		return fmt.Errorf("no instance URL: pass --url or set %s", config.EnvURL)
	}

	if f.check && report.Credential == "present" {
		validated := false
		auth, err := cred.Authenticator()
		if err != nil {
			return err
		}
		client, err := n8n.New(report.URL, append(opts.clientOptions(), n8n.WithAuth(auth))...)
		if err != nil {
			return err
		}
		if err := client.Verify(ctx); err != nil {
			report.Error = err.Error()
		} else {
			validated = true
		}
		report.Validated = &validated
	}

	if f.output == outputJSON {
		if err := writeJSON(opts.Streams.Out, report); err != nil {
			return err
		}
	} else if err := writeStatusText(opts, report); err != nil {
		return err
	}

	// --check exists to be used from scripts, so a credential the instance
	// will not accept has to fail the command, not just describe itself.
	if f.check && (report.Validated == nil || !*report.Validated) {
		return fmt.Errorf("credential for %s is not usable: %s", report.URL, report.Error)
	}
	return nil
}

func writeStatusText(opts Options, report statusReport) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	row := func(label, value string) {
		if value != "" {
			fmt.Fprintf(tw, "%s\t%s\n", label, value)
		}
	}
	name := report.Context
	if name == "" {
		name = "(none)"
	}
	if report.Current {
		name += " (current)"
	}
	row("Context:", name)
	row("URL:", report.URL)
	row("Auth type:", report.AuthType)
	row("Storage:", report.Storage)
	row("Source:", report.Source)
	row("Credential:", report.Credential)
	if report.Validated != nil {
		if *report.Validated {
			row("Validated:", "accepted by the instance")
		} else {
			row("Validated:", "rejected by the instance")
		}
	}
	row("Note:", report.Error)
	return tw.Flush()
}

type logoutFlags struct {
	context string
	yes     bool
	purge   bool
}

func newAuthLogoutCommand(opts Options) *cobra.Command {
	var f logoutFlags

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Delete the stored credential for a context",
		Long: "Delete the stored credential for a context.\n\n" +
			"Use this to revoke local access without touching the server. The context\n" +
			"itself is kept, so 'n8n auth login' can restore it. With --purge the context\n" +
			"is removed from config.json too, which is what 'n8n config context delete'\n" +
			"does. The API key stays valid on the n8n instance until revoked there. Use\n" +
			"--yes for scripts.",
		Example: "  n8n auth logout\n" +
			"  n8n auth logout --context production --yes\n" +
			"  # Forget the credential and the context itself:\n" +
			"  n8n auth logout --context production --purge --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthLogout(opts, f)
		},
	}

	cmd.Flags().StringVar(&f.context, "context", "", "saved context name to log out of (falls back to N8N_CONTEXT, then the saved current context)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "delete without asking (required in non-interactive use)")
	cmd.Flags().BoolVar(&f.purge, "purge", false, "also remove the context itself from config.json")

	return cmd
}

func runAuthLogout(opts Options, f logoutFlags) error {
	resolver, err := opts.resolver()
	if err != nil {
		return err
	}
	cfg, err := resolver.Store.Load()
	if err != nil {
		return err
	}
	name, saved, err := resolver.LookupContext(cfg, f.context)
	if err != nil {
		return err
	}

	question := fmt.Sprintf("Delete the stored credential for context %q (%s)?", name, saved.URL)
	if f.purge {
		question = fmt.Sprintf("Delete context %q (%s) and its stored credential?", name, saved.URL)
	}
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}

	hadCredential, err := contextops.New(resolver.Store, resolver.CredentialStore).Logout(contextops.Capture(cfg, name), f.purge)
	if err != nil {
		return err
	}

	switch {
	case hadCredential && f.purge:
		fmt.Fprintf(opts.Streams.Err, "Deleted the %s credential for context %q and removed the context.\n", saved.Storage, name)
	case hadCredential:
		fmt.Fprintf(opts.Streams.Err, "Deleted the %s credential for context %q. The context is kept.\n", saved.Storage, name)
	case f.purge:
		fmt.Fprintf(opts.Streams.Err, "No credential was stored for context %q. Removed the context.\n", name)
	default:
		fmt.Fprintf(opts.Streams.Err, "No credential was stored for context %q. The context is kept.\n", name)
	}
	return nil
}

func parseAuthType(value string) (n8n.AuthType, error) {
	switch t := n8n.AuthType(strings.TrimSpace(strings.ToLower(value))); t {
	case n8n.AuthAPIKey, n8n.AuthBearer, n8n.AuthCookie:
		return t, nil
	case "":
		return n8n.AuthAPIKey, nil
	default:
		return "", fmt.Errorf("unknown auth type %q: use %s, %s or %s", value, n8n.AuthAPIKey, n8n.AuthBearer, n8n.AuthCookie)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
