## n8n execution list

List executions with status, workflow and time filters

### Synopsis

List executions one cursor-paginated page at a time. Pass nextCursor to
--cursor, or use --all to follow every page, capped at 10,000 metadata rows.

Filter by status, workflow, project, or an RFC3339 start-time range. By default
the response contains metadata only. --include-data adds potentially large node
input/output and workflow data, so it cannot be combined with --all; page through
such results explicitly. Oversized data remains omitted unless
--ignore-data-size-limit is given. Requesting unredacted data additionally needs
execution:reveal. Listing requires execution:list.

```
n8n execution list [flags]
```

### Examples

```
  n8n execution list
  n8n execution list --status error --workflow-id WORKFLOW_ID
  n8n execution list --started-after 2026-09-17T00:00:00Z --limit 50
  n8n execution list --include-data --limit 10 --output json
  n8n execution list --all --output json
```

### Options

```
      --all                            follow every metadata page instead of one (maximum 10,000 executions; incompatible with --include-data)
      --context string                 saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string                  pagination cursor returned by a previous list (default: first page)
  -h, --help                           help for list
      --ignore-data-size-limit         return detailed data even when it exceeds the instance display-size limit (default: omit oversized data)
      --include-data                   include detailed node input/output and saved workflow data (default: metadata only)
      --limit int                      executions per API page, 1 to 250 (default: server default of 100)
      --output string                  output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --project-id string              only executions in this project, from 'n8n project list' (default: every accessible project)
      --redact-execution-data string   detailed-data redaction: true always redacts, false reveals and needs execution:reveal (default: workflow policy)
      --started-after string           only executions started after this RFC3339 timestamp, e.g. 2026-09-17T00:00:00Z (default: no lower bound)
      --started-before string          only executions started before this RFC3339 timestamp, e.g. 2026-09-18T00:00:00Z (default: no upper bound)
      --status string                  execution status: canceled, crashed, error, new, running, success, unknown, or waiting (default: every status)
      --url string                     instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --workflow-id string             only executions of this workflow, from 'n8n workflow list' (default: every workflow)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

