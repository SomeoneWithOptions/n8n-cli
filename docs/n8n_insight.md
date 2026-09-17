## n8n insight

Read aggregate execution insights

### Synopsis

Read aggregate execution insights for a selected period and optional project.
The summary reports total and failed executions, failure ratio, estimated time
saved in minutes, average run time in milliseconds, and each metric's deviation
from the preceding period. A null deviation means no comparison is available.

Use 'summary' with RFC3339 date bounds and --project-id. Omitting dates uses the
server defaults of seven days ago through now. Requires insights:read.

```
n8n insight [flags]
```

### Examples

```
  n8n insight summary
  n8n insight summary --start-date 2026-09-01T00:00:00Z --end-date 2026-09-17T00:00:00Z
  n8n insight summary --project-id PROJECT_ID --output json
```

### Options

```
  -h, --help   help for insight
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n insight summary](n8n_insight_summary.md)	 - Show aggregate execution insights

