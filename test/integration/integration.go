// Package integration holds opt-in tests against a live n8n instance.
//
// Every test skips unless N8N_INTEGRATION_URL and a credential are set, so the
// default suite stays offline. Mutating tests additionally require
// N8N_INTEGRATION_DESTRUCTIVE=1 and may only touch resources they created
// themselves, named with [ResourcePrefix].
package integration

import (
	"os"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
)

// Environment variables that enable these tests.
const (
	EnvURL         = "N8N_INTEGRATION_URL"
	EnvAPIKey      = "N8N_INTEGRATION_API_KEY"
	EnvDestructive = "N8N_INTEGRATION_DESTRUCTIVE"
)

// ResourcePrefix names every resource these tests create, so cleanup can find
// them and so nothing pre-existing is ever mistaken for test data.
const ResourcePrefix = "n8n-cli-test-"

// Instance is the live instance under test.
type Instance struct {
	URL    string
	APIKey n8n.Secret
}

// Require returns the instance under test, or skips the test when the
// integration environment is absent.
func Require(t *testing.T) Instance {
	t.Helper()
	url, key := os.Getenv(EnvURL), os.Getenv(EnvAPIKey)
	if url == "" || key == "" {
		t.Skipf("set %s and %s to run integration tests", EnvURL, EnvAPIKey)
	}
	return Instance{URL: url, APIKey: n8n.Secret(key)}
}

// RequireDestructive additionally demands opt-in for tests that mutate the
// instance. Only resources this suite created may be touched.
func RequireDestructive(t *testing.T) Instance {
	t.Helper()
	instance := Require(t)
	if os.Getenv(EnvDestructive) != "1" {
		t.Skipf("set %s=1 to run tests that mutate the instance", EnvDestructive)
	}
	return instance
}

// Client builds an API-key client for the instance under test.
func (i Instance) Client(t *testing.T) *n8n.Client {
	t.Helper()
	auth, err := n8n.NewAPIKeyAuth(i.APIKey)
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}
	client, err := n8n.New(i.URL, n8n.WithAuth(auth), n8n.WithUserAgent(version.Get().UserAgent()))
	if err != nil {
		t.Fatalf("n8n.New(%q): %v", i.URL, err)
	}
	return client
}
