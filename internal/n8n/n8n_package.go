// n8n package endpoints (beta).
//
// The whole group is beta: breaking changes may still occur without a major
// version bump. Export streams a gzipped tar archive (.n8np) down; import
// streams one up as multipart form data. Both directions stream: the archive
// is never buffered whole in memory.

package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"time"
)

// n8n package endpoints.
const (
	N8nPackageExportPath = "/n8n-packages/export"
	N8nPackageImportPath = "/n8n-packages/import"
)

// ExportCountsHeader carries the JSON-serialized per-entity counts of what
// actually ended up in an exported package, e.g.
// {"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0}.
const ExportCountsHeader = "X-N8n-Export-Counts"

// DefaultImportPackageFilename is the multipart filename sent when the caller
// supplies no better name for the archive.
const DefaultImportPackageFilename = "package.n8np"

// Missing-workflow-dependency policies for an export.
const (
	ExportMissingFail             = "fail"
	ExportMissingReferenceOnly    = "reference-only"
	ExportMissingIncludeInPackage = "include-in-package"
)

var exportMissingPolicies = []string{
	ExportMissingFail,
	ExportMissingReferenceOnly,
	ExportMissingIncludeInPackage,
}

// Workflow-version policies for an export.
const (
	ExportVersionPublishedStrict   = "published-strict"
	ExportVersionPreferPublished   = "prefer-published"
	ExportVersionIgnoreUnpublished = "ignore-unpublished"
	ExportVersionLatest            = "latest"
)

var exportVersionPolicies = []string{
	ExportVersionPublishedStrict,
	ExportVersionPreferPublished,
	ExportVersionIgnoreUnpublished,
	ExportVersionLatest,
}

// Credential-export policies for an export. Literal credential values never
// travel either way; only n8n expressions may.
const (
	ExportCredentialExpressionValuesOnly = "expression-values-only"
	ExportCredentialNoValues             = "no-values"
)

var exportCredentialPolicies = []string{
	ExportCredentialExpressionValuesOnly,
	ExportCredentialNoValues,
}

// maxExportIDs is the per-list limit the schema declares for workflow and
// folder selections.
const maxExportIDs = 300

// ExportPackageRequest selects what to export. Provide workflow and/or folder
// IDs to export loose workflows and folders, or project IDs to export whole
// projects, but not both groups in the same request. At least one ID must be
// supplied. Pointer booleans distinguish an omitted value (the server default
// applies) from an explicit false; use [Bool] when setting them directly.
type ExportPackageRequest struct {
	WorkflowIDs             []string `json:"workflowIds,omitempty"`
	FolderIDs               []string `json:"folderIds,omitempty"`
	ProjectIDs              []string `json:"projectIds,omitempty"`
	IncludeVariableValues   *bool    `json:"includeVariableValues,omitempty"`
	IncludeTags             *bool    `json:"includeTags,omitempty"`
	MissingDependencyPolicy string   `json:"missingWorkflowDependencyPolicy,omitempty"`
	WorkflowVersionPolicy   string   `json:"workflowVersionPolicy,omitempty"`
	CredentialExportPolicy  string   `json:"credentialExportPolicy,omitempty"`
	IncludeArchived         *bool    `json:"includeArchivedWorkflows,omitempty"`
}

