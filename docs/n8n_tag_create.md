## n8n tag create

Create a tag

### Synopsis

Create one tag with the given name. Name is the only field a tag has; the ID and
timestamps are assigned by the instance and printed on success.

Names are unique per instance, so reusing an existing name fails with a conflict.
n8n also limits how long a name may be (24 characters on current versions) and
reports a longer one as the same conflict. Run 'n8n tag list' first when unsure,
and quote names containing spaces. Requires tag:create.

```
n8n tag create <name> [flags]
```

### Examples

```
  n8n tag create Production
  n8n tag create "Customer facing" --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for create
      --output string    output format: text or json (JSON is the tag object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n tag](n8n_tag.md)	 - Manage workflow tags

