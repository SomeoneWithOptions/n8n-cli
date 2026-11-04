package n8n

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// PromotionEnvironmentsPath is the environment collection. Environments
// replace connections from n8n 2.46: one environment is one remote for one
// direction, with its own settings and checkout, where a connection held one
// remote plus a config per direction. Instances older than 2.46 serve only
// connections; both generations share the gitConnection:* scopes.
const PromotionEnvironmentsPath = "/promotions/environments"

var promotionEnvironmentID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidatePromotionEnvironmentID rejects an environment ID that is not a UUID,
// as the route requires, before transport.
func ValidatePromotionEnvironmentID(id string) error {
	if err := validatePromotionID(id, "promotion environment ID"); err != nil {
		return err
	}
	if !promotionEnvironmentID.MatchString(id) {
		return fmt.Errorf("promotion environment ID %q must be a UUID", id)
	}
	return nil
}

// PromotionRemoteDocument is the remote an environment create or update sends.
// For GitLab the remote URL must belong to the provider; the server stores only
// the repository path and derives remoteUrl from the provider base URL. The
// read-only repositoryPath of [PromotionTarget] is never sent.
type PromotionRemoteDocument struct {
	SchemaVersion int    `json:"schemaVersion"`
	RemoteURL     string `json:"remoteUrl"`
}

// Validate rejects a remote the API would reject before transport.
func (r PromotionRemoteDocument) Validate() error {
	if r.SchemaVersion != promotionTargetSchemaVersion {
		return fmt.Errorf("remote schemaVersion must be %d, got %d", promotionTargetSchemaVersion, r.SchemaVersion)
	}
	if strings.TrimSpace(r.RemoteURL) == "" {
		return fmt.Errorf("remote remoteUrl is required")
	}
	return nil
}

// PromotionEnvironmentSettings is the direction-specific settings union. An
// apply environment sets BranchName; a promote environment sets
// BaseBranchName and CreateBranchOnPromotion. The other direction's fields
// stay empty and are omitted.
type PromotionEnvironmentSettings struct {
	SchemaVersion           int    `json:"schemaVersion"`
	BranchName              string `json:"branchName,omitempty"`
	BaseBranchName          string `json:"baseBranchName,omitempty"`
	CreateBranchOnPromotion *bool  `json:"createBranchOnPromotion,omitempty"`
}

// Direction reports which direction the settings fit: apply, promote, or ""
// when they carry fields of both or of neither.
func (s PromotionEnvironmentSettings) Direction() string {
	apply := s.BranchName != ""
	promote := s.BaseBranchName != "" || s.CreateBranchOnPromotion != nil
	switch {
	case apply && !promote:
		return PromotionDirectionApply
	case promote && !apply:
		return PromotionDirectionPromote
	default:
		return ""
	}
}

// Validate rejects settings the API would reject before transport. direction
// is the environment's direction when known; "" accepts either shape.
func (s PromotionEnvironmentSettings) Validate(direction string) error {
	shape := s.Direction()
	if shape == "" {
		return fmt.Errorf("settings must be either apply {\"schemaVersion\":1,\"branchName\":\"main\"} or promote {\"schemaVersion\":1,\"baseBranchName\":\"main\",\"createBranchOnPromotion\":false}")
	}
	if direction != "" && shape != direction {
		return fmt.Errorf("settings are %s settings but the environment direction is %s: use the %s settings shape", shape, direction, direction)
	}
	if s.SchemaVersion != promotionTargetSchemaVersion {
		return fmt.Errorf("settings schemaVersion must be %d, got %d", promotionTargetSchemaVersion, s.SchemaVersion)
	}
	if shape == PromotionDirectionApply {
		return validatePromotionBranch(s.BranchName)
	}
	if err := validatePromotionBranch(s.BaseBranchName); err != nil {
		return fmt.Errorf("base branch: %w", err)
	}
	if s.CreateBranchOnPromotion == nil {
		return fmt.Errorf("promote settings need createBranchOnPromotion (true or false)")
	}
	return nil
}

// PromotionEnvironmentCheckoutState is the local checkout state an environment
// reports: whether it is connected and whether the checkout still matches
// the environment.
type PromotionEnvironmentCheckoutState struct {
	HasCheckout        bool `json:"hasCheckout"`
	MatchesEnvironment bool `json:"matchesEnvironment"`
}

