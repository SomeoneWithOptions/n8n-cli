## n8n promotion project change-details

Read the content of changed resources in a project preview

### Synopsis

Read the content before and after the change of up to 50 resources of a
change preview (n8n 2.49 and later), without changing anything. Start with
'n8n promotion project changes PROJECT_ID DIRECTION --output json >
changes.json'. --from changes.json takes the preview's commitSha and, without
--key, every changed resource; it also reports resources whose revision
differs from the preview, which changed after it. Or pass --commit-sha with
repeated --key values from the KEY column; a preview without commitSha needs
--from. The branch side is read at that commit, the instance side now.
Credential content holds expressions only. Variable values are left out
unless the API key and the user have variable:list. Apply requires
promotions:apply, promote promotions:promote.

Text lists key, type, name, change, and data-table column changes; JSON is
the full response with before and after content and missingKeys, the
requested keys neither side holds anymore.

```
n8n promotion project change-details <project-id> <apply|promote> [flags]
```

### Examples

```
  n8n promotion project change-details PROJECT_ID apply --from changes.json --output json
  n8n promotion project change-details PROJECT_ID promote --commit-sha SHA --key KEY
```

### Options

```
      --commit-sha string   commitSha of the preview, full lowercase 40- or 64-digit hex (default: from --from; required without it)
      --context string      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --from string         'promotion project changes' JSON file, or - for stdin: supplies commitSha and, without --key, every changed resource (default: none; --commit-sha and --key required)
  -h, --help                help for change-details
      --key stringArray     resource key from the preview; repeat for each, at most 50 (default: every changed resource in --from)
      --output string       output format: text or json (JSON is the full response with before and after content) (default "text")
      --url string          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

