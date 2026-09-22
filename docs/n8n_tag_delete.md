## n8n tag delete

Permanently delete a tag

### Synopsis

Permanently delete one tag. The tag is removed from every workflow that carries
it; those workflows keep working and are not deleted. The deletion cannot be
undone, and recreating the name produces a new ID.

Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires tag:delete.

```
n8n tag delete <tag-id> [flags]
```

### Examples

```
  n8n tag delete TAG_ID
  n8n tag delete TAG_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the tag object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n tag](n8n_tag.md)	 - Manage workflow tags

