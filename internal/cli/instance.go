package cli

import (
	"fmt"

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
	cmd.Flags().StringVar(&f.context, "context", "", "saved context to use (default: the current context; see 'n8n config context list')")
	cmd.Flags().StringVar(&f.url, "url", "", "instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)")
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
		return fmt.Errorf("%s denied the request (403): the credential lacks the scope, or the instance is not licensed for it; run %s to see what this credential can do", resolution.URL, discoverHint(resource))
	default:
		return err
	}
}

// loginHint names the command that repairs the credential in use. A credential
// from the environment is not repaired by logging in, so say so instead.
func loginHint(resolution config.Resolution) string {
	if resolution.Source == config.SourceEnv {
		return "'n8n auth status' (the credential comes from the environment, not from a saved context)"
	}
	if resolution.ContextName == "" || resolution.ContextName == DefaultContextName {
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
