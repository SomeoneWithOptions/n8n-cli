package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const ldapConfigurationResponse = `{
  "loginEnabled":true,
  "loginLabel":"Company LDAP",
  "connectionUrl":"ldap.example.com",
  "allowUnauthorizedCerts":false,
  "connectionSecurity":"startTls",
  "connectionPort":389,
  "baseDn":"dc=example,dc=com",
  "bindingAdminDn":"cn=admin,dc=example,dc=com",
  "bindingAdminPassword":"` + CredentialBlankingValue + `",
  "firstNameAttribute":"givenName",
  "lastNameAttribute":"sn",
  "emailAttribute":"mail",
  "loginIdAttribute":"mail",
  "ldapIdAttribute":"uid",
  "userFilter":"(objectClass=inetOrgPerson)",
  "synchronizationEnabled":true,
  "synchronizationInterval":60,
  "searchPageSize":1000,
  "searchTimeout":30,
  "enforceEmailUniqueness":true
}`

const ldapHistoryResponse = `{
  "id":42,
  "runMode":"dry",
  "status":"success",
  "startedAt":"2026-09-17T10:00:00Z",
  "endedAt":"2026-09-17T10:00:02Z",
  "scanned":12,
  "created":2,
  "updated":3,
  "disabled":1,
  "error":""
}`

func validLDAPUpdateRequest() UpdateLDAPConfigurationRequest {
	return UpdateLDAPConfigurationRequest{
		LoginEnabled:            ptr(false),
		LoginLabel:              ptr("LDAP"),
		ConnectionURL:           ptr("ldap.example.com"),
		AllowUnauthorizedCerts:  ptr(false),
		ConnectionSecurity:      ptr(LDAPSecurityNone),
		ConnectionPort:          ptr(0),
		BaseDN:                  ptr(""),
		BindingAdminDN:          ptr(""),
		BindingAdminPassword:    ptr(CredentialBlankingValue),
		FirstNameAttribute:      ptr("givenName"),
		LastNameAttribute:       ptr("sn"),
		EmailAttribute:          ptr("mail"),
		LoginIDAttribute:        ptr("mail"),
		LDAPIDAttribute:         ptr("uid"),
		UserFilter:              ptr(""),
		SynchronizationEnabled:  ptr(false),
		SynchronizationInterval: ptr(0),
		SearchPageSize:          ptr(0),
		SearchTimeout:           ptr(0),
		EnforceEmailUniqueness:  ptr(false),
	}
}

func TestGetLDAPConfiguration(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, ldapConfigurationResponse))
	configuration, err := server.client(t).GetLDAPConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetLDAPConfiguration: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+LDAPSettingsPath || req.URL.RawQuery != "" || body != "" {
		t.Errorf("request = %s %s?%s body %q", req.Method, req.URL.EscapedPath(), req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !configuration.LoginEnabled || configuration.ConnectionSecurity != LDAPSecurityStartTLS || configuration.ConnectionPort != 389 || configuration.BindingAdminPassword != CredentialBlankingValue {
		t.Errorf("configuration = %+v", configuration)
	}
}

func TestGetLDAPConfigurationNeverRetainsPlaintextPassword(t *testing.T) {
	const secret = "plaintext-bind-secret"
	body := strings.Replace(ldapConfigurationResponse, CredentialBlankingValue, secret, 1)
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	configuration, err := server.client(t).GetLDAPConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetLDAPConfiguration: %v", err)
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), secret) || configuration.BindingAdminPassword != CredentialBlankingValue {
		t.Errorf("plaintext password survived response sanitization: %s", encoded)
	}
}

func TestUpdateLDAPConfigurationFullReplacementAndSentinel(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, ldapConfigurationResponse))
	configuration, err := server.client(t).UpdateLDAPConfiguration(context.Background(), validLDAPUpdateRequest())
	if err != nil {
		t.Fatalf("UpdateLDAPConfiguration: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+LDAPSettingsPath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if len(sent) != 20 {
		t.Errorf("replacement has %d fields, want 20: %#v", len(sent), sent)
	}
	if sent["loginEnabled"] != false || sent["allowUnauthorizedCerts"] != false || sent["connectionPort"] != float64(0) || sent["bindingAdminPassword"] != CredentialBlankingValue {
		t.Errorf("false, zero, or sentinel values were not preserved: %#v", sent)
	}
	if configuration.BindingAdminPassword != CredentialBlankingValue {
		t.Errorf("response password = %q", configuration.BindingAdminPassword)
	}
}

