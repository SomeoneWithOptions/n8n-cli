package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const rateLimitResponse = `{
  "enabled":true,
  "globalReadLimit":600,
  "globalReadWindowSeconds":60,
  "globalWriteLimit":120,
  "globalWriteWindowSeconds":60,
  "endpointReadLimit":0,
  "endpointReadWindowSeconds":60,
  "endpointWriteLimit":30,
  "endpointWriteWindowSeconds":10,
  "envManagedFields":["globalReadLimit"]
}`

func validRateLimitRequest() UpdateRateLimitSettingsRequest {
	ints := []int{600, 60, 120, 60, 0, 60, 30, 10}
	request := UpdateRateLimitSettingsRequest{Enabled: Bool(true)}
	for i, field := range RateLimitIntFields {
		value := ints[i]
		*request.Int(field.Name) = &value
	}
	return request
}

func TestGetRateLimitSettings(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, rateLimitResponse))
	settings, err := server.client(t).GetRateLimitSettings(context.Background())
	if err != nil {
		t.Fatalf("GetRateLimitSettings: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+RateLimitSettingsPath || req.URL.RawQuery != "" || body != "" {
		t.Errorf("request = %s %s?%s body %q", req.Method, req.URL.EscapedPath(), req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !settings.Enabled || settings.GlobalReadLimit != 600 || settings.GlobalWriteLimit != 120 ||
		settings.EndpointReadLimit != 0 || settings.EndpointWriteLimit != 30 || settings.EndpointWriteWindowSeconds != 10 ||
		len(settings.EnvManagedFields) != 1 || settings.EnvManagedFields[0] != "globalReadLimit" {
		t.Errorf("settings = %+v", settings)
	}
}

func TestUpdateRateLimitSettingsSendsAllWritableFieldsOnly(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, rateLimitResponse))
	settings, err := server.client(t).UpdateRateLimitSettings(context.Background(), validRateLimitRequest())
	if err != nil {
		t.Fatalf("UpdateRateLimitSettings: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+RateLimitSettingsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	if len(sent) != 9 {
		t.Errorf("sent %d fields, want the 9 writable ones: %s", len(sent), body)
	}
	if _, ok := sent["envManagedFields"]; ok {
		t.Error("read-only envManagedFields was sent")
	}
	if sent["enabled"] != true || sent["endpointReadLimit"] != float64(0) || sent["endpointWriteWindowSeconds"] != float64(10) {
		t.Errorf("body = %s", body)
	}
	if settings.GlobalReadWindowSeconds != 60 {
		t.Errorf("settings = %+v", settings)
	}
}

func TestUpdateRateLimitSettingsValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*UpdateRateLimitSettingsRequest)
		want   string
	}{
		{"missing enabled", func(r *UpdateRateLimitSettingsRequest) { r.Enabled = nil }, "enabled is required"},
		{"missing limit", func(r *UpdateRateLimitSettingsRequest) { r.GlobalWriteLimit = nil }, "globalWriteLimit is required"},
		{"missing window", func(r *UpdateRateLimitSettingsRequest) { r.EndpointReadWindowSeconds = nil }, "endpointReadWindowSeconds is required"},
		{"negative limit", func(r *UpdateRateLimitSettingsRequest) { v := -1; r.EndpointWriteLimit = &v }, "endpointWriteLimit must be at least 0"},
		{"zero window", func(r *UpdateRateLimitSettingsRequest) { v := 0; r.GlobalReadWindowSeconds = &v }, "globalReadWindowSeconds must be at least 1"},
		{"enabled without limits", func(r *UpdateRateLimitSettingsRequest) {
			for _, field := range RateLimitIntFields {
				if !field.Window {
					zero := 0
					*r.Int(field.Name) = &zero
				}
			}
		}, "every limit is 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, rateLimitResponse))
			request := validRateLimitRequest()
			tt.mutate(&request)
			_, err := server.client(t).UpdateRateLimitSettings(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestUpdateRateLimitSettingsDisabledAllowsZeroLimits(t *testing.T) {
	request := validRateLimitRequest()
	request.Enabled = Bool(false)
	for _, field := range RateLimitIntFields {
		if !field.Window {
			zero := 0
			*request.Int(field.Name) = &zero
		}
	}
	if err := request.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestRateLimitSettingsErrorsPreserveAPIError(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusConflict, http.StatusTooManyRequests} {
		server := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"limits denied"}`))
		_, err := server.client(t).UpdateRateLimitSettings(context.Background(), validRateLimitRequest())
		if err == nil || StatusCodeOf(err) != status || !strings.Contains(err.Error(), "limits denied") {
			t.Errorf("status %d: error = %v", status, err)
		}
	}
}
