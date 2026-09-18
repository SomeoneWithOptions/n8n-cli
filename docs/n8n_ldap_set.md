## n8n ldap set

Fully replace LDAP configuration

### Synopsis

Fully replace the instance LDAP configuration from strict JSON. This is a PUT,
not a patch: all 20 fields are required, including false, zero, and empty values.
Start from 'n8n ldap get --output json'. Leave the returned password placeholder
unchanged to preserve the stored bind password, replace it to set a new password,
or use an empty string to clear it. Input files therefore require secret-safe
permissions. Response details are redacted on errors.

Every replacement asks for confirmation. Setting loginEnabled=false is high risk:
n8n deletes every stored LDAP identity and disables synchronization. Non-interactive
use and --input - require --yes. Requires ldap:manage and the LDAP license.

```
n8n ldap set [flags]
```

### Examples

```
  umask 077 && n8n ldap get --output json > ldap.json
  n8n ldap set --input ldap.json
  n8n ldap set --input ldap.json --yes --output json
  n8n ldap set --input - --yes < ldap.json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full LDAP configuration JSON file path, or - for stdin (required; maximum 1 MiB; may contain a bind password)
      --output string    output format: text or json (password is returned only as an n8n placeholder) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm full replacement without prompting; also accepts destructive identity deletion when loginEnabled=false (required for non-interactive use)
```

### SEE ALSO

* [n8n ldap](n8n_ldap.md)	 - Manage licensed LDAP settings and synchronization

