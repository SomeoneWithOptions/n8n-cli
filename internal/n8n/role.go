package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// RolesPath is the collection endpoint for global and project roles.
const RolesPath = "/roles"

// RoleType identifies where a role may be assigned.
type RoleType string

const (
	RoleTypeGlobal  RoleType = "global"
	RoleTypeProject RoleType = "project"
)

// Role is one global or project role. Licensed is a pointer because create,
// update, and delete responses omit licensing while list and get include it.
// Usage counts are present only when requested.
type Role struct {
	Slug           string   `json:"slug"`
	DisplayName    string   `json:"displayName"`
	Description    *string  `json:"description"`
	SystemRole     bool     `json:"systemRole"`
	RoleType       RoleType `json:"roleType"`
	Scopes         []string `json:"scopes"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
	Licensed       *bool    `json:"licensed,omitempty"`
	UsedByUsers    *float64 `json:"usedByUsers,omitempty"`
	UsedByProjects *float64 `json:"usedByProjects,omitempty"`
}

// Roles groups roles by the assignment context accepted by n8n.
type Roles struct {
	Global  []Role `json:"global"`
	Project []Role `json:"project"`
}

// RoleDescription represents the required, nullable description in a role
// replacement document. Its zero value is unset and fails validation.
type RoleDescription struct {
	value string
	set   bool
	null  bool
}

// NewRoleDescription creates a non-null replacement description.
func NewRoleDescription(value string) RoleDescription {
	return RoleDescription{value: value, set: true}
}

// NullRoleDescription creates an explicit JSON null replacement description.
func NullRoleDescription() RoleDescription { return RoleDescription{set: true, null: true} }

// IsNull reports whether the description is explicitly null.
func (d RoleDescription) IsNull() bool { return d.set && d.null }

// Value returns the description text and whether it is non-null and set.
func (d RoleDescription) Value() (string, bool) { return d.value, d.set && !d.null }

// MarshalJSON emits the required string or null value.
func (d RoleDescription) MarshalJSON() ([]byte, error) {
	if !d.set || d.null {
		return []byte("null"), nil
	}
	return json.Marshal(d.value)
}

// UnmarshalJSON preserves the difference between a missing field and an
// explicit null so UpdateRoleRequest can enforce the OpenAPI contract.
func (d *RoleDescription) UnmarshalJSON(raw []byte) error {
	d.set = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		d.value = ""
		d.null = true
		return nil
	}
	if err := json.Unmarshal(raw, &d.value); err != nil {
		return fmt.Errorf("role description must be a string or null: %w", err)
	}
	d.null = false
	return nil
}

// CreateRoleRequest is the complete custom-role creation document. Scopes must
// be present but may be an empty array, as permitted by the API schema.
type CreateRoleRequest struct {
	DisplayName string   `json:"displayName"`
	Description *string  `json:"description,omitempty"`
	RoleType    RoleType `json:"roleType"`
	Scopes      []string `json:"scopes"`
}

// Validate rejects malformed role creation documents before transport.
func (r CreateRoleRequest) Validate() error {
	if err := validateRoleDisplayName(r.DisplayName); err != nil {
		return err
	}
	if r.Description != nil {
		if err := validateRoleDescription(*r.Description); err != nil {
			return err
		}
	}
	if err := validateRoleType(r.RoleType); err != nil {
		return err
	}
	return validateRoleScopes(r.Scopes)
}

// UpdateRoleRequest fully replaces a custom role's writable fields.
type UpdateRoleRequest struct {
	DisplayName string          `json:"displayName"`
	Description RoleDescription `json:"description"`
	Scopes      []string        `json:"scopes"`
}

// Validate enforces full-replacement fields, including an explicit string or
// null description and a present (possibly empty) scopes array.
func (r UpdateRoleRequest) Validate() error {
	if err := validateRoleDisplayName(r.DisplayName); err != nil {
		return err
	}
	if !r.Description.set {
		return fmt.Errorf("role description is required and must be a string or null")
	}
	if !r.Description.null {
		if err := validateRoleDescription(r.Description.value); err != nil {
			return err
		}
	}
	return validateRoleScopes(r.Scopes)
}

// ListRoles returns all roles grouped into global and project roles.
func (c *Client) ListRoles(ctx context.Context, withUsageCount bool) (*Roles, error) {
	var roles Roles
	if _, err := c.Do(ctx, Request{Path: RolesPath, Query: roleUsageQuery(withUsageCount)}, &roles); err != nil {
		return nil, err
	}
	return &roles, nil
}

// CreateRole creates a global or project custom role.
func (c *Client) CreateRole(ctx context.Context, request CreateRoleRequest) (*Role, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var role Role
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: RolesPath, Body: request}, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// GetRole returns one role by slug. slug is escaped as one path segment.
func (c *Client) GetRole(ctx context.Context, slug string, withUsageCount bool) (*Role, error) {
	if err := validateRoleSlug(slug); err != nil {
		return nil, err
	}
	var role Role
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("roles", slug),
		Query: roleUsageQuery(withUsageCount),
	}, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// UpdateRole fully replaces a custom role's writable fields. System roles are
// immutable and are rejected by the server.
func (c *Client) UpdateRole(ctx context.Context, slug string, request UpdateRoleRequest) (*Role, error) {
	if err := validateRoleSlug(slug); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var role Role
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("roles", slug),
		Body:   request,
	}, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// DeleteRole deletes a custom role. When reassignRoleSlug is non-empty, n8n
// moves assigned users to that role before deleting. System roles are rejected.
func (c *Client) DeleteRole(ctx context.Context, slug, reassignRoleSlug string) (*Role, error) {
	if err := validateRoleSlug(slug); err != nil {
		return nil, err
	}
	if reassignRoleSlug != "" {
		if err := validateRoleSlug(reassignRoleSlug); err != nil {
			return nil, fmt.Errorf("reassignment %w", err)
		}
	}
	query := make(url.Values)
	if reassignRoleSlug != "" {
		query.Set("reassignRoleSlug", reassignRoleSlug)
	}
	var role Role
	if _, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("roles", slug),
		Query:  query,
	}, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func roleUsageQuery(withUsageCount bool) url.Values {
	query := make(url.Values)
	if withUsageCount {
		query.Set("withUsageCount", "true")
	}
	return query
}

func validateRoleSlug(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("role slug is required")
	}
	if strings.TrimSpace(slug) != slug {
		return fmt.Errorf("role slug must not start or end with whitespace")
	}
	return nil
}

func validateRoleDisplayName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("role display name is required")
	}
	length := utf8.RuneCountInString(name)
	if length < 2 || length > 100 {
		return fmt.Errorf("role display name must contain 2 to 100 characters")
	}
	return nil
}

func validateRoleDescription(description string) error {
	if utf8.RuneCountInString(description) > 500 {
		return fmt.Errorf("role description must contain at most 500 characters")
	}
	return nil
}

func validateRoleType(roleType RoleType) error {
	if roleType != RoleTypeGlobal && roleType != RoleTypeProject {
		return fmt.Errorf("role type %q is invalid: use %q or %q", roleType, RoleTypeGlobal, RoleTypeProject)
	}
	return nil
}

func validateRoleScopes(scopes []string) error {
	if scopes == nil {
		return fmt.Errorf("role scopes are required; use an empty array for a role with no scopes")
	}
	for i, scope := range scopes {
		if strings.TrimSpace(scope) == "" {
			return fmt.Errorf("role scope %d is empty", i+1)
		}
		if strings.TrimSpace(scope) != scope {
			return fmt.Errorf("role scope %q must not start or end with whitespace", scope)
		}
	}
	return nil
}
