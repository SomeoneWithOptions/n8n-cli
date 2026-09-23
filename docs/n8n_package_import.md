## n8n package import

Import an n8n package into a project (beta)

### Synopsis

Import a gzip-compressed tar package (.n8np) into the target project.
Send the archive with --file FILE or --file - for stdin, route it with
--project-id and --folder-id (omit both for the caller's personal project
root), and state --workflow-conflict-policy explicitly: new-version
updates matching workflows, fail rejects the import when any matches, and
skip leaves matching workflows unchanged.

Beta: breaking changes may still occur without a major version bump.
This mutates the instance: imported workflows arrive per
--workflow-publishing-policy, new-version follows the package archive
state (archiving needs workflow:delete), folder overwrite can remove
workflows the package does not contain (archive is recoverable,
hard-delete also drops executions permanently, so prefer archive),
missing credentials become empty stubs unless --credential-missing-mode
must-preexist, and folders, tags, variables, or tables may be created or
overwritten. Interactive runs ask for confirmation and non-interactive
runs require --yes; --file - also requires --yes because stdin cannot
answer. Needs workflow:import plus, depending on the package and the
policies, variable, tag, data-table, folder, and delete scopes, and the
Packages, Variables, and Folders features where the contents require
them.

```
n8n package import [flags]
```

### Examples

```
  n8n package import --file workflows.n8np --workflow-conflict-policy new-version --yes
  n8n package import --file project.n8np --workflow-conflict-policy fail --project-id Ox8O54VQrmBrb4qL --yes --output json
  n8n package import --file update.n8np --workflow-conflict-policy skip --workflow-publishing-policy publish-all --yes
```

### Options

```
      --bindings string                     explicit credential bindings as JSON, e.g. '{"credentials":{"pkg-id":"target-id"}}' (default: server default {})
      --context string                      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --credential-matching-mode string     how package credential references match: id-only, name-and-type, or type-only (default: server default id-only)
      --credential-missing-mode string      unresolvable credential handling: must-preexist rejects, create-stub creates empty placeholders (default: server default create-stub)
      --data-table-matching-mode string     data-table matching: by-id, the only mode (default: server default by-id)
      --data-table-missing-mode string      missing data-table handling: create, must-preexist, or do-nothing (default: server default create)
      --data-table-schema-policy string     matched table schema strictness: keep-existing or fail (default: server default keep-existing)
      --file string                         package archive (.n8np) to import: a file path, or - for stdin (required; stdin requires --yes)
      --folder-conflict-policy string       existing folder handling: merge, fail, or overwrite; empty follows --project-conflict-policy on project packages, merge otherwise (default: follow)
      --folder-id string                    folder within the target project, from 'n8n folder list PROJECT_ID' (default: the project root)
  -h, --help                                help for import
      --missing-node-type-mode string       unknown node type handling: fail rejects before anything is written, import-anyway imports without publishing those workflows (default: server default fail)
      --output string                       output format: text or json (JSON is the full import result) (default "text")
      --overwrite-deletion-policy string    how folder overwrite removes missing workflows: archive (recoverable) or hard-delete (also drops executions; prefer archive) (default: server default archive)
      --project-conflict-policy string      existing project handling for project packages: merge, fail, or overwrite (default: server default merge)
      --project-id string                   target project ID, from 'n8n project list' (default: the caller's personal project)
      --tag-conflict-policy string          conflicted tag handling: skip, fail, or rename (default: server default skip)
      --tag-missing-mode string             missing tag handling: create or do-nothing (default: server default create)
      --url string                          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --variable-conflict-policy string     differing variable value handling: keep-existing, overwrite (may rewrite globals), or fail (default: server default keep-existing)
      --variable-missing-mode string        missing variable handling: do-nothing, must-preexist, create-stub, or create-with-value (default: server default create-with-value)
      --variable-parent-policy string       where workflow and folder packages create missing variables: project or global; omit for project packages, which reject it (default: import target project)
      --workflow-conflict-policy string     matching workflow handling, required: new-version, fail, or skip
      --workflow-id-policy string           ID for newly created workflows, always sent: source reuses the package ID for promotions, new mints a fresh ID for repeat imports (default: source) (default "source")
      --workflow-publishing-policy string   post-import publishing: preserve-published-state, match-source, publish-all, or unpublish-all (default: server default preserve-published-state)
      --yes                                 confirm importing, publishing, archiving, deleting, and overwriting without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n package](n8n_package.md)	 - Export and import n8n packages (beta)

