## n8n oidc

Manage licensed OIDC SSO settings

### Synopsis

Read and fully replace the instance OIDC single sign-on configuration. Start
with 'get --output json', keep the client-secret placeholder unchanged unless
replacing the secret, then pass the complete edited document to 'set'. The
stored client secret is never rendered in plaintext. Protect any file where
you enter a replacement secret.

Every replacement requires confirmation because it can immediately change the
instance login flow. Requires oidc:manage and the OIDC license. Configuration
managed by environment variables is readable but cannot be replaced through
the API.

```
n8n oidc [flags]
```

### Examples

```
  n8n oidc get
  umask 077 && n8n oidc get --output json > oidc.json
  n8n oidc set --input oidc.json
  n8n oidc set --input oidc.json --yes --output json
```

### Options

```
  -h, --help   help for oidc
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n oidc get](n8n_oidc_get.md)	 - Get OIDC SSO configuration
* [n8n oidc set](n8n_oidc_set.md)	 - Fully replace OIDC SSO configuration

