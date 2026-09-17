package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// DefaultFolderPageSize is the page size used when following every page. The
// API defaults to 10 per page, which is too small to walk a whole project.
const DefaultFolderPageSize = 100

// FolderSortFields are the sort orders GET /projects/{id}/folders accepts.
var FolderSortFields = []string{
	"name:asc", "name:desc",
	"createdAt:asc", "createdAt:desc",
	"updatedAt:asc", "updatedAt:desc",
}

// FolderSelectFields are the field names the list endpoint can be narrowed to.
// The instance may answer 500 for some of them; see the CLI help.
var FolderSelectFields = []string{
	"id", "name", "createdAt", "updatedAt",
	"project", "tags", "parentFolder", "workflowCount", "subFolderCount", "path",
}

// Folder is one folder inside a project.
//
// The documented schema is id, name, parentFolderId, createdAt and updatedAt.
// List responses carry more than that — the home project, the parent folder,
// tags and the two counts — and the select parameter can add a path, so those
// fields are typed here as well and stay absent when the instance omits them.
type Folder struct {
	ID             string   `json:"id,omitempty"`
	Name           string   `json:"name,omitempty"`
	ParentFolderID string   `json:"parentFolderId,omitempty"`
	CreatedAt      string   `json:"createdAt,omitempty"`
	UpdatedAt      string   `json:"updatedAt,omitempty"`
	HomeProject    *Project `json:"homeProject,omitempty"`
	ParentFolder   *Folder  `json:"parentFolder,omitempty"`
	Tags           []Tag    `json:"tags,omitempty"`
	WorkflowCount  *int     `json:"workflowCount,omitempty"`
	SubFolderCount *int     `json:"subFolderCount,omitempty"`
	// Path is the folder names from the project root down to this folder.
	Path []string `json:"path,omitempty"`
}

// FolderDetail is the single-folder response. It reports recursive totals the
// list response does not, and the list response reports relations it does not,
// so the two are separate types rather than one lenient one.
type FolderDetail struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name,omitempty"`
	ParentFolderID  string `json:"parentFolderId,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
	TotalSubFolders int    `json:"totalSubFolders"`
	TotalWorkflows  int    `json:"totalWorkflows"`
}

// FolderPage is one offset-paginated folder response. Count is the total number
// of folders matching the query, not the number in Data.
type FolderPage struct {
	Count int      `json:"count"`
	Data  []Folder `json:"data"`
}

// FolderFilter is the documented filter object of the list endpoint. It is
// JSON-encoded into one query parameter.
type FolderFilter struct {
	ParentFolderID string   `json:"parentFolderId,omitempty"`
	Name           string   `json:"name,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	// ExcludeFolderIDAndDescendants drops one folder and its whole subtree.
	ExcludeFolderIDAndDescendants string `json:"excludeFolderIdAndDescendants,omitempty"`
}

// IsZero reports whether the filter narrows nothing, in which case the query
// parameter is left out entirely.
func (f FolderFilter) IsZero() bool {
	return f.ParentFolderID == "" && f.Name == "" && len(f.Tags) == 0 && f.ExcludeFolderIDAndDescendants == ""
}

// Validate rejects filter values the API would reject.
func (f FolderFilter) Validate() error {
	if err := validateFolderText("filter parent folder ID", f.ParentFolderID, false); err != nil {
		return err
	}
	if err := validateFolderText("filter name", f.Name, false); err != nil {
		return err
	}
	if err := validateFolderText("filter excluded folder ID", f.ExcludeFolderIDAndDescendants, false); err != nil {
		return err
	}
	for i, tag := range f.Tags {
		if err := validateFolderText("filter tag name", tag, true); err != nil {
			return fmt.Errorf("filter tag %d: %w", i+1, err)
		}
	}
	return nil
}

