package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// VariablesPath is the collection endpoint for instance and project variables.
const VariablesPath = "/variables"

// Variable is one n8n variable. A nil Project means the variable is global.
// Value is intentionally present because listing variables is the API's way to
// retrieve it; callers must keep it out of diagnostics and logs.
type Variable struct {
	ID      string           `json:"id"`
	Key     string           `json:"key"`
	Value   string           `json:"value"`
	Type    string           `json:"type,omitempty"`
	Project *VariableProject `json:"project,omitempty"`
}

// VariableProject is the project scope attached to a variable. The API
// returns the full project there, so it is the same model as [Project].
type VariableProject = Project

// VariableState is an optional server-side list filter.
type VariableState string

const (
	// VariableStateEmpty selects variables whose value is empty.
	VariableStateEmpty VariableState = "empty"
)

// ListVariablesOptions contains pagination and variable-specific filters.
type ListVariablesOptions struct {
	ListOptions
	// ProjectID limits results to one project. Empty leaves the filter unset.
	ProjectID string
	// State may be empty (no filter) or VariableStateEmpty.
	State VariableState
}

// Validate rejects invalid filters before transport.
func (o ListVariablesOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if err := validateVariableText("project ID", o.ProjectID, false); err != nil {
		return err
	}
	if o.State != "" && o.State != VariableStateEmpty {
		return fmt.Errorf("variable state %q is invalid: use %q or leave it unset", o.State, VariableStateEmpty)
	}
	return nil
}

func (o ListVariablesOptions) apply() url.Values {
	query := o.ListOptions.Apply(nil)
	if o.ProjectID != "" {
		query.Set("projectId", o.ProjectID)
	}
	if o.State != "" {
		query.Set("state", string(o.State))
	}
	return query
}

// Variable field limits from the contract.
const (
	MaxVariableKey       = 50
	MaxVariableValue     = 1000
	MaxVariableProjectID = 36
)

var (
	// variableCreateKeyPattern is the key shape create accepts: no leading
	// digit.
	variableCreateKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// variableUpdateKeyPattern is the looser key shape update accepts.
	variableUpdateKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// VariableRequest is the complete create or replacement document. ProjectID is
// optional; omitting it on create addresses global scope. Value is required
// by the API but may be the empty string.
type VariableRequest struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	ProjectID string `json:"projectId,omitempty"`
}

// Validate rejects malformed replacement documents before transport without
// including the value in any diagnostic. Create is stricter about the key;
// see [VariableRequest.ValidateCreate].
func (r VariableRequest) Validate() error {
	if err := validateVariableText("key", r.Key, true); err != nil {
		return err
	}
	if len(r.Key) > MaxVariableKey {
		return fmt.Errorf("variable key is %d characters; the API accepts at most %d", len(r.Key), MaxVariableKey)
	}
	if !variableUpdateKeyPattern.MatchString(r.Key) {
		return fmt.Errorf("variable key %q is invalid: use only letters, digits and underscores", r.Key)
	}
	if n := utf8.RuneCountInString(r.Value); n > MaxVariableValue {
		return fmt.Errorf("variable value is %d characters; the API accepts at most %d", n, MaxVariableValue)
	}
	if err := validateVariableText("project ID", r.ProjectID, false); err != nil {
		return err
	}
	if len(r.ProjectID) > MaxVariableProjectID {
		return fmt.Errorf("variable project ID is %d characters; the API accepts at most %d", len(r.ProjectID), MaxVariableProjectID)
	}
	return nil
}

// ValidateCreate is [VariableRequest.Validate] plus the create-only rule
// that a key must not start with a digit.
func (r VariableRequest) ValidateCreate() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !variableCreateKeyPattern.MatchString(r.Key) {
		return fmt.Errorf("variable key %q is invalid: start with a letter or underscore, then use only letters, digits and underscores", r.Key)
	}
	return nil
}

// ListVariables returns one cursor-paginated page. Returned values are API
// output and must not be copied into errors or debug logs.
func (c *Client) ListVariables(ctx context.Context, opts ListVariablesOptions) (Page[Variable], error) {
	if err := opts.Validate(); err != nil {
		return Page[Variable]{}, err
	}
	var page Page[Variable]
	if _, err := c.Do(ctx, Request{Path: VariablesPath, Query: opts.apply()}, &page); err != nil {
		return Page[Variable]{}, err
	}
	return page, nil
}

// CreateVariable creates a global or project-scoped variable. The documented
// 201 response has no body.
func (c *Client) CreateVariable(ctx context.Context, request VariableRequest) error {
	if err := request.ValidateCreate(); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodPost, Path: VariablesPath, Body: request}, nil)
	return redactVariableAPIError(err)
}

// UpdateVariable fully replaces one variable. id is escaped as one path
// segment. The documented 204 response has no body.
func (c *Client) UpdateVariable(ctx context.Context, id string, request VariableRequest) error {
	if err := validateVariableID(id); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("variables", id),
		Body:   request,
	}, nil)
	return redactVariableAPIError(err)
}

// DeleteVariable permanently removes one variable. The documented 204
// response has no body.
func (c *Client) DeleteVariable(ctx context.Context, id string) error {
	if err := validateVariableID(id); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("variables", id)}, nil)
	return err
}

func validateVariableID(id string) error {
	return validateVariableText("ID", id, true)
}

func validateVariableText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("variable %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("variable %s must not start or end with whitespace", field)
	}
	return nil
}

// redactVariableAPIError preserves transport metadata while removing
// server-controlled details that could echo a submitted variable value.
func redactVariableAPIError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "variable request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
