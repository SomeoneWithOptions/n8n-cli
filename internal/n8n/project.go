package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// ProjectsPath is the collection endpoint for projects.
const ProjectsPath = "/projects"

// maxProjectPageSize is the page size the API documents as its maximum.
const maxProjectPageSize = 250

// Project is one n8n project as the API returns it. Icon, description and
// creatorId are nullable on read, so they keep the difference between absent
// and null. Fields this type does not name are kept in Extra and written back
// out, so `--output json` does not lose what a newer instance sends.
type Project struct {
	ID                  string                `json:"id,omitempty"`
	Name                string                `json:"name"`
	Type                string                `json:"type,omitempty"`
	Icon                Nullable[ProjectIcon] `json:"icon,omitzero"`
	Description         Nullable[string]      `json:"description,omitzero"`
	CustomTelemetryTags []ProjectTelemetryTag `json:"customTelemetryTags,omitzero"`
	CreatorID           Nullable[string]      `json:"creatorId,omitzero"`
	CreatedAt           string                `json:"createdAt,omitempty"`
	UpdatedAt           string                `json:"updatedAt,omitempty"`
	// Role and Scopes are the caller's, returned by create only.
	Role   string   `json:"role,omitempty"`
	Scopes []string `json:"scopes,omitzero"`

	Extra map[string]json.RawMessage `json:"-"`
}

// projectFields are the field names [Project] decodes itself.
var projectFields = []string{
	"id", "name", "type", "icon", "description", "customTelemetryTags",
	"creatorId", "createdAt", "updatedAt", "role", "scopes",
}

// UnmarshalJSON decodes the named fields and keeps the rest in Extra.
func (p *Project) UnmarshalJSON(data []byte) error {
	type plain Project
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	rest, err := unknownFields(data, projectFields)
	if err != nil {
		return err
	}
	*p = Project(decoded)
	p.Extra = rest
	return nil
}

// MarshalJSON writes the named fields followed by Extra in sorted order.
func (p Project) MarshalJSON() ([]byte, error) {
	type plain Project
	base, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	return appendExtra(base, p.Extra)
}

// Project icon types.
const (
	ProjectIconEmoji = "emoji"
	ProjectIconIcon  = "icon"
)

// ProjectIcon is a project's icon: an emoji or a named icon, with an optional
// color.
type ProjectIcon struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Color string `json:"color,omitempty"`
}

// ProjectTelemetryTag is one key/value pair attached to a project's
// telemetry.
type ProjectTelemetryTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Project field limits from the contract.
const (
	MaxProjectName        = 255
	MaxProjectID          = 36
	MaxProjectDescription = 512
)

// CreateProjectRequest is the write document for POST /projects. Only Name
// is required; the instance generates the ID when it is empty. It is separate
// from [Project] so read-only fields are never sent.
type CreateProjectRequest struct {
	Name                string                `json:"name"`
	ID                  string                `json:"id,omitempty"`
	Icon                *ProjectIcon          `json:"icon,omitempty"`
	Description         string                `json:"description,omitempty"`
	CustomTelemetryTags []ProjectTelemetryTag `json:"customTelemetryTags,omitempty"`
}

// Validate rejects a creation the API would reject.
func (r CreateProjectRequest) Validate() error {
	if err := validateProjectName(r.Name); err != nil {
		return err
	}
	if r.ID != "" {
		if err := validateProjectText("ID", r.ID, true); err != nil {
			return err
		}
		if n := utf8.RuneCountInString(r.ID); n > MaxProjectID {
			return fmt.Errorf("project ID is %d characters; the API accepts at most %d", n, MaxProjectID)
		}
	}
	if r.Icon != nil {
		if r.Icon.Type != ProjectIconEmoji && r.Icon.Type != ProjectIconIcon {
			return fmt.Errorf("project icon type %q is invalid: use %s or %s", r.Icon.Type, ProjectIconEmoji, ProjectIconIcon)
		}
		if r.Icon.Value == "" {
			return fmt.Errorf("project icon value is required")
		}
	}
	if n := utf8.RuneCountInString(r.Description); n > MaxProjectDescription {
		return fmt.Errorf("project description is %d characters; the API accepts at most %d", n, MaxProjectDescription)
	}
	for i, tag := range r.CustomTelemetryTags {
		if strings.TrimSpace(tag.Key) == "" {
			return fmt.Errorf("telemetry tag %d has an empty key", i+1)
		}
	}
	return nil
}

// projectRequest is the write document for update, whose only writable field
// is the name.
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

// CreateProject creates a team project and returns it with the caller's role
// and scopes in it. Older instances answered 201 with no body, so the
// returned project may be empty; list the projects to recover the ID then.
func (c *Client) CreateProject(ctx context.Context, request CreateProjectRequest) (*Project, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var created Project
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   ProjectsPath,
		Body:   request,
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

func validateProjectName(name string) error {
	if err := validateProjectText("name", name, true); err != nil {
		return err
	}
	if n := utf8.RuneCountInString(name); n > MaxProjectName {
		return fmt.Errorf("project name is %d characters; the API accepts at most %d", n, MaxProjectName)
	}
	return nil
}

func validateProjectText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("project %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("project %s must not start or end with whitespace", field)
	}
	return nil
}
