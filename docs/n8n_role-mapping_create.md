## n8n role-mapping create

Create a role-mapping rule

### Synopsis

Create one rule from strict JSON supplied by --input. Required fields are
expression (the claim expression to match), role (an existing role slug, at
most 128 characters), and type (instance or project).

Optional order is the 0-based position within that type's evaluation order;
omitting it appends the rule to the end. Optional projectIds names the projects
a project rule grants its role on and is refused on an instance rule. Unknown
fields and trailing JSON documents are refused before transport. Run
'n8n role list' for valid slugs. Requires roleMappingRule:create.

```
n8n role-mapping create [flags]
```

### Examples

```
  n8n role-mapping create --input rule.json
  printf '%s\n' '{"expression":"groups contains \"admins\"","role":"global:admin","type":"instance"}' | n8n role-mapping create --input -
  printf '%s\n' '{"expression":"dept == \"ops\"","role":"project:admin","type":"project","order":0,"projectIds":["PROJECT_ID"]}' | n8n role-mapping create --input - --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     role-mapping rule JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the rule returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n role-mapping](n8n_role-mapping.md)	 - Manage identity-provider role-mapping rules

