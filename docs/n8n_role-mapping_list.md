## n8n role-mapping list

List role-mapping rules

### Synopsis

List role-mapping rules one cursor-paginated page at a time. Use --all to
follow every cursor, capped at 10,000 rules.

The order column is the rule's position within its own type, so an unfiltered
list contains two independent sequences that both start at 0. Pass
--type instance or --type project to read one evaluation order on its own.
Use a returned ID with update, move, or delete. Requires roleMappingRule:list.

```
n8n role-mapping list [flags]
```

### Examples

```
  n8n role-mapping list
  n8n role-mapping list --type project --limit 50
  n8n role-mapping list --all --output json
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 rules)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        rules per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --type string      return one evaluation order: instance or project (default: both)
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n role-mapping](n8n_role-mapping.md)	 - Manage identity-provider role-mapping rules

