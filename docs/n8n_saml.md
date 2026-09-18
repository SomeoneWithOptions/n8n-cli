## n8n saml

Manage licensed SAML SSO settings

### Synopsis

Read and fully replace the instance SAML single sign-on configuration. Start
with 'get --output json', leave redacted metadata, private-key, and certificate
placeholders unchanged to preserve stored values, then pass the complete edited
document to 'set'. Read-only entityID and returnUrl values are accepted but not
sent. Stored secret-bearing values are never rendered in plaintext.

Every replacement requires confirmation because it can immediately change the
instance login flow. Requires saml:manage and the SAML license. Configuration
managed by environment variables is readable but cannot be replaced via API.

```
n8n saml [flags]
```

### Examples

```
  n8n saml get
  umask 077 && n8n saml get --output json > saml.json
  n8n saml set --input saml.json
  n8n saml set --input saml.json --yes --output json
```

### Options

```
  -h, --help   help for saml
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n saml get](n8n_saml_get.md)	 - Get SAML SSO configuration
* [n8n saml set](n8n_saml_set.md)	 - Fully replace SAML SSO configuration

