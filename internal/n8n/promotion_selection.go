package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
// n8n 2.48 and earlier only: 2.49 dropped search, sort, and order and lists
// every changed resource with its context instead.
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

// PromotionChange is one selectable workflow difference, as instances before
// n8n 2.49 list them.
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

// Resource change values of an n8n 2.49 preview. unchanged resources give
// context, such as a parent folder or a workflow that uses a changed
// credential.
const (
	PromotionResourceAdded     = "added"
	PromotionResourceModified  = "modified"
	PromotionResourceArchived  = "archived"
	PromotionResourceDeleted   = "deleted"
	PromotionResourceUnchanged = "unchanged"
)

// PromotionResourceRevision fingerprints a resource before and after the
// change; a side the resource is absent from is null.
type PromotionResourceRevision struct {
	Before *string `json:"before"`
	After  *string `json:"after"`
}

// PromotionProjectRef names the project a workflow moved to or from. A
// personal project may come without a name.
type PromotionProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// PromotionResource is one resource of an n8n 2.49 change preview: a project,
// folder, workflow, credential, data table, variable, or tag. Key addresses
// it within the preview (parentKey, dependencies, and change details use it).
// Selectable resources (workflows, and added or modified projects and
// folders) are chosen by ID with workflowIds and folderIds; the rest travel
// with the workflows that use them.
type PromotionResource struct {
	Key               string                    `json:"key"`
	Type              string                    `json:"type"`
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	PreviousName      string                    `json:"previousName,omitempty"`
	PreviousID        string                    `json:"previousId,omitempty"`
	ParentKey         *string                   `json:"parentKey"`
	PreviousParentKey *string                   `json:"previousParentKey,omitempty"`
	Change            string                    `json:"change"`
	Selectable        bool                      `json:"selectable"`
	MovedToProject    *PromotionProjectRef      `json:"movedToProject,omitempty"`
	MovedFromProject  *PromotionProjectRef      `json:"movedFromProject,omitempty"`
	Revision          PromotionResourceRevision `json:"revision"`
}

// PromotionDependency links a workflow to a resource it uses. Included
// reports whether selecting the workflow also sends the resource; it is false
// for sub-workflows, which need their own selection.
type PromotionDependency struct {
	WorkflowKey string `json:"workflowKey"`
	ResourceKey string `json:"resourceKey"`
	Included    bool   `json:"included"`
}

// PromotionBlockerReviewOpen is the blocker code of a Promote whose changes an
// open review already holds.
const PromotionBlockerReviewOpen = "reviewOpen"

// PromotionBlocker is set when an open review holds the changes of a Promote.
// The preview then lists no resources.
type PromotionBlocker struct {
	Code     string `json:"code"`
	ReviewID string `json:"reviewId"`
}

// PromotionMovedResource is a workflow or folder the branch holds in Project
// and the instance in InstanceProject. ProjectExists reports whether the
// branch project exists on the instance.
type PromotionMovedResource struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Project         PromotionProjectRef `json:"project"`
	ProjectExists   bool                `json:"projectExists"`
	InstanceProject PromotionProjectRef `json:"instanceProject"`
}

// PromotionPreviewCheck is what would block an Apply of the previewed
// changes (n8n 2.49 and later): the blocked-apply check plus the workflows
// and folders that moved between projects.
type PromotionPreviewCheck struct {
	PromotionApplyPreflight
	MovedWorkflows []PromotionMovedResource `json:"movedWorkflows"`
	MovedFolders   []PromotionMovedResource `json:"movedFolders"`
}

// Empty reports whether the check lists nothing at all.
func (c PromotionPreviewCheck) Empty() bool {
	return c.PromotionApplyPreflight.Empty() && len(c.MovedWorkflows) == 0 && len(c.MovedFolders) == 0
}

