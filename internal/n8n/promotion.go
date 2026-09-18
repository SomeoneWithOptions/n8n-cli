package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// Promotions is the upstream-only generation of the Git sync module. The
// target instance serves GitConnections instead; both share the
// gitConnection:* scope namespace. The two generations are separate command
// groups with separate types.
const (
	PromotionProvidersPath   = "/promotions/providers"
	PromotionConnectionsPath = "/promotions/connections"
)

const (
	PromotionScopeInstance = "instance"
	PromotionScopeProjects = "projects"
)

var promotionScopes = []string{PromotionScopeInstance, PromotionScopeProjects}

const (
	PromotionDirectionApply   = "apply"
	PromotionDirectionPromote = "promote"
)

var promotionDirections = []string{PromotionDirectionApply, PromotionDirectionPromote}

const (
	PromotionProviderTypeGit = "git"
)

const (
	PromotionAuthTypeSSHKey = "ssh-key"
	PromotionAuthTypeToken  = "token"
)

var promotionAuthTypes = []string{PromotionAuthTypeSSHKey, PromotionAuthTypeToken}

const (
	PromotionKeyTypeED25519 = "ed25519"
	PromotionKeyTypeRSA     = "rsa"
)

var promotionKeyTypes = []string{PromotionKeyTypeED25519, PromotionKeyTypeRSA}

const (
	maxPromotionNameLength       = 128
	maxPromotionBranchLength     = 255
	maxPromotionCommitLength     = 1000
	maxPromotionIDLength         = 36
	maxPromotionRemoteURLLength  = 1 << 10
	promotionTargetSchemaVersion = 1
)

// PromotionProviderConfig is the public provider configuration. SSH providers
// report their public key and key type; token providers report only the schema
// version because the secret stays server-side.
type PromotionProviderConfig struct {
	SchemaVersion int     `json:"schemaVersion"`
	PublicKey     *string `json:"publicKey,omitempty"`
	KeyType       *string `json:"keyType,omitempty"`
}

// PromotionProvider is one promotion provider as list, get and update report
// it. Config is nil on list rows, which omit it.
type PromotionProvider struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Type      string                   `json:"type"`
	AuthType  string                   `json:"authType"`
	Config    *PromotionProviderConfig `json:"config,omitempty"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
}

// PromotionProviderCreateResult is the provider creation acknowledgement: the
// stored provider plus the deploy public key for ssh-key providers.
type PromotionProviderCreateResult struct {
	Provider  PromotionProvider `json:"provider"`
	PublicKey *string           `json:"publicKey"`
}

// PromotionProviderAuth carries one provider credential document. SSH-key auth
// takes an optional key type and no username or password; token auth takes a
// username and password and no key type.
type PromotionProviderAuth struct {
	AuthType string `json:"authType"`
	KeyType  string `json:"keyType,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// Validate rejects a credential document the API would reject before transport.
func (a PromotionProviderAuth) Validate() error {
	switch a.AuthType {
	case PromotionAuthTypeSSHKey:
		if a.KeyType != "" && !slices.Contains(promotionKeyTypes, a.KeyType) {
			return fmt.Errorf("unknown key type %q: use %s", a.KeyType, strings.Join(promotionKeyTypes, ", "))
		}
		if a.Username != "" || a.Password != "" {
			return fmt.Errorf("ssh-key auth takes no username or password: use token auth for username and password")
		}
		return nil
	case PromotionAuthTypeToken:
		if a.KeyType != "" {
			return fmt.Errorf("token auth takes no key type: use ssh-key auth for key types")
		}
		if strings.TrimSpace(a.Username) == "" {
			return fmt.Errorf("username is required for token auth")
		}
		if a.Password == "" {
			return fmt.Errorf("password is required for token auth")
		}
		return nil
	default:
		return fmt.Errorf("unknown auth type %q: use %s", a.AuthType, strings.Join(promotionAuthTypes, ", "))
	}
}

// CreatePromotionProviderRequest creates one promotion provider.
type CreatePromotionProviderRequest struct {
	Name string                `json:"name"`
	Type string                `json:"type"`
	Auth PromotionProviderAuth `json:"auth"`
}

