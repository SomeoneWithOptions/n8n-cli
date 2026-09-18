package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	// LDAPSettingsPath is the instance-wide LDAP configuration endpoint.
	LDAPSettingsPath = "/settings/ldap"
	// LDAPSyncPath lists and starts LDAP synchronization runs.
	LDAPSyncPath = "/settings/ldap/sync"

	// CredentialBlankingValue is n8n's stable placeholder for a stored secret.
	// Sending it back during a full replacement preserves the existing value.
	CredentialBlankingValue = "__n8n_BLANK_VALUE_e5362baf-c777-4d57-a609-6eaf1f9e87f6"
)

// LDAPConnectionSecurity controls transport security for the LDAP connection.
type LDAPConnectionSecurity string

const (
	LDAPSecurityNone     LDAPConnectionSecurity = "none"
	LDAPSecurityTLS      LDAPConnectionSecurity = "tls"
	LDAPSecurityStartTLS LDAPConnectionSecurity = "startTls"
)

// LDAPConfiguration is the effective instance-wide LDAP configuration.
// BindingAdminPassword is always either empty or CredentialBlankingValue after
// decoding, even if a non-conforming server returns plaintext.
type LDAPConfiguration struct {
	LoginEnabled            bool                   `json:"loginEnabled"`
	LoginLabel              string                 `json:"loginLabel"`
	ConnectionURL           string                 `json:"connectionUrl"`
	AllowUnauthorizedCerts  bool                   `json:"allowUnauthorizedCerts"`
	ConnectionSecurity      LDAPConnectionSecurity `json:"connectionSecurity"`
	ConnectionPort          int                    `json:"connectionPort"`
	BaseDN                  string                 `json:"baseDn"`
	BindingAdminDN          string                 `json:"bindingAdminDn"`
	BindingAdminPassword    string                 `json:"bindingAdminPassword"`
	FirstNameAttribute      string                 `json:"firstNameAttribute"`
	LastNameAttribute       string                 `json:"lastNameAttribute"`
	EmailAttribute          string                 `json:"emailAttribute"`
	LoginIDAttribute        string                 `json:"loginIdAttribute"`
	LDAPIDAttribute         string                 `json:"ldapIdAttribute"`
	UserFilter              string                 `json:"userFilter"`
	SynchronizationEnabled  bool                   `json:"synchronizationEnabled"`
	SynchronizationInterval int                    `json:"synchronizationInterval"`
	SearchPageSize          int                    `json:"searchPageSize"`
	SearchTimeout           int                    `json:"searchTimeout"`
	EnforceEmailUniqueness  bool                   `json:"enforceEmailUniqueness"`
}

func (c *LDAPConfiguration) redactPassword() {
	if c.BindingAdminPassword != "" {
		c.BindingAdminPassword = CredentialBlankingValue
	}
}

// UpdateLDAPConfigurationRequest is a full replacement document. Pointers
// distinguish omitted required fields from valid false, zero, and empty values.
type UpdateLDAPConfigurationRequest struct {
	LoginEnabled            *bool                   `json:"loginEnabled"`
	LoginLabel              *string                 `json:"loginLabel"`
	ConnectionURL           *string                 `json:"connectionUrl"`
	AllowUnauthorizedCerts  *bool                   `json:"allowUnauthorizedCerts"`
	ConnectionSecurity      *LDAPConnectionSecurity `json:"connectionSecurity"`
	ConnectionPort          *int                    `json:"connectionPort"`
	BaseDN                  *string                 `json:"baseDn"`
	BindingAdminDN          *string                 `json:"bindingAdminDn"`
	BindingAdminPassword    *string                 `json:"bindingAdminPassword"`
	FirstNameAttribute      *string                 `json:"firstNameAttribute"`
	LastNameAttribute       *string                 `json:"lastNameAttribute"`
	EmailAttribute          *string                 `json:"emailAttribute"`
	LoginIDAttribute        *string                 `json:"loginIdAttribute"`
	LDAPIDAttribute         *string                 `json:"ldapIdAttribute"`
	UserFilter              *string                 `json:"userFilter"`
	SynchronizationEnabled  *bool                   `json:"synchronizationEnabled"`
	SynchronizationInterval *int                    `json:"synchronizationInterval"`
	SearchPageSize          *int                    `json:"searchPageSize"`
	SearchTimeout           *int                    `json:"searchTimeout"`
	EnforceEmailUniqueness  *bool                   `json:"enforceEmailUniqueness"`
}

