## n8n community-package list

List installed community packages

### Synopsis

List community node packages installed on the selected n8n instance.

Text output shows installed and available versions, load status, and node
count. --output json emits the API array with package authors, nodes and
timestamps for scripts. This command changes nothing and needs no confirmation.

Requires API-key authentication and the communityPackage:list scope; check
availability with 'n8n discover --resource communitypackage'.

```
n8n community-package list [flags]
```

### Examples

```
  n8n community-package list
  n8n community-package list --output json
  n8n community-package list --context production
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (default text; json is stable for scripting) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n community-package](n8n_community-package.md)	 - Manage community node packages

