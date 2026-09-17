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
	"time"
)

// ExecutionsPath is the collection endpoint for executions.
const ExecutionsPath = "/executions"

// Execution is one workflow run. Data, WorkflowData and CustomData stay raw:
// their shape depends on the workflow and can be large. They are absent unless
// detailed data was requested (except for retry, whose response includes them).
type Execution struct {
	ID                     string               `json:"id"`
	Finished               bool                 `json:"finished"`
	Mode                   string               `json:"mode"`
	RetryOf                *string              `json:"retryOf,omitempty"`
	RetrySuccessID         *string              `json:"retrySuccessId,omitempty"`
	Status                 string               `json:"status"`
	CreatedAt              string               `json:"createdAt,omitempty"`
	StartedAt              *string              `json:"startedAt,omitempty"`
	StoppedAt              *string              `json:"stoppedAt,omitempty"`
	DeletedAt              *string              `json:"deletedAt,omitempty"`
	WorkflowID             string               `json:"workflowId"`
	WaitTill               *string              `json:"waitTill,omitempty"`
	StoredAt               string               `json:"storedAt,omitempty"`
	TracingContext         *ExecutionTrace      `json:"tracingContext,omitempty"`
	DeduplicationKey       *string              `json:"deduplicationKey,omitempty"`
	JSONSizeBytes          float64              `json:"jsonSizeBytes,omitempty"`
	BinaryDataSizeBytes    float64              `json:"binaryDataSizeBytes,omitempty"`
	WorkflowVersionID      *string              `json:"workflowVersionId,omitempty"`
	UsedPrivateCredentials bool                 `json:"usedPrivateCredentials,omitempty"`
	Data                   json.RawMessage      `json:"data,omitempty"`
	WorkflowData           json.RawMessage      `json:"workflowData,omitempty"`
	CustomData             json.RawMessage      `json:"customData,omitempty"`
	Annotation             *ExecutionAnnotation `json:"annotation,omitempty"`
	DataTooLargeToDisplay  bool                 `json:"dataTooLargeToDisplay,omitempty"`
}

// ExecutionTrace is W3C trace context recorded for an execution.
type ExecutionTrace struct {
	Traceparent string `json:"traceparent"`
	Tracestate  string `json:"tracestate,omitempty"`
}

// ExecutionAnnotation is review metadata carried by a retried execution.
type ExecutionAnnotation struct {
	ID   json.Number `json:"id"`
	Vote *string     `json:"vote"`
	Tags []Tag       `json:"tags"`
}

// DeletedExecution is the execution returned by DELETE /executions/{id}. That
// endpoint uniquely returns id as a JSON number, so it has a separate model.
type DeletedExecution struct {
	ID                     json.Number     `json:"id"`
	Finished               bool            `json:"finished"`
	Mode                   string          `json:"mode"`
	RetryOf                *string         `json:"retryOf"`
	RetrySuccessID         *string         `json:"retrySuccessId"`
	Status                 string          `json:"status"`
	CreatedAt              string          `json:"createdAt"`
	StartedAt              *string         `json:"startedAt"`
	StoppedAt              *string         `json:"stoppedAt"`
	DeletedAt              *string         `json:"deletedAt"`
	WorkflowID             string          `json:"workflowId"`
	WaitTill               *string         `json:"waitTill"`
	StoredAt               string          `json:"storedAt"`
	TracingContext         *ExecutionTrace `json:"tracingContext"`
	DeduplicationKey       *string         `json:"deduplicationKey"`
	JSONSizeBytes          float64         `json:"jsonSizeBytes"`
	BinaryDataSizeBytes    float64         `json:"binaryDataSizeBytes"`
	WorkflowVersionID      *string         `json:"workflowVersionId"`
	UsedPrivateCredentials bool            `json:"usedPrivateCredentials"`
}

// ExecutionStopResult describes one execution after it was stopped.
type ExecutionStopResult struct {
	Mode      string `json:"mode"`
	StartedAt string `json:"startedAt"`
	StoppedAt string `json:"stoppedAt,omitempty"`
	Finished  bool   `json:"finished"`
	Status    string `json:"status"`
}