// Validate enforces every field required by full-replacement PUT semantics.
func (r UpdateLDAPConfigurationRequest) Validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{"loginEnabled", r.LoginEnabled != nil},
		{"loginLabel", r.LoginLabel != nil},
		{"connectionUrl", r.ConnectionURL != nil},
		{"allowUnauthorizedCerts", r.AllowUnauthorizedCerts != nil},
		{"connectionSecurity", r.ConnectionSecurity != nil},
		{"connectionPort", r.ConnectionPort != nil},
		{"baseDn", r.BaseDN != nil},
		{"bindingAdminDn", r.BindingAdminDN != nil},
		{"bindingAdminPassword", r.BindingAdminPassword != nil},
		{"firstNameAttribute", r.FirstNameAttribute != nil},
		{"lastNameAttribute", r.LastNameAttribute != nil},
		{"emailAttribute", r.EmailAttribute != nil},
		{"loginIdAttribute", r.LoginIDAttribute != nil},
		{"ldapIdAttribute", r.LDAPIDAttribute != nil},
		{"userFilter", r.UserFilter != nil},
		{"synchronizationEnabled", r.SynchronizationEnabled != nil},
		{"synchronizationInterval", r.SynchronizationInterval != nil},
		{"searchPageSize", r.SearchPageSize != nil},
		{"searchTimeout", r.SearchTimeout != nil},
		{"enforceEmailUniqueness", r.EnforceEmailUniqueness != nil},
	}
	for _, field := range required {
		if !field.present {
			return fmt.Errorf("%s is required in the full LDAP replacement", field.name)
		}
	}
	switch *r.ConnectionSecurity {
	case LDAPSecurityNone, LDAPSecurityTLS, LDAPSecurityStartTLS:
		return nil
	default:
		return fmt.Errorf("connectionSecurity must be one of none, tls, or startTls")
	}
}

// LDAPSyncMode selects whether synchronization changes users or only previews.
type LDAPSyncMode string

const (
	LDAPSyncLive LDAPSyncMode = "live"
	LDAPSyncDry  LDAPSyncMode = "dry"
)

// LDAPSyncRequest starts one synchronization run.
type LDAPSyncRequest struct {
	Type LDAPSyncMode `json:"type"`
}

// Validate rejects missing and unsupported synchronization modes.
func (r LDAPSyncRequest) Validate() error {
	switch r.Type {
	case LDAPSyncLive, LDAPSyncDry:
		return nil
	default:
		return fmt.Errorf("type must be live or dry")
	}
}

// LDAPSyncHistory is one completed LDAP synchronization run.
type LDAPSyncHistory struct {
	ID        int64        `json:"id"`
	RunMode   LDAPSyncMode `json:"runMode"`
	Status    string       `json:"status"`
	StartedAt string       `json:"startedAt"`
	EndedAt   string       `json:"endedAt"`
	Scanned   int          `json:"scanned"`
	Created   int          `json:"created"`
	Updated   int          `json:"updated"`
	Disabled  int          `json:"disabled"`
	Error     string       `json:"error"`
}

// ListLDAPSyncHistoryOptions are pagination parameters for sync history.
type ListLDAPSyncHistoryOptions struct{ ListOptions }

// Validate rejects pagination values the endpoint does not accept.
func (o ListLDAPSyncHistoryOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > 250 {
		return fmt.Errorf("limit must not exceed 250, got %d", o.Limit)
	}
	return nil
}

// GetLDAPConfiguration returns the effective LDAP configuration with its bind
// password represented only by n8n's blanking placeholder.
func (c *Client) GetLDAPConfiguration(ctx context.Context) (*LDAPConfiguration, error) {
	var configuration LDAPConfiguration
	if _, err := c.Do(ctx, Request{Path: LDAPSettingsPath}, &configuration); err != nil {
		return nil, err
	}
	configuration.redactPassword()
	return &configuration, nil
}

// UpdateLDAPConfiguration fully replaces the LDAP configuration. Sending
// CredentialBlankingValue preserves the password stored by n8n.
func (c *Client) UpdateLDAPConfiguration(ctx context.Context, request UpdateLDAPConfigurationRequest) (*LDAPConfiguration, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var configuration LDAPConfiguration
	if _, err := c.Do(ctx, Request{Method: http.MethodPut, Path: LDAPSettingsPath, Body: request}, &configuration); err != nil {
		return nil, redactLDAPAPIError(err)
	}
	configuration.redactPassword()
	return &configuration, nil
}

// ListLDAPSyncHistory returns one cursor-paginated page, newest first.
func (c *Client) ListLDAPSyncHistory(ctx context.Context, opts ListLDAPSyncHistoryOptions) (Page[LDAPSyncHistory], error) {
	if err := opts.Validate(); err != nil {
		return Page[LDAPSyncHistory]{}, err
	}
	var page Page[LDAPSyncHistory]
	if _, err := c.Do(ctx, Request{Path: LDAPSyncPath, Query: opts.ListOptions.Apply(nil)}, &page); err != nil {
		return Page[LDAPSyncHistory]{}, err
	}
	return page, nil
}

// RunLDAPSync starts a dry preview or a live user synchronization.
func (c *Client) RunLDAPSync(ctx context.Context, request LDAPSyncRequest) (*LDAPSyncHistory, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var history LDAPSyncHistory
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: LDAPSyncPath, Body: request}, &history); err != nil {
		return nil, err
	}
	return &history, nil
}

// redactLDAPAPIError prevents a server validation response from echoing a bind
// password submitted in a replacement while preserving status classification.
func redactLDAPAPIError(err error) error {
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "LDAP configuration request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
