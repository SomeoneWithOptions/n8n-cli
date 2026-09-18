package n8n

import (
	"context"
	"fmt"
	"net/http"
)

// NodeTypePolicyInstancePath is the instance-scope node type policy endpoint.
const NodeTypePolicyInstancePath = "/node-type-policies/instance"

// NodeTypePolicyAction is what a rule or a scope default does with a node type.
type NodeTypePolicyAction string

const (
	// NodeTypePolicyAllow permits the node type.
	NodeTypePolicyAllow NodeTypePolicyAction = "allow"
	// NodeTypePolicyDeny blocks the node type.
	NodeTypePolicyDeny NodeTypePolicyAction = "deny"
	// NodeTypePolicyDelegate hands the decision to the next scope down. It is
	// an instance-scope action only; project scope rejects it.
	NodeTypePolicyDelegate NodeTypePolicyAction = "delegate"
)

// NodeTypePolicySelectorKind is how a rule addresses node types.
type NodeTypePolicySelectorKind string

const (
	// NodeTypePolicySelectorName matches one node type by name.
	NodeTypePolicySelectorName NodeTypePolicySelectorKind = "name"
	// NodeTypePolicySelectorPackage matches every node type in a package.
	NodeTypePolicySelectorPackage NodeTypePolicySelectorKind = "package"
)

// NodeTypePolicyScope is the policy scope an action set belongs to. The two
// scopes share a request shape but not their permitted actions.
type NodeTypePolicyScope string

const (
	// NodeTypePolicyScopeInstance is the instance-wide policy.
	NodeTypePolicyScopeInstance NodeTypePolicyScope = "instance"
	// NodeTypePolicyScopeProject is one project's own policy.
	NodeTypePolicyScopeProject NodeTypePolicyScope = "project"
)

// NodeTypePolicySelector addresses the node types one rule applies to.
type NodeTypePolicySelector struct {
	Kind  NodeTypePolicySelectorKind `json:"kind"`
	Value string                     `json:"value"`
}

// NodeTypePolicyRule is one rule of a policy document. Rules are evaluated in
// list order and the first match wins, so order is meaningful.
type NodeTypePolicyRule struct {
	ID       string                 `json:"id"`
	Action   NodeTypePolicyAction   `json:"action"`
	Selector NodeTypePolicySelector `json:"selector"`
}

// NodeTypePolicy is a composed policy for one scope: the scope default plus the
// rules of every attached policy document in evaluation order.
//
// ScopeID is nil for a scope that was never configured, which also reports no
// rules, an allow default, and version 0.
type NodeTypePolicy struct {
	ScopeID       *string              `json:"scopeId"`
	Rules         []NodeTypePolicyRule `json:"rules"`
	DefaultAction NodeTypePolicyAction `json:"defaultAction"`
	Version       int                  `json:"version"`
}

// Configured reports whether the scope has a policy document. An unconfigured
// scope allows every node type.
func (p NodeTypePolicy) Configured() bool { return p.ScopeID != nil }

// NodeTypePolicyWarning reports a rule that can never match because an earlier
// rule already decides the same node types.
type NodeTypePolicyWarning struct {
	RuleID           string `json:"ruleId"`
	ShadowedByRuleID string `json:"shadowedByRuleId"`
}

// NodeTypePolicyWriteResult is the policy as stored after a replacement, plus
// the shadowing warnings the server found. Warnings do not fail the write.
type NodeTypePolicyWriteResult struct {
	NodeTypePolicy
	Warnings []NodeTypePolicyWarning `json:"warnings"`
}

// ReplaceNodeTypePolicyRequest replaces the rules and default action of one
// scope's single policy document, creating the document on first write.
//
// Pointers preserve the difference between an omitted required field and a
// valid zero: version 0 is the version of a scope that was never configured.
// Version must equal the version last read or the server answers 409.
type ReplaceNodeTypePolicyRequest struct {
	Rules         []NodeTypePolicyRule  `json:"rules"`
	DefaultAction *NodeTypePolicyAction `json:"defaultAction"`
	Version       *int                  `json:"version"`
}

