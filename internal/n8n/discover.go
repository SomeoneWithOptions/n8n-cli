package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// DiscoverIncludeSchemas is the only value the API accepts for the `include`
// query parameter. It inlines each endpoint's request body schema, which is
// what lets a caller avoid fetching the whole OpenAPI document.
const DiscoverIncludeSchemas = "schemas"

// DiscoverOptions are the query filters of GET /discover. Every field is
// optional; the zero value asks for the full capability map.
type DiscoverOptions struct {
	// Include requests extra data. Only [DiscoverIncludeSchemas] is defined.
	Include string
	// Resource narrows the response to one resource key, e.g. "workflow".
	Resource string
	// Operation narrows the response to endpoints with one operation, e.g. "read".
	Operation string
}

// Validate rejects an include value the API does not define, so a typo fails
// before a request instead of silently returning an unfiltered map.
func (o DiscoverOptions) Validate() error {
	if o.Include != "" && o.Include != DiscoverIncludeSchemas {
		return fmt.Errorf("unknown include %q: the API defines only %q", o.Include, DiscoverIncludeSchemas)
	}
	return nil
}

// Apply writes the options into q, creating it when nil, and returns it.
func (o DiscoverOptions) Apply(q url.Values) url.Values {
	if q == nil {
		q = url.Values{}
	}
	for name, value := range map[string]string{
		"include":   o.Include,
		"resource":  o.Resource,
		"operation": o.Operation,
	} {
		if value != "" {
			q.Set(name, value)
		}
	}
	return q
}

// Discovery is the capability map GET /discover returns for one credential.
// It reflects what that credential may do, not what the instance supports in
// general: a missing resource means "not available to you", which covers an
// unlicensed feature and an unscoped key alike.
type Discovery struct {
	// Scopes are the credential's active scopes, e.g. "workflow:read".
	Scopes []string `json:"scopes"`
	// Resources is keyed by resource name, e.g. "workflow" or "datatable".
	Resources map[string]DiscoveredResource `json:"resources"`
	// Filters describes the query parameters this endpoint accepts and the
	// values the caller's scopes permit.
	Filters map[string]DiscoverFilter `json:"filters"`
	// SpecURL points at the full OpenAPI document, usually /api/v1/openapi.yml.
	SpecURL string `json:"specUrl"`
}

// DiscoveredResource is one resource group of the capability map.
type DiscoveredResource struct {
	Operations []string             `json:"operations"`
	Endpoints  []DiscoveredEndpoint `json:"endpoints"`
}

// DiscoveredEndpoint is one operation of a resource group.
type DiscoveredEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	OperationID string `json:"operationId"`
	// RequestSchema is set only when the request asked for
	// [DiscoverIncludeSchemas] and the endpoint takes a body. It is kept raw:
	// it is an arbitrary JSON Schema, and the CLI only ever passes it through.
	RequestSchema json.RawMessage `json:"requestSchema,omitempty"`
}

// DiscoverFilter is one query parameter the endpoint accepts.
type DiscoverFilter struct {
	Description string   `json:"description"`
	Values      []string `json:"values"`
}

// discoverEnvelope is the API's {"data": ...} wrapper.
type discoverEnvelope struct {
	Data Discovery `json:"data"`
}

// Discover reports the API capabilities and scopes available to this client's
// credential. It requires no scope, which is why login uses the same path to
// validate a credential; see [Client.Verify].
func (c *Client) Discover(ctx context.Context, opts DiscoverOptions) (*Discovery, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	var envelope discoverEnvelope
	if _, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   DiscoverPath,
		Query:  opts.Apply(nil),
	}, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}

// HasScope reports whether the credential holds an exact scope, e.g.
// "workflow:read".
func (d *Discovery) HasScope(scope string) bool {
	for _, s := range d.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Resource looks a resource group up by name, case-insensitively, because the
// API keys them in lowercase ("datatable") while commands are named for
// readability ("data-table").
func (d *Discovery) Resource(name string) (DiscoveredResource, bool) {
	if r, ok := d.Resources[name]; ok {
		return r, true
	}
	folded := strings.ToLower(strings.ReplaceAll(name, "-", ""))
	for key, r := range d.Resources {
		if strings.ToLower(key) == folded {
			return r, true
		}
	}
	return DiscoveredResource{}, false
}

// ResourceNames returns the resource keys in sorted order, so output and tests
// do not depend on Go's map iteration order.
func (d *Discovery) ResourceNames() []string {
	names := make([]string, 0, len(d.Resources))
	for name := range d.Resources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Endpoint finds an endpoint by method and path, ignoring method case. Paths
// are compared as returned, including the /api/v1 prefix.
func (d *Discovery) Endpoint(method, path string) (DiscoveredEndpoint, bool) {
	for _, name := range d.ResourceNames() {
		for _, e := range d.Resources[name].Endpoints {
			if strings.EqualFold(e.Method, method) && e.Path == path {
				return e, true
			}
		}
	}
	return DiscoveredEndpoint{}, false
}

// HasOperation reports whether a resource exposes an operation, e.g.
// HasOperation("workflow", "create"). Later phases use it to tell "you lack the
// scope" apart from "this instance does not have the feature".
func (d *Discovery) HasOperation(resource, operation string) bool {
	r, ok := d.Resource(resource)
	if !ok {
		return false
	}
	for _, op := range r.Operations {
		if op == operation {
			return true
		}
	}
	return false
}
