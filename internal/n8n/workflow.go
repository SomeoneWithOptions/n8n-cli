package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// WorkflowsPath is the collection endpoint for workflows.
const WorkflowsPath = "/workflows"

// Writable workflow fields. Everything else the API documents on a workflow is
// read-only, and sending a read-only field is a 400: the instance answers
// `request/body/id is read-only`. A workflow read with [Client.GetWorkflow]
// therefore cannot be sent back unchanged; filter it with
// [WorkflowDocument.ForCreate] or [WorkflowDocument.ForUpdate] first.
var (
	// CreateWorkflowFields are the fields POST /workflows accepts.
	CreateWorkflowFields = []string{
		"name", "nodes", "connections", "settings",
		"nodeGroups", "staticData", "pinData", "projectId", "parentFolderId",
	}
	// UpdateWorkflowFields are the fields PUT /workflows/{id} accepts. The
	// project is not among them: move a workflow with
	// [Client.TransferWorkflow].
	UpdateWorkflowFields = []string{
		"name", "description", "nodes", "connections", "settings",
		"nodeGroups", "staticData", "pinData", "parentFolderId",
	}
	// RequiredWorkflowFields must be present in every write document.
	RequiredWorkflowFields = []string{"name", "nodes", "connections", "settings"}
	// nullableWorkflowFields may be sent as JSON null. Any other field read
	// back as null (description is, on a workflow that has none) is dropped
	// rather than sent, because the schema types it as a string.
	nullableWorkflowFields = []string{"staticData", "pinData"}
)

// WorkflowDocument is a workflow definition as raw JSON, keyed by field name.
//
// Nodes, connections, settings and pinned data are open-ended documents whose
// shape depends on the node types installed on the instance, so they are kept
// as raw JSON instead of being modelled: what the user wrote, or what the API
// returned, is what gets sent back, byte for byte.
type WorkflowDocument map[string]json.RawMessage

// Text returns a string field, or the empty string when it is absent, null or
// not a string.
func (d WorkflowDocument) Text(field string) string {
	raw, ok := d[field]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// Set replaces one field with a JSON-encoded value.
func (d WorkflowDocument) Set(field string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode workflow field %q: %w", field, err)
	}
	d[field] = encoded
	return nil
}

// ForCreate returns the document reduced to the fields POST /workflows accepts,
// plus the names of the fields it dropped, sorted. Callers report the dropped
// names: silently discarding part of a user's document would be worse than the
// 400 it prevents.
func (d WorkflowDocument) ForCreate() (WorkflowDocument, []string) {
	return d.forWrite(CreateWorkflowFields)
}

// ForUpdate returns the document reduced to the fields PUT /workflows/{id}
// accepts, plus the names of the fields it dropped, sorted.
func (d WorkflowDocument) ForUpdate() (WorkflowDocument, []string) {
	return d.forWrite(UpdateWorkflowFields)
}

func (d WorkflowDocument) forWrite(allowed []string) (WorkflowDocument, []string) {
	kept := make(WorkflowDocument, len(allowed))
	var dropped []string
	for field, raw := range d {
		switch {
		case !slices.Contains(allowed, field):
			dropped = append(dropped, field)
		case isJSONNull(raw) && !slices.Contains(nullableWorkflowFields, field):
			dropped = append(dropped, field)
		default:
			kept[field] = raw
		}
	}
	slices.Sort(dropped)
	return kept, dropped
}

// Validate rejects a write document the API would reject.
func (d WorkflowDocument) Validate() error {
	var missing []string
	for _, field := range RequiredWorkflowFields {
		if _, ok := d[field]; !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("workflow document is missing %s: the API requires %s",
			strings.Join(missing, ", "), strings.Join(RequiredWorkflowFields, ", "))
	}
	if name := d.Text("name"); strings.TrimSpace(name) == "" {
		return fmt.Errorf("workflow name is required and must be a non-empty string")
	}
	return nil
}

