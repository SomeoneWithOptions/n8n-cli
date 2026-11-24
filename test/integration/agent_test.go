package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// skipUnlessAgents skips on instances before n8n 2.49 or without the Agents
// module, and on credentials without the agent scopes.
func skipUnlessAgents(t *testing.T, err error) {
	t.Helper()
	if n8n.IsNotFound(err) || n8n.IsForbidden(err) {
		t.Skipf("agents unavailable to the integration credential: %v", err)
	}
}

// TestListAgents is read-only.
func TestListAgents(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListAgents(context.Background(), n8n.AgentListOptions{ListOptions: n8n.ListOptions{Limit: 10}})
	skipUnlessAgents(t, err)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	for _, agent := range page.Data {
		if agent.ID == "" || agent.ProjectID == "" {
			t.Errorf("agent is missing its ID or project: %+v", agent)
		}
	}
}

// TestAgentLifecycle creates an unconfigured draft named with
// [integration.ResourcePrefix] in the caller's personal project, reads it
// back, and deletes it. It touches only the Agent it created.
func TestAgentLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	name := fmt.Sprintf("%sagent-%d", integration.ResourcePrefix, time.Now().UnixNano())

	created, err := client.CreateAgent(ctx, n8n.CreateAgentRequest{Name: name})
	skipUnlessAgents(t, err)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		if _, err := client.DeleteAgent(context.Background(), created.ID); err != nil {
			t.Errorf("cleanup DeleteAgent(%q): %v", created.ID, err)
		}
	})
	if created.ID == "" || created.Name != name {
		t.Fatalf("created = %+v, want name %q and an ID", created, name)
	}
	got, err := client.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.ID != created.ID || got.Name != name {
		t.Errorf("fetched = %+v, want %+v", got, created)
	}
	if _, err := client.DeleteAgent(ctx, created.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	deleted = true
	if _, err := client.GetAgent(ctx, created.ID); !n8n.IsNotFound(err) {
		t.Errorf("GetAgent after delete: err = %v, want 404", err)
	}
}
