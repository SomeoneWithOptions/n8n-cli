package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestListWorkflows is read-only. It checks the page shape and the filters
// against the live contract without creating anything.
func TestListWorkflows(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListWorkflows(context.Background(), n8n.ListWorkflowsOptions{
		ListOptions:       n8n.ListOptions{Limit: 10},
		ExcludePinnedData: true,
	})
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	for _, workflow := range page.Data {
		if workflow.ID == "" || workflow.Name == "" {
			t.Errorf("workflow is missing its ID or name: %+v", workflow)
		}
	}
	t.Logf("instance returned %d workflow(s)", len(page.Data))
}

// workflowDefinition is a minimal, harmless workflow: one webhook trigger on a
// path unique to this test run. Publishing it registers that path and nothing
// else; it never runs on its own.
func workflowDefinition(name, path string) n8n.WorkflowDocument {
	doc := n8n.WorkflowDocument{
		"connections": json.RawMessage(`{}`),
		"settings":    json.RawMessage(`{"executionOrder":"v1"}`),
	}
	nodes := fmt.Sprintf(`[{
		"id":"11111111-1111-4111-8111-111111111111",
		"name":"Webhook",
		"type":"n8n-nodes-base.webhook",
		"typeVersion":2,
		"position":[0,0],
		"webhookId":"22222222-2222-4222-8222-222222222222",
		"parameters":{"path":%q,"httpMethod":"GET"}
	}]`, path)
	doc["nodes"] = json.RawMessage(nodes)
	if err := doc.Set("name", name); err != nil {
		panic(err)
	}
	return doc
}

