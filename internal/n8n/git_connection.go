package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// GitConnectionsPath is the collection endpoint for the target-only Git
// connection module. It is what the target instance serves in place of the
// upstream Promotions generation; both share the gitConnection:* scope
// namespace.
const GitConnectionsPath = "/git-connections"

// Git connection types accepted by create and update documents.
const (
	GitConnectionTypeSSH   = "ssh"
	GitConnectionTypeHTTPS = "https"
)

var gitConnectionTypes = []string{GitConnectionTypeSSH, GitConnectionTypeHTTPS}

// SSH key generator types accepted by create and update documents.
const (
	GitKeyGeneratorED25519 = "ed25519"
	GitKeyGeneratorRSA     = "rsa"
)

var gitKeyGeneratorTypes = []string{GitKeyGeneratorED25519, GitKeyGeneratorRSA}

const (
	maxGitConnectionNameLength   = 128
	maxGitConnectionBranchLength = 255
	maxGitPushCommitLength       = 1000
)

// GitConnection is one Git connection as the public DTO reports it. Secrets
// are never returned: authentication material stays server-side and only the
// SSH public key to deploy travels back. Nullable fields stay pointers so an
// explicit null remains distinct from an absent value.
type GitConnection struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	RepositoryURL    string  `json:"repositoryUrl"`
	BranchName       *string `json:"branchName"`
	ConnectionType   string  `json:"connectionType"`
	PublicKey        *string `json:"publicKey"`
	KeyGeneratorType *string `json:"keyGeneratorType"`
	BaseCommit       *string `json:"baseCommit"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

// CreateGitConnectionRequest creates the single Git connection an instance
// holds. Name, repository URL and connection type are required; the rest is
// optional. Username and password travel only on the wire and are never part
// of any response.
type CreateGitConnectionRequest struct {
	Name             string `json:"name"`
	RepositoryURL    string `json:"repositoryUrl"`
	BranchName       string `json:"branchName,omitempty"`
	ConnectionType   string `json:"connectionType"`
	KeyGeneratorType string `json:"keyGeneratorType,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
}

// Validate rejects a creation document the API would reject before transport.
func (r CreateGitConnectionRequest) Validate() error {
	if err := validateGitConnectionName(r.Name); err != nil {
		return err
	}
	if strings.TrimSpace(r.RepositoryURL) == "" {
		return fmt.Errorf("repository URL is required")
	}
	if err := validateGitConnectionType(r.ConnectionType); err != nil {
		return err
	}
	if r.BranchName != "" {
		if err := validateGitConnectionBranch(r.BranchName); err != nil {
			return err
		}
	}
	if r.KeyGeneratorType != "" && !slices.Contains(gitKeyGeneratorTypes, r.KeyGeneratorType) {
		return fmt.Errorf("unknown key generator type %q: use %s", r.KeyGeneratorType, strings.Join(gitKeyGeneratorTypes, ", "))
	}
	if r.Username != "" && strings.TrimSpace(r.Username) == "" {
		return fmt.Errorf("username must not be blank")
	}
	if r.Password != "" && strings.TrimSpace(r.Password) == "" {
		return fmt.Errorf("password must not be blank")
	}
	return nil
}

// UpdateGitConnectionRequest patches a connection. The API updates only the
// supplied fields, so every field is a pointer and an empty document is
// rejected before transport. Sending a key-generator type, username or
// password replaces the stored authentication material.
type UpdateGitConnectionRequest struct {
	Name             *string `json:"name,omitempty"`
	RepositoryURL    *string `json:"repositoryUrl,omitempty"`
	BranchName       *string `json:"branchName,omitempty"`
	ConnectionType   *string `json:"connectionType,omitempty"`
	KeyGeneratorType *string `json:"keyGeneratorType,omitempty"`
	Username         *string `json:"username,omitempty"`
	Password         *string `json:"password,omitempty"`
}

