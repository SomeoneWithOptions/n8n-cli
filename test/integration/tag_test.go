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

// TestListTags is read-only: it pages through tags and checks the shape the
// client expects. An instance with no tags is a valid result.
func TestListTags(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListTags(context.Background(), n8n.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	for _, tag := range page.Data {
		if tag.ID == "" || tag.Name == "" {
			t.Errorf("tag is missing its ID or name: %+v", tag)
		}
	}
	t.Logf("instance returned %d tag(s), next cursor %q", len(page.Data), page.NextCursor)
}

// TestTagLifecycle creates one tag named with [integration.ResourcePrefix],
// reads it back, renames it and deletes it. It only ever touches the tag it
// created, and it refuses to start if that name already exists.
func TestTagLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()

	// n8n caps a tag name at 24 characters and reports a longer one as 409
	// "Tag already exists", so the generated name and its renamed form both
	// have to stay inside that budget: 13-character prefix plus 8 digits.
	name := fmt.Sprintf("%s%08d", integration.ResourcePrefix, time.Now().UnixNano()%1e8)
	existing, err := n8n.Collect(ctx, client.ListTags, n8n.ListOptions{}, 0)
	if err != nil {
		t.Fatalf("preflight ListTags: %v", err)
	}
	for _, tag := range existing {
		if tag.Name == name {
			t.Fatalf("refusing to touch pre-existing tag %q", name)
		}
	}

	created, err := client.CreateTag(ctx, name)
	if err != nil {
		t.Fatalf("CreateTag(%q): %v", name, err)
	}
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		if _, err := client.DeleteTag(context.Background(), created.ID); err != nil {
			t.Errorf("cleanup DeleteTag(%q): %v", created.ID, err)
		}
	})
	if created.ID == "" || created.Name != name {
		t.Fatalf("created tag = %+v, want name %q and an ID", created, name)
	}

	got, err := client.GetTag(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTag(%q): %v", created.ID, err)
	}
	if got.ID != created.ID || got.Name != name {
		t.Errorf("fetched tag = %+v, want %+v", got, created)
	}

	if _, err := client.CreateTag(ctx, name); !n8n.IsConflict(err) {
		t.Errorf("duplicate CreateTag error = %v, want 409", err)
	}

	renamed := name + "-r"
	updated, err := client.UpdateTag(ctx, created.ID, renamed)
	if err != nil {
		t.Fatalf("UpdateTag(%q): %v", created.ID, err)
	}
	if updated.ID != created.ID || updated.Name != renamed {
		t.Errorf("updated tag = %+v, want ID %q named %q", updated, created.ID, renamed)
	}

	if _, err := client.DeleteTag(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTag(%q): %v", created.ID, err)
	}
	deleted = true
	if _, err := client.GetTag(ctx, created.ID); !n8n.IsNotFound(err) {
		t.Errorf("GetTag after delete = %v, want 404", err)
	}
}

// TestTagListCommand runs the read-only list through the full CLI with an
// environment API key and a throwaway configuration directory.
func TestTagListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"tag", "list", "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("tag list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.Tag]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a tag page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
