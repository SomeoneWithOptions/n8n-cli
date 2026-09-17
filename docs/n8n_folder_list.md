## n8n folder list

List the folders of a project

### Synopsis

List folders in one project. This endpoint pages by offset, not by cursor: ask
for a page with --take, move through the result set with --skip, or use --all to
walk every page, capped at 10,000 folders. The reported count is the number of
folders matching the query, which is larger than one page.

Narrow the result set with --parent-folder-id, --name, --tag, and
--exclude-folder-id, or pass the whole documented filter object as JSON with
--filter; the two ways are mutually exclusive. --select limits the returned
fields, which is worth it on large projects; note that the 'project', 'tags',
and 'parentFolder' select fields are answered with a 500 by some instances, so
prefer leaving --select off when in doubt, since the full response already
carries them. Requires folder:list.

```
n8n folder list <project-id> [flags]
```

### Examples

```
  n8n folder list PROJECT_ID
  n8n folder list PROJECT_ID --take 50 --sort-by name:asc
  n8n folder list PROJECT_ID --skip 50 --take 50 --output json
  n8n folder list PROJECT_ID --parent-folder-id FOLDER_ID
  n8n folder list PROJECT_ID --name Invoices --tag finance
  n8n folder list PROJECT_ID --filter '{"name":"Invoices"}'
  n8n folder list PROJECT_ID --select id --select name --all
```

### Options

```
      --all                        follow every page instead of one (maximum 10,000 folders)
      --context string             saved context to use (default: the current context; see 'n8n config context list')
      --exclude-folder-id string   drop this folder and everything under it (cannot combine with --filter)
      --filter string              whole filter object as JSON, e.g. '{"name":"Invoices"}' (cannot combine with the single-filter flags)
  -h, --help                       help for list
      --name string                only folders whose name matches this value (cannot combine with --filter)
      --output string              output format: text or json (JSON is an object with count and data) (default "text")
      --parent-folder-id string    only folders directly inside this folder (cannot combine with --filter)
      --select stringArray         field to return, repeatable: one of id, name, createdAt, updatedAt, project, tags, parentFolder, workflowCount, subFolderCount, path (default: every field)
      --skip int                   number of folders to skip before the page (default: start at the first folder)
      --sort-by string             sort order: one of name:asc, name:desc, createdAt:asc, createdAt:desc, updatedAt:asc, updatedAt:desc (default: the server's order)
      --tag stringArray            only folders carrying this tag name, repeatable (cannot combine with --filter)
      --take int                   folders per API page (default: server default of 10; 100 with --all)
      --url string                 instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n folder](n8n_folder.md)	 - Manage the folders of a project

