## n8n workflow dependencies

List what a workflow uses and what uses it

### Synopsis

List the dependencies of one workflow in both directions: the credentials, data
tables, sub-workflows and error workflow it uses, and the workflows and agents
that use it, as a sub-workflow or as their error workflow. Check this before
archiving, deleting or transferring a workflow. Requires workflow:read.

TYPE is the relation to the requested resource:
  credential, dataTable, subWorkflow, errorWorkflow
      the requested workflow uses this resource
  parentWorkflow       this workflow uses the requested resource
  errorWorkflowParent  this workflow uses the requested workflow as its error
                       workflow
  agent                this agent uses the requested resource

Dependencies the account in use cannot access are not listed; the summary
counts them as Inaccessible (JSON: inaccessibleCount, over the whole list, not
one page). Pass a returned cursor to --cursor for the next page, or use --all
to follow every cursor, capped at 10,000 dependencies.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n workflow dependencies <workflow-id> [flags]
```

### Examples

```
  n8n workflow dependencies WORKFLOW_ID
  n8n workflow dependencies WORKFLOW_ID --all --output json
  n8n workflow dependencies WORKFLOW_ID --limit 50 --cursor CURSOR
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 dependencies; hitting it warns and sets JSON collection.truncated)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous page (default: the first page)
  -h, --help             help for dependencies
      --limit int        dependencies per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data, nextCursor and inaccessibleCount, plus collection with --all) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

