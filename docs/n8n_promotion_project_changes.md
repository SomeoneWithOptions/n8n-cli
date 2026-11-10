## n8n promotion project changes

List workflow changes for a project's promotion branch

### Synopsis

Compare a team project with its resolved promotion branch without changing content.
Start with 'n8n project list' for the project ID. The project's own
environment (n8n 2.46 and later) or connection (older) for the direction
takes precedence over the instance one. Prepare the checkout first: 'n8n
promotion environment connect' on 2.46, 'n8n promotion checkout clone'
before. Apply requires promotions:apply; promote requires
promotions:promote (gitConnection:pull and gitConnection:push before
n8n 2.47). Text lists workflow IDs, statuses, and dependency
counts; JSON includes changes, source (environmentId on 2.46, configId
before), and nullable commitSha. On 2.46 a deleted or new promote row can
be a workflow that moved between projects; promoting it is refused until
it moves back. Review the IDs before project apply or promote. Save apply
JSON for --expected-from to reject a branch that moves after review.

```
n8n promotion project changes <project-id> <apply|promote> [flags]
```

### Examples

```
  n8n promotion project changes PROJECT_ID apply --output json > changes.json
  n8n promotion project changes PROJECT_ID promote --sort updatedAt --order desc
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for changes
      --order string     sort order: asc or desc (default: asc) (default "asc")
      --output string    output format: text or json (default: text; JSON includes the reviewed source) (default "text")
      --search string    workflow name search string (default: no filter)
      --sort string      sort field: name, updatedAt, or status (default: name) (default "name")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