// StopManyExecutionsResult reports how many executions matched and stopped.
type StopManyExecutionsResult struct {
	Stopped int `json:"stopped"`
}

// ExecutionDataOptions control inclusion and redaction of detailed execution
// data. RedactExecutionData nil follows the workflow's redaction policy; false
// requests revealed data and therefore additionally needs execution:reveal.
type ExecutionDataOptions struct {
	IncludeData         bool
	IgnoreDataSizeLimit bool
	RedactExecutionData *bool
}

func (o ExecutionDataOptions) apply(query url.Values) url.Values {
	if query == nil {
		query = url.Values{}
	}
	if o.IncludeData {
		query.Set("includeData", "true")
	}
	if o.IgnoreDataSizeLimit {
		query.Set("ignoreDataSizeLimit", "true")
	}
	if o.RedactExecutionData != nil {
		query.Set("redactExecutionData", strconv.FormatBool(*o.RedactExecutionData))
	}
	return query
}

// ListExecutionsOptions are the query parameters of GET /executions.
type ListExecutionsOptions struct {
	ListOptions
	ExecutionDataOptions
	Status        string
	WorkflowID    string
	ProjectID     string
	StartedAfter  string
	StartedBefore string
}

var executionStatuses = []string{"canceled", "crashed", "error", "new", "running", "success", "unknown", "waiting"}
var stoppableExecutionStatuses = []string{"queued", "running", "waiting"}

// Validate rejects filters the API would reject.
func (o ListExecutionsOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.Limit)
	}
	if o.Status != "" && !slices.Contains(executionStatuses, o.Status) {
		return fmt.Errorf("unknown execution status %q: use %s", o.Status, strings.Join(executionStatuses, ", "))
	}
	if err := validateOptionalExecutionID("workflow ID", o.WorkflowID); err != nil {
		return err
	}
	if err := validateOptionalExecutionID("project ID", o.ProjectID); err != nil {
		return err
	}
	if err := validateExecutionTimeRange(o.StartedAfter, o.StartedBefore); err != nil {
		return err
	}
	return nil
}

func (o ListExecutionsOptions) apply() url.Values {
	query := o.ExecutionDataOptions.apply(o.ListOptions.Apply(nil))
	if o.Status != "" {
		query.Set("status", o.Status)
	}
	if o.WorkflowID != "" {
		query.Set("workflowId", o.WorkflowID)
	}
	if o.ProjectID != "" {
		query.Set("projectId", o.ProjectID)
	}
	if o.StartedAfter != "" {
		query.Set("startedAfter", o.StartedAfter)
	}
	if o.StartedBefore != "" {
		query.Set("startedBefore", o.StartedBefore)
	}
	return query
}

// StopManyExecutionsRequest selects queued, running, or waiting executions to
// cancel. Omitting WorkflowID selects executions across every accessible
// workflow, which callers should make explicit to users before sending.
type StopManyExecutionsRequest struct {
	Status        []string `json:"status"`
	WorkflowID    string   `json:"workflowId,omitempty"`
	StartedAfter  string   `json:"startedAfter,omitempty"`
	StartedBefore string   `json:"startedBefore,omitempty"`
}

// Validate rejects an empty or ambiguous bulk-stop request.
func (r StopManyExecutionsRequest) Validate() error {
	if len(r.Status) == 0 {
		return fmt.Errorf("at least one stop status is required: use queued, running, or waiting")
	}
	seen := map[string]bool{}
	for i, status := range r.Status {
		if !slices.Contains(stoppableExecutionStatuses, status) {
			return fmt.Errorf("stop status %d is %q: use queued, running, or waiting", i+1, status)
		}
		if seen[status] {
			return fmt.Errorf("stop status %q is listed twice", status)
		}
		seen[status] = true
	}
	if err := validateOptionalExecutionID("workflow ID", r.WorkflowID); err != nil {
		return err
	}
	return validateExecutionTimeRange(r.StartedAfter, r.StartedBefore)
}

// RetryExecutionOptions choose which workflow definition a retry runs.
type RetryExecutionOptions struct {
	// LoadWorkflow uses the currently saved workflow instead of the definition
	// stored with the original execution.
	LoadWorkflow bool `json:"loadWorkflow,omitempty"`
}

