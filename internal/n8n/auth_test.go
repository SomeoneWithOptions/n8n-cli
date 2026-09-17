package n8n

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecret = "sup3r-s3cret-value"

func TestSecretNeverRendersItsValue(t *testing.T) {
	s := Secret(testSecret)

	encoded, err := json.Marshal(struct {
		Key Secret `json:"key"`
	}{Key: s})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("auth", "secret", s)

	// Formatted through an any so the verbs, not a direct String call, are
	// what the test exercises.
	var boxed any = s

	renderings := map[string]string{
		"%v":          fmt.Sprintf("%v", boxed),
		"%s":          fmt.Sprintf("%s", boxed),
		"%q":          fmt.Sprintf("%q", boxed),
		"%#v":         fmt.Sprintf("%#v", boxed),
		"String":      s.String(),
		"error":       fmt.Errorf("login failed for %v", boxed).Error(),
		"json":        string(encoded),
		"slog":        logged.String(),
		"struct %v":   fmt.Sprintf("%v", struct{ Key Secret }{s}),
		"struct %+v":  fmt.Sprintf("%+v", struct{ Key Secret }{s}),
		"slice of it": fmt.Sprintf("%v", []Secret{s}),
	}
	for name, got := range renderings {
		if strings.Contains(got, testSecret) {
			t.Errorf("%s leaks the secret: %s", name, got)
		}
		if !strings.Contains(got, Redacted) {
			t.Errorf("%s = %s, want it to contain %q", name, got, Redacted)
		}
	}

	if s.Reveal() != testSecret {
		t.Errorf("Reveal() = %q, want the original value", s.Reveal())
	}
	if s.Empty() {
		t.Error("Empty() = true, want false")
	}
	if !Secret("").Empty() {
		t.Error(`Secret("").Empty() = false, want true`)
	}
}

func TestAuthenticatorsInjectExactlyOneCredential(t *testing.T) {
	apiKey, err := NewAPIKeyAuth(testSecret)
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}
	bearer, err := NewBearerAuth(testSecret)
	if err != nil {
		t.Fatalf("NewBearerAuth: %v", err)
	}
	cookie, err := NewCookieAuth(testSecret)
	if err != nil {
		t.Fatalf("NewCookieAuth: %v", err)
	}

	tests := []struct {
		name       string
		auth       Authenticator
		wantType   AuthType
		wantHeader string
		wantValue  string
	}{
		{"api key", apiKey, AuthAPIKey, HeaderAPIKey, testSecret},
		{"bearer", bearer, AuthBearer, "Authorization", "Bearer " + testSecret},
		{"cookie", cookie, AuthCookie, "Cookie", CookieName + "=" + testSecret},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://n8n.example.com/api/v1/discover", nil)
			tt.auth.Apply(req)

			if got := tt.auth.Type(); got != tt.wantType {
				t.Errorf("Type() = %q, want %q", got, tt.wantType)
			}
			if got := req.Header.Get(tt.wantHeader); got != tt.wantValue {
				t.Errorf("%s = %q, want %q", tt.wantHeader, got, tt.wantValue)
			}

			// Exactly one mechanism: the other two carry nothing.
			for _, other := range []string{HeaderAPIKey, "Authorization", "Cookie"} {
				if other == tt.wantHeader {
					continue
				}
				if got := req.Header.Get(other); got != "" {
					t.Errorf("%s = %q, want it unset", other, got)
				}
			}
		})
	}
}

func TestAuthenticatorsRejectEmptyCredential(t *testing.T) {
	if _, err := NewAPIKeyAuth(""); !errors.Is(err, ErrEmptyCredential) {
		t.Errorf("NewAPIKeyAuth(\"\") error = %v, want ErrEmptyCredential", err)
	}
	if _, err := NewBearerAuth(""); !errors.Is(err, ErrEmptyCredential) {
		t.Errorf("NewBearerAuth(\"\") error = %v, want ErrEmptyCredential", err)
	}
	if _, err := NewCookieAuth(""); !errors.Is(err, ErrEmptyCredential) {
		t.Errorf("NewCookieAuth(\"\") error = %v, want ErrEmptyCredential", err)
	}
}

func TestAuthenticatorLogValueIsRedacted(t *testing.T) {
	apiKey, _ := NewAPIKeyAuth(testSecret)
	bearer, _ := NewBearerAuth(testSecret)
	cookie, _ := NewCookieAuth(testSecret)

	for _, auth := range []Authenticator{apiKey, bearer, cookie} {
		var logged bytes.Buffer
		slog.New(slog.NewTextHandler(&logged, nil)).Info("auth", "auth", auth)
		if strings.Contains(logged.String(), testSecret) {
			t.Errorf("%T log output leaks the secret: %s", auth, logged.String())
		}
		if !strings.Contains(logged.String(), Redacted) {
			t.Errorf("%T log output = %s, want it redacted", auth, logged.String())
		}
	}
}
