## n8n project list

List projects

### Synopsis

List the projects visible to the selected credential, one cursor-paginated page
at a time. The page carries a next cursor; pass it to --cursor for the following
page, or use --all to follow every cursor, capped at 10,000 projects.

Use this to find the project ID that update, delete, and every 'project user'
command takes. Personal projects appear with type 'personal'. Requires
project:list and a licensed instance.

```
n8n project list [flags]
```

### Examples

```
  n8n project list
  n8n project list --limit 50 --output json
  n8n project list --cursor NEXT_CURSOR
  n8n project list --all
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 projects)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        projects per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n project](n8n_project.md)	 - Manage projects and their members

