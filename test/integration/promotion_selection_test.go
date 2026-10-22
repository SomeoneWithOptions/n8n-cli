package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// Discovery is read-only: never create providers, connections, or checkouts.
func TestPromotionRepositoryDiscovery(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()
	providers, err := client.ListPromotionProviders(ctx, n8n.ListOptions{Limit: 100})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatal(err)
	}
	var providerID string
	for _, provider := range providers.Data {
		if provider.Type == n8n.PromotionProviderTypeGitLab {
			providerID = provider.ID
			break
		}
	}
	if providerID == "" {
		t.Skip("no GitLab promotion provider available")
	}
	repositories, err := client.ListPromotionProviderRepositories(ctx, providerID, n8n.PromotionDiscoveryOptions{ListOptions: n8n.ListOptions{Limit: 10}})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatal(err)
	}
	runPromotionSelectionReadCLI(t, instance, []string{"promotion", "provider", "repositories", providerID, "--limit", "10"})
	if len(repositories.Data) == 0 {
		t.Skip("no accessible GitLab repository to list branches")
	}
	repository := repositories.Data[0]
	if repository.FullPath == "" || repository.RemoteURL == "" {
		t.Fatalf("repository lacks fullPath or remoteUrl: %+v", repository)
	}
	branches, err := client.ListPromotionProviderRepositoryBranches(ctx, providerID, repository.FullPath, n8n.PromotionDiscoveryOptions{ListOptions: n8n.ListOptions{Limit: 10}})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range branches.Data {
		if branch.Name == "" {
			t.Error("branch lacks a name")
		}
	}
	runPromotionSelectionReadCLI(t, instance, []string{"promotion", "provider", "branches", providerID, repository.FullPath, "--limit", "10"})
}

// The supplied project must already have a resolved, cloned promotion connection.
// These tests never clone or apply/promote content on the shared instance.
func TestPromotionProjectChanges(t *testing.T) {
	instance := integration.Require(t)
	projectID := os.Getenv(integration.EnvProjectID)
	if projectID == "" {
		t.Skip("set N8N_INTEGRATION_PROJECT_ID to a project with a cloned promotion direction")
	}
	client := instance.Client(t)
	for _, direction := range []string{n8n.PromotionDirectionApply, n8n.PromotionDirectionPromote} {
		t.Run(direction, func(t *testing.T) {
			changes, err := client.GetPromotionChanges(context.Background(), projectID, direction, n8n.PromotionChangesOptions{})
			skipUnlessPromotions(t, err)
			if err != nil {
				t.Fatal(err)
			}
			if changes.Source.ConfigID == "" {
				t.Error("changes lack source configId")
			}
			for _, change := range changes.Changes {
				if change.ID == "" || change.Type != "workflow" {
					t.Errorf("invalid change: %+v", change)
				}
			}
			runPromotionSelectionReadCLI(t, instance, []string{"promotion", "project", "changes", projectID, direction})
		})
	}
}

func runPromotionSelectionReadCLI(t *testing.T, instance integration.Instance, args []string) {
	t.Helper()
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var stdout, stderr strings.Builder
	code := cli.Run(context.Background(), append(args, "--output", "json"), cli.Options{
		Streams: cli.Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr},
		Version: version.Get(), ConfigDir: t.TempDir(), Env: func(name string) string { return env[name] },
		Keyring: config.NewMemoryStore(config.StorageKeyring), Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		for _, status := range []string{"403", "404", "503"} {
			if strings.Contains(stderr.String(), status) {
				t.Skip("promotion read unavailable to integration credential")
			}
		}
		t.Fatalf("%v exited %d: %s", args, code, stderr.String())
	}
	if !json.Valid([]byte(stdout.String())) {
		t.Fatalf("invalid JSON: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
