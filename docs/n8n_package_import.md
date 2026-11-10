## n8n package import

Import an n8n package into a project (beta)

### Synopsis

Import a gzip-compressed tar package (.n8np) into the target project:
Agent-only, workflow-only, mixed, or whole-project packages (Agents need
the Agents module). Send the archive with --file FILE or --file - for
stdin, route it with --project-id and --folder-id (omit both for the
caller's personal project root), and state --conflict-policy explicitly:
new-version replaces matching Agents and workflows (and saves a changed
skill of an imported Agent as a new version, which every Agent following
that skill picks up), fail rejects the import when any matches, and skip
leaves them unchanged.

n8n 2.47 renamed the workflow-only policies to --conflict-policy,
--id-policy, and --publishing-policy, which cover Agents and workflows;
the old --workflow-* flags remain as deprecated aliases. Older instances
get the old field names automatically, which needs --file PATH: stdin
cannot be sent twice.

Beta: breaking changes may still occur without a major version bump.
This mutates the instance: imported Agents and workflows arrive per
--publishing-policy (a failed publication keeps the imported definition
and is reported per entity), new-version follows the package archive
state (archiving needs workflow:delete), folder overwrite can remove
workflows the package does not contain (archive is recoverable,
hard-delete also drops executions permanently, so prefer archive),
missing credentials become empty stubs unless --credential-missing-mode
is must-preexist or create-with-values, --credential-conflict-policy
overwrite writes the package's credential name and expression values
to matched credentials, --data-table-schema-policy
overwrite permanently drops the values in removed or retyped columns,
and folders, tags, variables, or tables may be created or overwritten.
Interactive runs ask for confirmation and non-interactive runs require
--yes; --file - also requires --yes because stdin cannot answer.
Credentials created from incomplete package data are reported on
stderr and keep their workflows unpublished until completed. When the
import is blocked (409 or 422), each issue is listed on stderr, and
Agent channels that need setup are listed with their setup URL. Needs
workflow:import for workflows and agent:import plus agent:read for
Agents (both for mixed packages; agent:create, agent:update,
agent:publish, or agent:unpublish for the planned Agent actions),
workflow:activate or workflow:deactivate when publication changes, plus,
depending on the package and the policies, credential, variable, tag,
data-table, folder, skill, and delete scopes, and the Packages,
Variables, and Folders features where the contents require them.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr). A retry must send
the same archive under the same file name with the same flags.

```
n8n package import [flags]
```

### Examples

```
  n8n package import --file workflows.n8np --conflict-policy new-version --yes
  n8n package import --file project.n8np --conflict-policy fail --project-id Ox8O54VQrmBrb4qL --yes --output json
  n8n package import --file agent.n8np --conflict-policy skip --publishing-policy match-source --yes
```

### Options

```
      --bindings string                     explicit credential bindings as JSON, e.g. '{"credentials":{"pkg-id":"target-id"}}' (default: server default {})
      --conflict-policy string              matching Agent and workflow handling, required: new-version, fail, or skip (before n8n 2.47 workflows only)
      --context string                      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --credential-conflict-policy string   matched credential whose package values differ: keep-existing never changes it, overwrite writes the package name and expression values to credentials a project of the import owns (keeps secrets, other fields, and OAuth connections; reported in credentials.updated), fail rejects the import with a 409 credential-conflict; overwrite and fail need credential:update on matched credentials (default: server default keep-existing)
      --credential-matching-mode string     how package credential references match: id-only, name-and-type, or type-only (default: server default id-only)
      --credential-missing-mode string      unresolvable credential handling: must-preexist rejects, create-stub creates empty placeholders, create-with-values seeds them from the package's expression values (needs credential:create; unconfirmed data lands in credentials.incomplete and blocks publishing) (default: server default create-stub)
      --data-table-matching-mode string     data-table matching: by-id, the only mode (default: server default by-id)
      --data-table-missing-mode string      missing data-table handling: create, must-preexist, or do-nothing (default: server default create)
      --data-table-schema-policy string     matched table schema handling: keep-existing accepts extra target columns, fail rejects any difference, overwrite reshapes the table to the package and permanently drops values in removed or retyped columns for every workflow using it, overwrite-non-destructive applies the same changes but rejects removals and retypes; rows are kept (default: server default keep-existing)
      --file string                         package archive (.n8np) to import: a file path, or - for stdin (required; stdin requires --yes)
      --folder-conflict-policy string       existing folder handling: merge, fail, or overwrite; empty follows --project-conflict-policy on project packages, merge otherwise (default: follow)
      --folder-id string                    folder within the target project, from 'n8n folder list PROJECT_ID' (default: the project root)
  -h, --help                                help for import
      --id-policy string                    ID for newly created Agents and workflows, always sent: source reuses the package ID; new mints a fresh ID but only instances before n8n 2.47 accept it (default: source) (default "source")
      --idempotency-key string              retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --missing-node-type-mode string       unknown node type handling: fail rejects before anything is written, import-anyway imports without publishing those workflows (default: server default fail)
      --output string                       output format: text or json (JSON is the full import result) (default "text")
      --overwrite-deletion-policy string    how folder overwrite removes missing workflows: archive (recoverable) or hard-delete (also drops executions; prefer archive) (default: server default archive)
      --project-conflict-policy string      existing project handling for project packages: merge, fail, or overwrite (default: server default merge)
      --project-id string                   target project ID, from 'n8n project list' (default: the caller's personal project)
      --publishing-policy string            post-import publishing of Agents and workflows: preserve-published-state, match-source, publish-all, or unpublish-all (default: server default preserve-published-state)
      --tag-conflict-policy string          conflicted tag handling: skip, fail, or rename (default: server default skip)
      --tag-missing-mode string             missing tag handling: create or do-nothing (default: server default create)
      --url string                          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --variable-conflict-policy string     differing variable value handling: keep-existing, overwrite (may rewrite globals), or fail (default: server default keep-existing)
      --variable-missing-mode string        missing variable handling: do-nothing, must-preexist, create-stub, or create-with-value (default: server default create-with-value)
      --variable-parent-policy string       where workflow and folder packages create missing variables: project or global; omit for project packages, which reject it (default: import target project)
      --yes                                 confirm importing, publishing, archiving, deleting, and overwriting without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n package](n8n_package.md)	 - Export and import n8n packages (beta)

