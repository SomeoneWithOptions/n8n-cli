## n8n tag get

Get one tag

### Synopsis

Get one tag by ID, as listed in the ID column of 'n8n tag list'. A tag that does
not exist answers 404. This is a read-only call; requires tag:read.

```
n8n tag get <tag-id> [flags]
```

### Examples

```
  n8n tag get TAG_ID
  n8n tag get TAG_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the tag object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n tag](n8n_tag.md)	 - Manage workflow tags

