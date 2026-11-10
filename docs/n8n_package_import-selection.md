## n8n package import-selection

Import chosen workflows from one project of an n8n package (beta)

### Synopsis

Cherry-pick workflows from one source project of a project package
(.n8np) instead of importing the whole package. Send the archive with
--file FILE or --file - for stdin, name the source project with
--project-id as it appears in the package, and list the source workflows
to import with --workflow-id. The workflows are written into the
instance project with that same ID, which is created if it does not
exist yet; there is no target-project or folder flag.

Use this instead of 'n8n package import' to promote a few workflows
without touching the rest of the project. The import is additive: it
never removes a workflow the selection omits, and it deletes only the
destination workflows listed with --delete-workflow-id (archived by
default; --overwrite-deletion-policy hard-delete also drops their
executions permanently, so prefer archive). --workflow-id may be omitted
for a delete-only run. Selected IDs from another project are dropped,
absent delete IDs are a no-op, and an ID cannot be both imported and
deleted. Folder, tag, and project policies are fixed by the server; only
the conflict, ID, and deletion policies are settable, and publication
follows match-source. A workflow package is rejected with a 400: export a
project package with 'n8n package export --project-id'. Packages that
contain Agents are rejected before writes on n8n 2.47 and later: import
selected-Agent exports with 'n8n package import'. n8n 2.47 renamed
--workflow-conflict-policy and --workflow-id-policy to --conflict-policy
and --id-policy (the old flags remain as deprecated aliases); older
instances get the old field names automatically, which needs --file
PATH because stdin cannot be sent twice.

Beta: breaking changes may still occur without a major version bump.
This mutates the instance, so interactive runs ask for confirmation and
non-interactive runs require --yes; --file - also requires --yes because
stdin cannot answer. Needs workflow:import, plus workflow:delete when
deleting or when an update changes a workflow's archive state,
workflow:activate or workflow:deactivate when match-source changes
publication, and the licensed Packages feature. Output is the same
import result as
'n8n package import'.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr). A retry must send
the same archive under the same file name with the same flags.

```
n8n package import-selection [flags]
```

### Examples

```
  n8n package import-selection --file project.n8np --project-id Ox8O54VQrmBrb4qL --workflow-id 2tUt1wbLX592XDdX --workflow-id 7pQr3sTuVwXyZaBc --yes
  n8n package import-selection --file project.n8np --project-id Ox8O54VQrmBrb4qL --workflow-id 2tUt1wbLX592XDdX --delete-workflow-id 9xKp2mNqRzAbCdEf --conflict-policy fail --output json --yes
  n8n package import-selection --file project.n8np --project-id Ox8O54VQrmBrb4qL --delete-workflow-id 9xKp2mNqRzAbCdEf --overwrite-deletion-policy hard-delete
```

### Options

```
      --conflict-policy string             selected workflow that already exists in the target project: new-version updates it, fail rejects the import, skip leaves it unchanged (default: server default new-version)
      --context string                     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --delete-workflow-id stringArray     destination workflow ID to remove from the target project, repeatable; needs workflow:delete and must not match a --workflow-id (default: none)
      --file string                        project package archive (.n8np) to import from: a file path, or - for stdin (required; stdin requires --yes)
  -h, --help                               help for import-selection
      --id-policy string                   ID for newly created workflows, always sent: source reuses the package ID; new mints a fresh ID but only instances before n8n 2.47 accept it (default: source) (default "source")
      --idempotency-key string             retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --output string                      output format: text or json (JSON is the full import result) (default "text")
      --overwrite-deletion-policy string   how --delete-workflow-id removes workflows: archive (recoverable) or hard-delete (also drops executions permanently; prefer archive); needs --delete-workflow-id (default: server default archive)
      --project-id string                  source project ID as it appears in the package; also the target project ID, created if missing (required)
      --url string                         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --workflow-id stringArray            source workflow ID to import from --project-id, repeatable (default: none, for a delete-only run with --delete-workflow-id)
      --yes                                confirm importing and deleting the listed workflows without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n package](n8n_package.md)	 - Export and import n8n packages (beta)

