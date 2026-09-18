package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const oidcConfigurationResponse = `{
  "clientId":"n8n-client",
  "clientSecret":"` + OIDCClientSecretRedactedValue + `",
  "discoveryEndpoint":"https://accounts.example.com/.well-known/openid-configuration",
  "loginEnabled":true,
  "prompt":"select_account",
  "authenticationContextClassReference":["mfa","pwd"],
  "additionalScopes":"groups roles",
  "emailVerifiedRequired":false,
  "rpInitiatedLogoutEnabled":true
}`

func validOIDCSetRequest() SetOIDCConfigurationRequest {
	return SetOIDCConfigurationRequest{
		ClientID:                            ptr("n8n-client"),
		ClientSecret:                        ptr(OIDCClientSecretRedactedValue),
		DiscoveryEndpoint:                   ptr("https://accounts.example.com/.well-known/openid-configuration"),
		LoginEnabled:                        ptr(false),
		Prompt:                              ptr(OIDCPromptNone),
		AuthenticationContextClassReference: ptr([]string{}),
		AdditionalScopes:                    ptr(""),
		EmailVerifiedRequired:               ptr(false),
		RPInitiatedLogoutEnabled:            ptr(false),
	}
}

func TestGetOIDCConfiguration(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, oidcConfigurationResponse))
	configuration, err := server.client(t).GetOIDCConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetOIDCConfiguration: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+OIDCSettingsPath || req.URL.RawQuery != "" || body != "" {
		t.Errorf("request = %s %s?%s body %q", req.Method, req.URL.EscapedPath(), req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !configuration.LoginEnabled || configuration.Prompt != OIDCPromptSelectAccount || len(configuration.AuthenticationContextClassReference) != 2 || configuration.ClientSecret != OIDCClientSecretRedactedValue {
		t.Errorf("configuration = %+v", configuration)
	}
}

func TestGetOIDCConfigurationNeverRetainsPlaintextSecret(t *testing.T) {
	const secret = "plaintext-client-secret"
	body := strings.Replace(oidcConfigurationResponse, OIDCClientSecretRedactedValue, secret, 1)
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	configuration, err := server.client(t).GetOIDCConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetOIDCConfiguration: %v", err)
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), secret) || configuration.ClientSecret != OIDCClientSecretRedactedValue {
		t.Errorf("plaintext secret survived response sanitization: %s", encoded)
	}
}

func TestGetOIDCConfigurationPreservesUnsetSecret(t *testing.T) {
	body := strings.Replace(oidcConfigurationResponse, OIDCClientSecretRedactedValue, "", 1)
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	configuration, err := server.client(t).GetOIDCConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetOIDCConfiguration: %v", err)
	}
	if configuration.ClientSecret != "" {
		t.Errorf("client secret = %q, want empty", configuration.ClientSecret)
	}
}

func TestSetOIDCConfigurationFullReplacementAndSentinel(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, oidcConfigurationResponse))
	configuration, err := server.client(t).SetOIDCConfiguration(context.Background(), validOIDCSetRequest())
	if err != nil {
		t.Fatalf("SetOIDCConfiguration: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+OIDCSettingsPath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if len(sent) != 9 {
		t.Errorf("replacement has %d fields, want 9: %#v", len(sent), sent)
	}
	if sent["loginEnabled"] != false || sent["emailVerifiedRequired"] != false || sent["rpInitiatedLogoutEnabled"] != false || sent["additionalScopes"] != "" || sent["clientSecret"] != OIDCClientSecretRedactedValue {
		t.Errorf("false, empty, or sentinel values were not preserved: %#v", sent)
	}
	acr, ok := sent["authenticationContextClassReference"].([]any)
	if !ok || len(acr) != 0 {
		t.Errorf("empty ACR array was not preserved: %#v", sent["authenticationContextClassReference"])
	}
	if configuration.ClientSecret != OIDCClientSecretRedactedValue {
		t.Errorf("response secret = %q", configuration.ClientSecret)
	}
}

