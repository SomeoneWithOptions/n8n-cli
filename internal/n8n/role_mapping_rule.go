package n8n

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// RoleMappingRulesPath is the collection endpoint for identity-provider
// role-mapping rules.
const RoleMappingRulesPath = "/role-mapping-rules"

// maxRoleMappingRoleLength is the role slug limit the API schema declares for
// create and update documents.
const maxRoleMappingRoleLength = 128

// RoleMappingRuleType selects which role a rule grants: a global role on the
// instance, or a project role on the projects the rule names.
type RoleMappingRuleType string

const (
	RoleMappingRuleTypeInstance RoleMappingRuleType = "instance"
	RoleMappingRuleTypeProject  RoleMappingRuleType = "project"
)

// RoleMappingRule maps an identity-provider claim expression to a role.
//
// Order is the rule's evaluation position within its own type, so instance and
// project rules each have their own sequence starting at 0.
type RoleMappingRule struct {
	ID         string              `json:"id"`
	Expression string              `json:"expression"`
	Role       string              `json:"role"`
	Type       RoleMappingRuleType `json:"type"`
	Order      int                 `json:"order"`
	ProjectIDs []string            `json:"projectIds"`
	CreatedAt  string              `json:"createdAt"`
	UpdatedAt  string              `json:"updatedAt"`
}

// ListRoleMappingRulesOptions contains pagination and the documented type
// filter. Filtering by type is what turns the response into a single
// evaluation order.
type ListRoleMappingRulesOptions struct {
	ListOptions
	// Type limits results to instance or project rules. Empty returns both.
	Type RoleMappingRuleType
}

// Validate rejects options the API would reject before transport.
func (o ListRoleMappingRulesOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Type == "" {
		return nil
	}
	return validateRoleMappingRuleType(o.Type)
}

func (o ListRoleMappingRulesOptions) apply() url.Values {
	query := o.ListOptions.Apply(nil)
	if o.Type != "" {
		query.Set("type", string(o.Type))
	}
	return query
}

// CreateRoleMappingRuleRequest is the rule creation document. Order is a
// pointer because omitting it appends the rule to the end of its type's
// evaluation order, which is not the same as position 0.
type CreateRoleMappingRuleRequest struct {
	Expression string              `json:"expression"`
	Role       string              `json:"role"`
	Type       RoleMappingRuleType `json:"type"`
	Order      *int                `json:"order,omitempty"`
	ProjectIDs []string            `json:"projectIds,omitempty"`
}

// Validate rejects malformed creation documents before transport.
func (r CreateRoleMappingRuleRequest) Validate() error {
	if err := validateRoleMappingExpression(r.Expression); err != nil {
		return err
	}
	if err := validateRoleMappingRole(r.Role); err != nil {
		return err
	}
	if err := validateRoleMappingRuleType(r.Type); err != nil {
		return err
	}
	if r.Order != nil && *r.Order < 0 {
		return fmt.Errorf("role-mapping rule order must not be negative, got %d", *r.Order)
	}
	if err := validateRoleMappingProjectIDs(r.ProjectIDs); err != nil {
		return err
	}
	if r.Type == RoleMappingRuleTypeInstance && len(r.ProjectIDs) > 0 {
		return fmt.Errorf("project IDs apply only to %q rules: an %q rule grants a global role",
			RoleMappingRuleTypeProject, RoleMappingRuleTypeInstance)
	}
	return nil
}

// UpdateRoleMappingRuleRequest is the partial update document. Every field is
// optional, and ProjectIDs is a pointer so an explicit empty array, which
// clears the rule's projects, survives encoding.
//
// A rule's type cannot change, and reordering is the move operation, so
// neither is accepted here.
type UpdateRoleMappingRuleRequest struct {
	Expression *string   `json:"expression,omitempty"`
	Role       *string   `json:"role,omitempty"`
	ProjectIDs *[]string `json:"projectIds,omitempty"`
}

// Validate rejects empty or malformed update documents before transport.
func (r UpdateRoleMappingRuleRequest) Validate() error {
	if r.Expression == nil && r.Role == nil && r.ProjectIDs == nil {
		return fmt.Errorf("role-mapping rule update is empty: set expression, role, or projectIds")
	}
	if r.Expression != nil {
		if err := validateRoleMappingExpression(*r.Expression); err != nil {
			return err
		}
	}
	if r.Role != nil {
		if err := validateRoleMappingRole(*r.Role); err != nil {
			return err
		}
	}
	if r.ProjectIDs != nil {
		return validateRoleMappingProjectIDs(*r.ProjectIDs)
	}
	return nil
}

