## n8n auth status

Show the active context, instance and credential state

### Synopsis

Show the active context, instance and credential state.

Use this to answer 'why did my command fail with auth errors': it shows
which context is current, which URL and auth type it points at, where the
credential came from (keyring, file, or environment), and whether one is
present. No credential value is ever printed. With --check the credential
is used once against GET /api/v1/discover to report whether the instance
still accepts it; --check fails the command when the credential is missing
or rejected, so scripts and agents can gate on it.

```
n8n auth status [flags]
```

### Examples

```
  n8n auth status
  n8n auth status --check
  n8n auth status --context production --output json
```

### Options

```
      --check            validate the credential against the instance; exit non-zero when unusable
      --context string   context to describe (default: the current context)
  -h, --help             help for status
      --output string    output format: text or json (default text; json is stable for scripting) (default "text")
```

### SEE ALSO

* [n8n auth](n8n_auth.md)	 - Log in to an n8n instance and inspect stored credentials

