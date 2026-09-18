package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const samlConfigurationResponse = `{
  "entityID":"https://n8n.example.com/rest/sso/saml/metadata",
  "returnUrl":"https://n8n.example.com/rest/sso/saml/acs",
  "mapping":{"email":"email","firstName":"first","lastName":"last","userPrincipalName":"upn","n8nInstanceRole":"role","n8nProjectRoles":["project:role"]},
  "metadata":"` + SAMLRedactedValue + `",
  "metadataUrl":"https://idp.example.com/metadata",
  "ignoreSSL":false,
  "loginBinding":"redirect",
  "loginEnabled":true,
  "loginLabel":"Company SSO",
  "authnRequestsSigned":true,
  "wantAssertionsSigned":true,
  "wantMessageSigned":false,
  "signingPrivateKey":"` + SAMLRedactedValue + `",
  "signingCertificate":"` + SAMLRedactedValue + `",
  "acsBinding":"post",
  "signatureConfig":{"prefix":"ds","location":{"reference":"/samlp:Response/saml:Issuer","action":"after"}},
  "relayState":"https://n8n.example.com/home"
}`

func validSAMLSetRequest() SetSAMLConfigurationRequest {
	return SetSAMLConfigurationRequest{
		EntityID:  ptr("https://ignored.example.com/entity"),
		ReturnURL: ptr("https://ignored.example.com/acs"),
		Mapping: &SetSAMLMapping{
			Email: ptr("email"), FirstName: ptr("first"), LastName: ptr("last"),
			UserPrincipalName: ptr("upn"), N8nInstanceRole: ptr(""), N8nProjectRoles: ptr([]string{}),
		},
		Metadata: ptr(SAMLRedactedValue), MetadataURL: ptr(""), IgnoreSSL: ptr(false),
		LoginBinding: ptr(SAMLBindingRedirect), LoginEnabled: ptr(false), LoginLabel: ptr(""),
		AuthnRequestsSigned: ptr(false), WantAssertionsSigned: ptr(false), WantMessageSigned: ptr(false),
		SigningPrivateKey: ptr(SAMLRedactedValue), SigningCertificate: ptr(SAMLRedactedValue),
		ACSBinding: ptr(SAMLBindingPost),
		SignatureConfig: &SetSAMLSignatureConfig{Prefix: ptr("ds"), Location: &SetSAMLSignatureLocation{
			Reference: ptr("/samlp:Response/saml:Issuer"), Action: ptr(SAMLSignatureAfter),
		}},
		RelayState: ptr(""),
	}
}

func TestGetSAMLConfiguration(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, samlConfigurationResponse))
	configuration, err := server.client(t).GetSAMLConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetSAMLConfiguration: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+SAMLSettingsPath || req.URL.RawQuery != "" || body != "" {
		t.Errorf("request = %s %s?%s body %q", req.Method, req.URL.EscapedPath(), req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !configuration.LoginEnabled || configuration.LoginBinding != SAMLBindingRedirect || configuration.ACSBinding != SAMLBindingPost || configuration.Mapping.Email != "email" || len(configuration.Mapping.N8nProjectRoles) != 1 {
		t.Errorf("configuration = %+v", configuration)
	}
	if configuration.Metadata != SAMLRedactedValue || configuration.SigningPrivateKey != SAMLRedactedValue || configuration.SigningCertificate != SAMLRedactedValue {
		t.Errorf("secret placeholders = %q, %q, %q", configuration.Metadata, configuration.SigningPrivateKey, configuration.SigningCertificate)
	}
}

func TestGetSAMLConfigurationNeverRetainsPlaintextSecrets(t *testing.T) {
	const secret = "plaintext-private-material"
	body := strings.ReplaceAll(samlConfigurationResponse, SAMLRedactedValue, secret)
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	configuration, err := server.client(t).GetSAMLConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetSAMLConfiguration: %v", err)
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), secret) || strings.Count(string(encoded), SAMLRedactedValue) != 3 {
		t.Errorf("plaintext secret survived response sanitization: %s", encoded)
	}
}

func TestGetSAMLConfigurationPreservesUnsetSecrets(t *testing.T) {
	body := strings.ReplaceAll(samlConfigurationResponse, SAMLRedactedValue, "")
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	configuration, err := server.client(t).GetSAMLConfiguration(context.Background())
	if err != nil {
		t.Fatalf("GetSAMLConfiguration: %v", err)
	}
	if configuration.Metadata != "" || configuration.SigningPrivateKey != "" || configuration.SigningCertificate != "" {
		t.Errorf("unset secrets = %q, %q, %q", configuration.Metadata, configuration.SigningPrivateKey, configuration.SigningCertificate)
	}
}

func TestSetSAMLConfigurationFullReplacementAndIgnoresReadOnlyFields(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, samlConfigurationResponse))
	configuration, err := server.client(t).SetSAMLConfiguration(context.Background(), validSAMLSetRequest())
	if err != nil {
		t.Fatalf("SetSAMLConfiguration: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+SAMLSettingsPath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if len(sent) != 15 {
		t.Errorf("replacement has %d fields, want 15: %#v", len(sent), sent)
	}
	if _, ok := sent["entityID"]; ok {
		t.Error("read-only entityID was sent")
	}
	if _, ok := sent["returnUrl"]; ok {
		t.Error("read-only returnUrl was sent")
	}
	if sent["loginEnabled"] != false || sent["metadataUrl"] != "" || sent["relayState"] != "" || sent["signingPrivateKey"] != SAMLRedactedValue {
		t.Errorf("false, empty, or sentinel values were not preserved: %#v", sent)
	}
	mapping := sent["mapping"].(map[string]any)
	if roles, ok := mapping["n8nProjectRoles"].([]any); !ok || len(roles) != 0 {
		t.Errorf("empty project role array not preserved: %#v", mapping["n8nProjectRoles"])
	}
	if configuration.Metadata != SAMLRedactedValue || configuration.SigningPrivateKey != SAMLRedactedValue || configuration.SigningCertificate != SAMLRedactedValue {
		t.Errorf("response was not redacted: %+v", configuration)
	}
}