// ListExecutions returns one cursor-paginated page of executions.
func (c *Client) ListExecutions(ctx context.Context, opts ListExecutionsOptions) (Page[Execution], error) {
	if err := opts.Validate(); err != nil {
		return Page[Execution]{}, err
	}
	var page Page[Execution]
	if _, err := c.Do(ctx, Request{Path: ExecutionsPath, Query: opts.apply()}, &page); err != nil {
		return Page[Execution]{}, err
	}
	return page, nil
}

// GetExecution returns one execution, optionally with its detailed run data.
func (c *Client) GetExecution(ctx context.Context, id string, opts ExecutionDataOptions) (*Execution, error) {
	if err := validateExecutionID(id); err != nil {
		return nil, err
	}
	var execution Execution
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("executions", id),
		Query: opts.apply(nil),
	}, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

// DeleteExecution permanently deletes one stored execution and its run data.
func (c *Client) DeleteExecution(ctx context.Context, id string) (*DeletedExecution, error) {
	if err := validateExecutionID(id); err != nil {
		return nil, err
	}
	var deleted DeletedExecution
	if _, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("executions", id),
	}, &deleted); err != nil {
		return nil, err
	}
	return &deleted, nil
}

// GetExecutionTags returns the annotation tags attached to one execution.
func (c *Client) GetExecutionTags(ctx context.Context, id string) ([]Tag, error) {
	if err := validateExecutionID(id); err != nil {
		return nil, err
	}
	var tags []Tag
	if _, err := c.Do(ctx, Request{Path: PathJoin("executions", id, "tags")}, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// SetExecutionTags replaces all annotation tags on one execution. An empty
// slice clears them; tag IDs come from n8n tag list.
func (c *Client) SetExecutionTags(ctx context.Context, id string, tagIDs []string) ([]Tag, error) {
	if err := validateExecutionID(id); err != nil {
		return nil, err
	}
	body := make([]workflowTagRequest, 0, len(tagIDs))
	seen := map[string]bool{}
	for i, tagID := range tagIDs {
		if strings.TrimSpace(tagID) == "" {
			return nil, fmt.Errorf("tag %d: tag ID is required", i+1)
		}
		if strings.TrimSpace(tagID) != tagID {
			return nil, fmt.Errorf("tag %d: tag ID must not start or end with whitespace", i+1)
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
		Path:   PathJoin("executions", id, "tags"),
		Body:   body,
	}, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// StopManyExecutions cancels every accessible execution matching request.
func (c *Client) StopManyExecutions(ctx context.Context, request StopManyExecutionsRequest) (*StopManyExecutionsResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result StopManyExecutionsResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("executions", "stop"),
		Body:   request,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StopExecution cancels one queued, running, or waiting execution.
func (c *Client) StopExecution(ctx context.Context, id string) (*ExecutionStopResult, error) {
	if err := validateExecutionID(id); err != nil {
		return nil, err
	}
	var result ExecutionStopResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("executions", id, "stop"),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RetryExecution starts a new execution from an existing failed execution.
func (c *Client) RetryExecution(ctx context.Context, id string, opts RetryExecutionOptions) (*Execution, error) {
	if err := validateExecutionID(id); err != nil {
		return nil, err
	}
	var execution Execution
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("executions", id, "retry"),
		Body:   opts,
	}, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func validateExecutionID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("execution ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("execution ID must not start or end with whitespace")
	}
	return nil
}

func validateOptionalExecutionID(field, id string) error {
	if id == "" {
		return nil
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("execution %s must not start or end with whitespace", field)
	}
	return nil
}

func validateExecutionTimeRange(after, before string) error {
	parse := func(name, value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be RFC3339, for example 2026-09-17T14:30:00Z: %w", name, err)
		}
		return parsed, nil
	}
	start, err := parse("started-after", after)
	if err != nil {
		return err
	}
	end, err := parse("started-before", before)
	if err != nil {
		return err
	}
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return fmt.Errorf("started-after must not be later than started-before")
	}
	return nil
}
