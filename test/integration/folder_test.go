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

// TestListFolders is read-only. It checks offset paging and the response shape
// against the live contract without creating anything.
func TestListFolders(t *testing.T) {
	instance := integration.Require(t)
	projectID := integration.RequireProjectID(t)
	page, err := instance.Client(t).ListFolders(context.Background(), projectID, n8n.ListFoldersOptions{
		Take:   5,
		SortBy: "name:asc",
	})
	if err != nil {
		t.Fatalf("ListFolders(%q): %v", projectID, err)
	}
	if page.Count < len(page.Data) {
		t.Errorf("count = %d, but the page carries %d folder(s)", page.Count, len(page.Data))
	}
	for _, folder := range page.Data {
		if folder.ID == "" || folder.Name == "" {
			t.Errorf("folder is missing its ID or name: %+v", folder)
		}
	}
	t.Logf("project %s holds %d folder(s)", projectID, page.Count)
}

// TestFolderLifecycle creates one prefixed folder and one prefixed child inside
// it, reads them back, renames the parent, filters on the parent, and deletes
// both. Nothing that existed before the test is read for anything but paging,
// and nothing but these two folders is ever written to.
func TestFolderLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	projectID := integration.RequireProjectID(t)
	client := instance.Client(t)
	ctx := context.Background()
	name := fmt.Sprintf("%sfolder-%d", integration.ResourcePrefix, time.Now().UnixNano())

	parent, err := client.CreateFolder(ctx, projectID, n8n.CreateFolderRequest{Name: name})
	if err != nil {
		t.Fatalf("CreateFolder(%q): %v", name, err)
	}
	if parent.ID == "" {
		t.Fatalf("created folder has no ID: %+v", parent)
	}
	t.Cleanup(func() {
		if err := client.DeleteFolder(context.Background(), projectID, parent.ID, ""); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteFolder(%q): %v", parent.ID, err)
		}
	})

	child, err := client.CreateFolder(ctx, projectID, n8n.CreateFolderRequest{
		Name:           name + "-child",
		ParentFolderID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateFolder(child): %v", err)
	}
	if child.ParentFolderID != parent.ID {
		t.Errorf("child parent = %q, want %q", child.ParentFolderID, parent.ID)
	}

	renamed := name + "-renamed"
	if _, err := client.UpdateFolder(ctx, projectID, parent.ID, n8n.UpdateFolderRequest{Name: renamed}); err != nil {
		t.Fatalf("UpdateFolder(%q): %v", parent.ID, err)
	}

	detail, err := client.GetFolder(ctx, projectID, parent.ID)
	if err != nil {
		t.Fatalf("GetFolder(%q): %v", parent.ID, err)
	}
	if detail.Name != renamed {
		t.Errorf("folder name = %q, want %q", detail.Name, renamed)
	}
	if detail.TotalSubFolders != 1 {
		t.Errorf("total sub-folders = %d, want 1", detail.TotalSubFolders)
	}

	page, err := client.ListFolders(ctx, projectID, n8n.ListFoldersOptions{
		Filter: n8n.FolderFilter{ParentFolderID: parent.ID},
	})
	if err != nil {
		t.Fatalf("ListFolders(filtered): %v", err)
	}
	if page.Count != 1 || len(page.Data) != 1 || page.Data[0].ID != child.ID {
		t.Errorf("filtered page = %+v, want only the child folder", page)
	}

	// Deleting the parent without a transfer target deletes the child with it,
	// which is the documented behaviour and what the CLI warns about.
	if err := client.DeleteFolder(ctx, projectID, parent.ID, ""); err != nil {
		t.Fatalf("DeleteFolder(%q): %v", parent.ID, err)
	}
	if _, err := client.GetFolder(ctx, projectID, child.ID); !n8n.IsNotFound(err) {
		t.Errorf("GetFolder(child) after parent deletion = %v, want a 404", err)
	}
}

// TestFolderListCommand runs read-only listing through the full CLI.
func TestFolderListCommand(t *testing.T) {
	instance := integration.Require(t)
	projectID := integration.RequireProjectID(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"folder", "list", projectID, "--take", "5", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("folder list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.FolderPage
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a folder page: %v\n%s", err, out.String())
	}
	for _, folder := range page.Data {
		if folder.ID == "" {
			t.Errorf("folder omitted its ID: %+v", folder)
		}
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