func TestSetSAMLConfigurationCanReplaceSecretsWithoutReturningThem(t *testing.T) {
	const secret = "new-private-material"
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, strings.ReplaceAll(samlConfigurationResponse, SAMLRedactedValue, secret)))
	request := validSAMLSetRequest()
	request.Metadata = ptr(secret)
	request.SigningPrivateKey = ptr(secret)
	request.SigningCertificate = ptr(secret)
	configuration, err := server.client(t).SetSAMLConfiguration(context.Background(), request)
	if err != nil {
		t.Fatalf("SetSAMLConfiguration: %v", err)
	}
	_, body := server.last(t)
	if strings.Count(body, secret) != 3 {
		t.Errorf("request did not contain all replacements: %s", body)
	}
	encoded, _ := json.Marshal(configuration)
	if strings.Contains(string(encoded), secret) || strings.Count(string(encoded), SAMLRedactedValue) != 3 {
		t.Errorf("plaintext response survived sanitization: %s", encoded)
	}
}

func TestSetSAMLConfigurationValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SetSAMLConfigurationRequest)
		want   string
	}{
		{name: "missing mapping", mutate: func(r *SetSAMLConfigurationRequest) { r.Mapping = nil }, want: "mapping"},
		{name: "missing metadata", mutate: func(r *SetSAMLConfigurationRequest) { r.Metadata = nil }, want: "metadata"},
		{name: "missing metadata URL", mutate: func(r *SetSAMLConfigurationRequest) { r.MetadataURL = nil }, want: "metadataUrl"},
		{name: "missing false", mutate: func(r *SetSAMLConfigurationRequest) { r.IgnoreSSL = nil }, want: "ignoreSSL"},
		{name: "invalid login binding", mutate: func(r *SetSAMLConfigurationRequest) { r.LoginBinding = ptr(SAMLBinding("soap")) }, want: "loginBinding"},
		{name: "missing login enabled", mutate: func(r *SetSAMLConfigurationRequest) { r.LoginEnabled = nil }, want: "loginEnabled"},
		{name: "missing login label", mutate: func(r *SetSAMLConfigurationRequest) { r.LoginLabel = nil }, want: "loginLabel"},
		{name: "missing authn signed", mutate: func(r *SetSAMLConfigurationRequest) { r.AuthnRequestsSigned = nil }, want: "authnRequestsSigned"},
		{name: "missing assertions signed", mutate: func(r *SetSAMLConfigurationRequest) { r.WantAssertionsSigned = nil }, want: "wantAssertionsSigned"},
		{name: "missing message signed", mutate: func(r *SetSAMLConfigurationRequest) { r.WantMessageSigned = nil }, want: "wantMessageSigned"},
		{name: "missing key", mutate: func(r *SetSAMLConfigurationRequest) { r.SigningPrivateKey = nil }, want: "signingPrivateKey"},
		{name: "missing certificate", mutate: func(r *SetSAMLConfigurationRequest) { r.SigningCertificate = nil }, want: "signingCertificate"},
		{name: "invalid ACS binding", mutate: func(r *SetSAMLConfigurationRequest) { r.ACSBinding = ptr(SAMLBinding("soap")) }, want: "acsBinding"},
		{name: "missing signature config", mutate: func(r *SetSAMLConfigurationRequest) { r.SignatureConfig = nil }, want: "signatureConfig"},
		{name: "missing relay state", mutate: func(r *SetSAMLConfigurationRequest) { r.RelayState = nil }, want: "relayState"},
		{name: "missing mapping field", mutate: func(r *SetSAMLConfigurationRequest) { r.Mapping.N8nProjectRoles = nil }, want: "mapping.n8nProjectRoles"},
		{name: "null mapping array", mutate: func(r *SetSAMLConfigurationRequest) { r.Mapping.N8nProjectRoles = ptr([]string(nil)) }, want: "must be an array"},
		{name: "missing signature prefix", mutate: func(r *SetSAMLConfigurationRequest) { r.SignatureConfig.Prefix = nil }, want: "signatureConfig.prefix"},
		{name: "missing signature location", mutate: func(r *SetSAMLConfigurationRequest) { r.SignatureConfig.Location = nil }, want: "signatureConfig.location"},
		{name: "missing signature reference", mutate: func(r *SetSAMLConfigurationRequest) { r.SignatureConfig.Location.Reference = nil }, want: "location.reference"},
		{name: "invalid signature action", mutate: func(r *SetSAMLConfigurationRequest) {
			r.SignatureConfig.Location.Action = ptr(SAMLSignatureAction("inside"))
		}, want: "prepend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, samlConfigurationResponse))
			request := validSAMLSetRequest()
			tt.mutate(&request)
			_, err := server.client(t).SetSAMLConfiguration(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestSAMLAPIErrorPreservesStatusAndRedactsResponse(t *testing.T) {
	const secret = "submitted-private-key"
	for _, method := range []string{"get", "set"} {
		t.Run(method, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"bad key `+secret+`"}`))
			var err error
			if method == "get" {
				_, err = server.client(t).GetSAMLConfiguration(context.Background())
			} else {
				request := validSAMLSetRequest()
				request.SigningPrivateKey = ptr(secret)
				_, err = server.client(t).SetSAMLConfiguration(context.Background(), request)
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
