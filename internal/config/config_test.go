package config

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

func testContext(url string) Context {
	return Context{
		URL:           url,
		AuthType:      n8n.AuthAPIKey,
		CredentialRef: "production",
		Storage:       StorageKeyring,
	}
}

func TestValidateContextName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple", input: "production"},
		{name: "punctuation", input: "eu-1_staging.v2"},
		{name: "digits", input: "0"},
		{name: "empty", input: "", wantErr: true},
		{name: "space", input: "my context", wantErr: true},
		{name: "slash", input: "team/prod", wantErr: true},
		{name: "leading dot", input: ".hidden", wantErr: true},
		{name: "null byte", input: "prod\x00", wantErr: true},
		{name: "too long", input: strings.Repeat("a", maxContextName+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContextName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateContextName(%q) = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestConfigPutUseRemove(t *testing.T) {
	cfg := New()
	if err := cfg.Put("production", testContext("https://n8n.example.com")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := cfg.Put("staging", testContext("https://staging.example.com")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if diff := cmp.Diff([]string{"production", "staging"}, cfg.Names()); diff != "" {
		t.Errorf("Names() mismatch (-want +got):\n%s", diff)
	}

	if err := cfg.Use("production"); err != nil {
		t.Fatalf("Use: %v", err)
	}
	name, saved, err := cfg.Lookup("")
	if err != nil {
		t.Fatalf("Lookup(current): %v", err)
	}
	if name != "production" || saved.URL != "https://n8n.example.com" {
		t.Errorf("Lookup(current) = %q, %+v", name, saved)
	}

	if err := cfg.Remove("production"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if cfg.CurrentContext != "" {
		t.Errorf("CurrentContext = %q after removing the current context, want empty", cfg.CurrentContext)
	}
	if _, _, err := cfg.Lookup(""); err != ErrNoCurrentContext {
		t.Errorf("Lookup(\"\") = %v, want ErrNoCurrentContext", err)
	}
	if _, _, err := cfg.Lookup("production"); !IsNotFound(err) {
		t.Errorf("Lookup(removed) = %v, want a not-found error", err)
	}
	if err := cfg.Use("production"); !IsNotFound(err) {
		t.Errorf("Use(removed) = %v, want a not-found error", err)
	}
	if err := cfg.Remove("production"); !IsNotFound(err) {
		t.Errorf("Remove(removed) = %v, want a not-found error", err)
	}
}

func TestConfigPutRejectsInvalidContexts(t *testing.T) {
	valid := testContext("https://n8n.example.com")

	tests := []struct {
		name    string
		key     string
		context Context
	}{
		{name: "bad name", key: "bad name", context: valid},
		{name: "no url", key: "c", context: Context{AuthType: n8n.AuthAPIKey, CredentialRef: "c", Storage: StorageKeyring}},
		{name: "url with credentials", key: "c", context: testContext("https://user:pw@n8n.example.com")},
		{name: "unknown auth type", key: "c", context: Context{URL: "https://n8n.example.com", AuthType: "basic", CredentialRef: "c", Storage: StorageKeyring}},
		{name: "no credential ref", key: "c", context: Context{URL: "https://n8n.example.com", AuthType: n8n.AuthAPIKey, Storage: StorageKeyring}},
		{name: "unknown storage", key: "c", context: Context{URL: "https://n8n.example.com", AuthType: n8n.AuthAPIKey, CredentialRef: "c", Storage: "vault"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := New()
			if err := cfg.Put(tt.key, tt.context); err == nil {
				t.Fatal("Put succeeded, want an error")
			}
			if len(cfg.Contexts) != 0 {
				t.Errorf("Contexts = %v, want the rejected context not to be stored", cfg.Contexts)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	t.Run("newer schema version", func(t *testing.T) {
		cfg := &Config{Version: SchemaVersion + 1, Contexts: map[string]Context{}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "newer than this CLI") {
			t.Fatalf("Validate() = %v, want a schema version error", err)
		}
	})

	t.Run("dangling current context", func(t *testing.T) {
		cfg := &Config{Version: SchemaVersion, CurrentContext: "gone", Contexts: map[string]Context{}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "not defined") {
			t.Fatalf("Validate() = %v, want a dangling current context error", err)
		}
	})
}

func TestRename(t *testing.T) {
	for _, current := range []string{"old", "other", ""} {
		t.Run("current="+current, func(t *testing.T) {
			cfg := New()
			saved := testContext("https://n8n.example.com")
			if err := cfg.Put("old", saved); err != nil {
				t.Fatal(err)
			}
			if err := cfg.Put("other", saved); err != nil {
				t.Fatal(err)
			}
			cfg.CurrentContext = current
			changed, err := cfg.Rename("old", "new")
			if err != nil || !changed {
				t.Fatalf("rename: %v, %v", changed, err)
			}
			if cfg.Contexts["new"] != saved || cfg.Contexts["other"] != saved || len(cfg.Contexts) != 2 {
				t.Fatal("rename changed context fields")
			}
			if _, exists := cfg.Contexts["old"]; exists {
				t.Fatal("source remains")
			}
			want := current
			if current == "old" {
				want = "new"
			}
			if cfg.CurrentContext != want {
				t.Fatalf("current = %q, want %q", cfg.CurrentContext, want)
			}
		})
	}
	for _, tt := range []struct {
		old, new string
		wantErr  bool
	}{
		{"old", "old", false}, {"missing", "missing", true}, {"missing", "new", true},
		{"old", "other", true}, {"", "new", true}, {"old", "", true},
		{".old", "new", true}, {"old", ".new", true}, {"old", "with space", true},
		{"old", strings.Repeat("x", 65), true}, {"old", "é", true},
	} {
		t.Run(tt.old+"/"+tt.new, func(t *testing.T) {
			cfg := New()
			if err := cfg.Put("old", testContext("https://n8n.example.com")); err != nil {
				t.Fatal(err)
			}
			if err := cfg.Put("other", testContext("https://n8n.example.com")); err != nil {
				t.Fatal(err)
			}
			cfg.CurrentContext = "old"
			before, err := encodeConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := cfg.Rename(tt.old, tt.new)
			if changed || (err != nil) != tt.wantErr {
				t.Fatalf("rename = %v, %v", changed, err)
			}
			after, err := encodeConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("no-op/error changed metadata")
			}
		})
	}
}
