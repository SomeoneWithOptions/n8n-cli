package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	// SAMLSettingsPath is the instance-wide SAML SSO configuration endpoint.
	SAMLSettingsPath = "/settings/sso/saml"

	// SAMLRedactedValue is n8n's placeholder for stored SAML private keys,
	// certificates, and identity-provider metadata. Sending it back preserves
	// the corresponding stored value.
	SAMLRedactedValue = "**hidden**"
)

// SAMLBinding controls the SAML transport binding.
type SAMLBinding string

const (
	SAMLBindingRedirect SAMLBinding = "redirect"
	SAMLBindingPost     SAMLBinding = "post"
)

// SAMLSignatureAction controls where the XML signature is inserted.
type SAMLSignatureAction string

const (
	SAMLSignatureBefore  SAMLSignatureAction = "before"
	SAMLSignatureAfter   SAMLSignatureAction = "after"
	SAMLSignaturePrepend SAMLSignatureAction = "prepend"
	SAMLSignatureAppend  SAMLSignatureAction = "append"
)

// SAMLMapping maps identity-provider attributes to n8n user fields.
type SAMLMapping struct {
	Email             string   `json:"email"`
	FirstName         string   `json:"firstName"`
	LastName          string   `json:"lastName"`
	UserPrincipalName string   `json:"userPrincipalName"`
	N8nInstanceRole   string   `json:"n8nInstanceRole"`
	N8nProjectRoles   []string `json:"n8nProjectRoles"`
}

// SAMLSignatureLocation describes where the XML signature is inserted.
type SAMLSignatureLocation struct {
	Reference string              `json:"reference"`
	Action    SAMLSignatureAction `json:"action"`
}

// SAMLSignatureConfig configures XML signatures in SAML messages.
type SAMLSignatureConfig struct {
	Prefix   string                `json:"prefix"`
	Location SAMLSignatureLocation `json:"location"`
}

// SAMLConfiguration is the effective instance-wide SAML SSO configuration.
// Secret-bearing fields are always empty or SAMLRedactedValue after decoding,
// even if a non-conforming server returns plaintext.
type SAMLConfiguration struct {
	EntityID             string              `json:"entityID"`
	ReturnURL            string              `json:"returnUrl"`
	Mapping              SAMLMapping         `json:"mapping"`
	Metadata             string              `json:"metadata"`
	MetadataURL          string              `json:"metadataUrl"`
	IgnoreSSL            bool                `json:"ignoreSSL"`
	LoginBinding         SAMLBinding         `json:"loginBinding"`
	LoginEnabled         bool                `json:"loginEnabled"`
	LoginLabel           string              `json:"loginLabel"`
	AuthnRequestsSigned  bool                `json:"authnRequestsSigned"`
	WantAssertionsSigned bool                `json:"wantAssertionsSigned"`
	WantMessageSigned    bool                `json:"wantMessageSigned"`
	SigningPrivateKey    string              `json:"signingPrivateKey"`
	SigningCertificate   string              `json:"signingCertificate"`
	ACSBinding           SAMLBinding         `json:"acsBinding"`
	SignatureConfig      SAMLSignatureConfig `json:"signatureConfig"`
	RelayState           string              `json:"relayState"`
}

func (c *SAMLConfiguration) redactSecrets() {
	for _, secret := range []*string{&c.Metadata, &c.SigningPrivateKey, &c.SigningCertificate} {
		if *secret != "" {
			*secret = SAMLRedactedValue
		}
	}
}

// SetSAMLMapping is a full replacement mapping. Pointers distinguish omitted
// required fields from valid empty strings and arrays.
type SetSAMLMapping struct {
	Email             *string   `json:"email"`
	FirstName         *string   `json:"firstName"`
	LastName          *string   `json:"lastName"`
	UserPrincipalName *string   `json:"userPrincipalName"`
	N8nInstanceRole   *string   `json:"n8nInstanceRole"`
	N8nProjectRoles   *[]string `json:"n8nProjectRoles"`
}

// SetSAMLSignatureLocation is a full replacement signature location.
type SetSAMLSignatureLocation struct {
	Reference *string              `json:"reference"`
	Action    *SAMLSignatureAction `json:"action"`
}

