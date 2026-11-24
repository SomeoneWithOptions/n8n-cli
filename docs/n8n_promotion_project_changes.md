## n8n promotion project changes

Preview the changes of a project's promotion branch

### Synopsis

Compare a team project with its resolved promotion branch without changing content.
Start with 'n8n project list' for the project ID. The project's own
environment (n8n 2.46 and later) or connection (older) for the direction
takes precedence over the instance one. Prepare the checkout first: 'n8n
promotion environment connect' on 2.46, 'n8n promotion checkout clone'
before. Apply requires promotions:apply; promote requires
promotions:promote (gitConnection:pull and gitConnection:push before
n8n 2.47).

n8n 2.49 and later list every changed project, folder, workflow, credential,
data table, variable, and tag, plus unchanged parents for context. Text
shows each resource's key, type, ID, name, change, whether it can be
selected (--workflow-id or --folder-id), and for workflows how many
resources they use. Apply previews also show what blocks the Apply and the
preview revision; promote previews show an open review that holds the
changes. Read the content of changed resources with 'n8n promotion project
change-details'. Older instances list changed workflows with their status
and dependency count, and accept --search, --sort, and --order.

JSON is the full response, including source (environmentId on 2.46,
configId before) and nullable commitSha. A workflow that moved between
projects is refused by promote until it moves back. Save apply JSON for
--expected-from to reject a branch, and on 2.49 an instance, that changes
after review.

```
n8n promotion project changes <project-id> <apply|promote> [flags]
```

### Examples

```
  n8n promotion project changes PROJECT_ID apply --output json > changes.json
  n8n promotion project changes PROJECT_ID promote
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for changes
      --output string    output format: text or json (default: text; JSON includes the reviewed source) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

