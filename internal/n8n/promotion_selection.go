package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// PromotionRepository is a GitLab repository usable as a connection target.
type PromotionRepository struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	FullPath      string  `json:"fullPath"`
	RemoteURL     string  `json:"remoteUrl"`
	DefaultBranch *string `json:"defaultBranch"`
}

// PromotionRepositoryBranch identifies a branch and the repository default.
type PromotionRepositoryBranch struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

// PromotionDiscoveryOptions filters cursor-paginated repository discovery.
type PromotionDiscoveryOptions struct {
	ListOptions
	Search string
}

// Validate rejects unsupported page sizes and overlong searches before transport.
func (o PromotionDiscoveryOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.Limit)
	}
	if utf8.RuneCountInString(o.Search) > 255 {
		return fmt.Errorf("search must contain at most 255 characters")
	}
	return nil
}

// Apply encodes the page and search filters.
func (o PromotionDiscoveryOptions) Apply(q url.Values) url.Values {
	q = o.ListOptions.Apply(q)
	if o.Search != "" {
		q.Set("search", o.Search)
	}
	return q
}

// ListPromotionProviderRepositories lists one page. Follow nextCursor even on short pages.
func (c *Client) ListPromotionProviderRepositories(ctx context.Context, id string, opts PromotionDiscoveryOptions) (Page[PromotionRepository], error) {
	page := Page[PromotionRepository]{Data: []PromotionRepository{}}
	if err := validatePromotionID(id, "promotion provider ID"); err != nil {
		return page, err
	}
	if err := opts.Validate(); err != nil {
		return page, err
	}
	_, err := c.Do(ctx, Request{Path: PathJoin("promotions", "providers", id, "repositories"), Query: opts.Apply(nil)}, &page)
	if page.Data == nil {
		page.Data = []PromotionRepository{}
	}
	return page, err
}

// ListPromotionProviderRepositoryBranches accepts a raw fullPath or GitLab project ID.
// PathJoin escapes repository slashes exactly once. Short pages can have a next cursor.
func (c *Client) ListPromotionProviderRepositoryBranches(ctx context.Context, id, repository string, opts PromotionDiscoveryOptions) (Page[PromotionRepositoryBranch], error) {
	page := Page[PromotionRepositoryBranch]{Data: []PromotionRepositoryBranch{}}
	if err := validatePromotionID(id, "promotion provider ID"); err != nil {
		return page, err
	}
	if strings.TrimSpace(repository) == "" || utf8.RuneCountInString(repository) > 256 {
		return page, fmt.Errorf("repository path must contain 1 to 256 characters: pass the raw fullPath or GitLab project ID")
	}
	if err := opts.Validate(); err != nil {
		return page, err
	}
	_, err := c.Do(ctx, Request{Path: PathJoin("promotions", "providers", id, "repositories", repository, "branches"), Query: opts.Apply(nil)}, &page)
	if page.Data == nil {
		page.Data = []PromotionRepositoryBranch{}
	}
	return page, err
}

// PromotionChangesOptions filters and sorts the project change preview.
type PromotionChangesOptions struct{ Search, Sort, Order string }

// Validate checks the documented sort and order enums.
func (o PromotionChangesOptions) Validate() error {
	switch o.Sort {
	case "", "name", "updatedAt", "status":
	default:
		return fmt.Errorf("sort must be name, updatedAt, or status")
	}
	switch o.Order {
	case "", "asc", "desc":
	default:
		return fmt.Errorf("order must be asc or desc")
	}
	return nil
}

// PromotionChange is one selectable workflow difference.
type PromotionChange struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	Version         *int    `json:"version"`
	UpdatedAt       *string `json:"updatedAt"`
	UpdatedBy       *string `json:"updatedBy"`
	DependencyCount int     `json:"dependencyCount"`
}

// PromotionChangeSource identifies the configuration and branch used for a preview.
type PromotionChangeSource struct {
	ConfigID   string `json:"configId"`
	BranchName string `json:"branchName"`
}