// isJSONNull reports whether raw is the JSON null literal.
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// Workflow is one workflow as the API returns it.
//
// The scalar fields are typed because commands render and test them; the
// definition itself (nodes, connections, settings, pinned and static data) stays
// raw. Fields the instance sends that this type does not name are kept in Extra
// and marshalled back out, so `n8n workflow get --output json` is a faithful
// copy of the workflow and not a lossy projection of it.
type Workflow struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	Active           bool   `json:"active"`
	ActiveVersionID  string `json:"activeVersionId,omitempty"`
	IsArchived       bool   `json:"isArchived"`
	VersionID        string `json:"versionId,omitempty"`
	VersionCounter   int    `json:"versionCounter,omitempty"`
	SourceWorkflowID string `json:"sourceWorkflowId,omitempty"`
	TriggerCount     int    `json:"triggerCount,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
	Tags             []Tag  `json:"tags,omitempty"`

	Nodes         json.RawMessage `json:"nodes,omitempty"`
	Connections   json.RawMessage `json:"connections,omitempty"`
	NodeGroups    json.RawMessage `json:"nodeGroups,omitempty"`
	Settings      json.RawMessage `json:"settings,omitempty"`
	StaticData    json.RawMessage `json:"staticData,omitempty"`
	PinData       json.RawMessage `json:"pinData,omitempty"`
	Meta          json.RawMessage `json:"meta,omitempty"`
	Shared        json.RawMessage `json:"shared,omitempty"`
	ActiveVersion json.RawMessage `json:"activeVersion,omitempty"`

	// Extra holds fields this type does not name, so a workflow survives a
	// read-edit-write round trip even when the instance is newer than the CLI.
	Extra map[string]json.RawMessage `json:"-"`
}

// workflowFields are the field names [Workflow] decodes itself. Anything else
// lands in Extra.
var workflowFields = []string{
	"id", "name", "description", "active", "activeVersionId", "isArchived",
	"versionId", "versionCounter", "sourceWorkflowId", "triggerCount",
	"createdAt", "updatedAt", "tags", "nodes", "connections", "nodeGroups",
	"settings", "staticData", "pinData", "meta", "shared", "activeVersion",
}

// UnmarshalJSON decodes the named fields and keeps the rest in Extra.
func (w *Workflow) UnmarshalJSON(data []byte) error {
	type plain Workflow
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var rest map[string]json.RawMessage
	if err := json.Unmarshal(data, &rest); err != nil {
		return err
	}
	for _, known := range workflowFields {
		delete(rest, known)
	}
	*w = Workflow(decoded)
	if len(rest) > 0 {
		w.Extra = rest
	}
	return nil
}

// MarshalJSON writes the named fields followed by Extra, in sorted order, so
// the output is deterministic.
func (w Workflow) MarshalJSON() ([]byte, error) {
	type plain Workflow
	base, err := json.Marshal(plain(w))
	if err != nil {
		return nil, err
	}
	if len(w.Extra) == 0 {
		return base, nil
	}
	names := slices.Sorted(maps.Keys(w.Extra))

	var b bytes.Buffer
	b.Grow(len(base) + len(w.Extra)*32)
	trimmed := base
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '}' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	b.Write(trimmed)
	for i, name := range names {
		if i > 0 || len(base) > 2 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(w.Extra[name])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Document returns the workflow as a write document: every field the instance
// sent, raw. Reduce it with [WorkflowDocument.ForUpdate] before sending it back.
func (w Workflow) Document() (WorkflowDocument, error) {
	encoded, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}
	var doc WorkflowDocument
	if err := json.Unmarshal(encoded, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// WorkflowNode is the part of a node every node type has, which is what list
// and summary output needs. The full node, including its parameters, stays in
// [Workflow.Nodes].
type WorkflowNode struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	TypeVersion any    `json:"typeVersion,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

// NodeSummaries decodes the identifying part of each node. A workflow whose
// nodes are absent or unreadable yields no summaries and no error: this is
// display detail, never a reason to fail a command.
func (w Workflow) NodeSummaries() []WorkflowNode {
	if len(w.Nodes) == 0 {
		return nil
	}
	var nodes []WorkflowNode
	if err := json.Unmarshal(w.Nodes, &nodes); err != nil {
		return nil
	}
	return nodes
}

// NodeCount is the number of nodes in the workflow definition.
func (w Workflow) NodeCount() int { return len(w.NodeSummaries()) }

