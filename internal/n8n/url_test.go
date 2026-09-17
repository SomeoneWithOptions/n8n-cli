package n8n

import (
	"net/url"
	"strings"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare host defaults to https", "n8n.example.com", "https://n8n.example.com/api/v1"},
		{"https host", "https://n8n.example.com", "https://n8n.example.com/api/v1"},
		{"http host kept", "http://localhost:5678", "http://localhost:5678/api/v1"},
		{"trailing slash", "https://n8n.example.com/", "https://n8n.example.com/api/v1"},
		{"many trailing slashes", "https://n8n.example.com///", "https://n8n.example.com/api/v1"},
		{"surrounding space", "  https://n8n.example.com  ", "https://n8n.example.com/api/v1"},
		{"uppercase scheme and host", "HTTPS://N8N.Example.COM", "https://n8n.example.com/api/v1"},
		{"base path already present", "https://n8n.example.com/api/v1", "https://n8n.example.com/api/v1"},
		{"base path with trailing slash", "https://n8n.example.com/api/v1/", "https://n8n.example.com/api/v1"},
		{"sub path", "https://example.com/n8n", "https://example.com/n8n/api/v1"},
		{"sub path with base path", "https://example.com/n8n/api/v1", "https://example.com/n8n/api/v1"},
		{"ipv6 host", "http://[::1]:5678", "http://[::1]:5678/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.in)
			if err != nil {
				t.Fatalf("NormalizeBaseURL(%q) error = %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("NormalizeBaseURL(%q) = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}
}

func TestNormalizeBaseURLErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "empty"},
		{"blank", "   ", "empty"},
		{"credentials", "https://user:pass@n8n.example.com", "must not contain credentials"},
		{"user only", "https://user@n8n.example.com", "must not contain credentials"},
		{"unsupported scheme", "ftp://n8n.example.com", "unsupported instance URL scheme"},
		{"file scheme", "file:///etc/passwd", "unsupported instance URL scheme"},
		{"no host", "https://", "no host"},
		{"query string", "https://n8n.example.com?token=abc", "query string"},
		{"fragment", "https://n8n.example.com#frag", "fragment"},
		{"unparseable", "https://n8n.example.com/%zz", "parse instance URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.in)
			if err == nil {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want error", tt.in, got)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// A URL carrying a password must never be rendered back to the user.
func TestNormalizeBaseURLDoesNotEchoSecrets(t *testing.T) {
	_, err := NormalizeBaseURL("https://user:sup3rsecret@n8n.example.com")
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "sup3rsecret") {
		t.Errorf("error leaks the password: %q", err)
	}
}

func TestPathJoin(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"none", nil, "/"},
		{"single", []string{"workflows"}, "/workflows"},
		{"nested", []string{"workflows", "42", "activate"}, "/workflows/42/activate"},
		{"empty segments dropped", []string{"workflows", "", "42"}, "/workflows/42"},
		{"slash in id escaped", []string{"workflows", "a/b"}, "/workflows/a%2Fb"},
		{"space escaped", []string{"tags", "my tag"}, "/tags/my%20tag"},
		{"traversal escaped", []string{"workflows", "../../admin"}, "/workflows/..%2F..%2Fadmin"},
		{"percent escaped", []string{"workflows", "100%"}, "/workflows/100%25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PathJoin(tt.in...); got != tt.want {
				t.Errorf("PathJoin(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzNormalizeBaseURL(f *testing.F) {
	for _, seed := range []string{
		"", "n8n.example.com", "https://n8n.example.com/", "http://[::1]:5678",
		"https://example.com/n8n/api/v1", "ftp://x", "https://u:p@h", "%%", "https://h/%zz",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got, err := NormalizeBaseURL(raw)
		if err != nil {
			return
		}

		if got.User != nil {
			t.Fatalf("NormalizeBaseURL(%q) kept credentials", raw)
		}
		if !strings.HasSuffix(got.EscapedPath(), BasePath) {
			t.Fatalf("NormalizeBaseURL(%q) = %q, want a %s suffix", raw, got, BasePath)
		}
		if _, err := url.Parse(got.String()); err != nil {
			t.Fatalf("NormalizeBaseURL(%q) = %q, which does not parse: %v", raw, got, err)
		}

		// Normalization is idempotent: feeding the result back changes nothing.
		again, err := NormalizeBaseURL(got.String())
		if err != nil {
			t.Fatalf("NormalizeBaseURL(%q) rejects its own output %q: %v", raw, got, err)
		}
		if again.String() != got.String() {
			t.Fatalf("not idempotent: %q -> %q -> %q", raw, got, again)
		}
	})
}
