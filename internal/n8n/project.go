package n8n

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ProjectsPath is the collection endpoint for projects.
const ProjectsPath = "/projects"

// maxProjectPageSize is the page size the API documents as its maximum.
const maxProjectPageSize = 250

// Project is one n8n project. Name is the only writable field: the API marks
// id and type read-only and rejects any other property.
type Project struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// projectRequest is the write document for create and update. It is separate
// from [Project] so read-only fields are never sent back to the server.
type projectRequest struct {
	Name string `json:"name"`
}

// ProjectMember is one user with their role inside a project. Every field is
// read-only; membership changes go through [Client.AddUsersToProject],
// [Client.ChangeUserRoleInProject] and [Client.DeleteUserFromProject].
type ProjectMember struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Role      string `json:"role"`
}

// ProjectRelation is one user-to-role assignment in an add-members request.
type ProjectRelation struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

// Validate rejects a malformed relation before transport.
func (r ProjectRelation) Validate() error {
	if err := validateProjectText("user ID", r.UserID, true); err != nil {
		return err
	}
	return validateProjectText("role", r.Role, true)
}

// ListProjects returns one cursor-paginated page of projects.
func (c *Client) ListProjects(ctx context.Context, opts ListOptions) (Page[Project], error) {
	if err := validateProjectListOptions(opts); err != nil {
		return Page[Project]{}, err
	}
	var page Page[Project]
	if _, err := c.Do(ctx, Request{Path: ProjectsPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[Project]{}, err
	}
	return page, nil
}

// CreateProject creates a project. The contract declares a 201 with no
// documented body, so the returned project carries whatever the instance sent
// and may be empty; list the projects to recover the ID in that case.
func (c *Client) CreateProject(ctx context.Context, name string) (*Project, error) {
	if err := validateProjectName(name); err != nil {
		return nil, err
	}
	var created Project
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   ProjectsPath,
		Body:   projectRequest{Name: name},
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateProject replaces a project. The request is a full replacement, but
// name is the only writable field, so renaming is the whole operation. id is
// escaped as one path segment. The documented 204 response has no body.
func (c *Client) UpdateProject(ctx context.Context, id, name string) error {
	if err := validateProjectID(id); err != nil {
		return err
	}
	if err := validateProjectName(name); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("projects", id),
		Body:   projectRequest{Name: name},
	}, nil)
	return err
}

// DeleteProject permanently removes a project and everything it owns. The
// documented 204 response has no body.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	if err := validateProjectID(id); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("projects", id)}, nil)
	return err
}

// ListProjectUsers returns one cursor-paginated page of project members with
// their project roles. The endpoint requires the user:list scope in addition
// to project access.
func (c *Client) ListProjectUsers(ctx context.Context, projectID string, opts ListOptions) (Page[ProjectMember], error) {
	if err := validateProjectID(projectID); err != nil {
		return Page[ProjectMember]{}, err
	}
	if err := validateProjectListOptions(opts); err != nil {
		return Page[ProjectMember]{}, err
	}
	var page Page[ProjectMember]
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("projects", projectID, "users"),
		Query: opts.Apply(nil),
	}, &page); err != nil {
		return Page[ProjectMember]{}, err
	}
	return page, nil
}

// AddUsersToProject adds one or more users to a project with the given project
// roles. The documented 201 response has no body.
func (c *Client) AddUsersToProject(ctx context.Context, projectID string, relations []ProjectRelation) error {
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	if len(relations) == 0 {
		return fmt.Errorf("at least one project member relation is required")
	}
	for i, relation := range relations {
		if err := relation.Validate(); err != nil {
			return fmt.Errorf("project member relation %d: %w", i+1, err)
		}
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("projects", projectID, "users"),
		Body: struct {
			Relations []ProjectRelation `json:"relations"`
		}{Relations: relations},
	}, nil)
	return err
}

// ChangeUserRoleInProject changes one member's role inside one project. The
// documented 204 response has no body.
func (c *Client) ChangeUserRoleInProject(ctx context.Context, projectID, userID, role string) error {
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	if err := validateProjectText("user ID", userID, true); err != nil {
		return err
	}
	if err := validateProjectText("role", role, true); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("projects", projectID, "users", userID),
		Body: struct {
			Role string `json:"role"`
		}{Role: role},
	}, nil)
	return err
}

// DeleteUserFromProject removes one member from one project. The user account
// itself is untouched. The documented 204 response has no body.
func (c *Client) DeleteUserFromProject(ctx context.Context, projectID, userID string) error {
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	if err := validateProjectText("user ID", userID, true); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("projects", projectID, "users", userID),
	}, nil)
	return err
}

func validateProjectListOptions(opts ListOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.Limit > maxProjectPageSize {
		return fmt.Errorf("limit must not exceed %d, got %d", maxProjectPageSize, opts.Limit)
	}
	return nil
}

func validateProjectID(id string) error { return validateProjectText("ID", id, true) }

func validateProjectName(name string) error { return validateProjectText("name", name, true) }

func validateProjectText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("project %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("project %s must not start or end with whitespace", field)
	}
	return nil
}
