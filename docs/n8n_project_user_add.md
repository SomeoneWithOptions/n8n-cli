## n8n project user add

Add one or more users to a project

### Synopsis

Add users to one project, each with a project role. Pass --user once per member
as USER_ID=ROLE, or supply a strict JSON array of {"userId","role"} objects with
--input FILE or --input -. The two ways are mutually exclusive and one of them is
required.

User IDs come from 'n8n user list'; project role slugs come from 'n8n role list'.
Every member is sent in one request, so the API accepts or rejects the batch as a
whole, and a user already in the project is rejected rather than updated; change
an existing member with 'n8n project user role set'. Requires
project:manageMembers.

```
n8n project user add <project-id> [flags]
```

### Examples

```
  n8n project user add PROJECT_ID --user USER_ID=project:viewer
  n8n project user add PROJECT_ID --user USER_ONE=project:editor --user USER_TWO=project:viewer
  n8n project user add PROJECT_ID --input members.json
  printf '%s\n' '[{"userId":"USER_ID","role":"project:viewer"}]' | n8n project user add PROJECT_ID --input - --output json
```

### Options

```
      --context string     saved context to use (default: the current context; see 'n8n config context list')
  -h, --help               help for add
      --input string       member JSON array file path, or - for stdin (cannot combine with --user; maximum 1 MiB)
      --output string      output format: text or json (acknowledgement lists the members sent) (default "text")
      --url string         instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --user stringArray   member to add as USER_ID=ROLE, repeatable (cannot combine with --input)
```

### SEE ALSO

* [n8n project user](n8n_project_user.md)	 - Manage project members

