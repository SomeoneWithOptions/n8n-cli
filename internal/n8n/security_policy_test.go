package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const securityPolicyResponse = `{
  "personalSpacePublishing":true,
  "personalSpaceSharing":false,
  "publishedPersonalWorkflowsCount":3,
  "sharedPersonalWorkflowsCount":5,
  "sharedPersonalCredentialsCount":2,
  "redactionEnforcement":{"floor":"production"}
}`

func TestGetSecurityPolicy(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, securityPolicyResponse))
	policy, err := server.client(t).GetSecurityPolicy(context.Background())
	if err != nil {
		t.Fatalf("GetSecurityPolicy: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+SecurityPolicyPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.EscapedPath(), BasePath+SecurityPolicyPath)
	}
	if req.URL.RawQuery != "" || body != "" {
		t.Errorf("query, body = %q, %q; want empty", req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !policy.PersonalSpacePublishing || policy.PersonalSpaceSharing || policy.PublishedPersonalWorkflowsCount != 3 ||
		policy.SharedPersonalWorkflowsCount != 5 || policy.SharedPersonalCredentialsCount != 2 ||
		policy.RedactionEnforcement.Floor != SecurityRedactionProduction {
		t.Errorf("policy = %+v", policy)
	}
}

func TestUpdateSecurityPolicyFullReplacement(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, securityPolicyResponse))
	published, shared, credentials := 3, 5, 2
	policy, err := server.client(t).UpdateSecurityPolicy(context.Background(), UpdateSecurityPolicyRequest{
		PersonalSpacePublishing:         Bool(false),
		PersonalSpaceSharing:            Bool(true),
		RedactionEnforcement:            SecurityRedactionEnforcement{Floor: SecurityRedactionAll},
		PublishedPersonalWorkflowsCount: &published,
		SharedPersonalWorkflowsCount:    &shared,
		SharedPersonalCredentialsCount:  &credentials,
	})
	if err != nil {
		t.Fatalf("UpdateSecurityPolicy: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+SecurityPolicyPath {
		t.Errorf("request = %s %s, want PUT %s", req.Method, req.URL.EscapedPath(), BasePath+SecurityPolicyPath)
	}
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty", req.URL.RawQuery)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	if got, ok := sent["personalSpacePublishing"].(bool); !ok || got {
		t.Errorf("personalSpacePublishing = %#v, want false", sent["personalSpacePublishing"])
	}
	if got, ok := sent["personalSpaceSharing"].(bool); !ok || !got {
		t.Errorf("personalSpaceSharing = %#v, want true", sent["personalSpaceSharing"])
	}
	redaction, ok := sent["redactionEnforcement"].(map[string]any)
	if !ok || redaction["floor"] != "all" {
		t.Errorf("redactionEnforcement = %#v", sent["redactionEnforcement"])
	}
	if sent["publishedPersonalWorkflowsCount"] != float64(3) || sent["sharedPersonalWorkflowsCount"] != float64(5) || sent["sharedPersonalCredentialsCount"] != float64(2) {
		t.Errorf("read-only usage counts were not preserved in GET-edit-PUT body: %#v", sent)
	}
	if policy.RedactionEnforcement.Floor != SecurityRedactionProduction {
		t.Errorf("decoded policy = %+v", policy)
	}
}

func TestUpdateSecurityPolicyValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name    string
		request UpdateSecurityPolicyRequest
		want    string
	}{
		{
			name: "missing publishing",
			request: UpdateSecurityPolicyRequest{
				PersonalSpaceSharing: Bool(false),
				RedactionEnforcement: SecurityRedactionEnforcement{Floor: SecurityRedactionOff},
			},
			want: "personalSpacePublishing",
		},
		{
			name: "missing sharing",
			request: UpdateSecurityPolicyRequest{
				PersonalSpacePublishing: Bool(false),
				RedactionEnforcement:    SecurityRedactionEnforcement{Floor: SecurityRedactionOff},
			},
			want: "personalSpaceSharing",
		},
		{
			name: "missing floor",
			request: UpdateSecurityPolicyRequest{
				PersonalSpacePublishing: Bool(false),
				PersonalSpaceSharing:    Bool(false),
			},
			want: "redactionEnforcement.floor",
		},
		{
			name: "unknown floor",
			request: UpdateSecurityPolicyRequest{
				PersonalSpacePublishing: Bool(false),
				PersonalSpaceSharing:    Bool(false),
				RedactionEnforcement:    SecurityRedactionEnforcement{Floor: "sometimes"},
			},
			want: "off, production, or all",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, securityPolicyResponse))
			_, err := server.client(t).UpdateSecurityPolicy(context.Background(), tt.request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("UpdateSecurityPolicy error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestSecurityPolicyErrorsPreserveAPIError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		call   func(*Client) error
	}{
		{
			name:   "get forbidden license failure",
			status: http.StatusForbidden,
			call: func(client *Client) error {
				_, err := client.GetSecurityPolicy(context.Background())
				return err
			},
		},
		{
			name:   "update environment conflict",
			status: http.StatusConflict,
			call: func(client *Client) error {
				_, err := client.UpdateSecurityPolicy(context.Background(), UpdateSecurityPolicyRequest{
					PersonalSpacePublishing: Bool(true),
					PersonalSpaceSharing:    Bool(true),
					RedactionEnforcement:    SecurityRedactionEnforcement{Floor: SecurityRedactionProduction},
				})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, `{"message":"policy denied"}`))
			err := tt.call(server.client(t))
			if err == nil || StatusCodeOf(err) != tt.status || !strings.Contains(err.Error(), "policy denied") {
				t.Fatalf("error = %v, want API status %d", err, tt.status)
			}
		})
	}
}
