package n8n

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// CommunityPackagesPath is the collection endpoint for installed community
// node packages.
const CommunityPackagesPath = "/community-packages"

// CommunityPackage is one npm community package installed on the instance.
type CommunityPackage struct {
	PackageName      string                 `json:"packageName"`
	InstalledVersion string                 `json:"installedVersion"`
	AuthorName       string                 `json:"authorName,omitempty"`
	AuthorEmail      string                 `json:"authorEmail,omitempty"`
	InstalledNodes   []CommunityPackageNode `json:"installedNodes,omitempty"`
	CreatedAt        string                 `json:"createdAt,omitempty"`
	UpdatedAt        string                 `json:"updatedAt,omitempty"`
	UpdateAvailable  string                 `json:"updateAvailable,omitempty"`
	FailedLoading    bool                   `json:"failedLoading"`
}

// CommunityPackageNode is one node type supplied by a community package.
type CommunityPackageNode struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	LatestVersion float64 `json:"latestVersion"`
}

// InstallCommunityPackageRequest selects the npm package and version to
// install. The API no longer accepts a verify field: the instance setting for
// unverified packages decides which versions may be installed.
type InstallCommunityPackageRequest struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Validate checks the required package name before a request is sent.
func (r InstallCommunityPackageRequest) Validate() error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return fmt.Errorf("community package name is required")
	}
	if name != r.Name {
		return fmt.Errorf("community package name must not start or end with whitespace")
	}
	if !validCommunityPackageName(name) {
		return fmt.Errorf("community package name %q must have the form n8n-nodes-<name> or @<scope>/n8n-nodes-<name>", name)
	}
	if strings.TrimSpace(r.Version) != r.Version {
		return fmt.Errorf("community package version must not start or end with whitespace")
	}
	return nil
}

// UpdateCommunityPackageRequest selects an optional target version. The API
// declares the body optional, so its zero value sends no body and n8n updates
// to the latest version the instance setting for unverified packages allows.
type UpdateCommunityPackageRequest struct {
	Version string `json:"version,omitempty"`
}

// Validate checks update options before a request is sent.
func (r UpdateCommunityPackageRequest) Validate() error {
	if strings.TrimSpace(r.Version) != r.Version {
		return fmt.Errorf("community package version must not start or end with whitespace")
	}
	return nil
}

func (r UpdateCommunityPackageRequest) empty() bool {
	return r.Version == ""
}

// validCommunityPackageName accepts the two npm name forms n8n installs:
// unscoped n8n-nodes-<name> and scoped @<scope>/n8n-nodes-<name>.
func validCommunityPackageName(name string) bool {
	if scoped, ok := strings.CutPrefix(name, "@"); ok {
		scope, rest, found := strings.Cut(scoped, "/")
		if !found || scope == "" || strings.Contains(rest, "/") {
			return false
		}
		name = rest
	}
	return strings.HasPrefix(name, "n8n-nodes-") && len(name) > len("n8n-nodes-")
}

// Bool returns a pointer to value for optional boolean request fields.
func Bool(value bool) *bool { return &value }

// ListCommunityPackages returns every community package installed on the
// instance. This API group accepts API-key authentication only.
func (c *Client) ListCommunityPackages(ctx context.Context) ([]CommunityPackage, error) {
	var packages []CommunityPackage
	if _, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   CommunityPackagesPath,
	}, &packages); err != nil {
		return nil, err
	}
	return packages, nil
}

// InstallCommunityPackage installs one npm community package. Installing code
// changes the instance and should be guarded by confirmation at the CLI layer.
func (c *Client) InstallCommunityPackage(ctx context.Context, request InstallCommunityPackageRequest) (*CommunityPackage, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var installed CommunityPackage
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   CommunityPackagesPath,
		Body:   request,
	}, &installed); err != nil {
		return nil, err
	}
	return &installed, nil
}

// UpdateCommunityPackage updates one installed package. name is escaped as one
// path segment, including scoped npm names containing a slash.
func (c *Client) UpdateCommunityPackage(ctx context.Context, name string, request UpdateCommunityPackageRequest) (*CommunityPackage, error) {
	if err := validateInstalledCommunityPackageName(name); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var body any
	if !request.empty() {
		body = request
	}
	var updated CommunityPackage
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("community-packages", name),
		Body:   body,
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// UninstallCommunityPackage removes one installed package. A successful 204
// response has no body.
func (c *Client) UninstallCommunityPackage(ctx context.Context, name string) error {
	if err := validateInstalledCommunityPackageName(name); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("community-packages", name),
	}, nil)
	return err
}

func validateInstalledCommunityPackageName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("community package name is required")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("community package name must not start or end with whitespace")
	}
	return nil
}