// ListFoldersOptions are the query parameters of the folder list endpoint. It
// pages by offset (skip and take), not by cursor, so it does not use
// [ListOptions].
type ListFoldersOptions struct {
	// Skip is the number of folders to skip. Zero starts at the first folder.
	Skip int
	// Take is the page size. Zero uses the server default of 10.
	Take int
	// Filter narrows the result set. The zero value sends no filter.
	Filter FolderFilter
	// Select limits the returned fields to [FolderSelectFields]. Empty returns
	// every field the instance sends.
	Select []string
	// SortBy is one of [FolderSortFields]. Empty uses the server's order.
	SortBy string
}

// Validate rejects options the API would reject or that indicate a bug.
func (o ListFoldersOptions) Validate() error {
	if o.Skip < 0 {
		return fmt.Errorf("skip must not be negative, got %d", o.Skip)
	}
	if o.Take < 0 {
		return fmt.Errorf("take must not be negative, got %d", o.Take)
	}
	if o.SortBy != "" && !slices.Contains(FolderSortFields, o.SortBy) {
		return fmt.Errorf("unknown sort order %q: the API defines %s", o.SortBy, strings.Join(FolderSortFields, ", "))
	}
	for _, field := range o.Select {
		if !slices.Contains(FolderSelectFields, field) {
			return fmt.Errorf("unknown select field %q: the API defines %s", field, strings.Join(FolderSelectFields, ", "))
		}
	}
	return o.Filter.Validate()
}

// apply encodes the options as query parameters. The filter and select values
// are JSON documents inside one parameter each, which is what the API expects.
func (o ListFoldersOptions) apply() (url.Values, error) {
	query := url.Values{}
	if o.Skip > 0 {
		query.Set("skip", strconv.Itoa(o.Skip))
	}
	if o.Take > 0 {
		query.Set("take", strconv.Itoa(o.Take))
	}
	if o.SortBy != "" {
		query.Set("sortBy", o.SortBy)
	}
	if !o.Filter.IsZero() {
		encoded, err := json.Marshal(o.Filter)
		if err != nil {
			return nil, fmt.Errorf("encode folder filter: %w", err)
		}
		query.Set("filter", string(encoded))
	}
	if len(o.Select) > 0 {
		encoded, err := json.Marshal(o.Select)
		if err != nil {
			return nil, fmt.Errorf("encode folder select: %w", err)
		}
		query.Set("select", string(encoded))
	}
	return query, nil
}

// CreateFolderRequest is the write document for folder creation. ParentFolderID
// is optional: without it the folder is created at the project root.
type CreateFolderRequest struct {
	Name           string `json:"name"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
}

// Validate rejects a malformed creation before transport.
func (r CreateFolderRequest) Validate() error {
	if err := validateFolderText("name", r.Name, true); err != nil {
		return err
	}
	return validateFolderText("parent folder ID", r.ParentFolderID, false)
}

// UpdateFolderRequest is the partial write document for a folder update. At
// least one field must be set, because the API requires a non-empty document.
// There is no way to clear a parent: moving a folder back to the project root
// is not part of this contract.
type UpdateFolderRequest struct {
	Name           string `json:"name,omitempty"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
}

// Validate rejects an empty or malformed update before transport.
func (r UpdateFolderRequest) Validate() error {
	if r.Name == "" && r.ParentFolderID == "" {
		return fmt.Errorf("a folder update needs a new name or a new parent folder ID")
	}
	if err := validateFolderText("name", r.Name, false); err != nil {
		return err
	}
	return validateFolderText("parent folder ID", r.ParentFolderID, false)
}

// ListFolders returns one offset-paginated page of folders in one project.
func (c *Client) ListFolders(ctx context.Context, projectID string, opts ListFoldersOptions) (FolderPage, error) {
	if err := validateFolderProjectID(projectID); err != nil {
		return FolderPage{}, err
	}
	if err := opts.Validate(); err != nil {
		return FolderPage{}, err
	}
	query, err := opts.apply()
	if err != nil {
		return FolderPage{}, err
	}
	var page FolderPage
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("projects", projectID, "folders"),
		Query: query,
	}, &page); err != nil {
		return FolderPage{}, err
	}
	return page, nil
}

