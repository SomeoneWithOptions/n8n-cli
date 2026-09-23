## n8n auth login

Store a credential for an n8n instance

### Synopsis

Store a credential for an n8n instance and select its context.

The context name comes from --context, then N8N_CONTEXT, then default. Unlike
resource commands, login does not fall back to the saved current context. A
successful login selects the saved context, including when named by the environment.

Use this to save a credential; environment credentials also work without login. The
credential is read from a no-echo prompt, or from stdin with --stdin. It is
never accepted as a flag or an argument, because process lists and shell history
would expose it. It is validated against GET /api/v1/discover before anything
is saved, so a typo or revoked key saves nothing by default.

Use --skip-verify for offline setup, instances without /discover, or keys that
lack discovery access. This skips only the remote check: local validation,
storage consent and replacement confirmation still apply. A wrong URL or invalid
credential can be saved; access is checked on the first resource request.
This command saves local configuration and never changes remote resources.

Credentials are stored in the operating system credential store. With
--storage=file they are stored in auth.json instead, which is plaintext protected
only by file permissions, not by encryption. Environment credentials
(N8N_API_KEY and friends) override the saved one for a single run and are
never persisted by this command.

Each login saves a fresh credential reference, independent of context names.
A crash during saving or cleanup can leave an unused credential in storage.
Older CLI versions can reuse name-based references: avoid writing this shared
configuration with older binaries after renaming contexts.

Next step: 'n8n auth status --check'. After --skip-verify, inspect saved state
with 'n8n auth status', then run a resource command your key permits, such as
'n8n user list --limit 1' for user:list. Discovery and auth status --check
still require access to /discover.

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

  # Save a narrowly scoped key without discovery verification:
  n8n auth login --url https://n8n.example.com --context limited --skip-verify
```

### Options

```
      --context string   context name to create or replace (falls back to N8N_CONTEXT, then "default")
  -h, --help             help for login
      --skip-verify      skip the remote /discover credential check (default false; saves without verifying access)
      --stdin            read the credential from stdin instead of prompting (use for scripts and AI agents)
      --storage string   credential storage: keyring or file (default keyring; file is plaintext, permissions-only protection)
      --type string      authentication type: api-key, bearer or cookie (default api-key) (default "api-key")
      --url string       instance URL, e.g. https://n8n.example.com (falls back to N8N_URL, then saved context URL)
      --yes              replace the existing credential without asking
```

### SEE ALSO

* [n8n auth](n8n_auth.md)	 - Log in to an n8n instance and inspect stored credentials

