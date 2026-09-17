## n8n workflow history

List the saved versions of a workflow

### Synopsis

List the version history of one workflow, newest first, one cursor-paginated
page at a time. Each entry is metadata only: the version ID, who saved it, and
when.

Use it to find the version ID that 'n8n workflow version get' reads and that
'n8n workflow publish --version-id' puts live. Pass a returned cursor to
--cursor for the next page, or use --all to follow every cursor, capped at
10,000 versions. Requires workflow:read.

```
n8n workflow history <workflow-id> [flags]
```

### Examples

```
  n8n workflow history WORKFLOW_ID
  n8n workflow history WORKFLOW_ID --limit 5
  n8n workflow history WORKFLOW_ID --all --output json
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 versions)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous page (default: the first page)
  -h, --help             help for history
      --limit int        versions per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