// Validate rejects a creation document the API would reject before transport.
func (r CreatePromotionProviderRequest) Validate() error {
	if err := validatePromotionName(r.Name); err != nil {
		return err
	}
	if r.Type != PromotionProviderTypeGit {
		return fmt.Errorf("unknown provider type %q: use %q", r.Type, PromotionProviderTypeGit)
	}
	if err := r.Auth.Validate(); err != nil {
		return err
	}
	return nil
}

// UpdatePromotionProviderRequest replaces the name and/or credential of one
// provider. At least one field is required. Sending auth replaces the stored
// credential for every connection attached to the provider.
type UpdatePromotionProviderRequest struct {
	Name *string                `json:"name,omitempty"`
	Auth *PromotionProviderAuth `json:"auth,omitempty"`
}

// Validate rejects an empty or malformed update document before transport.
func (r UpdatePromotionProviderRequest) Validate() error {
	if r.Name == nil && r.Auth == nil {
		return fmt.Errorf("promotion provider update is empty: set name and/or auth")
	}
	if r.Name != nil {
		if err := validatePromotionName(*r.Name); err != nil {
			return err
		}
	}
	if r.Auth != nil {
		if err := r.Auth.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// PromotionTarget is the remote a connection syncs through.
type PromotionTarget struct {
	SchemaVersion int    `json:"schemaVersion"`
	RemoteURL     string `json:"remoteUrl"`
}

// Validate rejects a target the API would reject before transport.
func (t PromotionTarget) Validate() error {
	if t.SchemaVersion != promotionTargetSchemaVersion {
		return fmt.Errorf("target schemaVersion must be %d, got %d", promotionTargetSchemaVersion, t.SchemaVersion)
	}
	if strings.TrimSpace(t.RemoteURL) == "" {
		return fmt.Errorf("target remoteUrl is required")
	}
	return nil
}

// PromotionProviderRef is the provider summary embedded in a connection.
type PromotionProviderRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	AuthType  string `json:"authType"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// PromotionApplySettings configures the apply direction checkout.
type PromotionApplySettings struct {
	SchemaVersion int    `json:"schemaVersion"`
	BranchName    string `json:"branchName"`
}

// PromotionPromoteSettings configures the promote direction checkout.
type PromotionPromoteSettings struct {
	SchemaVersion         int    `json:"schemaVersion"`
	BaseBranchName        string `json:"baseBranchName"`
	CreateBranchOnPromote bool   `json:"createBranchOnPromotion"`
}

// PromotionApplyConfig is one stored apply direction config.
type PromotionApplyConfig struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
	Settings  PromotionApplySettings `json:"settings"`
}

// PromotionPromoteConfig is one stored promote direction config.
type PromotionPromoteConfig struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
	Settings  PromotionPromoteSettings `json:"settings"`
}

// PromotionConnectionConfigs holds the two optional direction configs of one
// connection. A direction without a config has no checkout.
type PromotionConnectionConfigs struct {
	Apply   *PromotionApplyConfig   `json:"apply,omitempty"`
	Promote *PromotionPromoteConfig `json:"promote,omitempty"`
}

// PromotionConnection is one promotion connection as the public DTO reports
// it. Secrets are never returned.
type PromotionConnection struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	Scope     string                     `json:"scope"`
	Target    PromotionTarget            `json:"target"`
	Provider  PromotionProviderRef       `json:"provider"`
	Configs   PromotionConnectionConfigs `json:"configs"`
	CreatedAt string                     `json:"createdAt"`
	UpdatedAt string                     `json:"updatedAt"`
}

// CreatePromotionApplyConfig is the optional apply config of a connection
// creation document.
type CreatePromotionApplyConfig struct {
	Name     string                 `json:"name,omitempty"`
	Settings PromotionApplySettings `json:"settings"`
}

// CreatePromotionPromoteConfig is the optional promote config of a connection
// creation document.
type CreatePromotionPromoteConfig struct {
	Name     string                   `json:"name,omitempty"`
	Settings PromotionPromoteSettings `json:"settings"`
}

// CreatePromotionConnectionConfigs groups the optional direction configs of a
// creation document.
type CreatePromotionConnectionConfigs struct {
	Apply   *CreatePromotionApplyConfig   `json:"apply,omitempty"`
	Promote *CreatePromotionPromoteConfig `json:"promote,omitempty"`
}

// CreatePromotionConnectionRequest creates one promotion connection with its
// remote target and optional direction configs.
type CreatePromotionConnectionRequest struct {
	Name       string                            `json:"name"`
	Scope      string                            `json:"scope"`
	ProviderID string                            `json:"providerId"`
	Target     PromotionTarget                   `json:"target"`
	Configs    *CreatePromotionConnectionConfigs `json:"configs,omitempty"`
}

// Validate rejects a creation document the API would reject before transport.
func (r CreatePromotionConnectionRequest) Validate() error {
	if err := validatePromotionName(r.Name); err != nil {
		return err
	}
	if err := validatePromotionScope(r.Scope); err != nil {
		return err
	}
	if err := validatePromotionID(r.ProviderID, "provider ID"); err != nil {
		return err
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if r.Configs != nil {
		if r.Configs.Apply != nil {
			if err := validatePromotionApplySettings(r.Configs.Apply.Settings, r.Configs.Apply.Name); err != nil {
				return fmt.Errorf("configs.apply: %w", err)
			}
		}
		if r.Configs.Promote != nil {
			if err := validatePromotionPromoteSettings(r.Configs.Promote.Settings, r.Configs.Promote.Name); err != nil {
				return fmt.Errorf("configs.promote: %w", err)
			}
		}
	}
	return nil
}

// UpdatePromotionConnectionRequest patches a connection. Only the supplied
// fields change; at least one is required.
type UpdatePromotionConnectionRequest struct {
	Name       *string          `json:"name,omitempty"`
	Target     *PromotionTarget `json:"target,omitempty"`
	ProviderID *string          `json:"providerId,omitempty"`
}

// Validate rejects an empty or malformed update document before transport.
func (r UpdatePromotionConnectionRequest) Validate() error {
	if r.Name == nil && r.Target == nil && r.ProviderID == nil {
		return fmt.Errorf("promotion connection update is empty: set at least one of name, target, or providerId")
	}
	if r.Name != nil {
		if err := validatePromotionName(*r.Name); err != nil {
			return err
		}
	}
	if r.Target != nil {
		if err := r.Target.Validate(); err != nil {
			return err
		}
	}
	if r.ProviderID != nil {
		if err := validatePromotionID(*r.ProviderID, "provider ID"); err != nil {
			return err
		}
	}
	return nil
}

// ListPromotionConnectionsOptions filters the connection list. Scope and
// provider ID are the only query filters the contract documents.
type ListPromotionConnectionsOptions struct {
	ListOptions
	Scope      string
	ProviderID string
}

// Apply writes pagination plus the documented filters into q.
func (o ListPromotionConnectionsOptions) Apply(q url.Values) url.Values {
	q = o.ListOptions.Apply(q)
	if o.Scope != "" {
		q.Set("scope", o.Scope)
	}
	if o.ProviderID != "" {
		q.Set("providerId", o.ProviderID)
	}
	return q
}

// Validate rejects options the API would reject before transport.
func (o ListPromotionConnectionsOptions) Validate() error {
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
	if o.ProviderID != "" {
		if err := validatePromotionID(o.ProviderID, "provider ID"); err != nil {
			return err
		}
	}
	return nil
}

// UpsertPromotionApplyConfigRequest replaces the apply direction config of one
// connection, creating it on first write.
type UpsertPromotionApplyConfigRequest struct {
	Name     string                 `json:"name,omitempty"`
	Settings PromotionApplySettings `json:"settings"`
}

// Validate rejects a config document the API would reject before transport.
func (r UpsertPromotionApplyConfigRequest) Validate() error {
	return validatePromotionApplySettings(r.Settings, r.Name)
}

// UpsertPromotionPromoteConfigRequest replaces the promote direction config of
// one connection, creating it on first write.
type UpsertPromotionPromoteConfigRequest struct {
	Name     string                   `json:"name,omitempty"`
	Settings PromotionPromoteSettings `json:"settings"`
}

// Validate rejects a config document the API would reject before transport.
func (r UpsertPromotionPromoteConfigRequest) Validate() error {
	return validatePromotionPromoteSettings(r.Settings, r.Name)
}

// PromotionCheckout is the checkout state of one direction of a connection.
type PromotionCheckout struct {
	ConnectionID string `json:"connectionId"`
	ConfigID     string `json:"configId"`
	Direction    string `json:"direction"`
	BranchName   string `json:"branchName"`
	HasCheckout  bool   `json:"hasCheckout"`
}

// PromotionConnectionProjects is the project-link response: the team projects
// added to one connection.
type PromotionConnectionProjects struct {
	ProjectIDs []string `json:"projectIds"`
}

// PromotionProjectLink acknowledges adding one project to a connection.
type PromotionProjectLink struct {
	ProjectID    string `json:"projectId"`
	ConnectionID string `json:"connectionId"`
}

// PromotePromotionRequest commits every team project and pushes to the remote.
type PromotePromotionRequest struct {
	CommitMessage string `json:"commitMessage"`
	Force         bool   `json:"force,omitempty"`
}

// Validate rejects a promote document the API would reject before transport.
func (r PromotePromotionRequest) Validate() error {
	if strings.TrimSpace(r.CommitMessage) == "" {
		return fmt.Errorf("commit message is required")
	}
	if utf8.RuneCountInString(r.CommitMessage) > maxPromotionCommitLength {
		return fmt.Errorf("commit message must contain at most %d characters", maxPromotionCommitLength)
	}
	return nil
}

// PromotionPromoteCounts counts what one promote exported, committed and pushed.
type PromotionPromoteCounts struct {
	Workflows   int `json:"workflows"`
	Folders     int `json:"folders"`
	Credentials int `json:"credentials"`
	DataTables  int `json:"dataTables"`
	Variables   int `json:"variables"`
	Tags        int `json:"tags"`
}

// PromotionGit names the commit and branch one promote or apply settled on.
type PromotionGit struct {
	CommitSHA  string `json:"commitSha"`
	BranchName string `json:"branchName"`
}

// PromotePromotionResult is the promote acknowledgement.
type PromotePromotionResult struct {
	ConnectionID string                 `json:"connectionId"`
	ConfigID     string                 `json:"configId"`
	Counts       PromotionPromoteCounts `json:"counts"`
	Git          PromotionGit           `json:"git"`
}

// PromotionApplyProjectCounts counts project rows created, updated, skipped or
// deleted by one apply import.
type PromotionApplyProjectCounts struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Deleted int `json:"deleted"`
}

// PromotionApplyFolderCounts counts folders created, skipped or removed.
type PromotionApplyFolderCounts struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Removed int `json:"removed"`
}

// PromotionApplyPublishingCounts counts per-workflow publishing outcomes.
type PromotionApplyPublishingCounts struct {
	Published   int `json:"published"`
	Unpublished int `json:"unpublished"`
	Unchanged   int `json:"unchanged"`
	Blocked     int `json:"blocked"`
	Failed      int `json:"failed"`
}

// PromotionApplyWorkflowCounts counts workflows created, updated, skipped,
// archived or deleted by one apply.
type PromotionApplyWorkflowCounts struct {
	Created    int                            `json:"created"`
	Updated    int                            `json:"updated"`
	Skipped    int                            `json:"skipped"`
	Archived   int                            `json:"archived"`
	Deleted    int                            `json:"deleted"`
	Publishing PromotionApplyPublishingCounts `json:"publishing"`
}

// PromotionApplyCredentialCounts counts credentials matched or stubbed.
type PromotionApplyCredentialCounts struct {
	Matched int `json:"matched"`
	Stubbed int `json:"stubbed"`
}

// PromotionApplyDataTableCounts counts data tables matched or created.
type PromotionApplyDataTableCounts struct {
	Matched int `json:"matched"`
	Created int `json:"created"`
}

// PromotionApplyVariableCounts counts variables by outcome of one apply.
type PromotionApplyVariableCounts struct {
	Matched int `json:"matched"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Stubbed int `json:"stubbed"`
	Missing int `json:"missing"`
}

// PromotionApplyTagCounts counts tags by outcome of one apply.
type PromotionApplyTagCounts struct {
	Matched    int `json:"matched"`
	Created    int `json:"created"`
	Renamed    int `json:"renamed"`
	Reconciled int `json:"reconciled"`
	Skipped    int `json:"skipped"`
}

// PromotionApplyCounts counts everything one apply import wrote.
type PromotionApplyCounts struct {
	Projects    PromotionApplyProjectCounts    `json:"projects"`
	Folders     PromotionApplyFolderCounts     `json:"folders"`
	Workflows   PromotionApplyWorkflowCounts   `json:"workflows"`
	Credentials PromotionApplyCredentialCounts `json:"credentials"`
	DataTables  PromotionApplyDataTableCounts  `json:"dataTables"`
	Variables   PromotionApplyVariableCounts   `json:"variables"`
	Tags        PromotionApplyTagCounts        `json:"tags"`
}

// ApplyPromotionResult is the apply acknowledgement: what the checkout
// overwrote and the commit it now matches.
type ApplyPromotionResult struct {
	ConnectionID string               `json:"connectionId"`
	ConfigID     string               `json:"configId"`
	Counts       PromotionApplyCounts `json:"counts"`
	Git          PromotionGit         `json:"git"`
}

// ListPromotionProviders returns one cursor-paginated page of providers.
func (c *Client) ListPromotionProviders(ctx context.Context, opts ListOptions) (Page[PromotionProvider], error) {
	if err := opts.Validate(); err != nil {
		return Page[PromotionProvider]{}, err
	}
	if opts.Limit > 250 {
		return Page[PromotionProvider]{}, fmt.Errorf("limit must not exceed 250, got %d", opts.Limit)
	}
	var page Page[PromotionProvider]
	if _, err := c.Do(ctx, Request{Path: PromotionProvidersPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[PromotionProvider]{}, err
	}
	return page, nil
}

// CreatePromotionProvider creates one promotion provider. Secret error details
// are redacted in case a server or proxy echoes the submitted password.
func (c *Client) CreatePromotionProvider(ctx context.Context, request CreatePromotionProviderRequest) (*PromotionProviderCreateResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var created PromotionProviderCreateResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PromotionProvidersPath,
		Body:   request,
	}, &created); err != nil {
		return nil, redactPromotionProviderAPIError(err)
	}
	return &created, nil
}

