package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// CredentialsPath is the collection endpoint for credentials.
const CredentialsPath = "/credentials"

// Credential is credential metadata returned by n8n. It deliberately has no
// data field: list, get, create, update, and delete must never expose stored
// credential secrets even if a server includes them unexpectedly.
type Credential struct {
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	Type                    string            `json:"type"`
	IsManaged               bool              `json:"isManaged,omitempty"`
	IsGlobal                bool              `json:"isGlobal,omitempty"`
	IsResolvable            bool              `json:"isResolvable,omitempty"`
	ResolvableAllowFallback bool              `json:"resolvableAllowFallback,omitempty"`
	ResolverID              *string           `json:"resolverId,omitempty"`
	CreatedAt               string            `json:"createdAt,omitempty"`
	UpdatedAt               string            `json:"updatedAt,omitempty"`
	Shared                  []CredentialShare `json:"shared,omitempty"`
}

// CredentialShare identifies a project with which a credential is shared.
type CredentialShare struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// CredentialData holds the secret data object sent when creating or updating
// a credential. Human-readable formatting, structured logging, and ordinary
// JSON marshaling redact it. Credential methods use a private wire shape to
// reveal it only to the HTTP request encoder.
type CredentialData struct {
	raw json.RawMessage
}

// NewCredentialData validates and copies a JSON object containing credential
// fields. Arrays, scalars, and null are rejected.
func NewCredentialData(raw []byte) (CredentialData, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return CredentialData{}, fmt.Errorf("credential data is required")
	}
	if !json.Valid(trimmed) {
		return CredentialData{}, fmt.Errorf("credential data must be valid JSON")
	}
	if trimmed[0] != '{' {
		return CredentialData{}, fmt.Errorf("credential data must be a JSON object")
	}
	return CredentialData{raw: append(json.RawMessage(nil), trimmed...)}, nil
}

// UnmarshalJSON accepts a credential data object without exposing its value in
// parse errors.
func (d *CredentialData) UnmarshalJSON(raw []byte) error {
	parsed, err := NewCredentialData(raw)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// MarshalJSON redacts secret data outside the private request wire shape.
func (d CredentialData) MarshalJSON() ([]byte, error) { return json.Marshal(Redacted) }

func (d CredentialData) String() string          { return Redacted }
func (d CredentialData) GoString() string        { return `"` + Redacted + `"` }
func (d CredentialData) LogValue() slog.Value    { return slog.StringValue(Redacted) }
func (d CredentialData) valid() bool             { return len(d.raw) != 0 }
func (d CredentialData) reveal() json.RawMessage { return append(json.RawMessage(nil), d.raw...) }

// CreateCredentialRequest is the secret-bearing document used to create one
// credential. Data is required; optional booleans retain omitted-vs-false.
type CreateCredentialRequest struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Data         CredentialData `json:"data"`
	IsResolvable *bool          `json:"isResolvable,omitempty"`
	ProjectID    string         `json:"projectId,omitempty"`
}

// Validate rejects malformed create documents before transport.
func (r CreateCredentialRequest) Validate() error {
	if err := validateCredentialText("name", r.Name, true); err != nil {
		return err
	}
	if err := validateCredentialText("type", r.Type, true); err != nil {
		return err
	}
	if !r.Data.valid() {
		return fmt.Errorf("credential data is required")
	}
	if err := validateCredentialText("project ID", r.ProjectID, false); err != nil {
		return err
	}
	return nil
}

// UpdateCredentialRequest contains fields accepted by PATCH /credentials/{id}.
// Pointers preserve omitted values, especially false booleans and empty data.
type UpdateCredentialRequest struct {
	Name          *string         `json:"name,omitempty"`
	Type          *string         `json:"type,omitempty"`
	Data          *CredentialData `json:"data,omitempty"`
	IsGlobal      *bool           `json:"isGlobal,omitempty"`
	IsResolvable  *bool           `json:"isResolvable,omitempty"`
	IsPartialData *bool           `json:"isPartialData,omitempty"`
}

// Validate rejects invalid text and an invalid programmatically-built data
// value before transport. Empty update objects remain valid per the schema.
func (r UpdateCredentialRequest) Validate() error {
	if r.Name != nil {
		if err := validateCredentialText("name", *r.Name, true); err != nil {
			return err
		}
	}
	if r.Type != nil {
		if err := validateCredentialText("type", *r.Type, true); err != nil {
			return err
		}
	}
	if r.Data != nil && !r.Data.valid() {
		return fmt.Errorf("credential data must be a JSON object")
	}
	return nil
}