// MoveRoleMappingRuleRequest positions a rule inside its own type's evaluation
// order. An index beyond the last position moves the rule to the end.
type MoveRoleMappingRuleRequest struct {
	TargetIndex int `json:"targetIndex"`
}

// Validate rejects negative positions before transport.
func (r MoveRoleMappingRuleRequest) Validate() error {
	if r.TargetIndex < 0 {
		return fmt.Errorf("target index must not be negative, got %d", r.TargetIndex)
	}
	return nil
}

// ListRoleMappingRules returns one cursor-paginated page of rules.
func (c *Client) ListRoleMappingRules(ctx context.Context, opts ListRoleMappingRulesOptions) (Page[RoleMappingRule], error) {
	if err := opts.Validate(); err != nil {
		return Page[RoleMappingRule]{}, err
	}
	var page Page[RoleMappingRule]
	if _, err := c.Do(ctx, Request{Path: RoleMappingRulesPath, Query: opts.apply()}, &page); err != nil {
		return Page[RoleMappingRule]{}, err
	}
	return page, nil
}

// CreateRoleMappingRule creates one rule and returns it with its assigned
// position in the evaluation order.
func (c *Client) CreateRoleMappingRule(ctx context.Context, request CreateRoleMappingRuleRequest) (*RoleMappingRule, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var rule RoleMappingRule
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   RoleMappingRulesPath,
		Body:   request,
	}, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// MoveRoleMappingRule changes a rule's position within its type's evaluation
// order. id is escaped as one path segment.
func (c *Client) MoveRoleMappingRule(ctx context.Context, id string, request MoveRoleMappingRuleRequest) (*RoleMappingRule, error) {
	if err := validateRoleMappingRuleID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var rule RoleMappingRule
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("role-mapping-rules", id, "move"),
		Body:   request,
	}, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// UpdateRoleMappingRule patches a rule's expression, role, or projects.
func (c *Client) UpdateRoleMappingRule(ctx context.Context, id string, request UpdateRoleMappingRuleRequest) (*RoleMappingRule, error) {
	if err := validateRoleMappingRuleID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var rule RoleMappingRule
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("role-mapping-rules", id),
		Body:   request,
	}, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// DeleteRoleMappingRule removes one rule and returns it. The remaining rules of
// the same type close the gap, so their order values stay contiguous.
func (c *Client) DeleteRoleMappingRule(ctx context.Context, id string) (*RoleMappingRule, error) {
	if err := validateRoleMappingRuleID(id); err != nil {
		return nil, err
	}
	var rule RoleMappingRule
	if _, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("role-mapping-rules", id),
	}, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func validateRoleMappingRuleID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("role-mapping rule ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("role-mapping rule ID must not start or end with whitespace")
	}
	return nil
}

func validateRoleMappingExpression(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("role-mapping rule expression is required")
	}
	return nil
}

func validateRoleMappingRole(role string) error {
	if strings.TrimSpace(role) == "" {
		return fmt.Errorf("role-mapping rule role slug is required")
	}
	if strings.TrimSpace(role) != role {
		return fmt.Errorf("role-mapping rule role slug must not start or end with whitespace")
	}
	if utf8.RuneCountInString(role) > maxRoleMappingRoleLength {
		return fmt.Errorf("role-mapping rule role slug must contain at most %d characters", maxRoleMappingRoleLength)
	}
	return nil
}

func validateRoleMappingRuleType(ruleType RoleMappingRuleType) error {
	if ruleType != RoleMappingRuleTypeInstance && ruleType != RoleMappingRuleTypeProject {
		return fmt.Errorf("role-mapping rule type %q is invalid: use %q or %q",
			ruleType, RoleMappingRuleTypeInstance, RoleMappingRuleTypeProject)
	}
	return nil
}

func validateRoleMappingProjectIDs(ids []string) error {
	for i, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("project ID %d is empty", i+1)
		}
		if strings.TrimSpace(id) != id {
			return fmt.Errorf("project ID %q must not start or end with whitespace", id)
		}
	}
	return nil
}
