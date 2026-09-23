## n8n workflow copy

Copy a saved workflow definition from one context to another

### Synopsis

Copy one saved workflow definition between two saved contexts, for example
staging to production, or production back to staging for a rollback. The source
instance is only ever read; every write lands on the target.

No API operation copies a workflow between instances, so this composes the ones
that exist: it reads the source with 'get' (resolving --name through 'list'),
then creates or fully replaces the target, and publishes it only with --publish.

Preview the same pair with 'n8n workflow diff', which writes nothing. 'get' and
'update' are the single-instance edit loop for one context. 'n8n promotion
promote' and 'n8n promotion apply' are a different machine: Git-remote batch
sync of whole projects through a source-control connection, not this.

Both contexts must be saved ('n8n auth login --context NAME'); this command uses
each context's saved URL and credential and ignores N8N_CONTEXT, N8N_URL and
environment credentials on both sides. Needs workflow:read on the source, plus workflow:list
when resolving --name, and workflow:read plus workflow:create and/or
workflow:update on the target. --publish additionally needs workflow:activate
and the project's workflow:publish permission on the target.

The target is created when it cannot be resolved by name, and otherwise fully
replaced: nodes, connections and settings the source omits are cleared on the
target. Pass --create-only or --update-only to allow only one of the two. An
existing target keeps its published version live unless --publish is given, so
a copy into production saves a draft by default. Tags, credentials, folders and
sharing do not transfer: their IDs are per instance. Pinned sample data and
runtime static data are excluded unless --include-pinned-data or
--include-static-data is passed. Refuses to run when both contexts resolve to
the same instance, and when either workflow is archived.

stdout carries the result only: a text summary, or with --output json a
versioned object with action, from, to, publish, droppedFields and warnings.
Fields not part of the write schema, the warnings and the prompt go to stderr.

Verify afterwards with 'n8n workflow diff', and put the copy live with
'n8n workflow publish' when --publish was not used.

```
n8n workflow copy [flags]
```

### Examples

```
  n8n workflow copy --from-context staging --from-id AAA --to-context prod --to-id BBB
  n8n workflow copy --name "Invoice sync" --from-context staging --to-context prod --to-project-id PROD --yes --output json
  n8n workflow copy --name "Invoice sync" --from-context prod --to-context staging --yes
  n8n workflow copy --from-context staging --from-id AAA --to-context prod --to-name "Invoice sync (prod)" --publish --yes
```

### Options

```
      --create-only                  fail instead of replacing an existing target workflow
      --from-context string          source saved context (required; ignores global environment overrides)
      --from-id string               source workflow ID (required unless --name is given)
      --from-project-id string       limit source name lookup to this project (requires --name)
  -h, --help                         help for copy
      --include-pinned-data          copy pinned sample data (default: excluded)
      --include-static-data          copy runtime static data such as poll cursors (default: excluded)
      --name string                  exact source workflow name; cannot be combined with --from-id
      --output string                output format: text or json (versioned copy result) (default "text")
      --publish                      publish the target after a successful write (default: write only, the live version is unchanged)
      --to-context string            target saved context (required; ignores global environment overrides)
      --to-id string                 target workflow ID; omit to resolve the target by name, creating it when absent
      --to-name string               name to write on the target (default: the source workflow name)
      --to-parent-folder-id string   destination folder when the target is created; rejected when the target exists
      --to-project-id string         limit target name lookup to this project, and the destination project when the target is created
      --update-only                  fail instead of creating a missing target workflow
      --yes                          confirm the copy without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

