## n8n tag list

List tags

### Synopsis

List the tags of the selected instance, one cursor-paginated page at a time.
The page carries a next cursor; pass it to --cursor for the following page, or
use --all to follow every cursor, capped at 10,000 tags.

Use this to find the tag ID that get, update and delete take. Requires tag:list.

Script views print rows only, with no header, count or blank lines, so stdout
pipes cleanly; the next cursor and any empty-result hint go to stderr, and an
empty result prints nothing:
  --quiet  one tag ID per line, printed exactly
  --brief  ID and name, separated by a literal tab
--brief escapes backslash, tab, newline, carriage return and other control
characters in fields as \\, \t, \n, \r and \uXXXX; use --output json for exact
values. The views combine neither with each other nor with --output json.

```
n8n tag list [flags]
```

### Examples

```
  n8n tag list
  n8n tag list --brief
  n8n tag list --all --quiet
  n8n tag list --limit 50 --output json
  n8n tag list --cursor NEXT_CURSOR
  n8n tag list --all
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 tags)
      --brief            print only ID and name, tab-separated and escaped, with no header (default: full table)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        tags per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --quiet            print only tag IDs, one per line, with no header (default: full table)
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n tag](n8n_tag.md)	 - Manage workflow tags