// GetPromotionProvider returns one promotion provider.
func (c *Client) GetPromotionProvider(ctx context.Context, id string) (*PromotionProvider, error) {
	if err := validatePromotionID(id, "promotion provider ID"); err != nil {
		return nil, err
	}
	var provider PromotionProvider
	if _, err := c.Do(ctx, Request{Path: PathJoin("promotions", "providers", id)}, &provider); err != nil {
		return nil, err
	}
	return &provider, nil
}

// UpdatePromotionProvider replaces the name and/or credential of one provider.
// Secret error details are redacted like on create.
func (c *Client) UpdatePromotionProvider(ctx context.Context, id string, request UpdatePromotionProviderRequest) (*PromotionProvider, error) {
	if err := validatePromotionID(id, "promotion provider ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var updated PromotionProvider
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("promotions", "providers", id),
		Body:   request,
	}, &updated); err != nil {
		return nil, redactPromotionProviderAPIError(err)
	}
	return &updated, nil
}

// DeletePromotionProvider removes one provider. Attached connections keep
// their stored content but lose their credential.
func (c *Client) DeletePromotionProvider(ctx context.Context, id string) error {
	if err := validatePromotionID(id, "promotion provider ID"); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("promotions", "providers", id)}, nil)
	return err
}

// ListPromotionConnections returns one page of promotion connections, with the
// only documented query filters applied.
func (c *Client) ListPromotionConnections(ctx context.Context, opts ListPromotionConnectionsOptions) (Page[PromotionConnection], error) {
	if err := opts.Validate(); err != nil {
		return Page[PromotionConnection]{}, err
	}
	var page Page[PromotionConnection]
	if _, err := c.Do(ctx, Request{Path: PromotionConnectionsPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[PromotionConnection]{}, err
	}
	return page, nil
}

// CreatePromotionConnection creates one promotion connection.
func (c *Client) CreatePromotionConnection(ctx context.Context, request CreatePromotionConnectionRequest) (*PromotionConnection, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var created PromotionConnection
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PromotionConnectionsPath,
		Body:   request,
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetPromotionConnection returns one promotion connection.
func (c *Client) GetPromotionConnection(ctx context.Context, id string) (*PromotionConnection, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	var connection PromotionConnection
	if _, err := c.Do(ctx, Request{Path: PathJoin("promotions", "connections", id)}, &connection); err != nil {
		return nil, err
	}
	return &connection, nil
}

// UpdatePromotionConnection patches one promotion connection.
func (c *Client) UpdatePromotionConnection(ctx context.Context, id string, request UpdatePromotionConnectionRequest) (*PromotionConnection, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var updated PromotionConnection
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("promotions", "connections", id),
		Body:   request,
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeletePromotionConnection removes one connection and can remove its local
// checkouts.
func (c *Client) DeletePromotionConnection(ctx context.Context, id string) error {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("promotions", "connections", id)}, nil)
	return err
}

// UpsertPromotionApplyConfig replaces the apply direction config of one
// connection, creating it on first write.
func (c *Client) UpsertPromotionApplyConfig(ctx context.Context, id string, request UpsertPromotionApplyConfigRequest) (*PromotionApplyConfig, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var config PromotionApplyConfig
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("promotions", "connections", id, "configs", "apply"),
		Body:   request,
	}, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpsertPromotionPromoteConfig replaces the promote direction config of one
// connection, creating it on first write.
func (c *Client) UpsertPromotionPromoteConfig(ctx context.Context, id string, request UpsertPromotionPromoteConfigRequest) (*PromotionPromoteConfig, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var config PromotionPromoteConfig
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("promotions", "connections", id, "configs", "promote"),
		Body:   request,
	}, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// DeletePromotionConfig removes one direction config and can remove its local
// checkout. Direction is apply or promote.
func (c *Client) DeletePromotionConfig(ctx context.Context, id, direction string) error {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return err
	}
	if err := validatePromotionDirection(direction); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("promotions", "connections", id, "configs", direction),
	}, nil)
	return err
}

