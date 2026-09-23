package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// instanceFlags are the connection flags every resource command shares. They
// select which saved context to talk to and, optionally, override its URL.
type instanceFlags struct {
	context string
	url     string
}

// register adds the shared connection flags to cmd.
func (f *instanceFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.context, "context", "", "saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')")
	cmd.Flags().StringVar(&f.url, "url", "", "instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)")
}

// selection turns the flags into a resolver selection.
func (f instanceFlags) selection() config.Selection {
	return config.Selection{Context: f.context, URL: f.url}
}

// apiClient resolves the instance and credential for a resource command and
// builds a client for them. The [config.Resolution] comes back too, because
// error messages have to name the context and URL the request actually used.
func (o Options) apiClient(f instanceFlags) (*n8n.Client, config.Resolution, error) {
	resolver, err := o.resolver()
	if err != nil {
		return nil, config.Resolution{}, err
	}
	resolution, err := resolver.Resolve(f.selection())
	if err != nil {
		return nil, config.Resolution{}, err
	}
	client, err := resolution.Client(o.clientOptions()...)
	if err != nil {
		return nil, resolution, err
	}
	return client, resolution, nil
}

// apiError turns a failed API call into a message that says what to run next.
// Agents act on stderr text, so the remediation belongs in the error, not only
// in the docs. resource is the `n8n discover` resource key the command belongs
// to, or empty when the call is not resource-scoped.
func apiError(err error, resolution config.Resolution, resource string) error {
	switch {
	case n8n.IsUnauthorized(err):
		return fmt.Errorf("%s rejected the credential (401): it is missing, expired or revoked; run %s", resolution.URL, loginHint(resolution))
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "the request", scopeNeed{},
			"a 403 can also mean the instance is not licensed for the feature.", resource)
	default:
		return err
	}
}

// scopeNeed is what one denied action requires: the scopes the API checks for
// it, and whether any single one of them is enough. The zero value means the
// requirement is unknown, so no scope is claimed.
type scopeNeed struct {
	scopes []string
	// any reports that one of the scopes is enough; otherwise all are needed.
	any bool
}

// allOf is a requirement every listed scope has to satisfy.
func allOf(scopes ...string) scopeNeed { return scopeNeed{scopes: scopes} }

// anyOf is a requirement one of the listed scopes satisfies, e.g. a role write
// that global role:manage and project role:manageProject both allow.
func anyOf(scopes ...string) scopeNeed { return scopeNeed{scopes: scopes, any: true} }

// known reports whether a scope requirement was mapped for the action.
func (n scopeNeed) known() bool { return len(n.scopes) > 0 }

// block renders the scope section of a 403: one scope per line, under a heading
// that says whether all of them are needed or only one.
func (n scopeNeed) block() string {
	if !n.known() {
		return "  missing scope: the CLI has no scope recorded for this call"
	}
	head := "  missing scope:"
	switch {
	case len(n.scopes) > 1 && n.any:
		head = "  missing scope (either one):"
	case len(n.scopes) > 1:
		head = "  missing scopes (all required):"
	}
	var b strings.Builder
	b.WriteString(head)
	for _, scope := range n.scopes {
		b.WriteString("\n    ")
		b.WriteString(scope)
	}
	return b.String()
}

// forbiddenScopeError is the shared 403 report: what was denied, the exact
// scopes the API requires for it on their own lines, what the server itself
// said, the other causes a status code cannot rule out, and where to inspect
// the credential. subject names the denied action, e.g. "tag list". extra is
// one sentence about those other causes (license, project access, role), empty
// when the scope is the only one.
//
// The server error is wrapped, not copied, so its status, code, hint and
// request ID stay reachable through [errors.As] for anything that inspects the
// failure instead of printing it.
func forbiddenScopeError(err error, resolution config.Resolution, subject string, need scopeNeed, extra, resource string) error {
	format := "%s denied %s (403)\n\n%s\n\n"
	args := []any{resolution.URL, subject, need.block()}
	if err != nil {
		format += "  server said: %w\n"
		args = append(args, err)
	}
	if extra != "" {
		format += "  %s\n"
		args = append(args, extra)
	}
	format += "  run %s to inspect access"
	args = append(args, discoverHint(resource))
	return fmt.Errorf(format, args...)
}

// loginHint names the command that repairs the credential in use. A credential
// from the environment is not repaired by logging in, so say so instead.
func loginHint(resolution config.Resolution) string {
	if resolution.Source == config.SourceEnv {
		return "'n8n auth status' (the credential comes from the environment, not from a saved context)"
	}
	if resolution.ContextName == "" {
		return "'n8n auth login'"
	}
	return fmt.Sprintf("'n8n auth login --context %s'", resolution.ContextName)
}

// discoverHint names the discover command that explains a denial.
func discoverHint(resource string) string {
	if resource == "" {
		return "'n8n discover'"
	}
	return fmt.Sprintf("'n8n discover --resource %s'", resource)
}
