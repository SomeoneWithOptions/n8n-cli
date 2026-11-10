package n8n

import (
	"context"
	"fmt"
	"net/http"
)

// RateLimitSettingsPath is the Public API rate-limit settings endpoint (n8n
// 2.47 and later).
const RateLimitSettingsPath = "/settings/rate-limits"

// RateLimitSettings is the effective Public API rate-limit configuration. A
// limit of 0 means no limit; windows are in seconds. EnvManagedFields lists
// the fields environment variables control: the API reports their effective
// values and refuses to change them.
type RateLimitSettings struct {
	Enabled                    bool     `json:"enabled"`
	GlobalReadLimit            int      `json:"globalReadLimit"`
	GlobalReadWindowSeconds    int      `json:"globalReadWindowSeconds"`
	GlobalWriteLimit           int      `json:"globalWriteLimit"`
	GlobalWriteWindowSeconds   int      `json:"globalWriteWindowSeconds"`
	EndpointReadLimit          int      `json:"endpointReadLimit"`
	EndpointReadWindowSeconds  int      `json:"endpointReadWindowSeconds"`
	EndpointWriteLimit         int      `json:"endpointWriteLimit"`
	EndpointWriteWindowSeconds int      `json:"endpointWriteWindowSeconds"`
	EnvManagedFields           []string `json:"envManagedFields"`
}

// UpdateRateLimitSettingsRequest is a full replacement document: every field
// is required. Pointers distinguish an omitted field from false or 0.
// envManagedFields is read-only and has no field here, so it is never sent.
type UpdateRateLimitSettingsRequest struct {
	Enabled                    *bool `json:"enabled"`
	GlobalReadLimit            *int  `json:"globalReadLimit"`
	GlobalReadWindowSeconds    *int  `json:"globalReadWindowSeconds"`
	GlobalWriteLimit           *int  `json:"globalWriteLimit"`
	GlobalWriteWindowSeconds   *int  `json:"globalWriteWindowSeconds"`
	EndpointReadLimit          *int  `json:"endpointReadLimit"`
	EndpointReadWindowSeconds  *int  `json:"endpointReadWindowSeconds"`
	EndpointWriteLimit         *int  `json:"endpointWriteLimit"`
	EndpointWriteWindowSeconds *int  `json:"endpointWriteWindowSeconds"`
}

// RateLimitField is one writable rate-limit field: its JSON name and, for the
// integer fields, whether it is a window (seconds, at least 1) rather than a
// limit (at least 0, 0 for no limit).
type RateLimitField struct {
	Name   string
	Window bool
}

// RateLimitIntFields lists the eight integer fields in document order.
var RateLimitIntFields = []RateLimitField{
	{Name: "globalReadLimit"},
	{Name: "globalReadWindowSeconds", Window: true},
	{Name: "globalWriteLimit"},
	{Name: "globalWriteWindowSeconds", Window: true},
	{Name: "endpointReadLimit"},
	{Name: "endpointReadWindowSeconds", Window: true},
	{Name: "endpointWriteLimit"},
	{Name: "endpointWriteWindowSeconds", Window: true},
}

// Int returns a pointer to the integer field named name, or nil when name is
// not one of [RateLimitIntFields].
func (r *UpdateRateLimitSettingsRequest) Int(name string) **int {
	switch name {
	case "globalReadLimit":
		return &r.GlobalReadLimit
	case "globalReadWindowSeconds":
		return &r.GlobalReadWindowSeconds
	case "globalWriteLimit":
		return &r.GlobalWriteLimit
	case "globalWriteWindowSeconds":
		return &r.GlobalWriteWindowSeconds
	case "endpointReadLimit":
		return &r.EndpointReadLimit
	case "endpointReadWindowSeconds":
		return &r.EndpointReadWindowSeconds
	case "endpointWriteLimit":
		return &r.EndpointWriteLimit
	case "endpointWriteWindowSeconds":
		return &r.EndpointWriteWindowSeconds
	}
	return nil
}

// Validate enforces the replacement rules before transport: all nine fields
// present, limits at least 0, windows at least 1 second, and at least one
// limit above 0 when rate limiting is enabled.
func (r UpdateRateLimitSettingsRequest) Validate() error {
	if r.Enabled == nil {
		return fmt.Errorf("enabled is required and must be true or false")
	}
	anyLimit := false
	for _, field := range RateLimitIntFields {
		value := *r.Int(field.Name)
		switch {
		case value == nil && field.Window:
			return fmt.Errorf("%s is required: a window in seconds, at least 1", field.Name)
		case value == nil:
			return fmt.Errorf("%s is required: a request count, at least 0 (0 for no limit)", field.Name)
		case field.Window && *value < 1:
			return fmt.Errorf("%s must be at least 1 second, got %d", field.Name, *value)
		case !field.Window && *value < 0:
			return fmt.Errorf("%s must be at least 0 (0 for no limit), got %d", field.Name, *value)
		case !field.Window && *value > 0:
			anyLimit = true
		}
	}
	if *r.Enabled && !anyLimit {
		return fmt.Errorf("rate limiting is enabled but every limit is 0: set at least one of globalReadLimit, globalWriteLimit, endpointReadLimit, or endpointWriteLimit above 0, or set enabled to false")
	}
	return nil
}

// GetRateLimitSettings returns the effective Public API rate-limit settings
// and the fields environment variables manage.
func (c *Client) GetRateLimitSettings(ctx context.Context) (*RateLimitSettings, error) {
	var settings RateLimitSettings
	if _, err := c.Do(ctx, Request{Path: RateLimitSettingsPath}, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpdateRateLimitSettings fully replaces the Public API rate-limit settings.
// The new limits apply to every API key, this one included.
func (c *Client) UpdateRateLimitSettings(ctx context.Context, request UpdateRateLimitSettingsRequest) (*RateLimitSettings, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var settings RateLimitSettings
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   RateLimitSettingsPath,
		Body:   request,
	}, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}
