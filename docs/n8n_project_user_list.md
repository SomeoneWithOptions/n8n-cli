## n8n project user list

List the members of a project

### Synopsis

List every member of one project with the project role each of them holds. The
response is cursor-paginated; pass a returned cursor to --cursor, or use --all to
follow every cursor, capped at 10,000 members.

This endpoint needs the user:list scope on top of project access, so a credential
that can read the project may still be refused here. Use the returned user IDs
with 'n8n project user role set' and 'n8n project user remove'.

```
n8n project user list <project-id> [flags]
```

### Examples

```
  n8n project user list PROJECT_ID
  n8n project user list PROJECT_ID --limit 50 --output json
  n8n project user list PROJECT_ID --all
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 members)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        members per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n project user](n8n_project_user.md)	 - Manage project members

