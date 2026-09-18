## n8n package

Export and import n8n packages (beta)

### Synopsis

Move workflows, folders, and projects between instances as gzipped tar
packages (.n8np). Start with 'export' to write a package file from this
instance, then 'import' to read one back into a project.

The whole group is beta: breaking changes may still occur without a major
version bump. Export needs the licensed Packages feature. Import can need
the Variables and Folders features as well, and it may publish, archive,
hard-delete workflows and executions, create credential stubs, and
overwrite folders, tags, variables, or tables, so it always asks for
confirmation first.

```
n8n package [flags]
```

### Examples

```
  n8n package export --workflow-id abc123 --out workflows.n8np
  n8n package export --project-id Ox8O54VQrmBrb4qL --out project.n8np --output json
  n8n package import --file workflows.n8np --workflow-conflict-policy new-version --yes
```

### Options

```
  -h, --help   help for package
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n package export](n8n_package_export.md)	 - Export workflows, folders, or projects as an n8n package (beta)
* [n8n package import](n8n_package_import.md)	 - Import an n8n package into a project (beta)

