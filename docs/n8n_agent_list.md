## n8n agent list

List Agents

### Synopsis

List the Agents in the projects where you hold agent:list, one cursor page at
a time. --project-id narrows the list to one project ('n8n project list'
finds it). Text shows ID, name, project, whether the Agent is published
and archived, and the update time. Requires agent:list and n8n 2.49 or
later.

Script views print rows only, with no header, count or blank lines, so stdout
pipes cleanly; the next cursor and any empty-result hint go to stderr, and an
empty result prints nothing:
  --quiet  one agent ID per line, printed exactly
  --brief  ID and name, separated by a literal tab
--brief escapes backslash, tab, newline, carriage return and other control
characters in fields as \\, \t, \n, \r and \uXXXX; use --output json for exact
values. The views combine neither with each other nor with --output json.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n agent list [flags]
```

### Examples

```
  n8n agent list
  n8n agent list --project-id PROJECT_ID --all
  n8n agent list --quiet
  n8n agent list --limit 50 --output json
```

### Options

```
      --all                 follow every page instead of one (maximum 10,000 Agents; hitting it warns and sets JSON collection.truncated)
      --brief               print only ID and name, tab-separated and escaped, with no header (default: full table)
      --context string      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string       pagination cursor returned by a previous list (default: the first page)
  -h, --help                help for list
      --limit int           Agents per API page, 1 to 250 (default: server default of 100)
      --output string       output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all) (default "text")
      --project-id string   only list Agents of this project (default: every project you can list)
      --quiet               print only agent IDs, one per line, with no header (default: full table)
      --url string          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n agent](n8n_agent.md)	 - Manage Agents

