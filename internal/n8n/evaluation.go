package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// EvaluationRunStatus is one asynchronous evaluation test-run state.
type EvaluationRunStatus string

const (
	EvaluationRunNew       EvaluationRunStatus = "new"
	EvaluationRunRunning   EvaluationRunStatus = "running"
	EvaluationRunCompleted EvaluationRunStatus = "completed"
	EvaluationRunError     EvaluationRunStatus = "error"
	EvaluationRunCancelled EvaluationRunStatus = "cancelled"
)

var evaluationRunStatuses = []string{
	string(EvaluationRunNew),
	string(EvaluationRunRunning),
	string(EvaluationRunCompleted),
	string(EvaluationRunError),
	string(EvaluationRunCancelled),
}

// EvaluationRun is the short response returned when an asynchronous test run
// is accepted.
type EvaluationRun struct {
	ID        string              `json:"id"`
	Status    EvaluationRunStatus `json:"status"`
	CreatedAt string              `json:"createdAt,omitempty"`
}

// EvaluationRunSummary is one test run with its aggregate result. Metrics and
// error details stay raw because metric names and error shapes are defined by
// the workflow rather than by the public API contract.
type EvaluationRunSummary struct {
	ID            string              `json:"id"`
	Status        EvaluationRunStatus `json:"status"`
	RunAt         *string             `json:"runAt"`
	CompletedAt   *string             `json:"completedAt"`
	Metrics       json.RawMessage     `json:"metrics,omitempty"`
	ErrorCode     *string             `json:"errorCode"`
	ErrorDetails  json.RawMessage     `json:"errorDetails,omitempty"`
	FinalResult   *string             `json:"finalResult"`
	TestCaseCount int                 `json:"testCaseCount"`
	CreatedAt     string              `json:"createdAt,omitempty"`
	UpdatedAt     string              `json:"updatedAt,omitempty"`
}

// EvaluationCancelResult acknowledges cancellation of an asynchronous run.
type EvaluationCancelResult struct {
	ID     string              `json:"id"`
	Status EvaluationRunStatus `json:"status"`
}

// EvaluationTestCase is one case within an evaluation run. Inputs, outputs,
// metrics and errors remain raw workflow-defined JSON.
type EvaluationTestCase struct {
	ID           string          `json:"id"`
	Status       string          `json:"status"`
	RunAt        *string         `json:"runAt"`
	CompletedAt  *string         `json:"completedAt"`
	Metrics      json.RawMessage `json:"metrics,omitempty"`
	ErrorCode    *string         `json:"errorCode"`
	ErrorDetails json.RawMessage `json:"errorDetails,omitempty"`
	Inputs       json.RawMessage `json:"inputs,omitempty"`
	Outputs      json.RawMessage `json:"outputs,omitempty"`
	ExecutionID  *string         `json:"executionId"`
}

// ListEvaluationRunsOptions controls run status filtering and cursor
// pagination for one workflow.
type ListEvaluationRunsOptions struct {
	ListOptions
	Status string
}

// Validate rejects list parameters outside the OpenAPI contract.
func (o ListEvaluationRunsOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.Limit)
	}
	if o.Status != "" && !slices.Contains(evaluationRunStatuses, o.Status) {
		return fmt.Errorf("unknown evaluation status %q: use %s", o.Status, strings.Join(evaluationRunStatuses, ", "))
	}
	return nil
}

func (o ListEvaluationRunsOptions) apply() url.Values {
	query := o.ListOptions.Apply(nil)
	if o.Status != "" {
		query.Set("status", o.Status)
	}
	return query
}

// ListEvaluationCasesOptions controls cursor pagination for cases in one run.
type ListEvaluationCasesOptions struct {
	ListOptions
}

// Validate rejects case page sizes outside the OpenAPI contract.
func (o ListEvaluationCasesOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.Limit)
	}
	return nil
}

// ListEvaluationRuns returns one cursor-paginated page of test runs for a
// workflow.
func (c *Client) ListEvaluationRuns(ctx context.Context, workflowID string, opts ListEvaluationRunsOptions) (Page[EvaluationRunSummary], error) {
	if err := validateEvaluationID("workflow ID", workflowID); err != nil {
		return Page[EvaluationRunSummary]{}, err
	}
	if err := opts.Validate(); err != nil {
		return Page[EvaluationRunSummary]{}, err
	}
	var page Page[EvaluationRunSummary]
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("workflows", workflowID, "test-runs"),
		Query: opts.apply(),
	}, &page); err != nil {
		return Page[EvaluationRunSummary]{}, err
	}
	return page, nil
}

// CreateEvaluationRun starts an asynchronous test run. The workflow must have
// a configured evaluation trigger.
func (c *Client) CreateEvaluationRun(ctx context.Context, workflowID string) (*EvaluationRun, error) {
	if err := validateEvaluationID("workflow ID", workflowID); err != nil {
		return nil, err
	}
	var run EvaluationRun
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("workflows", workflowID, "test-runs"),
	}, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// GetEvaluationRun returns one test run with aggregate metrics and final
// result.
func (c *Client) GetEvaluationRun(ctx context.Context, workflowID, runID string) (*EvaluationRunSummary, error) {
	if err := validateEvaluationIDs(workflowID, runID); err != nil {
		return nil, err
	}
	var run EvaluationRunSummary
	if _, err := c.Do(ctx, Request{
		Path: PathJoin("workflows", workflowID, "test-runs", runID),
	}, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// CancelEvaluationRun requests cancellation of a new or running test run.
func (c *Client) CancelEvaluationRun(ctx context.Context, workflowID, runID string) (*EvaluationCancelResult, error) {
	if err := validateEvaluationIDs(workflowID, runID); err != nil {
		return nil, err
	}
	var result EvaluationCancelResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("workflows", workflowID, "test-runs", runID, "cancel"),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListEvaluationCases returns one cursor-paginated page of per-case results.
func (c *Client) ListEvaluationCases(ctx context.Context, workflowID, runID string, opts ListEvaluationCasesOptions) (Page[EvaluationTestCase], error) {
	if err := validateEvaluationIDs(workflowID, runID); err != nil {
		return Page[EvaluationTestCase]{}, err
	}
	if err := opts.Validate(); err != nil {
		return Page[EvaluationTestCase]{}, err
	}
	var page Page[EvaluationTestCase]
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("workflows", workflowID, "test-runs", runID, "test-cases"),
		Query: opts.ListOptions.Apply(nil),
	}, &page); err != nil {
		return Page[EvaluationTestCase]{}, err
	}
	return page, nil
}

func validateEvaluationIDs(workflowID, runID string) error {
	if err := validateEvaluationID("workflow ID", workflowID); err != nil {
		return err
	}
	return validateEvaluationID("evaluation run ID", runID)
}

func validateEvaluationID(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	return nil
}
