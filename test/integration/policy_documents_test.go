package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

func skipUnlessPolicyAPI(t *testing.T, err error) {
	t.Helper()
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skipf("policy Preview API unavailable to integration credential: %v", err)
	}
}
func integrationPolicyAPIs(c *n8n.Client) map[string]n8n.PolicyClient {
	return map[string]n8n.PolicyClient{"node-policy": c.NodeTypePolicies(), "credential-policy": c.CredentialTypePolicies()}
}

// Reads never change instance/project attachments or existing documents.
func TestPolicyDocumentsReadOnly(t *testing.T) {
	instance := integration.Require(t)
	ctx := context.Background()
	for group, api := range integrationPolicyAPIs(instance.Client(t)) {
		t.Run(group, func(t *testing.T) {
			page, err := api.ListDocuments(ctx, n8n.ListOptions{Limit: 10})
			skipUnlessPolicyAPI(t, err)
			if err != nil {
				t.Fatal(err)
			}
			for _, doc := range page.Data {
				got, err := api.GetDocument(ctx, doc.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.ID != doc.ID || got.Version < 1 {
					t.Errorf("invalid policy document: %+v", got)
				}
			}
			scope, err := api.GetInstance(ctx)
			skipUnlessPolicyAPI(t, err)
			if err != nil {
				t.Fatal(err)
			}
			if scope.ScopeID != nil {
				attachments, err := api.GetAttachments(ctx, *scope.ScopeID)
				skipUnlessPolicyAPI(t, err)
				if err != nil {
					t.Fatal(err)
				}
				if attachments.Version < 1 {
					t.Errorf("invalid attachments version %d", attachments.Version)
				}
			}
			out := runNodePolicyCLI(t, instance, group, "document", "list", "--limit", "10", "--output", "json")
			var fromCLI n8n.Page[n8n.PolicyDocument]
			if err := json.Unmarshal([]byte(out), &fromCLI); err != nil {
				t.Fatalf("CLI JSON: %v", err)
			}
			if group == "credential-policy" {
				out = runNodePolicyCLI(t, instance, group, "instance", "get", "--output", "json")
				var policy n8n.NodeTypePolicy
				if err := json.Unmarshal([]byte(out), &policy); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Documents have no name field; use the required prefix on their unique rule IDs.
// They stay unattached, so this lifecycle never changes existing scope evaluation.
func TestPolicyDocumentLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	ctx := context.Background()
	for group, api := range integrationPolicyAPIs(instance.Client(t)) {
		t.Run(group, func(t *testing.T) {
			ruleID := fmt.Sprintf("%spolicy-%d", integration.ResourcePrefix, time.Now().UnixNano())
			rules := []n8n.NodeTypePolicyRule{{ID: ruleID, Action: n8n.NodeTypePolicyAllow, Selector: n8n.NodeTypePolicySelector{Kind: n8n.NodeTypePolicySelectorPackage, Value: integration.ResourcePrefix + "uninstalled"}}}
			created, err := api.CreateDocument(ctx, n8n.CreatePolicyDocumentRequest{Rules: rules})
			skipUnlessPolicyAPI(t, err)
			if err != nil {
				t.Fatal(err)
			}
			id := created.Policy.ID
			deleted := false
			t.Cleanup(func() {
				if !deleted && id != "" {
					if err := api.DeleteDocument(context.Background(), id); err != nil {
						t.Errorf("cleanup document %q: %v", id, err)
					}
				}
			})
			if id == "" || created.Policy.Version < 1 {
				t.Fatalf("invalid created document: %+v", created.Policy)
			}
			got, err := api.GetDocument(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := api.UpdateDocument(ctx, id, n8n.UpdatePolicyDocumentRequest{Rules: []n8n.NodeTypePolicyRule{}, Version: &got.Version})
			if err != nil {
				t.Fatal(err)
			}
			if updated.Policy.Version <= got.Version || len(updated.Policy.Rules) != 0 {
				t.Errorf("replacement failed: %+v", updated.Policy)
			}
			if err := api.DeleteDocument(ctx, id); err != nil {
				t.Fatal(err)
			}
			deleted = true
		})
	}
}

// Attachment writes target only a newly-created test project. Cleanup detaches
// first, deletes the reusable document, then deletes the project. Existing
// instance scopes and integration-supplied projects are never mutated.
func TestPolicyAttachmentsOwnedProject(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	for group, api := range integrationPolicyAPIs(client) {
		t.Run(group, func(t *testing.T) {
			if _, err := api.ListDocuments(ctx, n8n.ListOptions{Limit: 1}); err != nil {
				skipUnlessPolicyAPI(t, err)
				t.Fatal(err)
			}
			name := fmt.Sprintf("%spolicy-%d", integration.ResourcePrefix, time.Now().UnixNano())
			project, err := client.CreateProject(ctx, n8n.CreateProjectRequest{Name: name})
			skipUnlessPolicyAPI(t, err)
			if err != nil {
				t.Fatal(err)
			}
			projectID := project.ID
			var scopeID, documentID string
			t.Cleanup(func() {
				cleanupCtx := context.Background()
				if scopeID != "" {
					current, err := api.GetAttachments(cleanupCtx, scopeID)
					if err != nil {
						t.Errorf("cleanup get attachments: %v", err)
					} else if _, err := api.ReplaceAttachments(cleanupCtx, scopeID, n8n.ReplacePolicyAttachmentsRequest{Version: &current.Version}); err != nil {
						t.Errorf("cleanup detach: %v", err)
					}
				}
				if documentID != "" {
					if err := api.DeleteDocument(cleanupCtx, documentID); err != nil {
						t.Errorf("cleanup delete policy: %v", err)
					}
				}
				if projectID == "" {
					projects, err := n8n.Collect(cleanupCtx, client.ListProjects, n8n.ListOptions{Limit: 100}, 0)
					if err != nil {
						t.Errorf("cleanup find project %q: %v", name, err)
					} else {
						for _, p := range projects {
							if p.Name == name {
								projectID = p.ID
								break
							}
						}
					}
				}
				if projectID != "" {
					if err := client.DeleteProject(cleanupCtx, projectID); err != nil {
						t.Errorf("cleanup delete project: %v", err)
					}
				}
			})
			if projectID == "" {
				projects, err := n8n.Collect(ctx, client.ListProjects, n8n.ListOptions{Limit: 100}, 0)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range projects {
					if p.Name == name {
						projectID = p.ID
						break
					}
				}
			}
			if projectID == "" {
				t.Fatal("created test project could not be found")
			}
			initial, err := api.GetProject(ctx, projectID)
			skipUnlessPolicyAPI(t, err)
			if err != nil {
				t.Fatal(err)
			}
			allow := n8n.NodeTypePolicyAllow
			scope, err := api.ReplaceProject(ctx, projectID, n8n.ReplaceNodeTypePolicyRequest{Rules: []n8n.NodeTypePolicyRule{}, DefaultAction: &allow, Version: &initial.Version})
			skipUnlessPolicyAPI(t, err)
			if err != nil {
				t.Fatal(err)
			}
			if scope.ScopeID != nil {
				scopeID = *scope.ScopeID
			}
			if scopeID == "" {
				t.Fatal("scope write did not return scopeId")
			}
			doc, err := api.CreateDocument(ctx, n8n.CreatePolicyDocumentRequest{Rules: []n8n.NodeTypePolicyRule{{ID: name, Action: n8n.NodeTypePolicyAllow, Selector: n8n.NodeTypePolicySelector{Kind: n8n.NodeTypePolicySelectorPackage, Value: integration.ResourcePrefix + "uninstalled"}}}})
			if err != nil {
				t.Fatal(err)
			}
			documentID = doc.Policy.ID
			if documentID == "" {
				t.Fatal("missing policy ID")
			}
			current, err := api.GetAttachments(ctx, scopeID)
			if err != nil {
				t.Fatal(err)
			}
			priority := 0
			floor := false
			updated, err := api.ReplaceAttachments(ctx, scopeID, n8n.ReplacePolicyAttachmentsRequest{Attachments: []n8n.PolicyAttachmentInput{{PolicyID: documentID, Priority: &priority, IsFloor: &floor}}, Version: &current.Version})
			if err != nil {
				t.Fatal(err)
			}
			if len(updated.Attachments) != 1 || updated.Attachments[0].PolicyID != documentID || updated.Version <= current.Version {
				t.Fatalf("attachment write: %+v", updated)
			}
			// A document update must advance the attached scope's version too.
			if _, err := api.UpdateDocument(ctx, documentID, n8n.UpdatePolicyDocumentRequest{Rules: []n8n.NodeTypePolicyRule{}, Version: &doc.Policy.Version}); err != nil {
				t.Fatal(err)
			}
			after, err := api.GetAttachments(ctx, scopeID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Version <= updated.Version {
				t.Error("attached document update did not bump scope version")
			}
		})
	}
}