// WorkflowVersion is one stored version of a workflow definition.
type WorkflowVersion struct {
	VersionID   string          `json:"versionId,omitempty"`
	WorkflowID  string          `json:"workflowId,omitempty"`
	Authors     string          `json:"authors,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	CreatedAt   string          `json:"createdAt,omitempty"`
	UpdatedAt   string          `json:"updatedAt,omitempty"`
	Nodes       json.RawMessage `json:"nodes,omitempty"`
	Connections json.RawMessage `json:"connections,omitempty"`
	NodeGroups  json.RawMessage `json:"nodeGroups,omitempty"`
}

// WorkflowVersionSummary is one entry of the version history: metadata only,
// without the definition. Name and description are null for versions saved
// before the instance started naming them.
type WorkflowVersionSummary struct {
	VersionID   string `json:"versionId,omitempty"`
	WorkflowID  string `json:"workflowId,omitempty"`
	Authors     string `json:"authors,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// ListWorkflowsOptions are the query parameters of GET /workflows. It pages by
// cursor; offset is documented as well and kept for instances that answer with
// it, but cursor and offset are not meant to be combined.
type ListWorkflowsOptions struct {
	ListOptions
	// Offset skips this many workflows. Zero starts at the first one.
	Offset int
	// Active, when set, keeps only published (true) or unpublished (false)
	// workflows.
	Active *bool
	// Tags keeps only workflows carrying every one of these tag names.
	Tags []string
	// Name keeps only workflows with this exact name.
	Name string
	// ProjectID keeps only workflows in one project.
	ProjectID string
	// ExcludePinnedData drops pinned sample data from the response, which is
	// the bulk of it on workflows that carry any.
	ExcludePinnedData bool
}

// Validate rejects options the API would reject or that indicate a bug.
func (o ListWorkflowsOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Offset < 0 {
		return fmt.Errorf("offset must not be negative, got %d", o.Offset)
	}
	for i, tag := range o.Tags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("tag %d is empty: pass a tag name", i+1)
		}
		if strings.Contains(tag, ",") {
			return fmt.Errorf("tag %q contains a comma: the API separates tag names with commas, so a name cannot hold one", tag)
		}
	}
	return nil
}