// ClonePromotionCheckout clones one direction checkout into local storage.
func (c *Client) ClonePromotionCheckout(ctx context.Context, id, direction string) (*PromotionCheckout, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := validatePromotionDirection(direction); err != nil {
		return nil, err
	}
	var checkout PromotionCheckout
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, direction, "clone"),
	}, &checkout); err != nil {
		return nil, err
	}
	return &checkout, nil
}

// DisconnectPromotionCheckout removes the local checkout of one direction. The
// connection, its configs and its credentials are retained.
func (c *Client) DisconnectPromotionCheckout(ctx context.Context, id, direction string) (*PromotionCheckout, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := validatePromotionDirection(direction); err != nil {
		return nil, err
	}
	var checkout PromotionCheckout
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, direction, "disconnect"),
	}, &checkout); err != nil {
		return nil, err
	}
	return &checkout, nil
}

// ListPromotionConnectionProjects returns the team projects added to one
// connection.
func (c *Client) ListPromotionConnectionProjects(ctx context.Context, id string) (*PromotionConnectionProjects, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	var projects PromotionConnectionProjects
	if _, err := c.Do(ctx, Request{Path: PathJoin("promotions", "connections", id, "projects")}, &projects); err != nil {
		return nil, err
	}
	return &projects, nil
}

