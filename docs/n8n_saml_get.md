## n8n saml get

Get SAML SSO configuration

### Synopsis

Get every SAML SSO setting exposed by n8n, including the read-only service
provider entity ID and ACS return URL. Identity-provider metadata, signing
private keys, and signing certificates are empty when unset or replaced by
n8n's redacted placeholder when configured. Keep placeholders unchanged in a
GET-edit-PUT workflow. Text output only reports secret status.

Requires saml:manage and the SAML license.

```
n8n saml get [flags]
```

### Examples

```
  n8n saml get
  n8n saml get --output json
  umask 077 && n8n saml get --output json > saml.json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (secret-bearing values are placeholders, never stored plaintext) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n saml](n8n_saml.md)	 - Manage licensed SAML SSO settings

