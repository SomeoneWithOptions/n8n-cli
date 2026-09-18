## n8n promotion checkout clone

Clone one promotion checkout locally

### Synopsis

Clone one direction checkout of a promotion connection into local
storage. The direction argument is apply or promote.

Cloning is safe to repeat and changes no project content by itself:
it only prepares the checkout that 'project add', 'promote', and
'apply' work against, so it needs no confirmation. Clone before
promoting or applying. Requires gitConnection:clone.

```
n8n promotion checkout clone <connection-id> <direction> [flags]
```

### Examples

```
  n8n promotion checkout clone CONNECTION_ID apply
  n8n promotion checkout clone CONNECTION_ID promote --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for clone
      --output string    output format: text or json (JSON is the checkout returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n promotion checkout](n8n_promotion_checkout.md)	 - Manage promotion checkouts

