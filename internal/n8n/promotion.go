package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
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

// Provider types. A gitlab provider is a Git host provider: it talks to the
// GitLab API, so it can list repositories and carries an optional base URL for
// self-managed GitLab. A git provider is a plain Git remote.
const (
	PromotionProviderTypeGit    = "git"
	PromotionProviderTypeGitLab = "gitlab"
)

var promotionProviderTypes = []string{PromotionProviderTypeGit, PromotionProviderTypeGitLab}

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
// report their public key and key type; GitLab providers report their base
// URL; token providers otherwise report only the schema version because the
// secret stays server-side.
type PromotionProviderConfig struct {
	SchemaVersion int     `json:"schemaVersion"`
	PublicKey     *string `json:"publicKey,omitempty"`
	KeyType       *string `json:"keyType,omitempty"`
	BaseURL       *string `json:"baseUrl,omitempty"`
}

// PromotionProviderHostConfig is the write document for a Git host provider
// configuration: the base URL of a self-managed GitLab.
type PromotionProviderHostConfig struct {
	SchemaVersion int    `json:"schemaVersion"`
	BaseURL       string `json:"baseUrl"`
}

// Validate rejects a host configuration the API would reject before transport.
func (c PromotionProviderHostConfig) Validate() error {
	if c.SchemaVersion != promotionTargetSchemaVersion {
		return fmt.Errorf("config schemaVersion must be %d, got %d", promotionTargetSchemaVersion, c.SchemaVersion)
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("config baseUrl is required: pass the GitLab URL, e.g. https://gitlab.example.com")
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return fmt.Errorf("config baseUrl %q must be an absolute http or https URL, e.g. https://gitlab.example.com", c.BaseURL)
	}
	return nil
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
// password (the token) and no key type. A git provider also needs a username
// for token auth; GitLab ignores it.
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
		if a.Username != "" && strings.TrimSpace(a.Username) == "" {
			return fmt.Errorf("username must not be blank: omit it or pass the Git username")
		}
		if a.Password == "" {
			return fmt.Errorf("password is required for token auth")
		}
		return nil
	default:
		return fmt.Errorf("unknown auth type %q: use %s", a.AuthType, strings.Join(promotionAuthTypes, ", "))
	}
}

// CreatePromotionProviderRequest creates one promotion provider. Config is
// the optional Git host configuration (GitLab base URL).
type CreatePromotionProviderRequest struct {
	Name   string                       `json:"name"`
	Type   string                       `json:"type"`
	Auth   PromotionProviderAuth        `json:"auth"`
	Config *PromotionProviderHostConfig `json:"config,omitempty"`
}

