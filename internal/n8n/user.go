package n8n

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

// UsersPath is the collection endpoint for instance users.
const UsersPath = "/users"

// User is one public n8n user. Role is empty when includeRole was not requested.
type User struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	IsPending  bool   `json:"isPending"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Role       string `json:"role,omitempty"`
	MFAEnabled bool   `json:"mfaEnabled"`
}

// ListUsersOptions contains pagination and user-specific filters.
type ListUsersOptions struct {
	ListOptions
	// Offset skips this many users. It cannot be combined with Cursor.
	Offset int
	// IncludeRole asks n8n to include each user's global role.
	IncludeRole bool
	// ProjectID limits results to members of one project.
	ProjectID string
}

// Validate rejects malformed list options before transport.
func (o ListUsersOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.Limit)
	}
	if o.Offset < 0 {
		return fmt.Errorf("offset must not be negative, got %d", o.Offset)
	}
	if o.Offset > 0 && o.Cursor != "" {
		return fmt.Errorf("offset and cursor cannot be used together")
	}
	return validateUserText("project ID", o.ProjectID, false)
}

func (o ListUsersOptions) apply() url.Values {
	query := o.ListOptions.Apply(nil)
	if o.Offset > 0 {
		query.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.IncludeRole {
		query.Set("includeRole", "true")
	}
	if o.ProjectID != "" {
		query.Set("projectId", o.ProjectID)
	}
	return query
}

// CreateUserRequest is one invitation in a bulk user creation request.
type CreateUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

// InvitedUser describes one successfully created or re-invited user.
type InvitedUser struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	InviteAcceptURL string `json:"inviteAcceptUrl,omitempty"`
	EmailSent       bool   `json:"emailSent"`
	Role            string `json:"role,omitempty"`
}

// CreateUserResult preserves one per-invitation success or error returned by
// n8n. User can be present alongside Error when creation succeeded but email
// delivery failed.
type CreateUserResult struct {
	User  *InvitedUser `json:"user,omitempty"`
	Error string       `json:"error,omitempty"`
}

// ListUsers returns one page of users.
func (c *Client) ListUsers(ctx context.Context, opts ListUsersOptions) (Page[User], error) {
	if err := opts.Validate(); err != nil {
		return Page[User]{}, err
	}
	var page Page[User]
	if _, err := c.Do(ctx, Request{Path: UsersPath, Query: opts.apply()}, &page); err != nil {
		return Page[User]{}, err
	}
	return page, nil
}

// CreateUsers creates or invites one or more users and preserves every
// per-user result, including partial email-delivery failures.
func (c *Client) CreateUsers(ctx context.Context, requests []CreateUserRequest) ([]CreateUserResult, error) {
	if err := validateCreateUsers(requests); err != nil {
		return nil, err
	}
	var results []CreateUserResult
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: UsersPath, Body: requests}, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// GetUser returns one user by ID or email. identifier is escaped as one path
// segment, so email addresses and reserved characters cannot alter the path.
func (c *Client) GetUser(ctx context.Context, identifier string, includeRole bool) (*User, error) {
	if err := validateUserIdentifier(identifier); err != nil {
		return nil, err
	}
	query := make(url.Values)
	if includeRole {
		query.Set("includeRole", "true")
	}
	var user User
	if _, err := c.Do(ctx, Request{Path: PathJoin("users", identifier), Query: query}, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// ChangeUserRole changes one user's global role by user ID or email.
func (c *Client) ChangeUserRole(ctx context.Context, identifier, role string) error {
	if err := validateUserIdentifier(identifier); err != nil {
		return err
	}
	if err := validateUserText("role slug", role, true); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("users", identifier, "role"),
		Body: struct {
			NewRoleName string `json:"newRoleName"`
		}{NewRoleName: role},
	}, nil)
	return err
}

// DeleteUser permanently removes one user by ID or email.
func (c *Client) DeleteUser(ctx context.Context, identifier string) error {
	if err := validateUserIdentifier(identifier); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("users", identifier)}, nil)
	return err
}

func validateCreateUsers(requests []CreateUserRequest) error {
	if len(requests) == 0 {
		return fmt.Errorf("at least one user invitation is required")
	}
	for i, request := range requests {
		if err := validateUserEmail(request.Email); err != nil {
			return fmt.Errorf("user invitation %d: %w", i+1, err)
		}
		if err := validateUserText("role slug", request.Role, false); err != nil {
			return fmt.Errorf("user invitation %d: %w", i+1, err)
		}
	}
	return nil
}

func validateUserEmail(email string) error {
	if err := validateUserText("email", email, true); err != nil {
		return err
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		return fmt.Errorf("user email %q is invalid", email)
	}
	return nil
}

func validateUserIdentifier(identifier string) error {
	return validateUserText("ID or email", identifier, true)
}

func validateUserText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("user %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("user %s must not start or end with whitespace", field)
	}
	return nil
}
