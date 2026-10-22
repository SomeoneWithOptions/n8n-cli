package n8n

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// PolicyClient implements the shared Preview API for node and credential policies.
// Obtain it from Client.NodeTypePolicies or Client.CredentialTypePolicies.
type PolicyClient struct {
	client     *Client
	base       string
	credential bool
}

// NodeTypePolicies returns the node type policy API.
func (c *Client) NodeTypePolicies() PolicyClient {
	return PolicyClient{client: c, base: "node-type-policies"}
}

// CredentialTypePolicies returns the credential type policy API, including extends selectors.
func (c *Client) CredentialTypePolicies() PolicyClient {
	return PolicyClient{client: c, base: "credential-type-policies", credential: true}
}

// PolicyDocument is a reusable rule list. Kind is server-defined, without a closed enum.
type PolicyDocument struct {
	ID        string               `json:"id"`
	Kind      string               `json:"kind"`
	Rules     []NodeTypePolicyRule `json:"rules"`
	Version   int                  `json:"version"`
	UpdatedBy string               `json:"updatedBy"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}

// PolicyDocumentWriteResult contains the stored document and non-fatal warnings.
type PolicyDocumentWriteResult struct {
	Policy   PolicyDocument          `json:"policy"`
	Warnings []NodeTypePolicyWarning `json:"warnings"`
}

// CreatePolicyDocumentRequest contains only writable creation fields.
type CreatePolicyDocumentRequest struct {
	Rules []NodeTypePolicyRule `json:"rules"`
}

// UpdatePolicyDocumentRequest replaces every rule using the document's last-read version.
type UpdatePolicyDocumentRequest struct {
	Rules   []NodeTypePolicyRule `json:"rules"`
	Version *int                 `json:"version"`
}

// PolicyAttachment is an attached document, in server evaluation order.
type PolicyAttachment struct {
	PolicyID string               `json:"policyId"`
	Rules    []NodeTypePolicyRule `json:"rules"`
	Priority int                  `json:"priority"`
	IsFloor  bool                 `json:"isFloor"`
}

// PolicyAttachments contains attachments and the scope's version.
type PolicyAttachments struct {
	Attachments []PolicyAttachment `json:"attachments"`
	Version     int                `json:"version"`
}

// PolicyAttachmentInput distinguishes omitted required fields from false and zero.
type PolicyAttachmentInput struct {
	PolicyID string `json:"policyId"`
	Priority *int   `json:"priority"`
	IsFloor  *bool  `json:"isFloor"`
}

// ReplacePolicyAttachmentsRequest fully replaces a scope's attachments.
type ReplacePolicyAttachmentsRequest struct {
	Attachments []PolicyAttachmentInput `json:"attachments"`
	Version     *int                    `json:"version"`
}

func selectorHelp(credential bool) string {
	if credential {
		return "name, package, or extends"
	}
	return "name or package"
}
func validatePolicyID(kind, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s ID is required", kind)
	}
	return nil
}
func validatePolicyVersion(v *int) error {
	if v == nil {
		return fmt.Errorf("version is required: send the version last read")
	}
	if *v < 0 {
		return fmt.Errorf("version must be 0 or greater")
	}
	return nil
}

// ValidateRules checks selectors, actions, and unique rule IDs for this policy kind.
func (p PolicyClient) ValidateRules(rules []NodeTypePolicyRule, scope NodeTypePolicyScope) error {
	return validatePolicyRules(rules, scope, p.credential)
}

// ValidateScope checks a scope replacement before transport.
func (p PolicyClient) ValidateScope(r ReplaceNodeTypePolicyRequest, scope NodeTypePolicyScope) error {
	if !p.credential {
		return r.Validate(scope)
	}
	if r.DefaultAction == nil {
		return fmt.Errorf("defaultAction is required: %s", nodeTypePolicyActionHelp(scope))
	}
	if err := validateNodeTypePolicyAction(*r.DefaultAction, scope); err != nil {
		return fmt.Errorf("defaultAction: %w", err)
	}
	if err := validatePolicyVersion(r.Version); err != nil {
		return err
	}
	return p.ValidateRules(r.Rules, scope)
}

// Validate checks required fields and attachment uniqueness before transport.
func (r ReplacePolicyAttachmentsRequest) Validate() error {
	if err := validatePolicyVersion(r.Version); err != nil {
		return err
	}
	ids := map[string]bool{}
	pairs := map[struct {
		floor    bool
		priority int
	}]bool{}
	for i, a := range r.Attachments {
		if err := validatePolicyID("policy", a.PolicyID); err != nil {
			return fmt.Errorf("attachments[%d]: %w", i, err)
		}
		if a.Priority == nil || a.IsFloor == nil {
			return fmt.Errorf("attachments[%d]: priority and isFloor are required", i)
		}
		if ids[a.PolicyID] {
			return fmt.Errorf("attachments[%d]: policyId %q must be unique", i, a.PolicyID)
		}
		ids[a.PolicyID] = true
		pair := struct {
			floor    bool
			priority int
		}{*a.IsFloor, *a.Priority}
		if pairs[pair] {
			return fmt.Errorf("attachments[%d]: (isFloor, priority) pair must be unique", i)
		}
		pairs[pair] = true
	}
	return nil
}
func (p PolicyClient) scopePath(projectID string) string {
	if projectID == "" {
		return PathJoin(p.base, "instance")
	}
	return PathJoin(p.base, "projects", projectID)
}

// GetInstance returns the composed instance policy.
func (p PolicyClient) GetInstance(ctx context.Context) (*NodeTypePolicy, error) {
	return p.getScope(ctx, "")
}

// GetProject returns the composed project policy, without instance evaluation.
func (p PolicyClient) GetProject(ctx context.Context, id string) (*NodeTypePolicy, error) {
	if err := validatePolicyID("project", id); err != nil {
		return nil, err
	}
	return p.getScope(ctx, id)
}
func (p PolicyClient) getScope(ctx context.Context, id string) (*NodeTypePolicy, error) {
	result := NodeTypePolicy{Rules: []NodeTypePolicyRule{}}
	_, err := p.client.Do(ctx, Request{Path: p.scopePath(id)}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ReplaceInstance replaces only the instance scope's own document and default.
func (p PolicyClient) ReplaceInstance(ctx context.Context, r ReplaceNodeTypePolicyRequest) (*NodeTypePolicyWriteResult, error) {
	return p.replaceScope(ctx, "", r)
}

// ReplaceProject replaces only a project's own document and default.
func (p PolicyClient) ReplaceProject(ctx context.Context, id string, r ReplaceNodeTypePolicyRequest) (*NodeTypePolicyWriteResult, error) {
	if err := validatePolicyID("project", id); err != nil {
		return nil, err
	}
	return p.replaceScope(ctx, id, r)
}
func (p PolicyClient) replaceScope(ctx context.Context, id string, r ReplaceNodeTypePolicyRequest) (*NodeTypePolicyWriteResult, error) {
	scope := NodeTypePolicyScopeInstance
	if id != "" {
		scope = NodeTypePolicyScopeProject
	}
	if err := p.ValidateScope(r, scope); err != nil {
		return nil, err
	}
	result := NodeTypePolicyWriteResult{NodeTypePolicy: NodeTypePolicy{Rules: []NodeTypePolicyRule{}}, Warnings: []NodeTypePolicyWarning{}}
	_, err := p.client.Do(ctx, Request{Method: http.MethodPut, Path: p.scopePath(id), Body: r.normalized()}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListDocuments returns one cursor page, using a default page size of 100.
func (p PolicyClient) ListDocuments(ctx context.Context, o ListOptions) (Page[PolicyDocument], error) {
	if err := o.Validate(); err != nil {
		return Page[PolicyDocument]{}, err
	}
	if o.Limit > 250 {
		return Page[PolicyDocument]{}, fmt.Errorf("limit must be between 1 and 250")
	}
	if o.Limit == 0 {
		o.Limit = 100
	}
	var page Page[PolicyDocument]
	_, err := p.client.Do(ctx, Request{Path: PathJoin(p.base, "policies"), Query: o.Apply(nil)}, &page)
	if page.Data == nil {
		page.Data = []PolicyDocument{}
	}
	// A short page is terminal, even if a server erroneously offers another cursor.
	if len(page.Data) < o.Limit {
		page.NextCursor = ""
	}
	return page, err
}

// GetDocument retrieves a reusable policy document.
func (p PolicyClient) GetDocument(ctx context.Context, id string) (*PolicyDocument, error) {
	if err := validatePolicyID("policy", id); err != nil {
		return nil, err
	}
	result := PolicyDocument{Rules: []NodeTypePolicyRule{}}
	_, err := p.client.Do(ctx, Request{Path: PathJoin(p.base, "policies", id)}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateDocument creates an unattached reusable document.
func (p PolicyClient) CreateDocument(ctx context.Context, r CreatePolicyDocumentRequest) (*PolicyDocumentWriteResult, error) {
	if err := p.ValidateRules(r.Rules, NodeTypePolicyScopeInstance); err != nil {
		return nil, err
	}
	if r.Rules == nil {
		r.Rules = []NodeTypePolicyRule{}
	}
	return p.writeDocument(ctx, http.MethodPost, PathJoin(p.base, "policies"), r)
}

// UpdateDocument replaces every rule and bumps every attached scope's version.
func (p PolicyClient) UpdateDocument(ctx context.Context, id string, r UpdatePolicyDocumentRequest) (*PolicyDocumentWriteResult, error) {
	if err := validatePolicyID("policy", id); err != nil {
		return nil, err
	}
	if err := validatePolicyVersion(r.Version); err != nil {
		return nil, err
	}
	if err := p.ValidateRules(r.Rules, NodeTypePolicyScopeInstance); err != nil {
		return nil, err
	}
	if r.Rules == nil {
		r.Rules = []NodeTypePolicyRule{}
	}
	return p.writeDocument(ctx, http.MethodPut, PathJoin(p.base, "policies", id), r)
}
func (p PolicyClient) writeDocument(ctx context.Context, method, path string, body any) (*PolicyDocumentWriteResult, error) {
	result := PolicyDocumentWriteResult{Policy: PolicyDocument{Rules: []NodeTypePolicyRule{}}, Warnings: []NodeTypePolicyWarning{}}
	_, err := p.client.Do(ctx, Request{Method: method, Path: path, Body: body}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteDocument deletes an unattached document; attached documents return 409.
func (p PolicyClient) DeleteDocument(ctx context.Context, id string) error {
	if err := validatePolicyID("policy", id); err != nil {
		return err
	}
	_, err := p.client.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin(p.base, "policies", id)}, nil)
	return err
}

// GetAttachments returns attachments in floor-first, then ascending priority order.
func (p PolicyClient) GetAttachments(ctx context.Context, id string) (*PolicyAttachments, error) {
	if err := validatePolicyID("scope", id); err != nil {
		return nil, err
	}
	result := PolicyAttachments{Attachments: []PolicyAttachment{}}
	_, err := p.client.Do(ctx, Request{Path: PathJoin(p.base, "scopes", id, "attachments")}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ReplaceAttachments fully replaces attachments using the scope's last-read version.
func (p PolicyClient) ReplaceAttachments(ctx context.Context, id string, r ReplacePolicyAttachmentsRequest) (*PolicyAttachments, error) {
	if err := validatePolicyID("scope", id); err != nil {
		return nil, err
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if r.Attachments == nil {
		r.Attachments = []PolicyAttachmentInput{}
	}
	result := PolicyAttachments{Attachments: []PolicyAttachment{}}
	_, err := p.client.Do(ctx, Request{Method: http.MethodPut, Path: PathJoin(p.base, "scopes", id, "attachments"), Body: r}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