// PromotionEnvironment is one promotion environment as the public DTO reports
// it. Checkout is nil on list rows, which omit it. Secrets are never returned.
type PromotionEnvironment struct {
	ID        string                             `json:"id"`
	Name      string                             `json:"name"`
	Scope     string                             `json:"scope"`
	Direction string                             `json:"direction"`
	Remote    PromotionTarget                    `json:"remote"`
	Provider  PromotionProviderRef               `json:"provider"`
	Settings  PromotionEnvironmentSettings       `json:"settings"`
	Checkout  *PromotionEnvironmentCheckoutState `json:"checkout,omitempty"`
	CreatedAt string                             `json:"createdAt"`
	UpdatedAt string                             `json:"updatedAt"`
}

// CreatePromotionEnvironmentRequest creates one environment for one direction
// on an existing provider. The settings must match the direction.
type CreatePromotionEnvironmentRequest struct {
	Name       string                       `json:"name"`
	Scope      string                       `json:"scope"`
	ProviderID string                       `json:"providerId"`
	Remote     PromotionRemoteDocument      `json:"remote"`
	Direction  string                       `json:"direction"`
	Settings   PromotionEnvironmentSettings `json:"settings"`
}

// Validate rejects a creation document the API would reject before transport.
func (r CreatePromotionEnvironmentRequest) Validate() error {
	if err := validatePromotionName(r.Name); err != nil {
		return err
	}
	if err := validatePromotionScope(r.Scope); err != nil {
		return err
	}
	if err := validatePromotionID(r.ProviderID, "provider ID"); err != nil {
		return err
	}
	if err := r.Remote.Validate(); err != nil {
		return err
	}
	if err := validatePromotionDirection(r.Direction); err != nil {
		return err
	}
	return r.Settings.Validate(r.Direction)
}

// UpdatePromotionEnvironmentRequest patches an environment. Only the supplied
// fields change, at least one is required, and a remote or settings replaces
// the whole saved value. Scope and direction cannot change.
type UpdatePromotionEnvironmentRequest struct {
	Name       *string                       `json:"name,omitempty"`
	ProviderID *string                       `json:"providerId,omitempty"`
	Remote     *PromotionRemoteDocument      `json:"remote,omitempty"`
	Settings   *PromotionEnvironmentSettings `json:"settings,omitempty"`
}

// Validate rejects an empty or malformed update document before transport.
// Settings are checked against either direction; the server checks them
// against the stored one.
func (r UpdatePromotionEnvironmentRequest) Validate() error {
	if r.Name == nil && r.ProviderID == nil && r.Remote == nil && r.Settings == nil {
		return fmt.Errorf("promotion environment update is empty: set at least one of name, providerId, remote, or settings")
	}
	if r.Name != nil {
		if err := validatePromotionName(*r.Name); err != nil {
			return err
		}
	}
	if r.ProviderID != nil {
		if err := validatePromotionID(*r.ProviderID, "provider ID"); err != nil {
			return err
		}
	}
	if r.Remote != nil {
		if err := r.Remote.Validate(); err != nil {
			return err
		}
	}
	if r.Settings != nil {
		if err := r.Settings.Validate(""); err != nil {
			return err
		}
	}
	return nil
}

// ListPromotionEnvironmentsOptions filters the environment list by the
// documented query filters: scope, direction, and provider ID.
type ListPromotionEnvironmentsOptions struct {
	ListOptions
	Scope      string
	Direction  string
	ProviderID string
}

// Apply writes pagination plus the documented filters into q.
func (o ListPromotionEnvironmentsOptions) Apply(q url.Values) url.Values {
	q = o.ListOptions.Apply(q)
	if o.Scope != "" {
		q.Set("scope", o.Scope)
	}
	if o.Direction != "" {
		q.Set("direction", o.Direction)
	}
	if o.ProviderID != "" {
		q.Set("providerId", o.ProviderID)
	}
	return q
}

// Validate rejects options the API would reject before transport.
func (o ListPromotionEnvironmentsOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.ListOptions.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.ListOptions.Limit)
	}
	if o.Scope != "" {
		if err := validatePromotionScope(o.Scope); err != nil {
			return err
		}
	}
	if o.Direction != "" {
		if err := validatePromotionDirection(o.Direction); err != nil {
			return err
		}
	}
	if o.ProviderID != "" {
		if err := validatePromotionID(o.ProviderID, "provider ID"); err != nil {
			return err
		}
	}
	return nil
}

// PromotionEnvironmentCheckout is the connect and disconnect acknowledgement:
// the environment's direction, branch, and whether a checkout now exists.
type PromotionEnvironmentCheckout struct {
	EnvironmentID string `json:"environmentId"`
	Direction     string `json:"direction"`
	BranchName    string `json:"branchName"`
	HasCheckout   bool   `json:"hasCheckout"`
}

// PromotionEnvironmentProjects is the team projects linked to one environment.
type PromotionEnvironmentProjects struct {
	ProjectIDs []string `json:"projectIds"`
}

