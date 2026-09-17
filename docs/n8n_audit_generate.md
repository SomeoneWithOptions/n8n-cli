## n8n audit generate

Generate a security audit of the instance

### Synopsis

Generate a security audit of the instance.

The instance inspects its own credentials, database nodes, community and
filesystem nodes, and instance settings, and reports the risks it finds as
one report per category. Nothing is created, changed or deleted: the report
is derived from data that already exists, so this is safe on production.

Narrow the audit with --category, repeated or comma-separated; the documented
categories are credentials, database, nodes, filesystem and instance. Use
--days-abandoned to say how long a workflow may go unexecuted before the
credentials and instance reports call it abandoned; omit it to keep the
instance default.

A clean audit reports no findings and still exits 0. Text output is a summary
table plus each finding's title, recommendation and locations; --output json
emits the instance's own report document, including the per-risk fields the
summary leaves out, and is the stable form for scripts and agents.

Requires the securityAudit:generate scope; check it with
'n8n discover --resource audit'.

```
n8n audit generate [flags]
```

### Examples

```
  n8n audit generate
  n8n audit generate --category credentials --category nodes
  n8n audit generate --days-abandoned 30 --output json
  n8n audit generate --output json | jq -r '.["Nodes Risk Report"].sections[].title'
  n8n audit generate --context production
```

### Options

```
      --category strings     risk category to audit, repeatable or comma-separated: credentials, database, nodes, filesystem, instance (default: all categories)
      --context string       saved context to use (default: the current context; see 'n8n config context list')
      --days-abandoned int   days without an execution before a workflow counts as abandoned, e.g. 90 (default 0: use the instance setting)
  -h, --help                 help for generate
      --output string        output format: text or json (default text; json is the instance report verbatim and is stable for scripting) (default "text")
      --url string           instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n audit](n8n_audit.md)	 - Generate security audits of an n8n instance

