## n8n tag update

Rename a tag

### Synopsis

Rename one tag. The API replaces the whole tag, but name is its only writable
field, so --name is the full replacement document and nothing else is lost: the
ID stays the same and every workflow keeps the tag.

No read-modify-write step is needed. Names are unique per instance, so a name
already in use fails with a conflict, as does a name longer than the n8n limit
(24 characters on current versions). Requires tag:update.

```
n8n tag update <tag-id> [flags]
```

### Examples

```
  n8n tag update TAG_ID --name Staging
  n8n tag update TAG_ID --name "Customer facing" --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --name string      new tag name, replacing the current one (required)
      --output string    output format: text or json (JSON is the tag object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n tag](n8n_tag.md)	 - Manage workflow tags