// PromotionChanges preserves the reviewed source and the complete server response.
type PromotionChanges struct {
	CommitSHA *string               `json:"commitSha"`
	Source    PromotionChangeSource `json:"source"`
	Changes   []PromotionChange     `json:"changes"`
	raw       json.RawMessage
}

// UnmarshalJSON decodes the typed view and retains all response fields.
func (r *PromotionChanges) UnmarshalJSON(data []byte) error {
	type plain PromotionChanges
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = PromotionChanges(p)
	r.raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON preserves the server response and emits empty changes as an array.
func (r PromotionChanges) MarshalJSON() ([]byte, error) {
	if len(r.raw) > 0 {
		return r.raw, nil
	}
	type plain PromotionChanges
	if r.Changes == nil {
		r.Changes = []PromotionChange{}
	}
	return json.Marshal(plain(r))
}

// GetPromotionChanges compares a project with its cloned apply or promote branch.
func (c *Client) GetPromotionChanges(ctx context.Context, projectID, direction string, opts PromotionChangesOptions) (*PromotionChanges, error) {
	if err := validatePromotionID(projectID, "project ID"); err != nil {
		return nil, err
	}
	if err := validatePromotionDirection(direction); err != nil {
		return nil, err
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	q := url.Values{}
	if opts.Search != "" {
		q.Set("search", opts.Search)
	}
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	}
	if opts.Order != "" {
		q.Set("order", opts.Order)
	}
	var result PromotionChanges
	if _, err := c.Do(ctx, Request{Path: PathJoin("promotions", "projects", projectID, "changes", direction), Query: q}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ContinueApplyPromotionRequest rechecks a reviewed full apply without a server session.
type ContinueApplyPromotionRequest struct {
	ExpectedSource            PromotionExpectedSource `json:"expectedSource"`
	ConfirmDestructiveChanges *bool                   `json:"confirmDestructiveChanges,omitempty"`
}

// Validate requires a complete reviewed source.
func (r ContinueApplyPromotionRequest) Validate() error { return r.ExpectedSource.Validate() }

// ApplyProjectSelectionRequest applies only selected workflow changes.
type ApplyProjectSelectionRequest struct {
	WorkflowIDs    []string                 `json:"workflowIds"`
	ExpectedSource *PromotionExpectedSource `json:"expectedSource,omitempty"`
}

// Validate rejects invalid selections and incomplete expected sources.
func (r ApplyProjectSelectionRequest) Validate() error {
	if err := validatePromotionWorkflowIDs(r.WorkflowIDs); err != nil {
		return err
	}
	if r.ExpectedSource != nil {
		return r.ExpectedSource.Validate()
	}
	return nil
}

// ContinueApplyProjectSelectionRequest must repeat the full reviewed selection.
type ContinueApplyProjectSelectionRequest struct {
	WorkflowIDs               []string                `json:"workflowIds"`
	ExpectedSource            PromotionExpectedSource `json:"expectedSource"`
	ConfirmDestructiveChanges *bool                   `json:"confirmDestructiveChanges,omitempty"`
}

// Validate requires the full selection and a complete reviewed source.
func (r ContinueApplyProjectSelectionRequest) Validate() error {
	if err := validatePromotionWorkflowIDs(r.WorkflowIDs); err != nil {
		return err
	}
	return r.ExpectedSource.Validate()
}

// PromoteProjectSelectionRequest pushes selected workflows without force support.
type PromoteProjectSelectionRequest struct {
	WorkflowIDs   []string `json:"workflowIds"`
	CommitMessage *string  `json:"commitMessage,omitempty"`
}

// Validate rejects invalid selections and empty or overlong commit messages.
func (r PromoteProjectSelectionRequest) Validate() error {
	if err := validatePromotionWorkflowIDs(r.WorkflowIDs); err != nil {
		return err
	}
	if r.CommitMessage != nil && (strings.TrimSpace(*r.CommitMessage) == "" || utf8.RuneCountInString(*r.CommitMessage) > maxPromotionCommitLength) {
		return fmt.Errorf("commit message must contain 1 to 1000 characters")
	}
	return nil
}

func validatePromotionWorkflowIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("workflowIds requires at least one workflow ID: pass --workflow-id or --input")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if err := validatePromotionID(id, "workflow ID"); err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("duplicate workflow ID %q: select each workflow once", id)
		}
		seen[id] = true
	}
	return nil
}

