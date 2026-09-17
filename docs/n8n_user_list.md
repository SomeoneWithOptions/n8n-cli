## n8n user list

List instance or project users

### Synopsis

List users visible to the selected credential. Use --project-id to filter to
one project's members and --include-role to request global roles. The API is
cursor-paginated; --offset selects a starting position, while --cursor resumes
a previous response. They cannot be combined.

--all follows cursors and collects at most 10,000 users. Instance-wide listing
may require the instance owner. Requires user:list.

```
n8n user list [flags]
```

### Examples

```
  n8n user list --include-role
  n8n user list --project-id PROJECT_ID --limit 50
  n8n user list --offset 100 --output json
  n8n user list --all --include-role --output json
```

### Options

```
      --all                 follow every page instead of one (maximum 10,000 users)
      --context string      saved context to use (default: the current context; see 'n8n config context list')
      --cursor string       pagination cursor returned by a previous list (default: first page; cannot combine with --offset)
  -h, --help                help for list
      --include-role        include each user's global role (default: role is omitted by API)
      --limit int           users per API page, 1 to 250 (default: server default of 100)
      --offset int          users to skip before first page, zero or greater (default: 0; cannot combine with --cursor)
      --output string       output format: text or json (JSON is a paginated user object) (default "text")
      --project-id string   return members of this project ID (default: all visible instance users)
      --url string          instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n user](n8n_user.md)	 - Manage instance users and global roles