// apply encodes the options as query parameters.
func (o ListWorkflowsOptions) apply() url.Values {
	query := o.ListOptions.Apply(nil)
	if o.Offset > 0 {
		query.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.Active != nil {
		query.Set("active", strconv.FormatBool(*o.Active))
	}
	if len(o.Tags) > 0 {
		query.Set("tags", strings.Join(o.Tags, ","))
	}
	if o.Name != "" {
		query.Set("name", o.Name)
	}
	if o.ProjectID != "" {
		query.Set("projectId", o.ProjectID)
	}
	if o.ExcludePinnedData {
		query.Set("excludePinnedData", "true")
	}
	return query
}

// UpdateWorkflowOptions are the query parameters of PUT /workflows/{id}.
type UpdateWorkflowOptions struct {
	// PublishIfActive, when set to false, stores the change as a draft instead
	// of republishing a published workflow. Nil leaves the server default,
	// which republishes.
	PublishIfActive *bool
}

// PublishWorkflowRequest is the optional body of a publish call. The zero value
// publishes the latest version under its existing name.
type PublishWorkflowRequest struct {
	// VersionID publishes one specific version instead of the latest.
	VersionID string `json:"versionId,omitempty"`
	// Name labels the published version; it does not rename the workflow.
	Name string `json:"name,omitempty"`
	// Description describes the published version.
	Description string `json:"description,omitempty"`
}

// IsZero reports whether the request carries nothing, in which case an empty
// JSON object is sent.
func (r PublishWorkflowRequest) IsZero() bool {
	return r.VersionID == "" && r.Name == "" && r.Description == ""
}

// transferWorkflowRequest is the body of a workflow transfer.
type transferWorkflowRequest struct {
	DestinationProjectID string `json:"destinationProjectId"`
}

// workflowTagRequest is one entry of the tag replacement body. The API accepts
// tag IDs only, never names.
type workflowTagRequest struct {
	ID string `json:"id"`
}

// ListWorkflows returns one cursor-paginated page of workflows.
func (c *Client) ListWorkflows(ctx context.Context, opts ListWorkflowsOptions) (Page[Workflow], error) {
	if err := opts.Validate(); err != nil {
		return Page[Workflow]{}, err
	}
	var page Page[Workflow]
	if _, err := c.Do(ctx, Request{Path: WorkflowsPath, Query: opts.apply()}, &page); err != nil {
		return Page[Workflow]{}, err
	}
	return page, nil
}

// CreateWorkflow creates one workflow from a write document. The document must
// already be reduced with [WorkflowDocument.ForCreate]: a read-only field in the
// body is a 400, not an ignored field.
func (c *Client) CreateWorkflow(ctx context.Context, doc WorkflowDocument) (*Workflow, error) {
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	var created Workflow
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   WorkflowsPath,
		Body:   doc,
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetWorkflow returns one workflow with its full definition. excludePinnedData
// drops the pinned sample data, which is the bulk of the response on workflows
// that carry any.
func (c *Client) GetWorkflow(ctx context.Context, id string, excludePinnedData bool) (*Workflow, error) {
	if err := validateWorkflowID(id); err != nil {
		return nil, err
	}
	query := url.Values{}
	if excludePinnedData {
		query.Set("excludePinnedData", "true")
	}
	var workflow Workflow
	if _, err := c.Do(ctx, Request{Path: PathJoin("workflows", id), Query: query}, &workflow); err != nil {
		return nil, err
	}
	return &workflow, nil
}

// UpdateWorkflow replaces the definition of one workflow. This is a full
// replacement, not a patch: whatever the document leaves out is cleared.
//
// A published workflow is republished unless PublishIfActive is false. That
// republication needs workflow:activate and the project's workflow:publish
// permission; without them the new version is still saved as a draft and the
// call fails with 403, which [IsForbidden] reports. The saved draft is not
// rolled back, so a 403 here is a partial success, not a no-op.
func (c *Client) UpdateWorkflow(ctx context.Context, id string, doc WorkflowDocument, opts UpdateWorkflowOptions) (*Workflow, error) {
	if err := validateWorkflowID(id); err != nil {
		return nil, err
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	query := url.Values{}
	if opts.PublishIfActive != nil {
		query.Set("publishIfActive", strconv.FormatBool(*opts.PublishIfActive))
	}
	var updated Workflow
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("workflows", id),
		Query:  query,
		Body:   doc,
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteWorkflow permanently deletes one workflow and returns it as the server
// last saw it. Archiving is the reversible alternative; see
// [Client.ArchiveWorkflow].
func (c *Client) DeleteWorkflow(ctx context.Context, id string) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodDelete, id, "", nil)
}

// ArchiveWorkflow soft-deletes one workflow. It is idempotent: archiving an
// archived workflow returns it unchanged.
func (c *Client) ArchiveWorkflow(ctx context.Context, id string) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodPost, id, "archive", nil)
}

// UnarchiveWorkflow restores an archived workflow.
func (c *Client) UnarchiveWorkflow(ctx context.Context, id string) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodPost, id, "unarchive", nil)
}

// PublishWorkflow puts a workflow version live. In n8n v1 this was called
// activating a workflow. A publication blocked by an open review or a webhook
// path collision answers 409; see [IsConflict].
func (c *Client) PublishWorkflow(ctx context.Context, id string, req PublishWorkflowRequest) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodPost, id, "publish", req)
}

// UnpublishWorkflow takes the published version of a workflow offline. In n8n
// v1 this was called deactivating a workflow.
func (c *Client) UnpublishWorkflow(ctx context.Context, id string) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodPost, id, "unpublish", nil)
}

// ActivateWorkflow is the deprecated spelling of [Client.PublishWorkflow]. The
// API still serves POST /workflows/{id}/activate; new code should publish.
func (c *Client) ActivateWorkflow(ctx context.Context, id string, req PublishWorkflowRequest) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodPost, id, "activate", req)
}

// DeactivateWorkflow is the deprecated spelling of [Client.UnpublishWorkflow].
func (c *Client) DeactivateWorkflow(ctx context.Context, id string) (*Workflow, error) {
	return c.workflowCall(ctx, http.MethodPost, id, "deactivate", nil)
}

// workflowCall runs one workflow action that answers with the workflow itself.
// action is the trailing path segment, or empty for the workflow itself.
func (c *Client) workflowCall(ctx context.Context, method, id, action string, body any) (*Workflow, error) {
	if err := validateWorkflowID(id); err != nil {
		return nil, err
	}
	path := PathJoin("workflows", id)
	if action != "" {
		path = PathJoin("workflows", id, action)
	}
	var workflow Workflow
	if _, err := c.Do(ctx, Request{Method: method, Path: path, Body: body}, &workflow); err != nil {
		return nil, err
	}
	return &workflow, nil
}

