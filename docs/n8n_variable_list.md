## n8n variable list

List variables and their values

### Synopsis

List variables visible to the selected credential, including their values, one
cursor-paginated page at a time. Use --project-id to select one project or
--state empty to find variables with an empty value. Use --all to follow every
cursor, capped at 10,000 variables.

Values are requested output: text prints quoted values and JSON preserves exact
strings. Keep output and redirected files private. Requires variable:list.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n variable list [flags]
```

### Examples

```
  n8n variable list
  n8n variable list --project-id PROJECT_ID --limit 50
  n8n variable list --state empty --output json
  n8n variable list --all --output json
```

### Options

```
      --all                 follow every page instead of one (maximum 10,000 variables; hitting it warns and sets JSON collection.truncated)
      --context string      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string       pagination cursor returned by a previous list (default: the first page)
  -h, --help                help for list
      --limit int           variables per API page, 1 to 250 (default: server default of 100)
      --output string       output format: text or json (JSON is a page object containing exact variable values, plus collection with --all) (default "text")
      --project-id string   return variables for this project ID (default: all visible scopes)
      --state string        filter by value state: empty (default: all values)
      --url string          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n variable](n8n_variable.md)	 - Manage instance and project variables

