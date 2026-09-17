## n8n execution

Inspect, stop, retry and delete workflow executions

### Synopsis

Manage workflow executions and their annotation tags. Start with 'list' to
find execution IDs, then use 'get' for one run and --include-data only when its
node input and output are needed. Detailed run data can be large; list refuses
to combine it with --all, and the server omits oversized data unless explicitly
overridden.

'stop' cancels one active execution, 'stop-many' cancels a filtered set, and
'retry' creates a new run from a failed one. 'delete' permanently removes the
stored run. Stop and delete actions ask for confirmation. Execution annotation
tags are read and replaced under the 'tag' subgroup.

```
n8n execution [flags]
```

### Examples

```
  n8n execution list --status error
  n8n execution get EXECUTION_ID --include-data --output json
  n8n execution retry EXECUTION_ID
  n8n execution stop EXECUTION_ID
  n8n execution tag list EXECUTION_ID
  n8n execution delete EXECUTION_ID --yes
```

### Options

```
  -h, --help   help for execution
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n execution delete](n8n_execution_delete.md)	 - Permanently delete one stored execution
* [n8n execution get](n8n_execution_get.md)	 - Show one execution
* [n8n execution list](n8n_execution_list.md)	 - List executions with status, workflow and time filters
* [n8n execution retry](n8n_execution_retry.md)	 - Retry a failed execution as a new run
* [n8n execution stop](n8n_execution_stop.md)	 - Stop one queued, running, or waiting execution
* [n8n execution stop-many](n8n_execution_stop-many.md)	 - Stop active executions matching filters
* [n8n execution tag](n8n_execution_tag.md)	 - Read and replace annotation tags on an execution

