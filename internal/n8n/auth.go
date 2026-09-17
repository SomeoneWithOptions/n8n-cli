package n8n

import (
	"errors"
	"log/slog"
	"net/http"
)

// Redacted replaces every secret value that reaches a human-readable surface.
const Redacted = "[REDACTED]"

// Secret is credential material. Its String, GoString, LogValue and MarshalJSON
// methods all render [Redacted], so a secret cannot leak through fmt verbs,
// slog attributes, %#v dumps or an accidental json.Marshal of a struct that
// holds one. Persisting a secret is deliberate: call [Secret.Reveal].
type Secret string

// Reveal returns the underlying value. Every call site is a leak candidate;
// keep them at the transport and storage boundaries only.
func (s Secret) Reveal() string { return string(s) }

// Empty reports whether the secret holds no value.
func (s Secret) Empty() bool { return string(s) == "" }

func (s Secret) String() string { return Redacted }

func (s Secret) GoString() string { return `"` + Redacted + `"` }

func (s Secret) LogValue() slog.Value { return slog.StringValue(Redacted) }

// MarshalJSON redacts. Storage code must marshal [Secret.Reveal] explicitly.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + Redacted + `"`), nil }

// AuthType names an authentication mechanism supported by the n8n API.
type AuthType string

const (
	AuthAPIKey AuthType = "api-key"
	AuthBearer AuthType = "bearer"
	AuthCookie AuthType = "cookie"
)

// Header and cookie names used by the API.
const (
	HeaderAPIKey = "X-N8N-API-KEY"
	CookieName   = "n8n-auth"
)

// ErrEmptyCredential is returned when an authenticator is built without a value.
var ErrEmptyCredential = errors.New("credential is empty")

// Authenticator injects exactly one credential into an outgoing request.
type Authenticator interface {
	// Apply sets this authenticator's credential and touches no other
	// authentication header, so a request carries one mechanism only.
	Apply(*http.Request)
	// Type reports the mechanism, for diagnostics and for commands that the
	// API restricts to API-key authentication.
	Type() AuthType
}

// APIKeyAuth sends X-N8N-API-KEY. This is the documented automation path and
// the only mechanism accepted by the CommunityPackage and DataTable groups.
type APIKeyAuth struct{ key Secret }

// NewAPIKeyAuth builds an API key authenticator.
func NewAPIKeyAuth(key Secret) (*APIKeyAuth, error) {
	if key.Empty() {
		return nil, ErrEmptyCredential
	}
	return &APIKeyAuth{key: key}, nil
}

func (a *APIKeyAuth) Apply(r *http.Request) { r.Header.Set(HeaderAPIKey, a.key.Reveal()) }

func (a *APIKeyAuth) Type() AuthType { return AuthAPIKey }

func (a *APIKeyAuth) LogValue() slog.Value { return authLogValue(AuthAPIKey) }

// BearerAuth sends Authorization: Bearer <jwt>.
type BearerAuth struct{ token Secret }

// NewBearerAuth builds a bearer token authenticator.
func NewBearerAuth(token Secret) (*BearerAuth, error) {
	if token.Empty() {
		return nil, ErrEmptyCredential
	}
	return &BearerAuth{token: token}, nil
}

func (a *BearerAuth) Apply(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+a.token.Reveal())
}

func (a *BearerAuth) Type() AuthType { return AuthBearer }

func (a *BearerAuth) LogValue() slog.Value { return authLogValue(AuthBearer) }

// CookieAuth sends the n8n-auth browser session cookie.
type CookieAuth struct{ value Secret }

// NewCookieAuth builds an n8n-auth cookie authenticator.
func NewCookieAuth(value Secret) (*CookieAuth, error) {
	if value.Empty() {
		return nil, ErrEmptyCredential
	}
	return &CookieAuth{value: value}, nil
}

func (a *CookieAuth) Apply(r *http.Request) {
	r.AddCookie(&http.Cookie{Name: CookieName, Value: a.value.Reveal()})
}

func (a *CookieAuth) Type() AuthType { return AuthCookie }

func (a *CookieAuth) LogValue() slog.Value { return authLogValue(AuthCookie) }

func authLogValue(t AuthType) slog.Value {
	return slog.GroupValue(slog.String("type", string(t)), slog.String("credential", Redacted))
}
