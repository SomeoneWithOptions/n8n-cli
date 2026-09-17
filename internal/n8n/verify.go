package n8n

import (
	"context"
	"fmt"
	"net/http"
)

// DiscoverPath reports the capabilities and scopes available to the caller. It
// is the cheapest authenticated GET in the API, which makes it the credential
// check used by login. Phase 3 adds the typed command on top of the same path.
const DiscoverPath = "/discover"

// Verify sends one authenticated request and reports whether the credential is
// accepted. A 401 means missing, malformed, expired or revoked; a 403 means the
// credential is valid but lacks the scope.
func (c *Client) Verify(ctx context.Context) error {
	_, err := c.Do(ctx, Request{Method: http.MethodGet, Path: DiscoverPath}, nil)
	return err
}

// ErrAPIKeyRequired is returned for operations the API restricts to API-key
// authentication: the CommunityPackage and DataTable groups reject a bearer
// token or session cookie before doing any work.
type ErrAPIKeyRequired struct{ Actual AuthType }

func (e *ErrAPIKeyRequired) Error() string {
	actual := string(e.Actual)
	if actual == "" {
		actual = "no credential"
	}
	return fmt.Sprintf("this command requires API key authentication, but the current context uses %s", actual)
}

// RequireAPIKey rejects a non-API-key credential before a request is sent, so
// the user gets a usable message instead of the API's 401.
func RequireAPIKey(t AuthType) error {
	if t == AuthAPIKey {
		return nil
	}
	return &ErrAPIKeyRequired{Actual: t}
}
