## n8n workflow diff

Compare saved workflows across two contexts

### Synopsis

Compare saved workflow definitions without modifying either instance.

Select both saved contexts explicitly. This command uses each context's saved URL
and credential, ignoring N8N_CONTEXT, N8N_URL and environment credentials.
Pass --from-id and --to-id, or --name to resolve one exact name independently
on each side. Name lookup follows every page (maximum 10,000 results); missing
or ambiguous matches fail. Project filters apply only to name lookup. Requires
workflow:read on both instances, plus workflow:list when using --name.

Defaults compare name, description, nodes, connections, settings and nodeGroups.
Runtime staticData and pinned samples are opt-in. Other top-level fields,
including IDs, versions, timestamps, tags, sharing and metadata, are ignored.
Nodes match by shared ID, then unique exact name; IDs and node order are ignored.
Node positions, credential references and other array order remain significant.
Missing/null descriptions and empty collections normalize to empty values;
staticData retains null. Unknown nested properties are compared.

Output describes changes from source to target: added means present only on the
target. JSON includes schemaVersion, endpoints, fields, equal, summary and changes
with JSON Pointer paths and before/after values. Normalized nodes are objects
keyed by id:ID for shared IDs, otherwise name:NAME. Changes describe differences,
not an API patch. Text includes a unified diff of the same normalized definitions.

Published/archived status is reported separately and does not affect equality.
This compares saved definitions, not necessarily the versions running in production.
Exit codes: 0 equal, 1 different, 2 error, 130 canceled. --quiet suppresses stdout
but preserves diagnostics. Workflow parameters and included samples appear in output.

```
n8n workflow diff [flags]
```

### Examples

```
  n8n workflow diff --from-context staging --from-id AAA --to-context prod --to-id BBB
  n8n workflow diff --name "Invoice sync" --from-context staging --to-context prod
  n8n workflow diff --name "Invoice sync" --from-context staging --from-project-id DEV --to-context prod --to-project-id PROD --output json
  n8n workflow diff --from-context staging --from-id AAA --to-context prod --to-id BBB --only nodes,connections,settings --quiet
```

### Options

```
      --from-context string      source saved context (required; ignores global environment overrides)
      --from-id string           source workflow ID (required unless --name is given)
      --from-project-id string   limit source name lookup to this project (requires --name)
  -h, --help                     help for diff
      --include-pinned-data      include pinned sample data in the default comparison fields
      --include-static-data      include runtime static data in the default comparison fields
      --name string              exact workflow name to resolve on both sides; cannot be combined with IDs
      --only strings             compare only these comma-separated definition fields, replacing defaults and inclusion flags
      --output string            output format: text or json (versioned structural comparison) (default "text")
      --quiet                    suppress stdout; report differences through exit code (errors still go to stderr)
      --to-context string        target saved context (required; ignores global environment overrides)
      --to-id string             target workflow ID (required unless --name is given)
      --to-project-id string     limit target name lookup to this project (requires --name)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

