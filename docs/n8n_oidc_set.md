## n8n oidc set

Fully replace OIDC SSO configuration

### Synopsis

Fully replace the instance OIDC SSO configuration from strict JSON. This is
a PUT, not a patch: all nine fields are required, including false, empty-string,
and empty-array values. Start from 'n8n oidc get --output json'. Leave the
returned client-secret placeholder unchanged to preserve the stored secret, or
replace it to set a new secret. An empty client secret is not accepted.

Every replacement asks for confirmation because changing OIDC settings can alter
the login flow immediately. Non-interactive use and --input - require --yes.
Input files containing a replacement secret need secret-safe permissions, and
server response details are redacted on errors. Requires oidc:manage and the
OIDC license. Environment-managed configuration is rejected with 409.

```
n8n oidc set [flags]
```

### Examples

```
  umask 077 && n8n oidc get --output json > oidc.json
  n8n oidc set --input oidc.json
  n8n oidc set --input oidc.json --yes --output json
  n8n oidc set --input - --yes < oidc.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full OIDC configuration JSON file path, or - for stdin (required; maximum 1 MiB; may contain a client secret)
      --output string    output format: text or json (client secret is returned only as an n8n placeholder) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm full replacement and possible login-flow changes without prompting (required for non-interactive use)
```

### SEE ALSO

* [n8n oidc](n8n_oidc.md)	 - Manage licensed OIDC SSO settings

