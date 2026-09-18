package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// Source control endpoints. The instance must have the Source Control feature
// licensed and connected to a Git repository; otherwise these answer 403, 404
// or 503.
const (
	SourceControlStatusPath = "/source-control/status"
	SourceControlPushPath   = "/source-control/push"
	SourceControlPullPath   = "/source-control/pull"
)

// SourceControlDirection selects which pending-change preview to read.
type SourceControlDirection string

const (
	SourceControlDirectionPush SourceControlDirection = "push"
	SourceControlDirectionPull SourceControlDirection = "pull"
)

// SourceControlledFileType is the kind of tracked object a file entry holds.
type SourceControlledFileType string

const (
	SourceControlledFileCredential SourceControlledFileType = "credential"
	SourceControlledFileWorkflow   SourceControlledFileType = "workflow"
	SourceControlledFileTags       SourceControlledFileType = "tags"
	SourceControlledFileVariables  SourceControlledFileType = "variables"
	SourceControlledFileFile       SourceControlledFileType = "file"
	SourceControlledFileFolders    SourceControlledFileType = "folders"
	SourceControlledFileProject    SourceControlledFileType = "project"
	SourceControlledFileDataTable  SourceControlledFileType = "datatable"
)

var sourceControlledFileTypes = []string{
	string(SourceControlledFileCredential),
	string(SourceControlledFileWorkflow),
	string(SourceControlledFileTags),
	string(SourceControlledFileVariables),
	string(SourceControlledFileFile),
	string(SourceControlledFileFolders),
	string(SourceControlledFileProject),
	string(SourceControlledFileDataTable),
}

// SourceControlAutoPublish controls workflow publishing after a pull import.
type SourceControlAutoPublish string

const (
	SourceControlAutoPublishNone      SourceControlAutoPublish = "none"
	SourceControlAutoPublishAll       SourceControlAutoPublish = "all"
	SourceControlAutoPublishPublished SourceControlAutoPublish = "published"
)

var sourceControlAutoPublishes = []string{
	string(SourceControlAutoPublishNone),
	string(SourceControlAutoPublishAll),
	string(SourceControlAutoPublishPublished),
}