// AddProjectToPromotionConnection adds one team project to a connection. A
// project can belong to only one connection; a second link answers 409.
func (c *Client) AddProjectToPromotionConnection(ctx context.Context, id, projectID string) (*PromotionProjectLink, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := validatePromotionID(projectID, "project ID"); err != nil {
		return nil, err
	}
	var link PromotionProjectLink
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, "projects", projectID),
	}, &link); err != nil {
		return nil, err
	}
	return &link, nil
}

// RemoveProjectFromPromotionConnection unlinks one project from a connection.
// The project itself is untouched.
func (c *Client) RemoveProjectFromPromotionConnection(ctx context.Context, id, projectID string) error {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return err
	}
	if err := validatePromotionID(projectID, "project ID"); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("promotions", "connections", id, "projects", projectID),
	}, nil)
	return err
}

// PromotePackage exports all team projects, commits them and pushes to the
// remote. The promote checkout must be cloned first.
func (c *Client) PromotePackage(ctx context.Context, id string, request PromotePromotionRequest) (*PromotePromotionResult, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result PromotePromotionResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, "promote"),
		Body:   request,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ApplyPackage resets the apply checkout to the branch tip and imports
// projects into the instance, overwriting to match. The apply checkout must be
// cloned first.
func (c *Client) ApplyPackage(ctx context.Context, id string) (*ApplyPromotionResult, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	var result ApplyPromotionResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, "apply"),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validatePromotionID(id, label string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("%s must not start or end with whitespace", label)
	}
	if utf8.RuneCountInString(id) > maxPromotionIDLength {
		return fmt.Errorf("%s must contain at most %d characters", label, maxPromotionIDLength)
	}
	return nil
}