// Validate enforces the replacement contract for one scope before transport.
// Instance scope accepts allow, deny and delegate; project scope accepts allow
// and deny only.
func (r ReplaceNodeTypePolicyRequest) Validate(scope NodeTypePolicyScope) error {
	if r.DefaultAction == nil {
		return fmt.Errorf("defaultAction is required: %s", nodeTypePolicyActionHelp(scope))
	}
	if err := validateNodeTypePolicyAction(*r.DefaultAction, scope); err != nil {
		return fmt.Errorf("defaultAction: %w", err)
	}
	if r.Version == nil {
		return fmt.Errorf("version is required: send the version last read, or 0 for a scope that was never configured")
	}
	if *r.Version < 0 {
		return fmt.Errorf("version must be 0 or greater, got %d", *r.Version)
	}
	seen := make(map[string]int, len(r.Rules))
	for i, rule := range r.Rules {
		if rule.ID == "" {
			return fmt.Errorf("rules[%d]: id is required and must not be empty", i)
		}
		if first, ok := seen[rule.ID]; ok {
			return fmt.Errorf("rules[%d]: id %q is already used by rules[%d]; rule ids must be unique within the list", i, rule.ID, first)
		}
		seen[rule.ID] = i
		if err := validateNodeTypePolicyAction(rule.Action, scope); err != nil {
			return fmt.Errorf("rules[%d] (%s): %w", i, rule.ID, err)
		}
		switch rule.Selector.Kind {
		case NodeTypePolicySelectorName, NodeTypePolicySelectorPackage:
		default:
			return fmt.Errorf("rules[%d] (%s): selector.kind must be %s or %s, got %q",
				i, rule.ID, NodeTypePolicySelectorName, NodeTypePolicySelectorPackage, rule.Selector.Kind)
		}
		if rule.Selector.Value == "" {
			return fmt.Errorf("rules[%d] (%s): selector.value is required and must not be empty", i, rule.ID)
		}
	}
	return nil
}

func validateNodeTypePolicyAction(action NodeTypePolicyAction, scope NodeTypePolicyScope) error {
	switch action {
	case NodeTypePolicyAllow, NodeTypePolicyDeny:
		return nil
	case NodeTypePolicyDelegate:
		if scope == NodeTypePolicyScopeProject {
			return fmt.Errorf("delegate is an instance-scope action and is not accepted on a project policy: use allow or deny")
		}
		return nil
	default:
		return fmt.Errorf("action must be %s, got %q", nodeTypePolicyActionHelp(scope), action)
	}
}

func nodeTypePolicyActionHelp(scope NodeTypePolicyScope) string {
	if scope == NodeTypePolicyScopeProject {
		return "one of allow or deny"
	}
	return "one of allow, deny, or delegate"
}

// normalized returns the request with a non-nil rule list, so an empty policy
// is sent as [] rather than null.
func (r ReplaceNodeTypePolicyRequest) normalized() ReplaceNodeTypePolicyRequest {
	if r.Rules == nil {
		r.Rules = []NodeTypePolicyRule{}
	}
	return r
}

// GetInstanceNodeTypePolicy returns the composed instance-scope policy.
func (c *Client) GetInstanceNodeTypePolicy(ctx context.Context) (*NodeTypePolicy, error) {
	var policy NodeTypePolicy
	if _, err := c.Do(ctx, Request{Path: NodeTypePolicyInstancePath}, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// ReplaceInstanceNodeTypePolicy replaces the instance default action and the
// rules of its single policy document. A stale Version is rejected with 409.
func (c *Client) ReplaceInstanceNodeTypePolicy(ctx context.Context, request ReplaceNodeTypePolicyRequest) (*NodeTypePolicyWriteResult, error) {
	if err := request.Validate(NodeTypePolicyScopeInstance); err != nil {
		return nil, err
	}
	var result NodeTypePolicyWriteResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   NodeTypePolicyInstancePath,
		Body:   request.normalized(),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetProjectNodeTypePolicy returns the project's own composed policy. It is not
// combined with the instance policy; that composition happens at evaluation.
func (c *Client) GetProjectNodeTypePolicy(ctx context.Context, projectID string) (*NodeTypePolicy, error) {
	if err := validateNodeTypePolicyProjectID(projectID); err != nil {
		return nil, err
	}
	var policy NodeTypePolicy
	if _, err := c.Do(ctx, Request{Path: nodeTypePolicyProjectPath(projectID)}, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// ReplaceProjectNodeTypePolicy replaces one project's default action and the
// rules of its single policy document. A stale Version is rejected with 409, as
// is a policy document shared with another scope.
func (c *Client) ReplaceProjectNodeTypePolicy(ctx context.Context, projectID string, request ReplaceNodeTypePolicyRequest) (*NodeTypePolicyWriteResult, error) {
	if err := validateNodeTypePolicyProjectID(projectID); err != nil {
		return nil, err
	}
	if err := request.Validate(NodeTypePolicyScopeProject); err != nil {
		return nil, err
	}
	var result NodeTypePolicyWriteResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   nodeTypePolicyProjectPath(projectID),
		Body:   request.normalized(),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func nodeTypePolicyProjectPath(projectID string) string {
	return PathJoin("node-type-policies", "projects", projectID)
}

func validateNodeTypePolicyProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	return nil
}