func TestSetOIDCConfigurationCanReplaceSecret(t *testing.T) {
	const secret = "new-client-secret"
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, strings.Replace(oidcConfigurationResponse, OIDCClientSecretRedactedValue, secret, 1)))
	request := validOIDCSetRequest()
	request.ClientSecret = ptr(secret)
	configuration, err := server.client(t).SetOIDCConfiguration(context.Background(), request)
	if err != nil {
		t.Fatalf("SetOIDCConfiguration: %v", err)
	}
	_, body := server.last(t)
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if sent["clientSecret"] != secret {
		t.Errorf("submitted secret = %#v", sent["clientSecret"])
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), secret) || configuration.ClientSecret != OIDCClientSecretRedactedValue {
		t.Errorf("plaintext response secret survived sanitization: %s", encoded)
	}
}

func TestSetOIDCConfigurationValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SetOIDCConfigurationRequest)
		want   string
	}{
		{name: "missing client ID", mutate: func(r *SetOIDCConfigurationRequest) { r.ClientID = nil }, want: "clientId"},
		{name: "empty client ID", mutate: func(r *SetOIDCConfigurationRequest) { r.ClientID = ptr("") }, want: "clientId must not be empty"},
		{name: "missing secret", mutate: func(r *SetOIDCConfigurationRequest) { r.ClientSecret = nil }, want: "clientSecret"},
		{name: "empty secret", mutate: func(r *SetOIDCConfigurationRequest) { r.ClientSecret = ptr("") }, want: "sentinel"},
		{name: "missing endpoint", mutate: func(r *SetOIDCConfigurationRequest) { r.DiscoveryEndpoint = nil }, want: "discoveryEndpoint"},
		{name: "invalid endpoint", mutate: func(r *SetOIDCConfigurationRequest) { r.DiscoveryEndpoint = ptr("not-a-url") }, want: "discoveryEndpoint"},
		{name: "missing login enabled", mutate: func(r *SetOIDCConfigurationRequest) { r.LoginEnabled = nil }, want: "loginEnabled"},
		{name: "missing prompt", mutate: func(r *SetOIDCConfigurationRequest) { r.Prompt = nil }, want: "prompt"},
		{name: "invalid prompt", mutate: func(r *SetOIDCConfigurationRequest) { r.Prompt = ptr(OIDCPrompt("signup")) }, want: "select_account"},
		{name: "missing empty array", mutate: func(r *SetOIDCConfigurationRequest) { r.AuthenticationContextClassReference = nil }, want: "authenticationContextClassReference"},
		{name: "missing empty string", mutate: func(r *SetOIDCConfigurationRequest) { r.AdditionalScopes = nil }, want: "additionalScopes"},
		{name: "missing email verified required", mutate: func(r *SetOIDCConfigurationRequest) { r.EmailVerifiedRequired = nil }, want: "emailVerifiedRequired"},
		{name: "missing RP logout enabled", mutate: func(r *SetOIDCConfigurationRequest) { r.RPInitiatedLogoutEnabled = nil }, want: "rpInitiatedLogoutEnabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, oidcConfigurationResponse))
			request := validOIDCSetRequest()
			tt.mutate(&request)
			_, err := server.client(t).SetOIDCConfiguration(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestOIDCAPIErrorPreservesStatusAndRedactsResponse(t *testing.T) {
	const secret = "submitted-client-secret"
	for _, method := range []string{"get", "set"} {
		t.Run(method, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"bad secret `+secret+`"}`))
			var err error
			if method == "get" {
				_, err = server.client(t).GetOIDCConfiguration(context.Background())
			} else {
				request := validOIDCSetRequest()
				request.ClientSecret = ptr(secret)
				_, err = server.client(t).SetOIDCConfiguration(context.Background(), request)
			}
			if err == nil || StatusCodeOf(err) != http.StatusBadRequest {
				t.Fatalf("error = %v, want status 400", err)
			}
			if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "response details redacted") {
				t.Errorf("error was not safely redacted: %v", err)
			}
		})
	}
}