func TestUpdateLDAPConfigurationCanClearOrReplacePassword(t *testing.T) {
	for _, password := range []string{"", "new-bind-password"} {
		t.Run(password, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, ldapConfigurationResponse))
			request := validLDAPUpdateRequest()
			request.BindingAdminPassword = ptr(password)
			if _, err := server.client(t).UpdateLDAPConfiguration(context.Background(), request); err != nil {
				t.Fatalf("UpdateLDAPConfiguration: %v", err)
			}
			_, body := server.last(t)
			var sent map[string]any
			if err := json.Unmarshal([]byte(body), &sent); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if sent["bindingAdminPassword"] != password {
				t.Errorf("password = %#v, want submitted value", sent["bindingAdminPassword"])
			}
		})
	}
}

func TestUpdateLDAPConfigurationValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*UpdateLDAPConfigurationRequest)
		want   string
	}{
		{name: "missing login enabled", mutate: func(r *UpdateLDAPConfigurationRequest) { r.LoginEnabled = nil }, want: "loginEnabled"},
		{name: "missing empty string", mutate: func(r *UpdateLDAPConfigurationRequest) { r.BaseDN = nil }, want: "baseDn"},
		{name: "missing false boolean", mutate: func(r *UpdateLDAPConfigurationRequest) { r.SynchronizationEnabled = nil }, want: "synchronizationEnabled"},
		{name: "missing zero integer", mutate: func(r *UpdateLDAPConfigurationRequest) { r.SearchPageSize = nil }, want: "searchPageSize"},
		{name: "missing password", mutate: func(r *UpdateLDAPConfigurationRequest) { r.BindingAdminPassword = nil }, want: "bindingAdminPassword"},
		{name: "invalid security", mutate: func(r *UpdateLDAPConfigurationRequest) { r.ConnectionSecurity = ptr(LDAPConnectionSecurity("ssl")) }, want: "none, tls, or startTls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, ldapConfigurationResponse))
			request := validLDAPUpdateRequest()
			tt.mutate(&request)
			_, err := server.client(t).UpdateLDAPConfiguration(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestListLDAPSyncHistory(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+ldapHistoryResponse+`],"nextCursor":"next page"}`))
	page, err := server.client(t).ListLDAPSyncHistory(context.Background(), ListLDAPSyncHistoryOptions{ListOptions{Limit: 25, Cursor: "start & page"}})
	if err != nil {
		t.Fatalf("ListLDAPSyncHistory: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+LDAPSyncPath || body != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), body)
	}
	if req.URL.Query().Get("limit") != "25" || req.URL.Query().Get("cursor") != "start & page" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	if len(page.Data) != 1 || page.Data[0].ID != 42 || page.Data[0].RunMode != LDAPSyncDry || page.NextCursor != "next page" {
		t.Errorf("page = %+v", page)
	}
}

func TestListLDAPSyncHistoryValidatesLimit(t *testing.T) {
	for _, limit := range []int{-1, 251} {
		server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
		_, err := server.client(t).ListLDAPSyncHistory(context.Background(), ListLDAPSyncHistoryOptions{ListOptions{Limit: limit}})
		if err == nil || len(server.requests) != 0 {
			t.Errorf("limit %d: error=%v requests=%d", limit, err, len(server.requests))
		}
	}
}

func TestRunLDAPSyncModes(t *testing.T) {
	for _, mode := range []LDAPSyncMode{LDAPSyncDry, LDAPSyncLive} {
		t.Run(string(mode), func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, strings.Replace(ldapHistoryResponse, `"dry"`, `"`+string(mode)+`"`, 1)))
			history, err := server.client(t).RunLDAPSync(context.Background(), LDAPSyncRequest{Type: mode})
			if err != nil {
				t.Fatalf("RunLDAPSync: %v", err)
			}
			req, body := server.last(t)
			if req.Method != http.MethodPost || req.URL.EscapedPath() != BasePath+LDAPSyncPath || req.URL.RawQuery != "" {
				t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
			}
			assertJSONEqual(t, `{"type":"`+string(mode)+`"}`, body)
			if history.RunMode != mode || history.ID != 42 {
				t.Errorf("history = %+v", history)
			}
		})
	}
}

func TestRunLDAPSyncValidationBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, ldapHistoryResponse))
	_, err := server.client(t).RunLDAPSync(context.Background(), LDAPSyncRequest{Type: "weekly"})
	if err == nil || !strings.Contains(err.Error(), "live or dry") || len(server.requests) != 0 {
		t.Errorf("error=%v requests=%d", err, len(server.requests))
	}
}

func TestLDAPUpdateErrorPreservesStatusAndRedactsResponse(t *testing.T) {
	const secret = "submitted-bind-secret"
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"bad password `+secret+`"}`))
	request := validLDAPUpdateRequest()
	request.BindingAdminPassword = ptr(secret)
	_, err := server.client(t).UpdateLDAPConfiguration(context.Background(), request)
	if err == nil || StatusCodeOf(err) != http.StatusBadRequest {
		t.Fatalf("error = %v, want status 400", err)
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "response details redacted") {
		t.Errorf("error was not safely redacted: %v", err)
	}
}