// SetSAMLSignatureConfig is a full replacement signature configuration.
type SetSAMLSignatureConfig struct {
	Prefix   *string                   `json:"prefix"`
	Location *SetSAMLSignatureLocation `json:"location"`
}

// SetSAMLConfigurationRequest is a full replacement document. EntityID and
// ReturnURL are accepted so a GET document can be read directly, but are never
// sent because they are server-computed, read-only values.
type SetSAMLConfigurationRequest struct {
	EntityID             *string                 `json:"entityID,omitempty"`
	ReturnURL            *string                 `json:"returnUrl,omitempty"`
	Mapping              *SetSAMLMapping         `json:"mapping"`
	Metadata             *string                 `json:"metadata"`
	MetadataURL          *string                 `json:"metadataUrl"`
	IgnoreSSL            *bool                   `json:"ignoreSSL"`
	LoginBinding         *SAMLBinding            `json:"loginBinding"`
	LoginEnabled         *bool                   `json:"loginEnabled"`
	LoginLabel           *string                 `json:"loginLabel"`
	AuthnRequestsSigned  *bool                   `json:"authnRequestsSigned"`
	WantAssertionsSigned *bool                   `json:"wantAssertionsSigned"`
	WantMessageSigned    *bool                   `json:"wantMessageSigned"`
	SigningPrivateKey    *string                 `json:"signingPrivateKey"`
	SigningCertificate   *string                 `json:"signingCertificate"`
	ACSBinding           *SAMLBinding            `json:"acsBinding"`
	SignatureConfig      *SetSAMLSignatureConfig `json:"signatureConfig"`
	RelayState           *string                 `json:"relayState"`
}

// Validate enforces every writable full-replacement field and documented enum.
func (r SetSAMLConfigurationRequest) Validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{"mapping", r.Mapping != nil},
		{"metadata", r.Metadata != nil},
		{"metadataUrl", r.MetadataURL != nil},
		{"ignoreSSL", r.IgnoreSSL != nil},
		{"loginBinding", r.LoginBinding != nil},
		{"loginEnabled", r.LoginEnabled != nil},
		{"loginLabel", r.LoginLabel != nil},
		{"authnRequestsSigned", r.AuthnRequestsSigned != nil},
		{"wantAssertionsSigned", r.WantAssertionsSigned != nil},
		{"wantMessageSigned", r.WantMessageSigned != nil},
		{"signingPrivateKey", r.SigningPrivateKey != nil},
		{"signingCertificate", r.SigningCertificate != nil},
		{"acsBinding", r.ACSBinding != nil},
		{"signatureConfig", r.SignatureConfig != nil},
		{"relayState", r.RelayState != nil},
	}
	for _, field := range required {
		if !field.present {
			return fmt.Errorf("%s is required in the full SAML replacement", field.name)
		}
	}
	if err := r.Mapping.validate(); err != nil {
		return err
	}
	if err := validateSAMLBinding("loginBinding", *r.LoginBinding); err != nil {
		return err
	}
	if err := validateSAMLBinding("acsBinding", *r.ACSBinding); err != nil {
		return err
	}
	return r.SignatureConfig.validate()
}

func (m *SetSAMLMapping) validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{"mapping.email", m.Email != nil},
		{"mapping.firstName", m.FirstName != nil},
		{"mapping.lastName", m.LastName != nil},
		{"mapping.userPrincipalName", m.UserPrincipalName != nil},
		{"mapping.n8nInstanceRole", m.N8nInstanceRole != nil},
		{"mapping.n8nProjectRoles", m.N8nProjectRoles != nil},
	}
	for _, field := range required {
		if !field.present {
			return fmt.Errorf("%s is required in the full SAML replacement", field.name)
		}
	}
	if *m.N8nProjectRoles == nil {
		return fmt.Errorf("mapping.n8nProjectRoles must be an array; use [] when empty")
	}
	return nil
}