// Validate rejects a creation document the API would reject before transport.
func (r CreatePromotionProviderRequest) Validate() error {
	if err := validatePromotionName(r.Name); err != nil {
		return err
	}
	if !slices.Contains(promotionProviderTypes, r.Type) {
		return fmt.Errorf("unknown provider type %q: use %s", r.Type, strings.Join(promotionProviderTypes, ", "))
	}
	if err := r.Auth.Validate(); err != nil {
		return err
	}
	if r.Type == PromotionProviderTypeGit && r.Auth.AuthType == PromotionAuthTypeToken && r.Auth.Username == "" {
		return fmt.Errorf("username is required for token auth on a git provider (GitLab providers ignore it)")
	}
	if r.Config != nil {
		if r.Type != PromotionProviderTypeGitLab {
			return fmt.Errorf("config (baseUrl) applies to gitlab providers only, not %q", r.Type)
		}
		if err := r.Config.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePromotionProviderRequest replaces the name, credential and/or host
// configuration of one provider. At least one field is required. Sending auth
// replaces the stored credential for every connection attached to the
// provider; a changed base URL needs auth in the same request.
type UpdatePromotionProviderRequest struct {
	Name   *string                      `json:"name,omitempty"`
	Auth   *PromotionProviderAuth       `json:"auth,omitempty"`
	Config *PromotionProviderHostConfig `json:"config,omitempty"`
}

// Validate rejects an empty or malformed update document before transport.
// The provider type is not known here, so a missing token username is left to
// the server.
func (r UpdatePromotionProviderRequest) Validate() error {
	if r.Name == nil && r.Auth == nil && r.Config == nil {
		return fmt.Errorf("promotion provider update is empty: set at least one of name, auth, or config")
	}
	if r.Config != nil {
		if err := r.Config.Validate(); err != nil {
			return err
		}
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

// PromotionTarget is the remote a connection syncs through, as the API
// reports it. GitLab connections also report the stored repository path,
// which is read-only: write documents use [PromotionTargetDocument].
type PromotionTarget struct {
	SchemaVersion  int    `json:"schemaVersion"`
	RemoteURL      string `json:"remoteUrl"`
	RepositoryPath string `json:"repositoryPath,omitempty"`
}

// Document returns the writable part of the target.
func (t PromotionTarget) Document() PromotionTargetDocument {
	return PromotionTargetDocument{SchemaVersion: t.SchemaVersion, RemoteURL: t.RemoteURL}
}

// PromotionTargetDocument is the target a connection create or update sends.
// For GitLab the remote URL must belong to the provider; the server stores
// only the repository path and derives remoteUrl from the provider base URL.
type PromotionTargetDocument struct {
	SchemaVersion int    `json:"schemaVersion"`
	RemoteURL     string `json:"remoteUrl"`
}

// UnmarshalJSON decodes strictly and names the read-only repositoryPath, so a
// target copied from 'connection get' fails with the fix instead of a bare
// unknown-field error.
func (t *PromotionTargetDocument) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, ok := fields["repositoryPath"]; ok {
		return fmt.Errorf("target.repositoryPath is read-only: send only schemaVersion and remoteUrl")
	}
	type plain PromotionTargetDocument
	var decoded plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*t = PromotionTargetDocument(decoded)
	return nil
}

// Validate rejects a target the API would reject before transport.
func (t PromotionTargetDocument) Validate() error {
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

// PromotionConfigCheckout is the local checkout state of one direction config:
// whether it is cloned and whether the clone still matches the config.
type PromotionConfigCheckout struct {
	HasCheckout   bool `json:"hasCheckout"`
	MatchesConfig bool `json:"matchesConfig"`
}

// PromotionApplyConfig is one stored apply direction config. Checkout is nil
// on connection list rows, which omit it.
type PromotionApplyConfig struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
	Checkout  *PromotionConfigCheckout `json:"checkout,omitempty"`
	Settings  PromotionApplySettings   `json:"settings"`
}

// PromotionPromoteConfig is one stored promote direction config. Checkout is
// nil on connection list rows, which omit it.
type PromotionPromoteConfig struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
	Checkout  *PromotionConfigCheckout `json:"checkout,omitempty"`
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
	Target     PromotionTargetDocument           `json:"target"`
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
	Name       *string                  `json:"name,omitempty"`
	Target     *PromotionTargetDocument `json:"target,omitempty"`
	ProviderID *string                  `json:"providerId,omitempty"`
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

// PromotionApplyDataTableCounts counts data tables matched, created or updated.
type PromotionApplyDataTableCounts struct {
	Matched int `json:"matched"`
	Created int `json:"created"`
	Updated int `json:"updated"`
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

// Apply outcomes. The apply response is a union on status: only applied
// wrote anything. An unknown status is kept as sent so a newer server never
// fails the decode.
const (
	PromotionApplyStatusApplied       = "applied"
	PromotionApplyStatusBlocked       = "blocked"
	PromotionApplyStatusSourceChanged = "source-changed"
)

// Preflight conflict codes that need explicit confirmation at continue time.
const (
	PromotionConflictProjectDeletion   = "project-deletion"
	PromotionConflictDestructiveChange = "destructive-change"
)

// PromotionRef names one project or workflow in a preflight item.
type PromotionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PromotionConsumer is one project and the workflows in it that reference a
// preflight item.
type PromotionConsumer struct {
	Project   PromotionRef   `json:"project"`
	Workflows []PromotionRef `json:"workflows"`
}

// PromotionVariableScope is the scope of a variable binding: global, or one
// project.
type PromotionVariableScope struct {
	Kind    string        `json:"kind"`
	Project *PromotionRef `json:"project,omitempty"`
}

// PromotionDataTableChange is one column or table change a data-table
// destructive-change conflict reports. Destructive changes drop column values.
type PromotionDataTableChange struct {
	Kind        string `json:"kind"`
	Column      string `json:"column,omitempty"`
	Type        string `json:"type,omitempty"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Destructive bool   `json:"destructive"`
}

// PromotionApplyIssue is one preflight item or warning: a missing binding,
// an access requirement, a conflict, or a warning. Kind (credential, variable,
// project, data-table) and Code discriminate; the remaining fields are the
// union of the documented variants, and only those a variant carries are set.
// The full document is preserved verbatim so new variants survive without a
// client change.
type PromotionApplyIssue struct {
	Kind           string                     `json:"kind"`
	Code           string                     `json:"code,omitempty"`
	ID             string                     `json:"id,omitempty"`
	Name           string                     `json:"name,omitempty"`
	SourceID       *string                    `json:"sourceId,omitempty"`
	CredentialType string                     `json:"credentialType,omitempty"`
	ExpectedTypes  []string                   `json:"expectedTypes,omitempty"`
	TargetType     string                     `json:"targetType,omitempty"`
	UsageScope     string                     `json:"usageScope,omitempty"`
	VariableType   string                     `json:"variableType,omitempty"`
	Scope          *PromotionVariableScope    `json:"scope,omitempty"`
	Project        *PromotionRef              `json:"project,omitempty"`
	OwnerProject   *PromotionRef              `json:"ownerProject,omitempty"`
	FilePath       string                     `json:"filePath,omitempty"`
	ReferenceFiles []string                   `json:"referenceFiles,omitempty"`
	Workflows      []PromotionRef             `json:"workflows,omitempty"`
	Consumers      []PromotionConsumer        `json:"consumers,omitempty"`
	Changes        []PromotionDataTableChange `json:"changes,omitempty"`
	Raw            json.RawMessage            `json:"-"`
}

// UnmarshalJSON decodes the typed view and keeps the verbatim document.
func (i *PromotionApplyIssue) UnmarshalJSON(data []byte) error {
	type plain PromotionApplyIssue
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*i = PromotionApplyIssue(decoded)
	i.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON re-emits the verbatim document when present.
func (i PromotionApplyIssue) MarshalJSON() ([]byte, error) {
	if len(i.Raw) > 0 {
		return i.Raw, nil
	}
	type plain PromotionApplyIssue
	return json.Marshal(plain(i))
}

// PromotionMissingProject is one package project that does not exist on the
// instance. Icon and telemetry tags stay raw: they are display metadata.
type PromotionMissingProject struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description,omitempty"`
	Icon                json.RawMessage `json:"icon,omitempty"`
	CustomTelemetryTags json.RawMessage `json:"customTelemetryTags,omitempty"`
}

// PromotionApplyPreflight lists why a blocked apply wrote nothing.
type PromotionApplyPreflight struct {
	MissingProjects    []PromotionMissingProject `json:"missingProjects"`
	MissingBindings    []PromotionApplyIssue     `json:"missingBindings"`
	AccessRequirements []PromotionApplyIssue     `json:"accessRequirements"`
	Conflicts          []PromotionApplyIssue     `json:"conflicts"`
	Warnings           []PromotionApplyIssue     `json:"warnings"`
}

// Empty reports whether the preflight lists nothing at all.
func (p PromotionApplyPreflight) Empty() bool {
	return len(p.MissingProjects) == 0 && len(p.MissingBindings) == 0 &&
		len(p.AccessRequirements) == 0 && len(p.Conflicts) == 0 && len(p.Warnings) == 0
}

// PromotionExpectedSource pins the source an apply or continue may import:
// the config, branch and full commit a review saw. A mismatch answers status
// source-changed and writes nothing.
type PromotionExpectedSource struct {
	ConfigID   string `json:"configId"`
	BranchName string `json:"branchName"`
	CommitSHA  string `json:"commitSha"`
}

var promotionCommitSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// Validate rejects an expected source the API would reject before transport.
func (s PromotionExpectedSource) Validate() error {
	if err := validatePromotionID(s.ConfigID, "expected source config ID"); err != nil {
		return err
	}
	if strings.TrimSpace(s.BranchName) == "" {
		return fmt.Errorf("expected source branch name is required")
	}
	if !promotionCommitSHA.MatchString(s.CommitSHA) {
		return fmt.Errorf("expected source commit %q must be a full lowercase SHA-1 (40 hex) or SHA-256 (64 hex)", s.CommitSHA)
	}
	return nil
}

// ApplyPromotionResult is the apply acknowledgement. Inspect Status before
// reading anything else: applied carries Counts and Warnings; blocked carries
// Preflight and wrote nothing; source-changed carries neither and wrote
// nothing. ConfigID and Git identify the source for a later continue.
//
// Extra holds top-level fields this type does not name. JSON encoding
// re-emits the verbatim response, so --output json is lossless even for
// fields and variants newer than the client.
type ApplyPromotionResult struct {
	ConnectionID string                     `json:"connectionId"`
	ConfigID     string                     `json:"configId"`
	Status       string                     `json:"status,omitempty"`
	Git          PromotionGit               `json:"git"`
	Counts       *PromotionApplyCounts      `json:"counts,omitempty"`
	Warnings     []PromotionApplyIssue      `json:"warnings,omitempty"`
	Preflight    *PromotionApplyPreflight   `json:"preflight,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
	raw          json.RawMessage
}

// applyPromotionResultFields are the field names [ApplyPromotionResult]
// decodes itself. Anything else lands in Extra.
var applyPromotionResultFields = []string{
	"connectionId", "configId", "status", "git", "counts", "warnings", "preflight",
}

// UnmarshalJSON decodes by shape, keeps unknown fields in Extra, and keeps the
// verbatim response for re-encoding.
func (r *ApplyPromotionResult) UnmarshalJSON(data []byte) error {
	type plain ApplyPromotionResult
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var rest map[string]json.RawMessage
	if err := json.Unmarshal(data, &rest); err != nil {
		return err
	}
	for _, known := range applyPromotionResultFields {
		delete(rest, known)
	}
	*r = ApplyPromotionResult(decoded)
	r.Extra = nil
	if len(rest) > 0 {
		r.Extra = rest
	}
	r.raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON re-emits the verbatim response when present; otherwise the
// named fields followed by Extra in sorted order.
func (r ApplyPromotionResult) MarshalJSON() ([]byte, error) {
	if len(r.raw) > 0 {
		return r.raw, nil
	}
	type plain ApplyPromotionResult
	base, err := json.Marshal(plain(r))
	if err != nil {
		return nil, err
	}
	if len(r.Extra) == 0 {
		return base, nil
	}
	var b bytes.Buffer
	b.Write(bytes.TrimSuffix(base, []byte("}")))
	for i, name := range slices.Sorted(maps.Keys(r.Extra)) {
		if i > 0 || len(base) > 2 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(r.Extra[name])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Applied reports whether the apply wrote content. A response without status
// but with counts comes from a server older than the status union and is
// treated as applied.
func (r ApplyPromotionResult) Applied() bool {
	return r.Status == PromotionApplyStatusApplied || (r.Status == "" && r.Counts != nil)
}

// ExpectedSource returns the source a continue must name: the config and the
// branch and commit this result was checked against.
func (r ApplyPromotionResult) ExpectedSource() PromotionExpectedSource {
	return PromotionExpectedSource{ConfigID: r.ConfigID, BranchName: r.Git.BranchName, CommitSHA: r.Git.CommitSHA}
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
// remote. The promote checkout must be cloned first. opts accepts
// [IdempotencyKey].
func (c *Client) PromotePackage(ctx context.Context, id string, request PromotePromotionRequest, opts ...CallOption) (*PromotePromotionResult, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result PromotePromotionResult
	req := Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, "promote"),
		Body:   request,
	}
	applyCallOptions(&req, opts)
	if _, err := c.Do(ctx, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ApplyPackage resets the apply checkout to the branch tip and imports
// projects into the instance, overwriting to match. The apply checkout must be
// cloned first. opts accepts [IdempotencyKey].
func (c *Client) ApplyPackage(ctx context.Context, id string, opts ...CallOption) (*ApplyPromotionResult, error) {
	if err := validatePromotionID(id, "promotion connection ID"); err != nil {
		return nil, err
	}
	var result ApplyPromotionResult
	req := Request{
		Method: http.MethodPost,
		Path:   PathJoin("promotions", "connections", id, "apply"),
	}
	applyCallOptions(&req, opts)
	if _, err := c.Do(ctx, req, &result); err != nil {
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
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
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