// Validate rejects an export document the API would reject before transport.
func (r ExportPackageRequest) Validate() error {
	if len(r.WorkflowIDs) == 0 && len(r.FolderIDs) == 0 && len(r.ProjectIDs) == 0 {
		return fmt.Errorf("at least one workflow, folder, or project ID is required")
	}
	if len(r.ProjectIDs) > 0 && (len(r.WorkflowIDs) > 0 || len(r.FolderIDs) > 0) {
		return fmt.Errorf("projectIds cannot be combined with workflowIds or folderIds: export loose workflows and folders, or whole projects, not both")
	}
	for i, id := range r.WorkflowIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("workflowIds[%d] is required and must not be blank", i)
		}
		if id != strings.TrimSpace(id) {
			return fmt.Errorf("workflowIds[%d] must not start or end with whitespace", i)
		}
	}
	for i, id := range r.FolderIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("folderIds[%d] is required and must not be blank", i)
		}
		if id != strings.TrimSpace(id) {
			return fmt.Errorf("folderIds[%d] must not start or end with whitespace", i)
		}
	}
	for i, id := range r.ProjectIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("projectIds[%d] is required and must not be blank", i)
		}
		if id != strings.TrimSpace(id) {
			return fmt.Errorf("projectIds[%d] must not start or end with whitespace", i)
		}
	}
	if len(r.WorkflowIDs) > maxExportIDs {
		return fmt.Errorf("workflowIds holds %d IDs, want at most %d", len(r.WorkflowIDs), maxExportIDs)
	}
	if len(r.FolderIDs) > maxExportIDs {
		return fmt.Errorf("folderIds holds %d IDs, want at most %d", len(r.FolderIDs), maxExportIDs)
	}
	if r.MissingDependencyPolicy != "" && !slices.Contains(exportMissingPolicies, r.MissingDependencyPolicy) {
		return fmt.Errorf("unknown missing-workflow-dependency policy %q: use %s", r.MissingDependencyPolicy, strings.Join(exportMissingPolicies, ", "))
	}
	if r.WorkflowVersionPolicy != "" && !slices.Contains(exportVersionPolicies, r.WorkflowVersionPolicy) {
		return fmt.Errorf("unknown workflow-version policy %q: use %s", r.WorkflowVersionPolicy, strings.Join(exportVersionPolicies, ", "))
	}
	if r.CredentialExportPolicy != "" && !slices.Contains(exportCredentialPolicies, r.CredentialExportPolicy) {
		return fmt.Errorf("unknown credential-export policy %q: use %s", r.CredentialExportPolicy, strings.Join(exportCredentialPolicies, ", "))
	}
	return nil
}

// ExportPackageResult describes the archive streamed to the caller's writer.
type ExportPackageResult struct {
	// Counts holds the per-entity counts from the X-N8n-Export-Counts response
	// header, e.g. workflows, folders, credentials, dataTables, variables.
	Counts map[string]int `json:"counts"`
	// Filename is the attachment name from Content-Disposition, when sent.
	Filename string `json:"filename,omitempty"`
	// Bytes is how many archive bytes were written.
	Bytes int64 `json:"bytes"`
}

// ExportPackage posts the export selection and streams the resulting gzipped
// tar archive to w. The archive is never buffered whole in memory; counts and
// filename come from the response headers. Exporting reads instance content
// and writes nothing server-side.
func (c *Client) ExportPackage(ctx context.Context, request ExportPackageRequest, w io.Writer) (*ExportPackageResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if w == nil {
		return nil, fmt.Errorf("n8n: export needs a destination to write the archive to")
	}
	if ctx == nil {
		return nil, errors.New("n8n: nil context")
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("n8n: encode export request: %w", err)
	}

	endpoint, err := c.resolve(N8nPackageExportPath, nil)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("n8n: build request: %w", err)
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Accept", "application/gzip")
	req.Header.Set("User-Agent", c.userAgent)
	if c.auth != nil {
		c.auth.Apply(req)
	}

	start := time.Now()
	c.logger.DebugContext(ctx, "n8n request",
		"method", http.MethodPost,
		"url", endpoint.Redacted(),
		"auth", c.AuthType(),
	)
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.DebugContext(ctx, "n8n request failed",
			"method", http.MethodPost,
			"url", endpoint.Redacted(),
			"duration", time.Since(start),
			"error", err,
		)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("n8n: POST %s: %w", N8nPackageExportPath, ctxErr)
		}
		return nil, fmt.Errorf("n8n: POST %s: %w", N8nPackageExportPath, err)
	}
	defer drainAndClose(resp.Body)

	reqID := requestID(resp.Header)
	c.logger.DebugContext(ctx, "n8n response",
		"method", http.MethodPost,
		"url", endpoint.Redacted(),
		"status", resp.StatusCode,
		"requestID", reqID,
		"duration", time.Since(start),
	)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, truncated, err := readLimited(resp.Body, maxErrorBody)
		if err != nil {
			return nil, fmt.Errorf("n8n: POST %s: read error response: %w", N8nPackageExportPath, err)
		}
		return nil, newAPIError(resp, http.MethodPost, N8nPackageExportPath, body, truncated)
	}

	counts, err := decodeExportCounts(resp.Header.Get(ExportCountsHeader))
	if err != nil {
		return nil, err
	}
	written, err := io.Copy(w, resp.Body)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("n8n: POST %s: %w", N8nPackageExportPath, ctxErr)
		}
		return nil, fmt.Errorf("n8n: POST %s: stream export archive: %w", N8nPackageExportPath, err)
	}
	return &ExportPackageResult{
		Counts:   counts,
		Filename: exportFilename(resp.Header.Get("Content-Disposition")),
		Bytes:    written,
	}, nil
}