// Validate rejects an empty or malformed update document before transport.
func (r UpdateGitConnectionRequest) Validate() error {
	if r.Name == nil && r.RepositoryURL == nil && r.BranchName == nil &&
		r.ConnectionType == nil && r.KeyGeneratorType == nil &&
		r.Username == nil && r.Password == nil {
		return fmt.Errorf("git-connection update is empty: set at least one of name, repositoryUrl, branchName, connectionType, keyGeneratorType, username, or password")
	}
	if r.Name != nil {
		if err := validateGitConnectionName(*r.Name); err != nil {
			return err
		}
	}
	if r.RepositoryURL != nil && strings.TrimSpace(*r.RepositoryURL) == "" {
		return fmt.Errorf("repository URL must not be blank")
	}
	if r.BranchName != nil {
		if err := validateGitConnectionBranch(*r.BranchName); err != nil {
			return err
		}
	}
	if r.ConnectionType != nil {
		if err := validateGitConnectionType(*r.ConnectionType); err != nil {
			return err
		}
	}
	if r.KeyGeneratorType != nil && !slices.Contains(gitKeyGeneratorTypes, *r.KeyGeneratorType) {
		return fmt.Errorf("unknown key generator type %q: use %s", *r.KeyGeneratorType, strings.Join(gitKeyGeneratorTypes, ", "))
	}
	if r.Username != nil && strings.TrimSpace(*r.Username) == "" {
		return fmt.Errorf("username must not be blank")
	}
	if r.Password != nil && strings.TrimSpace(*r.Password) == "" {
		return fmt.Errorf("password must not be blank")
	}
	return nil
}

// CloneGitConnectionRequest optionally selects the branch to clone. Empty
// selects the connection's configured branch and sends no body.
type CloneGitConnectionRequest struct {
	BranchName string `json:"branchName,omitempty"`
}

// Validate rejects a branch name the API would reject before transport.
func (r CloneGitConnectionRequest) Validate() error {
	if r.BranchName == "" {
		return nil
	}
	return validateGitConnectionBranch(r.BranchName)
}

// body returns the request body, or nil when the connection's configured
// branch applies.
func (r CloneGitConnectionRequest) body() any {
	if strings.TrimSpace(r.BranchName) == "" {
		return nil
	}
	return r
}

// PushGitConnectionRequest commits every team project and pushes to the
// configured branch. Personal projects are ignored.
type PushGitConnectionRequest struct {
	CommitMessage string `json:"commitMessage"`
	Force         bool   `json:"force,omitempty"`
}

// Validate rejects a push document the API would reject before transport.
func (r PushGitConnectionRequest) Validate() error {
	if strings.TrimSpace(r.CommitMessage) == "" {
		return fmt.Errorf("commit message is required")
	}
	if utf8.RuneCountInString(r.CommitMessage) > maxGitPushCommitLength {
		return fmt.Errorf("commit message must contain at most %d characters", maxGitPushCommitLength)
	}
	return nil
}

// GitConnectionProjects is the project-link response: the team projects added
// to one connection.
type GitConnectionProjects struct {
	ProjectIDs []string `json:"projectIds"`
}

// GitConnectionProjectLink is the acknowledgement of adding one project to a
// connection.
type GitConnectionProjectLink struct {
	ProjectID       string `json:"projectId"`
	GitConnectionID string `json:"gitConnectionId"`
}

// GitPushCounts counts what one push exported, committed and pushed.
type GitPushCounts struct {
	Workflows   int `json:"workflows"`
	Folders     int `json:"folders"`
	Credentials int `json:"credentials"`
	DataTables  int `json:"dataTables"`
	Variables   int `json:"variables"`
	Tags        int `json:"tags"`
}

// PushGitConnectionResult is the push acknowledgement: what moved and the
// resulting commit.
type PushGitConnectionResult struct {
	ConnectionID string        `json:"connectionId"`
	Counts       GitPushCounts `json:"counts"`
	CommitSHA    string        `json:"commitSha"`
}

// GitPullProjectCounts counts project rows created, updated, skipped or
// deleted by one pull import.
type GitPullProjectCounts struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Deleted int `json:"deleted"`
}

// GitPullFolderCounts counts folders created, skipped or removed by one pull.
type GitPullFolderCounts struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Removed int `json:"removed"`
}

// GitPullPublishingCounts counts per-workflow publishing outcomes of one pull.
type GitPullPublishingCounts struct {
	Published   int `json:"published"`
	Unpublished int `json:"unpublished"`
	Unchanged   int `json:"unchanged"`
	Blocked     int `json:"blocked"`
	Failed      int `json:"failed"`
}