func validatePromotionName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("promotion name is required")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("promotion name must not start or end with whitespace")
	}
	if utf8.RuneCountInString(name) > maxPromotionNameLength {
		return fmt.Errorf("promotion name must contain at most %d characters", maxPromotionNameLength)
	}
	return nil
}

func validatePromotionScope(scope string) error {
	if !slices.Contains(promotionScopes, scope) {
		return fmt.Errorf("unknown promotion scope %q: use %s", scope, strings.Join(promotionScopes, ", "))
	}
	return nil
}

func validatePromotionDirection(direction string) error {
	if !slices.Contains(promotionDirections, direction) {
		return fmt.Errorf("unknown promotion direction %q: use %s", direction, strings.Join(promotionDirections, ", "))
	}
	return nil
}

func validatePromotionBranch(branch string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch name is required and must not be blank")
	}
	if strings.TrimSpace(branch) != branch {
		return fmt.Errorf("branch name must not start or end with whitespace")
	}
	if utf8.RuneCountInString(branch) > maxPromotionBranchLength {
		return fmt.Errorf("branch name must contain at most %d characters", maxPromotionBranchLength)
	}
	return nil
}

func validatePromotionApplySettings(settings PromotionApplySettings, name string) error {
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("name must not start or end with whitespace")
	}
	if name != "" && utf8.RuneCountInString(name) > maxPromotionNameLength {
		return fmt.Errorf("name must contain at most %d characters", maxPromotionNameLength)
	}
	if settings.SchemaVersion != promotionTargetSchemaVersion {
		return fmt.Errorf("settings schemaVersion must be %d, got %d", promotionTargetSchemaVersion, settings.SchemaVersion)
	}
	if err := validatePromotionBranch(settings.BranchName); err != nil {
		return err
	}
	return nil
}

func validatePromotionPromoteSettings(settings PromotionPromoteSettings, name string) error {
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("name must not start or end with whitespace")
	}
	if name != "" && utf8.RuneCountInString(name) > maxPromotionNameLength {
		return fmt.Errorf("name must contain at most %d characters", maxPromotionNameLength)
	}
	if settings.SchemaVersion != promotionTargetSchemaVersion {
		return fmt.Errorf("settings schemaVersion must be %d, got %d", promotionTargetSchemaVersion, settings.SchemaVersion)
	}
	if err := validatePromotionBranch(settings.BaseBranchName); err != nil {
		return fmt.Errorf("base branch: %w", err)
	}
	return nil
}

// redactPromotionProviderAPIError prevents server-controlled error text from
// echoing a submitted password while preserving HTTP status classification.
func redactPromotionProviderAPIError(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "promotion provider request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