// decodeExportCounts parses the X-N8n-Export-Counts header. An absent header
// yields no counts; a present but malformed one is an error, not silence.
func decodeExportCounts(header string) (map[string]int, error) {
	if strings.TrimSpace(header) == "" {
		return nil, nil
	}
	var counts map[string]int
	if err := json.Unmarshal([]byte(header), &counts); err != nil {
		return nil, fmt.Errorf("n8n: decode %s: %w", ExportCountsHeader, err)
	}
	return counts, nil
}

// exportFilename extracts the attachment filename from Content-Disposition.
// It tolerates an absent or malformed header by reporting no filename.
func exportFilename(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// Import conflict and mode values. Every optional mode takes its default when
// omitted; the CLI sends a field only when its flag is set, except
// WorkflowIDPolicy, which is always sent because the prose default (new) and
// the schema default (source) conflict.
const (
	ImportCredentialMatchIDOnly      = "id-only"
	ImportCredentialMatchNameAndType = "name-and-type"
	ImportCredentialMatchTypeOnly    = "type-only"

	ImportCredentialMissingMustPreexist = "must-preexist"
	ImportCredentialMissingCreateStub   = "create-stub"

	ImportWorkflowConflictNewVersion = "new-version"
	ImportWorkflowConflictFail       = "fail"
	ImportWorkflowConflictSkip       = "skip"

	ImportWorkflowIDNew    = "new"
	ImportWorkflowIDSource = "source"

	ImportMissingNodeFail         = "fail"
	ImportMissingNodeImportAnyway = "import-anyway"

	ImportPublishPreserve = "preserve-published-state"
	ImportPublishMatch    = "match-source"
	ImportPublishAll      = "publish-all"
	ImportPublishNone     = "unpublish-all"

	ImportProjectMerge     = "merge"
	ImportProjectFail      = "fail"
	ImportProjectOverwrite = "overwrite"

	ImportFolderMerge     = "merge"
	ImportFolderFail      = "fail"
	ImportFolderOverwrite = "overwrite"

	ImportDeletionArchive    = "archive"
	ImportDeletionHardDelete = "hard-delete"

	ImportDataTableMatchByID = "by-id"

	ImportDataTableMissingCreate       = "create"
	ImportDataTableMissingMustPreexist = "must-preexist"
	ImportDataTableMissingDoNothing    = "do-nothing"

	ImportDataTableSchemaKeepExisting = "keep-existing"
	ImportDataTableSchemaFail         = "fail"

	ImportVariableMissingDoNothing     = "do-nothing"
	ImportVariableMissingMustPreexist  = "must-preexist"
	ImportVariableMissingCreateStub    = "create-stub"
	ImportVariableMissingCreateWithVal = "create-with-value"

	ImportVariableConflictKeep      = "keep-existing"
	ImportVariableConflictOverwrite = "overwrite"
	ImportVariableConflictFail      = "fail"

	ImportVariableParentProject = "project"
	ImportVariableParentGlobal  = "global"

	ImportTagMissingCreate    = "create"
	ImportTagMissingDoNothing = "do-nothing"

	ImportTagConflictSkip   = "skip"
	ImportTagConflictFail   = "fail"
	ImportTagConflictRename = "rename"
)

var (
	importCredentialMatchModes   = []string{ImportCredentialMatchIDOnly, ImportCredentialMatchNameAndType, ImportCredentialMatchTypeOnly}
	importCredentialMissingModes = []string{ImportCredentialMissingMustPreexist, ImportCredentialMissingCreateStub}
	importWorkflowConflicts      = []string{ImportWorkflowConflictNewVersion, ImportWorkflowConflictFail, ImportWorkflowConflictSkip}
	importWorkflowIDPolicies     = []string{ImportWorkflowIDNew, ImportWorkflowIDSource}
	importMissingNodeModes       = []string{ImportMissingNodeFail, ImportMissingNodeImportAnyway}
	importPublishPolicies        = []string{ImportPublishPreserve, ImportPublishMatch, ImportPublishAll, ImportPublishNone}
	importProjectConflicts       = []string{ImportProjectMerge, ImportProjectFail, ImportProjectOverwrite}
	importFolderConflicts        = []string{ImportFolderMerge, ImportFolderFail, ImportFolderOverwrite}
	importDeletionPolicies       = []string{ImportDeletionArchive, ImportDeletionHardDelete}
	importDataTableMatchModes    = []string{ImportDataTableMatchByID}
	importDataTableMissingModes  = []string{ImportDataTableMissingCreate, ImportDataTableMissingMustPreexist, ImportDataTableMissingDoNothing}
	importDataTableSchemaModes   = []string{ImportDataTableSchemaKeepExisting, ImportDataTableSchemaFail}
	importVariableMissingModes   = []string{ImportVariableMissingDoNothing, ImportVariableMissingMustPreexist, ImportVariableMissingCreateStub, ImportVariableMissingCreateWithVal}
	importVariableConflictModes  = []string{ImportVariableConflictKeep, ImportVariableConflictOverwrite, ImportVariableConflictFail}
	importVariableParentModes    = []string{ImportVariableParentProject, ImportVariableParentGlobal}
	importTagMissingModes        = []string{ImportTagMissingCreate, ImportTagMissingDoNothing}
	importTagConflictModes       = []string{ImportTagConflictSkip, ImportTagConflictFail, ImportTagConflictRename}
)

// ImportPackageRequest is the routing and conflict policy for one package
// import. Empty optional fields are omitted so the server default applies,
// except WorkflowIDPolicy, which is always sent because the prose default
// (new) and the schema default (source) conflict. Bindings, when set, must be
// a JSON object; only credentials bindings are supported today.
type ImportPackageRequest struct {
	ProjectID                string `json:"-"`
	FolderID                 string `json:"-"`
	CredentialMatchingMode   string `json:"-"`
	CredentialMissingMode    string `json:"-"`
	Bindings                 string `json:"-"`
	WorkflowConflictPolicy   string `json:"-"`
	WorkflowIDPolicy         string `json:"-"`
	MissingNodeTypeMode      string `json:"-"`
	WorkflowPublishingPolicy string `json:"-"`
	ProjectConflictPolicy    string `json:"-"`
	FolderConflictPolicy     string `json:"-"`
	OverwriteDeletionPolicy  string `json:"-"`
	DataTableMatchingMode    string `json:"-"`
	DataTableMissingMode     string `json:"-"`
	DataTableSchemaConflict  string `json:"-"`
	VariableMissingMode      string `json:"-"`
	VariableConflictPolicy   string `json:"-"`
	VariableParentPolicy     string `json:"-"`
	TagMissingMode           string `json:"-"`
	TagConflictPolicy        string `json:"-"`
}

// Validate rejects an import policy the API would reject before transport.
// WorkflowConflictPolicy is required; WorkflowIDPolicy must be explicit.
func (r ImportPackageRequest) Validate() error {
	if strings.TrimSpace(r.ProjectID) != r.ProjectID {
		return fmt.Errorf("project ID must not start or end with whitespace")
	}
	if strings.TrimSpace(r.FolderID) != r.FolderID {
		return fmt.Errorf("folder ID must not start or end with whitespace")
	}
	if !slices.Contains(importWorkflowConflicts, r.WorkflowConflictPolicy) {
		return fmt.Errorf("unknown workflow-conflict policy %q: use %s", r.WorkflowConflictPolicy, strings.Join(importWorkflowConflicts, ", "))
	}
	if !slices.Contains(importWorkflowIDPolicies, r.WorkflowIDPolicy) {
		return fmt.Errorf("unknown workflow-id policy %q: use %s", r.WorkflowIDPolicy, strings.Join(importWorkflowIDPolicies, ", "))
	}
	checks := []struct {
		value   string
		allowed []string
		name    string
	}{
		{r.CredentialMatchingMode, importCredentialMatchModes, "credential-matching mode"},
		{r.CredentialMissingMode, importCredentialMissingModes, "credential-missing mode"},
		{r.MissingNodeTypeMode, importMissingNodeModes, "missing-node-type mode"},
		{r.WorkflowPublishingPolicy, importPublishPolicies, "workflow-publishing policy"},
		{r.ProjectConflictPolicy, importProjectConflicts, "project-conflict policy"},
		{r.FolderConflictPolicy, importFolderConflicts, "folder-conflict policy"},
		{r.OverwriteDeletionPolicy, importDeletionPolicies, "overwrite-deletion policy"},
		{r.DataTableMatchingMode, importDataTableMatchModes, "data-table-matching mode"},
		{r.DataTableMissingMode, importDataTableMissingModes, "data-table-missing mode"},
		{r.DataTableSchemaConflict, importDataTableSchemaModes, "data-table-schema policy"},
		{r.VariableMissingMode, importVariableMissingModes, "variable-missing mode"},
		{r.VariableConflictPolicy, importVariableConflictModes, "variable-conflict policy"},
		{r.VariableParentPolicy, importVariableParentModes, "variable-parent policy"},
		{r.TagMissingMode, importTagMissingModes, "tag-missing mode"},
		{r.TagConflictPolicy, importTagConflictModes, "tag-conflict policy"},
	}
	for _, check := range checks {
		if check.value != "" && !slices.Contains(check.allowed, check.value) {
			return fmt.Errorf("unknown %s %q: use %s", check.name, check.value, strings.Join(check.allowed, ", "))
		}
	}
	if strings.TrimSpace(r.Bindings) != "" {
		var bindings map[string]any
		if err := json.Unmarshal([]byte(r.Bindings), &bindings); err != nil {
			return fmt.Errorf("bindings must be a JSON object: %w", err)
		}
	}
	return nil
}

// fields renders the policy as multipart form fields. Empty optional fields
// stay omitted so the server default applies; the workflow ID policy is
// always present.
func (r ImportPackageRequest) fields() map[string]string {
	fields := map[string]string{
		"workflowConflictPolicy": r.WorkflowConflictPolicy,
		"workflowIdPolicy":       r.WorkflowIDPolicy,
	}
	set := func(name, value string) {
		if value != "" {
			fields[name] = value
		}
	}
	set("projectId", r.ProjectID)
	set("folderId", r.FolderID)
	set("credentialMatchingMode", r.CredentialMatchingMode)
	set("credentialMissingMode", r.CredentialMissingMode)
	set("bindings", r.Bindings)
	set("missingNodeTypeMode", r.MissingNodeTypeMode)
	set("workflowPublishingPolicy", r.WorkflowPublishingPolicy)
	set("projectConflictPolicy", r.ProjectConflictPolicy)
	set("folderConflictPolicy", r.FolderConflictPolicy)
	set("overwriteDeletionPolicy", r.OverwriteDeletionPolicy)
	set("dataTableMatchingMode", r.DataTableMatchingMode)
	set("dataTableMissingMode", r.DataTableMissingMode)
	set("dataTableSchemaConflictPolicy", r.DataTableSchemaConflict)
	set("variableMissingMode", r.VariableMissingMode)
	set("variableConflictPolicy", r.VariableConflictPolicy)
	set("variableParentPolicy", r.VariableParentPolicy)
	set("tagMissingMode", r.TagMissingMode)
	set("tagConflictPolicy", r.TagConflictPolicy)
	return fields
}

// ImportPackageResult is the import outcome: what was written, removed, and
// how every referenced entity resolved. Names only travel for variables and
// tags; values never travel in the response.
type ImportPackageResult struct {
	Package          ImportedPackage    `json:"package"`
	Workflows        []ImportedWorkflow `json:"workflows"`
	RemovedWorkflows []RemovedWorkflow  `json:"removedWorkflows"`
	RemovedFolders   []RemovedFolder    `json:"removedFolders"`
	Folders          []ImportedFolder   `json:"folders"`
	Projects         []ImportedProject  `json:"projects"`
	Bindings         ImportBindings     `json:"bindings"`
	Credentials      ImportCredentials  `json:"credentials"`
	DataTables       ImportDataTables   `json:"dataTables"`
	Variables        ImportVariables    `json:"variables"`
	Tags             ImportTags         `json:"tags"`
}

// ImportedPackage identifies the package the result came from.
type ImportedPackage struct {
	SourceN8nVersion string `json:"sourceN8nVersion"`
	SourceID         string `json:"sourceId"`
	ExportedAt       string `json:"exportedAt"`
}

// ImportedWorkflow is one package workflow and what happened to it.
type ImportedWorkflow struct {
	SourceWorkflowID string           `json:"sourceWorkflowId"`
	LocalID          string           `json:"localId"`
	Name             string           `json:"name"`
	ProjectID        string           `json:"projectId"`
	ParentFolderID   *string          `json:"parentFolderId"`
	ActiveVersionID  *string          `json:"activeVersionId"`
	IsArchived       bool             `json:"isArchived"`
	Publishing       ImportPublishing `json:"publishing"`
	Status           string           `json:"status"`
}

// ImportPublishing is the outcome of applying the publishing policy to one
// workflow.
type ImportPublishing struct {
	State                string `json:"state"`
	Error                string `json:"error,omitempty"`
	BlockedReason        string `json:"blockedReason,omitempty"`
	SkippedPublishReason string `json:"skippedPublishReason,omitempty"`
}

// RemovedWorkflow is a target workflow removed by folderConflictPolicy
// overwrite. Deletion reports what actually happened: a hard-delete that
// could not drop its row yet stays archived.
type RemovedWorkflow struct {
	WorkflowID     string  `json:"workflowId"`
	Name           string  `json:"name"`
	ProjectID      string  `json:"projectId"`
	ParentFolderID *string `json:"parentFolderId"`
	Deletion       string  `json:"deletion"`
}

// RemovedFolder is a target folder removed by folderConflictPolicy overwrite
// once nothing was left inside it.
type RemovedFolder struct {
	FolderID       string  `json:"folderId"`
	Name           string  `json:"name"`
	ProjectID      string  `json:"projectId"`
	ParentFolderID *string `json:"parentFolderId"`
}

// ImportedFolder is one package folder shell.
type ImportedFolder struct {
	SourceFolderID string  `json:"sourceFolderId"`
	LocalID        string  `json:"localId"`
	Name           string  `json:"name"`
	ParentFolderID *string `json:"parentFolderId"`
	Status         string  `json:"status"`
}

// ImportedProject is one package project shell.
type ImportedProject struct {
	SourceProjectID string `json:"sourceProjectId"`
	LocalID         string `json:"localId"`
	Name            string `json:"name"`
	Status          string `json:"status"`
}

// ImportBindings maps source IDs from the package to target IDs on the
// instance, one map per entity type.
type ImportBindings struct {
	Workflows   map[string]string `json:"workflows"`
	Credentials map[string]string `json:"credentials"`
}

// ImportCredentials groups source credential IDs by whether they matched or
// were created as stubs.
type ImportCredentials struct {
	Matched []string `json:"matched"`
	Stubbed []string `json:"stubbed"`
}

// ImportDataTables counts matched and created data tables.
type ImportDataTables struct {
	Matched int `json:"matched"`
	Created int `json:"created"`
}

// ImportVariables holds the referenced variable names by outcome.
type ImportVariables struct {
	Matched []string `json:"matched"`
	Missing []string `json:"missing"`
	Created []string `json:"created"`
	Stubbed []string `json:"stubbed"`
	Updated []string `json:"updated"`
}

// ImportTags holds the referenced tag names by outcome.
type ImportTags struct {
	Matched    []string `json:"matched"`
	Created    []string `json:"created"`
	Renamed    []string `json:"renamed"`
	Reconciled []string `json:"reconciled"`
	Skipped    []string `json:"skipped"`
}

// ImportBlockingIssue is one reason a 409 or 422 rejected an import. Only the
// discriminator is interpreted; the full document is preserved verbatim so new
// issue kinds survive without a client change.
type ImportBlockingIssue struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps the discriminator and the verbatim document.
func (issue *ImportBlockingIssue) UnmarshalJSON(data []byte) error {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typed); err != nil {
		return err
	}
	issue.Type = typed.Type
	issue.Raw = append([]byte(nil), data...)
	return nil
}