// GitPullWorkflowCounts counts workflows created, updated, skipped, archived
// or deleted by one pull, with their publishing outcomes.
type GitPullWorkflowCounts struct {
	Created    int                     `json:"created"`
	Updated    int                     `json:"updated"`
	Skipped    int                     `json:"skipped"`
	Archived   int                     `json:"archived"`
	Deleted    int                     `json:"deleted"`
	Publishing GitPullPublishingCounts `json:"publishing"`
}

// GitPullCredentialCounts counts credentials matched or stubbed by one pull.
type GitPullCredentialCounts struct {
	Matched int `json:"matched"`
	Stubbed int `json:"stubbed"`
}

// GitPullDataTableCounts counts data tables matched or created by one pull.
type GitPullDataTableCounts struct {
	Matched int `json:"matched"`
	Created int `json:"created"`
}

// GitPullVariableCounts counts variables by outcome of one pull.
type GitPullVariableCounts struct {
	Matched int `json:"matched"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Stubbed int `json:"stubbed"`
	Missing int `json:"missing"`
}

// GitPullTagCounts counts tags by outcome of one pull.
type GitPullTagCounts struct {
	Matched    int `json:"matched"`
	Created    int `json:"created"`
	Renamed    int `json:"renamed"`
	Reconciled int `json:"reconciled"`
	Skipped    int `json:"skipped"`
}

// GitPullCounts counts everything one pull import wrote.
type GitPullCounts struct {
	Projects    GitPullProjectCounts    `json:"projects"`
	Folders     GitPullFolderCounts     `json:"folders"`
	Workflows   GitPullWorkflowCounts   `json:"workflows"`
	Credentials GitPullCredentialCounts `json:"credentials"`
	DataTables  GitPullDataTableCounts  `json:"dataTables"`
	Variables   GitPullVariableCounts   `json:"variables"`
	Tags        GitPullTagCounts        `json:"tags"`
}

// PullGitConnectionResult is the pull acknowledgement: what the remote
// overwrote and the commit it now matches.
type PullGitConnectionResult struct {
	ConnectionID string        `json:"connectionId"`
	Counts       GitPullCounts `json:"counts"`
	CommitSHA    string        `json:"commitSha"`
}

// ListGitConnections returns one cursor-paginated page of Git connections.
// The instance holds at most one, so a page usually carries zero or one row.
func (c *Client) ListGitConnections(ctx context.Context, opts ListOptions) (Page[GitConnection], error) {
	if err := opts.Validate(); err != nil {
		return Page[GitConnection]{}, err
	}
	if opts.Limit > 250 {
		return Page[GitConnection]{}, fmt.Errorf("limit must not exceed 250, got %d", opts.Limit)
	}
	var page Page[GitConnection]
	if _, err := c.Do(ctx, Request{Path: GitConnectionsPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[GitConnection]{}, err
	}
	return page, nil
}

// CreateGitConnection creates the instance's Git connection. Only one can
// exist; a second answers 409. Secret error details are redacted in case a
// server or proxy echoes the submitted username or password.
func (c *Client) CreateGitConnection(ctx context.Context, request CreateGitConnectionRequest) (*GitConnection, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var created GitConnection
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   GitConnectionsPath,
		Body:   request,
	}, &created); err != nil {
		return nil, redactGitConnectionAPIError(err)
	}
	return &created, nil
}

// GetGitConnection returns one Git connection. id is escaped as one path
// segment.
func (c *Client) GetGitConnection(ctx context.Context, id string) (*GitConnection, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	var connection GitConnection
	if _, err := c.Do(ctx, Request{Path: PathJoin("git-connections", id)}, &connection); err != nil {
		return nil, err
	}
	return &connection, nil
}

// UpdateGitConnection updates only the supplied fields of one connection.
// Secret error details are redacted like on create.
func (c *Client) UpdateGitConnection(ctx context.Context, id string, request UpdateGitConnectionRequest) (*GitConnection, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var updated GitConnection
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("git-connections", id),
		Body:   request,
	}, &updated); err != nil {
		return nil, redactGitConnectionAPIError(err)
	}
	return &updated, nil
}

// DeleteGitConnection removes one connection and its local checkout.
func (c *Client) DeleteGitConnection(ctx context.Context, id string) error {
	if err := validateGitConnectionID(id); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("git-connections", id)}, nil)
	return err
}

// CloneGitConnection clones the repository into local storage. It is safe to
// call repeatedly; an empty branch clones the configured branch.
func (c *Client) CloneGitConnection(ctx context.Context, id string, request CloneGitConnectionRequest) (*GitConnection, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var cloned GitConnection
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("git-connections", id, "clone"),
		Body:   request.body(),
	}, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

// DisconnectGitConnection removes the local clone. The connection and its
// authentication material are retained.
func (c *Client) DisconnectGitConnection(ctx context.Context, id string) (*GitConnection, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	var disconnected GitConnection
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("git-connections", id, "disconnect"),
	}, &disconnected); err != nil {
		return nil, err
	}
	return &disconnected, nil
}

// ListGitConnectionProjects returns the team projects added to one connection.
func (c *Client) ListGitConnectionProjects(ctx context.Context, id string) (*GitConnectionProjects, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	var projects GitConnectionProjects
	if _, err := c.Do(ctx, Request{Path: PathJoin("git-connections", id, "projects")}, &projects); err != nil {
		return nil, err
	}
	return &projects, nil
}

// AddProjectToGitConnection adds one team project to a connection. A project
// can belong to only one connection; a second link answers 409.
func (c *Client) AddProjectToGitConnection(ctx context.Context, id, projectID string) (*GitConnectionProjectLink, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	if err := validateGitConnectionProjectID(projectID); err != nil {
		return nil, err
	}
	var link GitConnectionProjectLink
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("git-connections", id, "projects", projectID),
	}, &link); err != nil {
		return nil, err
	}
	return &link, nil
}

// RemoveProjectFromGitConnection unlinks one project from a connection. The
// project itself is untouched.
func (c *Client) RemoveProjectFromGitConnection(ctx context.Context, id, projectID string) error {
	if err := validateGitConnectionID(id); err != nil {
		return err
	}
	if err := validateGitConnectionProjectID(projectID); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("git-connections", id, "projects", projectID),
	}, nil)
	return err
}

// PushGitConnectionProjects exports all team projects, commits them and pushes
// to the configured branch. The repository must be cloned first.
func (c *Client) PushGitConnectionProjects(ctx context.Context, id string, request PushGitConnectionRequest) (*PushGitConnectionResult, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result PushGitConnectionResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("git-connections", id, "push"),
		Body:   request,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PullGitConnectionProjects resets the local clone to the configured branch
// tip and imports projects into the instance, overwriting to match. The
// repository must be cloned first.
func (c *Client) PullGitConnectionProjects(ctx context.Context, id string) (*PullGitConnectionResult, error) {
	if err := validateGitConnectionID(id); err != nil {
		return nil, err
	}
	var result PullGitConnectionResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("git-connections", id, "pull"),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateGitConnectionID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("git-connection ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("git-connection ID must not start or end with whitespace")
	}
	return nil
}

func validateGitConnectionProjectID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("project ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("project ID must not start or end with whitespace")
	}
	return nil
}

func validateGitConnectionName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("git-connection name is required")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("git-connection name must not start or end with whitespace")
	}
	if utf8.RuneCountInString(name) > maxGitConnectionNameLength {
		return fmt.Errorf("git-connection name must contain at most %d characters", maxGitConnectionNameLength)
	}
	return nil
}

func validateGitConnectionBranch(branch string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch name is required and must not be blank")
	}
	if strings.TrimSpace(branch) != branch {
		return fmt.Errorf("branch name must not start or end with whitespace")
	}
	if utf8.RuneCountInString(branch) > maxGitConnectionBranchLength {
		return fmt.Errorf("branch name must contain at most %d characters", maxGitConnectionBranchLength)
	}
	return nil
}

func validateGitConnectionType(connectionType string) error {
	if !slices.Contains(gitConnectionTypes, connectionType) {
		return fmt.Errorf("unknown connection type %q: use %s", connectionType, strings.Join(gitConnectionTypes, ", "))
	}
	return nil
}

// redactGitConnectionAPIError prevents server-controlled error text from
// echoing a submitted username or password while preserving HTTP status
// classification.
func redactGitConnectionAPIError(err error) error {
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "git-connection request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
