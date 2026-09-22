## n8n source-control status

Preview pending source-control changes

### Synopsis

Preview the pending changes between the instance and the connected Git
repository in one direction, without changing anything. --direction push
previews what a push would send; --direction pull previews what a pull would
bring in, including files marked with a conflict.

Run this before 'push' to select file entries and before 'pull' to see what
the remote would overwrite. A file entry carries its repository file path, its
object ID and type, its change status, and whether it conflicts. Requires
sourceControl:read and a licensed, connected Source Control feature.

```
n8n source-control status [flags]
```

### Examples

```
  n8n source-control status --direction push
  n8n source-control status --direction pull
  n8n source-control status --direction push --output json
```

### Options

```
      --context string     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --direction string   preview direction: push or pull (required)
  -h, --help               help for status
      --output string      output format: text or json (JSON is the preview object with data) (default "text")
      --url string         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n source-control](n8n_source-control.md)	 - Preview, push, and pull source-controlled instance changes