// MarshalJSON re-emits the verbatim document when present.
func (issue ImportBlockingIssue) MarshalJSON() ([]byte, error) {
	if len(issue.Raw) > 0 {
		return issue.Raw, nil
	}
	return json.Marshal(struct {
		Type string `json:"type"`
	}{Type: issue.Type})
}

// ImportBlockedError is a 409 (conflict) or 422 (unprocessable) import
// rejection with its issues preserved. The import wrote nothing when the
// missing-node-type, fail-policy, or validation path rejected it; a 409 from
// an overwrite race may have written content before failing.
type ImportBlockedError struct {
	StatusCode int                   `json:"-"`
	Status     string                `json:"-"`
	Method     string                `json:"-"`
	Path       string                `json:"-"`
	RequestID  string                `json:"-"`
	Message    string                `json:"message"`
	Issues     []ImportBlockingIssue `json:"issues"`
}

func (e *ImportBlockedError) Error() string {
	var b strings.Builder
	if e.Method != "" && e.Path != "" {
		fmt.Fprintf(&b, "%s %s: ", e.Method, e.Path)
	}
	if e.Status != "" {
		b.WriteString(e.Status)
	} else {
		fmt.Fprintf(&b, "HTTP %d", e.StatusCode)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if len(e.Issues) > 0 {
		types := make([]string, 0, len(e.Issues))
		for _, issue := range e.Issues {
			types = append(types, emptyImportIssueType(issue.Type))
		}
		fmt.Fprintf(&b, " (%d issue(s): %s)", len(e.Issues), strings.Join(types, ", "))
	}
	if e.RequestID != "" {
		b.WriteString(" (request id ")
		b.WriteString(e.RequestID)
		b.WriteString(")")
	}
	return b.String()
}

func emptyImportIssueType(t string) string {
	if t == "" {
		return "unknown"
	}
	return t
}

// AsImportBlocked reports whether err is an [*ImportBlockedError].
func AsImportBlocked(err error) (*ImportBlockedError, bool) {
	if err == nil {
		return nil, false
	}
	if blocked, ok := errors.AsType[*ImportBlockedError](err); ok {
		return blocked, true
	}
	return nil, false
}

// IsImportConflict reports a 409: the import was blocked by at least one
// conflict among the issues.
func IsImportConflict(err error) bool {
	blocked, ok := AsImportBlocked(err)
	return ok && blocked.StatusCode == http.StatusConflict
}

// IsImportUnprocessable reports a 422: the import was blocked by
// non-conflict issues only, such as unresolved credentials or variables.
func IsImportUnprocessable(err error) bool {
	blocked, ok := AsImportBlocked(err)
	return ok && blocked.StatusCode == http.StatusUnprocessableEntity
}

// ImportPackage streams a gzip-compressed tar package (.n8np) to the instance
// with its routing and conflict policy. The archive streams through a pipe
// and is never buffered whole in memory. The caller confirms first: imports
// may publish, archive, hard-delete workflows and executions, create
// credential stubs, and overwrite folders, tags, variables, or tables.
func (c *Client) ImportPackage(ctx context.Context, request ImportPackageRequest, packageData io.Reader, filename string) (*ImportPackageResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if packageData == nil {
		return nil, fmt.Errorf("package archive is required: pass the .n8np bytes")
	}
	if strings.TrimSpace(filename) == "" {
		filename = DefaultImportPackageFilename
	}
	if ctx == nil {
		return nil, errors.New("n8n: nil context")
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	importEndpoint, err := c.resolve(N8nPackageImportPath, nil)
	if err != nil {
		return nil, err
	}

	body, contentType := multipartImportBody(request, packageData, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, importEndpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("n8n: build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", contentTypeJSON)
	req.Header.Set("User-Agent", c.userAgent)
	if c.auth != nil {
		c.auth.Apply(req)
	}

	start := time.Now()
	c.logger.DebugContext(ctx, "n8n request",
		"method", http.MethodPost,
		"url", importEndpoint.Redacted(),
		"auth", c.AuthType(),
	)
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.DebugContext(ctx, "n8n request failed",
			"method", http.MethodPost,
			"url", importEndpoint.Redacted(),
			"duration", time.Since(start),
			"error", err,
		)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("n8n: POST %s: %w", N8nPackageImportPath, ctxErr)
		}
		return nil, fmt.Errorf("n8n: POST %s: %w", N8nPackageImportPath, err)
	}
	defer drainAndClose(resp.Body)

	c.logger.DebugContext(ctx, "n8n response",
		"method", http.MethodPost,
		"url", importEndpoint.Redacted(),
		"status", resp.StatusCode,
		"requestID", requestID(resp.Header),
		"duration", time.Since(start),
	)

	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity {
		raw, _, err := readLimited(resp.Body, maxErrorBody)
		if err != nil {
			return nil, fmt.Errorf("n8n: POST %s: read error response: %w", N8nPackageImportPath, err)
		}
		return nil, newImportBlockedError(resp, raw)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, truncated, err := readLimited(resp.Body, maxErrorBody)
		if err != nil {
			return nil, fmt.Errorf("n8n: POST %s: read error response: %w", N8nPackageImportPath, err)
		}
		return nil, newAPIError(resp, http.MethodPost, N8nPackageImportPath, raw, truncated)
	}

	var result ImportPackageResult
	if err := decodeBody(resp, &result); err != nil {
		return nil, fmt.Errorf("n8n: POST %s: %w", N8nPackageImportPath, err)
	}
	return &result, nil
}

// multipartImportBody streams the policy fields and the archive through a
// pipe: the writer side runs in a goroutine, so the HTTP transport reads
// while the archive streams without an intermediate buffer.
func multipartImportBody(request ImportPackageRequest, packageData io.Reader, filename string) (io.Reader, string) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	go func() {
		err := writeMultipartImport(multipartWriter, request, packageData, filename)
		_ = writer.CloseWithError(err)
	}()
	return reader, contentType
}

