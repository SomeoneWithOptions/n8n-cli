## n8n insight summary

Show aggregate execution insights

### Synopsis

Show total and failed executions, failure ratio, estimated time saved, and
average run time for a selected period. Each metric includes its deviation from
the preceding period; null means no comparison is available. Ratios remain raw
ratios rather than being converted to percentages.

--start-date and --end-date accept RFC3339 timestamps and may be used
independently. Omitting them lets the server select seven days ago through now.
--project-id limits the summary to one project; omitting it includes every
accessible project. Requires insights:read. JSON output preserves API field
names, numeric values, units, and null deviations for stable machine use.

```
n8n insight summary [flags]
```

### Examples

```
  n8n insight summary
  n8n insight summary --start-date 2026-09-01T00:00:00Z
  n8n insight summary --start-date 2026-09-01T00:00:00Z --end-date 2026-09-17T23:59:59Z --project-id PROJECT_ID
  n8n insight summary --project-id PROJECT_ID --output json
```

### Options

```
      --context string      saved context to use (default: the current context; see 'n8n config context list')
      --end-date string     period end as an RFC3339 timestamp (default: server default of now)
  -h, --help                help for summary
      --output string       output format: text or json (JSON preserves metric values, units, and null deviations) (default "text")
      --project-id string   only include this project, from 'n8n project list' (default: every accessible project)
      --start-date string   period start as an RFC3339 timestamp (default: server default of seven days ago)
      --url string          instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n insight](n8n_insight.md)	 - Read aggregate execution insights

