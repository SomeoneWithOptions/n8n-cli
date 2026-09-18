package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const (
	// OIDCSettingsPath is the instance-wide OIDC SSO configuration endpoint.
	OIDCSettingsPath = "/settings/sso/oidc"

	// OIDCClientSecretRedactedValue is n8n's stable placeholder for a stored
	// OIDC client secret. Sending it back preserves the existing secret.
	OIDCClientSecretRedactedValue = "__n8n_CLIENT_SECRET_VALUE_e5362baf-c777-4d57-a609-6eaf1f9e87f6"
)

// OIDCPrompt controls the prompt parameter sent to the identity provider.
type OIDCPrompt string

const (
	OIDCPromptNone          OIDCPrompt = "none"
	OIDCPromptLogin         OIDCPrompt = "login"
	OIDCPromptConsent       OIDCPrompt = "consent"
	OIDCPromptSelectAccount OIDCPrompt = "select_account"
	OIDCPromptCreate        OIDCPrompt = "create"
)

// OIDCConfiguration is the effective instance-wide OIDC SSO configuration.
// ClientSecret is always empty or OIDCClientSecretRedactedValue after decoding,
// even if a non-conforming server returns plaintext.
type OIDCConfiguration struct {
	ClientID                            string     `json:"clientId"`
	ClientSecret                        string     `json:"clientSecret"`
	DiscoveryEndpoint                   string     `json:"discoveryEndpoint"`
	LoginEnabled                        bool       `json:"loginEnabled"`
	Prompt                              OIDCPrompt `json:"prompt"`
	AuthenticationContextClassReference []string   `json:"authenticationContextClassReference"`
	AdditionalScopes                    string     `json:"additionalScopes"`
	EmailVerifiedRequired               bool       `json:"emailVerifiedRequired"`
	RPInitiatedLogoutEnabled            bool       `json:"rpInitiatedLogoutEnabled"`
}

func (c *OIDCConfiguration) redactClientSecret() {
	if c.ClientSecret != "" {
		c.ClientSecret = OIDCClientSecretRedactedValue
	}
}

// SetOIDCConfigurationRequest is a full replacement document. Pointers
// distinguish omitted required fields from valid false, empty string, and
// empty-array values.
type SetOIDCConfigurationRequest struct {
	ClientID                            *string     `json:"clientId"`
	ClientSecret                        *string     `json:"clientSecret"`
	DiscoveryEndpoint                   *string     `json:"discoveryEndpoint"`
	LoginEnabled                        *bool       `json:"loginEnabled"`
	Prompt                              *OIDCPrompt `json:"prompt"`
	AuthenticationContextClassReference *[]string   `json:"authenticationContextClassReference"`
	AdditionalScopes                    *string     `json:"additionalScopes"`
	EmailVerifiedRequired               *bool       `json:"emailVerifiedRequired"`
	RPInitiatedLogoutEnabled            *bool       `json:"rpInitiatedLogoutEnabled"`
}

// Validate enforces full-replacement fields and documented value constraints.
func (r SetOIDCConfigurationRequest) Validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{"clientId", r.ClientID != nil},
		{"clientSecret", r.ClientSecret != nil},
		{"discoveryEndpoint", r.DiscoveryEndpoint != nil},
		{"loginEnabled", r.LoginEnabled != nil},
		{"prompt", r.Prompt != nil},
		{"authenticationContextClassReference", r.AuthenticationContextClassReference != nil},
		{"additionalScopes", r.AdditionalScopes != nil},
		{"emailVerifiedRequired", r.EmailVerifiedRequired != nil},
		{"rpInitiatedLogoutEnabled", r.RPInitiatedLogoutEnabled != nil},
	}
	for _, field := range required {
		if !field.present {
			return fmt.Errorf("%s is required in the full OIDC replacement", field.name)
		}
	}
	if *r.ClientID == "" {
		return fmt.Errorf("clientId must not be empty")
	}
	if *r.ClientSecret == "" {
		return fmt.Errorf("clientSecret must not be empty; use the redacted sentinel to preserve the stored secret")
	}
	endpoint, err := url.Parse(*r.DiscoveryEndpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return fmt.Errorf("discoveryEndpoint must be an http:// or https:// URL with a host")
	}
	switch *r.Prompt {
	case OIDCPromptNone, OIDCPromptLogin, OIDCPromptConsent, OIDCPromptSelectAccount, OIDCPromptCreate:
		return nil
	default:
		return fmt.Errorf("prompt must be one of none, login, consent, select_account, or create")
	}
}

// GetOIDCConfiguration returns the effective OIDC configuration with the client
// secret represented only by n8n's redacted placeholder.
func (c *Client) GetOIDCConfiguration(ctx context.Context) (*OIDCConfiguration, error) {
	var configuration OIDCConfiguration
	if _, err := c.Do(ctx, Request{Path: OIDCSettingsPath}, &configuration); err != nil {
		return nil, redactOIDCAPIError(err)
	}
	configuration.redactClientSecret()
	return &configuration, nil
}

// SetOIDCConfiguration fully replaces the OIDC configuration. Sending
// OIDCClientSecretRedactedValue preserves the client secret stored by n8n.
func (c *Client) SetOIDCConfiguration(ctx context.Context, request SetOIDCConfigurationRequest) (*OIDCConfiguration, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var configuration OIDCConfiguration
	if _, err := c.Do(ctx, Request{Method: http.MethodPut, Path: OIDCSettingsPath, Body: request}, &configuration); err != nil {
		return nil, redactOIDCAPIError(err)
	}
	configuration.redactClientSecret()
	return &configuration, nil
}

// redactOIDCAPIError prevents server-controlled error text from exposing a
// submitted or stored client secret while preserving HTTP status classification.
func redactOIDCAPIError(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "OIDC configuration request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
