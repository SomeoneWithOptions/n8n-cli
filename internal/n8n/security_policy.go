package n8n

import (
	"context"
	"fmt"
	"net/http"
)

// SecurityPolicyPath is the instance-wide security policy endpoint.
const SecurityPolicyPath = "/settings/security-policy"

// SecurityRedactionFloor is the minimum execution-data redaction level the
// instance enforces.
type SecurityRedactionFloor string

const (
	SecurityRedactionOff        SecurityRedactionFloor = "off"
	SecurityRedactionProduction SecurityRedactionFloor = "production"
	SecurityRedactionAll        SecurityRedactionFloor = "all"
)

// SecurityRedactionEnforcement holds the instance-wide execution-data
// redaction floor.
type SecurityRedactionEnforcement struct {
	Floor SecurityRedactionFloor `json:"floor"`
}

// SecurityPolicy is the effective instance policy. The three usage counts are
// read-only awareness fields returned by the API.
type SecurityPolicy struct {
	PersonalSpacePublishing         bool                         `json:"personalSpacePublishing"`
	PersonalSpaceSharing            bool                         `json:"personalSpaceSharing"`
	PublishedPersonalWorkflowsCount int                          `json:"publishedPersonalWorkflowsCount"`
	SharedPersonalWorkflowsCount    int                          `json:"sharedPersonalWorkflowsCount"`
	SharedPersonalCredentialsCount  int                          `json:"sharedPersonalCredentialsCount"`
	RedactionEnforcement            SecurityRedactionEnforcement `json:"redactionEnforcement"`
}

// UpdateSecurityPolicyRequest is a full replacement document. Pointer booleans
// preserve the distinction between an omitted required field and false. Usage
// counts are accepted so JSON returned by GET can be edited and sent back; the
// server treats them as read-only and ignores them.
type UpdateSecurityPolicyRequest struct {
	PersonalSpacePublishing         *bool                        `json:"personalSpacePublishing"`
	PersonalSpaceSharing            *bool                        `json:"personalSpaceSharing"`
	RedactionEnforcement            SecurityRedactionEnforcement `json:"redactionEnforcement"`
	PublishedPersonalWorkflowsCount *int                         `json:"publishedPersonalWorkflowsCount,omitempty"`
	SharedPersonalWorkflowsCount    *int                         `json:"sharedPersonalWorkflowsCount,omitempty"`
	SharedPersonalCredentialsCount  *int                         `json:"sharedPersonalCredentialsCount,omitempty"`
}

// Validate enforces every writable replacement field before transport.
func (r UpdateSecurityPolicyRequest) Validate() error {
	if r.PersonalSpacePublishing == nil {
		return fmt.Errorf("personalSpacePublishing is required and must be true or false")
	}
	if r.PersonalSpaceSharing == nil {
		return fmt.Errorf("personalSpaceSharing is required and must be true or false")
	}
	switch r.RedactionEnforcement.Floor {
	case SecurityRedactionOff, SecurityRedactionProduction, SecurityRedactionAll:
		return nil
	default:
		return fmt.Errorf("redactionEnforcement.floor must be one of off, production, or all")
	}
}

// GetSecurityPolicy returns the effective instance-wide security policy and
// its read-only usage counts.
func (c *Client) GetSecurityPolicy(ctx context.Context) (*SecurityPolicy, error) {
	var policy SecurityPolicy
	if _, err := c.Do(ctx, Request{Path: SecurityPolicyPath}, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// UpdateSecurityPolicy fully replaces every writable security policy field.
func (c *Client) UpdateSecurityPolicy(ctx context.Context, request UpdateSecurityPolicyRequest) (*SecurityPolicy, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var policy SecurityPolicy
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   SecurityPolicyPath,
		Body:   request,
	}, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}