// CreateFolder creates one folder in one project. projectID may be the literal
// "personal" to create the folder in the caller's own personal project.
func (c *Client) CreateFolder(ctx context.Context, projectID string, req CreateFolderRequest) (*Folder, error) {
	if err := validateFolderProjectID(projectID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var created Folder
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("projects", projectID, "folders"),
		Body:   req,
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetFolder returns one folder with its recursive sub-folder and workflow
// totals.
func (c *Client) GetFolder(ctx context.Context, projectID, folderID string) (*FolderDetail, error) {
	if err := validateFolderProjectID(projectID); err != nil {
		return nil, err
	}
	if err := validateFolderID(folderID); err != nil {
		return nil, err
	}
	var folder FolderDetail
	if _, err := c.Do(ctx, Request{Path: PathJoin("projects", projectID, "folders", folderID)}, &folder); err != nil {
		return nil, err
	}
	return &folder, nil
}

// UpdateFolder renames one folder, moves it under another folder, or both. The
// request is a partial update: fields left empty stay as they are.
func (c *Client) UpdateFolder(ctx context.Context, projectID, folderID string, req UpdateFolderRequest) (*Folder, error) {
	if err := validateFolderProjectID(projectID); err != nil {
		return nil, err
	}
	if err := validateFolderID(folderID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var updated Folder
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("projects", projectID, "folders", folderID),
		Body:   req,
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteFolder deletes one folder. With transferToFolderID the workflows and
// sub-folders are moved into that folder first. Without it, the workflows are
// moved to the project root and archived, and every child folder is deleted.
// The documented 204 response has no body.
func (c *Client) DeleteFolder(ctx context.Context, projectID, folderID, transferToFolderID string) error {
	if err := validateFolderProjectID(projectID); err != nil {
		return err
	}
	if err := validateFolderID(folderID); err != nil {
		return err
	}
	if err := validateFolderText("transfer target folder ID", transferToFolderID, false); err != nil {
		return err
	}
	query := url.Values{}
	if transferToFolderID != "" {
		query.Set("transferToFolderId", transferToFolderID)
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("projects", projectID, "folders", folderID),
		Query:  query,
	}, nil)
	return err
}

// FolderPageFunc fetches one folder page for the given options.
type FolderPageFunc func(context.Context, ListFoldersOptions) (FolderPage, error)

// CollectFolders walks every offset page and returns the accumulated folders.
//
// max caps the number of folders collected; zero uses [DefaultCollectLimit].
// Collection stops on a short page, on the reported total, or on the cap, so a
// server that ignores skip cannot loop forever.
func CollectFolders(ctx context.Context, fetch FolderPageFunc, opts ListFoldersOptions, max int) ([]Folder, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = DefaultCollectLimit
	}
	if opts.Take <= 0 {
		opts.Take = DefaultFolderPageSize
	}

	start := opts.Skip
	var folders []Folder
	for {
		if err := ctx.Err(); err != nil {
			return folders, err
		}

		page, err := fetch(ctx, opts)
		if err != nil {
			return folders, err
		}
		folders = append(folders, page.Data...)

		if len(folders) >= max {
			return folders[:max], nil
		}
		if len(page.Data) < opts.Take {
			return folders, nil
		}
		if page.Count > 0 && start+len(folders) >= page.Count {
			return folders, nil
		}
		opts.Skip = start + len(folders)
	}
}

func validateFolderProjectID(id string) error { return validateFolderText("project ID", id, true) }

func validateFolderID(id string) error { return validateFolderText("ID", id, true) }

func validateFolderText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("folder %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("folder %s must not start or end with whitespace", field)
	}
	return nil
}