// TestWorkflowLifecycle creates one prefixed workflow and exercises every
// workflow operation on it: read, round-trip update, history, versions, tags,
// publish, archive and delete. It only ever touches what it created itself.
func TestWorkflowLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	name := fmt.Sprintf("%sworkflow-%d", integration.ResourcePrefix, stamp)
	hookPath := fmt.Sprintf("%s%d", integration.ResourcePrefix, stamp)

	created, err := client.CreateWorkflow(ctx, workflowDefinition(name, hookPath))
	if err != nil {
		t.Fatalf("CreateWorkflow(%q): %v", name, err)
	}
	if created.ID == "" || created.Name != name || created.Active {
		t.Fatalf("created workflow = %+v, want an ID, the name, and no publication", created)
	}
	t.Cleanup(func() {
		if _, err := client.DeleteWorkflow(context.Background(), created.ID); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteWorkflow(%q): %v", created.ID, err)
		}
	})

	// Read, edit, write: the read-only fields have to be filtered out, or the
	// instance answers 400 "request/body/id is read-only".
	read, err := client.GetWorkflow(ctx, created.ID, false)
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	doc, err := read.Document()
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	update, dropped := doc.ForUpdate()
	if len(dropped) == 0 {
		t.Error("a workflow read from the API carried no read-only fields, which is unexpected")
	}
	if err := update.Set("name", name+"-renamed"); err != nil {
		t.Fatalf("Set name: %v", err)
	}
	updated, err := client.UpdateWorkflow(ctx, created.ID, update, n8n.UpdateWorkflowOptions{})
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if updated.Name != name+"-renamed" || updated.NodeCount() != 1 {
		t.Errorf("updated workflow = %+v, want the new name and its one node", updated)
	}

	// History and versions.
	history, err := client.ListWorkflowHistory(ctx, created.ID, n8n.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkflowHistory: %v", err)
	}
	if len(history.Data) == 0 {
		t.Fatal("history is empty, want at least the version just saved")
	}
	versionID := history.Data[0].VersionID
	stored, err := client.GetWorkflowVersion(ctx, created.ID, versionID)
	if err != nil {
		t.Fatalf("GetWorkflowVersion: %v", err)
	}
	if stored.VersionID != versionID || stored.WorkflowID != created.ID {
		t.Errorf("version = %+v, want the one that was asked for", stored)
	}
	legacy, err := client.GetWorkflowVersionLegacy(ctx, created.ID, versionID)
	if err != nil {
		t.Fatalf("GetWorkflowVersionLegacy: %v", err)
	}
	if legacy.VersionID != versionID {
		t.Errorf("deprecated route returned %+v, want the same version", legacy)
	}

	// Tags: the workflow starts with none, gets one, and is cleared again.
	// Tag names are capped on the instance: a long one comes back as 409
	// "Tag already exists", so the stamp is shortened here.
	tag, err := client.CreateTag(ctx, fmt.Sprintf("%stag-%d", integration.ResourcePrefix, stamp%100000))
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	t.Cleanup(func() {
		if _, err := client.DeleteTag(context.Background(), tag.ID); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteTag(%q): %v", tag.ID, err)
		}
	})
	attached, err := client.SetWorkflowTags(ctx, created.ID, []string{tag.ID})
	if err != nil {
		t.Fatalf("SetWorkflowTags: %v", err)
	}
	if len(attached) != 1 || attached[0].ID != tag.ID {
		t.Errorf("tags = %+v, want the one that was set", attached)
	}
	listed, err := client.GetWorkflowTags(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWorkflowTags: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("tags = %+v, want one", listed)
	}
	if cleared, err := client.SetWorkflowTags(ctx, created.ID, nil); err != nil || len(cleared) != 0 {
		t.Errorf("SetWorkflowTags(nil) = %+v, %v; want an empty set", cleared, err)
	}

	// Publication, through both the current and the deprecated route.
	published, err := client.PublishWorkflow(ctx, created.ID, n8n.PublishWorkflowRequest{})
	if err != nil {
		t.Fatalf("PublishWorkflow: %v", err)
	}
	if !published.Active {
		t.Errorf("published workflow = %+v, want active", published)
	}
	unpublished, err := client.UnpublishWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("UnpublishWorkflow: %v", err)
	}
	if unpublished.Active {
		t.Errorf("unpublished workflow = %+v, want inactive", unpublished)
	}
	if activated, err := client.ActivateWorkflow(ctx, created.ID, n8n.PublishWorkflowRequest{}); err != nil || !activated.Active {
		t.Errorf("ActivateWorkflow = %+v, %v; want the deprecated route to publish", activated, err)
	}
	if deactivated, err := client.DeactivateWorkflow(ctx, created.ID); err != nil || deactivated.Active {
		t.Errorf("DeactivateWorkflow = %+v, %v; want the deprecated route to unpublish", deactivated, err)
	}

	// Transfer, only when a project to move it into is configured.
	if projectID := os.Getenv(integration.EnvProjectID); projectID != "" {
		if err := client.TransferWorkflow(ctx, created.ID, projectID); err != nil {
			t.Fatalf("TransferWorkflow(%q): %v", projectID, err)
		}
		if _, err := client.GetWorkflow(ctx, created.ID, true); err != nil {
			t.Errorf("GetWorkflow after transfer: %v", err)
		}
	}

	// Archiving is the reversible soft delete, and idempotent.
	archived, err := client.ArchiveWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("ArchiveWorkflow: %v", err)
	}
	if !archived.IsArchived {
		t.Errorf("archived workflow = %+v, want isArchived", archived)
	}
	if again, err := client.ArchiveWorkflow(ctx, created.ID); err != nil || !again.IsArchived {
		t.Errorf("second ArchiveWorkflow = %+v, %v; want it to be idempotent", again, err)
	}
	restored, err := client.UnarchiveWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("UnarchiveWorkflow: %v", err)
	}
	if restored.IsArchived {
		t.Errorf("restored workflow = %+v, want it out of the archive", restored)
	}

	deleted, err := client.DeleteWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
	if deleted.ID != created.ID {
		t.Errorf("deleted workflow = %+v, want the one that was created", deleted)
	}
	if _, err := client.GetWorkflow(ctx, created.ID, false); !n8n.IsNotFound(err) {
		t.Errorf("GetWorkflow after delete = %v, want a 404", err)
	}
}

// TestWorkflowListCommand runs read-only listing through the full CLI.
func TestWorkflowListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(),
		[]string{"workflow", "list", "--limit", "10", "--exclude-pinned-data", "--output", "json"},
		cli.Options{
			Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
			Version:     version.Get(),
			ConfigDir:   t.TempDir(),
			Env:         func(name string) string { return env[name] },
			Keyring:     config.NewMemoryStore(config.StorageKeyring),
			Interactive: &interactive,
		})
	if code != cli.ExitSuccess {
		t.Fatalf("workflow list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.Workflow]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a workflow page: %v\n%s", err, out.String())
	}
	for _, workflow := range page.Data {
		if workflow.ID == "" {
			t.Errorf("workflow omitted its ID: %+v", workflow)
		}
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
