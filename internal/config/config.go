// Package config owns the CLI's on-disk state: the non-secret context metadata
// in config.json, the credential stores behind it, and the precedence rules
// that turn flags, environment and saved state into one authenticated client.
//
// Secrets never enter [Config]. A context holds an opaque credential reference;
// the value behind it lives in a [CredentialStore].
package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// SchemaVersion is the version written to config.json and auth.json. A file
// from a newer CLI is rejected rather than silently misread.
const SchemaVersion = 1

// Storage names a credential backend.
type Storage string

const (
	// StorageKeyring is the OS credential store: Secret Service, Keychain or
	// Credential Manager. This is the default.
	StorageKeyring Storage = "keyring"
	// StorageFile is the plaintext auth.json fallback. It is never selected
	// implicitly; see [ErrStorageConsent].
	StorageFile Storage = "file"
)

// Valid reports whether s names a known backend.
func (s Storage) Valid() bool { return s == StorageKeyring || s == StorageFile }

// Context is one n8n instance and the credential reference used against it.
// Keeping the reference per context is what stops a credential for one
// instance from being sent to another.
type Context struct {
	URL           string       `json:"url"`
	AuthType      n8n.AuthType `json:"authType"`
	CredentialRef string       `json:"credentialRef"`
	Storage       Storage      `json:"storage"`
}

// Config is the whole of config.json.
type Config struct {
	Version        int                `json:"version"`
	CurrentContext string             `json:"currentContext,omitempty"`
	Contexts       map[string]Context `json:"contexts,omitempty"`
}

// New returns an empty configuration at the current schema version.
func New() *Config {
	return &Config{Version: SchemaVersion, Contexts: map[string]Context{}}
}

// ErrNoCurrentContext is returned when no context is selected and none was
// named on the command line.
var ErrNoCurrentContext = errors.New("no context selected: run 'n8n auth login' first")

// NotFoundError reports a named context that does not exist.
type NotFoundError struct{ Name string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("context %q not found", e.Name) }

// IsNotFound reports whether err is a missing-context error.
func IsNotFound(err error) bool {
	var nf *NotFoundError
	return errors.As(err, &nf)
}

// Names returns the context names in stable order.
func (c *Config) Names() []string {
	return slices.Sorted(maps.Keys(c.Contexts))
}

// Lookup returns the named context, or the current one when name is empty.
func (c *Config) Lookup(name string) (string, Context, error) {
	if name == "" {
		name = c.CurrentContext
	}
	if name == "" {
		return "", Context{}, ErrNoCurrentContext
	}
	ctx, ok := c.Contexts[name]
	if !ok {
		return "", Context{}, &NotFoundError{Name: name}
	}
	return name, ctx, nil
}

// Put adds or replaces a context. It does not change the current selection.
func (c *Config) Put(name string, ctx Context) error {
	if err := ValidateContextName(name); err != nil {
		return err
	}
	if err := ctx.validate(name); err != nil {
		return err
	}
	if c.Contexts == nil {
		c.Contexts = map[string]Context{}
	}
	c.Version = SchemaVersion
	c.Contexts[name] = ctx
	return nil
}

// Use selects an existing context.
func (c *Config) Use(name string) error {
	if _, ok := c.Contexts[name]; !ok {
		return &NotFoundError{Name: name}
	}
	c.CurrentContext = name
	return nil
}

// Rename moves a context without changing its credential identity. It reports
// whether the name changed; an existing same-name source is a successful no-op.
func (c *Config) Rename(oldName, newName string) (bool, error) {
	for _, name := range []string{oldName, newName} {
		if err := ValidateContextName(name); err != nil {
			return false, err
		}
	}
	saved, ok := c.Contexts[oldName]
	if !ok {
		return false, fmt.Errorf("%w: run 'n8n config context list'", &NotFoundError{Name: oldName})
	}
	if oldName == newName {
		return false, nil
	}
	if _, exists := c.Contexts[newName]; exists {
		return false, fmt.Errorf("context %q already exists: choose an unused name; run 'n8n config context list'", newName)
	}
	c.Contexts[newName] = saved
	delete(c.Contexts, oldName)
	if c.CurrentContext == oldName {
		c.CurrentContext = newName
	}
	return true, nil
}

// Remove deletes a context and clears the selection when it pointed there.
// Removing a context does not remove its credential; that is [CredentialStore].
func (c *Config) Remove(name string) error {
	if _, ok := c.Contexts[name]; !ok {
		return &NotFoundError{Name: name}
	}
	delete(c.Contexts, name)
	if c.CurrentContext == name {
		c.CurrentContext = ""
	}
	return nil
}

// Validate checks a decoded file before anything trusts it.
func (c *Config) Validate() error {
	if c.Version > SchemaVersion {
		return fmt.Errorf("config version %d is newer than this CLI understands (%d): upgrade n8n-cli", c.Version, SchemaVersion)
	}
	for name, ctx := range c.Contexts {
		if err := ValidateContextName(name); err != nil {
			return err
		}
		if err := ctx.validate(name); err != nil {
			return err
		}
	}
	if c.CurrentContext != "" {
		if _, ok := c.Contexts[c.CurrentContext]; !ok {
			return fmt.Errorf("current context %q is not defined", c.CurrentContext)
		}
	}
	return nil
}

func (ctx Context) validate(name string) error {
	if ctx.URL == "" {
		return fmt.Errorf("context %q has no url", name)
	}
	if _, err := n8n.NormalizeBaseURL(ctx.URL); err != nil {
		return fmt.Errorf("context %q url: %w", name, err)
	}
	switch ctx.AuthType {
	case n8n.AuthAPIKey, n8n.AuthBearer, n8n.AuthCookie:
	default:
		return fmt.Errorf("context %q has unknown auth type %q", name, ctx.AuthType)
	}
	if ctx.CredentialRef == "" {
		return fmt.Errorf("context %q has no credential reference", name)
	}
	if !ctx.Storage.Valid() {
		return fmt.Errorf("context %q has unknown credential storage %q", name, ctx.Storage)
	}
	return nil
}

// maxContextName keeps a name usable as a JSON key, a keyring account and a
// shell argument.
const maxContextName = 64

// ValidateContextName rejects names that cannot be round-tripped through the
// metadata file, the OS credential store and shell completion.
func ValidateContextName(name string) error {
	if name == "" {
		return errors.New("context name is empty")
	}
	if len(name) > maxContextName {
		return fmt.Errorf("context name %q is longer than %d characters", name, maxContextName)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("context name %q may only contain letters, digits, '-', '_' and '.'", name)
		}
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("context name %q must not start with '.'", name)
	}
	return nil
}
