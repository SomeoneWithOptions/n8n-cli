## n8n auth login

Store a credential for an n8n instance

### Synopsis

Store a credential for an n8n instance and select its context.

Use this first: no resource command works until one login succeeds. The
credential is read from a no-echo prompt, or from stdin with --stdin. It is
never accepted as a flag or an argument, because process lists and shell history
would expose it. It is validated against GET /api/v1/discover before anything
is saved, so a typo or revoked key saves nothing.

Credentials are stored in the operating system credential store. With
--storage=file they are stored in auth.json instead, which is plaintext protected
only by file permissions, not by encryption. Environment credentials
(N8N_API_KEY and friends) override the saved one for a single run and are
never persisted by this command.

Next step: 'n8n auth status --check'.

```
n8n auth login [flags]
```

### Examples

```
  # Interactive login (prompts for the API key):
  n8n auth login --url https://n8n.example.com

  # Non-interactive login from a secret manager or pipe:
  printf '%s' "$N8N_API_KEY" | n8n auth login --url https://n8n.example.com --stdin

  # Named context with an explicit auth type and plaintext fallback:
  n8n auth login --url https://n8n.example.com --context production --type api-key --storage=file
```

### Options

```
      --context string   context name to create or replace (default "default")
  -h, --help             help for login
      --stdin            read the credential from stdin instead of prompting (use for scripts and AI agents)
      --storage string   credential storage: keyring or file (default keyring; file is plaintext, permissions-only protection)
      --type string      authentication type: api-key, bearer or cookie (default api-key) (default "api-key")
      --url string       instance URL, e.g. https://n8n.example.com (falls back to N8N_URL, then saved context URL)
      --yes              replace the existing credential without asking
```

### SEE ALSO

* [n8n auth](n8n_auth.md)	 - Log in to an n8n instance and inspect stored credentials