// PromotionChangeSource identifies the environment (n8n 2.46 and later) or
// configuration (2.45 and earlier) and the branch used for a preview. Exactly
// one identifier is set, matching the instance generation.
type PromotionChangeSource struct {
	ConfigID      string `json:"configId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	BranchName    string `json:"branchName"`
}

// PromotionChanges preserves the reviewed source and the complete server
// response. The shape changed with n8n 2.49: older instances list changed
// workflows in Changes; 2.49 lists every changed resource in Resources, with
// Dependencies, an optional Blocker (promote), and for apply a Check and the
// preview Revision to send as expectedPreviewRevision.
type PromotionChanges struct {
	CommitSHA    *string                `json:"commitSha"`
	Source       PromotionChangeSource  `json:"source"`
	Changes      []PromotionChange      `json:"changes,omitempty"`
	Resources    []PromotionResource    `json:"resources,omitempty"`
	Dependencies []PromotionDependency  `json:"dependencies,omitempty"`
	Blocker      *PromotionBlocker      `json:"blocker,omitempty"`
	Check        *PromotionPreviewCheck `json:"check,omitempty"`
	Revision     string                 `json:"revision,omitempty"`
	raw          json.RawMessage
}

// ResourceView reports whether the response has the n8n 2.49 resource shape.
func (r PromotionChanges) ResourceView() bool { return r.Changes == nil }

// Changed returns the resources whose change is not unchanged.
func (r PromotionChanges) Changed() []PromotionResource {
	changed := make([]PromotionResource, 0, len(r.Resources))
	for _, resource := range r.Resources {
		if resource.Change != PromotionResourceUnchanged {
			changed = append(changed, resource)
		}
	}
	return changed
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

// MarshalJSON preserves the server response and emits empty resources as an
// array.
func (r PromotionChanges) MarshalJSON() ([]byte, error) {
	if len(r.raw) > 0 {
		return r.raw, nil
	}
	type plain PromotionChanges
	if r.Changes == nil && r.Resources == nil {
		r.Resources = []PromotionResource{}
	}
	return json.Marshal(plain(r))
}

// GetPromotionChanges compares a project with its cloned apply or promote
// branch. opts filters only on instances before n8n 2.49.
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

// Validate requires a complete reviewed source. Connection routes exist only
// before n8n 2.46, so the source must name configId.
func (r ContinueApplyPromotionRequest) Validate() error { return r.ExpectedSource.ValidateConfig() }

// ApplyProjectSelectionRequest applies selected workflow and folder changes.
// The project's own details always come from the branch, so from n8n 2.49 a
// request without workflowIds and folderIds applies only the project.
// Instances before 2.49 require at least one workflow ID and reject
// folderIds, so both are sent only when set.
//
// ExpectedPreviewRevision (n8n 2.47 and later) pins the reviewed Apply
// preview: when the rebuilt preview differs, the server answers status
// preview-changed and writes nothing. Older instances reject the field, so it
// is sent only when set.
type ApplyProjectSelectionRequest struct {
	WorkflowIDs             []string                 `json:"workflowIds,omitempty"`
	FolderIDs               []string                 `json:"folderIds,omitempty"`
	ExpectedSource          *PromotionExpectedSource `json:"expectedSource,omitempty"`
	ExpectedPreviewRevision string                   `json:"expectedPreviewRevision,omitempty"`
}

// Validate rejects invalid selections, incomplete expected sources, and
// malformed preview revisions.
func (r ApplyProjectSelectionRequest) Validate() error {
	if err := validatePromotionSelection(r.WorkflowIDs, r.FolderIDs); err != nil {
		return err
	}
	if r.ExpectedSource != nil {
		if err := r.ExpectedSource.Validate(); err != nil {
			return err
		}
	}
	return ValidatePromotionPreviewRevision(r.ExpectedPreviewRevision)
}

// ContinueApplyProjectSelectionRequest must repeat the full reviewed
// selection. Selection and ExpectedPreviewRevision work as in
// [ApplyProjectSelectionRequest].
type ContinueApplyProjectSelectionRequest struct {
	WorkflowIDs               []string                `json:"workflowIds,omitempty"`
	FolderIDs                 []string                `json:"folderIds,omitempty"`
	ExpectedSource            PromotionExpectedSource `json:"expectedSource"`
	ExpectedPreviewRevision   string                  `json:"expectedPreviewRevision,omitempty"`
	ConfirmDestructiveChanges *bool                   `json:"confirmDestructiveChanges,omitempty"`
}

// Validate requires a complete reviewed source and a valid selection.
func (r ContinueApplyProjectSelectionRequest) Validate() error {
	if err := validatePromotionSelection(r.WorkflowIDs, r.FolderIDs); err != nil {
		return err
	}
	if err := r.ExpectedSource.Validate(); err != nil {
		return err
	}
	return ValidatePromotionPreviewRevision(r.ExpectedPreviewRevision)
}

var promotionPreviewRevision = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidatePromotionPreviewRevision accepts an empty revision (not sent) or the
// 64 lowercase hex digits n8n 2.47 and later return as an Apply preview
// revision.
func ValidatePromotionPreviewRevision(revision string) error {
	if revision == "" || promotionPreviewRevision.MatchString(revision) {
		return nil
	}
	return fmt.Errorf("expected preview revision %q must be 64 lowercase hex digits: copy the revision of the reviewed Apply preview", revision)
}

// PromoteProjectSelectionRequest pushes selected workflows and folders without
// force support. The project's own details are always written, so from n8n
// 2.49 an empty selection promotes only the project; older instances require
// at least one workflow ID and reject folderIds.
type PromoteProjectSelectionRequest struct {
	WorkflowIDs   []string `json:"workflowIds,omitempty"`
	FolderIDs     []string `json:"folderIds,omitempty"`
	CommitMessage *string  `json:"commitMessage,omitempty"`
}

// Validate rejects invalid selections and empty or overlong commit messages.
func (r PromoteProjectSelectionRequest) Validate() error {
	if err := validatePromotionSelection(r.WorkflowIDs, r.FolderIDs); err != nil {
		return err
	}
	if r.CommitMessage != nil && (strings.TrimSpace(*r.CommitMessage) == "" || utf8.RuneCountInString(*r.CommitMessage) > maxPromotionCommitLength) {
		return fmt.Errorf("commit message must contain 1 to 1000 characters")
	}
	return nil
}

// EmptyPromotionSelection reports a selection without workflows or folders:
// n8n 2.49 and later then apply or promote only the project's own details.
func EmptyPromotionSelection(workflowIDs, folderIDs []string) bool {
	return len(workflowIDs) == 0 && len(folderIDs) == 0
}

func validatePromotionSelection(workflowIDs, folderIDs []string) error {
	if err := validatePromotionIDList(workflowIDs, "workflow"); err != nil {
		return err
	}
	return validatePromotionIDList(folderIDs, "folder")
}

func validatePromotionIDList(ids []string, kind string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if err := validatePromotionID(id, kind+" ID"); err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("duplicate %s ID %q: select each %s once", kind, id, kind)
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
