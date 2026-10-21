## n8n package export

Export agents, workflows, folders, or projects as an n8n package (beta)

### Synopsis

Export agents, workflows, and/or folders, or whole projects, as a gzipped
tar archive (.n8np) streamed to a local file. Pass --agent-id,
--workflow-id, and/or --folder-id for loose entities, or --project-id for
whole projects, but not both groups in the same request; at least one ID
is required. Each exported folder includes its nested folders, and whole
projects include their agents when the agents module is enabled.

Beta: breaking changes may still occur without a major version bump.
Requires the licensed Packages feature: agent exports need agent:export
and the agents module, workflow and folder exports need workflow:export,
project exports need project:export, and exports that reference
variables with values included also need variable:list. Workflow and
agent dependencies outside the selection must be included or the export
is rejected under the default --dependency-policy fail. The archive may
bundle variable values and credential expressions, so it is written with
owner-only file permissions and the destination file is overwritten.

Pass the selection as flags, or the whole request document with --input
FILE or --input - (capped at 1 MiB, decoded strictly). --out is always a
local path and is never part of --input: a file path streams the archive
there, while --out - writes the raw gzip bytes to stdout and moves the
summary to stderr. Exporting reads instance content and changes nothing
server-side. Follow with 'n8n package import' to restore the archive.

```
n8n package export [flags]
```

### Examples

```
  n8n package export --workflow-id 2tUt1wbLX592XDdX --out workflows.n8np
  n8n package export --workflow-id a --folder-id 9xKp2mNqRzAbCdEf --out mixed.n8np
  n8n package export --project-id Ox8O54VQrmBrb4qL --dependency-policy include-in-package --out project.n8np --output json
  n8n package export --agent-id uT9LdQx7rK2MvB4f --version-policy prefer-published --out agent.n8np
  n8n package export --input selection.json --out workflows.n8np
```

### Options

```
      --agent-id stringArray              agent to include, repeatable up to 300; needs the agents module (cannot combine with --project-id or --input)
      --context string                    saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --credential-export-policy string   credential data handling: expression-values-only or no-values; literal values never travel (default: server default expression-values-only)
      --dependency-policy string          workflow and agent dependencies outside the selection: fail, reference-only, or include-in-package (default: server default fail)
      --folder-id stringArray             folder to include with nested folders, repeatable up to 300 (cannot combine with --project-id or --input)
  -h, --help                              help for export
      --include-archived                  include archived workflows in folder and project exports; listed workflows always export (default: false)
      --include-tags                      bundle workflow tags, else no tag files or references (default: true) (default true)
      --include-variable-values           bundle referenced variable values, else names only (default: true) (default true)
      --input string                      export selection as JSON: a file path, or - for stdin (cannot combine with selection or policy flags; maximum 1 MiB)
      --out string                        local destination for the .n8np archive: a file path, or - for stdout raw bytes (required)
      --output string                     summary format: text or json (JSON is the counts, filename, byte size, and local file) (default "text")
      --project-id stringArray            project to include with its workflows and agents, repeatable up to 300 (cannot combine with --agent-id, --workflow-id, --folder-id, or --input)
      --url string                        instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --version-policy string             which workflow and agent version travels: published-strict, prefer-published, ignore-unpublished, or latest (default: server default latest)
      --workflow-id stringArray           workflow to include, repeatable up to 300 (cannot combine with --project-id or --input)
```

### SEE ALSO

* [n8n package](n8n_package.md)	 - Export and import n8n packages (beta)

