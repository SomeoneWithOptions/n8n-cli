## n8n source-control

Preview, push, and pull source-controlled instance changes

### Synopsis

Preview and move the instance content tracked by the licensed Source Control
feature and its connected Git repository. Start with 'status' to preview the
pending changes in one direction, then 'push' to commit local files to the
remote branch or 'pull' to rewrite local content from the remote.

'push' mutates the remote Git branch; 'pull' mutates this instance and can
discard local changes when forced. Both ask for confirmation, and previewing
first is the workflow: 'status --direction push' before a push and
'status --direction pull' before a pull.

```
n8n source-control [flags]
```

### Examples

```
  n8n source-control status --direction push
  n8n source-control status --direction pull --output json
  n8n source-control push --commit-message "sync workflows" --file abc:workflow
  n8n source-control pull --auto-publish none --yes
```

### Options

```
  -h, --help   help for source-control
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n source-control pull](n8n_source-control_pull.md)	 - Pull remote Git changes into this instance
* [n8n source-control push](n8n_source-control_push.md)	 - Commit and push local changes to the Git remote
* [n8n source-control status](n8n_source-control_status.md)	 - Preview pending source-control changes