// ContinueApplyPackage rechecks bindings and applies the reviewed full package.
// opts accepts [IdempotencyKey].
func (c *Client) ContinueApplyPackage(ctx context.Context, id string, request ContinueApplyPromotionRequest, opts ...CallOption) (*ApplyPromotionResult, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return c.applyPromotionSelection(ctx, PathJoin("promotions", "connections", id, "apply", "continue"), request, opts)
}

// ApplyProjectSelection imports or removes selected workflow IDs from the branch.
// opts accepts [IdempotencyKey].
func (c *Client) ApplyProjectSelection(ctx context.Context, id string, request ApplyProjectSelectionRequest, opts ...CallOption) (*ApplyPromotionResult, error) {
	if err := validatePromotionID(id, "project ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return c.applyPromotionSelection(ctx, PathJoin("promotions", "projects", id, "apply"), request, opts)
}

// ContinueApplyProjectSelection rechecks the reviewed source and full selection.
// opts accepts [IdempotencyKey].
func (c *Client) ContinueApplyProjectSelection(ctx context.Context, id string, request ContinueApplyProjectSelectionRequest, opts ...CallOption) (*ApplyPromotionResult, error) {
	if err := validatePromotionID(id, "project ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return c.applyPromotionSelection(ctx, PathJoin("promotions", "projects", id, "apply", "continue"), request, opts)
}

func (c *Client) applyPromotionSelection(ctx context.Context, path string, request any, opts []CallOption) (*ApplyPromotionResult, error) {
	var result ApplyPromotionResult
	req := Request{Method: http.MethodPost, Path: path, Body: request}
	applyCallOptions(&req, opts)
	if _, err := c.Do(ctx, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PromoteProjectSelection exports selected workflow states and pushes the branch.
// opts accepts [IdempotencyKey].
func (c *Client) PromoteProjectSelection(ctx context.Context, id string, request PromoteProjectSelectionRequest, opts ...CallOption) (*PromotePromotionResult, error) {
	if err := validatePromotionID(id, "project ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result PromotePromotionResult
	req := Request{Method: http.MethodPost, Path: PathJoin("promotions", "projects", id, "promote"), Body: request}
	applyCallOptions(&req, opts)
	if _, err := c.Do(ctx, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CollectPromotionDiscovery follows provider cursors even across empty filtered
// pages. Both items and requests are bounded, and any repeated cursor fails.
func CollectPromotionDiscovery[T any](ctx context.Context, fetch PageFunc[T], opts ListOptions) (Collection[T], error) {
	seen := make(map[string]bool)
	requests := 0
	return CollectWithInfo(ctx, func(ctx context.Context, pageOpts ListOptions) (Page[T], error) {
		for {
			if err := ctx.Err(); err != nil {
				return Page[T]{}, err
			}
			if seen[pageOpts.Cursor] {
				return Page[T]{}, fmt.Errorf("pagination cursor %q repeated: the API is looping", pageOpts.Cursor)
			}
			if requests >= DefaultCollectLimit {
				return Page[T]{}, fmt.Errorf("promotion discovery exceeded %d pages: narrow --search or page explicitly", DefaultCollectLimit)
			}
			seen[pageOpts.Cursor] = true
			requests++
			page, err := fetch(ctx, pageOpts)
			if err != nil {
				return page, err
			}
			if page.HasMore() && seen[page.NextCursor] {
				return Page[T]{}, fmt.Errorf("pagination cursor %q repeated: the API is looping", page.NextCursor)
			}
			if len(page.Data) > 0 || !page.HasMore() {
				return page, nil
			}
			pageOpts.Cursor = page.NextCursor
		}
	}, opts, DefaultCollectLimit)
}