// SourceControlOwner names the project that owns a tracked file.
type SourceControlOwner struct {
	Type        string `json:"type"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
}

// SourceControlledFile is one pending or moved file. Publishing-error details
// and content-import-policy results stay raw: they are open-ended per-file
// diagnostics the CLI reports but never interprets.
type SourceControlledFile struct {
	File                   string                   `json:"file"`
	ID                     string                   `json:"id"`
	Name                   string                   `json:"name"`
	Type                   SourceControlledFileType `json:"type"`
	Status                 string                   `json:"status"`
	Location               string                   `json:"location"`
	Conflict               bool                     `json:"conflict"`
	UpdatedAt              string                   `json:"updatedAt"`
	Pushed                 *bool                    `json:"pushed,omitempty"`
	IsLocalPublished       *bool                    `json:"isLocalPublished,omitempty"`
	IsRemoteArchived       *bool                    `json:"isRemoteArchived,omitempty"`
	ParentFolderID         *string                  `json:"parentFolderId"`
	FolderPath             []string                 `json:"folderPath,omitempty"`
	RemoteFolderPath       []string                 `json:"remoteFolderPath,omitempty"`
	Owner                  *SourceControlOwner      `json:"owner,omitempty"`
	PublishingError        *string                  `json:"publishingError,omitempty"`
	PublishingErrorDetails json.RawMessage          `json:"publishingErrorDetails,omitempty"`
	ContentImportPolicy    json.RawMessage          `json:"contentImportPolicy,omitempty"`
}

// SourceControlStatus is the preview response: the pending changes in one
// direction.
type SourceControlStatus struct {
	Data []SourceControlledFile `json:"data"`
}

// SourceControlFileSelector addresses one tracked file in a push.
type SourceControlFileSelector struct {
	ID   string                   `json:"id"`
	Type SourceControlledFileType `json:"type"`
}

// PushSourceControlRequest commits and pushes the selected files. Force pushes
// despite unresolved conflicts.
type PushSourceControlRequest struct {
	CommitMessage string                      `json:"commitMessage"`
	FileNames     []SourceControlFileSelector `json:"fileNames"`
	Force         bool                        `json:"force,omitempty"`
}

// PullSourceControlRequest fetches the remote into the instance. Force discards
// local changes to complete the pull. AutoPublish defaults to none when empty,
// which keeps every workflow in its local published state.
type PullSourceControlRequest struct {
	Force       bool                     `json:"force,omitempty"`
	AutoPublish SourceControlAutoPublish `json:"autoPublish,omitempty"`
}

// PushSourceControlResult is the push response: the files that were pushed.
type PushSourceControlResult struct {
	Data []SourceControlledFile `json:"data"`
}

// maxSourceControlCommitMessage is the commit-message limit the schema declares.
const maxSourceControlCommitMessage = 1000

// GetSourceControlStatus previews the pending changes in one direction.
// Direction is required: push previews what a push would send, pull previews
// what a pull would bring in.
func (c *Client) GetSourceControlStatus(ctx context.Context, direction SourceControlDirection) (*SourceControlStatus, error) {
	if err := validateSourceControlDirection(direction); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("direction", string(direction))
	var status SourceControlStatus
	if _, err := c.Do(ctx, Request{Path: SourceControlStatusPath, Query: query}, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// PushSourceControl commits and pushes the selected files to the connected Git
// repository. Each entry is resolved against a fresh preview server-side. A
// push that includes unresolved conflicts is rejected with 409 unless force is
// set.
func (c *Client) PushSourceControl(ctx context.Context, request PushSourceControlRequest) (*PushSourceControlResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result PushSourceControlResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   SourceControlPushPath,
		Body:   request,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PullSourceControl fetches the remote repository into the instance. It
// rewrites local instance content from the remote, so callers confirm first.
// A pull blocked by uncommitted local changes or merge conflicts is rejected
// with 409 unless force is set, which discards those local changes.
func (c *Client) PullSourceControl(ctx context.Context, request PullSourceControlRequest) ([]SourceControlledFile, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var files []SourceControlledFile
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   SourceControlPullPath,
		Body:   request.normalized(),
	}, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// Validate rejects a push document the API would reject before transport.
func (r PushSourceControlRequest) Validate() error {
	if strings.TrimSpace(r.CommitMessage) == "" {
		return fmt.Errorf("commit message is required")
	}
	if utf8.RuneCountInString(r.CommitMessage) > maxSourceControlCommitMessage {
		return fmt.Errorf("commit message must contain at most %d characters", maxSourceControlCommitMessage)
	}
	if len(r.FileNames) == 0 {
		return fmt.Errorf("at least one file is required: select files from 'n8n source-control status --direction push'")
	}
	for i, file := range r.FileNames {
		if err := validateSourceControlFileSelector(file); err != nil {
			return fmt.Errorf("fileNames[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate rejects a pull document the API would reject before transport.
func (r PullSourceControlRequest) Validate() error {
	if r.AutoPublish != "" && !slices.Contains(sourceControlAutoPublishes, string(r.AutoPublish)) {
		return fmt.Errorf("unknown auto-publish %q: use %s", r.AutoPublish, strings.Join(sourceControlAutoPublishes, ", "))
	}
	return nil
}

// normalized applies the documented default: an omitted auto-publish keeps
// every workflow in its local published state.
func (r PullSourceControlRequest) normalized() PullSourceControlRequest {
	if r.AutoPublish == "" {
		r.AutoPublish = SourceControlAutoPublishNone
	}
	return r
}

func validateSourceControlDirection(direction SourceControlDirection) error {
	switch direction {
	case SourceControlDirectionPush, SourceControlDirectionPull:
		return nil
	default:
		return fmt.Errorf("unknown source-control direction %q: use %q or %q", direction, SourceControlDirectionPush, SourceControlDirectionPull)
	}
}

func validateSourceControlFileSelector(file SourceControlFileSelector) error {
	if strings.TrimSpace(file.ID) == "" {
		return fmt.Errorf("id is required and must not be blank")
	}
	if strings.TrimSpace(string(file.Type)) == "" {
		return fmt.Errorf("type is required: use %s", strings.Join(sourceControlledFileTypes, ", "))
	}
	if !slices.Contains(sourceControlledFileTypes, string(file.Type)) {
		return fmt.Errorf("unknown type %q: use %s", file.Type, strings.Join(sourceControlledFileTypes, ", "))
	}
	return nil
}
