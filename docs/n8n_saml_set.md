## n8n saml set

Fully replace SAML SSO configuration

### Synopsis

Fully replace the instance SAML SSO configuration from strict JSON. This is
a PUT, not a patch: all 15 writable top-level fields and every nested mapping
and signature field are required, including false, empty-string, and empty-array
values. Start from 'n8n saml get --output json'. Read-only entityID and returnUrl
are accepted for that round trip but ignored on set. Leave returned redaction
placeholders unchanged to preserve metadata, private key, and certificate; use
empty strings to clear supported values or plaintext to replace them.

Every replacement asks for confirmation because changing SAML settings can alter
the login flow immediately. Non-interactive use and --input - require --yes.
Protect files containing replacement metadata, keys, or certificates. Server
error details are redacted. Requires saml:manage and the SAML license.
Environment-managed configuration is rejected with 409.

```
n8n saml set [flags]
```

### Examples

```
  umask 077 && n8n saml get --output json > saml.json
  n8n saml set --input saml.json
  n8n saml set --input saml.json --yes --output json
  n8n saml set --input - --yes < saml.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full SAML configuration JSON file path, or - for stdin (required; maximum 1 MiB; may contain private material)
      --output string    output format: text or json (secret-bearing values are returned only as n8n placeholders) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm full replacement and possible login-flow changes without prompting (required for non-interactive use)
```

### SEE ALSO

* [n8n saml](n8n_saml.md)	 - Manage licensed SAML SSO settings