// PromotionEnvironmentProjectLink acknowledges linking one project to an
// environment.
type PromotionEnvironmentProjectLink struct {
	ProjectID     string `json:"projectId"`
	EnvironmentID string `json:"environmentId"`
}

// ListPromotionEnvironments returns one cursor-paginated page of environments.
func (c *Client) ListPromotionEnvironments(ctx context.Context, opts ListPromotionEnvironmentsOptions) (Page[PromotionEnvironment], error) {
	if err := opts.Validate(); err != nil {
		return Page[PromotionEnvironment]{}, err
	}
	var page Page[PromotionEnvironment]
	if _, err := c.Do(ctx, Request{Path: PromotionEnvironmentsPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[PromotionEnvironment]{}, err
	}
	return page, nil
}

// CreatePromotionEnvironment creates one environment. The instance can have
// one environment for each direction; a second one answers 409.
func (c *Client) CreatePromotionEnvironment(ctx context.Context, request CreatePromotionEnvironmentRequest) (*PromotionEnvironment, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var created PromotionEnvironment
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PromotionEnvironmentsPath,
		Body:   request,
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetPromotionEnvironment returns one environment with its checkout state.
func (c *Client) GetPromotionEnvironment(ctx context.Context, id string) (*PromotionEnvironment, error) {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return nil, err
	}
	var environment PromotionEnvironment
	if _, err := c.Do(ctx, Request{Path: PathJoin("promotions", "environments", id)}, &environment); err != nil {
		return nil, err
	}
	return &environment, nil
}

// UpdatePromotionEnvironment patches one environment.
func (c *Client) UpdatePromotionEnvironment(ctx context.Context, id string, request UpdatePromotionEnvironmentRequest) (*PromotionEnvironment, error) {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var updated PromotionEnvironment
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("promotions", "environments", id),
		Body:   request,
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeletePromotionEnvironment removes one environment with its project links
// and local checkout. Its provider is kept and nothing in Git changes.
func (c *Client) DeletePromotionEnvironment(ctx context.Context, id string) error {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("promotions", "environments", id)}, nil)
	return err
}

// ConnectPromotionEnvironment prepares the environment's checkout on the
// instance that handles the request. It is safe to repeat.
func (c *Client) ConnectPromotionEnvironment(ctx context.Context, id string) (*PromotionEnvironmentCheckout, error) {
	return c.promotionEnvironmentCheckout(ctx, id, "connect")
}

// DisconnectPromotionEnvironment removes the environment's local checkout and
// keeps the environment, its credentials, and its trusted SSH host keys.
func (c *Client) DisconnectPromotionEnvironment(ctx context.Context, id string) (*PromotionEnvironmentCheckout, error) {
	return c.promotionEnvironmentCheckout(ctx, id, "disconnect")
}

func (c *Client) promotionEnvironmentCheckout(ctx context.Context, id, action string) (*PromotionEnvironmentCheckout, error) {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return nil, err
	}
	var checkout PromotionEnvironmentCheckout
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "environments", id, action),
	}, &checkout); err != nil {
		return nil, err
	}
	return &checkout, nil
}

// ListPromotionEnvironmentProjects returns the team projects linked to one
// environment.
func (c *Client) ListPromotionEnvironmentProjects(ctx context.Context, id string) (*PromotionEnvironmentProjects, error) {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return nil, err
	}
	var projects PromotionEnvironmentProjects
	if _, err := c.Do(ctx, Request{Path: PathJoin("promotions", "environments", id, "projects")}, &projects); err != nil {
		return nil, err
	}
	return &projects, nil
}

// AddProjectToPromotionEnvironment links one team project to a projects-scope
// environment. A project can have one environment per direction; a second
// link for the same direction answers 409.
func (c *Client) AddProjectToPromotionEnvironment(ctx context.Context, id, projectID string) (*PromotionEnvironmentProjectLink, error) {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return nil, err
	}
	if err := validatePromotionID(projectID, "project ID"); err != nil {
		return nil, err
	}
	var link PromotionEnvironmentProjectLink
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "environments", id, "projects", projectID),
	}, &link); err != nil {
		return nil, err
	}
	return &link, nil
}

// RemoveProjectFromPromotionEnvironment unlinks one project from an
// environment. The project itself is untouched.
func (c *Client) RemoveProjectFromPromotionEnvironment(ctx context.Context, id, projectID string) error {
	if err := ValidatePromotionEnvironmentID(id); err != nil {
		return err
	}
	if err := validatePromotionID(projectID, "project ID"); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("promotions", "environments", id, "projects", projectID),
	}, nil)
	return err
}