func writeMultipartImport(multipartWriter *multipart.Writer, request ImportPackageRequest, packageData io.Reader, filename string) error {
	for name, value := range request.fields() {
		if err := multipartWriter.WriteField(name, value); err != nil {
			_ = multipartWriter.Close()
			return fmt.Errorf("write import field %s: %w", name, err)
		}
	}
	part, err := multipartWriter.CreateFormFile("package", filename)
	if err != nil {
		_ = multipartWriter.Close()
		return fmt.Errorf("write import package: %w", err)
	}
	if _, err := io.Copy(part, packageData); err != nil {
		_ = multipartWriter.Close()
		return fmt.Errorf("stream import package: %w", err)
	}
	if err := multipartWriter.Close(); err != nil {
		return fmt.Errorf("finish import package: %w", err)
	}
	return nil
}

// newImportBlockedError decodes a 409 or 422 body, preserving every issue.
// A body that does not decode still yields the status and request ID.
func newImportBlockedError(resp *http.Response, raw []byte) *ImportBlockedError {
	blocked := &ImportBlockedError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Method:     http.MethodPost,
		Path:       N8nPackageImportPath,
		RequestID:  requestID(resp.Header),
	}
	var decoded struct {
		Message string                `json:"message"`
		Issues  []ImportBlockingIssue `json:"issues"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		blocked.Message = truncateText(strings.TrimSpace(string(raw)), 512)
		return blocked
	}
	blocked.Message = decoded.Message
	blocked.Issues = decoded.Issues
	if blocked.Message == "" && len(blocked.Issues) == 0 {
		blocked.Message = truncateText(strings.TrimSpace(string(raw)), 512)
	}
	return blocked
}
