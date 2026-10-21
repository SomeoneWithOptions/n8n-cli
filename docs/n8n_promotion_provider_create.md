## n8n promotion provider create

Create a promotion provider

### Synopsis

Create one promotion provider from flags or from strict JSON supplied by
--input; the two ways are mutually exclusive and one is required.

Type git is a plain Git remote. Type gitlab is a Git host provider: the
server checks API access before saving and can list repositories. Pass
--base-url for self-managed GitLab; it becomes config {"schemaVersion":1,
"baseUrl":"..."} and applies to gitlab providers only.

Auth is ssh-key (the response carries the generated public key; add it
to the remote as a deploy key) or token. A token is never a flag: the
flag way asks for it without echo, or reads it from stdin with
--password-stdin. A git provider needs --username with a token; GitLab
ignores the username and uses n8n for Git transport.

The --input document has name (at most 128 characters), type (git or
gitlab), auth {"authType":"ssh-key"} with an optional keyType
(ed25519 or rsa, default ed25519) or {"authType":"token",
"password":"..."} plus username for git, and the optional config.
Unknown fields and trailing JSON documents are refused before
transport. Requires gitConnection:create. Follow with 'n8n promotion
connection create'.

```
n8n promotion provider create [flags]
```

### Examples

```
  n8n promotion provider create --name github --type git --auth-type ssh-key
  printf '%s\n' "$GITLAB_TOKEN" | n8n promotion provider create --name gitlab --type gitlab --base-url https://gitlab.example.com --auth-type token --password-stdin
  n8n promotion provider create --input provider.json --output json
```

### Options

```
      --auth-type string   auth type: ssh-key or token (default: ssh-key)
      --base-url string    GitLab base URL for self-managed GitLab, e.g. https://gitlab.example.com; gitlab only (default: none, no config is sent)
      --context string     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help               help for create
      --input string       promotion provider JSON file path, or - for stdin (maximum 1 MiB; default: build the document from --name and the other flags)
      --key-type string    SSH key type for ssh-key auth: ed25519 or rsa (default: the server's ed25519)
      --name string        provider name, 1 to 128 characters (required without --input)
      --output string      output format: text or json (JSON is the creation result: provider plus publicKey) (default "text")
      --password-stdin     read the token for token auth from stdin instead of prompting without echo (default false; required when stdin is not a terminal)
      --type string        provider type: git or gitlab (default: git)
      --url string         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --username string    Git username for token auth; required for git, ignored by gitlab (default: none)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

