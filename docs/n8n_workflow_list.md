## n8n workflow list

List workflows

### Synopsis

List the workflows the credential can see, one cursor-paginated page at a time.
Pass a returned cursor to --cursor for the next page, or use --all to follow
every cursor, capped at 10,000 workflows.

Use this to find the workflow ID every other command takes. Narrow the result
set with --active, --name, --tag and --project-id. Each row carries the full
definition, so on a large instance prefer --exclude-pinned-data, which drops
the pinned sample data that dominates the response. Requires workflow:list.

--quiet prints only workflow IDs, one per line; --brief prints ID and name
separated by a tab. Both drop the header and summary so stdout pipes cleanly;
the next cursor, when there is one, goes to stderr. They are text-only views,
so they cannot be combined with each other or with --output json.

```
n8n workflow list [flags]
```

### Examples

```
  n8n workflow list
  n8n workflow list --all --brief --exclude-pinned-data
  n8n workflow list --tag production --quiet
  n8n workflow list --active true --limit 50
  n8n workflow list --tag production --tag finance
  n8n workflow list --project-id PROJECT_ID --exclude-pinned-data
  n8n workflow list --all --output json
```

### Options

```
      --active string         only published (true) or unpublished (false) workflows (default: both)
      --all                   follow every page instead of one (maximum 10,000 workflows)
      --brief                 print only ID and name, tab-separated, with no header (default: full table)
      --context string        saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string         pagination cursor returned by a previous list (default: the first page)
      --exclude-pinned-data   leave pinned sample data out of the response (default: include it)
  -h, --help                  help for list
      --limit int             workflows per API page, 1 to 250 (default: server default of 100)
      --name string           only workflows with exactly this name (default: any name)
      --offset int            number of workflows to skip before the page (default: start at the first workflow)
      --output string         output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --project-id string     only workflows in this project, from 'n8n project list' (default: every project)
      --quiet                 print only workflow IDs, one per line, with no header (default: full table)
      --tag stringArray       only workflows carrying this tag name, repeatable and combined with AND (default: any tag)
      --url string            instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

