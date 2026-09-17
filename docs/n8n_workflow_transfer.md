## n8n workflow transfer

Move a workflow to another project

### Synopsis

Move one workflow into another project, given by --destination-project-id.

Access follows the project, so everyone with access to the old project loses it
and everyone with access to the new one gains it. Credentials do not move with
the workflow: a credential that is not shared with the destination project
stops resolving, which makes a published workflow fail at run time. Check the
credentials first with 'n8n credential list'.

The API answers with no content, so the workflow is read back afterwards to
report where it ended up. Interactive runs ask for confirmation;
non-interactive runs require --yes. Requires workflow:move.

```
n8n workflow transfer <workflow-id> [flags]
```

### Examples

```
  n8n workflow transfer WORKFLOW_ID --destination-project-id PROJECT_ID
  n8n workflow transfer WORKFLOW_ID --destination-project-id PROJECT_ID --yes
  n8n workflow transfer WORKFLOW_ID --destination-project-id PROJECT_ID --yes --output json
```

### Options

```
      --context string                  saved context to use (default: the current context; see 'n8n config context list')
      --destination-project-id string   project to move the workflow into, from 'n8n project list' (required)
  -h, --help                            help for transfer
      --output string                   output format: text or json (JSON names the workflow, the destination project and the action) (default "text")
      --url string                      instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes                             confirm the move without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