// TransferWorkflow moves one workflow into another project. The documented 204
// response has no body, so the workflow has to be read again to see the result.
func (c *Client) TransferWorkflow(ctx context.Context, id, destinationProjectID string) error {
	if err := validateWorkflowID(id); err != nil {
		return err
	}
	if err := validateWorkflowText("destination project ID", destinationProjectID, true); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("workflows", id, "transfer"),
		Body:   transferWorkflowRequest{DestinationProjectID: destinationProjectID},
	}, nil)
	return err
}

// ListWorkflowHistory returns one cursor-paginated page of stored versions,
// newest first, with metadata only.
func (c *Client) ListWorkflowHistory(ctx context.Context, id string, opts ListOptions) (Page[WorkflowVersionSummary], error) {
	if err := validateWorkflowID(id); err != nil {
		return Page[WorkflowVersionSummary]{}, err
	}
	if err := opts.Validate(); err != nil {
		return Page[WorkflowVersionSummary]{}, err
	}
	var page Page[WorkflowVersionSummary]
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("workflows", id, "history"),
		Query: opts.Apply(nil),
	}, &page); err != nil {
		return Page[WorkflowVersionSummary]{}, err
	}
	return page, nil
}

// GetWorkflowVersion returns one stored version with its definition.
func (c *Client) GetWorkflowVersion(ctx context.Context, workflowID, versionID string) (*WorkflowVersion, error) {
	return c.workflowVersion(ctx, PathJoin("workflows", workflowID, "versions", versionID), workflowID, versionID)
}

// GetWorkflowVersionLegacy reads a version through the deprecated
// GET /workflows/{id}/{versionId} route, for instances too old to serve
// /versions/{versionId}. New code should call [Client.GetWorkflowVersion].
func (c *Client) GetWorkflowVersionLegacy(ctx context.Context, workflowID, versionID string) (*WorkflowVersion, error) {
	return c.workflowVersion(ctx, PathJoin("workflows", workflowID, versionID), workflowID, versionID)
}

func (c *Client) workflowVersion(ctx context.Context, path, workflowID, versionID string) (*WorkflowVersion, error) {
	if err := validateWorkflowID(workflowID); err != nil {
		return nil, err
	}
	if err := validateWorkflowText("version ID", versionID, true); err != nil {
		return nil, err
	}
	var version WorkflowVersion
	if _, err := c.Do(ctx, Request{Path: path}, &version); err != nil {
		return nil, err
	}
	return &version, nil
}

// GetWorkflowTags returns the tags attached to one workflow.
func (c *Client) GetWorkflowTags(ctx context.Context, id string) ([]Tag, error) {
	if err := validateWorkflowID(id); err != nil {
		return nil, err
	}
	var tags []Tag
	if _, err := c.Do(ctx, Request{Path: PathJoin("workflows", id, "tags")}, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// SetWorkflowTags replaces the tags of one workflow with the given tag IDs. It
// is a replacement, not an addition: an empty list clears every tag, and a tag
// left out of the list is removed from the workflow. Tag IDs come from
// `n8n tag list`; the API does not accept tag names here.
func (c *Client) SetWorkflowTags(ctx context.Context, id string, tagIDs []string) ([]Tag, error) {
	if err := validateWorkflowID(id); err != nil {
		return nil, err
	}
	body := make([]workflowTagRequest, 0, len(tagIDs))
	seen := map[string]bool{}
	for i, tagID := range tagIDs {
		if err := validateWorkflowText("tag ID", tagID, true); err != nil {
			return nil, fmt.Errorf("tag %d: %w", i+1, err)
		}
		if seen[tagID] {
			return nil, fmt.Errorf("tag ID %q is listed twice", tagID)
		}
		seen[tagID] = true
		body = append(body, workflowTagRequest{ID: tagID})
	}
	var tags []Tag
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("workflows", id, "tags"),
		Body:   body,
	}, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func validateWorkflowID(id string) error { return validateWorkflowText("ID", id, true) }

func validateWorkflowText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("workflow %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("workflow %s must not start or end with whitespace", field)
	}
	return nil
}