// CredentialTestResult is the result of testing stored credential data.
type CredentialTestResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ListCredentials returns one cursor-paginated metadata page. Only instance
// owners and admins may use this endpoint; the server enforces that rule.
func (c *Client) ListCredentials(ctx context.Context, opts ListOptions) (Page[Credential], error) {
	if err := opts.Validate(); err != nil {
		return Page[Credential]{}, err
	}
	var page Page[Credential]
	if _, err := c.Do(ctx, Request{Path: CredentialsPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[Credential]{}, err
	}
	return page, nil
}

// CreateCredential creates a credential. Secret data is revealed only inside
// the private wire document and API error details are stripped in case a
// server or proxy echoes request fields.
func (c *Client) CreateCredential(ctx context.Context, request CreateCredentialRequest) (*Credential, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	wire := struct {
		Name         string          `json:"name"`
		Type         string          `json:"type"`
		Data         json.RawMessage `json:"data"`
		IsResolvable *bool           `json:"isResolvable,omitempty"`
		ProjectID    string          `json:"projectId,omitempty"`
	}{request.Name, request.Type, request.Data.reveal(), request.IsResolvable, request.ProjectID}
	var created Credential
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: CredentialsPath, Body: wire}, &created); err != nil {
		return nil, redactCredentialAPIError(err)
	}
	return &created, nil
}

// GetCredential returns metadata only. Unknown response fields, including an
// unexpected data object, are discarded by the response model.
func (c *Client) GetCredential(ctx context.Context, id string) (*Credential, error) {
	if err := validateCredentialID(id); err != nil {
		return nil, err
	}
	var credential Credential
	if _, err := c.Do(ctx, Request{Path: PathJoin("credentials", id)}, &credential); err != nil {
		return nil, err
	}
	return &credential, nil
}

// UpdateCredential updates metadata or secret data for an owned credential.
func (c *Client) UpdateCredential(ctx context.Context, id string, request UpdateCredentialRequest) (*Credential, error) {
	if err := validateCredentialID(id); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	wire := struct {
		Name          *string         `json:"name,omitempty"`
		Type          *string         `json:"type,omitempty"`
		Data          json.RawMessage `json:"data,omitempty"`
		IsGlobal      *bool           `json:"isGlobal,omitempty"`
		IsResolvable  *bool           `json:"isResolvable,omitempty"`
		IsPartialData *bool           `json:"isPartialData,omitempty"`
	}{Name: request.Name, Type: request.Type, IsGlobal: request.IsGlobal, IsResolvable: request.IsResolvable, IsPartialData: request.IsPartialData}
	if request.Data != nil {
		wire.Data = request.Data.reveal()
	}
	var updated Credential
	if _, err := c.Do(ctx, Request{Method: http.MethodPatch, Path: PathJoin("credentials", id), Body: wire}, &updated); err != nil {
		return nil, redactCredentialAPIError(err)
	}
	return &updated, nil
}

// DeleteCredential permanently removes an owned credential and returns only
// non-secret metadata from the server response.
func (c *Client) DeleteCredential(ctx context.Context, id string) (*Credential, error) {
	if err := validateCredentialID(id); err != nil {
		return nil, err
	}
	var deleted Credential
	if _, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("credentials", id)}, &deleted); err != nil {
		return nil, err
	}
	return &deleted, nil
}

// TestCredential asks n8n to test its stored data; no secret data is sent.
func (c *Client) TestCredential(ctx context.Context, id string) (*CredentialTestResult, error) {
	if err := validateCredentialID(id); err != nil {
		return nil, err
	}
	var result CredentialTestResult
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: PathJoin("credentials", id, "test")}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CredentialSchema returns the arbitrary JSON Schema for a credential type.
func (c *Client) CredentialSchema(ctx context.Context, credentialType string) (json.RawMessage, error) {
	if err := validateCredentialText("type", credentialType, true); err != nil {
		return nil, err
	}
	var schema json.RawMessage
	if _, err := c.Do(ctx, Request{Path: PathJoin("credentials", "schema", credentialType)}, &schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// TransferCredential moves an owned credential to another project.
func (c *Client) TransferCredential(ctx context.Context, id, destinationProjectID string) error {
	if err := validateCredentialID(id); err != nil {
		return err
	}
	if err := validateCredentialText("destination project ID", destinationProjectID, true); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("credentials", id, "transfer"),
		Body: struct {
			DestinationProjectID string `json:"destinationProjectId"`
		}{destinationProjectID},
	}, nil)
	return err
}

func validateCredentialID(id string) error {
	return validateCredentialText("ID", id, true)
}

func validateCredentialText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("credential %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("credential %s must not start or end with whitespace", field)
	}
	return nil
}

// redactCredentialAPIError preserves status, path, and request ID while
// dropping server-controlled details which could echo secret request data.
func redactCredentialAPIError(err error) error {
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "credential request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