func (c *SetSAMLSignatureConfig) validate() error {
	if c.Prefix == nil {
		return fmt.Errorf("signatureConfig.prefix is required in the full SAML replacement")
	}
	if c.Location == nil {
		return fmt.Errorf("signatureConfig.location is required in the full SAML replacement")
	}
	if c.Location.Reference == nil {
		return fmt.Errorf("signatureConfig.location.reference is required in the full SAML replacement")
	}
	if c.Location.Action == nil {
		return fmt.Errorf("signatureConfig.location.action is required in the full SAML replacement")
	}
	switch *c.Location.Action {
	case SAMLSignatureBefore, SAMLSignatureAfter, SAMLSignaturePrepend, SAMLSignatureAppend:
		return nil
	default:
		return fmt.Errorf("signatureConfig.location.action must be one of before, after, prepend, or append")
	}
}

func validateSAMLBinding(name string, binding SAMLBinding) error {
	switch binding {
	case SAMLBindingRedirect, SAMLBindingPost:
		return nil
	default:
		return fmt.Errorf("%s must be redirect or post", name)
	}
}

type samlConfigurationUpdate struct {
	Mapping              *SetSAMLMapping         `json:"mapping"`
	Metadata             *string                 `json:"metadata"`
	MetadataURL          *string                 `json:"metadataUrl"`
	IgnoreSSL            *bool                   `json:"ignoreSSL"`
	LoginBinding         *SAMLBinding            `json:"loginBinding"`
	LoginEnabled         *bool                   `json:"loginEnabled"`
	LoginLabel           *string                 `json:"loginLabel"`
	AuthnRequestsSigned  *bool                   `json:"authnRequestsSigned"`
	WantAssertionsSigned *bool                   `json:"wantAssertionsSigned"`
	WantMessageSigned    *bool                   `json:"wantMessageSigned"`
	SigningPrivateKey    *string                 `json:"signingPrivateKey"`
	SigningCertificate   *string                 `json:"signingCertificate"`
	ACSBinding           *SAMLBinding            `json:"acsBinding"`
	SignatureConfig      *SetSAMLSignatureConfig `json:"signatureConfig"`
	RelayState           *string                 `json:"relayState"`
}

func (r SetSAMLConfigurationRequest) update() samlConfigurationUpdate {
	return samlConfigurationUpdate{
		Mapping: r.Mapping, Metadata: r.Metadata, MetadataURL: r.MetadataURL,
		IgnoreSSL: r.IgnoreSSL, LoginBinding: r.LoginBinding, LoginEnabled: r.LoginEnabled,
		LoginLabel: r.LoginLabel, AuthnRequestsSigned: r.AuthnRequestsSigned,
		WantAssertionsSigned: r.WantAssertionsSigned, WantMessageSigned: r.WantMessageSigned,
		SigningPrivateKey: r.SigningPrivateKey, SigningCertificate: r.SigningCertificate,
		ACSBinding: r.ACSBinding, SignatureConfig: r.SignatureConfig, RelayState: r.RelayState,
	}
}

// GetSAMLConfiguration returns the effective SAML configuration with all
// secret-bearing values represented only by n8n's redacted placeholder.
func (c *Client) GetSAMLConfiguration(ctx context.Context) (*SAMLConfiguration, error) {
	var configuration SAMLConfiguration
	if _, err := c.Do(ctx, Request{Path: SAMLSettingsPath}, &configuration); err != nil {
		return nil, redactSAMLAPIError(err)
	}
	configuration.redactSecrets()
	return &configuration, nil
}

// SetSAMLConfiguration fully replaces writable SAML configuration. Read-only
// entity and return URLs are discarded. Redacted placeholders preserve stored
// values.
func (c *Client) SetSAMLConfiguration(ctx context.Context, request SetSAMLConfigurationRequest) (*SAMLConfiguration, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var configuration SAMLConfiguration
	if _, err := c.Do(ctx, Request{Method: http.MethodPut, Path: SAMLSettingsPath, Body: request.update()}, &configuration); err != nil {
		return nil, redactSAMLAPIError(err)
	}
	configuration.redactSecrets()
	return &configuration, nil
}

// redactSAMLAPIError prevents server-controlled text from exposing submitted
// or stored SAML secrets while preserving HTTP status classification.
func redactSAMLAPIError(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "SAML configuration request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}
