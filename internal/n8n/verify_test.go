package n8n

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerify(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr func(error) bool
	}{
		{name: "accepted", status: http.StatusOK, body: `{"scopes":["workflow:read"]}`},
		{name: "empty body", status: http.StatusNoContent},
		{name: "rejected", status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`, wantErr: IsUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"forbidden"}`, wantErr: IsForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", contentTypeJSON)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			err = client.Verify(context.Background())
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("Verify() = %v, want nil", err)
			case tt.wantErr != nil && !tt.wantErr(err):
				t.Fatalf("Verify() = %v, want the documented status", err)
			}
			if want := BasePath + DiscoverPath; gotPath != want {
				t.Errorf("path = %q, want %q", gotPath, want)
			}
		})
	}
}

func TestRequireAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		authType AuthType
		wantErr  bool
		wantText string
	}{
		{name: "api key", authType: AuthAPIKey},
		{name: "bearer", authType: AuthBearer, wantErr: true, wantText: "bearer"},
		{name: "cookie", authType: AuthCookie, wantErr: true, wantText: "cookie"},
		{name: "no credential", authType: "", wantErr: true, wantText: "no credential"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireAPIKey(tt.authType)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("RequireAPIKey(%q) = %v, want nil", tt.authType, err)
				}
				return
			}
			var apiKeyErr *ErrAPIKeyRequired
			if !errors.As(err, &apiKeyErr) {
				t.Fatalf("RequireAPIKey(%q) = %v, want *ErrAPIKeyRequired", tt.authType, err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantText)
			}
			if !strings.Contains(err.Error(), "API key") {
				t.Errorf("error = %q, want it to say what is required", err)
			}
		})
	}
}
