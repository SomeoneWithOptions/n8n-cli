package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// skipUnlicensed turns the 403 an instance without the projects entitlement
// returns into a skip: the code under test is correct there, the feature is
// simply not enabled.
func skipUnlicensed(t *testing.T, err error) {
	t.Helper()
	if n8n.IsForbidden(err) {
		t.Skipf("instance denies the projects API (403): %v", err)
	}
}

// TestListProjects is read-only. It verifies cursor-page decoding against the
// live contract without creating anything.
func TestListProjects(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListProjects(context.Background(), n8n.ListOptions{Limit: 10})
	if err != nil {
		skipUnlicensed(t, err)
		t.Fatalf("ListProjects: %v", err)
	}
	for _, project := range page.Data {
		if project.ID == "" || project.Name == "" {
			t.Errorf("project is missing ID or name: %+v", project)
		}
	}
}

// TestProjectLifecycle creates one prefixed project, renames it, reads its
// member list, and deletes it again. It only ever touches the project it
// created itself and needs the destructive opt-in.
func TestProjectLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	name := fmt.Sprintf("%sproject-%d", integration.ResourcePrefix, time.Now().UnixNano())

	created, err := client.CreateProject(ctx, name)
	if err != nil {
		skipUnlicensed(t, err)
		t.Fatalf("CreateProject(%q): %v", name, err)
	}

	id := created.ID
	if id == "" {
		// The contract declares no response body, so fall back to a lookup.
		projects, err := n8n.Collect(ctx, client.ListProjects, n8n.ListOptions{Limit: 100}, 0)
		if err != nil {
			t.Fatalf("ListProjects after create: %v", err)
		}
		for _, project := range projects {
			if project.Name == name {
				id = project.ID
			}
		}
	}
	if id == "" {
		t.Fatalf("created project %q was not found in the project list", name)
	}
	t.Cleanup(func() {
		if err := client.DeleteProject(context.Background(), id); err != nil {
			t.Errorf("cleanup DeleteProject(%q): %v", id, err)
		}
	})

	renamed := name + "-renamed"
	if err := client.UpdateProject(ctx, id, renamed); err != nil {
		t.Fatalf("UpdateProject(%q): %v", id, err)
	}

	members, err := client.ListProjectUsers(ctx, id, n8n.ListOptions{Limit: 10})
	if err != nil {
		if n8n.IsForbidden(err) {
			t.Logf("member listing denied (needs user:list): %v", err)
		} else {
			t.Fatalf("ListProjectUsers(%q): %v", id, err)
		}
	}
	for _, member := range members.Data {
		if member.ID == "" || member.Role == "" {
			t.Errorf("member is missing ID or project role: %+v", member)
		}
	}

	// Membership changes are left out on purpose: they alter access for users
	// this suite did not create.
	//
	// The API has no single-project read, so the list stands in for one and
	// confirms the rename landed on the project we created.
	projects, err := n8n.Collect(ctx, client.ListProjects, n8n.ListOptions{Limit: 100}, 0)
	if err != nil {
		t.Fatalf("ListProjects after update: %v", err)
	}
	found := false
	for _, project := range projects {
		if project.ID == id {
			found = true
			if project.Name != renamed {
				t.Errorf("project name = %q, want %q", project.Name, renamed)
			}
		}
	}
	if !found {
		t.Errorf("project %q disappeared before cleanup", id)
	}
}

// TestProjectListCommand runs read-only listing through the full CLI.
func TestProjectListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"project", "list", "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") {
			t.Skipf("instance denies the projects API: %s", errOut.String())
		}
		t.Fatalf("project list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.Project]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a project page: %v\n%s", err, out.String())
	}
	for _, project := range page.Data {
		if project.ID == "" {
			t.Errorf("project omitted its ID: %+v", project)
		}
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
